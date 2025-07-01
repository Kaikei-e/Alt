# Alt

_The local-first, composable RSS knowledge pipeline._

Alt is a self-hosted RSS reader and “content refinery” that fetches feeds, scrubs & tags articles, indexes them for lightning-fast search and serves a clean reading UI – everything running in neatly isolated containers so you can swap parts out or scale them independently.
The project is built **mobile-first**, 100 % open-source (Apache-2.0), and engineered around a five-layer flavour of Clean Architecture with test-driven development at its core.

---

## Feature Highlights

| Category | What you get | Where it lives |
|----------|--------------|----------------|
| **Fast crawl** | Go workers pull and de-duplicate feeds in parallel | `alt-backend/` |
| **Readability cleanup** | Pre-processing & language detect articles for AI summarization | `pre-processor/` |
| **Auto-scoring summaries** | Using LLM to score summaries and remove bad ones | `pre-processor/` |
| **Auto-tagging** | ML tagging via Python | `tag-generator/` |
| **Full-text & faceted search** | Meilisearch via a Go proxy | `search-indexer/` |
| **Auto summarization** | LLM summariser | `news-creator/` |
| **One-command up** | `docker compose up` | `compose.yaml` |

---

## Tech Stack

- **Go** for backend services and data processing
- **TypeScript / React / Next.js** for the mobile-first frontend
- **Python** for machine learning tasks (tag generation)
- **Rust** for log forwarding and aggregation (stores logs in ClickHouse)
- **ClickHouse** for high-performance analytical data storage
- **PostgreSQL** as the primary data store
- **Meilisearch** for full-text search
- **Ollama** with the phi4-mini model for LLM summarization
- **Docker Compose** orchestrates all services

## Service Overview

| Service | Tech | Purpose |
|---------|------|---------|
| **nginx** | Nginx | Reverse proxy for frontend and backend |
| **alt-frontend** | Next.js / React | Web UI with mobile-first design |
| **alt-backend** | Go + Echo | Fetches RSS feeds, exposes REST API |
| **pre-processor** | Go | Cleans articles, detects language, scores LLM summaries |
| **tag-generator** | Python + KeyBERT | Generates article tags using ML |
| **search-indexer** | Go + Meilisearch | Indexes articles for fast search |
| **news-creator** | Ollama (LLM) | Summarises and scores content |
| **db** | PostgreSQL | Stores all persistent data |
| **meilisearch** | Meilisearch | Search engine service |
| **rask-log-forwarder** | Rust | Sidecar that streams logs to aggregator |
| **rask-log-aggregator** | Rust + Axum | Central log processing service (stores logs in ClickHouse) |
| **migrate** | Go | Runs database schema migrations |

Each service runs in its own container so components can be scaled or swapped independently. The containers communicate over an internal Docker network and can be started with a single `docker compose up` command.

## Project Characteristics

- Embraces a microservice approach where small, focused containers cooperate via HTTP and message passing.
- Clean Architecture principles guide the main Go services, keeping business rules isolated from infrastructure.
- Test-driven development and automated health checks help maintain reliability.
- The system is designed to be composable: you can replace any service (for example, the search engine or tag generator) without affecting the rest of the pipeline.

