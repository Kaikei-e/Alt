#!/usr/bin/env bash
# Pact CDC contract regression check for local development.
# Run before `docker compose up --build` to catch breaking changes.
#
# Usage:
#   ./scripts/pact-check.sh            # File-based mode (no Broker, fast)
#   ./scripts/pact-check.sh --broker   # Broker mode (starts Pact Broker via Docker Compose)
#   ./scripts/pact-check.sh --help     # Show help
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$REPO_ROOT"

MODE="file"
if [[ "${1:-}" == "--broker" ]]; then
  MODE="broker"
elif [[ "${1:-}" == "--help" || "${1:-}" == "-h" ]]; then
  echo "Usage: $0 [--broker]"
  echo ""
  echo "  (default)   File-based mode: run consumer + provider tests against local pact files"
  echo "  --broker    Broker mode: start Pact Broker, publish pacts, verify via Broker"
  exit 0
fi

PASS=0
FAIL=0
SKIP=0

run_step() {
  local label="$1"
  shift
  echo ""
  echo "=== ${label} ==="
  if "$@"; then
    PASS=$((PASS + 1))
  else
    echo "FAILED: ${label}"
    FAIL=$((FAIL + 1))
  fi
}

skip_step() {
  echo ""
  echo "=== SKIP: $1 (missing toolchain) ==="
  SKIP=$((SKIP + 1))
}

# Check FFI library for Go/Rust pact tests
check_ffi() {
  if [[ -n "${LD_LIBRARY_PATH:-}" ]] && ls "${LD_LIBRARY_PATH}"/libpact_ffi.so &>/dev/null; then
    return 0
  fi
  if ls "$HOME/.pact/lib/libpact_ffi.so" &>/dev/null; then
    export LD_LIBRARY_PATH="$HOME/.pact/lib${LD_LIBRARY_PATH:+:$LD_LIBRARY_PATH}"
    export CGO_LDFLAGS="-L$HOME/.pact/lib"
    return 0
  fi
  return 1
}

# ---------- Broker mode setup ----------
if [[ "$MODE" == "broker" ]]; then
  echo "Starting Pact Broker via Docker Compose..."
  docker compose -f compose/compose.yaml -f compose/pact.yaml -p alt up -d pact-broker
  echo "Waiting for Pact Broker to be healthy..."
  for i in $(seq 1 30); do
    if curl -fsS http://localhost:9292/diagnostic/status/heartbeat &>/dev/null; then
      echo "Pact Broker is ready."
      break
    fi
    if [[ $i -eq 30 ]]; then
      echo "ERROR: Pact Broker did not start within 30s"
      exit 1
    fi
    sleep 1
  done
  export PACT_BROKER_BASE_URL=http://localhost:9292
  export PACT_BROKER_USERNAME=pact
  export PACT_BROKER_PASSWORD=pact
  export PACT_PROVIDER_VERSION="local-$(git rev-parse --short HEAD)"
  export PACT_PROVIDER_BRANCH="$(git branch --show-current)"
fi

# ---------- Consumer tests ----------
echo ""
echo "============================="
echo " Consumer Tests (pact generation)"
echo "============================="

if command -v go &>/dev/null && check_ffi; then
  run_step "Go: alt-backend consumer" \
    bash -c 'cd alt-backend/app && CGO_ENABLED=1 go test -tags=contract ./driver/preprocessor_connect/contract/ -v'
  run_step "Go: pre-processor consumer" \
    bash -c 'cd pre-processor/app && CGO_ENABLED=1 go test -tags=contract ./driver/contract/ -v'
  run_step "Go: rag-orchestrator consumer" \
    bash -c 'cd rag-orchestrator && CGO_ENABLED=1 go test -tags=contract ./internal/adapter/contract/ -v'
  run_step "Go: search-indexer consumer" \
    bash -c 'cd search-indexer/app && CGO_ENABLED=1 go test -tags=contract ./driver/contract/ -v'
  run_step "Go: mq-hub consumer" \
    bash -c 'cd mq-hub/app && CGO_ENABLED=1 go test -tags=contract ./driver/contract/ -v'
else
  skip_step "Go consumer tests (go or libpact_ffi not found)"
fi

if command -v cargo &>/dev/null; then
  run_step "Rust: recap-worker consumer" \
    bash -c 'cd recap-worker/recap-worker && cargo test --lib contract -- --ignored'
else
  skip_step "Rust consumer tests (cargo not found)"
fi

if command -v uv &>/dev/null; then
  run_step "Python: recap-evaluator consumer" \
    bash -c 'cd recap-evaluator && uv run pytest tests/contract/ -v --no-cov'
else
  skip_step "Python consumer tests (uv not found)"
fi

# ---------- Broker publish (broker mode only) ----------
if [[ "$MODE" == "broker" ]]; then
  echo ""
  echo "============================="
  echo " Publishing pacts to Broker"
  echo "============================="
  VERSION="$PACT_PROVIDER_VERSION"
  BRANCH="$PACT_PROVIDER_BRANCH"
  COUNT=0
  for pact_file in alt-backend/pacts/*.json pacts/*.json rag-orchestrator/pacts/*.json; do
    if [ -f "$pact_file" ]; then
      CONSUMER=$(jq -r '.consumer.name' "$pact_file")
      PROVIDER=$(jq -r '.provider.name' "$pact_file")
      echo "Publishing ${CONSUMER} -> ${PROVIDER}"
      curl -fsS -X PUT \
        -H "Content-Type: application/json" \
        -u "pact:pact" \
        -d @"$pact_file" \
        "http://localhost:9292/pacts/provider/${PROVIDER}/consumer/${CONSUMER}/version/${VERSION}"
      curl -fsS -X PUT \
        -H "Content-Type: application/json" \
        -u "pact:pact" \
        "http://localhost:9292/pacticipants/${CONSUMER}/versions/${VERSION}/tags/${BRANCH}"
      COUNT=$((COUNT + 1))
    fi
  done
  echo "Published ${COUNT} pact files to Broker"
fi

# ---------- Provider verifications ----------
echo ""
echo "============================="
echo " Provider Verifications"
echo "============================="

if command -v uv &>/dev/null; then
  run_step "Python: news-creator provider" \
    bash -c 'cd news-creator/app && SERVICE_SECRET=test-secret uv run pytest tests/contract/ -v'
  run_step "Python: recap-subworker provider" \
    bash -c 'cd recap-subworker && SERVICE_SECRET=test-secret uv run pytest tests/contract/ -v'
  run_step "Python: tag-generator provider" \
    bash -c 'cd tag-generator/app && SERVICE_SECRET=test-secret uv run pytest tests/contract/ -v'
else
  skip_step "Python provider verifications (uv not found)"
fi

if command -v go &>/dev/null && check_ffi; then
  run_step "Go: alt-backend provider" \
    bash -c 'cd alt-backend/app && CGO_ENABLED=1 go test -tags=contract ./driver/contract/ -v'
else
  skip_step "Go provider verification (go or libpact_ffi not found)"
fi

# ---------- Summary ----------
echo ""
echo "============================="
echo " Pact Check Summary"
echo "============================="
echo "  Passed:  ${PASS}"
echo "  Failed:  ${FAIL}"
echo "  Skipped: ${SKIP}"

if [[ $FAIL -gt 0 ]]; then
  echo ""
  echo "Contract regressions detected. Fix before building."
  exit 1
fi

echo ""
echo "All contract checks passed."
