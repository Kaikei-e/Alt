# Alt Backend

_Last reviewed: December 18, 2025_

**Location:** `alt-backend/app`

## Role
- **Core API**: Primary Go 1.24+ HTTP API that exposes endpoints for feeds, articles, recaps, SSE, and system dashboards.
- **Orchestrator**: Manages background feed collection, service-to-service communication (e.g., with `recap-worker`), and search operations via `search-indexer`.
- **Gatekeeper**: Handles authentication, customizable DOS protection, SSE fan-out, and content fetching security (SSRF protection).
- **Architecture**: Follows Clean Architecture principles (`REST` → `Usecase` → `Port` → `Gateway` → `Driver`) to maintain specific boundaries and testability.

## Architecture Snapshot

| Layer | Notes |
| --- | --- |
| **REST Handlers** (`rest/*.go`) | **Middlewares**: Request ID → Recovery → Secure Headers (CSP, HSTS) → CORS → DOS Guard → Timeout (skips `/sse/`) → Validation → Logging → Gzip.<br>**Route Groups**: `/v1` (Public/Protected), `/v1/admin` (Internal/Admin), `/v1/dashboard` (System State). |
| **Usecases** (`usecase/*`) | Business logic layer. <br>- **Feed Logic**: `FetchSingleFeed`, `FetchFeedsList`, `RegisterFeeds`.<br>- **Recap Logic**: `RecapUsecase` (7-day summaries), `RecapArticlesUsecase`.<br>- **Jobs**: `job.HourlyJobRunner` (RSS Collection), `job.DailyScrapingPolicyJobRunner` (Robots.txt refresh). |
| **Ports & Gateways** (`port/*`, `gateway/*`) | Interface adapters. Gateways convert domain entities to/from specific driver implementations (Postgres DTOs, external API formats). |
| **Drivers** (`driver/*`) | - `alt_db`: Connects to PostgreSQL 17 via `pgx/v5` pool.<br>- `search_indexer`: Connects to `search-indexer` service via HTTP.<br>- `recap_job_driver`: Connects to `recap-worker`.<br>- `utils/secure_http_client.go`: Wraps outbound requests (SSRF protection). |
| **Dependency Injection** (`di/container.go`) | centralized dependency wiring (`ApplicationComponents`). Connects repositories, services, and middlewares. |

```mermaid
flowchart LR
    Browser -->|"X-Alt-* headers"| AltBackend
    AltBackend -->|Persistent SSE| SSE[/v1/sse/feeds/stats]
    AltBackend -->|Queries & writes| Postgres[(PostgreSQL)]
    AltBackend -->|HTTP Search API| SearchIndexer[search-indexer]
    SearchIndexer -->|Index & Search| Meili[(Meilisearch)]
    AltBackend -->|Recap worker sync| RecapWorker[recap-worker]
    AltBackend -->|Outbound feeds/API| External[External Web]
    RecapWorker -->|LLM summaries| NewsCreator[news-creator]
    RecapWorker --> Postgres
```

## Routes & API Surface

**Location**: `rest/routes.go` delegates to specific handlers.

### Public & User-Facing APIs (`/v1`)
- **Feeds** (`/v1/feeds`):
  - `GET /fetch/single`, `/fetch/list`, `/fetch/limit/:limit`, `/fetch/page/:page`: Retrieve news feeds.
  - `GET /count/unreads`, `/fetch/cursor`: Unread counts and cursor-based pagination.
  - `POST /read`, `/register/favorite`: Mark read, manage favorites.
  - `POST /search`, `/tags`: Search feeds and tags.
  - `GET /stats`: Feed processing statistics.
- **Recap** (`/v1/recap`):
  - `GET /7days`: Weekly recap summary (public).
  - `GET /articles`: Internal use for fetching articles by date range (Service Protected).
- **Articles** (`/v1/articles`):
  - `GET /fetch/content`: Fetch regular article content (cache/DB check first).
  - `GET /fetch/cursor`: List articles with pagination.
  - `POST /archive`: Archive articles.
  - `GET /search`: Search indexed articles (via `search-indexer`).
- **Images** (`/v1/images`):
  - `POST /fetch`: Proxy for fetching images to avoid CORS/Mixed Content issues.
- **Security**:
  - `GET /health`, `/csrf-token`.
  - `POST /security/csp-report`.
- **RSS Feed Links** (`/v1/rss-feed-link`):
  - `POST /register`, `GET /list`, `DELETE /:id`.

### Dashboard & System APIs (`/v1/dashboard`)
Provide visibility into system internal state.
- `GET /metrics`: Prometheus-style or internal metrics.
- `GET /overview`: High-level system health.
- `GET /logs`: Access recent application logs.
- `GET /jobs`: Status of background jobs (RSS scraping).
- `GET /recap_jobs`: Status of heavy recap generation jobs.

### Admin/Operations (`/v1/admin`)
- **Scraping Domains**:
  - `GET /scraping-domains`: List configured domains.
  - `POST /scraping-domains/:id/refresh-robots`: Force refresh of robots.txt policy.

### SSE (Server-Sent Events)
- `/v1/sse/feeds/stats`: Long-lived connection sending real-time updates on feed processing counts (`feedAmount`, `unsummarizedFeedAmount`, `articleAmount`).

## Background Jobs
Defined in `job/` package and started in `main.go`.

1.  **Hourly RSS Collection** (`job.HourlyJobRunner`)
    - Interval: 1 hour.
    - Logic: Fetches registered RSS feed URLs from DB -> `CollectMultipleFeeds` (concurrent fetch with rate limiting) -> Register new items.
2.  **Daily Policy Refresh** (`job.DailyScrapingPolicyJobRunner`)
    - Interval: 24 hours.
    - Logic: Refreshes `robots.txt` rules and scraping policies for all tracked domains to ensure compliance.

## Integrations & Data Flow

1.  **PostgreSQL**:
    - Primary persistence for Articles, Feeds, Users, and operational state.
    - Used by `alt-backend`, `recap-worker`, `tag-generator`.
2.  **Search Indexer**:
    - `alt-backend` does **not** connect to Meilisearch directly.
    - It delegates search and indexing requests to the `search-indexer` service (Port 9300) via `driver/search_indexer/api.go`.
3.  **Recap System**:
    - `alt-backend` exposes `POST /v1/recap/articles` (used by `recap-worker` to ingest data).
    - `alt-backend` fetches recap summaries via `recap-db` or triggers jobs via `recap-worker`.
    - **Service Tokens**: Service-to-service communication is secured via Shared Secret tokens (`X-Service-Token`).

## Configuration
Managed via `config` package and environment variables.
- **Server**: `SERVER_PORT` (default 9000), `SERVER_ReadTimeout`, `SERVER_WriteTimeout`.
- **Database**: `DB_HOST`, `DB_PORT`, `DB_USER` (Postgres).
- **Recap**: `RECAP_RATE_LIMIT_RPS`, `RECAP_WORKER_URL`.
- **Auth**: `AUTH_SHARED_SECRET` for service communication.
- **Cache**: `CACHE_FEED_EXPIRY`, `CACHE_SEARCH_EXPIRY`.

## Operational Notes
1.  **Health Check**: `curl http://localhost:9000/v1/health`
2.  **Development**: Run `make build` within `alt-backend`. Dockerfile uses standard Go build.
3.  **Logs**: Structured text/JSON logs via `slog` (Go 1.21+).
