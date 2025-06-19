# CLAUDE.md

## About this Project

- This project is a mobile-first RSS reader built with a micro-service stack.
- The codebase follows a five-layer variant of Clean Architecture for main application.
  — **REST (handler) → Usecase → Port → Gateway (ACL) → Driver** (Database, other APIs)
  — where **Gateway** acts as the anti-corruption layer that shields the domain from external semantics.
- helper applications are built with different architecture for different purposes.
- But Whole implementation process is based on TDD.

## Tech Stack for the whole project

### Language

- Go
- TypeScript
- Python

### Framework

- Echo
- Next.js
- React
- TensorFlow

### Database

- PostgreSQL

### Search Engine

- Meilisearch

### LLMs

- Phi4-mini


## Tech Stack for each application

### Main application

- Go/Echo (main backend application. aka. `alt-backend`)
- TypeScript/React/Next.js (main frontend application. aka. `alt-frontend`. But it's not built with Clean Architecture intentionally.)
- PostgreSQL (database. aka. `db`)
- Meilisearch (search engine. aka. `meilisearch`)

### Helper applications

- Go (helper applications. aka. `pre-processor`, `search-indexer`)
- Python (helper applications. aka. `tag-generator`)
- LLMs (helper applications. aka. `news-creator`)


# Architecture at a Glance

## 1 Architecture at a Glance

### 1.1 Layer Responsibilities

| Layer | Role | Depends on |
|-------|------|------------|
| **REST (handler)** | Map HTTP/gRPC I/O to DTOs | ↘ Usecase |
| **Usecase** | Orchestrate domain rules & transactions | ↘ Port |
| **Port** | Declare interfaces the app needs | ↘ Gateway |
| **Gateway (ACL)** | Translate external vocab ⇄ domain objects | ↘ Driver |
| **Driver** | Tech specifics: DB, queues, 3rd-party APIs | – |

The extra Gateway layer formalises the Anti-Corruption-Layer pattern, adding a buffer that classic Ports-and-Adapters lacks. :contentReference[oaicite:1]{index=1}

### 1.2 Main Clean Architecture Directory Layout

```

/
├─ rest            # REST entry-points/ Handlers / routers
├─ usecase         # Application services
├─ port            # Input/Output ports
├─ gateway         # Anti-corruption layer
└─ driver          # Database adapters, API clients, etc.
└─ domain          # Entities & value objects
└─ utils           # Utility functions like logging, error handling, etc.
```

---

## 2 Coding Guidelines

- Use ES6 module syntax for TypeScript.
- Use Python 3.13 for helper applications.
- Use PostgreSQL for database.
- Use Meilisearch for search engine.
- When you write Go, use "log/slog" for logging.

### 2.1 Mocking

- Use `gomock` by default which is maintaining by Uber for Go.


## Implementation Steps

- Use TDD for the whole project.
  - First, write a failing test.
  - Then, write the code to pass the test.
  - Then, refactor the code to make it clean and readable.
- CRITICAL: These are very important to implement.
  - Respect the existing implementation logic and you have to understard existing code logic first.
  - You must write test only usecase and gateway layers.
    - Do not write test for handler layer.
    - Do not write test for driver layer.
    - Do not write sql.