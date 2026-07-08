#!/bin/bash
#
# Alt Platform Offsite Sync Script
# Synchronizes local Restic repository to remote backup server via Tailscale
#
# Usage:
#   ./sync-offsite.sh [options]
#
# Options:
#   --check-only     Only check connectivity, don't sync
#   --verify         Verify remote repository after sync
#   --prune-remote   Apply retention policy to remote repository
#   --dry-run        Show what would be synced without executing
#   -h, --help       Show this help message
#
# Environment Variables:
#   RESTIC_REPOSITORY         Local Restic repository path
#   RESTIC_PASSWORD_FILE      Path to password file
#   REMOTE_REPO               Remote repository (sftp://host:/path)
#   TAILSCALE_HOST            Tailscale hostname of backup server
#   HEALTHCHECK_OFFSITE_URL   Healthchecks.io ping URL (optional)
#

set -uo pipefail

# Configuration
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TIMESTAMP=$(date +%Y%m%d_%H%M%S)
LOG_FILE="/backups/logs/sync-offsite-${TIMESTAMP}.log"

# Defaults
RESTIC_REPOSITORY="${RESTIC_REPOSITORY:-/backups/restic-repo}"
RESTIC_PASSWORD_FILE="${RESTIC_PASSWORD_FILE:-/run/secrets/restic_password}"
TAILSCALE_HOST="${TAILSCALE_HOST:-backup-server}"
REMOTE_REPO="${REMOTE_REPO:-sftp:${TAILSCALE_HOST}:/backups/alt/restic-repo}"
REMOTE_KEEP_DAILY="${REMOTE_KEEP_DAILY:-30}"
REMOTE_KEEP_WEEKLY="${REMOTE_KEEP_WEEKLY:-12}"
REMOTE_KEEP_MONTHLY="${REMOTE_KEEP_MONTHLY:-6}"

# Flags
CHECK_ONLY=false
VERIFY=false
PRUNE_REMOTE=false
DRY_RUN=false

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

# Logging
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

# Healthcheck ping
ping_healthcheck() {
    local status="$1"
    local url="${HEALTHCHECK_OFFSITE_URL:-}"

    if [[ -n "$url" ]]; then
        case "$status" in
            start) curl -fsS -m 10 --retry 5 "${url}/start" >/dev/null 2>&1 || true ;;
            success) curl -fsS -m 10 --retry 5 "${url}" >/dev/null 2>&1 || true ;;
            fail) curl -fsS -m 10 --retry 5 "${url}/fail" >/dev/null 2>&1 || true ;;
        esac
    fi
}

# Show help
show_help() {
    head -22 "$0" | tail -19 | sed 's/^# //' | sed 's/^#//'
}

# Parse arguments
parse_args() {
    while [[ $# -gt 0 ]]; do
        case $1 in
            --check-only)
                CHECK_ONLY=true
                shift
                ;;
            --verify)
                VERIFY=true
                shift
                ;;
            --prune-remote)
                PRUNE_REMOTE=true
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

# Check Tailscale connectivity
check_tailscale() {
    log_info "Checking Tailscale connectivity to $TAILSCALE_HOST..."

    # Check if tailscale is available
    if ! command -v tailscale &>/dev/null; then
        log_warn "tailscale command not found, will attempt direct SSH"
        return 0
    fi

    # Check Tailscale status
    if ! tailscale status >/dev/null 2>&1; then
        log_error "Tailscale is not running or not connected"
        return 1
    fi

    # Ping the backup server
    if ! tailscale ping --timeout 10s "$TAILSCALE_HOST" >/dev/null 2>&1; then
        log_error "Cannot reach $TAILSCALE_HOST via Tailscale"
        return 1
    fi

    log_success "Tailscale connectivity confirmed"
    return 0
}

# Check SSH connectivity
check_ssh() {
    log_info "Checking SSH connectivity..."

    if ! ssh -o ConnectTimeout=10 -o BatchMode=yes "$TAILSCALE_HOST" "echo OK" >/dev/null 2>&1; then
        log_error "Cannot SSH to $TAILSCALE_HOST"
        log_info "Ensure SSH key is configured for passwordless access"
        return 1
    fi

    log_success "SSH connectivity confirmed"
    return 0
}

# Helper to run restic (via Docker if not installed natively)
run_restic() {
    if [[ -n "$RESTIC_PASSWORD_FILE" && -z "${RESTIC_FROM_PASSWORD_FILE:-}" ]]; then
        export RESTIC_FROM_PASSWORD_FILE="$RESTIC_PASSWORD_FILE"
    fi

    if command -v restic &>/dev/null; then
        restic "$@"
    else
        docker run --rm -i \
            --net=host \
            -e RESTIC_PASSWORD_FILE="$RESTIC_PASSWORD_FILE" \
            -e RESTIC_FROM_PASSWORD_FILE="$RESTIC_FROM_PASSWORD_FILE" \
            -v "$SCRIPT_DIR/../../secrets/ssh/id_ed25519_backup:/root/.ssh/id_ed25519:ro" \
            -v "$SCRIPT_DIR/../../secrets/ssh/known_hosts:/root/.ssh/known_hosts:ro" \
            -v "$RESTIC_REPOSITORY:$RESTIC_REPOSITORY" \
            -v "$RESTIC_PASSWORD_FILE:$RESTIC_PASSWORD_FILE:ro" \
            restic/restic:0.17.3 "$@"
    fi
}

# Initialize remote repository if needed
init_remote_repo() {
    log_info "Checking remote repository..."

    if run_restic -r "$REMOTE_REPO" cat config >/dev/null 2>&1; then
        log_info "Remote repository already initialized"
        return 0
    fi

    log_info "Initializing remote repository..."

    if [[ "$DRY_RUN" == true ]]; then
        log_info "[DRY-RUN] Would initialize remote repository at $REMOTE_REPO"
        return 0
    fi

    run_restic -r "$REMOTE_REPO" init
    log_success "Remote repository initialized"
}

# Sync snapshots to remote
sync_snapshots() {
    log_info "Syncing snapshots to remote repository..."
    log_info "Source: $RESTIC_REPOSITORY"
    log_info "Destination: $REMOTE_REPO"

    if [[ "$DRY_RUN" == true ]]; then
        log_info "[DRY-RUN] Would sync the following snapshots:"
        run_restic -r "$RESTIC_REPOSITORY" snapshots --tag scheduled
        return 0
    fi

    # Use restic copy to transfer snapshots
    # Only copy snapshots tagged as 'scheduled' to avoid copying test backups
    if ! run_restic copy \
        --from-repo "$RESTIC_REPOSITORY" \
        --repo "$REMOTE_REPO" \
        --tag scheduled 2>&1 | tee -a "$LOG_FILE"; then
        log_error "Snapshot sync failed"
        return 1
    fi

    log_success "Snapshot sync completed"
}

# Build retention flags for remote prune
build_retention_flags() {
    local flags=()
    if [[ "$REMOTE_KEEP_DAILY" -gt 0 ]]; then
        flags+=(--keep-daily "$REMOTE_KEEP_DAILY")
    fi
    if [[ "$REMOTE_KEEP_WEEKLY" -gt 0 ]]; then
        flags+=(--keep-weekly "$REMOTE_KEEP_WEEKLY")
    fi
    if [[ "$REMOTE_KEEP_MONTHLY" -gt 0 ]]; then
        flags+=(--keep-monthly "$REMOTE_KEEP_MONTHLY")
    fi
    echo "${flags[@]}"
}

# Apply retention policy to remote
prune_remote() {
    if [[ "$PRUNE_REMOTE" != true ]]; then
        return
    fi

    log_info "Applying retention policy to remote repository..."

    local retention_flags
    retention_flags=($(build_retention_flags))

    if [[ ${#retention_flags[@]} -eq 0 ]]; then
        log_warn "All retention values are 0, skipping remote prune"
        return
    fi

    log_info "Retention flags: ${retention_flags[*]}"

    if [[ "$DRY_RUN" == true ]]; then
        log_info "[DRY-RUN] Would prune remote with:"
        log_info "  ${retention_flags[*]}"
        run_restic -r "$REMOTE_REPO" forget --dry-run \
            "${retention_flags[@]}"
        return
    fi

    if ! run_restic -r "$REMOTE_REPO" forget \
        "${retention_flags[@]}" \
        --prune 2>&1 | tee -a "$LOG_FILE"; then
        log_error "Remote prune failed"
        return 1
    fi

    log_success "Remote prune completed"
}

# Verify remote repository
verify_remote() {
    if [[ "$VERIFY" != true ]]; then
        return
    fi

    log_info "Verifying remote repository integrity..."

    if [[ "$DRY_RUN" == true ]]; then
        log_info "[DRY-RUN] Would run 'restic check' on remote"
        return
    fi

    if ! run_restic -r "$REMOTE_REPO" check 2>&1 | tee -a "$LOG_FILE"; then
        log_error "Remote verification failed"
        return 1
    fi

    log_success "Remote verification completed"
}

# Show sync summary
show_summary() {
    log_info "=========================================="
    log_info "Offsite Sync Summary"
    log_info "=========================================="

    log_info "Local repository snapshots:"
    run_restic -r "$RESTIC_REPOSITORY" snapshots --latest 3 2>/dev/null || true

    log_info ""
    log_info "Remote repository snapshots:"
    run_restic -r "$REMOTE_REPO" snapshots --latest 3 2>/dev/null || true
}

# Main execution
main() {
    parse_args "$@"

    mkdir -p "$(dirname "$LOG_FILE")"

    log_info "=========================================="
    log_info "Alt Platform Offsite Sync - ${TIMESTAMP}"
    log_info "=========================================="

    ping_healthcheck "start"

    # Connectivity checks
    if ! check_tailscale || ! check_ssh; then
        ping_healthcheck "fail"
        log_error "=========================================="
        log_error "Offsite Sync FAILED (connectivity) - ${TIMESTAMP}"
        log_error "=========================================="
        exit 1
    fi

    if [[ "$CHECK_ONLY" == true ]]; then
        log_success "Connectivity check passed"
        exit 0
    fi

    # Sync operations - track failures individually
    local sync_failed=false

    init_remote_repo || sync_failed=true
    if [[ "$sync_failed" != true ]]; then
        sync_snapshots || sync_failed=true
    fi
    prune_remote || sync_failed=true
    verify_remote || sync_failed=true

    if [[ "$sync_failed" != true ]]; then
        show_summary
    fi

    if [[ "$sync_failed" == true ]]; then
        ping_healthcheck "fail"
        log_error "=========================================="
        log_error "Offsite Sync FAILED - ${TIMESTAMP}"
        log_error "=========================================="
        exit 1
    else
        ping_healthcheck "success"
        log_success "=========================================="
        log_success "Offsite Sync completed - ${TIMESTAMP}"
        log_success "=========================================="
    fi
}

main "$@"


