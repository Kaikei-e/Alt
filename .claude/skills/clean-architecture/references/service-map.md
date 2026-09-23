# Service Map: Clean Architecture Layers in Alt

## Table of Contents
- [1. Naming Variants → Alt Layer Lookup Table](#1-naming-variants--alt-layer-lookup-table)
- [2. Per-Service Layer Directory & Trace Map](#2-per-service-layer-directory--trace-map)
  - [1. alt-backend (Go)](#1-alt-backend-go)
  - [2. search-indexer (Go)](#2-search-indexer-go)
  - [3. news-creator (Python / FastAPI)](#3-news-creator-python--fastapi)
  - [4. pre-processor (Go)](#4-pre-processor-go)
  - [5. recap-worker (Rust)](#5-recap-worker-rust)
  - [6. recap-subworker (Python)](#6-recap-subworker-python)
  - [7. rag-orchestrator (Go)](#7-rag-orchestrator-go)
  - [8. acolyte-orchestrator (Python)](#8-acolyte-orchestrator-python)
  - [9. alt-butterfly-facade (Go - BFF Proxy Variant)](#9-alt-butterfly-facade-go---bff-proxy-variant)
  - [10. knowledge-sovereign (Go - Documented Reduced Shape)](#10-knowledge-sovereign-go---documented-reduced-shape)

---

## 1. Naming Variants → Alt Layer Lookup Table

Different languages and frameworks across Alt use localized directory naming conventions. This table maps them to the canonical 5-layer model:

| Alt Layer | Service Directory Names / Conventions | Roles & Types Contained |
|---|---|---|
| **REST** | `rest/`, `handler/`, `api/`, `routers/`, `internal/adapter/connect/` | HTTP Echo/FastAPI handlers, Connect-RPC services, Redis stream event consumers. Parses DTOs, validates syntax, calls Usecases. |
| **Usecase** | `usecase/`, `service/`, `pipeline/` | Pure business workflow orchestration. Free of I/O, SQL, or transport protocols. |
| **Port** | `port/`, repository interfaces (`repository/interfaces.go`), stage traits (`pipeline/dispatch.rs`) | Consumer-owned interface contracts specifying what external capabilities or pipeline seams the Usecase needs. Concrete client calls in services without ports. |
| **Gateway** | `gateway/`, `internal/adapter/repository/`, client adapters | Anti-Corruption Layer (ACL). Implements Ports, translates Domain models ↔ Driver rows/payloads. |
| **Driver** | `driver/`, `infra/`, `store/`, `clients/` | Raw I/O implementations (`pgx`, `sqlx`, Meilisearch, HTTP client, Redis, mTLS). Owns DB rows and vendor DTOs. |
| **Domain** | `domain/`, `schema/` (in Rust/Python) | Core business entities, value objects, domain invariants, sentinel errors. Zero framework or I/O imports. |

---

## 2. Per-Service Layer Directory & Trace Map

### 1. `alt-backend` (Go)
- **Architecture context**: Package families partitioned into `orchestrator/`, `dataplane/`, and `shared/` per ADR-000945. `alt-data-hub` owns `alt_db` per ADR-000954, so `alt-backend` orchestrates via the `datahub_client` driver.
- **Layer directories**:
  - REST (`orchestrator/rest/`): HTTP handlers and Connect-RPC endpoints
  - Usecase (`orchestrator/usecase/`): Business orchestration
  - Port (`orchestrator/port/`): Capability interfaces
  - Gateway (`orchestrator/gateway/`): Domain/driver adapters
  - Driver (`shared/driver/`): Connect-RPC clients and raw utilities
  - Domain (`domain/`): Core vocabulary
- **Composition Root**: `alt-backend/app/cmd/backend/main.go`, `alt-backend/app/cmd/harvester/main.go`, `alt-backend/app/cmd/notifier/main.go`, `alt-backend/app/cmd/datahub/main.go`, `alt-backend/app/di/container.go`
- **Representative Trace**: Article Archive Request
  1. REST (`orchestrator/rest/`): `alt-backend/app/orchestrator/rest/article_handlers.go` (`handleArchiveArticle`)
  2. Usecase (`orchestrator/usecase/`): `alt-backend/app/orchestrator/usecase/archive_article_usecase/usecase.go` (`ArchiveArticleUsecase.Execute`)
  3. Port (`orchestrator/port/`): `alt-backend/app/orchestrator/port/archive_article_port/archive_port.go` (`ArchiveArticlePort.SaveArticle`)
  4. Gateway (`orchestrator/gateway/`): `alt-backend/app/orchestrator/gateway/archive_article_gateway/gateway.go` (`ArchiveArticleGateway.SaveArticle`, calling `g.repo.SaveArticle` on `ArticleSaver` interface)
  5. Gateway (Shared ACL): `alt-backend/app/shared/gateway/datahub_gateway/article.go` (`ArticleStoreGateway.SaveArticle`, wired in `alt-backend/app/di/article_module.go` as `ArticleSaver`, calling `g.client.ArchiveArticle`)
  6. Driver (`shared/driver/`): `alt-backend/app/shared/driver/datahub_client/client.go` (`DataHubServiceClient.ArchiveArticle`)

---

### 2. `search-indexer` (Go)
- **Layer directories**:
  - REST (`rest/`): HTTP search handler
  - Usecase (`usecase/`): Search coordination and query sanitization
  - Port (`port/`): Search engine abstraction
  - Gateway (`gateway/`): Meilisearch adapter
  - Driver (`driver/`): Meilisearch client wrapper
  - Domain (`domain/`): Search documents and filters
- **Composition Root**: `search-indexer/app/main.go`, `search-indexer/app/bootstrap/app.go`
- **Representative Trace**: Article Search
  1. REST (`rest/`): `search-indexer/app/rest/handler.go` (`Handler.SearchArticles`)
  2. Usecase (`usecase/`): `search-indexer/app/usecase/search_by_user.go` (`SearchByUserUsecase.ExecuteWithDateFilter`, using query validation helper in `usecase/search_articles.go` `ValidateQuery`)
  3. Port (`port/`): `search-indexer/app/port/search_engine.go` (`SearchEngine.SearchByUserIDWithDateFilter`)
  4. Gateway (`gateway/`): `search-indexer/app/gateway/search_engine_gateway.go` (`SearchEngineGateway.SearchByUserIDWithDateFilter`)
  5. Driver (`driver/`): `search-indexer/app/driver/meilisearch_driver.go` (`MeilisearchDriver.SearchByUserIDWithDateFilter`)

---

### 3. `news-creator` (Python / FastAPI)
- **Layer directories**:
  - REST (`news_creator/handler/`): FastAPI routers
  - Usecase (`news_creator/usecase/`): Prompt generation and LLM pipeline
  - Port (`news_creator/port/`): LLM and cache ports
  - Gateway (`news_creator/gateway/`): Ollama gateway and model router
  - Driver (`news_creator/driver/`): Ollama client
  - Domain (`news_creator/domain/`): Prompt templates and guard models
- **Composition Root**: `news-creator/app/main.py`
- **Representative Trace**: Article Summarization
  1. REST (`news_creator/handler/`): `news-creator/app/news_creator/handler/summarize_handler.py` (`summarize_endpoint` via `create_summarize_router`)
  2. Usecase (`news_creator/usecase/`): `news-creator/app/news_creator/usecase/summarize_usecase.py` (`SummarizeUsecase.generate_summary`)
  3. Port (`news_creator/port/`): `news-creator/app/news_creator/port/llm_provider_port.py` (`LLMProviderPort.generate`)
  4. Gateway (`news_creator/gateway/`): `news-creator/app/news_creator/gateway/ollama_gateway.py` (`OllamaGateway.generate`)
  5. Driver (`news_creator/driver/`): `news-creator/app/news_creator/driver/ollama_driver.py` (`OllamaDriver.generate`)

---

### 4. `pre-processor` (Go)
- **Layer directories**:
  - REST (`handler/`): Echo handlers
  - Usecase (`usecase/summarize/`, `service/`): Summarization on-demand and queue workers
  - Port (`repository/interfaces.go`): Repository capability interfaces
  - Gateway (`repository/`): Job repositories and external API gateways
  - Driver (`driver/`): Summarizer API client and database connection
  - Domain (`domain/`): Article and feed entities
- **Composition Root**: `pre-processor/app/main.go`, `pre-processor/app/bootstrap/`
- **Representative Trace**: On-Demand Summarization
  1. REST (`handler/`): `pre-processor/app/handler/summarize_handler.go` (`SummarizeHandler.HandleSummarize`)
  2. Usecase (`usecase/summarize/`): `pre-processor/app/usecase/summarize/on_demand.go` (`OnDemandService.Summarize`, coordinating with `service/article_summarizer.go`)
  3. Port (`repository/interfaces.go`): `pre-processor/app/repository/interfaces.go` (`ExternalAPIRepository.SummarizeArticle`)
  4. Gateway (`repository/`): `pre-processor/app/repository/external_api_repository.go` (`externalAPIRepository.SummarizeArticle`)
  5. Driver (`driver/`): `pre-processor/app/driver/summarizer_api.go` (`ArticleSummarizerAPIClient.SummarizeArticle`) *(Note: `summarizer_api.go` currently imports `pre-processor/domain`; Driver importing Domain is a violation—Driver should accept raw payloads and Gateway should map Domain ↔ Driver types)*

---

### 5. `recap-worker` (Rust)
- **Layer directories**:
  - REST (`src/api/`): Axum handlers (`generate.rs`, `fetch.rs`)
  - Usecase (`src/pipeline/`): Pipeline orchestrators and workflow stages
  - Port: None separate / concrete client wrappers; traits serve as pipeline stage seams (`src/pipeline/dispatch.rs`)
  - Gateway: Client adapters (`src/clients/subworker.rs`)
  - Driver (`src/store/`, `src/clients/mtls.rs`): DB access and mTLS transport
  - Domain (`src/schema.rs`): Recap domain types
- **Composition Root**: `recap-worker/recap-worker/src/main.rs`, `recap-worker/recap-worker/src/startup.rs`
- **Representative Trace**: Recap Pipeline Execution
  1. REST (`src/api/`): `recap-worker/recap-worker/src/api/generate.rs` (`trigger_7days` / `trigger_3days`)
  2. Usecase (`src/pipeline/`): `recap-worker/recap-worker/src/pipeline/orchestrator.rs` (`PipelineOrchestrator.run_pipeline`)
  3. Pipeline Seam (Port-like): `recap-worker/recap-worker/src/pipeline/dispatch.rs` (`DispatchStage` trait)
  4. Gateway (`src/pipeline/dispatch.rs` & `src/clients/`): `recap-worker/recap-worker/src/pipeline/dispatch.rs` (`MlLlmDispatchStage.dispatch` calling `SubworkerClient.cluster` in `recap-worker/recap-worker/src/clients/subworker.rs`; note: `src/clients/*_contract.rs` are Pact CDC tests, not ports)
  5. Driver (`src/clients/mtls.rs`): `recap-worker/recap-worker/src/clients/mtls.rs` (`ReloadingCertResolver` mTLS HTTPS transport)

---

### 6. `recap-subworker` (Python)
- **Layer directories**:
  - REST (`app/routers/`): FastAPI route endpoints
  - Usecase (`usecase/`): Clustering and run management
  - Port (`port/`): Run submitter, clusterer, and embedder interfaces
  - Gateway / Service (`services/`, `gateway/`): Run manager, HDBSCAN, and embedding gateways
  - Driver (`infra/db/`, `db/`): Database sessions and DAO queries
  - Domain (`domain/`): Value objects and cluster models
- **Pipeline vs Usecase Note**: Legacy endpoints like `app/routers/evidence.py` route through `EvidencePipeline` (`services/`), while updated endpoints like `app/routers/runs.py` route through the Clean Architecture `usecase/` layer.
- **Composition Root**: `recap-subworker/recap_subworker/app/main.py`, `recap-subworker/recap_subworker/app/container.py`
- **Representative Trace**: Run Submission Execution
  1. REST (`app/routers/`): `recap-subworker/recap_subworker/app/routers/runs.py` (`submit_run`)
  2. Usecase (`usecase/`): `recap-subworker/recap_subworker/usecase/submit_run.py` (`SubmitRunUsecase.execute`)
  3. Port (`port/`): `recap-subworker/recap_subworker/port/run_submitter.py` (`RunSubmitterPort.create_run`)
  4. Gateway / Service (`services/`): `recap-subworker/recap_subworker/services/run_manager.py` (`RunManager.create_run` implementing `RunSubmitterPort`, calling `dao.create_run`)
  5. Driver (`infra/db/` & `db/`): `recap-subworker/recap_subworker/infra/db/session.py` (`get_session_factory`) and `recap-subworker/recap_subworker/db/dao.py` (`create_run` SQL execution)

---

### 7. `rag-orchestrator` (Go)
- **Layer directories**:
  - REST (`internal/adapter/connect/`): Connect-RPC services
  - Usecase (`internal/usecase/`): Synthesis strategies and retrieval coordination
  - Port: Colocated in `internal/usecase/` and `internal/domain/` (see known deviation below)
  - Gateway (`internal/adapter/repository/`): SQL chunk repository
  - Driver (`internal/infra/`): Postgres connection pool
  - Domain (`internal/domain/`): Conversation models and chunk definitions
- **Known Deviation**: `internal/domain/repository.go` defines `RagChunkRepository` directly in Domain instead of `port/` or `usecase/`.
- **Composition Root**: `rag-orchestrator/cmd/server/main.go`, `rag-orchestrator/internal/di/container.go`
- **Representative Trace**: Augur Stream Synthesis
  1. REST (`internal/adapter/connect/`): `rag-orchestrator/internal/adapter/connect/augur/handler.go` (`Handler.StreamChat`)
  2. Usecase (`internal/usecase/`): `rag-orchestrator/internal/usecase/rag_answer_stream.go` (`answerWithRAGUsecase.Stream`)
  3. Strategy / Usecase (`internal/usecase/`): `RetrievalStrategy.Retrieve` (e.g. `AgenticSynthesisStrategy` in `agentic_synthesis_strategy.go`)
  4. Port (`internal/domain/` & `internal/usecase/`): `rag-orchestrator/internal/domain/repository.go` (`RagChunkRepository`) and `internal/usecase/knowledge_event_emit_port.go` (`KnowledgeEventEmitter`)
  5. Gateway (`internal/adapter/repository/`): `rag-orchestrator/internal/adapter/repository/rag_chunk_repo.go` (`ragChunkRepository.Search`)
  6. Driver (`internal/infra/`): `rag-orchestrator/internal/infra/postgres.go` (`NewPostgresDB` pgx connection pool)

---

### 8. `acolyte-orchestrator` (Python)
- **Layer directories**:
  - REST (`acolyte/handler/`): Connect-RPC service handlers
  - Usecase (`acolyte/usecase/`): Backfill and report workflows
  - Port (`acolyte/port/`): Report repository and LLM ports
  - Gateway (`acolyte/gateway/`): Postgres gateways
  - Driver: Async connection pool (`psycopg_pool`)
  - Domain (`acolyte/domain/`): Report entities
- **Composition Root**: `acolyte-orchestrator/main.py`
- **Representative Trace**: Report Retrieval
  1. REST (`acolyte/handler/`): `acolyte-orchestrator/acolyte/handler/connect_service.py` (`AcolyteConnectService.get_report`)
  2. Usecase (`acolyte/usecase/`): `acolyte-orchestrator/acolyte/usecase/get_report_uc.py` (`GetReportUsecase.execute`)
  3. Port (`acolyte/port/`): `acolyte-orchestrator/acolyte/port/report_repository.py` (`ReportRepositoryPort.get_report`)
  4. Gateway (`acolyte/gateway/`): `acolyte-orchestrator/acolyte/gateway/postgres_report_gw.py` (`PostgresReportGateway.get_report`)
  5. Driver: `psycopg_pool.AsyncConnectionPool` (raw PostgreSQL connection pool)

---

### 9. `alt-butterfly-facade` (Go - BFF Proxy Variant)
- **Role**: Backend-For-Frontend proxy aggregating REST and Connect-RPC endpoints for frontend clients.
- **Layer directories**:
  - REST (`internal/handler/`): BFF aggregation and proxy handlers
  - Gateway/Driver (`internal/client/`): Client forwarding requests to backend
  - Domain (`internal/domain/`): User session context
- **Composition Root**: `alt-butterfly-facade/main.go`
- **Representative Trace**: BFF Request Forwarding
  1. REST (`internal/handler/`): `alt-butterfly-facade/internal/handler/bff_handler.go` (`BFFHandler`)
  2. Gateway/Driver Client (`internal/client/`): `alt-butterfly-facade/internal/client/backend_client.go` (`BackendClient.ForwardRequest`)

---

### 10. `knowledge-sovereign` (Go - Documented Reduced Shape)
- **Role**: Durable knowledge state single owner (`knowledge-sovereign/app/CLAUDE.md`).
- **Layer structure**: `Handler (handler/) -> Usecase (usecase/) -> Driver (driver/sovereign_db/)`
- **Documented Exception**: `knowledge-sovereign/app/CLAUDE.md` documents that `handler/` and `usecase/` packages directly import `driver/sovereign_db/` without intermediary `port/` or `gateway/` layers. The no-reverse-import rule (`driver` must not import `usecase`) still holds strictly.
- **Composition Root**: `knowledge-sovereign/app/main.go`
- **Representative Trace**: Trail Footprints Query
  1. REST (`handler/`): `knowledge-sovereign/app/handler/rpc_trail.go` (`SovereignHandler.GetTrailFootprints`)
  2. Usecase (`usecase/`): `knowledge-sovereign/app/usecase/trail_episodes/trail_episodes.go` (`Derive`)
  3. Driver (`driver/`): `knowledge-sovereign/app/driver/sovereign_db/read_trail.go` (`Repository.GetTrailFootprints`)
