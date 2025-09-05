# 📝 Distributed Todo App — Architectural Showcase

![Demo](./Screencast%20From%202025-09-05%2022-11-04.gif)

---

## 📖 About

This project is a **simple Todo application**, but with a twist: it is intentionally designed to **showcase enterprise-grade system design thinking**.  

Most developers, when imagining an application, picture:
- A **Frontend** (UI) talking directly to a **Backend (API)**, which stores data in a **Database**.

While this works for small apps, real-world enterprise systems often require more nuanced **architectural patterns**.  

This project demonstrates:
- **Front-End For Back-End (BFF)** pattern.  
- Use of **RabbitMQ** for event-driven messaging.  
- A **Consumer** service for persistence to Postgres.  
- A **Projection Cache** for query optimization and decoupling.  
- **Docker Compose** for orchestrating services.  

The goal is not to provide a production-ready Todo app, but to showcase **how a developer should think architecturally** beyond just "UI + API + DB".

---

## 🏗️ High-Level Architecture

```
        ┌───────────┐
        │ Frontend  │
        └─────┬─────┘
              │
              ▼
        ┌───────────┐
        │   BFF     │ (ASP.NET Core + SSE)
        │  (Backend)│
        └───┬───┬───┘
            │   │
   Queries  │   │ Commands
            │   │
            ▼   ▼
   ┌───────────┐     ┌─────────────┐
   │ Projection │ <── │ RabbitMQ MQ │
   │   Cache    │     └──────┬──────┘
   └─────┬─────┘            │
         │                  │
   Read Models              │ Events
         │                  │
         ▼                  ▼
   ┌───────────┐     ┌───────────┐
   │   Backend │     │  Consumer │
   │  (queries)│     │ (subscribes,
   └───────────┘     │ persists) │
                      └─────┬─────┘
                            ▼
                      ┌───────────┐
                      │ Postgres  │
                      └───────────┘
```

---

### 🔑 Explanation of Flow

1. **Frontend → BFF (Backend)**  
   - User interacts with the UI.  
   - Reads go via SSE/API to the BFF.  
   - Writes (commands like *add todo*, *update todo*, *remove todo*) are sent to the BFF.

2. **BFF → RabbitMQ**  
   - Instead of writing to Postgres directly, the BFF publishes messages to RabbitMQ.  
   - This **decouples** the frontend/back-end from the persistence layer.

3. **Consumer → Postgres**  
   - The consumer service subscribes to RabbitMQ.  
   - It applies commands and persists the authoritative data into Postgres.  

4. **Projection Cache → Postgres + RabbitMQ**  
   - Listens to events, rebuilds the read model (a query-optimized cache).  
   - Exposes a gRPC interface for the BFF to subscribe to.  

5. **BFF → Frontend (SSE)**  
   - The BFF streams real-time updates from the projection cache to the frontend.  
   - All browsers see a consistent view of todos.  

---

## 🐳 Running with Docker Compose

The application is fully containerized. Services include:

- **Postgres**: Database to persist todos.  
- **RabbitMQ**: Message bus for decoupled event-driven communication.  
- **Projection Cache**: Maintains read models for efficient querying.  
- **Backend (BFF)**: Exposes APIs and SSE streams for the frontend.  
- **Consumer**: Subscribes to events and writes to Postgres.  
- **Frontend**: Simple React-based UI.  

### Start the stack:

```bash
docker compose up --build
```

Then visit the UI at: **http://localhost:3000**

RabbitMQ management console: **http://localhost:15672** (guest/guest).

---

🎯 Why This Matters

This project is not about todos themselves.
It’s about demonstrating system design thinking:

Designing with separation of concerns.

Using events, caching, and projections to scale reads.

Leveraging a message bus to decouple services.

Showing frontend doesn’t directly couple with the DB, but works through well-defined APIs.

It helps illustrate how a developer can move from “I built a CRUD app” → to “I can design enterprise-ready systems”.

🧭 Next Steps

Implement proper subscriber cleanup in the backend to avoid memory leaks.

Add observability (logging, metrics, tracing).

Add Unit Test/ Integration Test etc

Experiment with scaling individual services (e.g., multiple consumers).

## 🎯 Intent of This Project

The intent is **not** to build yet another todo app.  
The intent **is** to demonstrate:
- **System design thinking.**
- How different layers (UI, BFF, cache, DB, message bus) each have their **role**.  
- Why **decoupling** is important in enterprise-grade systems.  
- How you can extend even a small "toy project" into a **teachable example of architecture**.  

---

## 📌 Conclusion

By studying and running this app, developers can learn how to think about **frontend, backend, cache, DB, and messaging systems** not as isolated parts, but as a **cohesive architecture** that scales and adapts.

