# RAG Database & Migrations

_Last reviewed: September 5, 2026_

**Location:** `rag-db`, `rag-migration-atlas`

## Directory Structure

```
rag-db/
└── Dockerfile

rag-migration-atlas/
├── docker
│   ├── Dockerfile
│   └── scripts
│       ├── hash.sh
│       └── migrate.sh
├── migrations
│   ├── 20251225160000_initial_rag_schema.sql
│   ├── 20251225170000_add_title_url.sql
│   ├── 20251231120000_optimize_vector_search.sql
│   ├── 20260408120000_add_tsvector_hybrid_search.sql
│   ├── 20260413120000_create_augur_conversations.sql
│   ├── 20260527100000_add_related_citations.sql
│   ├── 20260802120000_widen_embedding_to_bge_m3.sql
│   └── atlas.sum
```

## rag-db

### Dockerfile

Pinned to `postgres:18.6` and pgvector `v0.8.6`, with the exact commit hash checked before build ([[000987]]: postgres 18.6 fixes 28 CVEs relative to the untagged `postgres:18` this used to build from).

```dockerfile
FROM postgres:18.6

# Build tools + pgvector compile + purge must share one RUN so the
# removed packages never land in a persisted layer (image-size).
RUN apt-get update && apt-get install -y --no-install-recommends \
  ca-certificates \
  git \
  make \
  gcc \
  postgresql-server-dev-18 \
  clang \
  llvm \
  && git clone --branch v0.8.6 --depth 1 https://github.com/pgvector/pgvector.git \
  && cd pgvector \
  && [ "$(git rev-parse HEAD)" = "8ee86c96f0fd72390f890aa8a336fda6d3ab4c6c" ] \
  && make \
  && make install \
  && cd .. \
  && rm -rf pgvector \
  && apt-get remove -y git make gcc postgresql-server-dev-18 clang llvm \
  && apt-get autoremove -y \
  && rm -rf /var/lib/apt/lists/*
```

### PostgreSQL configuration tuning (`docker/postgres/postgresql-rag.conf`, mounted via `compose/rag.yaml`)

Tuned for the vector workload against the container's `mem_limit: 4096m` / `shm_size: 2g` ([[000987]]):

| Setting | Value | Why |
|---------|-------|-----|
| `shared_buffers` | `1GB` | 25% of the container's memory limit |
| `effective_cache_size` | `3GB` | Planner hint, sized to the cgroup limit rather than host RAM |
| `work_mem` | `32MB` | RRF sorts + the HNSW iterative-scan buffer; concurrency is capped by `DB_MAX_CONNS=20` on the client side |
| `maintenance_work_mem` | `1GB` | HNSW index builds |
| `autovacuum_work_mem` | `128MB` | Each of 3 autovacuum workers would otherwise inherit `maintenance_work_mem` and blow the container limit |
| `hnsw.ef_search` | `100` | The previous default of `40` silently truncated any query whose `LIMIT` exceeded it — retrieval asks for up to 500 |
| `hnsw.iterative_scan` | `relaxed_order` | Keeps scanning past `ef_search` until `LIMIT` is satisfied; callers re-sort by distance anyway |
| `idle_in_transaction_session_timeout` | `30s` | Safe now that the index-upsert path embeds before opening its transaction |

## rag-migration-atlas

### docker/Dockerfile

Comments below are transcribed with the security-auditor finding reference, deploy-run id, and CDN hostname redacted for public-repo hygiene; the migrator-count instruction is preserved verbatim.

```dockerfile
# Atlas Migration Container for RAG DB
# Pinned to a specific Atlas release rather than `latest-alpine` for
# build determinism (supply-chain hardening). Bump in lockstep across
# all 7 migrators when upgrading.
FROM arigaio/atlas:1.2.0-alpine

# Connectivity check uses busybox `nc` (already in arigaio/atlas:*-alpine).
# The previous `apk add postgresql-client` was the sole network dependency
# at build time and could hang for hours on a slow Alpine CDN response.

WORKDIR /migrations

COPY migrations/ ./

RUN mkdir -p /scripts
COPY docker/scripts/migrate.sh /scripts/
COPY docker/scripts/hash.sh /scripts/

RUN chmod +x /scripts/*.sh
RUN chown -R 1001:1001 /migrations /scripts

USER 1001:1001

ENTRYPOINT ["/scripts/migrate.sh"]
CMD ["status"]
```

### docker/scripts/hash.sh

```bash
#!/bin/sh
# Generate atlas.sum for RAG DB migrations

set -euo pipefail

MIGRATION_DIR="${MIGRATION_DIR:-/migrations}"

RED='\033[0;31m'
GREEN='\033[0;32m'
BLUE='\033[0;34m'
NC='\033[0m'

log_info() {
    echo -e "${BLUE}[INFO]${NC} $*"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $*"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $*"
}

main() {
    log_info "Generating atlas.sum for RAG DB migrations"

    if [ ! -d "$MIGRATION_DIR" ]; then
        log_error "Migration directory not found: $MIGRATION_DIR"
        exit 1
    fi

    atlas migrate hash --dir "file://$MIGRATION_DIR"

    log_success "atlas.sum generated"

    if [ -f "$MIGRATION_DIR/atlas.sum" ]; then
        log_info "Last few lines of atlas.sum:"
        tail -n 10 "$MIGRATION_DIR/atlas.sum"
    fi
}

main "$@"
```

### docker/scripts/migrate.sh

```bash
#!/bin/sh
# Atlas Migration Script for RAG DB

set -euo pipefail

DATABASE_URL="${DATABASE_URL:-}"
MIGRATION_DIR="${MIGRATION_DIR:-/migrations}"
ATLAS_CONFIG="${ATLAS_CONFIG:-/migrations/atlas.hcl}"

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

log_info() {
    echo -e "${BLUE}[INFO]${NC} $*"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $*"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $*"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $*"
}

check_requirements() {
    local require_db="${1:-true}"

    # Construct DATABASE_URL if not provided but components are available
    if [ -z "$DATABASE_URL" ] && [ -n "${DB_HOST:-}" ]; then
        log_info "Constructing DATABASE_URL from environment variables..."

        DB_USER="${DB_USER:-postgres}"
        DB_NAME="${DB_NAME:-postgres}"
        DB_PORT="${DB_PORT:-5432}"

        if [ -n "${DB_PASSWORD_FILE:-}" ] && [ -f "$DB_PASSWORD_FILE" ]; then
            DB_PASSWORD=$(cat "$DB_PASSWORD_FILE")
        else
            DB_PASSWORD="${DB_PASSWORD:-}"
        fi

        # URL encode password if needed (basic check)
        # Note: simplistic encoding, might need python or generic approach if special chars exist.
        # Check if python3 is available in alpine/atlas? Probably not.
        # Assuming simple password for now or user provides DATABASE_URL.

        DATABASE_URL="postgres://$DB_USER:$DB_PASSWORD@$DB_HOST:$DB_PORT/$DB_NAME?sslmode=disable&search_path=public"
        export DATABASE_URL
    fi

    if [ "$require_db" = "true" ] && [ -z "$DATABASE_URL" ]; then
        log_error "DATABASE_URL environment variable is required"
        exit 1
    fi

    if [ ! -d "$MIGRATION_DIR" ]; then
        log_error "Migration directory not found: $MIGRATION_DIR"
        exit 1
    fi

    log_info "Atlas migration requirements validated"
}

test_connection() {
    log_info "Testing database connectivity..."

    DB_HOST=$(echo "$DATABASE_URL" | sed -n 's/.*@\([^:]*\):.*/\1/p')
    DB_PORT=$(echo "$DATABASE_URL" | sed -n 's/.*:\([0-9]*\)\/.*/\1/p')

    # Busybox `nc` ships in arigaio/atlas:*-alpine; no apk install needed
    # at build time. TCP open-port probe is sufficient — Atlas surfaces
    # auth/database-name errors on `migrate apply` itself.
    if nc -z -w 5 "$DB_HOST" "$DB_PORT"; then
        log_success "Database listener reachable at $DB_HOST:$DB_PORT"
    else
        log_error "Cannot reach database listener at $DB_HOST:$DB_PORT"
        exit 1
    fi
}

baseline_existing_schema() {
    local baseline_version="${MIGRATE_BASELINE_VERSION:-}"

    if [ -z "$baseline_version" ]; then
        log_error "Existing database schema detected but MIGRATE_BASELINE_VERSION is not set"
        log_error "See https://atlasgo.io/docs/reference/cli/migrate/baseline for guidance"
        exit 1
    fi

    log_warn "Existing schema detected; applying Atlas baseline to version $baseline_version"

    atlas migrate set "$baseline_version" \
        --url "$DATABASE_URL" \
        --dir "file://$MIGRATION_DIR" \
        --revisions-schema "${ATLAS_REVISIONS_SCHEMA:-public}" || {
            log_error "Atlas baseline failed"
            exit 1
        }

    log_success "Atlas baseline applied at version $baseline_version"
}

ensure_hash_file() {
    if [ ! -f "$MIGRATION_DIR/atlas.sum" ]; then
        log_info "Generating migration checksum file (atlas.sum)..."
        atlas migrate hash \
            --dir "file://$MIGRATION_DIR" || {
                log_error "Failed to generate atlas.sum"
                exit 1
            }
    fi
}

migration_status() {
    log_info "Checking migration status..."

    ensure_hash_file

    if ! status_output=$(atlas migrate status \
        --url "$DATABASE_URL" \
        --dir "file://$MIGRATION_DIR" \
        --revisions-schema "${ATLAS_REVISIONS_SCHEMA:-public}" 2>&1); then
        echo "$status_output"

        if echo "$status_output" | grep -qi "connected database is not clean"; then
            baseline_existing_schema
            log_info "Re-running migration status after baseline..."

            atlas migrate status \
                --url "$DATABASE_URL" \
                --dir "file://$MIGRATION_DIR" \
                --revisions-schema "${ATLAS_REVISIONS_SCHEMA:-public}" || {
                    log_error "Migration status still failing after baseline"
                    exit 1
                }
            return 0
        fi

        log_warn "Migration status check failed, attempting to initialize..."
        return 1
    fi

    echo "$status_output"
}

init_migrations() {
    log_info "Initializing Atlas migration tracking..."

    atlas migrate hash \
        --dir "file://$MIGRATION_DIR" || {
            log_error "Failed to initialize migration tracking"
            exit 1
        }

    log_success "Migration tracking initialized"
}

apply_migrations() {
    log_info "Applying database migrations..."

    # Force PostgreSQL to fail any single SQL statement that cannot
    # acquire its required lock within 30s instead of waiting forever.
    local apply_url="$DATABASE_URL"
    case "$apply_url" in
        *\?*) apply_url="${apply_url}&options=-c%20lock_timeout%3D30000" ;;
        *)    apply_url="${apply_url}?options=-c%20lock_timeout%3D30000" ;;
    esac

    log_info "Performing dry run..."
    atlas migrate apply \
        --url "$apply_url" \
        --dir "file://$MIGRATION_DIR" \
        --revisions-schema "${ATLAS_REVISIONS_SCHEMA:-public}" \
        --dry-run || {
            log_error "Dry run failed"
            exit 1
        }

    log_success "Dry run completed successfully"

    log_info "Applying migrations to database..."
    atlas migrate apply \
        --url "$apply_url" \
        --dir "file://$MIGRATION_DIR" \
        --revisions-schema "${ATLAS_REVISIONS_SCHEMA:-public}" || {
            log_error "Migration apply failed"
            exit 1
        }

    log_success "All migrations applied successfully"
}

validate_migrations() {
    log_info "Validating migration files..."

    ensure_hash_file

    atlas migrate validate \
        --dir "file://$MIGRATION_DIR" || {
            log_error "Migration validation failed"
            exit 1
        }

    log_success "All migrations validated successfully"
}

syntax_check_migrations() {
    log_info "Checking migration syntax (offline)..."

    atlas migrate hash \
        --dir "file://$MIGRATION_DIR" || {
            log_warn "Could not generate hash file, but continuing with syntax check..."
        }

    atlas migrate validate \
        --dir "file://$MIGRATION_DIR" || {
            log_error "Migration syntax check failed"
            exit 1
        }

    log_success "All migration syntax validated successfully"
}

rollback_migrations() {
    local target_version="${1:-}"

    if [ -z "$target_version" ]; then
        log_error "Rollback target version required"
        exit 1
    fi

    log_warn "Rolling back to version: $target_version"
    log_warn "Manual rollback may be required - check Atlas documentation"
}

main() {
    local command="${1:-status}"

    log_info "Atlas Migration Manager for RAG DB"
    log_info "Command: $command"

    case "$command" in
        "status")
            check_requirements
            test_connection
            migration_status
            ;;
        "validate")
            check_requirements
            test_connection
            validate_migrations
            ;;
        "syntax-check")
            check_requirements false
            syntax_check_migrations
            ;;
        "init")
            check_requirements
            test_connection
            init_migrations
            ;;
        "apply")
            check_requirements
            test_connection
            validate_migrations
            apply_migrations
            ;;
        "rollback")
            check_requirements
            test_connection
            rollback_migrations "${2:-}"
            ;;
        "help")
            echo "Usage: $0 {status|validate|syntax-check|init|apply|rollback <version>|help}"
            exit 0
            ;;
        *)
            log_error "Unknown command: $command"
            echo "Usage: $0 {status|validate|syntax-check|init|apply|rollback <version>|help}"
            exit 1
            ;;
    esac

    log_success "Migration command completed: $command"
}

main "$@"
```

### migrations/20251225160000_initial_rag_schema.sql

```sql
-- Enable pgvector extension
CREATE EXTENSION IF NOT EXISTS vector;

-- Use UUID for IDs
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- rag_documents: Manages the current version of a document (article)
CREATE TABLE rag_documents (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    article_id TEXT NOT NULL UNIQUE, -- Reference to alt-backend article_id
    current_version_id UUID, -- Will be FK to rag_document_versions, nullable initially
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- rag_document_versions: Immutable versions of a document
CREATE TABLE rag_document_versions (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    document_id UUID NOT NULL REFERENCES rag_documents(id) ON DELETE CASCADE,
    version_number INTEGER NOT NULL,
    source_hash TEXT NOT NULL,
    chunker_version TEXT NOT NULL,
    embedder_version TEXT NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,

    UNIQUE(document_id, version_number)
);

-- Add the cyclic FK constraint after table creation
ALTER TABLE rag_documents
ADD CONSTRAINT fk_current_version
FOREIGN KEY (current_version_id)
REFERENCES rag_document_versions(id) DEFERRABLE INITIALLY DEFERRED;

-- rag_chunks: Stores the actual text and embeddings for a version
CREATE TABLE rag_chunks (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    version_id UUID NOT NULL REFERENCES rag_document_versions(id) ON DELETE CASCADE,
    ordinal INTEGER NOT NULL,
    content TEXT NOT NULL,
    embedding vector(768), -- Embedding vector
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,

    UNIQUE(version_id, ordinal)
);

-- HNSW Index for vector search
CREATE INDEX rag_chunks_embedding_idx ON rag_chunks USING hnsw (embedding vector_cosine_ops);

-- rag_chunk_events: Tracks changes between versions (added, updated, deleted, unchanged)
-- This provides the "audit trail" or "diff" explanations.
CREATE TYPE chunk_event_type AS ENUM ('added', 'updated', 'deleted', 'unchanged');

CREATE TABLE rag_chunk_events (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    version_id UUID NOT NULL REFERENCES rag_document_versions(id) ON DELETE CASCADE,
    chunk_id UUID REFERENCES rag_chunks(id), -- Nullable for deleted events if we don't keep the old chunk ref, or if we want to link to specific chunk
    ordinal INTEGER, -- The ordinal in the CURRENT version (for added/updated/unchanged) or PREVIOUS version (for deleted??). Let's assume ordinal in this version.
    event_type chunk_event_type NOT NULL,
    metadata JSONB, -- simplified, can store DEBUG info
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- rag_jobs: Queue for background tasks like backfilling, indexing
CREATE TYPE job_status AS ENUM ('new', 'processing', 'completed', 'failed');

CREATE TABLE rag_jobs (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    job_type TEXT NOT NULL,
    payload JSONB NOT NULL,
    status job_status NOT NULL DEFAULT 'new',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    error_message TEXT
);

-- Index for queue polling
CREATE INDEX rag_jobs_status_created_at_idx ON rag_jobs (status, created_at);
```

### migrations/20251225170000_add_title_url.sql

```sql
-- Add title and url to rag_document_versions
ALTER TABLE rag_document_versions
ADD COLUMN title TEXT,
ADD COLUMN url TEXT;
```

### migrations/20251231120000_optimize_vector_search.sql

```sql
-- Optimize Vector Search Performance
-- This migration adds indexes to improve the performance of the two-stage vector search.

-- Index on current_version_id for faster JOIN in Stage 2
-- This helps when filtering chunks by current version
CREATE INDEX IF NOT EXISTS idx_rag_documents_current_version
ON rag_documents(current_version_id);

-- Index on version_id for faster chunk lookups by version
-- This helps in Stage 2 when enriching chunk data
CREATE INDEX IF NOT EXISTS idx_rag_chunks_version_id
ON rag_chunks(version_id);

-- Composite index on document_id and version_id for rag_document_versions
-- This helps speed up the JOIN between versions and documents
CREATE INDEX IF NOT EXISTS idx_rag_document_versions_doc_id
ON rag_document_versions(document_id);
```

### migrations/20260408120000_add_tsvector_hybrid_search.sql

```sql
-- Add tsvector column for in-database BM25-style search (hybrid search with pgvector).
-- Uses a generated column so existing and future chunks get tsvectors automatically.
-- 'english' config for stemmed English, 'simple' config for CJK passthrough.

ALTER TABLE rag_chunks ADD COLUMN tsv tsvector
  GENERATED ALWAYS AS (
    to_tsvector('english', content) || to_tsvector('simple', content)
  ) STORED;

-- GIN index for efficient full-text search
CREATE INDEX idx_rag_chunks_tsv ON rag_chunks USING GIN (tsv);
```

This is the storage backing `HYBRID_BM25_SOURCE=postgres` in rag-orchestrator's in-DB hybrid search path (`hybrid_search_repo.go`); the default `HYBRID_BM25_SOURCE=meilisearch` does not use it.

### migrations/20260413120000_create_augur_conversations.sql

Append-first chat history for Ask Augur. `augur_conversations` is write-once at creation (no `updated_at`, no denormalized counters); `augur_messages` is append-only (INSERT + CASCADE DELETE on conversation removal only). `augur_conversation_index` is a disposable read projection (last activity, message count, preview) derived from `augur_messages` at query time rather than stored.

```sql
CREATE TABLE augur_conversations (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL,
    title TEXT NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_augur_conversations_user_created
    ON augur_conversations (user_id, created_at DESC);

CREATE TABLE augur_messages (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    conversation_id UUID NOT NULL REFERENCES augur_conversations(id) ON DELETE CASCADE,
    role TEXT NOT NULL CHECK (role IN ('user', 'assistant')),
    content TEXT NOT NULL,
    citations JSONB NOT NULL DEFAULT '[]'::jsonb,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_augur_messages_conversation_created
    ON augur_messages (conversation_id, created_at);

CREATE VIEW augur_conversation_index AS
SELECT
    c.id,
    c.user_id,
    c.title,
    c.created_at,
    COALESCE(m.last_activity_at, c.created_at) AS last_activity_at,
    COALESCE(m.message_count, 0)               AS message_count,
    m.last_message_preview
FROM augur_conversations c
LEFT JOIN LATERAL (
    SELECT
        MAX(created_at) AS last_activity_at,
        COUNT(*)::int   AS message_count,
        (SELECT LEFT(content, 140)
            FROM augur_messages
            WHERE conversation_id = c.id
            ORDER BY created_at DESC
            LIMIT 1) AS last_message_preview
    FROM augur_messages
    WHERE conversation_id = c.id
) m ON TRUE;
```

### migrations/20260527100000_add_related_citations.sql

```sql
-- Add related_citations JSONB column to augur_messages.
--
-- Captures an inline-projected snapshot of articles semantically and
-- lexically near the direct citations at the moment an assistant turn is
-- materialized. Written on the same INSERT as citations and never updated
-- afterwards (append-only). Legacy rows read back with the default empty
-- array, which the UI renders as no "Related" section.
ALTER TABLE augur_messages
    ADD COLUMN related_citations JSONB NOT NULL DEFAULT '[]'::jsonb;
```

### migrations/20260802120000_widen_embedding_to_bge_m3.sql

Widens `rag_chunks.embedding` from `vector(768)` (`embeddinggemma`) to `vector(1024)` (`bge-m3`, the current `EMBEDDING_MODEL` — see `docs/services/rag-orchestrator.md`). This is the migration referenced by the "embedding dimension change needs a separate ADR" caution in `docs/wiki/services/rag-db.md`.

```sql
-- The stored vectors are discarded rather than converted: a 768-dim
-- embeddinggemma vector has no meaning in bge-m3's space, so there is no cast
-- that preserves it. rag_chunks is a disposable projection — the embeddings
-- are rebuilt from article text by the backfill/rebuild CLI; the append-only
-- rag_document_versions / rag_chunk_events history is untouched.
--
-- The HNSW index is dropped first because pgvector cannot alter the dimension
-- of an indexed vector column; rebuilding it after the rewrite is also the
-- documented bulk-load order.

DROP INDEX IF EXISTS rag_chunks_embedding_idx;

ALTER TABLE rag_chunks
    ALTER COLUMN embedding TYPE vector(1024) USING NULL::vector(1024);

CREATE INDEX rag_chunks_embedding_idx ON rag_chunks USING hnsw (embedding vector_cosine_ops);
```

### migrations/atlas.sum

```text
h1:7s6UI52E3zVnfFYOAc2lf2HmvlirwuN69bfEuXylhno=
20251225160000_initial_rag_schema.sql h1:LrMxzPQ9gbRyBCsHxkZau4KoFMtOIIBhnwV6pajshNE=
20251225170000_add_title_url.sql h1:XWHJ8Funs35jRcBt8eq19AHTT24QfQHl4v2Lu3v4UYY=
20251231120000_optimize_vector_search.sql h1:mb0LXo2obvfYGikZkqReN9bM9ESTAzbfi3U6Fhkc4DQ=
20260408120000_add_tsvector_hybrid_search.sql h1:BSKinuUgh+Vpk2pgIEi+zksjwzN7Ega5GOku3yiU5pA=
20260413120000_create_augur_conversations.sql h1:p/19BYOVBZ3C1gF0kRxB6Az53fkClkWikCM3KlJmhWo=
20260527100000_add_related_citations.sql h1:auNL7D81gsoWNiYnSS8VszPYKu4036v34Nt74dZaYF4=
20260802120000_widen_embedding_to_bge_m3.sql h1:9QB8iIsDKdM1GX+i3MsEkywFKAULnEpfmPZznr8OgVA=
```

## Known failure patterns

Distilled from the ADR / postmortem corpus (see `docs/runbooks/crystallized-knowledge.md`).

- **HNSW indexes make INSERT ~5x slower** — bulk loads into `rag_chunks` should DROP INDEX → INSERT → REINDEX, with raised `maintenance_work_mem`, parallel workers, and an expanded container `shm_size` ([[000620]]).
- **pgvector type registration is not automatic** — `pgxpool.New` does not register the `vector` type; connections must go through `infra.NewPostgresDB`, or vector params fail at runtime ([[000620]]).
- **HNSW + post-filtering returns 0 rows when the candidate set is small** — time-bounded searches must pre-filter in a single-pass query; keep HNSW for the two-stage full-text path ([[000053]], [[000036]]).
- **Missing `shm_size` kills PostgreSQL under load** — Docker's 64 MB default collides with work_mem dynamic shared memory (SQLSTATE 53100); set it explicitly ([[000521]]).
- **Soft-deleted articles leak into RAG unless every upstream query filters them** — `deleted_at IS NULL` must be applied exhaustively when reading article sources ([[000282]]).
- **The embedder is a SPOF unless a degraded mode exists** — embedding failure treated as fatal took retrieval fully down even though BM25 was healthy; design a BM25-only degraded mode (PM-2026-020, [[000693]]).
