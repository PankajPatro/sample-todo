package main

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/google/uuid"
	_ "github.com/lib/pq"
	amqp "github.com/rabbitmq/amqp091-go"
)

type Todo struct {
	ID        uuid.UUID `json:"id"`
	Title     string    `json:"title"`
	Completed bool      `json:"completed"`
	CreatedAt time.Time `json:"created_at"`
}

var (
	todosCache = make(map[uuid.UUID]Todo)
	cacheLock  sync.RWMutex
)

func main() {
	// DB connection
	db, err := sql.Open("postgres", os.Getenv("POSTGRES_CONN"))
	if err != nil {
		log.Fatal("DB connection failed:", err)
	}
	defer db.Close()

	// Hydrate cache by replaying events
	if err := hydrateCache(db); err != nil {
		log.Fatal("Cache hydration failed:", err)
	}

	// Connect RabbitMQ
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
		"todo-events", true, false, false, false, nil,
	)
	if err != nil {
		log.Fatal(err)
	}

	msgs, err := ch.Consume(q.Name, "", true, false, false, false, nil)
	if err != nil {
		log.Fatal(err)
	}

	// Listen to events async
	go func() {
		for d := range msgs {
			handleEvent(db, d.Body)
		}
	}()

	// HTTP Handlers
	http.HandleFunc("/todos", getTodosHandler)

	log.Println("Cache service running on :8081")
	log.Fatal(http.ListenAndServe(":8081", nil))
}

func hydrateCache(db *sql.DB) error {
	// Find last applied event timestamp
	var lastApplied time.Time
	err := db.QueryRow(`SELECT COALESCE(MAX(last_event_created_at), 'epoch') FROM todos`).Scan(&lastApplied)
	if err != nil {
		return err
	}

	log.Printf("Hydrating cache from events newer than %v\n", lastApplied)

	// Get all relevant events from persistence
	rows, err := db.Query(`SELECT type, payload, created_at FROM events WHERE created_at > $1 ORDER BY created_at ASC`, lastApplied)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var eventType string
		var payload []byte
		var createdAt time.Time

		if err := rows.Scan(&eventType, &payload, &createdAt); err != nil {
			return err
		}

		event := map[string]interface{}{
			"type":      eventType,
			"payload":   payload,
			"createdAt": createdAt,
		}
		data, _ := json.Marshal(event)
		handleEvent(db, data)
	}

	// Finally hydrate cache from todos table
	return hydrateFromTodos(db)
}

func hydrateFromTodos(db *sql.DB) error {
	rows, err := db.Query(`SELECT id, title, completed, created_at FROM todos`)
	if err != nil {
		return err
	}
	defer rows.Close()

	cacheLock.Lock()
	defer cacheLock.Unlock()

	todosCache = make(map[uuid.UUID]Todo)

	for rows.Next() {
		var t Todo
		if err := rows.Scan(&t.ID, &t.Title, &t.Completed, &t.CreatedAt); err != nil {
			return err
		}
		todosCache[t.ID] = t
	}
	log.Printf("Hydrated %d todos into memory\n", len(todosCache))
	return nil
}

func handleEvent(db *sql.DB, body []byte) {
	var msg struct {
		Type      string          `json:"type"`
		Payload   json.RawMessage `json:"payload"`
		CreatedAt time.Time       `json:"createdAt"`
	}
	if err := json.Unmarshal(body, &msg); err != nil {
		log.Println("Invalid event:", err)
		return
	}

	switch msg.Type {
	case "TodoCreated":
		var e struct {
			ID    uuid.UUID `json:"id"`
			Title string    `json:"title"`
		}
		if err := json.Unmarshal(msg.Payload, &e); err != nil {
			log.Println("Bad payload:", err)
			return
		}

		_, err := db.Exec(`
			INSERT INTO todos (id, title, completed, created_at, last_event_created_at)
			VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (id) DO NOTHING
		`, e.ID, e.Title, false, msg.CreatedAt, msg.CreatedAt)
		if err != nil {
			log.Println("DB insert error:", err)
			return
		}

		cacheLock.Lock()
		todosCache[e.ID] = Todo{ID: e.ID, Title: e.Title, Completed: false, CreatedAt: msg.CreatedAt}
		cacheLock.Unlock()

	case "TodoCompleted":
		var e struct {
			ID uuid.UUID `json:"id"`
		}
		if err := json.Unmarshal(msg.Payload, &e); err != nil {
			log.Println("Bad payload:", err)
			return
		}

		_, err := db.Exec(`
			UPDATE todos SET completed = true, last_event_created_at = $2 WHERE id = $1
		`, e.ID, msg.CreatedAt)
		if err != nil {
			log.Println("DB update error:", err)
			return
		}

		cacheLock.Lock()
		if t, ok := todosCache[e.ID]; ok {
			t.Completed = true
			todosCache[e.ID] = t
		}
		cacheLock.Unlock()
	}
}

func getTodosHandler(w http.ResponseWriter, r *http.Request) {
	cacheLock.RLock()
	defer cacheLock.RUnlock()

	var list []Todo
	for _, t := range todosCache {
		list = append(list, t)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(list)
}
