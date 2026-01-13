# RAG Orchestrator

_Last reviewed: January 13, 2026_

**Location:** `rag-orchestrator`

The `rag-orchestrator` is a Go 1.25+ service responsible for managing the RAG (Retrieval Augmented Generation) pipeline. It handles article indexing, vector embedding, context retrieval, and answer generation using an LLM.

## Directory Structure

```
rag-orchestrator/
├── Dockerfile
├── Makefile
├── go.mod
├── cmd
│   ├── backfill-cli
│   │   └── main.go
│   └── server
│       └── main.go
├── internal
│   ├── adapter
│   │   ├── http
│   │   │   └── openapi
│   │   │       └── server.gen.go
│   │   ├── rag_augur
│   │   │   ├── ollama_embedder.go
│   │   │   └── ollama_generator.go
│   │   ├── rag_http
│   │   │   ├── handler.go
│   │   │   └── search_indexer_client.go
│   │   └── repository
│   │       ├── postgres_tx.go
│   │       ├── rag_chunk_repo.go
│   │       ├── rag_document_repo.go
│   │       └── rag_job_repo.go
│   ├── domain
│   │   ├── chunker.go
│   │   ├── diff_chunks.go
│   │   ├── llm_client.go
│   │   ├── repository.go
│   │   ├── search_client.go
│   │   ├── source_hash_policy.go
│   │   └── vector_encoder.go
│   ├── infra
│   │   ├── config
│   │   │   └── config.go
│   │   ├── logger
│   │   │   └── logger.go
│   │   └── postgres.go
│   └── usecase
│       ├── answer_with_rag_usecase.go
│       ├── index_article_usecase.go
│       ├── output_validator.go
│       ├── prompt_builder.go
│       └── retrieve_context_usecase.go
└── spec
    └── openapi.yaml
```

## Core Infrastructure

### Dockerfile

Builds a minimal alpine-based image for the Go application.

```dockerfile
# Build Stage
FROM golang:1.25-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -o rag-orchestrator ./cmd/server

# Runtime Stage
FROM alpine:latest
WORKDIR /app
COPY --from=builder /app/rag-orchestrator .
CMD ["./rag-orchestrator"]
```

### Configuration (`internal/infra/config/config.go`)

Loads configuration from environment variables.

| Environment Variable | Description | Default |
|----------------------|-------------|---------|
| `PORT` | HTTP Server Port | `8080` |
| `DB_HOST` | Database Host | `localhost` |
| `DB_PORT` | Database Port | `5432` |
| `DB_USER` | Database User | `rag_user` |
| `DB_PASSWORD` | Database Password | `raw_password` (or via file) |
| `DB_NAME` | Database Name | `rag_db` |
| `OLLAMA_BASE_URL` | Ollama API URL | `http://localhost:11434` |
| `EMBEDDING_MODEL` | Model for embeddings | `embeddinggemma` |
| `GENERATION_MODEL` | LLM for generation | `gpt-oss:20b` |
| `RAG_MAX_CHUNKS` | Max contexts to retrieve | `10` |
| `RAG_MAX_TOKENS` | Max generation tokens | `512` |

### API Specification (`spec/openapi.yaml`)

Defines the REST API endpoints:

- `POST /internal/rag/index/upsert`: Indempotently indexes an article.
- `POST /internal/rag/index/delete`: Soft-deletes an article.
- `POST /v1/rag/retrieve`: Retrieves relevant context chunks for a query.
- `POST /v1/rag/answer`: Generates an answer using RAG.
- `POST /v1/rag/answer/stream`: Streams a generated answer via SSE.

## Logic and Implementation

### Domain Layer (`internal/domain`)

Defines the core entities and interfaces:

- **Entities**: `RagDocument`, `RagDocumentVersion`, `RagChunk`, `RagChunkEvent`, `RagJob`.
- **Interfaces**:
    - `RagDocumentRepository`, `RagChunkRepository`: Persistence.
    - `VectorEncoder`: Embedding generation.
    - `LLMClient`: Chat generation.
    - `Chunker`: Text splitting logic.
    - `SourceHashPolicy`: Idempotency hashing.

- **Utilities**:
    - `DiffChunks`: Logic to compute `added`, `updated`, `deleted`, `unchanged` diffs between chunk versions.

### Adapter Layer (`internal/adapter`)

- **Repositories**: `postgres_tx.go`, `rag_chunk_repo.go`, `rag_document_repo.go` implement persistence using `pgx` and `pgvector`.
- **RAG Augur**:
    - `ollama_embedder.go`: Calls Ollama `/api/embed`.
    - `ollama_generator.go`: Calls Ollama `/api/chat` (supports streaming).
- **HTTP Handlers**: `rag_http/handler.go` implements `ServerInterface` from generated OpenAPI code.
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

1.  Translates query to English if Japanese.
2.  Expands query using tags (via `search-indexer` client).
3.  Embeds the queries.
4.  Performs vector search on `rag_chunks` tables.
5.  Merges and deduplicates results.
6.  Returns context items with metadata.

#### 3. Answer with RAG (`answer_with_rag_usecase.go`)

**Single-Phase Generation:**
1.  **Retrieval**: Calls `RetrieveContextUsecase`.
2.  **Prompt Building**: Constructs a structured XML-like prompt containing instructions, the user query, and retrieved context chunks.
3.  **Generation**: Calls `LLMClient.Chat` (or `ChatStream`).
    - Enforces a JSON response format for structure.
4.  **Validation**: Parses and validates the LLM's JSON output (e.g., checks citations).
5.  **Output**: Returns the answer, citations, and debug info.
    - Supports caching of answers.
    - Supports streaming via SSE, with partial JSON parsing to stream text token-by-token.
