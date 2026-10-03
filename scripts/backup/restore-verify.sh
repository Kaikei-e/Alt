#!/bin/bash
#
# Alt Platform Automated Restore Verification
# Validates that the latest backup can be successfully restored
#
# Usage:
#   ./restore-verify.sh [options]
#
# Options:
#   --snapshot ID    Verify specific Restic snapshot (default: latest)
#   --skip-pg        Skip PostgreSQL restore verification
#   --skip-cleanup   Don't remove temporary resources after verification
#   --dry-run        Show what would be done without executing
#   -h, --help       Show this help message
#
# Environment Variables:
#   RESTIC_REPOSITORY    Path to Restic repository (default: /backups/restic-repo)
#   RESTIC_PASSWORD_FILE Path to password file (default: /run/secrets/restic_password)
#   HEALTHCHECK_VERIFY_URL  Healthchecks.io URL for restore verification
#   METRICS_DIR          Output directory for metrics (default: /backups/metrics)
#

set -euo pipefail

# Configuration
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TIMESTAMP=$(date +%Y%m%d_%H%M%S)

BACKUP_ROOT="${BACKUP_ROOT:-/backups}"
LOG_FILE="${LOG_FILE:-${BACKUP_ROOT}/logs/restore-verify-${TIMESTAMP}.log}"
RESTIC_REPOSITORY="${RESTIC_REPOSITORY:-${BACKUP_ROOT}/restic-repo}"
RESTIC_PASSWORD_FILE="${RESTIC_PASSWORD_FILE:-/run/secrets/restic_password}"
HEALTHCHECK_VERIFY_URL="${HEALTHCHECK_VERIFY_URL:-}"
METRICS_DIR="${METRICS_DIR:-${BACKUP_ROOT}/metrics}"
BACKUP_CONTAINER="${BACKUP_CONTAINER:-alt-backup}"
POSTGRES_BACKUP_DIR="${POSTGRES_BACKUP_DIR:-${BACKUP_ROOT}/postgres}"

# Fail-fast: restore verification requires host Docker access and cannot run inside container
IN_CONTAINER="${IN_CONTAINER:-}"
if [[ -n "$IN_CONTAINER" || -f "/.dockerenv" ]]; then
    echo "ERROR: restore-verify.sh must be run on the host (requires Docker for verification containers)" >&2
    exit 1
fi

# Temporary resources prefix
VERIFY_PREFIX="alt-restore-verify"
CONTAINER_RESTORE_DIR=""
HOST_RESTORE_DIR=""
RESTORE_DIR=""

# Flags
SNAPSHOT_ID="latest"
SKIP_PG=false
SKIP_CLEANUP=false
DRY_RUN=false

# State tracking for cleanup
TEMP_VOLUMES=()
TEMP_CONTAINERS=()
TEMP_DIRS=()

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

# Logging
log() {
    local level="$1"
    shift
    local timestamp
    timestamp=$(date '+%Y-%m-%d %H:%M:%S')
    echo -e "${timestamp} [${level}] $*" | tee -a "$LOG_FILE" 2>/dev/null || echo -e "${timestamp} [${level}] $*"
}

log_info() { log "INFO" "$*"; }
log_warn() { log "${YELLOW}WARN${NC}" "$*"; }
log_error() { log "${RED}ERROR${NC}" "$*"; }
log_success() { log "${GREEN}OK${NC}" "$*"; }

# Helper to run restic (directly if in container, via docker exec if on host)
run_restic() {
    if [[ -n "$IN_CONTAINER" ]]; then
        restic "$@"
    else
        docker exec "$BACKUP_CONTAINER" restic "$@"
    fi
}

# Show help
show_help() {
    head -25 "$0" | tail -22 | sed 's/^# //' | sed 's/^#//'
}

# Parse arguments
parse_args() {
    while [[ $# -gt 0 ]]; do
        case $1 in
            --snapshot)
                SNAPSHOT_ID="$2"
                shift 2
                ;;
            --skip-pg)
                SKIP_PG=true
                shift
                ;;
            --skip-cleanup)
                SKIP_CLEANUP=true
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

# Cleanup function - always runs on exit
cleanup() {
    if [[ "$SKIP_CLEANUP" == true ]]; then
        log_warn "Skipping cleanup (--skip-cleanup). Manual cleanup required:"
        log_warn "  Containers: ${TEMP_CONTAINERS[*]:-none}"
        log_warn "  Volumes: ${TEMP_VOLUMES[*]:-none}"
        log_warn "  Container restore dir: ${CONTAINER_RESTORE_DIR:-none} (inside ${BACKUP_CONTAINER})"
        log_warn "  Directories: ${TEMP_DIRS[*]:-none}"
        return
    fi

    log_info "Cleaning up temporary resources..."

    # Clean up container restore directory inside backup container
    if [[ -n "${CONTAINER_RESTORE_DIR:-}" ]]; then
        docker exec "$BACKUP_CONTAINER" rm -rf "$CONTAINER_RESTORE_DIR" 2>/dev/null || true
    fi

    # Stop and remove containers
    for container in "${TEMP_CONTAINERS[@]}"; do
        docker rm -f "$container" 2>/dev/null || true
    done

    # Remove volumes
    for vol in "${TEMP_VOLUMES[@]}"; do
        docker volume rm -f "$vol" 2>/dev/null || true
    done

    # Remove temporary directories
    for dir in "${TEMP_DIRS[@]}"; do
        rm -rf "$dir" 2>/dev/null || true
    done

    log_info "Cleanup complete"
}

# Write Prometheus metrics
write_metrics() {
    local success="$1"
    local duration="$2"
    local metrics_file="${METRICS_DIR}/backup_restore_verify.prom"

    mkdir -p "$METRICS_DIR"

    cat > "$metrics_file" << EOF
# HELP backup_restore_verify_last_timestamp Unix timestamp of last restore verification
# TYPE backup_restore_verify_last_timestamp gauge
backup_restore_verify_last_timestamp $(date +%s)

# HELP backup_restore_verify_success Whether the last restore verification succeeded (0=no, 1=yes)
# TYPE backup_restore_verify_success gauge
backup_restore_verify_success ${success}

# HELP backup_restore_verify_duration_seconds Duration of the last restore verification in seconds
# TYPE backup_restore_verify_duration_seconds gauge
backup_restore_verify_duration_seconds ${duration}
EOF

    log_info "Metrics written to $metrics_file"
}

# Ping healthcheck
ping_verify_healthcheck() {
    local status="$1"
    if [[ -n "$HEALTHCHECK_VERIFY_URL" ]]; then
        case "$status" in
            start)   curl -fsS -m 10 --retry 3 "${HEALTHCHECK_VERIFY_URL}/start" >/dev/null 2>&1 || true ;;
            success) curl -fsS -m 10 --retry 3 "${HEALTHCHECK_VERIFY_URL}" >/dev/null 2>&1 || true ;;
            fail)    curl -fsS -m 10 --retry 3 "${HEALTHCHECK_VERIFY_URL}/fail" >/dev/null 2>&1 || true ;;
        esac
    fi
}

# Step 1: Get latest snapshot info
get_snapshot_info() {
    log_info "Fetching snapshot information (ID: $SNAPSHOT_ID)..."

    local snapshots_json
    snapshots_json=$(run_restic -r "$RESTIC_REPOSITORY" snapshots --json 2>/dev/null)

    local count
    count=$(echo "$snapshots_json" | jq 'length')

    if [[ "$count" -eq 0 ]]; then
        log_error "No snapshots found in repository"
        return 1
    fi

    if [[ "$SNAPSHOT_ID" == "latest" ]]; then
        SNAPSHOT_ID=$(echo "$snapshots_json" | jq -r 'sort_by(.time) | last | .short_id')
    fi

    log_info "Using snapshot: $SNAPSHOT_ID"
    run_restic -r "$RESTIC_REPOSITORY" snapshots "$SNAPSHOT_ID" 2>/dev/null | tee -a "$LOG_FILE"
}

# Step 2: Restore snapshot to temporary directory
restore_snapshot() {
    # Allocate unique host temporary directory
    HOST_RESTORE_DIR="$(mktemp -d "/tmp/${VERIFY_PREFIX}-host-${TIMESTAMP}-XXXXXX")" || return 1
    RESTORE_DIR="$HOST_RESTORE_DIR"
    TEMP_DIRS+=("$HOST_RESTORE_DIR")

    if [[ "$DRY_RUN" == true ]]; then
        CONTAINER_RESTORE_DIR="/tmp/${VERIFY_PREFIX}-ctr-${TIMESTAMP}"
        log_info "[DRY-RUN] Would restore to $CONTAINER_RESTORE_DIR inside $BACKUP_CONTAINER and copy to host $RESTORE_DIR"
        return 0
    fi

    # Allocate unique container temporary directory via actual docker exec mktemp
    CONTAINER_RESTORE_DIR="$(docker exec "$BACKUP_CONTAINER" mktemp -d "/tmp/${VERIFY_PREFIX}-ctr-XXXXXX" 2>/dev/null)" || CONTAINER_RESTORE_DIR=""
    if [[ -z "$CONTAINER_RESTORE_DIR" ]]; then
        log_error "Failed to allocate unique temporary directory in container $BACKUP_CONTAINER"
        return 1
    fi

    log_info "Restoring snapshot $SNAPSHOT_ID to container directory $CONTAINER_RESTORE_DIR and copying to host $RESTORE_DIR..."

    # 1. Restic restores into container's isolated filesystem
    # Explicit pipe failure check: run_restic ... | tee ... || return 1 (pipefail active)
    run_restic -r "$RESTIC_REPOSITORY" restore "$SNAPSHOT_ID" \
        --target "$CONTAINER_RESTORE_DIR" 2>&1 | tee -a "$LOG_FILE" || return 1

    # 2. Copy restored snapshot tree from container namespace to host temp directory
    docker cp "${BACKUP_CONTAINER}:${CONTAINER_RESTORE_DIR}/." "$HOST_RESTORE_DIR/" || return 1

    # 3. Clean up container temporary files immediately
    docker exec "$BACKUP_CONTAINER" rm -rf "$CONTAINER_RESTORE_DIR" 2>/dev/null || true
    CONTAINER_RESTORE_DIR=""

    # 4. Verify something was restored on host
    if [[ ! -d "$RESTORE_DIR/data" ]]; then
        log_error "Restore did not produce expected /data directory"
        return 1
    fi

    local restored_count
    restored_count=$(ls -1 "$RESTORE_DIR/data/" 2>/dev/null | wc -l)
    log_info "Restored $restored_count volume directories"

    if [[ "$restored_count" -eq 0 ]]; then
        log_error "No volumes restored from snapshot"
        return 1
    fi
}

# Step 3: Verify PostgreSQL dump files can be restored
verify_pg_restore() {
    if [[ "$SKIP_PG" == true ]]; then
        log_info "Skipping PostgreSQL restore verification (--skip-pg)"
        return 0
    fi

    # Real verification requires RESTORE_DIR to exist and be a valid directory.
    # Fallback to POSTGRES_BACKUP_DIR is STRICTLY for dry-run only.
    if [[ "$DRY_RUN" != true ]]; then
        if [[ -z "$RESTORE_DIR" || ! -d "$RESTORE_DIR" ]]; then
            log_error "RESTORE_DIR missing or invalid during real PostgreSQL verification: '${RESTORE_DIR:-}'"
            return 1
        fi
    fi

    log_info "Verifying PostgreSQL backup restorability..."

    # Verified databases: include pact-db along with alt-db, kratos-db, recap-db, rag-db
    local databases=("alt-db" "kratos-db" "recap-db" "rag-db" "pact-db")
    local pg_verified=0
    local pg_failed=0

    for db in "${databases[@]}"; do
        local dump_file=""
        # In dry-run mode, simulate verification cleanly with fallback to POSTGRES_BACKUP_DIR
        if [[ "$DRY_RUN" == true ]]; then
            if [[ -n "$RESTORE_DIR" && -d "$RESTORE_DIR" ]]; then
                dump_file=$(find "$RESTORE_DIR" -name "${db}-*.dump" -type f 2>/dev/null | sort -r | head -1)
            fi
            if [[ -z "$dump_file" || ! -f "$dump_file" ]]; then
                dump_file=$(find "$POSTGRES_BACKUP_DIR" -name "${db}-*.dump" -type f 2>/dev/null | sort -r | head -1)
            fi
            if [[ -n "$dump_file" && -f "$dump_file" ]]; then
                local dump_size
                dump_size=$(stat -c %s "$dump_file" 2>/dev/null || echo 0)
                log_info "[DRY-RUN] Would verify $db dump ($dump_file, $(numfmt --to=iec "$dump_size" 2>/dev/null || echo "${dump_size}B"))"
            else
                log_info "[DRY-RUN] Would verify $db dump (simulated from snapshot $SNAPSHOT_ID)"
            fi
            pg_verified=$((pg_verified + 1))
            continue
        fi

        # Real verification: dump_file MUST be found strictly inside restored snapshot directory
        dump_file=$(find "$RESTORE_DIR" -name "${db}-*.dump" -type f 2>/dev/null | sort -r | head -1)

        if [[ -z "$dump_file" || ! -f "$dump_file" ]]; then
            if [[ "$db" == "alt-db" || "$db" == "kratos-db" ]]; then
                log_error "Required database dump file not found in snapshot $SNAPSHOT_ID for $db"
                pg_failed=$((pg_failed + 1))
            else
                log_warn "No dump file found in snapshot $SNAPSHOT_ID for optional database $db (skipping)"
            fi
            continue
        fi

        local dump_size
        dump_size=$(stat -c %s "$dump_file" 2>/dev/null || echo 0)

        if [[ "$dump_size" -eq 0 ]]; then
            log_error "Dump file for $db is empty: $dump_file"
            pg_failed=$((pg_failed + 1))
            continue
        fi

        # Determine appropriate pinned PostgreSQL 18 container image matching database requirements
        local pg_image="postgres:18.6-alpine"
        local pg_extra_args=()
        if [[ "$db" == "rag-db" ]]; then
            # rag-db requires pgvector extension on PG 18 (matching rag-db / alt-rag-db from Compose)
            pg_image="${VERIFY_PG_RAG_IMAGE:-alt-rag-db}"
        elif [[ "$db" == "recap-db" ]]; then
            # recap-db requires pg_cron on PG 18 (matching actual Compose built image alt-recap-db with pg_cron)
            pg_image="${VERIFY_PG_RECAP_IMAGE:-alt-recap-db}"
            pg_extra_args=(
                "-c" "shared_preload_libraries=pg_cron"
                "-c" "cron.database_name=verify_db"
                "-c" "cron.launch_active_jobs=off"
            )
        fi
        pg_image="${VERIFY_PG_IMAGE:-$pg_image}"

        # Spin up a temporary PostgreSQL container and attempt restore
        local container_name="${VERIFY_PREFIX}-pg-${db}-${TIMESTAMP}"
        local volume_name="${VERIFY_PREFIX}-pgdata-${db}-${TIMESTAMP}"

        TEMP_CONTAINERS+=("$container_name")
        TEMP_VOLUMES+=("$volume_name")

        log_info "Starting temporary PostgreSQL ($pg_image) for $db verification..."

        docker volume create "$volume_name" >/dev/null 2>&1

        local run_cmd=()
        if [[ ${#pg_extra_args[@]} -gt 0 ]]; then
            run_cmd=("postgres" "${pg_extra_args[@]}")
        fi

        docker run -d \
            --name "$container_name" \
            -v "${volume_name}:/var/lib/postgresql" \
            -e POSTGRES_PASSWORD=verify_temp_pass \
            -e POSTGRES_DB=verify_db \
            "$pg_image" \
            "${run_cmd[@]}" >/dev/null 2>&1

        # Wait for PostgreSQL to be ready
        local wait_count=0
        local max_wait=30
        while ! docker exec "$container_name" pg_isready -U postgres >/dev/null 2>&1; do
            sleep 1
            wait_count=$((wait_count + 1))
            if [[ $wait_count -ge $max_wait ]]; then
                log_error "Temporary PostgreSQL for $db failed to start"
                pg_failed=$((pg_failed + 1))
                continue 2
            fi
        done

        # Attempt restore
        # CRITICAL: Do NOT fall back to 'pg_restore --list' as a success indicator.
        # Archive readability does not mean data was restored. An exit code != 0 from
        # pg_restore MUST be treated as an absolute failure.
        # Isolated verifier restore uses --no-owner --no-privileges to avoid role errors.
        log_info "Restoring $db dump into verification container..."
        local restore_exit=0
        docker exec -i "$container_name" pg_restore \
            -U postgres \
            -d verify_db \
            --clean --if-exists \
            --no-owner --no-privileges \
            < "$dump_file" 2>> "$LOG_FILE" || restore_exit=$?

        if [[ $restore_exit -ne 0 ]]; then
            log_error "PostgreSQL restore execution FAILED for $db (exit code: $restore_exit)"
            pg_failed=$((pg_failed + 1))
            docker rm -f "$container_name" >/dev/null 2>&1 || true
            docker volume rm -f "$volume_name" >/dev/null 2>&1 || true
            continue
        fi

        # Assert meaningful restored data: table count in public schema must be > 0
        local table_count
        table_count=$(docker exec "$container_name" psql -U postgres -d verify_db -t -c \
            "SELECT count(*) FROM information_schema.tables WHERE table_schema = 'public';" 2>/dev/null | tr -d '[:space:]' || echo "0")

        if [[ ! "$table_count" =~ ^[0-9]+$ || "$table_count" -le 0 ]]; then
            log_error "PostgreSQL restore verification FAILED for $db: zero public tables restored (found: '$table_count')"
            pg_failed=$((pg_failed + 1))
            docker rm -f "$container_name" >/dev/null 2>&1 || true
            docker volume rm -f "$volume_name" >/dev/null 2>&1 || true
            continue
        fi

        log_success "PostgreSQL restore verification passed for $db ($table_count tables restored)"
        pg_verified=$((pg_verified + 1))

        # Cleanup this container immediately to save resources
        docker rm -f "$container_name" >/dev/null 2>&1
        docker volume rm -f "$volume_name" >/dev/null 2>&1
    done

    log_info "PostgreSQL verification: $pg_verified passed, $pg_failed failed"

    if [[ $pg_failed -gt 0 ]]; then
        return 1
    fi
    return 0
}

# Step 4: Verify volume data integrity
# Note: Raw volume backups exclude pg_wal and live scan mutable volumes without snapshot.
# They are auxiliary best-effort copies, NOT crash-consistent physical recovery or PITR guarantees.
verify_volume_data() {
    log_info "Verifying restored volume data integrity..."

    local verified=0
    local failed=0

    if [[ ! -d "$RESTORE_DIR/data" ]]; then
        log_warn "No restored data directory found, skipping volume verification"
        return 0
    fi

    for vol_dir in "$RESTORE_DIR/data/"*/; do
        local vol_name
        vol_name=$(basename "$vol_dir")

        if [[ ! -d "$vol_dir" ]]; then
            continue
        fi

        # Check if directory has content
        local file_count
        file_count=$(find "$vol_dir" -type f 2>/dev/null | head -100 | wc -l)

        if [[ "$file_count" -gt 0 ]]; then
            log_success "Volume $vol_name: $file_count files found"
            verified=$((verified + 1))
        else
            log_warn "Volume $vol_name: empty (may be expected for some volumes)"
            verified=$((verified + 1))
        fi
    done

    log_info "Volume data verification: $verified verified, $failed failed"

    if [[ $failed -gt 0 ]]; then
        return 1
    fi
    return 0
}

# Main
main() {
    parse_args "$@"

    mkdir -p "$(dirname "$LOG_FILE")" 2>/dev/null || true

    log_info "=========================================="
    log_info "Alt Platform Restore Verification - ${TIMESTAMP}"
    log_info "=========================================="

    # Set trap for cleanup
    trap cleanup EXIT

    ping_verify_healthcheck "start"

    local start_time
    start_time=$(date +%s)
    local verify_success=true

    get_snapshot_info || verify_success=false
    if [[ "$verify_success" == true ]]; then
        restore_snapshot || verify_success=false
    fi
    if [[ "$verify_success" == true ]]; then
        verify_pg_restore || verify_success=false
    fi
    if [[ "$verify_success" == true ]]; then
        verify_volume_data || verify_success=false
    fi

    local end_time
    end_time=$(date +%s)
    local duration=$((end_time - start_time))

    if [[ "$verify_success" == true ]]; then
        write_metrics 1 "$duration"
        ping_verify_healthcheck "success"
        log_success "=========================================="
        log_success "Restore verification PASSED (${duration}s)"
        log_success "=========================================="
    else
        write_metrics 0 "$duration"
        ping_verify_healthcheck "fail"
        log_error "=========================================="
        log_error "Restore verification FAILED (${duration}s)"
        log_error "=========================================="
        exit 1
    fi
}

main "$@"
