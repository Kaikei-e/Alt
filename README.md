[![Backend Go Tests](https://github.com/Kaikei-e/Alt/actions/workflows/backend-go.yaml/badge.svg)](https://github.com/Kaikei-e/Alt/actions/workflows/backend-go.yaml)
[![Alt Frontend SV Tests](https://github.com/Kaikei-e/Alt/actions/workflows/alt-frontend-sv.yml/badge.svg?branch=main)](https://github.com/Kaikei-e/Alt/actions/workflows/alt-frontend-sv.yml)
[![Tag Generator](https://github.com/Kaikei-e/Alt/actions/workflows/tag-generator.yaml/badge.svg)](https://github.com/Kaikei-e/Alt/actions/workflows/tag-generator.yaml)
[![Pre-processor Quality Gates](https://github.com/Kaikei-e/Alt/actions/workflows/pre-processor-quality.yml/badge.svg)](https://github.com/Kaikei-e/Alt/actions/workflows/pre-processor-quality.yml)
[![Search Indexer Tests](https://github.com/Kaikei-e/Alt/actions/workflows/search-indexer.yaml/badge.svg)](https://github.com/Kaikei-e/Alt/actions/workflows/search-indexer.yaml)
[![Pre-processor Sidecar Go Tests](https://github.com/Kaikei-e/Alt/actions/workflows/pre-processor-sidecar-go.yaml/badge.svg)](https://github.com/Kaikei-e/Alt/actions/workflows/pre-processor-sidecar-go.yaml)
[![Recap Worker CI](https://github.com/Kaikei-e/Alt/actions/workflows/recap-worker.yml/badge.svg)](https://github.com/Kaikei-e/Alt/actions/workflows/recap-worker.yml)


# Alt – Compose-First AI Knowledge Platform

_Last reviewed on January 8, 2026._

> Compose-first knowledge platform that ingests RSS content, enriches it with AI (LLM summaries, tag extraction, RAG-powered Q&A), and serves curated insights with a unified developer workflow across Go, Python, Rust, and TypeScript (Next.js + SvelteKit) services.

## Table of Contents

- [Platform Snapshot](#platform-snapshot)
- [Architecture](#architecture)
  - [Data Flow Overview](#data-flow-overview)
  - [Compose Topology](#compose-topology)
  - [Data Intelligence Flow](#data-intelligence-flow)
  - [Microservice Communication Map](#microservice-communication-map)
  - [Identity & Edge Access](#identity--edge-access)
- [Technology & Version Matrix](#technology--version-matrix)
- [Getting Started](#getting-started)
  - [Prerequisites](#prerequisites)
  - [First-Time Setup](#first-time-setup)
  - [Compose Profiles](#compose-profiles)
  - [Developer Setup Checklist](#developer-setup-checklist)
- [Service Catalog & Documentation](#service-catalog--documentation)
- [Service Deep Dives](#service-deep-dives)
- [Recap Experience & Pipeline](#recap-experience--pipeline)
  - [Mobile & API Surfaces](#mobile--api-surfaces)
  - [Running the Recap Stack](#running-the-recap-stack)
  - [Pipeline Flow](#pipeline-flow)
- [Observability & Operations](#observability--operations)
- [Development Workflow & Testing](#development-workflow--testing)
- [Testing Playbook](#testing-playbook)
- [Data & Storage](#data--storage)
- [Security & Compliance](#security--compliance)
- [External Integrations](#external-integrations)
- [Contribution Checklist](#contribution-checklist)
- [Roadmap & Historical Context](#roadmap--historical-context)
- [Change Management & Communication](#change-management--communication)
- [Troubleshooting & FAQs](#troubleshooting--faqs)
- [Glossary](#glossary)
- [Reference Resources](#reference-resources)
- [Appendix](#appendix)

## Platform Snapshot

- Compose-first developer experience: `make up` builds images, runs Atlas migrations, and starts the full stack under Docker Compose v2 profiles.
- Clean Architecture across languages: Go services follow handler → usecase → port → gateway → driver, while Python, Rust, and TypeScript counterparts mirror the same contract-first approach.
- Dual frontend architecture: Next.js 16 (React 19) at root path for full-featured desktop/mobile UI, plus SvelteKit 5 (Svelte Runes) at `/sv` for high-performance modern dashboards with Connect-RPC integration.
- Type-safe API evolution: Connect-RPC (port 9101) runs parallel to REST (port 9000), enabling Protocol Buffers-defined schemas with auto-generated Go/TypeScript clients via `make buf-generate`.
- AI enrichment pipeline: pre-processor deduplicates and scores feeds, news-creator produces Ollama summaries, and tag-generator runs ONNX-backed extraction with SentenceTransformer fallback, keeps a background worker thread alive for batch throughput, and exposes a service-token gated `/api/v1/tags/batch` so downstream systems (Recap, recap-worker replays, etc.) can fetch cascade-aware tags and refresh the rolling `tag_label_graph` priors.
- RAG-powered knowledge Q&A: rag-orchestrator indexes articles into pgvector chunks, retrieves relevant context via similarity search, and generates grounded answers with citations through knowledge-augur (Ollama LLM) and knowledge-embedder services.
- Recap experience: recap-worker (Rust 2024) + recap-subworker (FastAPI) condense the latest seven days of articles into genre cards, evidence links, and summaries that power the mobile `/mobile/recap/7days` dashboard and backend `/v1/recap/7days` API, while deduplicating evidence, persisting `recap_cluster_evidence`, emitting `recap_genre_learning_results`, and shipping golden dataset evaluation metrics plus an offline `scripts/replay_genre_pipeline` helper.
- Recap ingestion: the backend also hosts `GET /v1/recap/7days` (public) plus service-authenticated `POST /v1/recap/articles`; the latter enforces `X-Service-Token` + `RecapRateLimiter` meters, returns `X-RateLimit-*` headers, and validates `from/to` RFC3339 ranges before calling `RecapGateway`.
- Dedicated recap-db (PostgreSQL 18) tracks jobs, cached articles, cluster evidence, tag-label graph priors, refine telemetry, and published recaps so reruns stay deterministic and audits replayable via Atlas migrations in `recap-migration-atlas/`.
- Search-ready delivery: search-indexer batches 200-document upserts into Meilisearch 1.27.0 with tuned searchable/filterable attributes and semantic-ready schema defaults.
- Observability built in: Rust rask log services stream structured JSON into ClickHouse 25.9, complemented by health endpoints and targeted dashboards.
- Streamed metrics: `/v1/sse/feeds/stats` keeps a heartbeat (10s) + ticker (configured via `SERVER_SSE_INTERVAL`) open so dashboards immediately reflect `feedAmount`, `unsummarizedFeedAmount`, and `articleAmount`.
- Identity at the edge: auth-hub validates Kratos sessions, emits authoritative `X-Alt-*` headers, and caches them for five minutes so downstream services remain auth-agnostic.
- TDD-first change management: every service mandates Red → Green → Refactor with exhaustive unit suites, integration hooks, and deterministic mocks before production merges.
- Developer ergonomics & safety: shared Make targets, lint/format tooling, env guards, and secrets hygiene keep onboarding fast and safe.
- Production parity: Compose profiles mirror production paths so GPU summarisation (`ollama`) and log pipelines (`logging`) toggle locally without ad-hoc scripts.

## Architecture

Alt is designed to keep local parity with production by centering on Docker Compose while preserving historical Kubernetes manifests for reference only.

## Data Flow Overview

- Ingested RSS feeds enter the Go pre-processor, undergo deduplication/sanitation, and emit canonical articles plus summaries for downstream services (`docs/pre-processor.md`).
- The tag-generator consumes those articles, runs the ONNX-backed extractor with cascade controls, and refreshes the `tag_label_graph` priors that recap-worker uses during genre refinement (`docs/tag-generator.md`).
- The recap pipeline (worker + subworker + recap-db + news-creator) orchestrates evidence deduction, clustering, LLM summarisation, and persistence of deduplicated proof links plus genre learning results (`docs/recap-worker.md`, `docs/recap-subworker.md`, `docs/recap-db.md`), while `alt-backend` surfaces the curated recap and articles APIs (`docs/alt-backend.md`).
- The backend also exposes `/v1/sse/feeds/stats` (heartbeat + periodic tickers) for live feed metrics and a service-authenticated `POST /v1/recap/articles` that enforces `X-Service-Token`, paged `from/to` queries, and `RecapRateLimiter` headers before handing data to recap-worker; these flows keep dashboards in sync and avoid Kratos/session churn.
- Observability services (rask log forwarder/aggregator) capture `recap_genre_refine_*` counters, `recap_api_evidence_duplicates_total`, and related metrics for ClickHouse dashboards (`docs/rask-log-forwarder.md`, `docs/rask-log-aggregator.md`).
- Identity flows (auth-hub, auth-token-manager) ensure Kratos sessions and Inoreader tokens stay fresh for the entire pipeline, and the frontend renders the `/mobile/recap/7days` experience from the recap summary DTOs (`docs/auth-hub.md`, `docs/auth-token-manager.md`, `docs/alt-frontend.md`).

```mermaid
flowchart LR
    subgraph Ingestion
        RSS[External RSS / Inoreader] --> PreProc[pre-processor :9200<br/>dedupe + summary]
    end
    subgraph Tagging
        PreProc -->|/api/v1/summarize| TagGen[tag-generator<br/>ONNX + cascade]
        TagGen --> Graph[tag_label_graph]
        TagGen -->|SQL| AltDB[(Postgres `<article_tags>`)]
    end
    subgraph RecapPipeline
        AltBackend[alt-backend :9000/:9101<br/>REST + Connect-RPC]
        RecapWorker[recap-worker :9005<br/>fetch → dedup → genre → evidence]
        RecapSub[recap-subworker :8002<br/>clustering + dedup]
        RecapDB[(recap-db<br/>jobs, cluster evidence, graph, learning results)]
        NewsCreator[news-creator :8001<br/>Ollama summaries]
        RecapWorker -->|/v1/runs| RecapSub
        RecapWorker -->|/v1/summary/generate| NewsCreator
        RecapWorker -->|SQL| RecapDB
        RecapWorker -->|/v1/recap/articles| AltBackend
        RecapSub -->|SQL| RecapDB
        NewsCreator -->|SQL| RecapDB
        AltBackend -->|/v1/recap/7days| RecapWorker
    end
    subgraph "Frontend & Metrics"
        RecapDB --> Frontend[alt-frontend<br/>/mobile/recap/7days]
        RecapWorker --> Logs[rask-log-forwarder]
        Logs -->|/v1/aggregate| ClickHouse[ClickHouse via rask-log-aggregator :9600]
        AuthHub[auth-hub :8888<br/>Kratos → X-Alt-*] --> AltBackend
        AuthToken[auth-token-manager<br/>Inoreader OAuth] --> PreProc
    end
    Graph --> RecapWorker
    PreProc --> AltBackend
    AuthHub --> Frontend
    AuthToken --> TagGen
```

### Compose Topology

```mermaid
flowchart TD
    classDef client fill:#e6f4ff,stroke:#1f5aa5,stroke-width:2px
    classDef edge fill:#f2e4ff,stroke:#8a4bd7,stroke-width:2px
    classDef core fill:#e8f5e9,stroke:#2f855a,stroke-width:2px
    classDef ai fill:#fff4e5,stroke:#f97316,stroke-width:2px,stroke-dasharray:4
    classDef data fill:#fef3c7,stroke:#d97706,stroke-width:2px
    classDef recap fill:#fce7f3,stroke:#db2777,stroke-width:2px,stroke-dasharray:4
    classDef rag fill:#dbeafe,stroke:#1d4ed8,stroke-width:2px,stroke-dasharray:4
    classDef obs fill:#fde4f7,stroke:#c026d3,stroke-width:2px,stroke-dasharray:4

    Browser((Browser)):::client
    Browser --> Nginx

    subgraph Edge["Edge :80"]
        Nginx[nginx]:::edge
        Nginx --> AuthHub[auth-hub :8888]:::edge
    end

    AuthHub --> Kratos
    Nginx --> UI
    Nginx --> UISv

    subgraph FE["Frontend"]
        UI[alt-frontend :3000]:::core
        UISv[alt-frontend-sv :4173]:::core
    end

    UI --> API
    UISv --> API

    subgraph BE["Backend"]
        API[alt-backend :9000/:9101]:::core
        API --> Idx[search-indexer :9300]:::core
        API --> Tag[tag-generator :9400]:::core
    end

    API --> RW
    API --> RO
    Idx --> Meili
    Tag --> DB

    subgraph OL["Profile: ollama"]
        PP[pre-processor :9200]:::ai
        PP --> NC[news-creator :11434]:::ai
    end

    subgraph RC["Profile: recap"]
        RW[recap-worker :9005]:::recap
        RW --> RS[recap-subworker :8002]:::recap
    end

    subgraph RG["Profile: rag-extension"]
        RO[rag-orchestrator :9010]:::rag
    end

    PP --> DB
    RW --> NC
    RW --> RecapDB
    RO --> RagDB
    RO --> Idx

    subgraph Data["Data Stores"]
        DB[(db :5432)]:::data
        Meili[(meilisearch :7700)]:::data
        Kratos[(kratos :4433)]:::data
        RecapDB[(recap-db :5435)]:::recap
        RagDB[(rag-db :5436)]:::rag
    end

    subgraph Obs["Profile: logging"]
        Rask[rask-log-agg :9600]:::obs
        Rask --> CH[(clickhouse :8123)]:::obs
    end
```

**Legend:**

| Style | Profile | Description |
|-------|---------|-------------|
| Green (solid) | default | Core services (always running) |
| Orange (dashed) | `--profile ollama` | AI/LLM services |
| Pink (dashed) | `--profile recap` | Summarization pipeline |
| Blue (dashed) | `--profile rag-extension` | RAG services |
| Magenta (dashed) | `--profile logging` | Log aggregation |
| Yellow | — | Data stores |

**Network:** `alt-network` (shared by all services)

**Persistent Volumes:**
| Volume | Service | Description |
|--------|---------|-------------|
| `db_data_17` | db | PostgreSQL 17 main data |
| `kratos_db_data` | kratos-db | Identity provider data |
| `meili_data` | meilisearch | Search indices |
| `recap_db_data` | recap-db | Recap pipeline data |
| `rag_db_data` | rag-db | RAG vectors (pgvector) |
| `clickhouse_data` | clickhouse | Log analytics |
| `news_creator_models` | news-creator | Ollama LLM models |
| `oauth_token_data` | auth-token-manager | OAuth2 tokens |

### Data Intelligence Flow

```mermaid
flowchart LR
    classDef ingest fill:#e0f7fa,stroke:#00838f,color:#004d40
    classDef ai fill:#ffe0f0,stroke:#d81b60,color:#880e4f
    classDef storage fill:#fff4d5,stroke:#fb8c00,color:#5d2c00
    classDef surface fill:#e8f5e9,stroke:#388e3c,color:#1b5e20

    RSS[External RSS feeds]:::ingest --> Fetch[pre-processor :9200<br/>Fetch & dedupe]:::ingest
    Fetch --> Score[Quality scoring + language detection]:::ingest
    Score -->|SQL| RawDB[(PostgreSQL<br/>raw articles)]:::storage
    RawDB --> TagJob[tag-generator<br/>/api/v1/tags/batch]:::ai
    RawDB --> SummaryJob[news-creator :8001<br/>/api/v1/summarize]:::ai
    TagJob -->|SQL| TagDB[(PostgreSQL<br/>article_tags)]:::storage
    SummaryJob -->|SQL| SummaryDB[(PostgreSQL<br/>article_summaries)]:::storage
    TagDB --> IndexBatch[search-indexer :9300<br/>batch 200 docs]:::ai
    SummaryDB --> IndexBatch
    IndexBatch -->|upsert| Meili[(Meilisearch :7700<br/>search index)]:::storage
    Meili --> API[alt-backend :9000<br/>/v1/search + /v1/feeds/*]:::surface
    API --> Frontend[alt-frontend :3000<br/>Chakra themes]:::surface
```

### Microservice Communication Map

Direct API calls and data flow between microservices.

```mermaid
flowchart LR
    classDef client fill:#e6f4ff,stroke:#1f5aa5,stroke-width:2px
    classDef edge fill:#f2e4ff,stroke:#8a4bd7,stroke-width:2px
    classDef fe fill:#d4e6f1,stroke:#2874a6,stroke-width:2px
    classDef be fill:#d5f5e3,stroke:#1e8449,stroke-width:2px
    classDef wk fill:#fcf3cf,stroke:#d4ac0d,stroke-width:2px
    classDef db fill:#fef3c7,stroke:#d97706,stroke-width:2px
    classDef rag fill:#dbeafe,stroke:#1d4ed8,stroke-width:2px,stroke-dasharray:4
    classDef obs fill:#fde4f7,stroke:#c026d3,stroke-width:2px,stroke-dasharray:4

    subgraph Entry["Entry"]
        direction TB
        U((User)):::client
        U --> N[nginx :80]:::edge
        N --> A[auth-hub :8888]:::edge
        A --> K[(kratos)]:::db
        K --> KD[(kratos-db)]:::db
    end

    subgraph Frontend
        direction TB
        F1[alt-frontend :3000]:::fe
        F2[alt-frontend-sv :4173]:::fe
    end

    subgraph Core["Core"]
        direction TB
        API[alt-backend<br/>:9000/:9101]:::be
        API --> Idx[search-indexer :9300]:::wk
        API --> Tag[tag-generator :9400]:::wk
        Idx --> DB[(db :5432)]:::db
        Tag --> DB
        Idx --> M[(meilisearch :7700)]:::db
    end

    subgraph AI["AI Pipeline"]
        direction TB
        PP[pre-processor :9200]:::wk
        PP --> NC[news-creator :11434]:::wk
        NC --> DB2[(db)]:::db
    end

    subgraph Recap["Recap"]
        direction TB
        RW[recap-worker :9005]:::wk
        RW --> RS[recap-subworker :8002]:::wk
        RW --> RD[(recap-db :5435)]:::db
        RS --> RD
    end

    subgraph RAG["RAG"]
        direction TB
        RO[rag-orchestrator :9010]:::rag
        RO --> AU[knowledge-augur]:::rag
        RO --> EM[knowledge-embedder]:::rag
        RO --> VD[(rag-db :5436)]:::db
    end

    subgraph Obs["Logging"]
        direction TB
        LF[log-forwarders]:::obs
        LF --> LA[rask-log-agg :9600]:::obs
        LA --> CH[(clickhouse :8123)]:::obs
    end

    N --> F1 & F2
    F1 & F2 --> API
    API --> PP
    API --> RW
    API --> RO
    RW --> NC
    N -.-> LF
    API -.-> LF
```

### Identity & Edge Access

Nginx fronts every `/api` call with `auth_request`, sending it to auth-hub. auth-hub validates the session via Kratos `/sessions/whoami`, caches the result for five minutes, and forwards authoritative `X-Alt-*` headers. alt-backend trusts those headers for user context while delegating outbound HTTP to `sidecar-proxy`, which enforces HTTPS allowlists and shared timeouts.

#### Component Responsibilities

- **Client tier** – Next.js UI (root path) and SvelteKit UI (`/sv` path) deliver responsive dashboards, handle optimistic interactions, and mirror backend feature flags via `NEXT_PUBLIC_*` / `PUBLIC_*` variables. SvelteKit uses Connect-RPC for type-safe API calls.
- **Edge tier** – Nginx terminates TLS (when enabled), normalises headers, triggers auth-hub checks, and fan-outs requests to backend APIs or static assets.
- **Core services** – alt-backend orchestrates domain logic, while pre-processor, tag-generator, news-creator, and search-indexer cooperate to enrich, store, and surface content.
- **Data tier** – PostgreSQL persists canonical entities, Meilisearch powers discovery, ClickHouse retains observability telemetry, and Kratos maintains identities.
- **Observability tier** – Rust rask services guarantee durable log delivery, enabling replay into ClickHouse dashboards and anomaly detectors.
- **Optional profiles** – `ollama` introduces GPU inference footprint, `logging` deploys extended telemetry, and additional bespoke profiles can be layered for experiments.

#### Deployment Interaction Diagram

```mermaid
sequenceDiagram
    participant User
    participant Browser
    participant Nginx
    participant AuthHub
    participant Kratos
    participant Backend
    participant Sidecar
    participant ExternalAPI

    User->>Browser: Request dashboard
    Browser->>Nginx: GET /api/articles
    Nginx->>AuthHub: auth_request /validate
    AuthHub->>Kratos: GET /sessions/whoami
    Kratos-->>AuthHub: Session payload
    AuthHub-->>Nginx: 200 + X-Alt-* headers
    Nginx->>Backend: GET /api/articles (with headers)
    Backend->>Sidecar: Fetch RSS feed (if stale)
    Sidecar->>ExternalAPI: GET https://example.com/rss
    ExternalAPI-->>Sidecar: RSS XML
    Sidecar-->>Backend: Normalised response
    Backend-->>Browser: Article JSON payload
    Browser-->>User: Rendered dashboard
```

### Connect-RPC Architecture

Alt runs REST (port 9000) and Connect-RPC (port 9101) in parallel, enabling gradual migration to type-safe APIs while maintaining backward compatibility.

**Key Design Decisions:**
- **Parallel operation** – REST `/v1/*` on port 9000 remains the default; Connect-RPC on port 9101 serves type-safe clients (primarily alt-frontend-sv).
- **Protocol Buffers** – Schema definitions in `proto/alt/` generate Go handlers and TypeScript clients via `make buf-generate`.
- **Auth interceptor** – JWT validation via `X-Alt-Backend-Token` header, reusing existing auth-hub token exchange.
- **5 Services, 21 Methods** – ArticleService (3), FeedService (10), RSSService (4), AugurService (2), MorningLetterService (1 via rag-orchestrator).

**Connect-RPC Services:**

| Service | Methods | Streaming | Description |
|---------|---------|-----------|-------------|
| ArticleService | 3 | No | FetchArticleContent, ArchiveArticle, FetchArticlesCursor |
| FeedService | 10 | Yes | Stats, feeds, search, summarize (StreamFeedStats, StreamSummarize) |
| RSSService | 4 | No | RegisterRSSFeed, ListRSSFeedLinks, DeleteRSSFeedLink, RegisterFavoriteFeed |
| AugurService | 2 | Yes | StreamChat (RAG Q&A), RetrieveContext |
| MorningLetterService | 1 | Yes | StreamChat (via rag-orchestrator :9010) |

```mermaid
flowchart TD
    subgraph Frontends
        NextJS[alt-frontend :3000<br/>Next.js REST client]
        SvelteKit[alt-frontend-sv :4173<br/>Connect-RPC + REST]
    end
    subgraph "alt-backend :9000/:9101"
        REST[REST API<br/>:9000 /v1/*]
        subgraph ConnectRPC[:9101 Connect-RPC]
            ArticleSvc[ArticleService<br/>3 methods]
            FeedSvc[FeedService<br/>10 methods ★stream]
            RSSSvc[RSSService<br/>4 methods]
            AugurSvc[AugurService<br/>2 methods ★stream]
            MLGateway[MorningLetterService<br/>Gateway → rag-orchestrator]
        end
    end
    subgraph "rag-orchestrator :9010"
        MLSvc[MorningLetterService<br/>Server ★stream]
    end
    Proto[proto/*.proto<br/>Protocol Buffers]

    NextJS --> REST
    SvelteKit --> REST
    SvelteKit --> ArticleSvc
    SvelteKit --> FeedSvc
    SvelteKit --> RSSSvc
    SvelteKit --> AugurSvc
    MLGateway -->|gRPC| MLSvc
    Proto -.->|buf generate| ConnectRPC
    Proto -.->|buf generate| MLSvc
    Proto -.->|buf generate| SvelteKit
```

### RAG Pipeline Architecture

The RAG (Retrieval Augmented Generation) pipeline enables knowledge-based question answering with grounded, citation-backed responses.

**Pipeline Components:**
- **rag-orchestrator :9010** – Go service managing document indexing, context retrieval, and answer generation. Implements MorningLetterService for Connect-RPC.
- **rag-db** – PostgreSQL 18 with pgvector extension for vector similarity search.
- **knowledge-embedder** – Generates vector embeddings for article chunks.
- **knowledge-augur** – Ollama-based LLM for generating answers from retrieved context.

**Flow:**
1. **Indexing** – Articles are chunked, embedded, and stored with version tracking.
2. **Retrieval** – Queries are embedded and matched against chunks via pgvector similarity search.
3. **Generation** – Top-K context chunks are assembled into a prompt for LLM generation with structured JSON output.

**Connect-RPC Integration:**
- alt-backend → rag-orchestrator via MorningLetterService (StreamChat for time-bounded RAG Q&A)
- AugurService (alt-backend) for general RAG queries via RetrieveContext + StreamChat

```mermaid
flowchart LR
    subgraph Indexing
        Article[Article Content] --> IndexUC[rag-orchestrator :9010<br/>Index Usecase]
        IndexUC --> Chunk[Chunker]
        Chunk --> Embed[knowledge-embedder<br/>Vector Encoding]
        Embed -->|SQL + vector| RagDB[(rag-db<br/>pgvector)]
    end
    subgraph "Query & Answer"
        Query[User Query via Connect-RPC] --> Retrieve[rag-orchestrator :9010<br/>Retrieve Context]
        Retrieve -->|similarity search| RagDB
        RagDB --> Context[Top K Chunks]
        Context --> Answer[Answer Usecase]
        Answer --> Augur[knowledge-augur<br/>LLM Generation]
        Augur -->|Ollama| Response[Answer + Citations]
    end
    subgraph "Connect-RPC Entry"
        AltBackend[alt-backend :9101<br/>MorningLetterService Gateway] -->|gRPC| Retrieve
        AltBackend2[alt-backend :9101<br/>AugurService] -->|StreamChat| Answer
    end
```

## Technology & Version Matrix

| Layer | Primary Tech | Version (Dec 2025) | Notes |
| --- | --- | --- | --- |
| Web UI (Next.js) | Next.js 16, React 19.2, TypeScript 5.9, pnpm 10.25 | Node.js 24 LTS | Chakra UI 3.30; App Router; Playwright 1.57 + Vitest 4.0. |
| Web UI (SvelteKit) | SvelteKit 2.49, Svelte 5.46, TailwindCSS v4, Vite 7.3 | Node.js 24 LTS | Svelte 5 Runes; `/sv` base path; Connect-RPC client; Biome linter. |
| Go API & RPC | Go 1.24/1.25, Echo 4.14, Connect-RPC 1.19 | Port 9000 (REST), 9101 (RPC) | Clean Architecture with GoMock; Protocol Buffers via `make buf-generate`. |
| Go Data Pipeline | Go 1.24/1.25, `mercari/go-circuitbreaker`, `singleflight` | - | Pre-processor, scheduler, search-indexer; rate limit ≥5 s; 200-doc Meilisearch batches. |
| RAG Pipeline | Go 1.25, pgvector, Ollama | Port 9010 | rag-orchestrator + knowledge-augur + knowledge-embedder; chunk-based retrieval with LLM generation. |
| Python AI Services | Python 3.11+/3.12/3.13, FastAPI, Ollama, `uv` | Ollama 0.3.x | news-creator (3.11+), recap-subworker (3.12), tag-generator (3.13+); Clean Architecture; Ruff gates. |
| Recap pipeline | Rust 1.87 (Axum, Tokio, sqlx), FastAPI 0.115, PostgreSQL 18 recap-db | Rust 2024 edition | recap-worker orchestrates fetch→preprocess→dedupe→genre→persist; recap-subworker clusters evidence; news-creator via `ollama` profile. |
| Identity & Tokens | Ory Kratos 1.3.0, auth-hub (Go 1.25) | - | 5-minute TTL cache; emits `X-Alt-*` headers; auth-token-manager for Inoreader OAuth. |
| Observability | Rust 1.87 (2024 edition), ClickHouse 25.9 | - | SIMD log forwarder; Axum aggregator; `criterion` benchmarks. |
| Storage & Search | PostgreSQL 17/18, Meilisearch 1.27.0 | - | Atlas migrations; pgvector for RAG; tuned searchable/filterable attributes; persisted volumes. |
| Orchestration | Docker Desktop 4.36+, Compose v2.27+, Makefile | - | `make up/down/build`; profiles: `ollama`, `logging`, `recap`, `rag-extension`. |

> **Version cadence:** Go/Rust toolchains track stable releases quarterly, Next.js/SvelteKit updates follow LTS adoption, and Python runtimes are pinned per service to avoid cross-environment drift. Update the matrix whenever upgrade stories land.

## Getting Started

### Prerequisites

- Docker Desktop 4.36+ (or Colima/Lima with Compose v2.27+) with at least 4 CPU / 8 GB memory allocated.
- Node.js 24 LTS with `pnpm` ≥10 installed globally (`corepack enable pnpm`).
- Go 1.24.x or 1.25.x toolchain with `GOBIN` on your `PATH`.
- Python 3.12/3.13 (for tag-generator, recap-subworker) with `uv` for environment management.
- Rust 1.87 (2024 edition) and Cargo for recap-worker and rask-* services.
- Optional: GPU runtime (CUDA 12+) if you plan to run Ollama locally for news-creator and RAG.

### First-Time Setup

1. **Install dependencies** – run `pnpm -C alt-frontend install`, `uv sync --project tag-generator/app`, `uv sync --project news-creator/app`, and `go mod download ./...`.
2. **Seed environment** – copy `.env.template` to `.env`; `make up` performs this automatically if the file is missing.
3. **Start the stack** – execute `make up` to build images, run Atlas migrations, seed Meilisearch, and boot the default profile.
4. **Verify health** – hit `http://localhost:3000/api/health`, `http://localhost:9000/v1/health`, `http://localhost:7700/health`, and `http://localhost:8888/health`.
5. **Stop or reset** – use `make down` to stop while retaining volumes or `make down-volumes` to reset data.
6. **Configure service secrets & models** – set `SERVICE_SECRET`, `TAG_LABEL_GRAPH_WINDOW`, and `TAG_LABEL_GRAPH_TTL_SECONDS` inside `.env`, place the ONNX assets under `tag-generator/models/onnx`, and let Compose mount them so tag-generator can reuse the ONNX runtime volume without extra downloads.

### Compose Profiles

- **Default** – Frontend (Next.js + SvelteKit), backend, PostgreSQL 17, Kratos, Meilisearch, search-indexer, tag-generator (mounts `./tag-generator/models/onnx` for the ONNX runtime and respects `SERVICE_SECRET`), ClickHouse, rask-log-aggregator.
- **`--profile ollama`** – Adds news-creator (FastAPI + Ollama) and pre-processor ingestion services with persistent model volume at `news_creator_models`.
- **`--profile logging`** – Launches rask-log-forwarder sidecars (8 services) that stream container logs into the aggregator; includes `x-rask-env` defaults.
- **`--profile recap`** – Starts recap-worker (Rust), recap-subworker (FastAPI), recap-db (PostgreSQL 18), dashboard, and the recap Atlas migrator. Pair it with `--profile ollama` so news-creator is available to finish summaries.
- **`--profile rag-extension`** – Starts rag-orchestrator, rag-db (PostgreSQL 18 + pgvector), rag-db-migrator, knowledge-augur, and knowledge-embedder for RAG-powered Q&A.

Enable combinations as needed:
```bash
docker compose --profile ollama --profile logging up -d
docker compose --profile recap --profile ollama up -d
docker compose --profile rag-extension up -d
```

### Developer Setup Checklist

1. **Install toolchains** – Docker Desktop/Colima, Go 1.24/1.25, Node.js 24 + `pnpm`, Python 3.12/3.13 with `uv`, and Rust 1.87 should all respond to `--version`.
2. **Bootstrap dependencies** – Run `pnpm -C alt-frontend install`, `uv sync` for Python services, `go mod download ./...`, and `cargo fetch` to warm caches.
3. **Prepare environment** – Copy `.env.template` to `.env`, fill local-safe secrets, and confirm `scripts/check-env.js` passes.
4. **Smoke the stack** – Execute `pnpm -C alt-frontend build`, `go test ./...`, `uv run pytest`, `cargo test`, then `make up`/`make down` to validate orchestration.
5. **Align practices** – Read root/service `CLAUDE.md`, enable editor format-on-save, install optional pre-commit hooks, and keep credentials out of git.
6. **Recap-specific prep** – Run `make recap-migrate` (Atlas-backed) once before bringing up the `recap` profile, and confirm `http://localhost:9005/health/ready` plus `http://localhost:8002/health/ready` respond when testing the new mobile Recap surfaces.

## Service Catalog & Documentation

Each microservice maintains a `CLAUDE.md` for process guardrails plus a snapshot under `docs/<service>.md` that captures the latest architecture, configuration, and testing guidance. The table below links the primary doc per service so you can jump straight to the most concrete contract before coding.

| Service | Primary Doc | Focus |
| --- | --- | --- |
| `alt-frontend/` | [docs/alt-frontend.md](docs/alt-frontend.md) | Next.js 16 + React 19 App Router UI with Chakra themes and the `/mobile/recap/7days` experience. |
| `alt-frontend-sv/` | [docs/alt-frontend-sv.md](docs/alt-frontend-sv.md) | SvelteKit `/sv` experience with Runes, Tailwind v4, Connect-RPC client, and authenticated dashboards. |
| `alt-backend/app/` | [docs/alt-backend.md](docs/alt-backend.md) | Go 1.24/1.25 Clean Architecture REST + Connect-RPC API, SSE + recap endpoints, and background job runners. |
| `alt-backend/sidecar-proxy/` | [docs/sidecar-proxy.md](docs/sidecar-proxy.md) | Go egress proxy that enforces HTTPS allowlists, bypasses internal DNS, and exposes health/metrics/debug hooks. |
| `pre-processor/app/` | [docs/pre-processor.md](docs/pre-processor.md) | Go ingestion service with dedupe/sanitization, circuit breakers, and async summarization queue. |
| `pre-processor-sidecar/app/` | [docs/pre-processor-sidecar.md](docs/pre-processor-sidecar.md) | Scheduler handling Inoreader token refresh, Cron-mode toggles, and secret-watchers. |
| `news-creator/app/` | [docs/news-creator.md](docs/news-creator.md) | FastAPI Ollama orchestrator with Model Bucket Routing, Map-Reduce summarisation, and golden prompts. |
| `tag-generator/app/` | [docs/tag-generator.md](docs/tag-generator.md) | Python 3.13 ML pipeline batching tag extractions, cascade controls, and rolling `tag_label_graph` priors. |
| `search-indexer/app/` | [docs/search-indexer.md](docs/search-indexer.md) | Go batch indexer (200 docs) into Meilisearch with tokenizer + schema bootstrapping plus `/v1/search`. |
| `auth-hub/` | [docs/auth-hub.md](docs/auth-hub.md) | Kratos-aware IAP that validates sessions, caches 5m, and emits authoritative `X-Alt-*` headers. |
| `auth-token-manager/` | [docs/auth-token-manager.md](docs/auth-token-manager.md) | Deno OAuth2 CLI refreshing Inoreader tokens, writing Kubernetes secrets, and monitoring horizons. |
| `rask-log-forwarder/app/` | [docs/rask-log-forwarder.md](docs/rask-log-forwarder.md) | Rust SIMD log tailer with buffering, disk fallback, and ClickHouse-ready batching. |
| `rask-log-aggregator/app/` | [docs/rask-log-aggregator.md](docs/rask-log-aggregator.md) | Axum API ingesting JSON logs and persisting to ClickHouse via exporter traits. |
| `recap-worker/` | [docs/recap-worker.md](docs/recap-worker.md) | Rust pipeline orchestrating fetch → preprocess → dedup → genre → evidence → news-creator → persist. |
| `recap-subworker/` | [docs/recap-subworker.md](docs/recap-subworker.md) | FastAPI worker running clustering, classification, diagnostics, and admin learning jobs. |
| `recap-db` | [docs/recap-db.md](docs/recap-db.md) | PostgreSQL schema contract for recap jobs, sections, evidence, tag graphs, and learning results. |
| `rag-orchestrator/` | [docs/rag-orchestrator.md](docs/rag-orchestrator.md) | Go RAG service: article indexing, vector retrieval via pgvector, and LLM answer generation with citations. |
| `rag-db` | [docs/rag-db.md](docs/rag-db.md) | PostgreSQL 18 + pgvector for RAG documents, versions, chunks, and events. |
| `knowledge-augur/` | – | Ollama LLM variant for knowledge-based question answering with structured JSON output. |
| `knowledge-embedder/` | – | Embedding service for generating vector representations of article chunks. |
| `altctl/` | [altctl/CLAUDE.md](altctl/CLAUDE.md) | Go CLI tool for Docker Compose orchestration with stack-based dependency management. |
| `alt-perf/` | [alt-perf/CLAUDE.md](alt-perf/CLAUDE.md) | Deno E2E performance measurement tool using Astral for Core Web Vitals testing. |

Additional reference docs include `docs/Alt-Architecture-07.md` for historical Kubernetes/Compose topology, the `docs/recap-*` retrospectives (runbooks, investigations, and pipeline notes), and the `docs/recap-7days-pipeline.md` overview for mobile/genre sequencing. Keep `CLAUDE.md` plus the linked snapshot fresh as you edit.

## Service Deep Dives

Every `docs/<service>.md` snapshot pairs prose with architecture diagrams, configuration, testing commands, and operational notes derived from the current tree. Consult the linked doc before touching the service and refresh it whenever contracts or lint/test surface changes.

- **alt-frontend** – Next.js 16 App Router UI with Chakra palettes, SWR/react-query caching, middleware-protected flows, and the `/mobile/recap/7days` experience that now renders evidence links + genre payloads. [docs/alt-frontend.md](docs/alt-frontend.md)
- **alt-frontend-sv** – SvelteKit `/sv` gateway with Svelte 5 Runes, Tailwind v4, Connect-RPC client for type-safe APIs, SSE hooks, and Kratos-aware middleware powering the modern dashboard while exchanging tokens via auth-hub. [docs/alt-frontend-sv.md](docs/alt-frontend-sv.md)
- **alt-backend/app** – Go 1.24/1.25 Clean Architecture API (handler → usecase → port → gateway → driver) with REST (port 9000) + Connect-RPC (port 9101), GoMock suites, Atlas migrations, structured slog, and the recap endpoints `/v1/recap/7days` + service-auth `/v1/recap/articles`. [docs/alt-backend.md](docs/alt-backend.md)
- **alt-backend/sidecar-proxy** – Go egress proxy enforcing HTTPS allowlists, external DNS resolution, CONNECT tunnels, health/metrics/debug endpoints, and structured logging while remaining light enough to attach to Compose/Kubernetes. [docs/sidecar-proxy.md](docs/sidecar-proxy.md)
- **pre-processor/app** – Go ingestion worker that deduplicates feeds, sanitizes content, queues summarisation jobs, and obeys 5-second host pacing with circuit breakers. [docs/pre-processor.md](docs/pre-processor.md)
- **pre-processor-sidecar/app** – Scheduler rotating Inoreader tokens, syncing subscriptions, and exposing health/admin endpoints; uses `singleflight`, pluggable clocks, and secret watching. [docs/pre-processor-sidecar.md](docs/pre-processor-sidecar.md)
- **news-creator/app** – FastAPI Ollama orchestrator featuring Model Bucket Routing, hierarchical Map-Reduce summarization, golden prompts, and fallback strategies for OOM or repetition. [docs/news-creator.md](docs/news-creator.md)
- **tag-generator/app** – Python 3.13 ML pipeline batching tag extraction, running ONNX Runtime (with SentenceTransformer fallback), exposing `/api/v1/tags/batch`, and refreshing the rolling `tag_label_graph`. [docs/tag-generator.md](docs/tag-generator.md)
- **search-indexer/app** – Go Meilisearch indexer that batches 200 documents, ensures schema settings, and exposes a `/v1/search` handler. [docs/search-indexer.md](docs/search-indexer.md)
- **auth-hub** – Kratos-aware IAP that validates sessions, caches identities for five minutes, and emits authoritative `X-Alt-*` headers for downstream services. [docs/auth-hub.md](docs/auth-hub.md)
- **auth-token-manager** – Deno OAuth2 CLI refreshing Inoreader tokens, writing Kubernetes secrets, and monitoring token horizon alerts. [docs/auth-token-manager.md](docs/auth-token-manager.md)
- **rask-log-forwarder/app** – Rust SIMD JSON log forwarder with lock-free buffers, disk fallback, and ClickHouse-friendly batching. [docs/rask-log-forwarder.md](docs/rask-log-forwarder.md)
- **rask-log-aggregator/app** – Rust Axum API ingesting newline-delimited JSON logs and persisting them into ClickHouse via exporter traits. [docs/rask-log-aggregator.md](docs/rask-log-aggregator.md)
- **recap-worker** – Rust 2024 Axum orchestrator that fetches articles, preprocesses/deduplicates, assigns genres, assembles evidence, clusters via recap-subworker, summarizes via news-creator, and persists outputs for `/v1/recap/7days`. [docs/recap-worker.md](docs/recap-worker.md)
- **recap-subworker** – FastAPI/Gunicorn clustering and classification worker using process pools, embeddings, and diagnostics; supports admin warmups and graph builds. [docs/recap-subworker.md](docs/recap-subworker.md)
- **recap-db** – PostgreSQL schema contract for recap jobs, sections, evidence, tag graphs, and learning results plus migration helpers. [docs/recap-db.md](docs/recap-db.md)
- **rag-orchestrator** – Go 1.25 RAG service implementing article indexing with chunking, vector embedding via knowledge-embedder, context retrieval via pgvector similarity search, and LLM answer generation with citations via knowledge-augur. [docs/rag-orchestrator.md](docs/rag-orchestrator.md)
- **rag-db** – PostgreSQL 18 with pgvector extension storing documents, document versions, chunks, chunk events, and jobs for the RAG pipeline. [docs/rag-db.md](docs/rag-db.md)
- **knowledge-augur** – Ollama-based LLM service for generating grounded answers with structured JSON output and citation tracking.
- **knowledge-embedder** – Embedding service generating vector representations for article chunks using the configured embedding model.

- **Cross-cutting note** – Structured logging, context propagation, deterministic tests, and environment-driven configuration apply to every service. Refresh the relevant `docs/<service>.md` snapshot whenever contracts, env guards, or test commands change, and read the `CLAUDE.md` file for process guardrails before committing.

## Recap Experience & Pipeline

`recap-worker` (Rust 2024) orchestrates the 7-day recap job described in `docs/recap-worker.md` via `PipelineOrchestrator` (`recap-worker/recap-worker/src/pipeline.rs`). A scheduler or manual `POST /v1/generate/recaps/7days` trigger creates a `JobContext` that drives seven stages:

1.  **Fetch** (`AltBackendFetchStage`) pulls the current 7‑day window from `alt-backend` `/v1/recap/articles`, optionally enriches articles with `tag-generator` tags, and writes raw HTML/metadata copies to `recap_job_articles`.
2.  **Preprocess** (`TextPreprocessStage`) strips HTML, normalises Unicode, runs language detection, tokenises via Lindera, and extracts tag signals with `recap_job_articles` data before handing the batch to the deduper.
3.  **Dedup** (`HashDedupStage`) removes near-duplicates with XXH3 + sentence similarity so each article appears once.
4.  **Genre** (`RemoteGenreStage` + `TwoStageGenreStage`) sends batches to `recap-subworker` for remote coarse classification, then, when `genre_refine_enabled` is configured, reloads the cached `tag_label_graph` (preloaded from `recap-db`) and refines genres using `DefaultRefineEngine` + rollout settings.
5.  **Select** (`SummarySelectStage`) trims per-genre documents (min counts, subgenre limits), optionally reruns coherence filtering with the embedding service, and builds the evidence bundle.
6.  **Dispatch** (`MlLlmDispatchStage`) fans out the bundle: `recap-subworker` clusters evidence via `/v1/runs`, `news-creator` summarises per-cluster content through Ollama, and the orchestrator collects the summaries plus diagnostics.
7.  **Persist** (`FinalSectionPersistStage`) stores the curated recap (`recap_outputs`, `recap_genres`), `recap_cluster_evidence`, `recap_genre_learning_results`, and related diagnostics in `recap-db`, updating metrics and structured logs consumed by ClickHouse/rust observability services.

Before each run the builder optionally refreshes the graph cache (`recap_pre_refresh_graph_enabled`) and reloads `GraphOverrideSettings` so genre thresholds can be tuned live; resumable jobs look up the last completed stage when the orchestrator replays the handler. The same workflow emits the `/v1/recap/7days` DTO, `recap_cluster_evidence` links, and `recap_genre_learning_results` telemetry that drive the `/mobile/recap/7days` surface plus downstream dashboards.

### Mobile & API Surfaces

- `/mobile/recap/7days` renders the Recap, Genres, Articles, and Jobs tabs defined in `PLAN6.md`, using SWR hooks (`useRecapData`) plus Chakra skeletons so data appears instantly via `stale-while-revalidate`.
- `GET /v1/recap/7days` (alt-backend) is public and streams the latest recap summary, genre clusters, and deduplicated evidence links (`recap_cluster_evidence`) via the `RecapSummary` → `RecapGenre` → `EvidenceLink` DTOs so clients always get the final, proofed payload.
- `GET /v1/recap/articles` is service-authenticated (service token + `middleware_custom.ServiceAuth`) and supplies recap-worker with deterministic article corpora (window, pagination, language filters) before every run. Pair it with `POST /v1/generate/recaps/7days` for manual retries or narrow-genre jobs.
- `recap-worker` exposes `/health/live`, `/health/ready`, `/metrics`, and admin retries; `recap-subworker` mirrors the health endpoints and publishes queue depth gauges so Grafana, ClickHouse, or CLI checks can flag stalls early.

### Running the Recap Stack

- Run `make recap-migrate` (or `docker compose --profile recap run recap-db-migrator atlas migrate apply`) before enabling the profile so recap-db includes the latest schema (`recap_jobs`, `recap_outputs`, `recap_cluster_evidence`, diagnostics tables).
- Start everything with GPU + worker profiles, e.g.:
  ```bash
  docker compose --profile recap --profile ollama up recap-worker recap-subworker recap-db news-creator -d
  ```
- Trigger a job manually via `curl -X POST http://localhost:9005/v1/generate/recaps/7days -H 'Content-Type: application/json' -d '{"genres":[]}'` or wait for the default 04:00 JST scheduler. Watch `docker compose logs -f recap-worker recap-subworker` for per-stage metrics.
- Once the job completes, `curl http://localhost:9000/v1/recap/7days` should return fresh data and the mobile route will refresh automatically thanks to SWR’s `stale-if-error` fallback.
- The genre refinement stage depends on the `TAG_LABEL_GRAPH_WINDOW`/`TAG_LABEL_GRAPH_TTL_SECONDS` cache window, so update those envs and re-run `scripts/build_label_graph.py` or the `scripts/replay_genre_pipeline.rs` helper (pass `--dataset`, optional `--graph-window`, `--graph-ttl`, `--require-tags`, `--dry-run`) whenever you change the tag priors; the recap-worker also ships golden dataset evaluation + ROUGE scoring utilities to guard summary quality alongside these offline replays.

### Pipeline Flow

```mermaid
flowchart LR
    classDef svc fill:#eef2ff,stroke:#4338ca,color:#111827
    classDef data fill:#ecfccb,stroke:#16a34a,color:#052e16
    classDef ui fill:#cffafe,stroke:#0891b2,color:#083344
    classDef ctrl fill:#fde68a,stroke:#d97706,color:#713f12

    Scheduler[[04:00 JST Cron]]:::ctrl
    Admin[Manual trigger<br/>POST :9005/v1/generate/recaps/7days]:::ctrl
    Worker{{recap-worker :9005<br/>Rust pipeline}}:::svc
    AltAPI["alt-backend :9000<br/>GET /v1/recap/articles"]:::svc
    TagGen["tag-generator<br/>/api/v1/tags/batch"]:::svc
    Subworker["recap-subworker :8002<br/>/v1/runs clustering"]:::svc
    News["news-creator :8001<br/>/v1/summary/generate"]:::svc
    RecapDB[(recap-db<br/>PostgreSQL 18)]:::data
    BackendAPI["alt-backend :9000<br/>GET /v1/recap/7days"]:::svc
    Mobile["Mobile UI<br/>/mobile/recap/7days"]:::ui

    Scheduler --> Worker
    Admin --> Worker
    Worker -->|X-Service-Token| AltAPI
    AltAPI --> Worker
    Worker -->|/api/v1/tags/batch| TagGen
    Worker -->|/v1/runs| Subworker
    Subworker --> Worker
    Worker -->|/v1/summary/generate| News
    News --> Worker
    Worker -->|SQL| RecapDB
    RecapDB --> BackendAPI
    BackendAPI --> Mobile
```

## Observability & Operations

- Enable the `logging` profile to run rask-log-forwarder sidecars; defaults stream 1 000-log batches (flush 500 ms) to `http://rask-log-aggregator:9600/v1/aggregate`. When testing Recap, pair the `recap` and `ollama` profiles so recap-worker can reach both recap-db and news-creator. ClickHouse data lives in `clickhouse_data` and is accessible via `docker compose exec clickhouse clickhouse-client`.
- Monitor recap-specific metrics (`recap_genre_refine_rollout_enabled_total`, `_skipped_total`, `recap_genre_refine_graph_hits_total`, `recap_api_evidence_duplicates_total`, `recap_api_latest_fetch_duration_seconds`, etc.) alongside the `recap` profile logs, and use the golden dataset evaluation tasks (`recap-worker/tests/golden_eval.rs`, ROUGE helpers under `recap-worker/src/evaluation/golden.rs`, and `scripts/replay_genre_pipeline.rs`) whenever you change summarisation, clustering, or graph priors.
- Monitor core endpoints below; Kratos (`http://localhost:4433/health/ready`) and ClickHouse (`http://localhost:8123/ping`) should also respond during smoke tests.

  | Service | Endpoint | Expectation |
  | --- | --- | --- |
  | Frontend | `http://localhost:3000/api/health` | `{ "status": "ok" }` |
  | Backend | `http://localhost:9000/v1/health` | `{ "status": "ok" }` |
  | Auth Hub | `http://localhost:8888/health` | HTTP 200 |
  | Meilisearch | `http://localhost:7700/health` | `{ "status": "available" }` |
  | Recap Worker | `http://localhost:9005/health/ready` | Probes recap-subworker + news-creator before `200`; metrics at `/metrics`. |
  | Recap Subworker | `http://localhost:8002/health/ready` | `200` only when queue + process pool are healthy. |
  | Recap DB | `docker compose exec recap-db pg_isready -U $RECAP_DB_USER` | `accepting connections` |
  | RAG Orchestrator | `http://localhost:9010/health` | HTTP 200 when rag-db + Ollama reachable. |
  | alt-frontend-sv | `http://localhost:4173/sv` | SvelteKit UI at `/sv` base path. |
  | Connect-RPC | `http://localhost:9101` | gRPC-Web/Connect protocol endpoint. |

- Use `docker compose logs -f <service>` for quick debugging, query ClickHouse for high-volume analysis, and run `backup-postgres.sh` / `backup-postgres-docker.sh` only when the stack is quiesced.

## Development Workflow & Testing

- Follow Red → Green → Refactor, starting with business-layer tests and regenerating mocks when interfaces evolve.
- Keep changes surgical and deterministic—lean on dependency injection, fake clocks (`testing/synctest`, custom `Clock`), and table/parameterized tests.
- Run formatters before committing (`pnpm fmt`, `gofmt`, `uv run ruff format`, `cargo fmt`, `deno fmt`) and document any new env vars or migrations.

### Test Matrix

| Area | Scope | Command | Notes |
| --- | --- | --- | --- |
| Frontend unit | alt-frontend components | `pnpm -C alt-frontend test` | Vitest + Testing Library + `userEvent`. |
| Frontend e2e | alt-frontend Playwright POM | `pnpm -C alt-frontend test:e2e` | Requires `make up`. |
| Go services | alt-backend/app, sidecar-proxy, pre-processor/app, pre-processor-sidecar/app, search-indexer/app, auth-hub | `go test ./...` from each directory | Add `-race -cover` when touching concurrency; regenerate mocks via `make generate-mocks`. |
| Python services | news-creator/app, tag-generator/app, recap-subworker | `SERVICE_SECRET=test-secret pytest`; `uv run pytest` | Async tests use `pytest-asyncio`; Ruff and mypy enforce quality gates; recap-subworker exercises clustering + queue backpressure. |
| Rust services | recap-worker, rask-log-forwarder/app, rask-log-aggregator/app | `cargo test -p <crate>` | Use `cargo bench` for recap-worker preprocessing + rask hot paths; run `SQLX_OFFLINE=true cargo test -p recap-worker` in CI. |
| Deno service | auth-token-manager | `deno test` | BDD-style assertions with `@std/testing/bdd`. |
| Recap pipeline | recap-worker ↔ recap-subworker contract + alt-backend `/v1/recap/*` DTOs | `cargo test -p recap-worker && uv run pytest -q` | Run before modifying JSON Schemas or evidence DTOs; follow with `docker compose --profile recap --profile ollama up` smoke to validate `/v1/recap/7days`. |
| Compose smoke | Full stack health | `make up` then `curl` health endpoints | Confirms migrations, Meilisearch settings, and auth-hub session flow. |

## Testing Playbook

Alt’s quality bar depends on disciplined, layered tests:

- **Unit** – Pure functions, usecases, and adapters using table-driven Go tests, pytest fixtures, Vitest `describe.each`, or Rust unit modules.
- **Integration** – Boundary checks (Go ↔ Postgres, FastAPI ↔ Ollama mock, Rust ↔ ClickHouse) run via Compose services or lightweight doubles.
- **End-to-end** – Playwright journeys ensure auth headers, summarisation flows, and search UX remain intact; rely on Page Object Models.
- **Golden/Performance** – Guard LLM prompts and hot paths with golden datasets, ROUGE scoring helpers, offline replays (`scripts/replay_genre_pipeline.rs`), and `criterion`/`testing.B` benchmarks.

Authoring guidelines: name tests descriptively, isolate dependencies (GoMock, `pytest-mock`, `mockall`, `@std/testing/mock`), control time via fake clocks, and keep suites fast to avoid flaky CI.

CI expectations: PRs run lint + unit suites per language plus targeted integration/E2E jobs when code touches those areas. Record commands and outcomes in PR descriptions. If tests fail, prefer fixing root causes over blanket retries; update snapshots only when behaviour changes intentionally.

## Data & Storage

- PostgreSQL 17 (`db_data_17`) stores canonical entities: `feeds`, `articles`, `article_summaries`, `article_tags`, `ingestion_jobs`, `users`, and audit tables. Atlas migrations live in `migrations-atlas/` and must remain backward-compatible because `make up` replays them on every boot.
- Kratos maintains its own database (`kratos_db_data`) for identity state; never cross-link application tables to Kratos schemas—consume identity via auth-hub headers instead.
- Meilisearch (`meili_data`) holds denormalised search documents built by `search-indexer`; run `docker compose exec meilisearch index list` to inspect configured indices.
- ClickHouse (`clickhouse_data`) captures structured logs from rask-aggregator, enabling time-series queries, dashboards, and anomaly alerts.
- recap-db (PostgreSQL 18, `recap_db_data`) is the dedicated instance for recap-worker; it stores `recap_jobs`, cached articles, `recap_cluster_evidence`, `tag_label_graph`, `recap_genre_learning_results`, and published summaries. Keep it in sync via `recap-migration-atlas/` + `make recap-migrate` before running the `recap` profile.
- rag-db (PostgreSQL 18 + pgvector, `rag_db_data`) stores RAG documents, versions, chunks, chunk events, and jobs. The pgvector extension enables similarity search over embeddings. Keep it in sync via `rag-migration-atlas/` before running the `rag-extension` profile.
- Backups: `backup-postgres.sh` (local Docker) and `backup-postgres-docker.sh` (Compose-aware) provide snapshot scripts; schedule them before major migrations. ClickHouse backups can be scripted via `clickhouse-client` or S3-based storage (future).

### Data Model Overview

```mermaid
erDiagram
    FEEDS ||--o{ ARTICLES : contains
    ARTICLES ||--o{ ARTICLE_SUMMARIES : summarised_by
    ARTICLES ||--o{ ARTICLE_TAGS : tagged_with
    ARTICLES }o--o{ INGESTION_JOBS : processed_in
    USERS ||--o{ ARTICLES : archived_by
    ARTICLES ||--o{ RECAP_JOB_ARTICLES : cached_for
    RECAP_JOB_ARTICLES }o--|| RECAP_JOBS : belongs_to
    RECAP_JOBS ||--o{ RECAP_OUTPUTS : produces
    RECAP_OUTPUTS ||--o{ RECAP_CLUSTER_EVIDENCE : references

    ARTICLES ||--o{ RAG_DOCUMENTS : indexed_as
    RAG_DOCUMENTS ||--o{ RAG_DOCUMENT_VERSIONS : versioned_by
    RAG_DOCUMENT_VERSIONS ||--o{ RAG_CHUNKS : split_into
    RAG_CHUNKS ||--o{ RAG_CHUNK_EVENTS : tracked_by
    RAG_JOBS ||--o{ RAG_DOCUMENTS : processes
```

### Storage Guardrails

- **Retention** – Articles stay until explicitly archived; summaries and tags follow cascading rules defined in migrations—avoid manual deletes.
- **Indices** – Postgres indexes `(feed_id, archived)` and `(published_at DESC)` keep queries snappy; adjust Meilisearch filterable attributes when adding new facets.
- **Migrations** – Preview drift with Atlas when available; keep changes idempotent and reversible.
- **Resets** – `make down-volumes` clears state; note any seed scripts so teammates can repopulate fixtures quickly.
- **Recap state** – Use `make recap-migrate` + `docker compose --profile recap run recap-worker sqlx migrate info` after editing `recap-migration-atlas/`; keep JSONB payloads backwards compatible so the public `/v1/recap/7days` API never breaks.

## Security & Compliance

- Never commit real credentials; keep developer defaults in `.env.template` and load real secrets via `.env` or Kubernetes Secrets.
- auth-hub is the single source of identity—consume `X-Alt-*` headers and reject conflicting user context.
- Sanitize logs and use the TLS helpers (`make dev-ssl-setup`, `make dev-ssl-test`, `make dev-clean-ssl`) to keep traffic encrypted while redacting sensitive fields.
- Service-to-service calls (e.g., `/v1/recap/articles`, `/api/v1/tags/batch`, and tag label graph refreshes) now rely on `SERVICE_SECRET` + `X-Service-Token` headers; keep the secret in `.env`/Secrets, rotate it consistently, and only share it with Compose services that need to authenticate.
- Validate inputs, prefer parameterized queries, and wrap errors with context without leaking private details.

## External Integrations

- **Inoreader OAuth2** – Managed by `auth-token-manager` and `pre-processor-sidecar`; tokens refresh proactively and live in Secrets.
- **Ollama (Gemma 3 4B)** – Powers LLM summaries; install GPU drivers before enabling the `ollama` profile.
- **RSS & downstream connectors** – pre-processor respects publisher rate limits today; planned notification channels must preserve the same guardrails.

## Contribution Checklist

- Read the root and service-specific `CLAUDE.md` files before making changes.
- Start every change with a failing test and keep the affected suites green.
- Run formatters/linters and document new configuration, migrations, or APIs.
- Prove the change with the smallest meaningful test or health probe and note the result.
- Leave `stopped-using-k8s/` untouched unless asked and verify `make up` succeeds after edits.

## Roadmap & Historical Context

- Upcoming initiatives: extend auth-hub with tenant scoping, add semantic embeddings to Meilisearch, deliver live article status (SSE/WebSocket), and harden ClickHouse dashboards.
- Historical posture: Kubernetes assets in `stopped-using-k8s/` and the legacy `Alt-arch.mmd` diagram are reference-only—Compose remains the authoritative workflow.

## Change Management & Communication

- **Planning** – Open a GitHub issue or ADR for significant architectural work. Reference impacted services and note required Compose profile changes or migrations.
- **Documentation** – Update this README, relevant `CLAUDE.md`, and any runbooks when behaviour changes. Diagram diffs (Mermaid, Alt-arch.mmd) should be kept in sync.
- **Code reviews** – Default to reviewer pairs who own affected services; call out risky areas, test coverage, and rollback strategy. Highlight any rate limit, security, or compliance implications.
- **Release cadence** – Weekly Compose releases roll forward once smoke tests pass. Emergency fixes require tagged releases with changelog entries and communication in #alt-platform.
- **Communication channels** – Use #alt-dev for day-to-day collaboration, #alt-ops for incident coordination, and the platform RFC Notion space for long-form proposals.
- **Post-merge validation** – After merging, run `make up`, verify health endpoints, and monitor ClickHouse dashboards for anomalies during the first ingestion cycle.

## Troubleshooting & FAQs

| Symptom | Likely Cause | Resolution |
| --- | --- | --- |
| `pnpm dev` fails with missing env vars | `.env` not aligned with `.env.template` | Re-run `cp .env.template .env`, ensure `scripts/check-env.js` passes. |
| Backend returns 401 despite valid session | auth-hub cache stale or Kratos offline | Restart auth-hub container; verify Kratos `/sessions/whoami` responds; purge cache by restarting service. |
| Recap dashboard shows skeletons forever | `--profile recap` not running, recap-worker job failed, or recap-db lacks data | Run `docker compose --profile recap --profile ollama up recap-worker recap-subworker recap-db`; trigger `POST /v1/generate/recaps/7days`; inspect `docker compose logs recap-worker`. |
| Recap evidence links keep returning duplicates or empty lists | `recap_cluster_evidence`/`tag_label_graph` migrations missing, label graph cache expired (`TAG_LABEL_GRAPH_TTL_SECONDS`), or tag-generator hasn't refreshed `tag_label_graph` | Run `make recap-migrate`, confirm `recap_cluster_evidence` & `tag_label_graph` exist, refresh the graph with `tag-generator/app/scripts/build_label_graph.py` (or background thread), and rerun `curl POST /v1/generate/recaps/7days`. |
| Meilisearch searches empty after ingest | search-indexer not running or index misconfigured | Check `docker compose logs search-indexer`; rerun `search-indexer` manually; confirm index settings via Meili dashboard. |
| Ollama summary timeouts | Model not pulled or GPU unavailable | Run `docker compose --profile ollama logs news-creator`; preload model with `ollama pull gemma:4b`; confirm GPU drivers. |
| Rust services crash on startup | Insufficient ulimit or missing env | Ensure `LOG_LEVEL` and `RASK_ENDPOINT` set; increase file descriptors via Docker Compose `ulimits`. |
| Go tests flaky with timeouts | Missing fake clock or context deadline | Inject `testing/synctest` clock, set explicit deadlines, and avoid sleeping blindly in tests. |
| Tag-generator batch fetch returns 401 | `SERVICE_SECRET` missing/mismatched or `X-Service-Token` header not sent | Align `.env` values for `SERVICE_SECRET`, include the same value when calling `/api/v1/tags/batch`, and confirm clients supply `X-Service-Token`. |
| Playwright tests hang | Stack not running or selectors outdated | Start stack with `make up`; update POM selectors to match `data-testid` or page changes. |
| RAG returns empty context | rag-db has no indexed articles or pgvector not enabled | Run indexing job via rag-orchestrator; check `docker compose logs rag-orchestrator`; verify pgvector extension with `docker compose exec rag-db psql -c "SELECT * FROM pg_extension"`. |
| Connect-RPC calls fail | Port 9101 not exposed or Connect-RPC server not running | Verify `compose.yaml` exposes 9101; check `docker compose ps alt-backend`; confirm Connect-RPC server started in logs. |
| alt-frontend-sv returns 404 | Wrong base path or SvelteKit not built | Requests must target `/sv` path; run `pnpm -C alt-frontend-sv build`; check svelte.config.js `kit.paths.base`. |
| pgvector extension missing | rag-db migration not applied | Run `docker compose --profile rag-extension up rag-db-migrator`; verify with `docker compose exec rag-db psql -c "CREATE EXTENSION IF NOT EXISTS vector"`. |

**General tip:** Use `docker compose ps` and `docker compose logs -f <service>` for health checks, `docker compose exec db psql -U $POSTGRES_USER $POSTGRES_DB` for database inspection, and `make down-volumes` to reset state (only when data loss is acceptable).

## Glossary

- **Alt** – The Compose-first AI knowledge platform described in this repository.
- **Clean Architecture** – Layered approach separating interface (handlers), business logic (usecases), and infrastructure (gateways/drivers).
- **Compose profile** – Named group of services (e.g., `ollama`, `logging`) that can be toggled on via `docker compose --profile`.
- **Golden dataset** – Curated set of inputs/outputs used to detect regressions in LLM or ML-driven features.
- **IAP (Identity-Aware Proxy)** – Pattern where an edge service (auth-hub) centralises authentication before requests reach backend services.
- **LLM** – Large Language Model; in this project, Ollama-powered Gemma 3 4B produces article summaries.
- **Meilisearch** – Lightweight search engine used for full-text indexing and filtering of enriched content.
- **Rask** – Codename for the Rust observability duo: `rask-log-forwarder` (ingest) and `rask-log-aggregator` (ClickHouse sink).
- **Recap** – The seven-day batch summarization feature driven by recap-worker (Rust), recap-subworker (FastAPI), recap-db (PostgreSQL), and the mobile `/mobile/recap/7days` UI exposed via `/v1/recap/7days`.
- **Singleflight** – Go concurrency primitive ensuring only one duplicate request executes; used for token refresh.
- **TDD** – Test-Driven Development; the Red → Green → Refactor cycle enforced across all services.
- **Connect-RPC** – Type-safe RPC framework using Protocol Buffers; HTTP/1.1 and HTTP/2 compatible with gRPC interoperability; runs on port 9101.
- **RAG** – Retrieval Augmented Generation; combines vector similarity search with LLM generation for grounded, citation-backed answers.
- **SvelteKit** – Modern web framework for Svelte with SSR/SSG support and file-based routing; powers alt-frontend-sv at `/sv`.
- **Runes** – Svelte 5's reactive primitives (`$state`, `$derived`, `$effect`) replacing legacy reactivity; used in alt-frontend-sv.
- **pgvector** – PostgreSQL extension for vector similarity search; enables RAG context retrieval in rag-db.

## Reference Resources

- Internal docs: `CLAUDE.md` (root) and service-specific `CLAUDE.md` files.
- Architecture diagrams: `README.md` Mermaid blocks and `Alt-arch.mmd` for historical Kubernetes topology.
- Runbooks: `docs/` folder (if present) and scripts under `scripts/` for auth checks, log collection, and index resets.
- External references: [Next.js](https://nextjs.org/docs), [Go 1.25](https://go.dev/doc/devel/release), [Meilisearch](https://www.meilisearch.com/docs), [ClickHouse](https://clickhouse.com/docs), [Ollama](https://ollama.com/), [Kratos](https://www.ory.sh/kratos/docs/).
- Communication hubs: #alt-dev, #alt-ops Slack channels, and the Alt Notion workspace for RFCs and runbooks.

## Appendix

### Command Cheat Sheet

```bash
# Core stack management
make up                                              # Build and start full stack
make down                                            # Stop stack (keep volumes)
make down-volumes                                    # Stop and reset all data

# Profile activation
docker compose --profile ollama --profile logging up -d    # AI + observability
docker compose --profile recap --profile ollama up -d      # Recap pipeline
docker compose --profile rag-extension up -d               # RAG services

# Testing
pnpm -C alt-frontend test                            # Next.js unit tests
pnpm -C alt-frontend-sv check                        # SvelteKit type check
cd alt-backend/app && go test ./...                  # Go backend tests

# Migrations
make recap-migrate                                   # Apply recap-db migrations
docker compose --profile rag-extension up rag-db-migrator  # Apply RAG migrations

# Connect-RPC code generation
make buf-generate                                    # Generate Go + TypeScript from proto

# Development servers
pnpm -C alt-frontend-sv dev                          # SvelteKit dev server on :5173

# Health checks
curl http://localhost:9000/v1/health                 # Backend REST
curl http://localhost:9101                           # Connect-RPC
curl http://localhost:9010/health                    # RAG orchestrator
curl http://localhost:4173/sv                        # SvelteKit frontend
curl http://localhost:9000/v1/recap/7days            # Recap API
```

### Essential Environment Variables

| Variable | Purpose | Default/Location |
| --- | --- | --- |
| `POSTGRES_USER`, `POSTGRES_PASSWORD`, `POSTGRES_DB` | Alt backend database credentials | `.env.template` |
| `KRATOS_INTERNAL_URL`, `KRATOS_PUBLIC_URL` | Ory Kratos internal/public endpoints | `.env.template` |
| `AUTH_HUB_INTERNAL_URL` | Internal URL for auth-hub | `.env.template` |
| `NEXT_PUBLIC_APP_ORIGIN`, `NEXT_PUBLIC_RETURN_TO_DEFAULT` | Frontend routing and redirects | `.env.template` |
| `SIDECAR_PROXY_BASE_URL`, `RASK_ENDPOINT` (+ batching vars) | Outbound proxy and log pipeline config | `.env` or compose `x-rask-env` |
| `SERVICE_SECRET`, `INOREADER_CLIENT_ID`, `INOREADER_CLIENT_SECRET` | News-creator tests and Inoreader OAuth tokens | Export locally or inject via Secrets |
| `RECAP_DB_USER`, `RECAP_DB_PASSWORD`, `RECAP_DB_NAME`, `RECAP_DB_PORT` | Recap PostgreSQL credentials shared by recap-worker, recap-subworker, and the migrator | `.env.template`, `recap-migration-atlas/.env.example` |
| `RECAP_WORKER_URL`, `RECAP_SUBWORKER_BASE_URL`, `RECAP_WINDOW_DAYS` | alt-backend → recap-worker client target plus worker scheduling window | `.env.template` and Compose `recap` profile |
| `BACKEND_CONNECT_URL` | Connect-RPC endpoint URL for SvelteKit frontend | `.env.template` |
| `PUBLIC_USE_CONNECT_STREAMING` | Feature flag to enable Connect-RPC streaming in alt-frontend-sv | `.env.template` |
| `OLLAMA_BASE_URL` | Ollama API URL for RAG and news-creator services | `http://localhost:11434` |
| `EMBEDDING_MODEL` | Model name for generating embeddings in rag-orchestrator | `embeddinggemma` |
| `GENERATION_MODEL` | LLM model for RAG answer generation | `gpt-oss:20b` |
| `RAG_MAX_CHUNKS` | Maximum context chunks to retrieve for RAG queries | `10` |

Keep `.env.template` updated with non-sensitive defaults whenever configuration changes, and mirror new variables here.

## Open-Source Credits

Alt stands on the shoulders of many open-source projects. We gratefully acknowledge the communities that maintain the tools and frameworks powering this platform.

- **Docker & Docker Compose** – Container orchestration backbone for local and CI workflows. © Docker Inc. Licensed under Apache 2.0/MIT.
- **Node.js & pnpm** – JavaScript runtime and package manager enabling the Next.js frontend. Node.js is available under the MIT License; pnpm under MIT.
- **Next.js & React** – Frontend framework and UI library (MIT) by Vercel and Meta. Chakra UI (MIT) provides the design system.
- **Go** – Systems language (BSD-style license) powering backend, ingestion, proxy, and auth services. Includes Echo (MIT), GoMock (Apache 2.0), and other Go ecosystem libraries.
- **Python & FastAPI** – Python (PSF License) with FastAPI (MIT) drives LLM summarisation and tagging services, alongside `uv` (MIT), `pytest` (MIT), and the wider scientific stack (varied permissive licenses).
- **Rust** – Rust compiler/toolchain (Apache 2.0 / MIT dual license) underpins observability sidecars, supplemented by crates like Axum (MIT/Apache 2.0), Tokio (MIT/Apache 2.0), and Criterion (Apache 2.0).
- **Deno** – Secure TypeScript/JavaScript runtime (MIT) powering the auth-token-manager.
- **Ollama** – Open-source LLM runtime (MIT) providing Gemma 3 4B hosting for news-creator.
- **Meilisearch** – Search engine (MIT) delivering full-text indexing. Integrated via official Go client (MIT).
- **PostgreSQL & pgx** – PostgreSQL database (PostgreSQL License) and associated drivers for data persistence.
- **ClickHouse** – Columnar database (Apache 2.0) backing observability analytics.
- **Ory Kratos** – Identity infrastructure (Apache 2.0) enabling session validation via auth-hub.
- **Mercari/go-circuitbreaker, singleflight, slog, Atlas** – Key Go libraries (varied BSD/MIT/Apache licenses) supporting resilience, logging, and migrations.
- **Testing frameworks** – Vitest, Playwright, pytest, Go `testing`, Cargo test, and Deno test suites (MIT/BSD/Apache) enforcing the TDD workflow.
- **Linux base images** – Alpine Linux (MIT), Debian/Ubuntu (varied open-source licenses) form the runtime foundation for service containers.

Each dependency retains its respective license; review individual repositories for full terms. We remain committed to upstream contributions and timely upgrades to honour these communities.
