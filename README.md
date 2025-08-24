Todo event-sourced example

Services:
- React frontend served by nginx (port 3000)
- ASP.NET backend exposes /api/events (push events to RabbitMQ)
- Go consumer subscribes to RabbitMQ, stores events and projections in Postgres
- RabbitMQ
- Postgres

Run with: docker compose up --build
