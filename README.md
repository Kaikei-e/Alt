[![Backend Go Tests](https://github.com/Kaikei-e/Alt/actions/workflows/backend-go.yaml/badge.svg)](https://github.com/Kaikei-e/Alt/actions/workflows/backend-go.yaml)
[![Alt Frontend E2E Test](https://github.com/Kaikei-e/Alt/actions/workflows/alt-frontend-e2e.yaml/badge.svg)](https://github.com/Kaikei-e/Alt/actions/workflows/alt-frontend-e2e.yaml)
[![Alt Frontend Unit Test](https://github.com/Kaikei-e/Alt/actions/workflows/alt-frontend-unit-test.yaml/badge.svg)](https://github.com/Kaikei-e/Alt/actions/workflows/alt-frontend-unit-test.yaml)
[![Pre-processor Quality Gates](https://github.com/Kaikei-e/Alt/actions/workflows/pre-processor-quality.yml/badge.svg)](https://github.com/Kaikei-e/Alt/actions/workflows/pre-processor-quality.yml)
[![Search Indexer Tests](https://github.com/Kaikei-e/Alt/actions/workflows/search-indexer.yaml/badge.svg)](https://github.com/Kaikei-e/Alt/actions/workflows/search-indexer.yaml)
[![Tag Generator Tests](https://github.com/Kaikei-e/Alt/actions/workflows/tag-generator.yaml/badge.svg)](https://github.com/Kaikei-e/Alt/actions/workflows/tag-generator.yaml)


# Alt - AI-Powered RSS Knowledge Pipeline

> October 2025 update: Alt now runs locally on Docker Compose (`compose.yaml`). Kubernetes manifests remain in the repo for future redeployment but are no longer the primary development path.

## Executive Summary

**Alt** transforms a traditional RSS reader into an intelligent knowledge discovery platform. The system blends microservices, AI enrichment, and a polished frontend to deliver curated content across devices.

### Key Capabilities

- **📱 Mobile-First Design**: Responsive TypeScript/React frontend with glassmorphism UI
- **🤖 AI Content Enhancement**: Automatic tagging and LLM-powered summarisation
- **⚡ High-Performance Processing**: Go-based backend with Clean Architecture and TDD
- **🔍 Intelligent Search**: Meilisearch-backed full-text and semantic discovery
- **📊 Observability-Ready**: Rust logging pipeline into ClickHouse with optional sidecars
- **🧱 Docker Compose-First**: Single-command local stack with optional `ollama` and `logging` profiles; Kubernetes manifests are maintained only for future production rollout

### Target Audience

- **Knowledge Workers**: Professionals who need to stay informed across multiple domains
- **Researchers**: Academic and industry researchers tracking developments in their fields
- **Content Creators**: Writers, bloggers, and journalists seeking inspiration and trends
- **Platform Engineers**: Developers exploring multi-service stacks with Compose orchestration

---

## Features & Capabilities

### Core Features

#### 📰 Intelligent Content Aggregation
- **RSS Feed Management**: Subscribe to unlimited RSS feeds with automatic discovery
- **Duplicate Detection**: Advanced algorithms prevent content duplication across feeds
- **Language Detection**: Multi-language support with automatic language identification

#### 🧠 AI-Powered Enhancement
- **Automatic Tagging**: Machine learning models generate contextually relevant tags
- **Content Summarization**: LLM-powered summaries using Ollama (Gemma3:4b model)
- **Topic Classification**: Articles automatically categorized by subject matter

#### 🔍 Advanced Search & Discovery
- **Full-Text Search**: Lightning-fast search across all content using Meilisearch
- **Semantic Search**: AI-powered semantic matching beyond keyword search
- **Filter & Sort**: Advanced filtering by date, source, tags, reading status

### Technical Features

#### 🏗️ Modern Architecture
- **Microservices Design**: 8 specialized services with clear boundaries
- **Clean Architecture**: 5-layer pattern ensuring maintainability and testability
- **Event-Driven Communication**: Asynchronous processing with reliable delivery
- **API-First Design**: RESTful APIs with comprehensive OpenAPI documentation

#### 🚀 Performance & Scalability
- **High Throughput**: 100K+ logs/second processing capability
- **Low Latency**: Sub-5ms response times for critical operations
- **Profile-Based Scaling**: Optional Docker Compose profiles isolate heavy services from the baseline stack
- **Efficient Resource Usage**: Optimized memory and CPU consumption

#### 🔒 Security & Reliability
- **Rate Limiting**: Intelligent rate limiting prevents abuse and ensures stability
- **Input Validation**: Comprehensive validation prevents injection attacks
- **Graceful Degradation**: System remains functional during partial failures

---

## Architecture Overview

### System Architecture

```mermaid
graph TB
    subgraph "User Interface"
        UI[Mobile-First Frontend<br/>TypeScript/React/Next.js]
    end

    subgraph "Gateway Layer"
        NGINX[NGINX Reverse Proxy<br/>Load Balancer]
    end

    subgraph "Core Services"
        API[alt-backend<br/>Go/Echo Clean Architecture]
        PREP[pre-processor<br/>Feed Processing]
        TAG[tag-generator<br/>Python ML Service]
        NEWS[news-creator<br/>LLM Summarization]
        SEARCH[search-indexer<br/>Index Management]
    end

    subgraph "Data Layer"
        PG[(PostgreSQL 16<br/>Primary Database)]
        MEILI[(Meilisearch<br/>Search Engine)]
        CLICK[(ClickHouse<br/>Analytics DB)]
    end

    subgraph "Logging Infrastructure"
        FORWARDER[rask-log-forwarder<br/>Rust Sidecar Collectors]
        AGGREGATOR[rask-log-aggregator<br/>Central Log Processor]
    end

    UI --> NGINX
    NGINX --> API
    API --> PG
    API --> MEILI

    PREP --> PG
    PREP --> NEWS
    TAG --> PG
    SEARCH --> PG
    SEARCH --> MEILI

    FORWARDER --> AGGREGATOR
    AGGREGATOR --> CLICK

    classDef frontend fill:#e1f5fe
    classDef backend fill:#f3e5f5
    classDef data fill:#e8f5e8
    classDef logging fill:#fff3e0

    class UI frontend
    class API,PREP,TAG,NEWS,SEARCH backend
    class PG,MEILI,CLICK data
    class FORWARDER,AGGREGATOR logging
```

### Data Processing Pipeline

```mermaid
sequenceDiagram
    participant User
    participant Frontend as alt-frontend
    participant Backend as alt-backend
    participant Preprocessor as pre-processor
    participant TagGen as tag-generator
    participant NewsCreator as news-creator
    participant SearchIndexer as search-indexer
    participant Database as PostgreSQL
    participant Search as Meilisearch

    User->>Frontend: Subscribe to RSS feed
    Frontend->>Backend: POST /api/feeds
    Backend->>Database: Store feed URL
    Backend->>Preprocessor: Trigger processing (requires `--profile ollama`)

    Preprocessor->>Preprocessor: Fetch RSS content
    Preprocessor->>Preprocessor: Clean HTML, detect language
    Preprocessor->>Preprocessor: Quality scoring
    Preprocessor->>Database: Store articles

    Preprocessor->>TagGen: Request tag generation
    TagGen->>TagGen: ML-based tag extraction
    TagGen->>Database: Store tags

    Preprocessor->>NewsCreator: Request summarization
    NewsCreator->>NewsCreator: LLM processing (Gemma3:4b)
    NewsCreator->>Database: Store summaries

    SearchIndexer->>Database: Query new content
    SearchIndexer->>Search: Update search indexes

    Frontend->>Backend: GET /api/feeds/enhanced
    Backend->>Database: Fetch enhanced articles
    Backend->>Frontend: Return enriched content
    Frontend->>User: Display enhanced articles
```

---

## Microservice Architecture

### Service Responsibilities

#### 🎯 alt-backend (Go/Echo)
**Primary API Gateway & Business Logic**

- **Technology**: Go 1.23+, Echo framework, Clean Architecture (5-layer)
- **Responsibilities**:
  - RESTful API endpoints for frontend communication
  - User feed management and subscription handling
  - Content aggregation and presentation logic
  - Authentication and authorization (future)
  - Rate limiting and request validation
- **Architecture Pattern**: REST → Usecase → Port → Gateway → Driver
- **Key Features**:
  - CSRF protection on state-changing endpoints
  - Structured logging with `slog`
  - Comprehensive test coverage (>80%)
  - Clean separation of concerns

```mermaid
graph LR
    subgraph "alt-backend Clean Architecture"
        REST[REST Layer<br/>HTTP Handlers]
        USE[Usecase Layer<br/>Business Logic]
        PORT[Port Layer<br/>Interfaces]
        GATE[Gateway Layer<br/>Anti-Corruption]
        DRIVER[Driver Layer<br/>External Systems]

        REST --> USE
        USE --> PORT
        PORT --> GATE
        GATE --> DRIVER
    end
```

#### 🔄 pre-processor (Go)
**RSS Feed Processing & Content Extraction**

- **Technology**: Go 1.23+, Custom HTML parser, Quality scoring algorithms
- **Responsibilities**:
  - RSS feed fetching with configurable intervals
  - HTML content cleaning and sanitization
  - Language detection and content normalization
  - Quality scoring using readability metrics
  - Content deduplication across feeds
- **Performance**: Handles 1000+ feeds with batched processing
- **Error Handling**: Comprehensive retry logic with exponential backoff

#### 🏷️ tag-generator (Python)
**ML-Powered Content Classification**

- **Technology**: Python 3.13, scikit-learn, UV package manager
- **Responsibilities**:
  - Automatic tag generation using ML models
  - Content classification and topic modeling
  - Multi-language text processing
  - Feature extraction from article content
  - Tag relevance scoring and filtering
- **ML Pipeline**: TF-IDF → Feature Extraction → Multi-class Classification
- **Development**: TDD with pytest, comprehensive model validation

#### 📝 news-creator (Ollama)
**LLM-Based Content Summarization**

- **Technology**: Ollama runtime, Gemma3:4b model, GPU acceleration
- **Responsibilities**:
  - Article summarization using large language models
  - Content quality assessment and filtering
  - Context-aware summary generation
  - Multi-format output (brief, detailed, bullet points)
- **Performance**: GPU-optimized with NVIDIA runtime support
- **Scaling**: Model caching and efficient batch processing

#### 🔍 search-indexer (Go)
**Search Index Management**

- **Technology**: Go 1.23+, Meilisearch client, Clean Architecture
- **Responsibilities**:
  - Real-time search index updates
  - Content synchronization with database
  - Search relevance optimization
  - Index health monitoring and maintenance
- **Features**: Incremental indexing, faceted search, typo tolerance

#### 🎨 alt-frontend (TypeScript/React)
**Mobile-First User Interface**

- **Technology**: TypeScript, React, Next.js (Pages Router), Chakra UI
- **Responsibilities**:
  - Responsive mobile-first user interface
  - Real-time content updates via SSE
  - Glassmorphism design system
- **Performance**: Virtual scrolling, lazy loading, optimized bundling
- **Testing**: Playwright for E2E, Vitest for unit tests

#### 📊 rask-log-forwarder (Rust)
**High-Performance Log Collection**

- **Technology**: Rust 1.87+, SIMD JSON parsing, Lock-free data structures
- **Responsibilities**:
  - Zero-copy log collection from Docker containers
  - SIMD-accelerated JSON parsing (>4GB/s throughput)
  - Service-aware log enrichment
  - Reliable delivery with disk fallback
- **Architecture**: Sidecar pattern with one forwarder per service
- **Performance**: >100K logs/second, <16MB memory per instance

#### 🏪 rask-log-aggregator (Rust/Axum)
**Centralized Log Processing**

- **Technology**: Rust 1.87+, Axum web framework, ClickHouse client
- **Responsibilities**:
  - Central log aggregation and processing
  - Real-time analytics and metrics generation
  - Log storage in ClickHouse for analytics
  - System health monitoring and alerting
- **Capabilities**: Stream processing, data compression, query optimization

---

## Data Flow & Processing

### RSS Content Enhancement Pipeline

```mermaid
flowchart TD
    START([RSS Feed URL]) --> FETCH[Fetch RSS Content]
    FETCH --> PARSE[Parse RSS XML]
    PARSE --> EXTRACT[Extract Articles]

    EXTRACT --> CLEAN[HTML Cleaning]
    CLEAN --> LANG[Language Detection]
    LANG --> QUALITY[Quality Scoring]

    QUALITY --> STORE_RAW[(Store Raw Articles)]

    STORE_RAW --> TAG_ML[ML Tag Generation]
    TAG_ML --> STORE_TAGS[(Store Tags)]

    STORE_RAW --> LLM_SUM[LLM Summarization]
    LLM_SUM --> STORE_SUM[(Store Summaries)]

    STORE_TAGS --> INDEX[Search Indexing]
    STORE_SUM --> INDEX
    INDEX --> SEARCH_DB[(Meilisearch)]

    SEARCH_DB --> API_SERVE[API Endpoint]
    API_SERVE --> FRONTEND[Frontend Display]
    FRONTEND --> USER([End User])

    classDef process fill:#e3f2fd
    classDef storage fill:#e8f5e8
    classDef ai fill:#fce4ec
    classDef endpoint fill:#fff3e0

    class FETCH,PARSE,EXTRACT,CLEAN,LANG,QUALITY process
    class STORE_RAW,STORE_TAGS,STORE_SUM,SEARCH_DB storage
    class TAG_ML,LLM_SUM ai
    class API_SERVE,FRONTEND,USER endpoint
```

### AI Enhancement Workflow

```mermaid
graph TB
    subgraph "Content Input"
        ARTICLE[Raw Article Content]
        META[Metadata & Context]
    end

    subgraph "Tag Generation Pipeline"
        PREP_TAG[Text Preprocessing]
        FEAT_EXT[Feature Extraction]
        ML_MODEL[ML Classification Model]
        TAG_FILTER[Tag Filtering & Ranking]
    end

    subgraph "Summarization Pipeline"
        PREP_SUM[Content Preparation]
        LLM[Gemma3:4b Model]
        SUM_POST[Summary Post-processing]
    end

    subgraph "Quality Assessment"
        READ_SCORE[Readability Scoring]
        REL_SCORE[Relevance Scoring]
        FINAL_SCORE[Final Quality Score]
    end

    subgraph "Output"
        ENHANCED[Enhanced Article]
        TAGS_OUT[Generated Tags]
        SUMMARY[Article Summary]
        SCORE[Quality Metrics]
    end

    ARTICLE --> PREP_TAG
    ARTICLE --> PREP_SUM
    ARTICLE --> READ_SCORE

    META --> FEAT_EXT
    META --> REL_SCORE

    PREP_TAG --> FEAT_EXT
    FEAT_EXT --> ML_MODEL
    ML_MODEL --> TAG_FILTER
    TAG_FILTER --> TAGS_OUT

    PREP_SUM --> LLM
    LLM --> SUM_POST
    SUM_POST --> SUMMARY

    READ_SCORE --> FINAL_SCORE
    REL_SCORE --> FINAL_SCORE
    FINAL_SCORE --> SCORE

    TAGS_OUT --> ENHANCED
    SUMMARY --> ENHANCED
    SCORE --> ENHANCED
```

---

## Deployment Architecture

### Docker Compose Runtime

```mermaid
flowchart TB
    subgraph Default
        NGX[nginx\n:80]
        FE[alt-frontend\n:3000]
        BE[alt-backend\n:9000]
        MIGRATE[migrate Atlas init job]
        DB[(postgres\n:5432)]
        SEARCHIDX[search-indexer\n:9300]
        TAGGER[tag-generator\n:9400]
        MEILI[meilisearch\n:7700]
        LOGAGG[rask-log-aggregator\n:9600]
        CLICK[(clickhouse\n:8123/:9009)]
        KRATOSDB[(kratos-db\n:5433)]
        KRATOS[kratos\n:4433/:4434]
        AUTH[auth-hub\n:8888]
    end

    subgraph Profile_Ollama [Optional profile: ollama]
        OLLAMA_INIT[news-creator-volume-init]
        NEWS[news-creator\n:11434]
        PREPROC[pre-processor\n:9200]
    end

    subgraph Profile_Logging [Optional profile: logging]
        LOGFWD[log forwarder sidecars\n(nginx, backend, db, etc.)]
    end

    NGX --> FE
    NGX --> BE
    BE --> DB
    BE --> MEILI
    SEARCHIDX --> DB
    SEARCHIDX --> MEILI
    TAGGER --> DB
    LOGAGG --> CLICK
    LOGFWD -.-> LOGAGG
    PREPROC --> DB
    PREPROC --> NEWS
    NEWS -.-> OLLAMA_INIT
    AUTH --> KRATOS
    KRATOS --> KRATOSDB
```

### Docker Compose Profiles

- **Baseline (default)**: Frontend, backend, PostgreSQL, Meilisearch, tagging, indexing, identity (Kratos), analytics (ClickHouse + log aggregator).
- **`--profile ollama`**: Adds GPU-enabled `news-creator`, its volume initialiser, and the `pre-processor` ingestion service.
- **`--profile logging`**: Starts Rust log forwarders that tail Docker logs and ship them to `rask-log-aggregator`.

Profiles can be combined, e.g. `docker compose --profile ollama --profile logging up --build`.

## Service Catalog (compose.yaml)

| Service | Language / Image | Ports (host) | Compose profile | Purpose |
|---------|------------------|--------------|-----------------|---------|
| `nginx` | nginx:latest | 80 | default | Reverse proxy for frontend (`/`) and backend (`/api`) with health gating |
| `alt-frontend` | Next.js 15 (./alt-frontend) | 3000 | default | Serves the web UI; exposes `/api/health` |
| `alt-backend` | Go 1.24 (./alt-backend) | 9000 | default | Clean Architecture API with `/v1/health` |
| `migrate` | Atlas CLI (./migrations-atlas) | – (one-shot) | default | Applies schema migrations before backend starts |
| `db` | postgres:16-alpine | 5432 | default | Primary relational database seeded from `db/init` |
| `meilisearch` | getmeili/meilisearch:v1.15.2 | 7700 | default | Full-text search engine |
| `search-indexer` | Go (./search-indexer) | 9300 | default | Syncs Postgres records into Meilisearch |
| `tag-generator` | Python 3.13 (./tag-generator) | 9400 | default | ML-driven keyword extraction |
| `rask-log-aggregator` | Rust (./rask-log-aggregator) | 9600 | default | Collects structured logs into ClickHouse |
| `clickhouse` | clickhouse/clickhouse-server:25.6 | 8123, 9009 | default | Analytics warehouse backing observability |
| `kratos-db` | postgres:16-alpine | 5433 | default | Identity database for Kratos |
| `kratos-migrate` | oryd/kratos:v1.2.0 | – (one-shot) | default | Runs Kratos SQL migrations |
| `kratos` | oryd/kratos:v1.2.0 | 4433, 4434 | default | Identity & self-service flows surfaced in UI |
| `auth-hub` | Go (./auth-hub) | 8888 | default | Auth facade aggregating Kratos flows |
| `news-creator` | Ollama runtime (./news-creator) | 11434 | `ollama` | LLM summarisation service (GPU-enabled) |
| `pre-processor` | Go (./pre-processor) | 9200 | `ollama` | RSS ingestion orchestrator feeding news-creator |
| `news-creator-volume-init` | Ollama image | – (one-shot) | `ollama` | Fixes ownership for shared model volume |
| `*-logs` forwarders | Rust (./rask-log-forwarder/app) | – | `logging` | Tail Docker logs per service and push to aggregator |

Persistent volumes: `db_data`, `kratos_db_data`, `meili_data`, `clickhouse_data`, `news_creator_models`, `rask_log_aggregator_data`.

---

## Local Development Workflow

1. **Install prerequisites**: Docker Desktop (or compatible engine) and Node.js 24 with pnpm 10 (`corepack enable pnpm`).
2. **Create environment file**: `cp .env.template .env` (or let `make up` create it automatically).
3. **Start the stack**: `make up` → builds images and launches the baseline services.
4. **Enable optional profiles** as needed:
   - AI services: `docker compose --profile ollama up -d`
   - Log forwarders: `docker compose --profile logging up -d`
5. **Run the frontend locally**: `pnpm -C alt-frontend install` then `pnpm -C alt-frontend dev`.
6. **Shut down**: `make down` (keep data) or `make down-volumes` (reset everything).

### Helpful Commands

| Command | Description |
|---------|-------------|
| `make build` | Force rebuild of all images |
| `make up-clean-frontend` | Rebuild the frontend image without cache before starting |
| `make up-with-news-creator` | Rebuild AI image then start the stack |
| `make dev-ssl-setup` / `make dev-ssl-test` | Generate and verify PostgreSQL SSL certificates |
| `docker compose logs -f <service>` | Stream logs from a specific container |
| `docker compose ps --status=running` | Check which services (and profiles) are online |
| `docker compose exec db psql ...` | Connect to the Postgres shell |

### Environment Notes

- `scripts/check-env.js` validates required variables during `next build`. Keep `.env.template`, `.env`, and compose `env_file` entries in sync.
- Frontend defaults: `NEXT_PUBLIC_API_BASE_URL=/api`, `API_URL=http://alt-backend:9000`. When running `pnpm dev` outside Docker, set `NEXT_PUBLIC_API_BASE_URL=http://localhost/api`.
- Identity routes rely on Kratos URLs (`NEXT_PUBLIC_KRATOS_PUBLIC_URL`, `KRATOS_PUBLIC_URL`, `KRATOS_INTERNAL_URL`) injected via compose build args.

### Troubleshooting Quick Checks

- `curl http://localhost:3000/api/health` → frontend container health
- `curl http://localhost:9000/v1/health` → backend availability
- `curl http://localhost:7700/health` → Meilisearch readiness
- `docker compose logs -f <service>` → targeted diagnostics (all services share the `alt-network` bridge)
- `docker compose exec db pg_isready -U ${POSTGRES_USER}` → Postgres connectivity

---

## Testing & Quality Gates

| Area | Command | Notes |
|------|---------|-------|
| Frontend unit tests | `pnpm -C alt-frontend test` | Vitest + React Testing Library |
| Frontend watch mode | `pnpm -C alt-frontend test:watch` | Red/Green/Refactor loop |
| Frontend coverage | `pnpm -C alt-frontend test:coverage` | Istanbul output |
| Frontend E2E | `pnpm -C alt-frontend test:e2e` | Requires stack running; Playwright projects cover auth + content |
| Frontend lint | `pnpm -C alt-frontend lint` | ESLint 9 with Next plugin |
| Frontend format | `pnpm -C alt-frontend fmt` | Prettier |
| Backend tests | `cd alt-backend/app && go test ./...` | Add `-race -cover` as needed |
| Backend mocks | `make generate-mocks` | GoMock against `alt-backend/app/port` interfaces |
| Search indexer | `cd search-indexer && go test ./...` | Validates index sync logic |
| Tag generator | `cd tag-generator && uv run pytest` | Python tests (requires UV/venv) |

CI pipelines mirror these commands (see badges above).

---

## Technology Stack

### Programming Languages & Versions

| Language | Version | Usage | Key Features |
|----------|---------|--------|--------------|
| **Go** | 1.24 | Backend services, processing, auth hub | Clean Architecture, generics, structured logging |
| **TypeScript** | 5.9 | Frontend development | Strict typing, modern Next.js features |
| **Python** | 3.13 | ML/AI services | UV package management, async-ready |
| **Rust** | 1.87 | Logging pipeline | SIMD JSON parsing, zero-copy abstractions |

### Frameworks & Libraries

#### Backend (Go)
- **Echo v4**: High-performance HTTP framework
- **GORM**: ORM with PostgreSQL driver
- **gomock**: Mock generation for testing
- **slog**: Structured logging (stdlib)
- **testify**: Testing assertions and suites

#### Frontend (TypeScript)
- **Next.js 15 (App Router)**: React framework with server/client component support
- **React 19**: Hooks-first UI layer with Suspense-ready primitives
- **Chakra UI**: Component library with theme tokens and responsive props
- **Playwright**: End-to-end testing
- **Vitest**: Unit testing framework

#### ML/AI (Python)
- **UV**: Modern Python package manager
- **scikit-learn**: Machine learning library
- **transformers**: Hugging Face transformers
- **FastAPI**: API framework (if needed)
- **pytest**: Testing framework

#### Logging (Rust)
- **Tokio**: Async runtime
- **Axum**: Web framework
- **SIMD-JSON**: High-performance JSON parsing
- **Bollard**: Docker API client
- **ClickHouse**: Database client

### Infrastructure & Databases

#### Databases
- **PostgreSQL 16**: Primary relational database with SSL/TLS
- **Meilisearch v1.15.2**: Full-text search engine
- **ClickHouse 25.6**: Analytics database for logs

#### Container & Orchestration
- **Docker**: Multi-stage builds for every service
- **Docker Compose**: Primary orchestration with optional profiles (`compose.yaml`)
- **Makefile**: Idempotent helpers for builds, environment setup, SSL, and mock generation
- **Skaffold / Kubernetes**: Legacy manifests retained for future redeployment (not part of the default workflow)

#### Networking & Security
- **NGINX**: Reverse proxy and load balancer
- **SSL/TLS**: End-to-end encryption
- **Let's Encrypt**: Automated certificate management
- **CORS**: Cross-origin resource sharing
- **Docker network (`alt-network`)**: Isolated bridge network shared by all services

### Development Tools & Practices

#### Code Quality
- **TDD**: Test-driven development across all services
- **Clean Architecture**: Layered architecture in Go services
- **ESLint/Prettier**: TypeScript code formatting
- **Ruff**: Python linting and formatting
- **Clippy**: Rust linting

#### CI/CD & DevOps
- **Git**: Version control with conventional commits
- **GitHub Actions**: Continuous integration
- **Docker Registry**: Container image storage
- **Docker Compose**: Declarative local runtime shared across contributors

#### Monitoring & Observability
- **Structured Logging**: JSON logs across all services
- **Metrics Collection**: Performance and business metrics
- **Health Checks**: Service health monitoring
- **Distributed Tracing**: Request tracing (future)

---

## Key Design Patterns

### Clean Architecture Implementation

Alt's backend services follow Uncle Bob's Clean Architecture principles with a 5-layer variant:

```mermaid
graph TD
    subgraph "Clean Architecture Layers"
        subgraph "External Layer"
            REST[REST Layer<br/>HTTP Handlers, Routing]
        end

        subgraph "Application Layer"
            USECASE[Usecase Layer<br/>Business Logic Orchestration]
        end

        subgraph "Interface Layer"
            PORT[Port Layer<br/>Interface Definitions]
        end

        subgraph "Infrastructure Layer"
            GATEWAY[Gateway Layer<br/>Anti-Corruption Layer]
        end

        subgraph "Framework Layer"
            DRIVER[Driver Layer<br/>External Systems, DBs, APIs]
        end
    end

    REST --> USECASE
    USECASE --> PORT
    PORT --> GATEWAY
    GATEWAY --> DRIVER

    classDef external fill:#e3f2fd
    classDef application fill:#f3e5f5
    classDef interface fill:#e8f5e8
    classDef infrastructure fill:#fff3e0
    classDef framework fill:#fce4ec

    class REST external
    class USECASE application
    class PORT interface
    class GATEWAY infrastructure
    class DRIVER framework
```

#### Layer Responsibilities

1. **REST Layer**: HTTP request/response handling, input validation, error responses
2. **Usecase Layer**: Business logic orchestration, workflow coordination
3. **Port Layer**: Interface definitions, contracts between layers
4. **Gateway Layer**: Anti-corruption layer, external service translation
5. **Driver Layer**: Technical implementations, database access, API clients

### Test-Driven Development (TDD)

All services follow strict TDD practices with the Red-Green-Refactor cycle:

```mermaid
flowchart LR
    RED[🔴 RED<br/>Write Failing Test] --> GREEN[🟢 GREEN<br/>Write Minimal Code]
    GREEN --> REFACTOR[🔄 REFACTOR<br/>Improve Design]
    REFACTOR --> RED

    classDef red fill:#ffebee
    classDef green fill:#e8f5e8
    classDef refactor fill:#e3f2fd

    class RED red
    class GREEN green
    class REFACTOR refactor
```

#### Testing Strategy

- **Unit Tests**: >80% coverage for usecase and gateway layers
- **Integration Tests**: End-to-end workflow testing
- **Performance Tests**: Load testing for critical paths
- **Contract Tests**: API contract validation

### Microservice Communication Patterns

#### Synchronous Communication
- **REST APIs**: Service-to-service communication
- **HTTP Keep-Alive**: Connection pooling for performance
- **Circuit Breakers**: Failure isolation and recovery

#### Asynchronous Communication
- **Event-Driven**: Database triggers for workflow initiation
- **Message Queues**: Future implementation for scalability
- **Batch Processing**: Efficient bulk operations

### Logging and Observability Strategy

#### Sidecar Logging Pattern
Each service has a dedicated Rust-based log forwarder running as a sidecar container:

```mermaid
graph LR
    subgraph "Service Pod"
        APP[Application Container]
        SIDECAR[Log Forwarder Sidecar]
    end

    subgraph "Shared Resources"
        LOGS[Docker Logs Volume]
        NETWORK[Shared Network Namespace]
    end

    APP --> LOGS
    SIDECAR --> LOGS
    SIDECAR --> NETWORK

    classDef app fill:#e3f2fd
    classDef sidecar fill:#fff3e0
    classDef shared fill:#e8f5e8

    class APP app
    class SIDECAR sidecar
    class LOGS,NETWORK shared
```

#### Benefits of Sidecar Pattern
- **Isolation**: Log forwarder failures don't affect application
- **Performance**: Zero-copy log processing with SIMD acceleration
- **Scalability**: Each service scales independently
- **Flexibility**: Service-specific log processing rules

---

## Development Practices

### Code Quality Standards

#### Go Services
- **Structured Logging**: All logs use `slog` with context
- **Error Handling**: Comprehensive error wrapping with `fmt.Errorf`
- **Code Coverage**: Minimum 80% for business logic layers
- **Static Analysis**: `go vet`, `golangci-lint` for code quality

#### TypeScript Frontend
- **Type Safety**: Strict TypeScript configuration
- **Component Testing**: Playwright for E2E, Vitest for units
- **Performance**: Bundle optimization, lazy loading
- **Accessibility**: WCAG 2.1 AA compliance

#### Python ML Services
- **Type Hints**: Comprehensive type annotations
- **Testing**: pytest with fixtures and mocking
- **Code Quality**: Ruff for linting and formatting
- **Package Management**: UV for fast dependency resolution

#### Rust Logging Services
- **Memory Safety**: Zero unsafe code blocks
- **Performance**: SIMD optimizations, lock-free data structures
- **Error Handling**: `thiserror` and `anyhow` for error management
- **Testing**: Property-based testing with quickcheck

### Security Practices

#### Application Security
- **Input Validation**: All external inputs validated at entry points
- **SQL Injection Prevention**: Parameterized queries only
- **CSRF Protection**: Token-based protection for state changes
- **Content Security Policy**: Strict CSP headers

#### Infrastructure Security
- **TLS Everywhere**: Dev certificates generated via `make dev-ssl-setup`; production TLS handled by NGINX
- **Secret Management**: Environment variables, no hardcoded secrets
- **Network Segmentation**: Docker `alt-network` bridge isolates services; optional profiles add no extra host ports
- **Minimal Attack Surface**: Alpine-based container images

#### Operational Security
- **Principle of Least Privilege**: Service-specific database users
- **Audit Logging**: Comprehensive audit trails
- **Security Updates**: Automated dependency updates
- **Penetration Testing**: Regular security assessments

---


## 📈 Roadmap

### Planned Features
- [ ] Multi-user support with authentication
- [ ] Advanced filtering and saved searches
- [ ] Export functionality (OPML, JSON)
- [ ] Webhook notifications
- [ ] GraphQL API option
- [ ] Reintroduce Kubernetes deployment manifests once Compose-first workflow stabilises

### Performance Goals
- Maintain sub-100 ms API response times on commodity hardware
- Support for 10,000+ subscribed feeds
- Real-time updates via WebSocket (future)
- Horizontal scale-out plan via optional Kubernetes profile

## 📄 License

Apache 2.0 — see `LICENSE` for full text.

## 🙏 Acknowledgments

- Built with inspiration from Clean Architecture principles by Robert C. Martin
- Powered by Go, Rust, TypeScript, Python, React, Next.js, PostgreSQL, Meilisearch, ClickHouse, Ollama, and countless OSS contributors
- Special thanks to the RSS community for keeping web feeds alive

---

For additional deep dives, check the Wiki or the `docs/` directory.
```mermaid
graph TB
    %% Style definitions
    classDef frontend fill:#4a90e2,stroke:#2e5aa8,stroke-width:3px,color:#fff
    classDef backend fill:#50c878,stroke:#3aa860,stroke-width:3px,color:#fff
    classDef helper fill:#f39c12,stroke:#d68910,stroke-width:3px,color:#fff
    classDef database fill:#e74c3c,stroke:#c0392b,stroke-width:3px,color:#fff
    classDef logging fill:#9b59b6,stroke:#7d3c98,stroke-width:3px,color:#fff
    classDef external fill:#34495e,stroke:#2c3e50,stroke-width:3px,color:#fff
    classDef layer fill:#ecf0f1,stroke:#bdc3c7,stroke-width:2px,color:#2c3e50

    %% User Interface Layer
    subgraph UI["User Interface"]
        User[👤 User]
        Mobile[📱 Mobile Device]
        Desktop[💻 Desktop]
    end

    %% Nginx Reverse Proxy
    Nginx[nginx<br/>Reverse Proxy]:::external

    %% Frontend
    subgraph FE["Frontend Layer"]
        Frontend[alt-frontend<br/>TypeScript/React/Next.js<br/>Mobile-first Design]:::frontend
    end

    %% Main Backend - 5-Layer Clean Architecture
    subgraph Backend["alt-backend - 5-Layer Clean Architecture"]
        REST[REST Handler<br/>HTTP Handling]:::layer
        Usecase[Usecase<br/>Business Logic]:::layer
        Port[Port<br/>Interface Definitions]:::layer
        Gateway[Gateway ACL<br/>Anti-Corruption Layer]:::layer
        Driver[Driver<br/>External Integrations]:::layer

        REST --> Usecase
        Usecase --> Port
        Port --> Gateway
        Gateway --> Driver
    end

    %% Data Processing Pipeline
    subgraph Pipeline["Data Processing Pipeline"]
        PreProcessor[pre-processor<br/>Go<br/>Data Preprocessing<br/>Language Detection<br/>Auto-scoring]:::helper
        TagGenerator[tag-generator<br/>Python<br/>ML-based Tag Generation]:::helper
        NewsCreator[news-creator<br/>LLM Gemma3:4b<br/>Content Generation<br/>Summarization]:::helper
        SearchIndexer[search-indexer<br/>Go<br/>Search Index Management]:::helper
    end

    %% Logging Infrastructure
    subgraph LogInfra["Logging Infrastructure"]
        LogForwarder[rask-log-forwarder<br/>Rust<br/>Log Forwarding]:::logging
        LogAggregator[rask-log-aggregator<br/>Rust<br/>Log Aggregation<br/>Analytics]:::logging
    end

    %% Data Stores
    subgraph DataStore["Data Store Layer"]
        PostgreSQL[(PostgreSQL<br/>Primary Data Store)]:::database
        Meilisearch[(Meilisearch<br/>Full-text Search Engine)]:::database
        ClickHouse[(ClickHouse<br/>Analytics Database)]:::database
    end

    %% External Services
    Ollama[Ollama<br/>LLM Runtime]:::external
    RSSFeeds[RSS Feeds<br/>External RSS Sources]:::external

    %% Connection Relationships - User Flow
    User --> Mobile
    User --> Desktop
    Mobile --> Nginx
    Desktop --> Nginx
    Nginx --> Frontend
    Frontend --> REST

    %% Backend Internal Flow
    Driver --> PostgreSQL
    Driver --> PreProcessor
    Driver --> SearchIndexer

    %% Data Processing Flow
    PreProcessor --> TagGenerator
    PreProcessor --> NewsCreator
    TagGenerator --> PostgreSQL
    NewsCreator --> PostgreSQL
    NewsCreator -.-> Ollama
    SearchIndexer --> Meilisearch

    %% Logging Flow
    REST -.-> LogForwarder
    PreProcessor -.-> LogForwarder
    TagGenerator -.-> LogForwarder
    NewsCreator -.-> LogForwarder
    SearchIndexer -.-> LogForwarder
    LogForwarder --> LogAggregator
    LogAggregator --> ClickHouse

    %% External Data Sources
    Driver --> RSSFeeds
```
