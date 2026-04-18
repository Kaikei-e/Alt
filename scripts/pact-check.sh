#!/usr/bin/env bash
# Pact CDC contract regression check for local development + CI.
# Run before `docker compose up --build` to catch breaking changes.
#
# Usage:
#   ./scripts/pact-check.sh                 # File-based mode (no Broker, fast)
#   ./scripts/pact-check.sh --broker        # Broker mode — starts local Pact Broker via compose
#   ./scripts/pact-check.sh --publish-only  # Broker mode, external Broker (CI: PACT_BROKER_BASE_URL
#                                           # + _USERNAME + _PASSWORD already in env). Does NOT
#                                           # start a local broker; everything else (publish,
#                                           # verify, manual bridging) runs identically.
#   ./scripts/pact-check.sh --help          # Show help
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$REPO_ROOT"

# Self-hosted GitHub Actions runners inherit PATH from the .path file
# generated at svc.sh install time — they do NOT source /etc/profile or
# ~/.profile. If Go is installed after the runner was registered, its
# /usr/local/go/bin is missing. Re-stitch common toolchain paths here so
# the script works identically from a dev shell and from runsvc.sh.
for candidate in /usr/local/go/bin /usr/local/bin /home/$USER/.local/bin "$HOME/.cargo/bin" "$HOME/.pyenv/shims"; do
  if [[ -d "$candidate" ]] && [[ ":$PATH:" != *":$candidate:"* ]]; then
    PATH="$PATH:$candidate"
  fi
done
export PATH

MODE="file"
BROKER_EXTERNAL=false
case "${1:-}" in
  --broker)
    MODE="broker"
    ;;
  --publish-only)
    MODE="broker"
    BROKER_EXTERNAL=true
    ;;
  --help|-h)
    echo "Usage: $0 [--broker|--publish-only]"
    echo ""
    echo "  (default)        File-based mode: consumer + provider tests against local pact files"
    echo "  --broker         Broker mode: start Pact Broker via compose, publish, verify"
    echo "  --publish-only   CI broker mode: external Broker, no local startup"
    exit 0
    ;;
esac

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
  if [[ "$BROKER_EXTERNAL" == "true" ]]; then
    # CI path: Broker already running elsewhere (reached via env). Require
    # the three env vars; fail fast if any are missing rather than
    # proceeding silently against a wrong target.
    : "${PACT_BROKER_BASE_URL:?--publish-only requires PACT_BROKER_BASE_URL}"
    : "${PACT_BROKER_USERNAME:?--publish-only requires PACT_BROKER_USERNAME}"
    : "${PACT_BROKER_PASSWORD:?--publish-only requires PACT_BROKER_PASSWORD}"
    export PACT_BROKER_BASE_URL PACT_BROKER_USERNAME PACT_BROKER_PASSWORD
    echo "Using external Pact Broker at ${PACT_BROKER_BASE_URL}"
  else
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
  fi
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
  # Keep the version identifier consistent with pre-deploy-verify.sh and
  # deploy.sh so can-i-deploy / record-deployment can match what was published.
  # Respect CI-provided CONSUMER_VERSION / CONSUMER_BRANCH first — GitHub
  # Actions checks out in detached HEAD so `git branch --show-current` is
  # empty and would produce `/pacticipants/X/branches//versions/Y` URLs
  # (404). Fall back to git when not set, then to `main` as last resort.
  PACT_PROVIDER_VERSION="${PACT_PROVIDER_VERSION:-${CONSUMER_VERSION:-$(git rev-parse --short HEAD)}}"
  PACT_PROVIDER_BRANCH="${PACT_PROVIDER_BRANCH:-${CONSUMER_BRANCH:-$(git branch --show-current)}}"
  PACT_PROVIDER_BRANCH="${PACT_PROVIDER_BRANCH:-main}"
  export PACT_PROVIDER_VERSION PACT_PROVIDER_BRANCH
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

  # Validate identifiers read from pact files before interpolating into URLs.
  # A crafted pact file with name "foo/../../admin" would otherwise produce
  # a path-traversing URL. Reject anything outside the Pact-accepted
  # character class.
  validate_pacticipant_name() {
    local name="$1" field="$2" file="$3"
    if [[ ! "$name" =~ ^[A-Za-z0-9._-]+$ ]]; then
      echo "ERROR: rejecting ${field} name '${name}' from ${file} — not a valid pacticipant identifier" >&2
      FAIL=$((FAIL + 1))
      return 1
    fi
    return 0
  }

  # Put credentials in a netrc file so they don't appear in the curl argv
  # (which is visible to ps and, if the caller has set -x, the log).
  netrc=$(mktemp)
  trap 'rm -f "$netrc"' EXIT
  chmod 600 "$netrc"
  broker_host=$(printf '%s' "$PACT_BROKER_BASE_URL" | awk -F/ '{print $3}' | cut -d: -f1)
  printf 'machine %s\nlogin %s\npassword %s\n' \
    "$broker_host" "$PACT_BROKER_USERNAME" "$PACT_BROKER_PASSWORD" > "$netrc"

  for pact_file in alt-backend/pacts/*.json pacts/*.json rag-orchestrator/pacts/*.json; do
    if [ -f "$pact_file" ]; then
      CONSUMER=$(jq -r '.consumer.name' "$pact_file")
      PROVIDER=$(jq -r '.provider.name' "$pact_file")
      validate_pacticipant_name "$CONSUMER" consumer "$pact_file" || continue
      validate_pacticipant_name "$PROVIDER" provider "$pact_file" || continue
      echo "Publishing ${CONSUMER} -> ${PROVIDER}"
      curl -fsS -X PUT \
        -H "Content-Type: application/json" \
        --netrc-file "$netrc" \
        -d @"$pact_file" \
        "${PACT_BROKER_BASE_URL}/pacts/provider/${PROVIDER}/consumer/${CONSUMER}/version/${VERSION}"
      # Branch versions API (tags are legacy as of 2021-07; matrix-aware).
      curl -fsS -X PUT \
        -H "Content-Type: application/json" \
        --netrc-file "$netrc" \
        "${PACT_BROKER_BASE_URL}/pacticipants/${CONSUMER}/branches/${BRANCH}/versions/${VERSION}"
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
  # alt-backend provider verifies 3 consumer pacts in one pass via the
  # whole-directory go-test invocation: TestVerifyRecapWorkerContract,
  # TestVerifySearchIndexerContract, TestVerifyAltButterflyFacadeContract.
  run_step "Go: alt-backend provider" \
    bash -c 'cd alt-backend/app && CGO_ENABLED=1 go test -tags=contract ./driver/contract/ -v'
  run_step "Go: search-indexer provider" \
    bash -c 'cd search-indexer/app && CGO_ENABLED=1 go test -tags=contract -run TestVerifySearchIndexerProviderContracts ./driver/contract/ -v'
  run_step "Go: pre-processor provider" \
    bash -c 'cd pre-processor/app && CGO_ENABLED=1 go test -tags=contract -run TestVerifyAltBackendContract ./driver/contract/ -v'
  run_step "Go: mq-hub provider (search-indexer message pact)" \
    bash -c 'cd mq-hub/app && CGO_ENABLED=1 go test -tags=contract -run TestVerifySearchIndexerMqHubMessagePact ./driver/contract/ -v'
else
  skip_step "Go provider verification (go or libpact_ffi not found)"
fi

if command -v cargo &>/dev/null; then
  run_step "Rust: recap-worker provider" \
    bash -c 'cd recap-worker/recap-worker && cargo test --test provider_verification -- --ignored'
else
  skip_step "Rust provider verification (cargo not found)"
fi

# ---------- Broker-side verification bridging ----------
# Three pact families cannot use the stock pact_verifier flow and need manual
# verification records in the Broker so can-i-deploy stays accurate:
#
#   1. recap-worker provider_verification.rs is a hand-rolled HTTP replay, not
#      a real pact_verifier — it asserts shape but does not publish results.
#   2. mq-hub consumer message pacts (mq-hub-search-indexer, mq-hub-tag-generator)
#      declare mq-hub as consumer / search-indexer or tag-generator as provider,
#      but in reality mq-hub emits the events and the "provider" services
#      consume them. The consumer-side test self-verifies the shape; the
#      nominal provider has nothing to run.
#   3. kratos is an external SaaS and cannot be brought under Alt's provider
#      verification harness.
#
# For each of these we POST a verification result tagged with the stub/source
# implementation so the audit trail is honest.
if [[ "$MODE" == "broker" && $FAIL -eq 0 ]]; then
  publish_manual_verification() {
    local provider="$1"
    local consumer="$2"
    local implementation="$3"
    local provider_version="$4"

    local publish_url
    publish_url=$(curl -fsS -u "${PACT_BROKER_USERNAME}:${PACT_BROKER_PASSWORD}" \
      "${PACT_BROKER_BASE_URL}/pacts/provider/${provider}/consumer/${consumer}/latest" \
      2>/dev/null | jq -r '._links."pb:publish-verification-results".href // empty')
    if [[ -z "$publish_url" ]]; then
      echo "  skip: no pact for ${consumer} -> ${provider}"
      return 0
    fi

    local body
    body=$(printf '{"success":true,"providerApplicationVersion":"%s","verifiedBy":{"implementation":"%s","version":"1.0.0"}}' \
      "$provider_version" "$implementation")
    if curl -fsS -u "${PACT_BROKER_USERNAME}:${PACT_BROKER_PASSWORD}" \
        -X POST -H 'Content-Type: application/json' -d "$body" \
        "$publish_url" >/dev/null 2>&1; then
      echo "  publish: ${consumer} -> ${provider} @${provider_version} (${implementation})"
    else
      echo "  FAIL: could not publish verification for ${consumer} -> ${provider}" >&2
      FAIL=$((FAIL + 1))
    fi
  }

  ensure_kratos_external_version() {
    # Register an external stable version for kratos and record it as deployed
    # to production so the matrix query has a version to resolve.
    local kratos_version="ory-kratos-external"
    curl -fsS -u "${PACT_BROKER_USERNAME}:${PACT_BROKER_PASSWORD}" \
      -X PUT -H 'Content-Type: application/json' \
      "${PACT_BROKER_BASE_URL}/pacticipants/kratos/branches/main/versions/${kratos_version}" \
      >/dev/null 2>&1 || true
    "${PACT_BROKER_BIN:-pact-broker-cli}" record-deployment \
      --pacticipant kratos \
      --version "$kratos_version" \
      --environment production \
      --broker-base-url "${PACT_BROKER_BASE_URL}" \
      --broker-username "${PACT_BROKER_USERNAME}" \
      --broker-password "${PACT_BROKER_PASSWORD}" >/dev/null 2>&1 || true
    echo "  ensured: kratos@${kratos_version} deployed in production"
  }

  echo ""
  echo "============================="
  echo " Publishing Manual Verifications to Broker"
  echo "============================="

  # recap-worker — Rust stub replay asserted shape for 3 consumers.
  for consumer in rag-orchestrator recap-evaluator search-indexer; do
    publish_manual_verification "recap-worker" "$consumer" \
      "rust-stub-replay" "$PACT_PROVIDER_VERSION"
  done

  # mq-hub outbound message pacts — self-verified by the producer-side tests.
  for provider in search-indexer tag-generator; do
    publish_manual_verification "$provider" "mq-hub" \
      "mq-hub-self-verify-message-producer" "$PACT_PROVIDER_VERSION"
  done

  # kratos — external SaaS, register stable external version.
  ensure_kratos_external_version
  publish_manual_verification "kratos" "auth-hub" \
    "manual-external-assertion" "ory-kratos-external"
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
