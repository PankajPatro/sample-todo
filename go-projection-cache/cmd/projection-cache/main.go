package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/google/uuid"
	_ "github.com/lib/pq"
	amqp "github.com/rabbitmq/amqp091-go"
	pb "github.com/todossample/projection-cache/protos"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"
)

type Todo struct {
	ID        uuid.UUID `json:"id"`
	Title     string    `json:"title"`
	Completed bool      `json:"completed"`
}

var (
	todosCache = make(map[uuid.UUID]Todo)
	cacheLock  sync.RWMutex
	updatesCh  = make(chan any, 100)
)

// The gRPC server struct.
type projectionServer struct {
	pb.UnimplementedProjectionServiceServer
	cache *sync.RWMutex
}

func (s *projectionServer) Subscribe(req *emptypb.Empty, stream pb.ProjectionService_SubscribeServer) error {
	log.Println("gRPC client connected")

	// Create and send the initial snapshot
	s.cache.RLock()
	allTodos := make([]*pb.Todo, 0)
	for _, t := range todosCache {
		allTodos = append(allTodos, &pb.Todo{
			Id:        t.ID.String(),
			Title:     t.Title,
			Completed: t.Completed,
		})
	}
	s.cache.RUnlock()

	snapshot := &pb.ProjectionEvent{
		Event: &pb.ProjectionEvent_Snapshot{
			Snapshot: &pb.Snapshot{Todos: allTodos},
		},
	}
	if err := stream.Send(snapshot); err != nil {
		log.Printf("Failed to send snapshot: %v", err)
		return err
	}

	// This loop will now correctly handle updates and disconnections
	for {
		select {
		// Case 1: Receive a new update from the channel
		case update := <-updatesCh:
			var event *pb.ProjectionEvent
			switch v := update.(type) {
			case Todo:
				event = &pb.ProjectionEvent{
					Event: &pb.ProjectionEvent_TodoUpdated{
						TodoUpdated: &pb.Todo{
							Id:        v.ID.String(),
							Title:     v.Title,
							Completed: v.Completed,
						},
					},
				}
			case string:
				event = &pb.ProjectionEvent{
					Event: &pb.ProjectionEvent_TodoRemovedId{
						TodoRemovedId: v,
					},
				}
			}
			if event != nil {
				// We wrap the stream.Send in a non-blocking check for the context.
				// This prevents the Send call from indefinitely blocking the select.
				select {
				case <-stream.Context().Done():
					log.Println("gRPC client disconnected")
					return nil
				default:
					log.Printf("Sending update to gRPC client: %+v", event)
					if err := stream.Send(event); err != nil {
						log.Printf("Failed to send update: %v", err)
						return err
					}
				}
			}
		// Case 2: The client has disconnected, as signaled by the context.
		case <-stream.Context().Done():
			log.Println("gRPC client disconnected")
			return nil
		}
	}
}

func main() {
	db, err := sql.Open("postgres", os.Getenv("POSTGRES_CONN"))
	if err != nil {
		log.Fatal("DB connection failed:", err)
	}
	defer db.Close()

	go func() {
		lis, err := net.Listen("tcp", ":50051")
		if err != nil {
			log.Fatalf("failed to listen: %v", err)
		}
		s := grpc.NewServer()
		pb.RegisterProjectionServiceServer(s, &projectionServer{
			cache: &cacheLock,
		})
		log.Printf("gRPC server listening at %v", lis.Addr())
		if err := s.Serve(lis); err != nil {
			log.Fatalf("failed to serve: %v", err)
		}
	}()

	// Perform initial hydration on startup (the "cold start")
	if err := populateProjection(db); err != nil {
		log.Fatal("Initial cache population failed:", err)
	}

	rmq, err := amqp.Dial(os.Getenv("RABBITMQ_URL"))
	if err != nil {
		log.Fatal("RabbitMQ connection failed:", err)
	}
	defer rmq.Close()

	ch, err := rmq.Channel()
	if err != nil {
		log.Fatal(err)
	}
	defer ch.Close()

	q, err := ch.QueueDeclare(
		"projection-events", true, false, false, false, nil,
	)
	if err != nil {
		log.Fatal(err)
	}

	msgs, err := ch.Consume(q.Name, "", true, false, false, false, nil)
	if err != nil {
		log.Fatal(err)
	}

	// Listen to events and process the delta
	go func() {
		for range msgs {
			// A message is received, now process the new events from the database
			log.Println("RabbitMQ message received, processing new events...")
			if err := processNewEvents(db); err != nil {
				log.Printf("Error processing new events: %v", err)
			}
		}
	}()

	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	http.HandleFunc("/ready", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	log.Println("Cache service running on 0.0.0.0:8081")
	log.Fatal(http.ListenAndServe("0.0.0.0:8081", nil))
}

// populateProjection performs a full cache rehydration from the 'todos' table.
// This is only called at startup for a cold start.
func populateProjection(db *sql.DB) error {
	rows, err := db.Query(`SELECT id, title, completed FROM todos WHERE deleted = false`)
	if err != nil {
		return err
	}
	defer rows.Close()

	cacheLock.Lock()
	defer cacheLock.Unlock()

	todosCache = make(map[uuid.UUID]Todo)
	for rows.Next() {
		var t Todo
		if err := rows.Scan(&t.ID, &t.Title, &t.Completed); err != nil {
			return err
		}
		todosCache[t.ID] = t
	}

	log.Printf("Initial hydration complete, %d todos in cache\n", len(todosCache))
	return nil
}

// processNewEvents queries for new events since the last timestamp and applies them incrementally.
func processNewEvents(db *sql.DB) error {
	var lastProcessedTimestamp time.Time
	err := db.QueryRow(`SELECT COALESCE(last_processed_timestamp, 'epoch') FROM projection_metadata`).Scan(&lastProcessedTimestamp)
	if err != nil {
		return fmt.Errorf("failed to get last processed timestamp: %w", err)
	}

	rows, err := db.Query(`SELECT event_id, type, payload, created_at FROM events WHERE created_at > $1 ORDER BY created_at ASC`, lastProcessedTimestamp)
	if err != nil {
		return err
	}
	defer rows.Close()

	var latestEventTimestamp time.Time

	for rows.Next() {
		var eventId sql.NullString
		var eventType string
		var payload []byte
		var createdAt time.Time

		if err := rows.Scan(&eventId, &eventType, &payload, &createdAt); err != nil {
			return err
		}

		// Apply the event to both the DB projection and the in-memory cache
		handleEventDetails(db, eventId, eventType, payload, createdAt)

		// Track the latest timestamp to checkpoint
		latestEventTimestamp = createdAt
	}

	if !latestEventTimestamp.IsZero() {
		_, err = db.Exec(`UPDATE projection_metadata SET last_processed_timestamp = $1`, latestEventTimestamp)
		if err != nil {
			return fmt.Errorf("failed to update last processed timestamp: %w", err)
		}
	}

	log.Println("Processed new events and updated timestamp.")
	return nil
}

// handleEventDetails updates the database and the in-memory cache based on a single event.
func handleEventDetails(db *sql.DB, eventId sql.NullString, eventType string, payload json.RawMessage, createdAt time.Time) {
	switch eventType {
	case "TodoCreated":
		var e struct {
			Title string `json:"title"`
		}
		if err := json.Unmarshal(payload, &e); err != nil {
			log.Println("Bad payload:", err)
			return
		}

		_, err := db.Exec(`
			INSERT INTO todos (id, title, completed, created_at, last_modified)
			VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (id) DO NOTHING
		`, eventId, e.Title, false, createdAt, createdAt)
		if err != nil {
			log.Println("DB insert error:", err)
			return
		}

		parsedUUID, err := convertToUUID(eventId)
		if err != nil {
			log.Fatalf("Error converting UUID: %v", err)
		}

		cacheLock.Lock()
		newTodo := Todo{ID: parsedUUID, Title: e.Title, Completed: false}
		todosCache[parsedUUID] = newTodo
		cacheLock.Unlock()
		updatesCh <- newTodo

	case "TodoUpdated":
		var e struct {
			ID uuid.UUID `json:"id"`
		}
		if err := json.Unmarshal(payload, &e); err != nil {
			log.Println("Bad payload:", err)
			return
		}

		_, err := db.Exec(`
			UPDATE todos SET completed = true, last_modified = $2 WHERE id = $1
		`, e.ID, createdAt)
		if err != nil {
			log.Println("DB update error:", err)
			return
		}

		cacheLock.Lock()
		if t, ok := todosCache[e.ID]; ok {
			t.Completed = true
			todosCache[e.ID] = t
			updatesCh <- t
		}
		cacheLock.Unlock()

	case "TodoRemoved":
		var e struct {
			ID uuid.UUID `json:"id"`
		}
		if err := json.Unmarshal(payload, &e); err != nil {
			log.Println("Bad payload:", err)
			return
		}

		_, err := db.Exec(`
			UPDATE todos SET deleted = true, last_modified = $2 WHERE id = $1
		`, e.ID, createdAt)
		if err != nil {
			log.Println("DB update error:", err)
			return
		}

		cacheLock.Lock()
		delete(todosCache, e.ID) // Corrected bug
		cacheLock.Unlock()
		updatesCh <- e.ID.String()
	}
}

func convertToUUID(s sql.NullString) (uuid.UUID, error) {
	if !s.Valid {
		return uuid.Nil, fmt.Errorf("cannot convert SQL NULL string to UUID")
	}

	parsedUUID, err := uuid.Parse(s.String)
	if err != nil {
		return uuid.Nil, fmt.Errorf("failed to parse UUID from string: %w", err)
	}

	return parsedUUID, nil
}
