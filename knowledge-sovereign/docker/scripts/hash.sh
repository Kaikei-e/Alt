#!/bin/sh
# Hash Migration Files - Generate/Update atlas.sum
# Pattern: migrations-atlas/docker/scripts/hash.sh

set -euo pipefail

MIGRATION_DIR="${MIGRATION_DIR:-/migrations}"

BLUE='\033[0;34m'
RED='\033[0;31m'
GREEN='\033[0;32m'
NC='\033[0m'

log_info() { echo -e "${BLUE}[INFO]${NC} $*"; }
log_error() { echo -e "${RED}[ERROR]${NC} $*"; }
log_success() { echo -e "${GREEN}[SUCCESS]${NC} $*"; }

main() {
    log_info "Atlas Migration Hash Generator for Knowledge Sovereign"

    if [ ! -d "$MIGRATION_DIR" ]; then
        log_error "Migration directory not found: $MIGRATION_DIR"
        exit 1
    fi

    log_info "Generating atlas.sum checksum file..."

    atlas migrate hash \
        --dir "file://$MIGRATION_DIR" || {
            log_error "Failed to generate atlas.sum"
            exit 1
        }

    log_success "atlas.sum generated successfully"

    if [ -f "$MIGRATION_DIR/atlas.sum" ]; then
        log_info "Hash file content (last 10 lines):"
        tail -n 10 "$MIGRATION_DIR/atlas.sum"
    fi
}

main "$@"
