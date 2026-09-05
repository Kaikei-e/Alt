# RAG Orchestrator

_Last reviewed: September 5, 2026_

**Location:** `rag-orchestrator`

The `rag-orchestrator` is a Go 1.26+ service responsible for managing the RAG (Retrieval Augmented Generation) pipeline. It handles article indexing, vector embedding, context retrieval, answer generation using an LLM, agentic tool-calling, conversation persistence, and morning-letter topic extraction. The service exposes both a REST API (Echo) and a Connect-RPC API for streaming. Generation is served by `news-creator` (its FastAPI priority-queue proxy on :11434, which fronts `news-creator-backend`'s Ollama on :11435) by default (see [[000951]] / [[000987]]); `knowledge-augur` is a separate, non-default Ollama host (see `docs/services/knowledge-augur.md`).

## Directory Structure

```
rag-orchestrator/
├── Dockerfile
├── Makefile
├── go.mod                          # Go 1.26+, connectrpc.com/connect, pgx, echo, cobra
├── cmd
│   ├── backfill
│   │   └── main.go                 # Backfill / rebuild CLI (cobra) — not shipped in the server image
│   ├── eval
│   │   └── main.go                 # Golden-set retrieval/generation eval CLI (dev/CI only, not in the Docker image)
│   └── server
│       └── main.go                 # Main server entrypoint
├── eval/                            # Eval harness (metrics, A/B profiles, golden cases) used by cmd/eval
├── internal
│   ├── adapter
│   │   ├── altdb
│   │   │   ├── article_client.go        # alt-backend article fetcher
│   │   │   ├── articles_by_tag_client.go
│   │   │   ├── datahub_client.go        # alt-data-hub mTLS client (DataHubService)
│   │   │   ├── recap_search_client.go
│   │   │   └── tag_cloud_client.go
│   │   ├── connect
│   │   │   ├── server.go           # Connect-RPC server setup (AugurService, MorningLetterService)
│   │   │   ├── augur
│   │   │   │   ├── handler.go      # AugurService: StreamChat, RetrieveContext, conversation CRUD
│   │   │   │   └── stream_telemetry.go
│   │   │   └── morning_letter
│   │   │       └── handler.go      # MorningLetterService: StreamChat
│   │   ├── contract/                # Pact CDC consumer contracts (alt-data-hub, knowledge-sovereign, news-creator chat + plan-query, recap-worker, search-indexer)
│   │   ├── eino                    # Eino chat-model / tool adapters for the agentic tool-calling path
│   │   ├── rag_augur
│   │   │   ├── ollama_embedder.go
│   │   │   ├── ollama_generator.go
│   │   │   ├── query_expander_client.go
│   │   │   ├── query_planner_client.go  # LLM-based query planner (replaces legacy rule-based planner)
│   │   │   └── reranker_client.go
│   │   ├── rag_http
│   │   │   ├── handler.go          # REST handler (OpenAPI ServerInterface + manual routes)
│   │   │   ├── openapi/
│   │   │   │   └── server.gen.go   # Generated OpenAPI code
│   │   │   └── search_indexer_client.go
│   │   ├── recap_worker
│   │   │   └── morning_letter_client.go
│   │   ├── repository
│   │   │   ├── postgres_tx.go
│   │   │   ├── augur_conversation_repo.go  # augur_conversations / augur_messages persistence
│   │   │   ├── hybrid_search_repo.go       # in-DB pgvector + tsvector RRF
│   │   │   ├── rag_chunk_repo.go
│   │   │   ├── rag_document_repo.go
│   │   │   └── rag_job_repo.go
│   │   ├── sovereign_client                # knowledge-sovereign event emit (augur.conversation_linked.v1)
│   │   └── tools                            # Agentic tool implementations (article lookup, tag search, recap search, ...)
│   ├── backfill
│   │   ├── runner.go               # Legacy HTTP backfill runner with cursor-based resume
│   │   ├── cursor.go               # Cursor persistence (JSON file, legacy `run` path only)
│   │   ├── direct_indexer.go       # `run --direct`: bypass HTTP, index straight into rag-db
│   │   ├── enqueue.go              # `rebuild enqueue`: queue rag_jobs for a chunker/embedder target
│   │   ├── rebuild.go              # `rebuild run`: bounded worker pool draining rag_jobs
│   │   ├── sql_source.go
│   │   └── hyperboost.go           # Local GPU embedding via temporary Ollama container
│   ├── di
│   │   └── container.go            # Dependency injection wiring
│   ├── domain
│   │   ├── article_client.go
│   │   ├── augur_conversation.go
│   │   ├── chunker.go              # ChunkerVersion v10: CJK sentence-boundary overlap, merge-forward
│   │   ├── conversation_state.go
│   │   ├── diff_chunks.go
│   │   ├── llm_client.go
│   │   ├── planner.go / query_planner.go   # PlannerOutput, QueryPlannerPort
│   │   ├── reranker.go             # RerankCandidate, ScoreKind (vector/bm25/rrf/rerank)
│   │   ├── repository.go
│   │   ├── sanitizer.go            # HTML sanitization before chunking
│   │   ├── search_client.go
│   │   ├── source_hash_policy.go
│   │   ├── tool.go                 # Agentic tool interface
│   │   └── vector_encoder.go
│   ├── gen/proto                    # Generated protobuf + Connect-RPC stubs
│   │   └── alt/
│   │       ├── augur/v2/            # AugurService proto (StreamChat, RetrieveContext, conversation CRUD)
│   │       └── morning_letter/v2/   # MorningLetterService proto
│   ├── infra
│   │   ├── config
│   │   │   └── config.go
│   │   ├── httpclient
│   │   │   ├── pool.go
│   │   │   ├── datahub.go          # mTLS client for alt-data-hub
│   │   │   └── mtls.go
│   │   ├── logger/
│   │   ├── metrics/                 # Prometheus metrics (augur.go, coverage.go — embedding coverage gauge)
│   │   ├── otel/                    # OpenTelemetry tracing + log bridge
│   │   ├── tlsutil/
│   │   └── postgres.go
│   ├── middleware
│   │   └── peer_identity.go         # Inbound mTLS peer-CN allowlist for the Connect-RPC listener
│   ├── pki                          # In-process cert enrollment (step-ca) for outbound + inbound mTLS
│   ├── usecase
│   │   ├── answer_with_rag_usecase.go       # Retry/acceptance profiles, corrective retry, related citations
│   │   ├── agentic_synthesis_strategy.go / agent_step.go / tool_dispatcher.go / tool_planner.go
│   │   ├── augur_conversation_usecase.go
│   │   ├── index_article_usecase.go
│   │   ├── morning_letter_usecase.go
│   │   ├── morning_letter_prompt_builder.go
│   │   ├── output_validator.go
│   │   ├── prompt_builder.go / prompt_template*.go
│   │   ├── query_classifier.go / query_intent.go
│   │   ├── relevance_gate.go        # Cross-encoder score gate (Good/Marginal/Insufficient)
│   │   ├── rag_answer_stream.go
│   │   ├── rag_answer_types.go
│   │   ├── retrieve_context_usecase.go
│   │   ├── retrieval_config.go
│   │   ├── strategy_*.go            # Per-intent retrieval strategies (causal, comparison, temporal, ...)
│   │   ├── temporal_boost_config.go
│   │   └── retrieval/              # Retrieval sub-pipeline (RetrievalGraph, 5 stages)
│   │       ├── types.go
│   │       ├── expand_queries.go
│   │       ├── embed_and_search.go
│   │       ├── fuse_results.go
│   │       ├── rerank.go
│   │       ├── allocate.go
│   │       └── graph.go
│   └── worker                       # Background job worker (backfill_article jobs)
└── spec
    └── openapi.yaml
```

## Core Infrastructure

### Dockerfile

Builds an optimized distroless image with both the server and backfill binaries. Both the builder and the runtime base image are digest-pinned. `cmd/eval` is not built into this image — it is a dev/CI-only tool.

```dockerfile
FROM golang:1.26-alpine@sha256:70b46548e42db77e0966aaf3619fd068734dc6c77584d526b91126504fd95816 AS builder
WORKDIR /app
COPY go.mod go.sum ./
COPY internal/gen/proto/go.mod internal/gen/proto/go.sum* ./internal/gen/proto/
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o rag-orchestrator cmd/server/main.go
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o backfill cmd/backfill/main.go

FROM gcr.io/distroless/static-debian12:nonroot@sha256:d093aa3e30dbadd3efe1310db061a14da60299baff8450a17fe0ccc514a16639
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
| `EMBEDDER_EXTERNAL` / `EMBEDDER_EXTERNAL_URL` | Embedder (Ollama) URL | `http://embedder-external:11436` in code; compose/rag.yaml sets `http://knowledge-embedder-local:11434` (dedicated local Ollama instance, [[000951]]) |
| `EMBEDDING_MODEL` | Model for embeddings | `bge-m3` (1024-dim; the pgvector column was widened from 768-dim `embeddinggemma` for this — see `docs/services/rag-db.md`) |
| `EMBEDDER_TIMEOUT` | Embedder timeout (seconds) | `30` in code; compose sets `60` |
| `AUGUR_EXTERNAL` / `AUGUR_EXTERNAL_URL` | LLM generation backend URL | `http://news-creator-backend:11435` in code and in `.env.template`; compose/rag.yaml falls back to `http://news-creator:11434` (news-creator's FastAPI priority-queue proxy, which fronts news-creator-backend's Ollama on :11435) when `AUGUR_EXTERNAL` is unset in `.env` — either way generation is served by news-creator, not by `knowledge-augur` (see `docs/services/knowledge-augur.md`) |
| `AUGUR_KNOWLEDGE_MODEL` | LLM model for generation | `gemma4-e4b-12k` |
| `OLLAMA_TIMEOUT` | LLM timeout (seconds) | `300` |
| `LLM_BACKEND` | Generation backend selector | `ollama` |

#### Search & Query Expansion

| Environment Variable | Description | Default |
|----------------------|-------------|---------|
| `SEARCH_INDEXER_URL` | Search indexer service URL | `http://search-indexer:8080` in code; compose sets `http://search-indexer:9300` |
| `SEARCH_INDEXER_TIMEOUT` | Search indexer timeout (seconds) | `10` |
| `QUERY_EXPANSION_URL` | Query expansion service URL (news-creator) | `http://news-creator:11434` |
| `QUERY_EXPANSION_TIMEOUT` | Query expansion timeout (seconds) | `3` in code; compose sets `30` |
| `QUERY_PLANNER_TIMEOUT` | Timeout for the LLM-based query planner (thinking mode; replaces the legacy rule-based `ConversationPlanner` when wired) | `60` |

#### RAG Retrieval

| Environment Variable | Description | Default |
|----------------------|-------------|---------|
| `RAG_SEARCH_LIMIT` | Pre-ranking pool size | `50` |
| `RAG_QUOTA_ORIGINAL` | Quota for original query results | `5` |
| `RAG_QUOTA_EXPANDED` | Quota for expanded query results | `5` |
| `RAG_RRF_K` | Reciprocal Rank Fusion constant | `60.0` |
| `RAG_DEFAULT_MAX_CHUNKS` | Max context chunks | `10` — defaults to the rerank top-k so every reranked hit reaches the prompt (previously defaulted to `7`, which silently dropped 3 of the reranker's 10 hits; fixed 2026-09) |
| `RAG_DEFAULT_MAX_TOKENS` | Max generation tokens (answer) | `3072` in code; compose sets the same |
| `MORNING_LETTER_MAX_TOKENS` | Max generation tokens (morning letter) | `4096` |
| `RAG_MAX_PROMPT_TOKENS` | Max prompt tokens for context limiting | `6000` |
| `RAG_DEFAULT_LOCALE` | Default response locale | `ja` |
| `RAG_PROMPT_VERSION` | Prompt version tag | `alpha-v1` in code; compose sets `alpha-v2` (per-intent templates with an explicit length floor — see "Answer length and retry" below) |
| `RAG_DYNAMIC_LANGUAGE_ALLOCATION` | Dynamic score-based language allocation | `true` |
| `RAG_MIN_ANSWER_LENGTH` | Minimum rune count for a "quality" answer (used by answer-quality assessment, distinct from the per-intent length floors below) | `800` |
| `RAG_COVERAGE_SAMPLE_INTERVAL_MINUTES` | Poll interval for the embedding-coverage gauge (chunker × embedder version census over rag-db) | `15` |

#### Retrieval Quality Gate

Two independent quality gates exist in the code, and only one of them runs on the shipped default path:

- **`RelevanceGate`** (`relevance_gate.go`) — cross-encoder score-based gating, added with the 2026-09 retrieval redesign ([[000987]]). This is the gate that actually runs: DI (`di/container.go`) wires it unconditionally via `NewRelevanceGate(0.5, 0.25)`, with both thresholds hardcoded rather than read from config. The `RAG_QUALITY_THRESHOLD_GOOD`/`RAG_QUALITY_THRESHOLD_MARGINAL` variables below do **not** configure it, and it has no minimum-context knob. Only a reranked (`ScoreKind=rerank`) top-1 score is compared against 0.5/0.25; a vector/BM25/RRF score is a ranking signal, not a calibrated quality score, and always reads as `Marginal`.
- **`RetrievalQualityAssessor`** (legacy heuristic assessor) — this is what the env vars in the table below actually configure (`di/container.go`), and it is also what `strategy_causal.go`'s `NewCausalStrategy` uses for causal-intent retrieval. It only runs in the legacy `buildPrompt` branch of `answer_with_rag_usecase.go`, which is unreachable on the shipped default path: DI wires `WithQueryPlanner` unconditionally, and the query-planner path (`buildPromptWithQueryPlanner`) prefers `RelevanceGate` whenever it is present and never falls back to this assessor.

| Environment Variable | Description | Default |
|----------------------|-------------|---------|
| `RAG_QUALITY_GATE_ENABLED` | Enable the legacy `RetrievalQualityAssessor` (does not affect `RelevanceGate`) | `true` |
| `RAG_QUALITY_THRESHOLD_GOOD` | `RetrievalQualityAssessor` good-quality threshold (also passed to `strategy_causal.go`) | `0.5` |
| `RAG_QUALITY_THRESHOLD_MARGINAL` | `RetrievalQualityAssessor` marginal-quality threshold | `0.25` |
| `RAG_QUALITY_MIN_CONTEXTS` | Minimum contexts required before the legacy assessor runs | `3` |

On the current default path (`buildPromptWithQueryPlanner`, active because the query planner is wired unconditionally), `RelevanceGate.Evaluate` produces the verdict: an `Insufficient` verdict returns a fallback response carrying `fallback_code=relevance_low` (`ErrRelevanceInsufficient`, mapped in `fallback_code.go`), and a `Marginal` verdict proceeds straight to generation with no retry. The low-confidence-disclaimer degrade and the `Marginal` expanded/decomposed-query retry that the legacy `RetrievalQualityAssessor` performs (see `buildPrompt` in `answer_with_rag_usecase.go`) exist only on that unreachable legacy branch.

#### Re-ranking

| Environment Variable | Description | Default |
|----------------------|-------------|---------|
| `RERANK_ENABLED` | Enable cross-encoder reranking | `true` |
| `RERANK_URL` | Reranker service URL | `http://news-creator:11434` in code; compose/rag.yaml sets `http://rerank-local:8080` (dedicated ONNX int8 CPU reranker — see `rerank-server`, [[000951]]) |
| `RERANK_MODEL` | Reranker model | `cl-nagoya/ruri-v3-reranker-310m` (adopted 2026-09 after an A/B eval against `BAAI/bge-reranker-v2-m3`; must match `rerank-local`'s `RERANK_MODEL` or the server 422s) |
| `RERANK_TOP_K` | Hits kept after reranking | `10` |
| `RERANK_MAX_CANDIDATES` | Hits sent to the cross-encoder (input cap). Kept larger than `RERANK_TOP_K` on purpose — capping the input at `RERANK_TOP_K` made a hit ranked 11th by retrieval unpromotable, which defeated the point of reranking | `40` |
| `RERANK_TIMEOUT` | Reranker client timeout (seconds) | `15` (`RERANK_SERVER_TIMEOUT` of 10 + 5s margin) in code; compose sets `12`, kept just above rerank-local's own `RERANK_SERVER_TIMEOUT=10` budget |

#### Hybrid Search

| Environment Variable | Description | Default |
|----------------------|-------------|---------|
| `HYBRID_SEARCH_ENABLED` | Enable BM25+vector hybrid search | `true` |
| `HYBRID_BM25_LIMIT` | BM25 search limit | `50` |
| `HYBRID_BM25_SOURCE` | BM25 backend: `meilisearch` (search-indexer) or `postgres` (in-DB `tsvector` hybrid via `hybrid_search_repo.go`) | `meilisearch` |

`HYBRID_ALPHA` (BM25/vector score balance) has been removed — RRF fusion (k=`RAG_RRF_K`) replaces score-space blending.

#### Temporal Boost

| Environment Variable | Description | Default |
|----------------------|-------------|---------|
| `TEMPORAL_BOOST_6H` | Score boost for 0-6h old articles | `1.3` |
| `TEMPORAL_BOOST_12H` | Score boost for 6-12h old articles | `1.15` |
| `TEMPORAL_BOOST_18H` | Score boost for 12-18h old articles | `1.05` |

#### alt-data-hub (alt_db access) and Peer Identity

`ALT_BACKEND_URL` / `ALT_BACKEND_CONNECT_URL` no longer exist. The 3-binary split ([[000954]]) turned `alt-backend:9102` into an admin-only listener; every route backed by `alt_db` now goes through `alt-data-hub`'s mutual-TLS `services.datahub.v1.DataHubService`. Every `DataHub*` variable below is required — `config.Load()` panics at startup if any is missing, rather than starting and 404ing every tag-cloud/morning-letter query.

| Environment Variable | Description | Default |
|----------------------|-------------|---------|
| `DATAHUB_MTLS_URL` | alt-data-hub Connect-RPC base URL (must be `https://`) | none — required |
| `DATAHUB_MTLS_SERVER_NAME` | Pins `tls.Config.ServerName` | empty (derived from the URL host) |
| `DATAHUB_TIMEOUT` | Timeout for a single DataHub RPC (seconds) | `30` |
| `MTLS_CERT_FILE` / `MTLS_KEY_FILE` / `MTLS_CA_FILE` | Client cert material presented to alt-data-hub — the same pki-agent-issued leaf the inbound Connect-RPC listener presents | required when `PEER_IDENTITY_MODE=mtls` or DataHub is used |
| `PEER_IDENTITY_MODE` | Inbound auth mode for the Connect-RPC listener (`:9011`): `mtls` (RequireAndVerifyClientCert + CN allowlist) or `disabled` (plaintext h2c) | none — required, no inferred default |
| `PEER_IDENTITY_ALLOWED_PEERS` | Comma-separated client-cert CNs admitted when `PEER_IDENTITY_MODE=mtls` | none — required in mtls mode |

#### Backend & recap-worker

| Environment Variable | Description | Default |
|----------------------|-------------|---------|
| `ALT_BACKEND_TIMEOUT` | Reserved timeout setting (the `alt-backend` URL it used to pair with is gone; see above) | `30` |
| `RECAP_WORKER_URL` | recap-worker REST base URL, used for morning-letter fetching | `http://recap-worker:9005` |

#### Cache

| Environment Variable | Description | Default |
|----------------------|-------------|---------|
| `RAG_CACHE_SIZE` | Answer cache max entries | `256` |
| `RAG_CACHE_TTL_MINUTES` | Answer cache TTL (minutes) | `10` |

#### PKI Enrollment & Ops Listener

In-process certificate enrollment via step-ca ([[000978]]) — no pki-agent sidecar. The dedicated ops listener it opens (default `127.0.0.1:9110`) is independent of the Echo API `/metrics` mux; it serves `/health` and `/metrics` from a private Prometheus registry (`internal/pki/ops.go`), so PKI series never leak onto the public API surface.

| Environment Variable | Description | Default |
|----------------------|-------------|---------|
| `PKI_ENROLLMENT` | Enable in-process cert enrollment | `enabled` in compose |
| `CERT_SUBJECT` | Certificate subject name | `rag-orchestrator` |
| `CERT_SANS` | Certificate SANs (comma-separated) | `rag-orchestrator,localhost` |
| `CERT_PATH` / `KEY_PATH` | Paths the enrolled leaf cert/key are written to | `/certs/svc-cert.pem` / `/certs/svc-key.pem` |
| `STEP_CA_URL` | step-ca enrollment endpoint | `https://step-ca:9000` |
| `STEP_CA_ROOT_FILE` | CA trust bundle | `/trust/ca-bundle.pem` |
| `STEP_CA_PROVISIONER` | step-ca provisioner name (subject-scoped JWK) | `pki-agent-rag-orchestrator` |
| `STEP_CA_PROVISIONER_PASSWORD_FILE` | Provisioner JWK password file | `/run/secrets/pki-agent-rag-orchestrator-jwk` |
| `RENEW_AT_FRACTION` | Renew once this fraction of the cert's lifetime has elapsed | `0.66` |
| `OPS_LISTEN` | Bind address for the PKI ops listener (`/health`, `/metrics`) | `127.0.0.1:9110` in code; compose sets `:9110` |

### API Endpoints

The service runs three servers concurrently:

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
| `GET`  | `/metrics` | Prometheus metrics (Echo mux) |

`POST /internal/rag/index/upsert` accepts an `X-Embedder-URL` override only when it exactly matches an entry in the `RAG_EMBEDDER_ALLOWED_OVERRIDE_URLS` allowlist (default: the hyper-boost backfill container's fixed origin) — the header is not honored for an arbitrary URL.

#### Connect-RPC API -- port 9011

Served over HTTP/2 (h2c or mTLS, per `PEER_IDENTITY_MODE`) with Connect-RPC protocol:

| Service | RPC | Description |
|---------|-----|-------------|
| `AugurService` | `StreamChat` | Server-streaming RAG chat (delta/meta/done/fallback/error/thinking/progress events) |
| `AugurService` | `RetrieveContext` | Unary context retrieval |
| `AugurService` | `ListConversations` | List a user's persisted Ask Augur conversations |
| `AugurService` | `GetConversation` | Fetch one conversation's message history |
| `AugurService` | `DeleteConversation` | Delete a conversation (cascades to its messages) |
| `MorningLetterService` | `StreamChat` | Server-streaming morning letter with time-bounded RAG context |
| -- | `GET /connect/health` | Connect-RPC server health check |

#### PKI Ops Listener -- port 9110

Serves `/health` and `/metrics` from a private Prometheus registry carrying the PKI enrollment/renewal series — see "PKI Enrollment & Ops Listener" above. Bound to `127.0.0.1:9110` by default; compose sets `OPS_LISTEN=:9110`.

Ask Augur chat turns are persisted append-only to `augur_conversations` / `augur_messages` in rag-db (see `docs/services/rag-db.md`); `ListConversations`/`GetConversation`/`DeleteConversation` read and manage that history.

## Logic and Implementation

### Domain Layer (`internal/domain`)

Defines the core entities and interfaces:

- **Entities**: `RagDocument`, `RagDocumentVersion`, `RagChunk`, `RagChunkEvent`, `RagJob`, `TopicSummary`, `MorningLetterResponse`, `ArticleRef`, `AugurConversation`/`AugurMessage`.
- **Interfaces**:
    - `RagDocumentRepository`, `RagChunkRepository`: Persistence.
    - `VectorEncoder`: Embedding generation.
    - `LLMClient`: Chat generation (sync + streaming).
    - `Chunker`: Text splitting logic (ChunkerVersion v10; see "Chunking" below).
    - `SourceHashPolicy`: Idempotency hashing.
    - `ArticleClient`: Fetches recent articles from alt-backend (via alt-data-hub mTLS).
    - `Reranker`: Cross-encoder reranking.
    - `BM25Searcher` / `HybridSearcher`: BM25 (Meilisearch) or in-DB pgvector+tsvector hybrid search.
    - `SearchClient`: BM25/keyword search against search-indexer.
    - `QueryPlannerPort`: LLM-based query planning (clarification, operation classification).
    - `Tool`: Agentic tool interface (see "Agentic tool dispatch" below).

- **Types**:
    - `ScoreKind` (`vector` / `bm25` / `rrf` / `rerank`): declares which score space a `ContextItem.Score` lives in, so only a calibrated cross-encoder score is compared against `RelevanceGate` thresholds.

- **Utilities**:
    - `DiffChunks`: Logic to compute `added`, `updated`, `deleted`, `unchanged` diffs between chunk versions.

### Adapter Layer (`internal/adapter`)

- **Repositories**: `postgres_tx.go`, `rag_chunk_repo.go`, `rag_document_repo.go`, `rag_job_repo.go` implement persistence using `pgx` and `pgvector`. `hybrid_search_repo.go` implements the in-DB pgvector+tsvector hybrid search (`HYBRID_BM25_SOURCE=postgres`). `augur_conversation_repo.go` persists chat history (`augur_conversations`/`augur_messages`).
- **RAG Augur**:
    - `ollama_embedder.go`: Calls Ollama `/api/embed`.
    - `ollama_generator.go`: Calls Ollama `/api/chat` (supports streaming); derives sampling options per model family (e.g. Gemma runs with `repeat_penalty=1.0` — see "Answer length and retry").
    - `query_expander_client.go`: Legacy LLM-based query expansion.
    - `query_planner_client.go`: LLM-based query planner (news-creator), used when wired in place of the legacy expander/classifier/`ConversationPlanner` chain.
    - `reranker_client.go`: Cross-encoder reranking via `rerank-server` (`rerank-local` in compose).
- **HTTP Handlers**: `rag_http/handler.go` implements `ServerInterface` from generated OpenAPI code plus manual routes for morning-letter and backfill.
- **Connect-RPC Handlers**:
    - `connect/augur/handler.go`: AugurService -- streaming RAG chat, unary context retrieval, and conversation history CRUD.
    - `connect/morning_letter/handler.go`: MorningLetterService -- streaming morning letter with time-bounded article fetching from alt-backend.
- **AltDB / DataHub Client**: `altdb/article_client.go`, `articles_by_tag_client.go`, `tag_cloud_client.go`, `recap_search_client.go` fetch data from alt-backend's article store via `altdb/datahub_client.go`, an mTLS client to alt-data-hub's `services.datahub.v1.DataHubService` ([[000954]]) — there is no plaintext route to `alt_db`.
- **Search Client**: `search_indexer_client.go` queries `search-indexer` for candidate articles.
- **Sovereign Client**: `sovereign_client/` emits `augur.conversation_linked.v1` into knowledge-sovereign's append-only event log when `RAG_ORCHESTRATOR_KNOWLEDGE_EVENT_EMIT=true` (see `docs/wiki/services/knowledge-sovereign.md`).
- **Tools**: `tools/` implements the agentic tool set the query planner can dispatch to (article lookup, tag search, tag-cloud explore, related articles, recap search, date-range filter, summarize-for-context).
- **Eino**: `eino/` adapts `LLMClient` and the tool set to the Eino `ChatModelAgent` interface for the agentic tool-calling path.

### Usecases (`internal/usecase`)

#### 1. Index Article (`index_article_usecase.go`)

Handles the lifecycle of a document version:
1.  Computes source hash of title + body.
2.  Checks if content has changed (idempotency).
3.  Splits body into chunks using `domain.Chunker` (`ChunkerVersionV10`: HTML sanitization → paragraph split → merge-forward so no short fragment is dropped → sentence-aligned split at `MaxChunkLength`=1000 runes, with CJK-aware sentence-boundary detection (`。！？` as well as `.!?`) → sentence-boundary overlap of ~15% (`DefaultOverlapRatio`=0.15) carried into the next chunk). The chunker version is recorded per document version so a corpus can be queried for mixed states and a rebuild can tell "already current" from "still stale".
4.  Embeds new chunks (embedding happens *before* opening the DB transaction, so the transaction never holds open across network I/O).
5.  Calculates diffs against the previous version.
6.  Persists chunks and events (`added`, `updated` etc.).
7.  Updates the current version pointer.

#### 2. Retrieve Context (`retrieve_context_usecase.go` → `usecase/retrieval.RetrievalGraph`)

5-stage retrieval pipeline (`usecase/retrieval/` sub-package), each `ContextItem`/`SearchResult` carrying a `ScoreKind` (`vector`/`bm25`/`rrf`/`rerank`) so a later stage never mistakes a ranking signal for a calibrated score:
1.  **Query Expansion** (`expand_queries.go`): Translates and expands the query using an LLM, plus tag-query extraction; skipped when the query planner already supplied `SearchQueries`.
2.  **Embed & Search** (`embed_and_search.go`): Embeds queries and performs vector search. Runs BM25 (Meilisearch) or in-DB hybrid search in parallel when `HYBRID_SEARCH_ENABLED=true`.
3.  **Fusion** (`fuse_results.go`): Merges vector and BM25/hybrid results using Reciprocal Rank Fusion (RRF, k=`RAG_RRF_K`, default 60). A hit's identity is its chunk id, or its article id when the source is BM25 (Meilisearch indexes articles, not chunks, so every BM25 hit would otherwise share `uuid.Nil`).
4.  **Rerank** (`rerank.go`): Cross-encoder reranking. Up to `RERANK_MAX_CANDIDATES` (default 40) fused hits are sent to the cross-encoder, and the top `RERANK_TOP_K` (default 10) survive — the input cap is deliberately larger than the output cap, since capping the input at `RERANK_TOP_K` would make a hit ranked 11th by retrieval unpromotable. Reranking is a degrade-safe enhancement tier: a failed/unwired/empty-scoring reranker is logged at error level and the pipeline falls back to retrieval order rather than failing the request.
5.  **Allocate** (`allocate.go`): Language-aware allocation with dynamic score-based mode.
6.  Returns context items with metadata, plus the pre-rerank order and whether reranking was actually applied (for offline eval and debug telemetry).

**Retrieval quality gate**: `RelevanceGate` (`relevance_gate.go`) reads the top-1 context's `ScoreKind`. Only a `rerank`-scored top hit is judged against the gate's thresholds (hardcoded 0.5/0.25 — see "Retrieval Quality Gate" below, `RAG_QUALITY_THRESHOLD_GOOD`/`_MARGINAL` do not configure this gate); a vector/BM25/RRF score (rerank skipped, failed, or embedder-down BM25-only mode) always reads as `Marginal`, since those scales carry no calibrated meaning. On the shipped default path, `Marginal` proceeds to generation with no retry, and `Insufficient` returns a fallback response (`fallback_code=relevance_low`) rather than degrading to a disclaimer — the retry/disclaimer behavior exists only in the legacy, currently-unreachable `buildPrompt` branch (see "Retrieval Quality Gate" below).

#### 3. Answer with RAG (`answer_with_rag_usecase.go`)

**Generation:**
1.  **Retrieval**: Calls `RetrieveContextUsecase` (via the query planner when wired, else the legacy intent/classifier/`ConversationPlanner` path — see intent-specific `strategy_*.go` files).
2.  **Prompt Building**: Constructs a structured XML-like prompt (per-intent template under `alpha-v2`) containing instructions, the user query, and retrieved context chunks.
3.  **Generation**: Calls `LLMClient.Chat` (or `ChatStream`), enforcing a JSON response format for structure.
4.  **Validation**: Parses and validates the LLM's JSON output (e.g., checks citations).
5.  **Output**: Returns the answer, citations, an inline-projected "related citations" snapshot (semantic + lexical neighbors of the direct citations, when a neighbor searcher/encoder is wired), and debug info.
    - Supports caching of answers (LRU, 256 entries, 10min TTL).
    - Supports streaming via SSE, with partial JSON parsing to stream text token-by-token.

**Hybrid long-form streaming (`stream_hybrid_longform.go`)**: detail/synthesis intents stream through a different path than the token-by-token one above. It sends a `progress=drafting` event, runs `ChatStream` through an incremental answer parser and `ParagraphFlusher` to emit paragraph-level provisional (PROVISIONAL) delta events as they complete, and — if the corrective retry described below fires — sends `progress=refining` before re-running generation (no new deltas during the retry itself). The stream's `done.answer` always carries the authoritative final text, which replaces every provisional preview the client rendered.

**Answer length and retry (`alpha-v2`, added 2026-09, [[000987]])**: a production causal answer once came back as a single heading and three bullets (362 runes) because the prompt templates had no length anchor and the news-creator sampling options penalized every repeated token, including EOS. The fix, still in force:
-   Per-intent templates state an explicit length floor (`lengthFloor()`): general/comparison/temporal 600 runes, causal 800 runes, with real-sized few-shot answers instead of `"..."` skeletons the model would otherwise mirror.
-   `deriveAcceptanceProfile` picks an acceptance profile per request: `default` (1 retry, no length floor), `detail` (queries that ask for detail, `IntentTopicDeepDive`, or `SubIntentDetail`: 2 retries, ≥240 runes, ≥2 citations, rejects truncated-JSON recoveries), `synthesis` (`IntentSynthesis`: 2 retries, ≥420 runes, ≥3 citations).
-   A causal-explanation answer under `causalRetryMinRunes`=500 runes always triggers a corrective retry, independent of the acceptance profile.
-   The corrective retry (`buildCorrectiveRetryInput`) raises `MaxTokens` and appends a Japanese instruction asking for a longer, structured answer (worded per intent/profile/attempt), then rebuilds the prompt and regenerates. If the retry cannot be built, generated, or validated, the already-validated original answer is kept rather than turning the request into a fallback.
-   Gemma-family models (the default `gemma4-e4b-12k`) run with `repeat_penalty=1.0` (`ollama_generator.go`) — the news-creator proxy's default 1.15, tuned for summaries, penalizes every repeated heading/particle token but never EOS, so structured Japanese answers used to stop after the first section.

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

#### 5. Agentic Tool Dispatch (`tool_dispatcher.go`, `tool_planner.go`, `agentic_synthesis_strategy.go`, `agent_step.go`)

`ToolDispatcher` selects and executes tools (`internal/adapter/tools/`: article lookup, tag search, tag-cloud explore, related articles, recap search, date-range filter, summarize-for-context) based on intent classification rather than LLM-driven tool choice — a deliberate reliability tradeoff for the small on-box generation models. Each tool call runs under a 5s timeout. `internal/adapter/eino/` adapts this tool set and `LLMClient` to the Eino `ChatModelAgent` interface for the agentic synthesis strategy. When the tool-calling loop itself fails, retrieval falls back to plain (non-agentic) context and the result carries `AgenticDegraded=true` rather than failing the request.

### Backfill CLI (`cmd/backfill`)

A standalone cobra-based CLI for bulk-indexing and full-corpus re-indexing. Built as a separate binary in the Docker image; not exposed as a network service.

**Subcommands:**

| Command | Description |
|---------|-------------|
| `backfill run` | Run the (legacy) HTTP backfill process against the orchestrator's upsert endpoint (resumes from a local cursor file) |
| `backfill status` | Show current cursor position |
| `backfill reset-cursor` | Reset cursor to start from beginning |
| `backfill rebuild enqueue` | Queue one `rag_jobs` row per source article not already at the target chunker/embedder version (idempotent; `--dry-run` reports without writing) |
| `backfill rebuild run` | Drain the rebuild queue with a bounded pool of parallel workers ([[000987]]); safe to restart — state lives in `rag_jobs` and the recorded per-document chunker/embedder versions, not a local file |

**Flags (`run`):**

| Flag | Default | Description |
|------|---------|-------------|
| `--from` | -- | Start date (YYYY-MM-DD) |
| `--to` | today | End date (YYYY-MM-DD) |
| `--concurrency` | `4` | Concurrent requests |
| `--batch-size` | `40` | Articles per batch |
| `--dry-run` | `false` | Preview without processing |
| `--hyper-boost` | `false` | Use local GPU for embedding (starts temporary Ollama container) |
| `--direct` | `false` | Bypass HTTP: index straight into rag-db + embedder (requires `RAG_DB_URL`, `EMBEDDER_URL`) |
| `--cursor-file` | `cursor.json` | Cursor persistence file |

**Flags (`rebuild enqueue` / `rebuild run`):**

| Flag | Default | Description |
|------|---------|-------------|
| `--dry-run` | `false` | (`enqueue`) Scan and report without writing any job |
| `--all` | `false` | (`enqueue`) Queue every article, including ones already at the target |
| `--page-size` | `500` | (`enqueue`) Source rows fetched per keyset page |
| `--workers` | (package default) | (`run`) Documents embedded concurrently |

**Environment (`run`):**
- `DATABASE_URL` (required): PostgreSQL connection string for fetching articles.
- `ORCHESTRATOR_URL` (default `http://localhost:9010`): rag-orchestrator REST endpoint.

**Environment (`rebuild`):**
- `RAG_DB_URL` (required): rag-db connection string.
- `DATABASE_URL` (required): source database (articles) for enqueue.
- `EMBEDDING_MODEL` (required, no default — the model chosen by evaluation).
- `EMBEDDER_URL` or `EMBEDDER_URLS` (comma-separated replicas of the same model).

Hyper-boost mode starts a temporary Ollama container for local GPU embedding and sends an `X-Embedder-URL` header to the orchestrator's upsert endpoint.

### Eval CLI (`cmd/eval`)

A dev/CI-only cobra CLI (not built into the Docker image) that runs the golden-set retrieval/generation evaluation harness (`eval/`) against a live orchestrator: per-stage metrics (Recall@K, nDCG@10, MRR, rerank delta, citation recall), A/B profile comparisons, and diff reports. Golden cases are split between a committed, fully synthetic set (`eval/testdata/golden_cases.json`) and an optional, gitignored, production-derived set generated read-only from rag-db (`golden_cases.local.json`) — committed eval fixtures must never carry real article IDs, titles, or user queries ([[000987]]).

### Connect-RPC (`internal/adapter/connect`)

The Connect-RPC server runs on a separate port (default 9011) and supports HTTP/2 (h2c, or terminated mTLS when `PEER_IDENTITY_MODE=mtls`) for server-streaming RPCs. Inbound peer authentication is handled by `internal/middleware/peer_identity.go`.

**AugurService** (`connect/augur/handler.go`):
- `StreamChat`: Extracts the last user message, streams answer via `AnswerWithRAGUsecase.Stream()`. Events: `delta`, `meta`, `done`, `fallback`, `error`, `thinking`, `progress` (drafting/refining stages of the hybrid long-form path for detail/synthesis intents — see "#### 3. Answer with RAG" above). Sanitizes UTF-8 for protobuf compatibility.
- `RetrieveContext`: Unary RPC wrapping `RetrieveContextUsecase.Execute()`.
- `ListConversations` / `GetConversation` / `DeleteConversation`: Manage the append-only chat history persisted in `augur_conversations`/`augur_messages`.

**MorningLetterService** (`connect/morning_letter/handler.go`):
- `StreamChat`: Fetches recent articles from alt-backend (time-bounded, default 24h, max 7 days), sends a `meta` event with time window info, then streams the RAG answer. Events: `meta`, `delta`, `done`, `fallback`, `error`.

## Known failure patterns

Distilled from postmortems and ADRs; see [[crystallized-knowledge]] §1/§8 for the broader classes.

- **401 cascade from provider auth hardening**: retrieval returned 0 chunks while handlers kept answering 200 and the LLM generated low-quality output → search-indexer started requiring service auth but this consumer was never updated; the 401 was swallowed as a WARN → PM-2026-026 (one day after the identical PM-2026-025 in Acolyte). A consumer without a Pact is not protected by CDC. rag-orchestrator now carries Pact consumer contracts for six providers (alt-data-hub, knowledge-sovereign, news-creator chat, news-creator plan-query, recap-worker, search-indexer) in `internal/adapter/contract/`, closing that specific gap.
- **Model migration without rebuild → immediate EOF**: Ask Augur completely down for 67 min → the Gemma4 migration renamed the model but rag-orchestrator was not rebuilt and kept sending the stale name → immediate EOF is the "model not found" signal; rebuild every container that references the model name via env → PM-2026-016.
- **BM25 always returning 0 hits**: hybrid search silently degraded to vector-only → the `user_id="rag-orchestrator-system"` filter matched zero articles → question search-filter assumptions first; downstream ranking improvements are void while upstream retrieval is broken → [[000643]] [[000648]].
- **Embedder is a SPOF without a degraded mode**: Ask Augur fully down while BM25 was healthy → embedding failure was treated as fatal → embedding failures are non-fatal; BM25-only degraded mode with the `degraded_mode: bm25_only` log tag → PM-2026-020, [[000693]]. Failure boundary: original-query embedding fatal, expanded-query embedding non-fatal → [[000519]].
- **Raw HTML chunk contamination**: Ask Augur could not find articles that clearly existed → fulltext-fetch raw HTML passed the plain-text chunker, so 26% of all embeddings encoded site structure → DOM sanitizer + `ChunkerVersion` in the idempotency key so backfill auto-reindexes → PM-2026-018, [[000619]].
- **Invalid UTF-8 hangs the stream**: `done` event never delivered, UI loads forever → citation metadata skipped `sanitizeUTF8()` and protobuf serialization failed (frequent with 3-byte Japanese characters) → sanitize every string field right before protobuf → PM-2026-009.
- **Dead optional dependency = full timeout on every request**: +15s latency per answer with zero quality gain → a dead rerank-server timed out on every call → monitor optional dependencies; size timeouts from measurements → [[000386]].
- **Cross-lingual retrieval fails multiplicatively**: Japanese queries could not reach 372 English articles → three independent gaps (prompt missing an English label, BM25 using only the original query, Meilisearch CJK locale) compounded → per-layer tests are insufficient; test JA→EN end to end → PM-2026-021.
- **Few-shot examples get copied into output**: query expansion contaminated by example domains (AI chips, weather) → small models treat few-shot content as material, not format → abstract `[TOPIC_A]` placeholders + explicit "SAME TOPIC as input" constraint → [[000648]] [[000650]].
