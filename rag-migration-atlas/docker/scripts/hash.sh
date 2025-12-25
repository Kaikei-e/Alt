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
