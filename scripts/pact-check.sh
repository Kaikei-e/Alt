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
SERVICE_FILTER=""
ROLE_FILTER=""
DRY_RUN=false
MANUAL_ONLY=false
SKIP_MANUAL_BRIDGE=false
while [[ $# -gt 0 ]]; do
  case "$1" in
    --broker)
      MODE="broker"
      shift
      ;;
    --publish-only)
      MODE="broker"
      BROKER_EXTERNAL=true
      shift
      ;;
    --services)
      SERVICE_FILTER="$2"
      shift 2
      ;;
    --services=*)
      SERVICE_FILTER="${1#--services=}"
      shift
      ;;
    --role)
      ROLE_FILTER="$2"
      shift 2
      ;;
    --role=*)
      ROLE_FILTER="${1#--role=}"
      shift
      ;;
    --dry-run)
      DRY_RUN=true
      shift
      ;;
    --publish-manual-verifications)
      MANUAL_ONLY=true
      shift
      ;;
    --skip-manual-bridge)
      SKIP_MANUAL_BRIDGE=true
      shift
      ;;
    --help|-h)
      cat <<'EOF'
Usage: pact-check.sh [flags]

  (default)                 File-based mode: all consumer + provider tests
  --broker                  Broker mode: start local Pact Broker, publish, verify
  --publish-only            CI broker mode: external Broker, no local startup
  --services svc1,svc2      Limit run_step entries to labels matching these
                            service names. With --role, matches are strict on
                            the parsed service token; without --role, falls
                            back to substring (legacy).
  --role consumer|provider  Tighten matching so only labels of that role run.
                            The label is parsed as "<Lang>: <svc> <role>[ ...]"
                            and compared exactly. Guarantees that, e.g.,
                            --services search-indexer --role provider does NOT
                            accidentally drag in
                            "Go: mq-hub provider (search-indexer message pact)".
  --publish-manual-verifications
                            Run only the verifications that emit bridging
                            evidence, then the broker-side bridging block.
                            Every other run_step entry is skipped. Used by
                            the per-service deploy matrix to post bridging
                            records from a single dedicated leg — the
                            evidence has to be produced in the same
                            invocation, so the leg cannot post a record for
                            a verification it did not run.
  --skip-manual-bridge      Do not run the broker-side manual-verification
                            bridging block. For callers that know a dedicated
                            --publish-manual-verifications leg already covers
                            it, so the ~20 per-service publish legs of a
                            release stop replaying the same playbook. Ignored
                            when --publish-manual-verifications is given.
  --dry-run                 Print "WOULD RUN: <label>" for each step that
                            would execute, and "WOULD POST MANUAL
                            VERIFICATION: <provider>/<consumer>" for each
                            manual bridging record. Does not touch the
                            Broker, Go, Rust, or Python toolchains. Intended
                            for filter-behaviour tests.
EOF
      exit 0
      ;;
    *)
      echo "unknown flag: $1" >&2
      exit 2
      ;;
  esac
done

# Validate --role after parsing so we can emit a clear error.
case "$ROLE_FILTER" in
  ""|consumer|provider) ;;
  *)
    echo "invalid --role: $ROLE_FILTER (expected consumer|provider)" >&2
    exit 2
    ;;
esac

PASS=0
FAIL=0
SKIP=0

# ---------- Verification evidence ----------
# The manual-verification bridge (playbooks/publish-manual-verifications.yml)
# refuses to publish a success record for a (provider, consumer) pair unless
# the verifier that ran in THIS invocation left an alt.pact.evidence.v1
# document in $PACT_EVIDENCE_DIR. $PACT_EVIDENCE_RUN_ID binds each document to
# this run, and the purge below stops a document from an earlier run being
# replayed as if it were fresh. Both are exported so verifiers pick them up
# without per-step plumbing.
PACT_EVIDENCE_DIR="${PACT_EVIDENCE_DIR:-$REPO_ROOT/.pact-evidence}"
PACT_EVIDENCE_RUN_ID="${PACT_EVIDENCE_RUN_ID:-$(date -u +%Y%m%dT%H%M%SZ)-$$-${RANDOM}}"
export PACT_EVIDENCE_DIR PACT_EVIDENCE_RUN_ID
mkdir -p "$PACT_EVIDENCE_DIR"
rm -f "$PACT_EVIDENCE_DIR"/*__*.json

# Return 0 if the step label should execute under the current --services and
# --role filters. Label convention (parsed here):
#
#   "<Lang>: <service> [<qualifier>] <role>[ <extras>]"
#
# e.g. "Go: alt-backend consumer", "Rust: recap-worker provider",
# "Go: mq-hub provider (search-indexer message pact)",
# "Go: altctl sovereign consumer" (qualifier disambiguates a service with
# more than one consumer/provider contract).
#
# Semantics:
#   - Empty --services + empty --role → run everything (legacy default).
#   - --role set → require the parsed role token to match. Labels that do not
#     parse (e.g. free-form headers) are rejected in strict role mode.
#   - --services set + --role set → exact match on parsed service token.
#   - --services set + no --role → legacy substring match on the full label.
#   - --publish-manual-verifications → run only the steps that produce
#     bridging evidence; skip everything else at this layer. The manual
#     block at the bottom then reads what those steps wrote.
should_run_service_filter() {
  local label="$1"
  local produces_evidence="${2:-}"

  # --publish-manual-verifications short-circuits: the only steps worth
  # running are the ones whose output the bridging block consumes. Skipping
  # those too would leave the block with nothing to read, and "publish a
  # success record for a verification this run never executed" is exactly
  # the failure this gate removes.
  if [[ "$MANUAL_ONLY" == "true" ]]; then
    [[ "$produces_evidence" == "evidence" ]] && return 0
    return 1
  fi

  local label_svc="" label_role=""
  if [[ "$label" =~ ^[A-Za-z]+:\ ([a-z0-9][a-z0-9-]*)\ ([a-z0-9-]+\ )?(consumer|provider)(\ .*)?$ ]]; then
    label_svc="${BASH_REMATCH[1]}"
    label_role="${BASH_REMATCH[3]}"
  fi

  if [[ -n "$ROLE_FILTER" ]]; then
    [[ -z "$label_role" || "$label_role" != "$ROLE_FILTER" ]] && return 1
  fi

  [[ -z "$SERVICE_FILTER" ]] && return 0

  local IFS=','
  for svc in $SERVICE_FILTER; do
    [[ -z "$svc" ]] && continue
    if [[ -n "$ROLE_FILTER" && -n "$label_svc" ]]; then
      [[ "$label_svc" == "$svc" ]] && return 0
    else
      [[ "$label" == *"$svc"* ]] && return 0
    fi
  done
  return 1
}

run_step() {
  local label="$1"
  local produces_evidence="$2"
  shift 2
  if ! should_run_service_filter "$label" "$produces_evidence"; then
    if [[ "$DRY_RUN" != "true" ]]; then
      echo ""
      echo "=== SKIP (filter: --services ${SERVICE_FILTER} --role ${ROLE_FILTER}): ${label} ==="
    fi
    SKIP=$((SKIP + 1))
    return 0
  fi
  if [[ "$DRY_RUN" == "true" ]]; then
    echo "WOULD RUN: ${label}"
    PASS=$((PASS + 1))
    return 0
  fi
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
# In --dry-run we skip every network and broker interaction — the dry run
# exists to exercise filter logic only. PACT_PROVIDER_VERSION is still
# initialised because the manual-verification bridging block interpolates it.
if [[ "$DRY_RUN" == "true" ]]; then
  PACT_PROVIDER_VERSION="${PACT_PROVIDER_VERSION:-dry-run}"
  PACT_PROVIDER_BRANCH="${PACT_PROVIDER_BRANCH:-main}"
  export PACT_PROVIDER_VERSION PACT_PROVIDER_BRANCH
fi

if [[ "$MODE" == "broker" && "$DRY_RUN" != "true" ]]; then
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
  # Bound every probe: without --connect-timeout/--max-time a black-holed
  # broker host makes each curl block on TCP connect for the kernel default
  # (~2min), so a nominal "30s" loop silently hangs for over an hour before
  # failing. A wall-clock deadline keeps the wait honest in every failure mode.
  broker_wait="${PACT_BROKER_WAIT_SECONDS:-30}"
  echo "Waiting for Pact Broker at ${PACT_BROKER_BASE_URL} to be healthy (up to ${broker_wait}s)..."
  broker_wait_start=$SECONDS
  until curl -fsS --connect-timeout 3 --max-time 5 \
      -u "${PACT_BROKER_USERNAME}:${PACT_BROKER_PASSWORD}" \
      "${PACT_BROKER_BASE_URL}/diagnostic/status/heartbeat" &>/dev/null; do
    if (( SECONDS - broker_wait_start >= broker_wait )); then
      echo "ERROR: Pact Broker at ${PACT_BROKER_BASE_URL} not healthy within ${broker_wait}s" >&2
      exit 1
    fi
    sleep 1
  done
  echo "Pact Broker is ready."
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

# ---------- Toolchain detection + step registry ----------
# Detect each language/runtime once at script start so the per-step loop
# below can skip cleanly without re-shelling command(1) 18 times.
# Reference: Wooledge BashFAQ #105 on set-e reliability; Google Shell Style
# Guide on using arrays over ad-hoc control flow.
declare -A HAVE=()
HAVE[go]=0
HAVE[cargo]=0
HAVE[uv]=0
if command -v go &>/dev/null && check_ffi; then HAVE[go]=1; fi
if command -v cargo &>/dev/null; then HAVE[cargo]=1; fi
if command -v uv &>/dev/null; then HAVE[uv]=1; fi

# In --dry-run every toolchain is treated as present so the filter/output
# tests are deterministic regardless of runner host state.
need_tool() {
  [[ "$DRY_RUN" == "true" ]] && return 0
  [[ "${HAVE[$1]:-0}" == "1" ]]
}

# Step registry. Each entry is `label|tool|workdir|command[|evidence]`
# split on `|` (no command contains a pipe, so the optional trailing field
# parses unambiguously).
# - label: exact run_step label; parsed by should_run_service_filter as
#   "<Lang>: <svc> <role>[ <extras>]".
# - tool: key in $HAVE (go / cargo / uv). Also the skip_step suffix.
# - workdir: relative from repo root.
# - command: shell fragment executed via `bash -c` inside workdir.
# - evidence: literal "evidence" if the step writes an alt.pact.evidence.v1
#   document that a bridge_registry row depends on. Those steps also run
#   under --publish-manual-verifications.
# Mirroring consumer and provider tables keeps the script linear and
# removes the six language-gate `if/else` blocks the pre-refactor version
# had (one per lang × role combination).
STEPS_CONSUMER=(
  "Go: alt-backend consumer|go|alt-backend/app|CGO_ENABLED=1 go test -tags=contract ./orchestrator/driver/preprocessor_connect/contract/ -v"
  "Go: alt-backend sovereign consumer|go|alt-backend/app|CGO_ENABLED=1 go test -tags=contract ./shared/driver/sovereign_client/contract/ -v"
  "Go: pre-processor consumer|go|pre-processor/app|CGO_ENABLED=1 go test -tags=contract ./driver/contract/ -v"
  "Go: rag-orchestrator consumer|go|rag-orchestrator|CGO_ENABLED=1 go test -tags=contract ./internal/adapter/contract/ -v"
  "Go: search-indexer consumer|go|search-indexer/app|CGO_ENABLED=1 go test -tags=contract ./driver/contract/ -v"
  "Go: mq-hub consumer|go|mq-hub/app|CGO_ENABLED=1 go test -tags=contract ./driver/contract/ -v"
  "Go: alt-butterfly-facade consumer|go|alt-butterfly-facade|CGO_ENABLED=1 go test -tags=contract ./internal/handler/contract/ -v"
  "Go: altctl sovereign consumer|go|altctl|CGO_ENABLED=1 go test -tags=contract ./internal/sovereignclient/contract/ -v"
  "Rust: recap-worker consumer|cargo|recap-worker/recap-worker|cargo test --lib contract -- --ignored"
  "Python: recap-evaluator consumer|uv|recap-evaluator|uv run pytest tests/contract/ -v --no-cov"
  "Python: tag-generator consumer|uv|tag-generator/app|uv run pytest tests/contract/test_mqhub_tags_consumer.py tests/contract/test_datahub_consumer.py -v --no-cov"
)

STEPS_PROVIDER=(
  "Python: news-creator provider|uv|news-creator/app|uv run pytest tests/contract/ -v"
  "Python: recap-subworker provider|uv|recap-subworker|uv run pytest tests/contract/ -v"
  "Python: tag-generator provider|uv|tag-generator/app|uv run pytest tests/contract/ -v"
  "Python: tts-speaker provider|uv|tts-speaker|uv run pytest tests/contract/ -v"
  "Go: alt-backend provider|go|alt-backend/app|CGO_ENABLED=1 go test -tags=contract ./dataplane/driver/contract/ -v"
  "Go: search-indexer provider|go|search-indexer/app|CGO_ENABLED=1 go test -tags=contract -run TestVerifySearchIndexerProviderContracts ./driver/contract/ -v"
  "Go: pre-processor provider|go|pre-processor/app|CGO_ENABLED=1 go test -tags=contract -run TestVerifyAltBackendContract ./driver/contract/ -v"
  "Go: mq-hub provider (search-indexer message pact)|go|mq-hub/app|CGO_ENABLED=1 go test -tags=contract -run TestVerifySearchIndexerMqHubMessagePact ./driver/contract/ -v"
  "Go: mq-hub provider (tag-generator message pact)|go|mq-hub/app|CGO_ENABLED=1 go test -tags=contract -run TestVerifyTagGeneratorMqHubMessagePact ./driver/contract/ -v"
  "Go: knowledge-sovereign provider|go|knowledge-sovereign/app|CGO_ENABLED=1 go test -tags=contract ./driver/contract/ -v"
  "Rust: recap-worker provider|cargo|recap-worker/recap-worker|cargo test --test provider_verification -- --ignored|evidence"
)

# Iterate a STEPS_* registry. Uses a nameref so the caller passes a bare
# identifier (`execute_steps STEPS_CONSUMER`) instead of a resolved array.
execute_steps() {
  local -n steps="$1"
  local spec label tool wd cmd evidence
  for spec in "${steps[@]}"; do
    IFS='|' read -r label tool wd cmd evidence <<<"$spec"
    if ! need_tool "$tool"; then
      skip_step "$label (missing $tool toolchain)"
      continue
    fi
    run_step "$label" "$evidence" bash -c "cd \"$wd\" && $cmd"
  done
}

# ---------- Consumer tests ----------
echo ""
echo "============================="
echo " Consumer Tests (pact generation)"
echo "============================="
execute_steps STEPS_CONSUMER

# ---------- Broker publish (broker mode only) ----------
#
# Publish scope is restricted to pact files whose .consumer.name matches the
# --services CSV filter. Running in dry-run or without a filter, every
# candidate file is listed (and, in non-dry-run, published).
#
# This is intentional partitioning for parallel CI matrix legs: each
# (service, consumer-role) leg owns exactly the pacts where it IS the
# consumer, preventing cross-leg republish races that trigger HTTP 409 on
# the Pact Broker's publish-contracts endpoint. Provider-role legs don't
# generate pact content, so they skip publish entirely.
#
# The publish call itself uses `pact-broker-cli publish --merge`, which
# handles concurrent publish of identical pacts as a no-op and merges
# interactions when content differs slightly (documented behaviour for
# "running Pact tests concurrently on different build nodes" in the
# pact-broker-cli README).
should_publish_pact_file() {
  local consumer_name="$1"
  [[ -z "$SERVICE_FILTER" ]] && return 0
  local IFS=','
  for svc in $SERVICE_FILTER; do
    [[ -z "$svc" ]] && continue
    [[ "$consumer_name" == "$svc" ]] && return 0
  done
  return 1
}

if [[ "$MODE" == "broker" && "$ROLE_FILTER" != "provider" && "$MANUAL_ONLY" != "true" && $FAIL -eq 0 ]]; then
  # --publish-manual-verifications runs in broker mode (MODE=broker set by
  # --publish-only) but must NOT touch consumer pact files. The alt-deploy
  # pact-manual job 2026-04-20 crashed with "Cannot change the content of
  # the pact for recap-worker version ... and provider alt-backend" because
  # this block was re-publishing the consumer pacts at the same SHA as the
  # upstream pact-publish job, and Pact libraries that use type matchers
  # without concrete examples generate non-deterministic bodies that the
  # Broker rejects as content mutation. Manual bridging only POSTs
  # verification records (block at ~L474), never consumer pacts.
  #
  # The $FAIL -eq 0 gate prevents a second failure mode seen on
  # 2026-04-21: if consumer tests fail (e.g. torch-sys network flake
  # on a fresh runner), the workspace still carries the last committed
  # pact file on disk and this block would publish that stale snapshot.
  # The next green run then tries to publish the freshly generated
  # pact at the same SHA and the Broker rejects it as content mutation.
  # Skipping publish on any consumer-test failure keeps the Broker in
  # sync with what the code at $SHA actually produces.
  if [[ "$DRY_RUN" != "true" ]]; then
    echo ""
    echo "============================="
    echo " Publishing pacts to Broker"
    echo "============================="
  fi
  VERSION="${PACT_PROVIDER_VERSION:-}"
  BRANCH="${PACT_PROVIDER_BRANCH:-}"

  # Validate identifiers read from pact files. A crafted pact file with
  # name "foo/../../admin" would otherwise flow through to the Broker CLI.
  # Reject anything outside the Pact-accepted character class.
  validate_pacticipant_name() {
    local name="$1" field="$2" file="$3"
    if [[ ! "$name" =~ ^[A-Za-z0-9._-]+$ ]]; then
      echo "ERROR: rejecting ${field} name '${name}' from ${file} — not a valid pacticipant identifier" >&2
      FAIL=$((FAIL + 1))
      return 1
    fi
    return 0
  }

  declare -a FILES_TO_PUBLISH=()
  for pact_file in alt-backend/pacts/*.json pacts/*.json rag-orchestrator/pacts/*.json altctl/pacts/*.json; do
    [ -f "$pact_file" ] || continue
    CONSUMER=$(jq -r '.consumer.name' "$pact_file")
    PROVIDER=$(jq -r '.provider.name' "$pact_file")
    validate_pacticipant_name "$CONSUMER" consumer "$pact_file" || continue
    validate_pacticipant_name "$PROVIDER" provider "$pact_file" || continue
    if ! should_publish_pact_file "$CONSUMER"; then
      continue
    fi
    if [[ "$DRY_RUN" == "true" ]]; then
      echo "WOULD PUBLISH: consumer=${CONSUMER} provider=${PROVIDER} file=${pact_file}"
    fi
    FILES_TO_PUBLISH+=("$pact_file")
  done

  if [[ "$DRY_RUN" != "true" ]]; then
    if [[ ${#FILES_TO_PUBLISH[@]} -eq 0 ]]; then
      echo "No pact files matching --services '${SERVICE_FILTER}' — skipping publish"
    else
      PACT_BROKER_BIN="${PACT_BROKER_BIN:-pact-broker-cli}"
      # --merge covers the concurrent-publish race documented in
      # pact-broker-cli: "If a pact already exists for this consumer version
      # and provider, merge the contents. Useful when running Pact tests
      # concurrently on different build nodes."
      publish_args=(
        publish "${FILES_TO_PUBLISH[@]}"
        --broker-base-url "$PACT_BROKER_BASE_URL"
        --broker-username "$PACT_BROKER_USERNAME"
        --broker-password "$PACT_BROKER_PASSWORD"
        --consumer-app-version "$VERSION"
        --merge
      )
      if [[ -n "$BRANCH" ]]; then
        publish_args+=(--branch "$BRANCH")
      fi
      if [[ -n "${GITHUB_SERVER_URL:-}" && -n "${GITHUB_REPOSITORY:-}" && -n "${GITHUB_RUN_ID:-}" ]]; then
        publish_args+=(--build-url "${GITHUB_SERVER_URL}/${GITHUB_REPOSITORY}/actions/runs/${GITHUB_RUN_ID}")
      fi
      "$PACT_BROKER_BIN" "${publish_args[@]}"
      echo "Published ${#FILES_TO_PUBLISH[@]} pact files to Broker"
    fi
  fi
elif [[ "$MODE" == "broker" && "$ROLE_FILTER" != "provider" && "$MANUAL_ONLY" != "true" && $FAIL -gt 0 ]]; then
  echo ""
  echo "=== SKIP: broker publish (${FAIL} consumer test(s) failed — refusing to publish potentially stale on-disk pacts) ==="
fi

# ---------- Provider verifications ----------
echo ""
echo "============================="
echo " Provider Verifications"
echo "============================="

execute_steps STEPS_PROVIDER

# ---------- Broker-side verification bridging ----------
# recap-worker's provider_verification.rs is a hand-rolled HTTP replay, not a
# real pact_verifier — it asserts shape but does not publish results. It does
# write an alt.pact.evidence.v1 document per replayed pact, which is what the
# bridge posts against. It is the only remaining bridged pacticipant.
#
# Two entries were removed rather than fixed:
#
#   - alt:events:tags declared mq-hub as consumer and tag-generator as
#     provider, which is backwards — mq-hub writes that stream. Both inverted
#     pacts are gone. The correct directions, search-indexer-mq-hub.json and
#     tag-generator-mq-hub.json, are verified for real by
#     TestVerifySearchIndexerMqHubMessagePact and
#     TestVerifyTagGeneratorMqHubMessagePact.
#   - kratos ← auth-hub was dropped entirely. Pact detects drift by holding a
#     provider team to a consumer's expectations; Ory neither sees nor answers
#     this repo's pacts, so the pair could only ever replay a pinned image
#     against itself. Two of its three interactions turned out to assert
#     behaviour Kratos cannot produce (traits.role is forbidden by
#     kratos/identity.schema.json; Kratos OSS has no rate limiter on
#     /sessions/whoami). auth-hub's assumptions about that response are
#     covered by httptest cases in internal/adapter/gateway/kratos_test.go,
#     which is the right tool for an upstream we consume but do not drive.
#
# The playbook publishes a success record only for rows whose verifier left
# evidence in $PACT_EVIDENCE_DIR carrying $PACT_EVIDENCE_RUN_ID. Rows without
# it publish nothing, are named in the output, and exit the playbook non-zero.
#
# Opting out is explicit (--skip-manual-bridge) rather than inferred from the
# presence of --services/--role: verify-pact-on-demand.yaml passes --services
# without --role and has no other path that posts a verification record for
# recap-worker / tag-generator, so inferring would make it report green while
# publishing nothing.
if [[ "$MANUAL_ONLY" == "true" ]] || \
   [[ "$MODE" == "broker" && $FAIL -eq 0 && "$SKIP_MANUAL_BRIDGE" != "true" ]]; then
  echo ""
  echo "============================="
  echo " Publishing Manual Verifications to Broker"
  echo "============================="

  # Delegate to the Ansible playbook. Bridge registry, evidence gate, broker
  # iteration, and credential handling all live in
  # playbooks/publish-manual-verifications.yml so the logic is data-driven,
  # uri-basic-auth hides creds from argv (shell curl -u leaked them into ps
  # listings), and every sensitive task is wrapped in no_log per security
  # audit F-001 / F-002.
  #
  # --dry-run passes bridge_dry_run=true so the playbook prints the plan and
  # its evidence verdict per row without touching the Broker. Broker vars may
  # legitimately be unset on that path, hence the :- defaults.
  ansible_extra_args=()
  if [[ "$DRY_RUN" == "true" ]]; then
    ansible_extra_args+=(--check -e bridge_dry_run=true)
  fi

  if PACT_BROKER_PASSWORD="${PACT_BROKER_PASSWORD:-}" \
     ansible-playbook -i localhost, -c local \
       "${ansible_extra_args[@]}" \
       -e provider_version="${PACT_PROVIDER_VERSION:-}" \
       -e broker_base_url="${PACT_BROKER_BASE_URL:-}" \
       -e broker_username="${PACT_BROKER_USERNAME:-}" \
       -e pact_evidence_dir="${PACT_EVIDENCE_DIR}" \
       -e pact_evidence_run_id="${PACT_EVIDENCE_RUN_ID}" \
       "${REPO_ROOT}/playbooks/publish-manual-verifications.yml"; then
    :
  else
    echo "  FAIL: manual verification publish playbook exited non-zero" >&2
    FAIL=$((FAIL + 1))
  fi
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
