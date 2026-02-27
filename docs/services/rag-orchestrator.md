# RAG Orchestrator

_Last reviewed: February 28, 2026_

**Location:** `rag-orchestrator`

The `rag-orchestrator` is a Go 1.25+ service responsible for managing the RAG (Retrieval Augmented Generation) pipeline. It handles article indexing, vector embedding, context retrieval, answer generation using an LLM, and morning-letter topic extraction. The service exposes both a REST API (Echo) and a Connect-RPC API for streaming.

## Directory Structure

```
rag-orchestrator/
├── Dockerfile
├── Makefile
├── go.mod                          # Go 1.25.5, connectrpc.com/connect, pgx, echo, cobra
├── cmd
│   ├── backfill
│   │   └── main.go                 # Backfill CLI (cobra)
│   └── server
│       └── main.go                 # Main server entrypoint
├── internal
│   ├── adapter
│   │   ├── altdb
│   │   │   └── article_client.go   # alt-backend article fetcher
│   │   ├── connect
│   │   │   ├── server.go           # Connect-RPC server setup (AugurService, MorningLetterService)
│   │   │   ├── augur
│   │   │   │   └── handler.go      # AugurService: StreamChat, RetrieveContext
│   │   │   └── morning_letter
│   │   │       └── handler.go      # MorningLetterService: StreamChat
│   │   ├── rag_augur
│   │   │   ├── ollama_embedder.go
│   │   │   ├── ollama_generator.go
│   │   │   ├── query_expander_client.go
│   │   │   └── reranker_client.go
│   │   ├── rag_http
│   │   │   ├── handler.go          # REST handler (OpenAPI ServerInterface + manual routes)
│   │   │   ├── openapi/
│   │   │   │   └── server.gen.go   # Generated OpenAPI code
│   │   │   └── search_indexer_client.go
│   │   └── repository
│   │       ├── postgres_tx.go
│   │       ├── rag_chunk_repo.go
│   │       ├── rag_document_repo.go
│   │       └── rag_job_repo.go
│   ├── backfill
│   │   ├── runner.go               # Backfill runner with cursor-based resume
│   │   ├── cursor.go               # Cursor persistence (JSON file)
│   │   └── hyperboost.go           # Local GPU embedding via temporary Ollama container
│   ├── di
│   │   └── container.go            # Dependency injection wiring
│   ├── domain
│   │   ├── article_client.go
│   │   ├── chunker.go
│   │   ├── diff_chunks.go
│   │   ├── llm_client.go
│   │   ├── merger.go
│   │   ├── morning_letter.go
│   │   ├── reranker.go
│   │   ├── repository.go
│   │   ├── search_client.go
│   │   ├── source_hash_policy.go
│   │   ├── splitter.go
│   │   └── vector_encoder.go
│   ├── gen/proto                    # Generated protobuf + Connect-RPC stubs
│   │   └── alt/
│   │       ├── augur/v2/            # AugurService proto
│   │       └── morning_letter/v2/   # MorningLetterService proto
│   ├── infra
│   │   ├── config
│   │   │   └── config.go
│   │   ├── httpclient
│   │   │   └── pool.go
│   │   ├── logger
│   │   │   ├── logger.go
│   │   │   ├── context_logger.go
│   │   │   └── trace_context_handler.go
│   │   ├── otel
│   │   │   └── provider.go         # OpenTelemetry tracing + log bridge
│   │   └── postgres.go
│   ├── usecase
│   │   ├── answer_with_rag_usecase.go
│   │   ├── index_article_usecase.go
│   │   ├── morning_letter_usecase.go
│   │   ├── morning_letter_prompt_builder.go
│   │   ├── output_validator.go
│   │   ├── prompt_builder.go
│   │   ├── rag_answer_stream.go
│   │   ├── rag_answer_types.go
│   │   ├── retrieve_context_usecase.go
│   │   ├── retrieval_config.go
│   │   ├── temporal_boost_config.go
│   │   └── retrieval/              # Retrieval sub-pipeline
│   │       ├── types.go
│   │       ├── expand_queries.go
│   │       ├── embed_and_search.go
│   │       ├── fuse_results.go
│   │       ├── rerank.go
│   │       └── allocate.go
│   └── worker                       # Background job worker (backfill_article jobs)
└── spec
    └── openapi.yaml
```

## Core Infrastructure

### Dockerfile

Builds an optimized distroless image with both the server and backfill binaries.

```dockerfile
FROM golang:1.25-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
COPY internal/gen/proto/go.mod internal/gen/proto/go.sum* ./internal/gen/proto/
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o rag-orchestrator cmd/server/main.go
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o backfill cmd/backfill/main.go

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=builder /app/rag-orchestrator /rag-orchestrator
COPY --from=builder /app/backfill /backfill
USER nonroot:nonroot
EXPOSE 9010
EXPOSE 9011
ENTRYPOINT ["/rag-orchestrator"]
```

### Configuration (`internal/infra/config/config.go`)

Loads configuration from environment variables. Secrets support both env vars and file-based injection (`DB_PASSWORD_FILE`).

#### Server

| Environment Variable | Description | Default |
|----------------------|-------------|---------|
| `PORT` | REST (Echo) server port | `9010` |
| `CONNECT_PORT` | Connect-RPC server port | `9011` |
| `ENV` | Environment name | `development` |

#### Database

| Environment Variable | Description | Default |
|----------------------|-------------|---------|
| `DB_HOST` | Database host | `rag-db` |
| `DB_PORT` | Database port | `5432` |
| `DB_USER` | Database user | `rag_user` |
| `DB_PASSWORD` / `DB_PASSWORD_FILE` | Database password (env or file) | `rag_password` |
| `DB_NAME` | Database name | `rag_db` |
| `DB_MAX_CONNS` | Max pool connections | `20` |
| `DB_MIN_CONNS` | Min pool connections | `5` |

#### Embedder & LLM

| Environment Variable | Description | Default |
|----------------------|-------------|---------|
| `EMBEDDER_EXTERNAL` / `EMBEDDER_EXTERNAL_URL` | Embedder (Ollama) URL | `http://embedder-external:11436` |
| `EMBEDDING_MODEL` | Model for embeddings | `embeddinggemma` |
| `EMBEDDER_TIMEOUT` | Embedder timeout (seconds) | `30` |
| `AUGUR_EXTERNAL` / `AUGUR_EXTERNAL_URL` | Knowledge Augur (LLM) URL | `http://augur-external:11435` |
| `AUGUR_KNOWLEDGE_MODEL` | LLM model for generation | `gemma3-12b-rag` |
| `OLLAMA_TIMEOUT` | LLM timeout (seconds) | `300` |

#### Search & Query Expansion

| Environment Variable | Description | Default |
|----------------------|-------------|---------|
| `SEARCH_INDEXER_URL` | Search indexer service URL | `http://search-indexer:8080` |
| `SEARCH_INDEXER_TIMEOUT` | Search indexer timeout (seconds) | `10` |
| `QUERY_EXPANSION_URL` | Query expansion service URL | `http://news-creator:11434` |
| `QUERY_EXPANSION_TIMEOUT` | Query expansion timeout (seconds) | `3` |

#### RAG Retrieval

| Environment Variable | Description | Default |
|----------------------|-------------|---------|
| `RAG_SEARCH_LIMIT` | Pre-ranking pool size | `50` |
| `RAG_QUOTA_ORIGINAL` | Quota for original query results | `5` |
| `RAG_QUOTA_EXPANDED` | Quota for expanded query results | `5` |
| `RAG_RRF_K` | Reciprocal Rank Fusion constant | `60.0` |
| `RAG_DEFAULT_MAX_CHUNKS` | Max context chunks | `7` |
| `RAG_DEFAULT_MAX_TOKENS` | Max generation tokens (answer) | `6144` |
| `MORNING_LETTER_MAX_TOKENS` | Max generation tokens (morning letter) | `4096` |
| `RAG_MAX_PROMPT_TOKENS` | Max prompt tokens for context limiting | `6000` |
| `RAG_DEFAULT_LOCALE` | Default response locale | `ja` |
| `RAG_PROMPT_VERSION` | Prompt version tag | `alpha-v1` |
| `RAG_DYNAMIC_LANGUAGE_ALLOCATION` | Dynamic score-based language allocation | `true` |

#### Re-ranking

| Environment Variable | Description | Default |
|----------------------|-------------|---------|
| `RERANK_ENABLED` | Enable cross-encoder reranking | `true` |
| `RERANK_URL` | Reranker service URL | `http://news-creator:11434` |
| `RERANK_MODEL` | Reranker model | `BAAI/bge-reranker-v2-m3` |
| `RERANK_TOP_K` | Rerank 50 -> top K | `10` |
| `RERANK_TIMEOUT` | Reranker timeout (seconds) | `10` |

#### Hybrid Search

| Environment Variable | Description | Default |
|----------------------|-------------|---------|
| `HYBRID_SEARCH_ENABLED` | Enable BM25+vector hybrid search | `true` |
| `HYBRID_ALPHA` | BM25/vector balance (lower = more BM25) | `0.3` |
| `HYBRID_BM25_LIMIT` | BM25 search limit | `50` |

#### Temporal Boost

| Environment Variable | Description | Default |
|----------------------|-------------|---------|
| `TEMPORAL_BOOST_6H` | Score boost for 0-6h old articles | `1.3` |
| `TEMPORAL_BOOST_12H` | Score boost for 6-12h old articles | `1.15` |
| `TEMPORAL_BOOST_18H` | Score boost for 12-18h old articles | `1.05` |

#### Other

| Environment Variable | Description | Default |
|----------------------|-------------|---------|
| `ALT_BACKEND_URL` | alt-backend service URL | `http://alt-backend:9000` |
| `ALT_BACKEND_TIMEOUT` | alt-backend timeout (seconds) | `30` |
| `RAG_CACHE_SIZE` | Answer cache max entries | `256` |
| `RAG_CACHE_TTL_MINUTES` | Answer cache TTL (minutes) | `10` |

### API Endpoints

The service runs two servers concurrently:

#### REST API (Echo) -- port 9010

Registered via OpenAPI-generated code (`RegisterHandlers`) plus manual routes:

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/internal/rag/index/upsert` | Idempotently index an article (supports `X-Embedder-URL` header for hyper-boost) |
| `POST` | `/internal/rag/index/delete` | Soft-delete an article (not implemented) |
| `POST` | `/v1/rag/retrieve` | Retrieve relevant context chunks for a query |
| `POST` | `/v1/rag/answer` | Generate an answer using RAG |
| `POST` | `/v1/rag/answer/stream` | Stream a generated answer via SSE |
| `POST` | `/v1/rag/morning-letter` | Extract important topics from recent articles |
| `POST` | `/internal/rag/backfill` | Enqueue an article for background backfill indexing |
| `GET`  | `/healthz` | Liveness probe (always 200) |
| `GET`  | `/readyz` | Readiness probe (checks DB connectivity) |

#### Connect-RPC API -- port 9011

Served over HTTP/2 (h2c) with Connect-RPC protocol:

| Service | RPC | Description |
|---------|-----|-------------|
| `AugurService` | `StreamChat` | Server-streaming RAG chat (delta/meta/done/fallback/error/thinking events) |
| `AugurService` | `RetrieveContext` | Unary context retrieval |
| `MorningLetterService` | `StreamChat` | Server-streaming morning letter with time-bounded RAG context |
| -- | `GET /connect/health` | Connect-RPC server health check |

## Logic and Implementation

### Domain Layer (`internal/domain`)

Defines the core entities and interfaces:

- **Entities**: `RagDocument`, `RagDocumentVersion`, `RagChunk`, `RagChunkEvent`, `RagJob`, `TopicSummary`, `MorningLetterResponse`, `ArticleRef`.
- **Interfaces**:
    - `RagDocumentRepository`, `RagChunkRepository`: Persistence.
    - `VectorEncoder`: Embedding generation.
    - `LLMClient`: Chat generation (sync + streaming).
    - `Chunker`: Text splitting logic.
    - `SourceHashPolicy`: Idempotency hashing.
    - `ArticleClient`: Fetches recent articles from alt-backend.
    - `Reranker`: Cross-encoder reranking.
    - `SearchClient`: BM25/keyword search against search-indexer.

- **Utilities**:
    - `DiffChunks`: Logic to compute `added`, `updated`, `deleted`, `unchanged` diffs between chunk versions.

### Adapter Layer (`internal/adapter`)

- **Repositories**: `postgres_tx.go`, `rag_chunk_repo.go`, `rag_document_repo.go`, `rag_job_repo.go` implement persistence using `pgx` and `pgvector`.
- **RAG Augur**:
    - `ollama_embedder.go`: Calls Ollama `/api/embed`.
    - `ollama_generator.go`: Calls Ollama `/api/chat` (supports streaming).
    - `query_expander_client.go`: LLM-based query expansion.
    - `reranker_client.go`: Cross-encoder reranking via external service.
- **HTTP Handlers**: `rag_http/handler.go` implements `ServerInterface` from generated OpenAPI code plus manual routes for morning-letter and backfill.
- **Connect-RPC Handlers**:
    - `connect/augur/handler.go`: AugurService -- streaming RAG chat and unary context retrieval.
    - `connect/morning_letter/handler.go`: MorningLetterService -- streaming morning letter with time-bounded article fetching from alt-backend.
- **AltDB Client**: `altdb/article_client.go` fetches recent articles from alt-backend for morning letter.
- **Search Client**: `search_indexer_client.go` queries `search-indexer` for candidate articles.

### Usecases (`internal/usecase`)

#### 1. Index Article (`index_article_usecase.go`)

Handles the lifecycle of a document version:
1.  Computes source hash of title + body.
2.  Checks if content has changed (idempotency).
3.  Splits body into chunks.
4.  Generates embeddings for new chunks.
5.  Calculates diffs against the previous version.
6.  Persists chunks and events (`added`, `updated` etc.).
7.  Updates the current version pointer.

#### 2. Retrieve Context (`retrieve_context_usecase.go`)

Multi-stage retrieval pipeline (see `usecase/retrieval/` sub-package):
1.  **Query Expansion** (`expand_queries.go`): Translates and expands the query using LLM.
2.  **Embed & Search** (`embed_and_search.go`): Embeds queries and performs vector search. Optionally runs BM25 hybrid search in parallel.
3.  **Fusion** (`fuse_results.go`): Merges vector and BM25 results using Reciprocal Rank Fusion (RRF, k=60).
4.  **Rerank** (`rerank.go`): Cross-encoder reranking (50 -> top 10) using BAAI/bge-reranker-v2-m3.
5.  **Allocate** (`allocate.go`): Language-aware allocation with dynamic score-based mode.
6.  Returns context items with metadata.

#### 3. Answer with RAG (`answer_with_rag_usecase.go`)

**Single-Phase Generation:**
1.  **Retrieval**: Calls `RetrieveContextUsecase`.
2.  **Prompt Building**: Constructs a structured XML-like prompt containing instructions, the user query, and retrieved context chunks.
3.  **Generation**: Calls `LLMClient.Chat` (or `ChatStream`).
    - Enforces a JSON response format for structure.
4.  **Validation**: Parses and validates the LLM's JSON output (e.g., checks citations).
5.  **Output**: Returns the answer, citations, and debug info.
    - Supports caching of answers (LRU, 256 entries, 10min TTL).
    - Supports streaming via SSE, with partial JSON parsing to stream text token-by-token.

#### 4. Morning Letter (`morning_letter_usecase.go`)

Extracts important topics from recent articles for a daily briefing:
1.  **Validate & Defaults**: Time window (default 24h, max 7 days), topic limit (default 10, max 20), locale (default `ja`).
2.  **Fetch Articles**: Gets recent articles from alt-backend via `ArticleClient.GetRecentArticles`.
3.  **Retrieve Context**: Calls `RetrieveContextUsecase` with article IDs as candidates.
4.  **Temporal Boost**: Applies configurable time-decay boost factors (1.3x for 0-6h, 1.15x for 6-12h, 1.05x for 12-18h) and re-sorts by boosted score.
5.  **Token-Based Context Limiting**: Caps prompt size at `RAG_MAX_PROMPT_TOKENS` using ~3 chars/token heuristic.
6.  **Prompt Building**: Uses `MorningLetterPromptBuilder` to construct topic extraction prompt.
7.  **LLM Generation**: Generates structured JSON with topics via `LLMClient.Chat` (max `MORNING_LETTER_MAX_TOKENS`=4096 tokens).
8.  **Parse & Enrich**: Parses JSON response into `TopicSummary` slice with article references.

### Backfill CLI (`cmd/backfill`)

A standalone cobra-based CLI for bulk-indexing articles into the RAG system. Built as a separate binary in the Docker image.

**Subcommands:**

| Command | Description |
|---------|-------------|
| `backfill run` | Run the backfill process (resumes from cursor) |
| `backfill status` | Show current cursor position |
| `backfill reset-cursor` | Reset cursor to start from beginning |

**Flags (run):**

| Flag | Default | Description |
|------|---------|-------------|
| `--from` | -- | Start date (YYYY-MM-DD) |
| `--to` | today | End date (YYYY-MM-DD) |
| `--concurrency` | `4` | Concurrent requests |
| `--batch-size` | `40` | Articles per batch |
| `--dry-run` | `false` | Preview without processing |
| `--hyper-boost` | `false` | Use local GPU for embedding (starts temporary Ollama container) |
| `--cursor-file` | `cursor.json` | Cursor persistence file |

**Environment:**
- `DATABASE_URL` (required): PostgreSQL connection string for fetching articles.
- `ORCHESTRATOR_URL` (default `http://localhost:9010`): rag-orchestrator REST endpoint.

Hyper-boost mode starts a temporary Ollama container for local GPU embedding and sends an `X-Embedder-URL` header to the orchestrator's upsert endpoint.

### Connect-RPC (`internal/adapter/connect`)

The Connect-RPC server runs on a separate port (default 9011) and supports HTTP/2 (h2c) for server-streaming RPCs.

**AugurService** (`connect/augur/handler.go`):
- `StreamChat`: Extracts the last user message, streams answer via `AnswerWithRAGUsecase.Stream()`. Events: `delta`, `meta`, `done`, `fallback`, `error`, `thinking`. Sanitizes UTF-8 for protobuf compatibility.
- `RetrieveContext`: Unary RPC wrapping `RetrieveContextUsecase.Execute()`.

**MorningLetterService** (`connect/morning_letter/handler.go`):
- `StreamChat`: Fetches recent articles from alt-backend (time-bounded, default 24h, max 7 days), sends a `meta` event with time window info, then streams the RAG answer. Events: `meta`, `delta`, `done`, `fallback`, `error`.
