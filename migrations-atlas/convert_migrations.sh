#!/bin/bash
# Convert existing SQL migrations to Atlas format
# Kubernetes-native migration conversion script

set -euo pipefail

SOURCE_DIR="/Alt/db/migrations"
TARGET_DIR="/Alt/migrations-atlas/migrations"
ATLAS_VERSION="v0.35"

# Colors for output
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

# Check if source directory exists
if [ ! -d "$SOURCE_DIR" ]; then
    log_error "Source migration directory not found: $SOURCE_DIR"
    exit 1
fi

# Create target directory
mkdir -p "$TARGET_DIR"

log_info "Converting migrations from $SOURCE_DIR to $TARGET_DIR"
log_info "Target format: Atlas-compatible SQL migrations"

# Counter for sequential numbering
counter=1

# Process each migration file
for migration_file in "$SOURCE_DIR"/*.up.sql; do
    if [ ! -f "$migration_file" ]; then
        continue
    fi

    # Extract migration info
    basename=$(basename "$migration_file" .up.sql)
    migration_number=$(echo "$basename" | cut -d'_' -f1)
    migration_name=$(echo "$basename" | cut -d'_' -f2- | tr '_' ' ')

    # Skip migration 000036 (doesn't exist)
    if [ "$migration_number" = "000036" ]; then
        log_warn "Skipping non-existent migration 000036"
        continue
    fi

    # Generate timestamp-based filename for Atlas
    timestamp=$(printf "202401%02d%06d" $((counter / 100 + 1)) $((counter * 100)))
    target_filename="${timestamp}_${basename#*_}.sql"
    target_path="$TARGET_DIR/$target_filename"

    log_info "Converting: $basename -> $target_filename"

    # Read original migration content
    content=$(cat "$migration_file")

    # Fix CONCURRENTLY issues for transaction safety
    content=$(echo "$content" | sed 's/CREATE INDEX CONCURRENTLY/CREATE INDEX/g')
    content=$(echo "$content" | sed 's/DROP INDEX CONCURRENTLY/DROP INDEX/g')

    # Create Atlas-format migration
    cat > "$target_path" << EOF
-- Migration: $migration_name
-- Created: $(date '+%Y-%m-%d %H:%M:%S')
-- Atlas Version: $ATLAS_VERSION
-- Source: $basename.up.sql

$content
EOF

    log_success "Converted: $target_filename"
    ((counter++))
done

log_info "Migration conversion completed"
log_info "Total migrations converted: $((counter - 1))"

# Create Atlas migration hash file
log_info "Generating Atlas migration hash..."
cd "$TARGET_DIR/.."
if command -v atlas >/dev/null 2>&1; then
    atlas migrate hash --dir "file://migrations" || log_warn "Atlas hash generation failed - will be done in container"
else
    log_warn "Atlas CLI not found locally - hash will be generated in container"
fi

log_success "Migration conversion process completed!"
echo
echo "Next steps:"
echo "1. Review converted migrations in: $TARGET_DIR"
echo "2. Build Atlas migration container: docker build -t alt-migrations:latest migrations-atlas/docker/"
echo "3. Deploy with Helm chart using pre-upgrade hooks"