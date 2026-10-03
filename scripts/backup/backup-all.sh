#!/bin/bash
#
# Alt Platform Master Backup Script
# Performs comprehensive backup of all data stores using Restic (Daemonless, Networked)
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
#   IN_CONTAINER         Must be set to 1 when running inside restic-backup container
#   RESTIC_REPOSITORY    Path to Restic repository (default: /backups/restic-repo)
#   RESTIC_PASSWORD_FILE Path to password file (default: /run/secrets/restic_password)
#   HEALTHCHECK_URL      Healthchecks.io ping URL (optional)
#   POSTGRES_BACKUP_DIR  Postgres dump directory (default: /backups/postgres)
#

set -euo pipefail

# Configuration
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TIMESTAMP=$(date +%Y%m%d_%H%M%S)

BACKUP_ROOT="${BACKUP_ROOT:-/backups}"
RESTIC_REPOSITORY="${RESTIC_REPOSITORY:-${BACKUP_ROOT}/restic-repo}"
RESTIC_PASSWORD_FILE="${RESTIC_PASSWORD_FILE:-/run/secrets/restic_password}"
LOG_FILE="${LOG_FILE:-${BACKUP_ROOT}/logs/backup_${TIMESTAMP}.log}"
POSTGRES_BACKUP_DIR="${POSTGRES_BACKUP_DIR:-${BACKUP_ROOT}/postgres}"
METRICS_DIR="${METRICS_DIR:-${BACKUP_ROOT}/metrics}"

INIT_REPO=false
PG_ONLY=false
VOLUMES_ONLY=false
PRUNE=false
VERIFY=false
DRY_RUN=false

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

log() {
    local level="$1"
    shift
    local message="$*"
    local timestamp
    timestamp=$(date '+%Y-%m-%d %H:%M:%S')
    echo -e "${timestamp} [${level}] ${message}" | tee -a "$LOG_FILE" 2>/dev/null || echo -e "${timestamp} [${level}] ${message}"
}

log_info() { log "INFO" "$*"; }
log_warn() { log "${YELLOW}WARN${NC}" "$*"; }
log_error() { log "${RED}ERROR${NC}" "$*"; }
log_success() { log "${GREEN}SUCCESS${NC}" "$*"; }

run_restic() {
    restic "$@"
}

ping_healthcheck() {
    local status="$1"
    local url="${HEALTHCHECK_URL:-}"
    if [[ -n "$url" ]]; then
        case "$status" in
            start)   curl -fsS -m 10 --retry 5 "${url}/start" >/dev/null 2>&1 || true ;;
            success) curl -fsS -m 10 --retry 5 "${url}" >/dev/null 2>&1 || true ;;
            fail)    curl -fsS -m 10 --retry 5 "${url}/fail" >/dev/null 2>&1 || true ;;
        esac
    fi
}

show_help() {
    head -25 "$0" | tail -22 | sed 's/^# //' | sed 's/^#//'
}

parse_args() {
    while [[ $# -gt 0 ]]; do
        case $1 in
            --init) INIT_REPO=true; shift ;;
            --pg-only) PG_ONLY=true; shift ;;
            --volumes-only) VOLUMES_ONLY=true; shift ;;
            --prune) PRUNE=true; shift ;;
            --verify) VERIFY=true; shift ;;
            --dry-run) DRY_RUN=true; shift ;;
            -h|--help) show_help; exit 0 ;;
            *) log_error "Unknown option: $1"; show_help; exit 1 ;;
        esac
    done
}

verify_prerequisites() {
    log_info "Verifying prerequisites..."
    if [[ -z "${IN_CONTAINER:-}" ]]; then
        log_error "This script must run IN the backup container (IN_CONTAINER=1)."
        return 1
    fi

    for cmd in restic pg_dump psql curl jq; do
        if ! command -v "$cmd" &>/dev/null; then
            log_error "Required tool '$cmd' not found in PATH"
            return 1
        fi
    done

    mkdir -p "$POSTGRES_BACKUP_DIR"
    mkdir -p "$(dirname "$LOG_FILE")"
    if [[ "$RESTIC_REPOSITORY" == "${BACKUP_ROOT}"/* || "$RESTIC_REPOSITORY" == /backups/* ]]; then
        mkdir -p "$RESTIC_REPOSITORY" || true
    fi
    log_success "Prerequisites verified"
    return 0
}

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

backup_postgres() {
    log_info "Starting PostgreSQL backups..."

    local default_alt_user="${ALT_DB_USER:-alt_db_user}"
    local default_alt_pwd="/run/secrets/postgres_password"
    if [[ "$default_alt_user" == "alt_appuser" ]]; then
        default_alt_pwd="/run/secrets/db_password"
    fi
    local alt_pwd_file="${ALT_DB_PASSWORD_FILE:-$default_alt_pwd}"

    # name:host:user:dbname:password_file:is_required
    local databases=(
        "alt-db:${ALT_DB_HOST:-alt-db}:${default_alt_user}:${ALT_DB_NAME:-alt}:${alt_pwd_file}:true"
        "kratos-db:${KRATOS_DB_HOST:-kratos-db}:${KRATOS_DB_USER:-kratos_user}:${KRATOS_DB_NAME:-kratos}:${KRATOS_DB_PASSWORD_FILE:-/run/secrets/kratos_db_password}:true"
        "recap-db:${RECAP_DB_HOST:-recap-db}:${RECAP_DB_USER:-recap_user}:${RECAP_DB_NAME:-recap}:${RECAP_DB_PASSWORD_FILE:-/run/secrets/recap_db_password}:${BACKUP_RECAP_DB:-auto}"
        "rag-db:${RAG_DB_HOST:-rag-db}:${RAG_DB_USER:-rag_user}:${RAG_DB_NAME:-rag_db}:${RAG_DB_PASSWORD_FILE:-/run/secrets/rag_db_password}:${BACKUP_RAG_DB:-auto}"
        "pact-db:${PACT_DB_HOST:-pact-db}:${PACT_DB_USER:-pact}:${PACT_DB_NAME:-pact}:${PACT_DB_PASSWORD_FILE:-/run/secrets/pact_db_password}:${BACKUP_PACT_DB:-auto}"
    )

    local pg_verified=0
    local pg_failed=0

    for db_config in "${databases[@]}"; do
        IFS=':' read -r name host user dbname pwd_file required_flag <<< "$db_config"
        local backup_file="${POSTGRES_BACKUP_DIR}/${name}-${TIMESTAMP}.dump"

        if [[ "$DRY_RUN" == true ]]; then
            log_info "[DRY-RUN] Would backup $name to $backup_file"
            pg_verified=$((pg_verified + 1))
            continue
        fi

        # Check explicit disabled state
        if [[ "$required_flag" == "false" || "$required_flag" == "0" ]]; then
            log_info "Skipping optional database $name (explicitly disabled)"
            continue
        fi

        # Check DNS resolution
        local resolved=true
        if ! getent hosts "$host" >/dev/null 2>&1; then
            if [[ "$host" == "alt-db" ]] && getent hosts "db" >/dev/null 2>&1; then
                host="db"
            else
                resolved=false
            fi
        fi

        if [[ "$resolved" != true ]]; then
            if [[ "$required_flag" == "true" || "$required_flag" == "1" ]]; then
                log_error "Required database service $name ($host) not found in DNS"
                pg_failed=$((pg_failed + 1))
            else
                log_info "Optional database service $name ($host) not found in DNS; skipping"
            fi
            continue
        fi

        # Active/required database must have valid credentials file
        if [[ ! -f "$pwd_file" ]]; then
            log_error "Missing credentials file $pwd_file for $name"
            pg_failed=$((pg_failed + 1))
            continue
        fi

        export PGPASSWORD
        PGPASSWORD="$(cat "$pwd_file")"
        if pg_dump -h "$host" -U "$user" -d "$dbname" --format=custom --compress=6 --verbose > "$backup_file" 2>> "$LOG_FILE"; then
            local size
            size=$(ls -lh "$backup_file" 2>/dev/null | awk '{print $5}' || stat -c %s "$backup_file" 2>/dev/null || echo "unknown")
            log_success "Backed up $name: $backup_file ($size)"
            pg_verified=$((pg_verified + 1))
        else
            log_error "Failed to backup $name"
            pg_failed=$((pg_failed + 1))
        fi
        unset PGPASSWORD
    done

    # Clean old PostgreSQL dumps
    local retention="${PG_RETENTION_DAYS:-7}"
    for db_config in "${databases[@]}"; do
        IFS=':' read -r name host user dbname pwd_file required_flag <<< "$db_config"
        find "$POSTGRES_BACKUP_DIR" -name "${name}-*.dump" -mtime +"$retention" -delete 2>/dev/null || true
    done

    log_info "PostgreSQL backup summary: $pg_verified passed, $pg_failed failed"
    if [[ $pg_failed -gt 0 ]]; then
        return 1
    fi
    return 0
}

backup_meilisearch() {
    log_info "Creating Meilisearch snapshot..."
    if [[ "$DRY_RUN" == true ]]; then
        log_info "[DRY-RUN] Would create Meilisearch snapshot"
        return 0
    fi

    local meili_host="${MEILI_HOST:-meilisearch}"
    local meili_port="${MEILI_PORT:-7700}"
    local meili_enabled="${BACKUP_MEILISEARCH:-auto}"

    if [[ "$meili_enabled" == "false" || "$meili_enabled" == "0" ]]; then
        log_info "Meilisearch backup explicitly disabled; skipping"
        return 0
    fi

    if ! getent hosts "$meili_host" >/dev/null 2>&1; then
        if [[ "$meili_enabled" == "true" || "$meili_enabled" == "1" ]]; then
            log_error "Meilisearch host $meili_host not found in DNS despite being explicitly enabled"
            return 1
        else
            log_info "Meilisearch DNS not found. Assuming optional service down."
            return 0
        fi
    fi

    local meili_key_file="${MEILI_MASTER_KEY_FILE:-/run/secrets/meili_master_key}"
    if [[ ! -f "$meili_key_file" ]]; then
        log_error "Missing Meilisearch master key file: $meili_key_file"
        return 1
    fi
    local meili_key
    meili_key=$(cat "$meili_key_file")

    local snap_out
    snap_out=$(mktemp 2>/dev/null || echo "/tmp/meili_snap_out")
    local http_code
    http_code=$(curl -s -w "%{http_code}" -o "$snap_out" -X POST \
        "http://${meili_host}:${meili_port}/snapshots" \
        -H "Authorization: Bearer $meili_key" \
        -H "Content-Type: application/json" 2>&1 || echo "curl_failed")

    if [[ "$http_code" != "200" && "$http_code" != "202" ]]; then
        local err
        err=$(cat "$snap_out" 2>/dev/null || echo "HTTP $http_code")
        log_error "Meilisearch snapshot trigger failed (HTTP $http_code): $err"
        rm -f "$snap_out"
        return 1
    fi

    local task_uid
    task_uid=$(jq -r '.taskUid // empty' "$snap_out" 2>/dev/null || echo "")
    rm -f "$snap_out"
    if [[ -z "$task_uid" ]]; then
        log_error "Could not extract taskUid from Meilisearch snapshot response"
        return 1
    fi
    log_info "Meilisearch snapshot triggered (taskUid: $task_uid). Waiting for completion..."

    local elapsed=0
    local timeout="${MEILI_POLL_TIMEOUT:-120}"
    local poll_interval="${MEILI_POLL_INTERVAL:-5}"
    while [[ $elapsed -lt $timeout ]]; do
        sleep "$poll_interval"
        elapsed=$((elapsed + poll_interval))
        local task_out
        task_out=$(mktemp 2>/dev/null || echo "/tmp/meili_task_out")
        local t_code
        t_code=$(curl -s -w "%{http_code}" -o "$task_out" \
            "http://${meili_host}:${meili_port}/tasks/${task_uid}" \
            -H "Authorization: Bearer $meili_key" 2>&1 || echo "curl_failed")
        if [[ "$t_code" == "200" ]]; then
            local status
            status=$(jq -r '.status // empty' "$task_out" 2>/dev/null || echo "unknown")
            case "$status" in
                succeeded)
                    log_success "Meilisearch snapshot completed successfully (${elapsed}s)"
                    rm -f "$task_out"
                    return 0
                    ;;
                failed)
                    local error_msg
                    error_msg=$(jq -r '.error.message // "unknown error"' "$task_out" 2>/dev/null || echo "task failed")
                    log_error "Meilisearch snapshot task failed: $error_msg"
                    rm -f "$task_out"
                    return 1
                    ;;
                *)
                    log_info "Meilisearch snapshot status: ${status} (${elapsed}s/${timeout}s)"
                    ;;
            esac
        fi
        rm -f "$task_out"
    done
    log_error "Meilisearch snapshot timed out after ${timeout}s"
    return 1
}

backup_clickhouse() {
    log_info "Backing up ClickHouse..."
    if [[ "$DRY_RUN" == true ]]; then
        log_info "[DRY-RUN] Would backup ClickHouse"
        return 0
    fi

    local ch_host="${CLICKHOUSE_HOST:-clickhouse}"
    local ch_port="${CLICKHOUSE_PORT:-8123}"
    local ch_db="${CLICKHOUSE_DB:-rask_logs}"
    local ch_enabled="${BACKUP_CLICKHOUSE:-auto}"

    if [[ "$ch_enabled" == "false" || "$ch_enabled" == "0" ]]; then
        log_info "ClickHouse backup explicitly disabled; skipping"
        return 0
    fi

    if ! getent hosts "$ch_host" >/dev/null 2>&1; then
        if [[ "$ch_enabled" == "true" || "$ch_enabled" == "1" ]]; then
            log_error "ClickHouse host $ch_host not found in DNS despite being explicitly enabled"
            return 1
        else
            log_info "ClickHouse DNS not found. Assuming optional service down."
            return 0
        fi
    fi

    local backup_name="backup_${TIMESTAMP}"
    local ch_user="${CLICKHOUSE_USER:-rask_user}"
    local ch_pwd_file="${CLICKHOUSE_PASSWORD_FILE:-/run/secrets/clickhouse_password}"
    if [[ ! -f "$ch_pwd_file" ]]; then
        log_error "Missing credentials file $ch_pwd_file for ClickHouse"
        return 1
    fi
    local ch_pass
    ch_pass=$(cat "$ch_pwd_file")

    local query="BACKUP DATABASE ${ch_db} TO Disk('backups', '${backup_name}')"
    local ch_out
    ch_out=$(mktemp 2>/dev/null || echo "/tmp/ch_backup_out")
    local resp_code
    resp_code=$(curl -s -w "%{http_code}" -o "$ch_out" -X POST \
        -u "${ch_user}:${ch_pass}" \
        --data-binary "$query" \
        "http://${ch_host}:${ch_port}/" 2>&1 || echo "curl_failed")

    if [[ "$resp_code" == "200" ]]; then
        log_success "ClickHouse native backup completed successfully to Disk('backups', '${backup_name}')"
        rm -f "$ch_out"
        return 0
    else
        local err_detail
        err_detail=$(cat "$ch_out" 2>/dev/null || echo "unknown")
        log_error "ClickHouse native backup failed (HTTP $resp_code): $err_detail"
        rm -f "$ch_out"
        return 1
    fi
}

backup_volumes() {
    log_info "Starting Restic volume backup..."
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
        "/data/pact_db_data"
        "$POSTGRES_BACKUP_DIR"
        "/backups/clickhouse"
    )

    if [[ "$DRY_RUN" == true ]]; then
        log_info "[DRY-RUN] Would backup volumes:"
        printf '%s\n' "${volumes[@]}"
        return 0
    fi

    local exclude_args=(
        --exclude="*.tmp"
        --exclude="*.log"
        --exclude="**/pg_wal/*"
        --exclude="**/pg_replslot/*"
        --exclude="**/pg_stat_tmp/*"
        --exclude="**/tmp_merge_*"
        --exclude="**/tmp_insert_*"
    )

    local paths_to_backup=()
    for vol in "${volumes[@]}"; do
        if [[ -d "$vol" ]]; then
            paths_to_backup+=("$vol")
        else
            log_warn "Volume path not found: $vol"
        fi
    done

    if [[ ${#paths_to_backup[@]} -eq 0 ]]; then
        log_error "No volumes found to backup"
        return 1
    fi

    if ! run_restic -r "$RESTIC_REPOSITORY" backup \
        --tag "scheduled" \
        --tag "$(date +%Y%m%d)" \
        "${exclude_args[@]}" \
        "${paths_to_backup[@]}" 2>&1 | tee -a "$LOG_FILE"; then
        log_error "Restic volume backup failed"
        return 1
    fi

    log_success "Restic backup completed"
    return 0
}

prune_snapshots() {
    if [[ "$PRUNE" != true ]]; then return 0; fi
    log_info "Pruning old snapshots..."
    if ! run_restic -r "$RESTIC_REPOSITORY" forget \
        --keep-hourly 24 --keep-daily 7 --keep-weekly 4 --keep-monthly 3 \
        --prune 2>&1 | tee -a "$LOG_FILE"; then
        log_error "Prune failed"
        return 1
    fi
    log_success "Prune completed"
    return 0
}

verify_backup() {
    if [[ "$VERIFY" != true ]]; then return 0; fi
    log_info "Verifying backup integrity..."
    if ! run_restic -r "$RESTIC_REPOSITORY" check 2>&1 | tee -a "$LOG_FILE"; then
        log_error "Backup verification failed"
        return 1
    fi
    log_success "Backup verification completed"
    return 0
}

generate_metrics() {
    local metrics_file="${METRICS_DIR}/backup_metrics.prom"
    mkdir -p "$(dirname "$metrics_file")"
    local snapshot_count
    snapshot_count=$(run_restic -r "$RESTIC_REPOSITORY" snapshots --json 2>/dev/null | jq 'length' || echo 0)
    local repo_stats
    repo_stats=$(run_restic -r "$RESTIC_REPOSITORY" stats --json 2>/dev/null || echo '{}')
    local total_size
    total_size=$(echo "$repo_stats" | jq -r '.total_size // 0')

    cat > "$metrics_file" << MEOF
# HELP backup_last_success_timestamp Unix timestamp of last successful backup
# TYPE backup_last_success_timestamp gauge
backup_last_success_timestamp{type="full"} $(date +%s)
# HELP backup_restic_snapshot_count Number of Restic snapshots
# TYPE backup_restic_snapshot_count gauge
backup_restic_snapshot_count $snapshot_count
# HELP backup_total_size_bytes Total size of backup repository in bytes
# TYPE backup_total_size_bytes gauge
backup_total_size_bytes $total_size
MEOF
}

main() {
    parse_args "$@"
    ping_healthcheck "start"
    local backup_failed=false
    set +e

    verify_prerequisites || backup_failed=true
    if [[ "$backup_failed" != true ]]; then init_repo; fi

    if [[ "$backup_failed" != true && "$VOLUMES_ONLY" != true ]]; then
        backup_postgres || backup_failed=true
        backup_meilisearch || backup_failed=true
        backup_clickhouse || backup_failed=true
    fi

    if [[ "$backup_failed" != true && "$PG_ONLY" != true ]]; then
        # Note: CHECKPOINT flushes dirty shared buffers to disk prior to raw volume copying.
        # However, raw volume file copying excludes active pg_wal write-ahead logs and scans
        # mutable database directories without atomic filesystem-level snapshots.
        # Therefore, raw volume copies are strictly auxiliary/best-effort copies and do NOT
        # provide crash-consistency, physical point-in-time recovery (PITR), or ACID recovery guarantees.
        # Authoritative database recovery relies on logical dumps (pg_dump custom format,
        # ClickHouse native BACKUP, and Meilisearch snapshots).
        log_info "Issuing PostgreSQL CHECKPOINTs to flush buffers..."
        local default_alt_user="${ALT_DB_USER:-alt_db_user}"
        local default_alt_pwd="/run/secrets/postgres_password"
        if [[ "$default_alt_user" == "alt_appuser" ]]; then
            default_alt_pwd="/run/secrets/db_password"
        fi
        local alt_pwd_file="${ALT_DB_PASSWORD_FILE:-$default_alt_pwd}"

        local checkpoint_configs=(
            "${ALT_DB_HOST:-alt-db}:${default_alt_user}:${alt_pwd_file}"
            "${KRATOS_DB_HOST:-kratos-db}:${KRATOS_DB_USER:-kratos_user}:${KRATOS_DB_PASSWORD_FILE:-/run/secrets/kratos_db_password}"
            "${RECAP_DB_HOST:-recap-db}:${RECAP_DB_USER:-recap_user}:${RECAP_DB_PASSWORD_FILE:-/run/secrets/recap_db_password}"
            "${RAG_DB_HOST:-rag-db}:${RAG_DB_USER:-rag_user}:${RAG_DB_PASSWORD_FILE:-/run/secrets/rag_db_password}"
            "${PACT_DB_HOST:-pact-db}:${PACT_DB_USER:-pact}:${PACT_DB_PASSWORD_FILE:-/run/secrets/pact_db_password}"
        )
        for ckpt_config in "${checkpoint_configs[@]}"; do
            IFS=':' read -r host user pwd_file <<< "$ckpt_config"
            if ! getent hosts "$host" >/dev/null 2>&1; then
                if [[ "$host" == "alt-db" ]] && getent hosts "db" >/dev/null 2>&1; then
                    host="db"
                else
                    continue
                fi
            fi
            if [[ -f "$pwd_file" ]]; then
                export PGPASSWORD
                PGPASSWORD="$(cat "$pwd_file")"
                psql -h "$host" -U "$user" -d postgres -c "CHECKPOINT;" 2>/dev/null || true
                unset PGPASSWORD
            fi
        done
        backup_volumes || backup_failed=true
    fi

    if [[ "$backup_failed" != true ]]; then
        prune_snapshots || backup_failed=true
        verify_backup || backup_failed=true
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
        log_info "Summary:"
        run_restic -r "$RESTIC_REPOSITORY" snapshots --latest 1 2>/dev/null || true
    fi
    find "${BACKUP_ROOT}/logs/" -name "*.log" -mtime +30 -delete 2>/dev/null || true
}

main "$@"
