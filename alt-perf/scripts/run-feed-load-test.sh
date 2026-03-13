#!/bin/bash
# run-feed-load-test.sh - Orchestrate feed registration load test
#
# Phases:
#   1. Start mock-rss-server + health check
#   2. Create test users (Deno setup script)
#   3. Run K6 load test
#   4. Teardown (Deno cleanup script)
#   5. Stop mock-rss-server
#
# Usage:
#   ./alt-perf/scripts/run-feed-load-test.sh
#   USER_COUNT=10 FEED_COUNT=5 ./alt-perf/scripts/run-feed-load-test.sh  # smoke

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
cd "$PROJECT_ROOT"

USER_COUNT="${USER_COUNT:-1000}"
FEED_COUNT="${FEED_COUNT:-100}"
DURATION="${DURATION:-60}"
RAMP_UP="${RAMP_UP:-30}"
MOCK_REPLICAS="${MOCK_REPLICAS:-1}"
DB_USER="${POSTGRES_USER:-alt_db_user}"
DB_NAME="${POSTGRES_DB:-alt}"
K6_SCENARIO="/scripts/scenarios/feed-registration.js"
GENERATED_OVERLAY="compose/load-test-generated.yaml"

# Auto-calculate VALIDATE_RATE_LIMIT (per-IP rate in req/s).
# With X-Real-IP forwarded to auth-hub, each VU gets its own rate limiter.
# Per-IP ~2 req/s is sufficient; use 10 req/s for safety margin.
# For small VU counts without IP forwarding, keep USER_COUNT as fallback.
if [ "${USER_COUNT}" -gt 1000 ]; then
  VALIDATE_RATE_LIMIT="${VALIDATE_RATE_LIMIT:-10}"
else
  VALIDATE_RATE_LIMIT="${VALIDATE_RATE_LIMIT:-$USER_COUNT}"
fi
if [ "$VALIDATE_RATE_LIMIT" -lt 10 ]; then
  VALIDATE_RATE_LIMIT=10
fi

# Auto-adjust RAMP_UP for large VU counts to avoid thundering herd
if [ "$USER_COUNT" -gt 1000 ] && [ "$RAMP_UP" -eq 30 ]; then
  RAMP_UP=60
fi

echo "========================================"
echo "  Feed Registration Load Test"
echo "  Users: $USER_COUNT"
echo "  Feeds/user: $FEED_COUNT"
echo "  Duration: ${DURATION}s"
echo "  Ramp-up: ${RAMP_UP}s"
echo "  Mock-rss replicas: $MOCK_REPLICAS"
echo "  Validate rate limit: $VALIDATE_RATE_LIMIT req/s"
echo "========================================"
echo ""

# --- Phase 0: Generate compose overlay with network aliases ---
# DNS bucketing: cap aliases at 1000 to avoid Docker DNS overload
if [ "$USER_COUNT" -gt 1000 ]; then
  ALIAS_COUNT=1000
else
  ALIAS_COUNT="$USER_COUNT"
fi

echo "Phase 0: Generating compose overlay for $ALIAS_COUNT DNS aliases ($USER_COUNT VUs)..."

ALIASES=""
ALLOWED_HOSTS=""
for i in $(seq 1 "$ALIAS_COUNT"); do
  PADDED=$(printf "%0${#ALIAS_COUNT}d" "$i")
  ALIAS="mock-rss-${PADDED}"
  ALIASES="${ALIASES}          - ${ALIAS}"$'\n'
  if [ -n "$ALLOWED_HOSTS" ]; then
    ALLOWED_HOSTS="${ALLOWED_HOSTS},${ALIAS}"
  else
    ALLOWED_HOSTS="${ALIAS}"
  fi
done

# Build nginx volumes and optional k6/resource scaling based on VU count
NGINX_VOLUMES="      - ../alt-perf/k6/config/nginx-loadtest-realip.conf:/etc/nginx/conf.d/realip.conf:ro"
K6_SCALE=""
EXTRA_OVERRIDES=""

if [ "$USER_COUNT" -gt 1000 ]; then
  echo "  Scaling resources for $USER_COUNT VUs: K6 16G/8CPU, PgBouncer pooling, PG 250 conns, backend 100 conns"
  NGINX_VOLUMES="${NGINX_VOLUMES}
      - ../alt-perf/k6/config/nginx-loadtest-10k.conf:/etc/nginx/nginx.conf:ro"
  K6_SCALE="
  k6:
    deploy:
      resources:
        limits:
          memory: 16G
          cpus: '8'"
  EXTRA_OVERRIDES="
  db:
    command: >
      postgres
      -c max_connections=250
      -c shared_buffers=512MB
      -c work_mem=16MB
      -c effective_cache_size=1536MB
      -c log_statement=none
      -c log_connections=off
      -c log_disconnections=off
    deploy:
      resources:
        limits:
          memory: 4G

  pgbouncer:
    environment:
      - DEFAULT_POOL_SIZE=80
      - MAX_DB_CONNECTIONS=200
      - MAX_CLIENT_CONN=2000
      - QUERY_WAIT_TIMEOUT=120
    deploy:
      resources:
        limits:
          memory: 512M

  pgbouncer-kratos:
    environment:
      - DEFAULT_POOL_SIZE=40
      - MAX_DB_CONNECTIONS=80
      - MAX_CLIENT_CONN=500
      - QUERY_WAIT_TIMEOUT=60

  alt-backend:
    environment:
      - FEED_ALLOWED_HOSTS=${ALLOWED_HOSTS}
      - DB_HOST=pgbouncer
      - DB_PORT=6432
      - DB_MAX_CONNS=200
      - DB_MIN_CONNS=10
      - DOS_PROTECTION_RATE_LIMIT=10000
      - DOS_PROTECTION_BURST_LIMIT=20000
      - DOS_PROTECTION_BLOCK_DURATION=10s
      - FEED_FETCH_CONCURRENCY=500
      - CB_MAX_CONCURRENT=500
      - CB_FAILURE_THRESHOLD=1000
      - CB_RESET_TIMEOUT=5s
    deploy:
      resources:
        limits:
          memory: 4G
          cpus: '8.0'

  auth-hub:
    environment:
      - VALIDATE_RATE_LIMIT=${VALIDATE_RATE_LIMIT}
      - CACHE_TTL=30m
    deploy:
      replicas: 3
      resources:
        limits:
          memory: 512M
        reservations:
          memory: 256M

  kratos:
    ports: !reset []
    environment:
      - LOG_LEVEL=warning
    deploy:
      replicas: 3
      resources:
        limits:
          memory: 512M
        reservations:
          memory: 256M

  nginx:
    volumes:
${NGINX_VOLUMES}
    deploy:
      resources:
        limits:
          memory: 1G"
  export TEARDOWN_BATCH_SIZE=200
elif [ "$USER_COUNT" -gt 256 ]; then
  echo "  Scaling resources for $USER_COUNT VUs: K6 4G memory, nginx worker_connections 4096"
  NGINX_VOLUMES="${NGINX_VOLUMES}
      - ../alt-perf/k6/config/nginx-loadtest.conf:/etc/nginx/nginx.conf:ro"
  K6_SCALE="
  k6:
    deploy:
      resources:
        limits:
          memory: 4G"
fi

# For > 1000 VUs, alt-backend + nginx + auth-hub are in EXTRA_OVERRIDES; skip duplicate sections
MOCK_REPLICAS_OVERRIDE=""
if [ "$MOCK_REPLICAS" -gt 1 ]; then
  MOCK_REPLICAS_OVERRIDE="
    ports: !reset []
    deploy:
      replicas: ${MOCK_REPLICAS}"
fi

if [ "$USER_COUNT" -gt 1000 ]; then
cat > "$GENERATED_OVERLAY" <<OVERLAY_EOF
# Generated by run-feed-load-test.sh — DO NOT EDIT
services:
  mock-rss-server:${MOCK_REPLICAS_OVERRIDE}
    networks:
      alt-network:
        aliases:
${ALIASES}
${EXTRA_OVERRIDES}
${K6_SCALE}
OVERLAY_EOF
else
cat > "$GENERATED_OVERLAY" <<OVERLAY_EOF
# Generated by run-feed-load-test.sh — DO NOT EDIT
services:
  mock-rss-server:${MOCK_REPLICAS_OVERRIDE}
    networks:
      alt-network:
        aliases:
${ALIASES}
  alt-backend:
    environment:
      - FEED_ALLOWED_HOSTS=${ALLOWED_HOSTS}

  auth-hub:
    environment:
      - VALIDATE_RATE_LIMIT=${VALIDATE_RATE_LIMIT}

  nginx:
    volumes:
${NGINX_VOLUMES}
${K6_SCALE}
OVERLAY_EOF
fi

echo "  Generated $GENERATED_OVERLAY with $ALIAS_COUNT aliases"

COMPOSE="docker compose -f compose/compose.yaml -f compose/load-test.yaml -f ${GENERATED_OVERLAY} -p alt"

# --- Phase 1: Start mock-rss-server & restart alt-backend with FEED_ALLOWED_HOSTS ---
echo ""
echo "Phase 1: Starting mock-rss-server & restarting services..."
$COMPOSE up -d --build mock-rss-server
if [ "$USER_COUNT" -gt 1000 ]; then
  $COMPOSE up -d --force-recreate db pgbouncer pgbouncer-kratos alt-backend auth-hub kratos nginx
else
  $COMPOSE up -d --force-recreate alt-backend auth-hub nginx
fi

echo "Waiting for mock-rss-server health..."
for i in $(seq 1 30); do
  if $COMPOSE exec -T mock-rss-server wget --spider -q http://localhost:8080/health 2>/dev/null; then
    echo "  mock-rss-server is healthy (${MOCK_REPLICAS} replica(s))"
    break
  fi
  if [ "$i" -eq 30 ]; then
    echo "  ERROR: mock-rss-server failed to start"
    $COMPOSE logs mock-rss-server
    exit 1
  fi
  sleep 1
done

echo "Waiting for kratos health..."
for i in $(seq 1 60); do
  if $COMPOSE exec -T kratos wget -qO- http://127.0.0.1:4434/admin/health/ready > /dev/null 2>&1; then
    echo "  kratos is healthy"
    break
  fi
  if [ "$i" -eq 60 ]; then
    echo "  ERROR: kratos failed to start"
    $COMPOSE logs kratos --tail 20
    exit 1
  fi
  sleep 1
done

echo "Waiting for auth-hub health..."
for i in $(seq 1 30); do
  if $COMPOSE exec -T auth-hub /auth-hub healthcheck > /dev/null 2>&1; then
    echo "  auth-hub is healthy"
    break
  fi
  if [ "$i" -eq 30 ]; then
    echo "  ERROR: auth-hub failed to start"
    $COMPOSE logs auth-hub --tail 20
    exit 1
  fi
  sleep 1
done

echo "Waiting for alt-backend health..."
for i in $(seq 1 60); do
  if curl -sf http://localhost:9000/v1/health > /dev/null 2>&1; then
    echo "  alt-backend is healthy"
    break
  fi
  if [ "$i" -eq 60 ]; then
    echo "  ERROR: alt-backend failed to start"
    $COMPOSE logs alt-backend --tail 20
    exit 1
  fi
  sleep 1
done

# When kratos replicas > 1, host ports are cleared; resolve admin URL via container IP
if [ "$USER_COUNT" -gt 1000 ]; then
  KRATOS_IP=$($COMPOSE exec -T --index=1 kratos hostname -i 2>/dev/null | tr -d '[:space:]')
  if [ -n "$KRATOS_IP" ]; then
    export KRATOS_ADMIN_URL="http://${KRATOS_IP}:4434"
    echo "  Kratos admin URL (container IP): $KRATOS_ADMIN_URL"
  fi
fi

# --- Phase 2: Create test users ---
echo ""
echo "Phase 2: Creating $USER_COUNT test users..."
deno run \
  --allow-net --allow-write --allow-read --allow-env \
  "$SCRIPT_DIR/feed-load-test-setup.ts" \
  --count="$USER_COUNT"

echo ""

# --- Phase 3: Run K6 load test ---
# Configure thresholds for high-VU tests
P95_LIMIT=15000
ERROR_LIMIT=10000
if [ "$USER_COUNT" -gt 1000 ]; then
  P95_LIMIT=30000
  ERROR_LIMIT=100000
fi

echo "Phase 3: Running K6 feed registration load test..."
$COMPOSE run --rm \
  -e FEED_COUNT="$FEED_COUNT" -e DURATION="$DURATION" \
  -e ALIAS_COUNT="$ALIAS_COUNT" \
  -e P95_LIMIT="$P95_LIMIT" -e ERROR_LIMIT="$ERROR_LIMIT" \
  -e RAMP_UP="$RAMP_UP" \
  k6 run "$K6_SCENARIO" && K6_EXIT=0 || K6_EXIT=$?

echo ""
echo "Saving service logs..."
$COMPOSE logs alt-backend --tail 500 > /tmp/alt-backend-loadtest.log 2>&1
$COMPOSE logs auth-hub --tail 500 > /tmp/auth-hub-loadtest.log 2>&1
$COMPOSE logs kratos --tail 500 > /tmp/kratos-loadtest.log 2>&1
$COMPOSE logs nginx --tail 500 > /tmp/nginx-loadtest.log 2>&1
$COMPOSE logs mock-rss-server --tail 500 > /tmp/mock-rss-server-loadtest.log 2>&1

# --- Phase 3b: Collect infrastructure metrics ---
echo "Collecting infrastructure metrics..."

# PgBouncer pool stats (ADR 327 verification)
$COMPOSE exec -T pgbouncer psql -h localhost -p 6432 pgbouncer -c "SHOW POOLS;" \
  > /tmp/pgbouncer-pools-loadtest.log 2>&1 || true
$COMPOSE exec -T pgbouncer psql -h localhost -p 6432 pgbouncer -c "SHOW STATS;" \
  > /tmp/pgbouncer-stats-loadtest.log 2>&1 || true

# DB connection count
$COMPOSE exec -T db psql -U "$DB_USER" -d "$DB_NAME" -c \
  "SELECT count(*) as active_connections FROM pg_stat_activity WHERE state = 'active';" \
  > /tmp/db-connections-loadtest.log 2>&1 || true

# Feed registration count (verification)
$COMPOSE exec -T db psql -U "$DB_USER" -d "$DB_NAME" -c \
  "SELECT count(*) as total_feed_links FROM feed_links WHERE url LIKE 'http://mock-rss-%:8080/%';" \
  > /tmp/feed-count-loadtest.log 2>&1 || true

echo "  Metrics saved to /tmp/*-loadtest.log"

# --- Phase 4: Teardown ---
echo "Phase 4: Cleaning up test data..."
export COMPOSE_CMD="$COMPOSE"
deno run \
  --allow-net --allow-read --allow-write --allow-env --allow-run \
  "$SCRIPT_DIR/feed-load-test-teardown.ts"

echo ""

# --- Phase 5: Stop mock-rss-server & cleanup ---
echo "Phase 5: Stopping mock-rss-server..."
$COMPOSE stop mock-rss-server
$COMPOSE rm -f mock-rss-server

echo "  Restoring services to default config..."
if [ "$USER_COUNT" -gt 1000 ]; then
  docker compose -f compose/compose.yaml -p alt up -d --force-recreate --remove-orphans db pgbouncer pgbouncer-kratos alt-backend auth-hub kratos nginx
else
  docker compose -f compose/compose.yaml -p alt up -d --force-recreate alt-backend auth-hub nginx
fi

echo "  Removing generated overlay..."
rm -f "$GENERATED_OVERLAY"

echo ""
echo "========================================"
echo "  Load test complete (k6 exit: $K6_EXIT)"
echo "  Reports: alt-perf/reports/"
echo "========================================"

exit "$K6_EXIT"
