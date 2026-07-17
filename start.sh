#!/bin/bash

# Enhanced startup script with dynamic service detection
set -euo pipefail

echo "=== Rask-Log-Forwarder Dynamic Startup ==="

# Get Docker group ID dynamically (fail-fast; compose has no fallback GID)
DOCKER_GROUP_ID="$(./scripts/get-docker-gid.sh)"
echo "✅ Using Docker group ID: $DOCKER_GROUP_ID"
export DOCKER_GROUP_ID

# Function to detect actual container name for a service
detect_target_service() {
    local base_service="$1"
    local target_name

    # Use the detection script
    if [ -f "./rask-log-forwarder/scripts/detect-target-service.sh" ]; then
        target_name=$(./rask-log-forwarder/scripts/detect-target-service.sh "$base_service")
    else
        # Fallback: direct Docker query
        target_name=$(docker ps --format "{{.Names}}" | grep -E "(^|-)${base_service}(-|$)" | head -1)
        if [ -z "$target_name" ]; then
            target_name="$base_service"
        fi
    fi

    echo "$target_name"
}

# Dynamically set target services
echo "🔍 Detecting target services..."

# For alt-backend (the problematic one)
ALT_BACKEND_TARGET=$(detect_target_service "alt-backend")
export ALT_BACKEND_TARGET
echo "  • alt-backend → $ALT_BACKEND_TARGET"

# For other services (optional, for future expansion)
ALT_FRONTEND_TARGET=$(detect_target_service "alt-frontend")
export ALT_FRONTEND_TARGET
echo "  • alt-frontend → $ALT_FRONTEND_TARGET"

NGINX_TARGET=$(detect_target_service "nginx")
export NGINX_TARGET
echo "  • nginx → $NGINX_TARGET"

TAG_GENERATOR_TARGET=$(detect_target_service "tag-generator")
export TAG_GENERATOR_TARGET
echo "  • tag-generator → $TAG_GENERATOR_TARGET"

PRE_PROCESSOR_TARGET=$(detect_target_service "pre-processor")
export PRE_PROCESSOR_TARGET
echo "  • pre-processor → $PRE_PROCESSOR_TARGET"

SEARCH_INDEXER_TARGET=$(detect_target_service "search-indexer")
export SEARCH_INDEXER_TARGET
echo "  • search-indexer → $SEARCH_INDEXER_TARGET"

NEWS_CREATOR_TARGET=$(detect_target_service "news-creator")
export NEWS_CREATOR_TARGET
echo "  • news-creator → $NEWS_CREATOR_TARGET"

echo ""
echo "🚀 Starting docker compose with logging profile..."

# Start docker compose with the logging profile
docker compose --profile logging up -d "$@"

echo ""
echo "✅ Rask-Log-Forwarder started successfully!"
