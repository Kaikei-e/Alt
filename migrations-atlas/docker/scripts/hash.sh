#!/bin/sh
# Hash Migration Files - Generate/Update atlas.sum
# This script regenerates the atlas.sum checksum file for all migration files

set -euo pipefail

# Configuration
MIGRATION_DIR="${MIGRATION_DIR:-/migrations}"

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

log_error() {
    echo -e "${RED}[ERROR]${NC} $*"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $*"
}

# Main execution
main() {
    log_info "Atlas Migration Hash Generator for Alt RSS Reader"

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

    # Show the hash file content for verification
    if [ -f "$MIGRATION_DIR/atlas.sum" ]; then
        log_info "Hash file content (last 10 lines):"
        tail -n 10 "$MIGRATION_DIR/atlas.sum"
    fi
}

main "$@"
