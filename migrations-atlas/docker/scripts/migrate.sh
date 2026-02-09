#!/bin/sh
# Atlas Migration Script for Alt RSS Reader
# Kubernetes-native database migration management

set -euo pipefail

# Configuration
DATABASE_URL="${DATABASE_URL:-}"
MIGRATION_DIR="${MIGRATION_DIR:-/migrations}"
ATLAS_CONFIG="${ATLAS_CONFIG:-/migrations/atlas.hcl}"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Logging functions
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

# Validation
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

# Test database connectivity
test_connection() {
    log_info "Testing database connectivity..."

    # Extract connection details for pg client test
    DB_HOST=$(echo "$DATABASE_URL" | sed -n 's/.*@\([^:]*\):.*/\1/p')
    DB_PORT=$(echo "$DATABASE_URL" | sed -n 's/.*:\([0-9]*\)\/.*/\1/p')
    DB_NAME=$(echo "$DATABASE_URL" | sed -n 's/.*\/\([^?]*\).*/\1/p')
    DB_USER=$(echo "$DATABASE_URL" | sed -n 's/.*:\/\/\([^:]*\):.*/\1/p')

    # Test basic connectivity with timeout
    if timeout 30 pg_isready -h "$DB_HOST" -p "$DB_PORT" -U "$DB_USER" -d "$DB_NAME"; then
        log_success "Database connectivity verified"
    else
        log_error "Cannot connect to database"
        exit 1
    fi
}

# Detect an already populated schema and register a baseline revision.
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

# Ensure atlas.sum exists before running commands that require it.
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

# Migration status
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

# Initialize migration tracking
init_migrations() {
    log_info "Initializing Atlas migration tracking..."

    atlas migrate hash \
        --dir "file://$MIGRATION_DIR" || {
            log_error "Failed to initialize migration tracking"
            exit 1
        }

    log_success "Migration tracking initialized"
}

# Apply migrations
apply_migrations() {
    log_info "Applying database migrations..."

    # Dry run first for safety
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

    # Apply actual migrations
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

# Validate migrations
validate_migrations() {
    log_info "Validating migration files..."

    # Ensure hash file exists or regenerate it
    ensure_hash_file

    atlas migrate validate \
        --dir "file://$MIGRATION_DIR" || {
            log_error "Migration validation failed"
            exit 1
        }

    log_success "All migrations validated successfully"
}

# Syntax check migrations (offline)
syntax_check_migrations() {
    log_info "Checking migration syntax (offline)..."

    # First try to generate hash file
    atlas migrate hash \
        --dir "file://$MIGRATION_DIR" || {
            log_warn "Could not generate hash file, but continuing with syntax check..."
        }

    # Then validate syntax
    atlas migrate validate \
        --dir "file://$MIGRATION_DIR" || {
            log_error "Migration syntax check failed"
            exit 1
        }

    log_success "All migration syntax validated successfully"
}

# Rollback migrations (if supported)
rollback_migrations() {
    local target_version="${1:-}"

    if [ -z "$target_version" ]; then
        log_error "Rollback target version required"
        exit 1
    fi

    log_warn "Rolling back to version: $target_version"

    # Note: Atlas rollback depends on version and configuration
    log_warn "Manual rollback may be required - check Atlas documentation"
}

# Main execution
main() {
    local command="${1:-status}"

    log_info "Atlas Migration Manager for Alt RSS Reader"
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
            echo ""
            echo "Commands:"
            echo "  status       - Check migration status (requires database)"
            echo "  validate     - Validate migrations against database"
            echo "  syntax-check - Check migration syntax offline (no database required)"
            echo "  init         - Initialize migration tracking"
            echo "  apply        - Apply migrations to database"
            echo "  rollback     - Rollback to specified version"
            echo "  help         - Show this help message"
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

# Execute main function with all arguments
main "$@"