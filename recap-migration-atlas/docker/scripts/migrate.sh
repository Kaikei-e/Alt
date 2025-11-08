#!/bin/sh
# Atlas Migration Script for Recap DB

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

    log_info "Atlas Migration Manager for Recap DB"
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
