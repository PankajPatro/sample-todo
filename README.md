# Todo Event‑Sourced Example

This repository contains a minimal, **event‑sourced** micro‑service architecture that illustrates how a typical enterprise application can be decomposed into independent, loosely‑coupled layers.

## Services

| Service | Responsibility | Key Technology |
|---------|-----------------|----------------|
| **React frontend** | UI that submits events | React |
| **ASP.NET backend** | Handles UI requests, publishes events to RabbitMQ | C#, ASP.NET |
| **Go consumer (Persistence)** | Consumes events from RabbitMQ, persists to Postgres | Go, RabbitMQ, Postgres |
| **Cache service (Projection Cache)** | Maintains an in‑memory cache of projections, listens for updates from RabbitMQ | Go, RabbitMQ, Postgres |
| **RabbitMQ** | Message broker that decouples services |
| **Postgres** | Durable event store and projection persistence |
| **nginx** | Reverse proxy that exposes UI and API to the outside world |

> **Why this matters**
> The application demonstrates the principle of *separation of concerns*:
> - Each component only knows about its own responsibilities.
> - Changes to one layer (e.g., replacing the cache implementation) can be made without affecting the others.
> - The architecture is **fully async**; there are no synchronous calls between services.

## How it Works

1. **UI → C# API**
 - User actions are converted into events and sent to the C# API via nginx.

2. **C# API → RabbitMQ**
 - The C# API handles the UI requests, publishes events to RabbitMQ.

3. **C# API → Cache Service**
 - The C# API also acts as a proxy to the Cache Service, exposing the cached projections to the UI via HTTP/SSE.

4. **Cache Service (Projection Cache)**
 - Listens for updates from RabbitMQ and updates its cache in real time.
 - On startup, it hydrates its cache by replaying all events stored in Postgres.

5. **Cache Service → RabbitMQ**
 - The Cache Service listens for updates from RabbitMQ.

6. **Cache Service → Postgres**
 - The Cache Service hydrates its cache from Postgres on startup.

7. **Persistence (Go Consumer) → RabbitMQ**
 - Consumes events from RabbitMQ, persists to Postgres.

8. **Persistence (Go Consumer) → Postgres**
 - Persists events to Postgres.
## Running the Sample

```bash
# Build and start all services
docker compose up --build
```

The React app will be served at **`http://localhost:3000`**, and the API will be reachable at **`http://localhost:3000/api`**.

## `.gitignore`

The repository uses the following ignore rules to keep build artefacts, IDE settings, and third‑party dependencies out of source control:

```gitignore
# Backend
backend/bin/
sample.sln
backend/obj/
backend/.vs/
backend/.vscode/
backend/*.user

# Frontend
frontend/node_modules/
frontend/dist/
frontend/.vscode/
frontend/package-lock.json

# Service
go-projection-cache/projection-cache
```

Feel free to add more patterns as your environment grows.

## Further Reading

- Explore the **`frontend`** directort for the UI call to the backend
- Explore the **`backend`** directory for the ASP.NET API that handles UI requests and publishes events to RabbitMQ.
- The **`go-projection-cache`** service shows how a separate cache can be hydrated from the event store and stay in sync via RabbitMQ.
- The **`go-consumer`** folder contains the RabbitMQ consumer that writes events to Postgres.

