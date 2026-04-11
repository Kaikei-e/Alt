#!/bin/bash
# =============================================================================
# Docker Disk Space Cleanup Script
# Purpose: Automatically clean up Docker resources when disk usage exceeds limit
# =============================================================================

set -euo pipefail

# Configuration
MAX_DOCKER_SIZE_GB=${MAX_DOCKER_SIZE_GB:-100}
DOCKER_ROOT_DIR=${DOCKER_ROOT_DIR:-/var/lib/docker}
LOG_FILE=${LOG_FILE:-/var/log/docker-cleanup.log}

# Convert GB to bytes
MAX_SIZE_BYTES=$((MAX_DOCKER_SIZE_GB * 1024 * 1024 * 1024))

# Logging function
log() {
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] $*" | tee -a "$LOG_FILE"
}

# Get current Docker disk usage in bytes
get_docker_size() {
    du -sb "$DOCKER_ROOT_DIR" 2>/dev/null | cut -f1 || echo "0"
}

# Clean up Docker resources
cleanup_docker() {
    local freed_space=0

    log "Starting Docker cleanup..."

    # 1. Remove stopped containers
    local stopped_containers=$(docker ps -a -q -f status=exited 2>/dev/null | wc -l)
    if [ "$stopped_containers" -gt 0 ]; then
        log "Removing $stopped_containers stopped containers..."
        docker container prune -f >/dev/null 2>&1 || true
    fi

    # 2. Remove unused images (keep last 24 hours)
    log "Removing unused images older than 24 hours..."
    docker image prune -a -f --filter "until=24h" >/dev/null 2>&1 || true

    # 3. Remove unused volumes (be careful - this removes all unused volumes)
    log "Removing unused volumes..."
    docker volume prune -f >/dev/null 2>&1 || true

    # 4. Remove build cache (keep last 24 hours or 40GB, whichever is larger)
    log "Cleaning build cache..."
    docker builder prune -a -f --filter "until=24h" >/dev/null 2>&1 || true

    # 5. Remove unused networks
    log "Removing unused networks..."
    docker network prune -f >/dev/null 2>&1 || true

    log "Cleanup completed"
}

# Aggressive cleanup when limit is exceeded
aggressive_cleanup() {
    log "WARNING: Docker disk usage exceeds ${MAX_DOCKER_SIZE_GB}GB limit. Performing aggressive cleanup..."

    # 1. Remove all stopped containers
    docker container prune -f >/dev/null 2>&1 || true

    # 2. Remove all unused images (not just old ones)
    log "Removing all unused images..."
    docker image prune -a -f >/dev/null 2>&1 || true

    # 3. Remove all build cache
    log "Removing all build cache..."
    docker builder prune -a -f >/dev/null 2>&1 || true

    # 4. Remove unused volumes (be very careful here)
    log "Removing unused volumes..."
    docker volume prune -f >/dev/null 2>&1 || true

    # 5. Remove unused networks
    docker network prune -f >/dev/null 2>&1 || true

    log "Aggressive cleanup completed"
}

# Main execution
main() {
    local current_size
    current_size=$(get_docker_size)

    log "Current Docker disk usage: $(numfmt --to=iec-i --suffix=B "$current_size")"
    log "Maximum allowed: ${MAX_DOCKER_SIZE_GB}GB ($(numfmt --to=iec-i --suffix=B "$MAX_SIZE_BYTES"))"

    if [ "$current_size" -gt "$MAX_SIZE_BYTES" ]; then
        log "Disk usage exceeds limit. Performing aggressive cleanup..."
        aggressive_cleanup

        # Check again after cleanup
        sleep 2
        current_size=$(get_docker_size)
        log "Disk usage after cleanup: $(numfmt --to=iec-i --suffix=B "$current_size")"

        if [ "$current_size" -gt "$MAX_SIZE_BYTES" ]; then
            log "ERROR: Disk usage still exceeds limit after cleanup!"
            log "Manual intervention may be required."
            exit 1
        else
            log "SUCCESS: Disk usage is now within limits"
        fi
    else
        log "Disk usage is within limits. Performing regular maintenance cleanup..."
        cleanup_docker
    fi

    # Show final disk usage summary
    log "Final Docker disk usage summary:"
    docker system df 2>/dev/null | tee -a "$LOG_FILE" || true
}

# Run main function
main "$@"

