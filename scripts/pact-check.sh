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

# Check FFI library for Go/Rust pact tests. Accept any of:
#   - already-on-LD_LIBRARY_PATH
#   - $HOME/.pact/lib (pact-foundation default)
#   - /usr/local/lib (system install)
check_ffi() {
  if [[ -n "${LD_LIBRARY_PATH:-}" ]] && ls "${LD_LIBRARY_PATH}"/libpact_ffi.so &>/dev/null; then
    return 0
  fi
  for dir in "$HOME/.pact/lib" /usr/local/lib; do
    if ls "$dir/libpact_ffi.so" &>/dev/null; then
      export LD_LIBRARY_PATH="$dir${LD_LIBRARY_PATH:+:$LD_LIBRARY_PATH}"
      export CGO_LDFLAGS="-L$dir"
      return 0
    fi
  done
  return 1
}

# ---------- Broker mode setup ----------
if [[ "$MODE" == "broker" ]]; then
  echo "Starting Pact Broker via Docker Compose..."
  docker compose -f compose/compose.yaml -f compose/pact.yaml -p alt up -d pact-broker
  export PACT_BROKER_BASE_URL=http://localhost:9292
  export PACT_BROKER_USERNAME=pact
  if [[ -r "$REPO_ROOT/secrets/pact_broker_basic_auth_password.txt" ]]; then
    PACT_BROKER_PASSWORD="$(tr -d '\n' < "$REPO_ROOT/secrets/pact_broker_basic_auth_password.txt")"
  else
    PACT_BROKER_PASSWORD="${PACT_BROKER_PASSWORD:-pact}"
  fi
  export PACT_BROKER_PASSWORD
  echo "Waiting for Pact Broker to be healthy..."
  for i in $(seq 1 30); do
    if curl -fsS -u "${PACT_BROKER_USERNAME}:${PACT_BROKER_PASSWORD}" \
        "${PACT_BROKER_BASE_URL}/diagnostic/status/heartbeat" &>/dev/null; then
      echo "Pact Broker is ready."
      break
    fi
    if [[ $i -eq 30 ]]; then
      echo "ERROR: Pact Broker did not start within 30s"
      exit 1
    fi
    sleep 1
  done
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
  run_step "Go: alt-butterfly-facade consumer" \
    bash -c 'cd alt-butterfly-facade && CGO_ENABLED=1 go test -tags=contract ./internal/handler/contract/ -v'
  run_step "Go: auth-hub consumer" \
    bash -c 'cd auth-hub && CGO_ENABLED=1 go test -tags=contract ./internal/adapter/gateway/contract/ -v'
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
        -u "${PACT_BROKER_USERNAME}:${PACT_BROKER_PASSWORD}" \
        -d @"$pact_file" \
        "${PACT_BROKER_BASE_URL}/pacts/provider/${PROVIDER}/consumer/${CONSUMER}/version/${VERSION}"
      # Branch versions API (tags are legacy as of 2021-07; matrix-aware).
      curl -fsS -X PUT \
        -H "Content-Type: application/json" \
        -u "${PACT_BROKER_USERNAME}:${PACT_BROKER_PASSWORD}" \
        "${PACT_BROKER_BASE_URL}/pacticipants/${CONSUMER}/versions/${VERSION}/branches/${BRANCH}"
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
    bash -c 'cd news-creator/app && uv run pytest tests/contract/ -v'
  run_step "Python: recap-subworker provider" \
    bash -c 'cd recap-subworker && uv run pytest tests/contract/ -v'
  run_step "Python: tag-generator provider" \
    bash -c 'cd tag-generator/app && uv run pytest tests/contract/ -v'
  run_step "Python: tts-speaker provider" \
    bash -c 'cd tts-speaker && uv run pytest tests/contract/ -v'
else
  skip_step "Python provider verifications (uv not found)"
fi

if command -v go &>/dev/null && check_ffi; then
  run_step "Go: alt-backend provider" \
    bash -c 'cd alt-backend/app && CGO_ENABLED=1 go test -tags=contract ./driver/contract/ -v'
  run_step "Go: search-indexer provider" \
    bash -c 'cd search-indexer/app && CGO_ENABLED=1 go test -tags=contract -run TestVerifySearchIndexerProviderContracts ./driver/contract/ -v'
else
  skip_step "Go provider verification (go or libpact_ffi not found)"
fi

if command -v cargo &>/dev/null; then
  run_step "Rust: recap-worker provider" \
    bash -c 'cd recap-worker/recap-worker && cargo test --test provider_verification -- --ignored'
else
  skip_step "Rust provider verification (cargo not found)"
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
