#!/bin/bash
#
# Alt Platform Master Backup Script
# Performs comprehensive backup of all data stores using Restic
#
# Usage:
#   ./backup-all.sh [options]
#
# Options:
#   --init           Initialize Restic repository (first run only)
#   --pg-only        Only backup PostgreSQL databases
#   --volumes-only   Only backup Docker volumes
#   --prune          Prune old snapshots after backup
#   --verify         Verify backup integrity after completion
#   --dry-run        Show what would be backed up without executing
#   -h, --help       Show this help message
#
# Environment Variables:
#   RESTIC_REPOSITORY    Path to Restic repository (default: /backups/restic-repo)
#   RESTIC_PASSWORD_FILE Path to password file (default: /run/secrets/restic_password)
#   HEALTHCHECK_URL      Healthchecks.io ping URL (optional)
#   BACKUP_DIR           Base backup directory (default: /backups)
#

set -euo pipefail

# Configuration
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
TIMESTAMP=$(date +%Y%m%d_%H%M%S)
LOG_FILE="/backups/logs/backup-${TIMESTAMP}.log"

# Defaults
RESTIC_REPOSITORY="${RESTIC_REPOSITORY:-/backups/restic-repo}"
RESTIC_PASSWORD_FILE="${RESTIC_PASSWORD_FILE:-/run/secrets/restic_password}"
BACKUP_DIR="${BACKUP_DIR:-/backups}"
POSTGRES_BACKUP_DIR="${BACKUP_DIR}/postgres"
COMPOSE_FILE="${PROJECT_ROOT}/compose/compose.yaml"
COMPOSE_PROJECT="alt"
BACKUP_CONTAINER="alt-backup"

# Detect if running inside the backup container
IN_CONTAINER="${IN_CONTAINER:-}"
if [[ -z "$IN_CONTAINER" && -f "/.dockerenv" ]]; then
    IN_CONTAINER=1
fi

# Flags
INIT_REPO=false
PG_ONLY=false
VOLUMES_ONLY=false
PRUNE=false
VERIFY=false
DRY_RUN=false

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Logging functions
log() {
    local level="$1"
    shift
    local message="$*"
    local timestamp
    timestamp=$(date '+%Y-%m-%d %H:%M:%S')
    echo -e "${timestamp} [${level}] ${message}" | tee -a "$LOG_FILE"
}

log_info() { log "INFO" "$*"; }
log_warn() { log "${YELLOW}WARN${NC}" "$*"; }
log_error() { log "${RED}ERROR${NC}" "$*"; }
log_success() { log "${GREEN}SUCCESS${NC}" "$*"; }

# Helper to run restic (directly if in container, via docker exec if on host)
run_restic() {
    if [[ -n "$IN_CONTAINER" ]]; then
        restic "$@"
    else
        docker exec "$BACKUP_CONTAINER" restic "$@"
    fi
}

# Healthcheck ping
ping_healthcheck() {
    local status="$1"
    local url="${HEALTHCHECK_URL:-}"

    if [[ -n "$url" ]]; then
        case "$status" in
            start)
                curl -fsS -m 10 --retry 5 "${url}/start" >/dev/null 2>&1 || true
                ;;
            success)
                curl -fsS -m 10 --retry 5 "${url}" >/dev/null 2>&1 || true
                ;;
            fail)
                curl -fsS -m 10 --retry 5 "${url}/fail" >/dev/null 2>&1 || true
                ;;
        esac
    fi
}

# Show help
show_help() {
    head -30 "$0" | tail -27 | sed 's/^# //' | sed 's/^#//'
}

# Parse arguments
parse_args() {
    while [[ $# -gt 0 ]]; do
        case $1 in
            --init)
                INIT_REPO=true
                shift
                ;;
            --pg-only)
                PG_ONLY=true
                shift
                ;;
            --volumes-only)
                VOLUMES_ONLY=true
                shift
                ;;
            --prune)
                PRUNE=true
                shift
                ;;
            --verify)
                VERIFY=true
                shift
                ;;
            --dry-run)
                DRY_RUN=true
                shift
                ;;
            -h|--help)
                show_help
                exit 0
                ;;
            *)
                log_error "Unknown option: $1"
                show_help
                exit 1
                ;;
        esac
    done
}

# Verify prerequisites
verify_prerequisites() {
    log_info "Verifying prerequisites..."

    # Check for docker (needed for pg_dump via docker exec into DB containers)
    if ! command -v docker &>/dev/null; then
        log_error "docker not found. Please install docker first."
        exit 1
    fi

    # When running on host, ensure backup container is running for restic
    if [[ -z "$IN_CONTAINER" ]]; then
        if ! docker ps --format '{{.Names}}' | grep -q "^${BACKUP_CONTAINER}$"; then
            log_warn "Backup container ($BACKUP_CONTAINER) is not running. Attempting to start it..."

            local compose_cmd="docker compose"
            if ! docker compose version &>/dev/null 2>&1; then
                if command -v docker-compose &>/dev/null; then
                    compose_cmd="docker-compose"
                fi
            fi

            if $compose_cmd -f "$COMPOSE_FILE" -p "$COMPOSE_PROJECT" --profile backup up -d restic-backup; then
                log_success "Started backup container."
                sleep 5
            else
                log_error "Failed to start backup container. Please start it manually: docker compose --profile backup up -d restic-backup"
                exit 1
            fi
        fi
    fi

    # Create directories
    mkdir -p "$POSTGRES_BACKUP_DIR"
    mkdir -p "$(dirname "$LOG_FILE")"

    # Ensure repo dir exists if it's local
    if [[ "$RESTIC_REPOSITORY" == /backups/* ]]; then
        mkdir -p "$RESTIC_REPOSITORY" || true
    fi

    log_success "Prerequisites verified"
}

# Initialize Restic repository
init_repo() {
    if [[ "$INIT_REPO" == true ]]; then
        log_info "Initializing Restic repository at $RESTIC_REPOSITORY..."

        if run_restic -r "$RESTIC_REPOSITORY" cat config >/dev/null 2>&1; then
            log_warn "Repository already initialized"
        else
            run_restic -r "$RESTIC_REPOSITORY" init
            log_success "Repository initialized"
        fi
    fi
}

# Backup PostgreSQL databases
backup_postgres() {
    log_info "Starting PostgreSQL backups..."

    # Database configurations: name:container:user:dbname
    # Updated to match running containers and actual users/dbnames
    local databases=(
        "alt-db:alt-db:alt_db_user:alt"
        "kratos-db:alt-kratos-db-1:kratos_user:kratos"
        "recap-db:recap-db:recap_user:recap"
        "rag-db:rag-db:rag_user:rag_db"
        "pact-db:alt-pact-db-1:pact:pact"
    )

    for db_config in "${databases[@]}"; do
        IFS=':' read -r name container user dbname <<< "$db_config"
        local backup_file="${POSTGRES_BACKUP_DIR}/${name}-${TIMESTAMP}.dump"

        log_info "Backing up $name ($dbname)..."

        if [[ "$DRY_RUN" == true ]]; then
            log_info "[DRY-RUN] Would backup $name to $backup_file"
            continue
        fi

        # Check if container is running
        if ! docker ps --format '{{.Names}}' | grep -q "^${container}$"; then
            log_warn "Container $container not running, skipping $name"
            continue
        fi

        # Perform pg_dump with custom format
        # Note: writing to host filesystem
        if docker exec "$container" pg_dump \
            -U "$user" \
            --format=custom \
            --compress=6 \
            --verbose \
            "$dbname" > "$backup_file" 2>> "$LOG_FILE"; then

            local size
            size=$(ls -lh "$backup_file" | awk '{print $5}')
            log_success "Backed up $name: $backup_file ($size)"
        else
            log_error "Failed to backup $name"
        fi
    done

    # Cleanup old PostgreSQL backups (per-database retention)
    log_info "Cleaning up old PostgreSQL backups..."
    local default_retention="${PG_RETENTION_DAYS:-7}"
    local recap_retention="${RECAP_DB_RETENTION_DAYS:-1}"
    for db_config in "${databases[@]}"; do
        IFS=':' read -r name _ _ _ <<< "$db_config"
        local retention="$default_retention"
        if [[ "$name" == "recap-db" ]]; then
            retention="$recap_retention"
        fi
        find "$POSTGRES_BACKUP_DIR" -name "${name}-*.dump" -mtime +"$retention" -delete 2>/dev/null || true
    done
}

# Create Meilisearch snapshot
backup_meilisearch() {
    log_info "Creating Meilisearch snapshot..."

    if [[ "$DRY_RUN" == true ]]; then
        log_info "[DRY-RUN] Would create Meilisearch snapshot"
        return
    fi

    # Check if Meilisearch is running
    local meili_container="alt-meilisearch-1"
    if ! docker ps --format '{{.Names}}' | grep -q "^${meili_container}$"; then
        if ! docker ps --format '{{.Names}}' | grep -q "^alt-meilisearch$"; then
             log_warn "Meilisearch not running, skipping snapshot"
             return
        else
             meili_container="alt-meilisearch"
        fi
    fi

    # Get master key from secrets
    local meili_key
    if [[ -f "/run/secrets/meili_master_key" ]]; then
        meili_key=$(cat /run/secrets/meili_master_key)
    elif [[ -f "${PROJECT_ROOT}/secrets/meili_master_key.txt" ]]; then
        meili_key=$(cat "${PROJECT_ROOT}/secrets/meili_master_key.txt")
    else
        log_warn "Meilisearch master key not found, skipping snapshot"
        return
    fi

    # Trigger snapshot via API
    local response
    response=$(curl -s -X POST "http://localhost:7700/snapshots" \
        -H "Authorization: Bearer $meili_key" \
        -H "Content-Type: application/json" 2>&1)

    if ! echo "$response" | grep -q "taskUid"; then
        log_warn "Meilisearch snapshot may have failed: $response"
        return
    fi

    local task_uid
    task_uid=$(echo "$response" | jq -r '.taskUid // empty')
    if [[ -z "$task_uid" ]]; then
        log_warn "Could not extract taskUid from response: $response"
        return
    fi

    log_info "Meilisearch snapshot triggered (taskUid: $task_uid). Waiting for completion..."

    local elapsed=0
    local timeout=120
    local poll_interval=5

    while [[ $elapsed -lt $timeout ]]; do
        sleep "$poll_interval"
        elapsed=$((elapsed + poll_interval))

        local task_status
        task_status=$(curl -s "http://localhost:7700/tasks/${task_uid}" \
            -H "Authorization: Bearer $meili_key" 2>&1)

        local status
        status=$(echo "$task_status" | jq -r '.status // empty')

        case "$status" in
            succeeded)
                log_success "Meilisearch snapshot completed successfully (${elapsed}s)"
                return
                ;;
            failed)
                local error_msg
                error_msg=$(echo "$task_status" | jq -r '.error.message // "unknown error"')
                log_error "Meilisearch snapshot failed: $error_msg"
                return
                ;;
            *)
                log_info "Meilisearch snapshot status: ${status:-unknown} (${elapsed}s/${timeout}s)"
                ;;
        esac
    done

    log_warn "Meilisearch snapshot timed out after ${timeout}s (task may still be running)"
}

# Backup ClickHouse
backup_clickhouse() {
    log_info "Backing up ClickHouse..."

    if [[ "$DRY_RUN" == true ]]; then
        log_info "[DRY-RUN] Would backup ClickHouse"
        return
    fi

    # Check if ClickHouse is running
    local clickhouse_container="alt-clickhouse-1"
    if ! docker ps --format '{{.Names}}' | grep -q "^${clickhouse_container}$"; then
        if ! docker ps --format '{{.Names}}' | grep -q "^alt-clickhouse$"; then
             log_warn "ClickHouse not running, skipping backup"
             return
        else
             clickhouse_container="alt-clickhouse"
        fi
    fi

    local backup_name="backup_${TIMESTAMP}"

    # Create backup using ClickHouse native BACKUP command
    # Using bash -c to read password from file since --password-file might not be supported
    local ch_user="${CLICKHOUSE_USER:-rask_user}"
    docker exec "$clickhouse_container" bash -c "clickhouse-client -u ${ch_user} --password \"\$(cat /run/secrets/clickhouse_password)\" --query \"BACKUP DATABASE default TO Disk('backups', '${backup_name}')\"" 2>> "$LOG_FILE" || {
        log_warn "ClickHouse native backup failed, will use volume backup"
    }
}


# Backup Docker volumes with Restic
backup_volumes() {
    log_info "Starting Restic volume backup..."

    # Volumes to backup (paths inside backup container)
    local volumes=(
        "/data/db_data_17"
        "/data/kratos_db_data"
        "/data/recap_db_data"
        "/data/rag_db_data"
        "/data/meili_data"
        "/data/clickhouse_data"
        "/data/redis-streams-data"
        "/data/oauth_token_data"
        "/data/prometheus_data"
        "/data/grafana_data"
    )

    # Also backup PostgreSQL dumps
    # This path must be valid inside the container.
    # compose/backup.yaml mounts /backups/postgres:/backups/postgres
    # And POSTGRES_BACKUP_DIR defaults to /backups/postgres
    volumes+=("$POSTGRES_BACKUP_DIR")

    if [[ "$DRY_RUN" == true ]]; then
        log_info "[DRY-RUN] Would backup volumes:"
        printf '%s\n' "${volumes[@]}"
        return
    fi

    # Build exclude patterns
    local exclude_args=(
        --exclude="*.tmp"
        --exclude="*.log"
        --exclude="**/pg_wal/*"
        --exclude="**/pg_replslot/*"
        --exclude="**/pg_stat_tmp/*"
        --exclude="**/tmp_merge_*"
        --exclude="**/tmp_insert_*"
    )

    # Run Restic backup
    log_info "Running Restic backup..."

    local paths_to_backup=()
    for vol in "${volumes[@]}"; do
        # Check if directory exists (directly if in container, via docker exec on host)
        if [[ -n "$IN_CONTAINER" ]]; then
            if [[ -d "$vol" ]]; then
                paths_to_backup+=("$vol")
            else
                log_warn "Volume path not found: $vol"
            fi
        else
            if docker exec "$BACKUP_CONTAINER" test -d "$vol"; then
                paths_to_backup+=("$vol")
            else
                log_warn "Volume path not found in container: $vol"
            fi
        fi
    done

    if [[ ${#paths_to_backup[@]} -eq 0 ]]; then
        log_error "No volumes found to backup"
        return 1
    fi

    run_restic -r "$RESTIC_REPOSITORY" backup \
        --tag "scheduled" \
        --tag "$(date +%Y%m%d)" \
        "${exclude_args[@]}" \
        "${paths_to_backup[@]}" 2>&1 | tee -a "$LOG_FILE"

    log_success "Restic backup completed"
}

# Prune old snapshots
prune_snapshots() {
    if [[ "$PRUNE" != true ]]; then
        return
    fi

    log_info "Pruning old snapshots..."

    if [[ "$DRY_RUN" == true ]]; then
        log_info "[DRY-RUN] Would prune with: --keep-hourly 24 --keep-daily 7 --keep-weekly 4 --keep-monthly 3"
        run_restic -r "$RESTIC_REPOSITORY" forget --dry-run \
            --keep-hourly 24 \
            --keep-daily 7 \
            --keep-weekly 4 \
            --keep-monthly 3
        return
    fi

    run_restic -r "$RESTIC_REPOSITORY" forget \
        --keep-hourly 24 \
        --keep-daily 7 \
        --keep-weekly 4 \
        --keep-monthly 3 \
        --prune 2>&1 | tee -a "$LOG_FILE"

    log_success "Prune completed"
}

# Verify backup integrity
verify_backup() {
    if [[ "$VERIFY" != true ]]; then
        return
    fi

    log_info "Verifying backup integrity..."

    if [[ "$DRY_RUN" == true ]]; then
        log_info "[DRY-RUN] Would run 'restic check'"
        return
    fi

    run_restic -r "$RESTIC_REPOSITORY" check 2>&1 | tee -a "$LOG_FILE"

    log_success "Backup verification completed"
}

# Generate backup metrics for Prometheus
generate_metrics() {
    local metrics_file="/backups/metrics/backup_metrics.prom"
    mkdir -p "$(dirname "$metrics_file")"

    log_info "Generating backup metrics..."

    # Get snapshot stats
    local snapshot_count
    snapshot_count=$(run_restic -r "$RESTIC_REPOSITORY" snapshots --json 2>/dev/null | jq 'length' || echo 0)

    # Get repository stats
    local repo_stats
    repo_stats=$(run_restic -r "$RESTIC_REPOSITORY" stats --json 2>/dev/null || echo '{}')
    local total_size
    total_size=$(echo "$repo_stats" | jq -r '.total_size // 0')

    cat > "$metrics_file" << EOF
# HELP backup_last_success_timestamp Unix timestamp of last successful backup
# TYPE backup_last_success_timestamp gauge
backup_last_success_timestamp{type="full"} $(date +%s)

# HELP backup_restic_snapshot_count Number of Restic snapshots
# TYPE backup_restic_snapshot_count gauge
backup_restic_snapshot_count $snapshot_count

# HELP backup_total_size_bytes Total size of backup repository in bytes
# TYPE backup_total_size_bytes gauge
backup_total_size_bytes $total_size
EOF

    log_success "Metrics written to $metrics_file"
}

# Main execution
main() {
    parse_args "$@"

    log_info "=========================================="
    log_info "Alt Platform Backup - ${TIMESTAMP}"
    log_info "=========================================="

    ping_healthcheck "start"

    local backup_failed=false

    # Run backup steps (disable set -e to handle errors manually)
    set +e

    verify_prerequisites
    if [[ $? -ne 0 ]]; then backup_failed=true; fi

    if [[ "$backup_failed" != true ]]; then
        init_repo
    fi

    if [[ "$backup_failed" != true && "$VOLUMES_ONLY" != true ]]; then
        backup_postgres || log_warn "PostgreSQL backup had errors"
        backup_meilisearch || log_warn "Meilisearch backup had errors"
        backup_clickhouse || log_warn "ClickHouse backup had errors"
    fi

    if [[ "$backup_failed" != true && "$PG_ONLY" != true ]]; then
        # Issue CHECKPOINT to all PostgreSQL databases for consistency
        log_info "Issuing PostgreSQL CHECKPOINTs..."
        local checkpoint_configs=(
            "alt-db:alt_db_user"
            "alt-kratos-db-1:kratos_user"
            "recap-db:recap_user"
            "rag-db:rag_user"
        )
        for ckpt_config in "${checkpoint_configs[@]}"; do
            IFS=':' read -r ckpt_container ckpt_user <<< "$ckpt_config"
            docker exec "$ckpt_container" psql -U "$ckpt_user" -c "CHECKPOINT;" 2>/dev/null || true
        done

        backup_volumes
        if [[ $? -ne 0 ]]; then backup_failed=true; fi
    fi

    if [[ "$backup_failed" != true ]]; then
        prune_snapshots
        verify_backup
        generate_metrics || true
    fi

    set -e

    if [[ "$backup_failed" == true ]]; then
        ping_healthcheck "fail"
        log_error "=========================================="
        log_error "Backup FAILED - ${TIMESTAMP}"
        log_error "=========================================="
        exit 1
    else
        ping_healthcheck "success"
        log_success "=========================================="
        log_success "Backup completed successfully - ${TIMESTAMP}"
        log_success "=========================================="

        # Show summary
        log_info "Summary:"
        run_restic -r "$RESTIC_REPOSITORY" snapshots --latest 1 2>/dev/null || true
    fi

    # Rotate old logs (keep 30 days)
    find /backups/logs/ -name "*.log" -mtime +30 -delete 2>/dev/null || true
}

main "$@"
