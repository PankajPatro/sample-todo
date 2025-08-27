package main

import (
	"database/sql"
	"encoding/json"
	"log"
	"os"
	"time"

	"github.com/google/uuid"
	_ "github.com/lib/pq"
	amqp "github.com/rabbitmq/amqp091-go"
)

func nullableString(s string) sql.NullString {
	if s == "" {
		return sql.NullString{Valid: false}
	}
	return sql.NullString{String: s, Valid: true}
}

type Event struct {
	Type      string          `json:"type"`
	Payload   json.RawMessage `json:"payload"`
	EventId   string          `json:"eventId,omitempty"`
	Aggregate uuid.UUID       `json:"aggregate,omitempty"`
	CreatedAt time.Time       `json:"createdAt,omitempty"`
}

type ProjectionAction struct {
	Act bool `json:"act"`
}

func main() {
	amqpUrl := os.Getenv("RABBITMQ_URL")
	pgConn := os.Getenv("POSTGRES_CONN")
	if amqpUrl == "" {
		amqpUrl = "amqp://guest:guest@rabbitmq:5672/"
	}
	if pgConn == "" {
		pgConn = "host=postgres user=postgres password=postgres dbname=todos sslmode=disable"
	}

	db, err := sql.Open("postgres", pgConn)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	// Connect to rabbit
	conn, err := amqp.Dial(amqpUrl)
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()

	ch, err := conn.Channel()
	if err != nil {
		log.Fatal(err)
	}
	defer ch.Close()

	q, err := ch.QueueDeclare("events", true, false, false, false, nil)
	if err != nil {
		log.Fatal(err)
	}

	msgs, err := ch.Consume(q.Name, "", false, false, false, false, nil)
	if err != nil {
		log.Fatal(err)
	}

	forever := make(chan bool)

	go func() {
		for d := range msgs {
			var ev Event
			if err := json.Unmarshal(d.Body, &ev); err != nil {
				log.Printf("bad event: %v", err)
				d.Nack(false, false)
				continue
			}

			// dedupe by eventId if present
			if ev.EventId != "" {
				var exists bool
				err := db.QueryRow(`SELECT EXISTS(SELECT 1 FROM events WHERE event_id = $1)`, ev.EventId).Scan(&exists)
				if err != nil {
					log.Printf("db check event err: %v", err)
					d.Nack(false, true)
					continue
				}
				if exists {
					log.Printf("skipping already-processed event %s", ev.EventId)
					d.Ack(false)
					continue
				}
			}

			// determine aggregate id based on event type
			var aggId uuid.UUID
			// default payload string
			payload := string(ev.Payload)
			if ev.Type == "TodoCreated" {
				aggId = uuid.New()
			} else {
				// try to extract id from payload for update/remove events
				var maybe struct {
					Id string `json:"id"`
				}
				if err := json.Unmarshal(ev.Payload, &maybe); err != nil || maybe.Id == "" {
					// if we can't determine an id, nack and retry later
					log.Printf("invalid payload for %s: missing id", ev.Type)
					d.Nack(false, true)
					continue
				}
				parsed, err := uuid.Parse(maybe.Id)
				if err != nil {
					log.Printf("invalid aggregate id in payload: %v", err)
					d.Nack(false, true)
					continue
				}
				aggId = parsed
			}

			ev.Aggregate = aggId
			ev.CreatedAt = time.Now().UTC()
			ev.Payload = json.RawMessage(payload)
			// store event (include event_id for idempotency) and capture DB sequence id
			var seq int64
			err = db.QueryRow(`INSERT INTO events(aggregate_id,event_id,type,payload,created_at) VALUES($1,$2,$3,$4,$5) RETURNING id`, aggId, nullableString(ev.EventId), ev.Type, payload, time.Now().UTC()).Scan(&seq)
			if err != nil {
				log.Printf("db insert event err: %v", err)
				d.Nack(false, true)
				continue
			}
			var action = ProjectionAction{Act: true}

			body, _ := json.Marshal(action)

			ch.Publish(
				"", "projection-events", false, false,
				amqp.Publishing{
					ContentType: "application/json",
					Body:        body,
				},
			)

			d.Ack(false)
			log.Printf("processed event %s", ev.Type)
		}
	}()

	log.Println("consumer running")
	<-forever
}
