# RAG Database & Migrations

_Last reviewed: February 28, 2026_

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
│   └── atlas.sum
```

## rag-db

### Dockerfile

```dockerfile
FROM postgres:18

RUN apt-get update && apt-get install -y \
  git \
  make \
  gcc \
  postgresql-server-dev-18 \
  clang \
  llvm \
  && rm -rf /var/lib/apt/lists/*

RUN git clone https://github.com/pgvector/pgvector.git \
  && cd pgvector \
  && make \
  && make install \
  && cd .. \
  && rm -rf pgvector

RUN apt-get remove -y git make gcc postgresql-server-dev-18 clang llvm \
  && apt-get autoremove -y
```

## rag-migration-atlas

### docker/Dockerfile

```dockerfile
# Atlas Migration Container for RAG DB
FROM arigaio/atlas:latest-alpine

RUN apk add --no-cache postgresql-client

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
    DB_NAME=$(echo "$DATABASE_URL" | sed -n 's/.*\/\([^?]*\).*/\1/p')
    DB_USER=$(echo "$DATABASE_URL" | sed -n 's/.*:\/\/\([^:]*\):.*/\1/p')

    if timeout 30 pg_isready -h "$DB_HOST" -p "$DB_PORT" -U "$DB_USER" -d "$DB_NAME"; then
        log_success "Database connectivity verified"
    else
        log_error "Cannot connect to database"
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

    log_info "Performing dry run..."
    atlas migrate apply \
        --url "$DATABASE_URL" \
        --dir "file://$MIGRATION_DIR" \
        --revisions-schema "${ATLAS_REVISIONS_SCHEMA:-public}" \
        --dry-run || {
            log_error "Dry run failed"
            exit 1
        }

    log_success "Dry run completed successfully"

    log_info "Applying migrations to database..."
    atlas migrate apply \
        --url "$DATABASE_URL" \
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

### migrations/atlas.sum

```text
h1:gwz5BQHngx5i5pPJMWYeOkhTxu6aoZE2caOHlFecrh0=
20251225160000_initial_rag_schema.sql h1:LrMxzPQ9gbRyBCsHxkZau4KoFMtOIIBhnwV6pajshNE=
20251225170000_add_title_url.sql h1:XWHJ8Funs35jRcBt8eq19AHTT24QfQHl4v2Lu3v4UYY=
```
