#!/usr/bin/env bash
# Shared lifecycle for every Playwright API suite in the fleet.
#
# Each `e2e/playwright/<service>/run.sh` sources this file and reduces to a
# declaration of what is genuinely its own — its images, its compose services,
# its endpoints, whether it needs mutual-TLS material:
#
#     ROOT="$(cd "$(dirname "$0")/../../.." && pwd)"
#     source "$ROOT/e2e/playwright/_lib/suite.sh"
#
#     suite_init auth-hub
#     suite_image_tags AUTH_HUB_IMAGE_TAG
#     suite_endpoint BASE_URL   "http://auth-hub:9200"
#     suite_endpoint KRATOS_URL "http://kratos:4433"
#     suite_up auth-hub-db auth-hub-db-migrator kratos auth-hub
#     suite_test
#
# Before this library the same 150 lines of compose lifecycle — slice
# rendering, network-pool reclaim, retrying `up`, the failure log dump, secret
# redaction, teardown ordering — were copied into every suite. Copies drift:
# one grows a redaction rule after a security audit and eleven do not; one
# learns to dump logs when `up` fails and eleven still report "connection
# refused" with no evidence. Keeping the lifecycle in one file makes a fix to
# it a fix everywhere.
#
# Ordering contract
# -----------------
# `suite_up` is what renders the compose slice, and `docker compose config`
# bakes environment values in as it resolves. So everything compose.staging.yaml
# interpolates — the `*_IMAGE_TAG` vars and `STAGING_PKI_DIR` — must be set
# before `suite_up` runs. `suite_image_tags` and `suite_pki` therefore go
# between `suite_init` and `suite_up`, and calling them later is a silent
# no-op: the slice was already rendered from the old values.
#
# Environment (read by every suite):
#   NODE_IMAGE           container image for install + run (default: node:22-bookworm-slim)
#   IMAGE_TAG            Docker tag for the service images (default: ci)
#   GHCR_OWNER           GHCR namespace (default: kaikei-e)
#   RUN_ID               unique run identifier (default: $(date +%s))
#   STAGING_PROJECT_NAME compose project + network name (default: alt-staging-<service>)
#   SHARD                Playwright --shard value, N/M (default: unset = whole suite)
#   PW_WORKERS           worker count override (default: the suite's own)
#   PW_BLOB=1            emit a blob report for merge-reports (the CI matrix sets this)
#   PW_GREP              value for --grep, to run a subset
#   PW_STRICT_FLAKE=1    fail the run if any test passed only on retry (nightly)
#   KEEP_STACK=1         do not tear the stack down on exit (for debugging)

SUITE_NAME=""
SUITE_DIR=""
SUITE_PKI_DIR=""
# REPORT_DIR is deliberately NOT initialised here. It is the one variable in
# this file a *caller* is allowed to set — CI exports it so the upload step
# knows where the blob landed without re-deriving suite.sh's naming — and
# `REPORT_DIR=""` at source time silently overwrote it, because run.sh sources
# this file before anything else runs. `suite_init`'s `${REPORT_DIR:-...}`
# then always took the fallback, every suite wrote to
# `e2e/reports/<suite>-<RUN_ID>`, and the workflow uploaded
# `e2e/reports/<suite>/blob`, which never existed. Nine green suites uploaded
# nothing and the merge job reported "no blob reports to merge" as though they
# had all failed to start.
# `-e NAME=value` pairs forwarded into the test container.
declare -a SUITE_ENV_ARGS=()

# ---------------------------------------------------------------------------
# Secret redaction
# ---------------------------------------------------------------------------

# Redact secrets from a log stream.
#
# `docker compose logs` dumps container stdout/stderr verbatim into the CI job
# output, and any error path that logs a request header can carry a staging
# test token. Redact before the bytes hit the job log, not after. The first
# rule matches the 3-segment JWS compact form with the mandatory `eyJ` header
# prefix and the RFC 7515 base64url charset; the second catches a PEM private
# key echoed by a TLS diagnostic.
redact_secrets() {
  sed -E \
    -e 's#eyJ[A-Za-z0-9_=-]+\.eyJ[A-Za-z0-9_=-]+\.[A-Za-z0-9_=-]+#[REDACTED_JWT]#g' \
    -e 's#(-----BEGIN [A-Z ]*PRIVATE KEY-----).*#\1[REDACTED_KEY]#g'
}

# ---------------------------------------------------------------------------
# Lifecycle
# ---------------------------------------------------------------------------

# suite_init <service>
#
# Establishes every path and identifier the rest of the run depends on, sources
# the shared compose helpers, and installs the EXIT trap. Call first.
suite_init() {
  SUITE_NAME="$1"

  : "${NODE_IMAGE:=node:22-bookworm-slim}"
  : "${IMAGE_TAG:=ci}"
  : "${GHCR_OWNER:=kaikei-e}"
  : "${RUN_ID:=$(date +%s)}"
  # A per-service project name by default rather than a bare `alt-staging`: the
  # network is named after the project, so this is what stops two suites — or
  # two shards — from fighting over one network name on a shared daemon.
  : "${STAGING_PROJECT_NAME:=alt-staging-$SUITE_NAME}"

  SUITE_DIR="$ROOT/e2e/playwright/$SUITE_NAME"
  REPORT_DIR="${REPORT_DIR:-$ROOT/e2e/reports/$SUITE_NAME-$RUN_ID}"
  mkdir -p "$REPORT_DIR"

  export IMAGE_TAG GHCR_OWNER STAGING_PROJECT_NAME

  cd "$ROOT"

  # Slice rendering, network reclaim, retrying `up` and PKI minting are shared
  # with what the Hurl suites used; the library is framework-agnostic and moved
  # to e2e/_lib/ when Hurl was retired.
  # shellcheck source=../../_lib/render-slice.sh
  source "$ROOT/e2e/_lib/render-slice.sh"
  # shellcheck source=../../_lib/reclaim-network-pool.sh
  source "$ROOT/e2e/_lib/reclaim-network-pool.sh"
  # shellcheck source=../../_lib/compose-up-with-retry.sh
  source "$ROOT/e2e/_lib/compose-up-with-retry.sh"
  # shellcheck source=./install-suite-deps.sh
  source "$ROOT/e2e/playwright/_lib/install-suite-deps.sh"

  trap suite_cleanup EXIT
}

# suite_image_tags <VAR>...
#
# compose.staging.yaml keys each GHCR image off its own `<SERVICE>_IMAGE_TAG`
# (default `main`) so unrelated dependency services stay on the last successful
# main build even when the dispatch SHA has no rebuild for them. A suite
# forwards its own IMAGE_TAG into the vars for the images it actually builds.
suite_image_tags() {
  local var
  for var in "$@"; do
    export "$var=${!var:-$IMAGE_TAG}"
  done
}

# suite_pki <server-name> <client-name>...
#
# Mints throwaway mutual-TLS material for suites whose service under test sits
# behind an mTLS listener. Must run before `suite_up`: the Go clients read
# their CA at construction and panic if they cannot (CLAUDE.md rules 8/9), so
# material minted after the stack is up is material the stack never saw.
#
# The directory has to sit under e2e/fixtures/ for compose to bind-mount it,
# which also puts private keys one `git add e2e/` away from a commit — hence
# the entries in e2e/.gitignore. It is absolute and suite-scoped so parallel
# matrix jobs never write to the same directory.
suite_pki() {
  SUITE_PKI_DIR="$ROOT/e2e/fixtures/$SUITE_NAME/pki"
  export STAGING_PKI_DIR="$SUITE_PKI_DIR"

  # shellcheck source=../../_lib/mint-staging-pki.sh
  source "$ROOT/e2e/_lib/mint-staging-pki.sh"
  mint_staging_pki "$SUITE_PKI_DIR" "$@"
}

# suite_endpoint <NAME> <default>
#
# Declares a value the specs read from the environment — a URL, a token path, a
# seed identifier. Defaulted here and forwarded into the test container; the
# suite's own `src/env.ts` reads it with `requiredEnv`, so a typo on either
# side is a named error rather than a connection refused on scenario 1.
suite_endpoint() {
  local name="$1" default="$2"
  local value="${!name:-$default}"
  export "$name=$value"
  SUITE_ENV_ARGS+=("-e" "$name=$value")
}

# suite_up <compose service>...
#
# Renders the slice, reclaims the network pool, and brings the stack up —
# dumping logs itself if `up` fails.
#
# The dump cannot be left to the EXIT trap: a dependency that fails during
# `up --wait` (a migrator exiting 1, a stub failing its first import) makes
# compose tear the failing container down *immediately*, so by the time the
# trap runs `docker compose logs` finds nothing and the job's only evidence is
# gone.
suite_up() {
  # Renders a per-project slice (sets $SLICE + $SLICE_DIR) so parallel matrix
  # jobs can coexist on one Docker daemon.
  render_slice "$SUITE_NAME"

  # Reclaim Docker's pre-defined address pool from networks left by cancelled
  # prior runs. Safe by default: `docker network prune` refuses to touch a
  # network an active container is attached to.
  reclaim_network_pool

  echo "==> bringing up $SUITE_NAME slice ($STAGING_PROJECT_NAME)" >&2
  if ! compose_up_with_retry "$@"; then
    echo "==> compose up failed — dumping logs before trap cleanup" >&2
    docker compose -f "$SLICE" -p "$STAGING_PROJECT_NAME" ps -a >&2 2>&1 || true
    docker compose -f "$SLICE" -p "$STAGING_PROJECT_NAME" logs --no-color --tail=500 2>&1 \
      | redact_secrets >&2 || true
    exit 1
  fi
}

# suite_test [extra playwright args...]
#
# Installs dependencies on the default bridge, then runs the suite inside the
# staging network so the service's DNS name resolves. The staging network is
# `internal: true` — no egress — which is why those are two containers.
suite_test() {
  install_suite_deps

  local -a args=("$@")

  # --shard is passed through only when set: Playwright rejects an empty value,
  # and `1/1` is not the same as omitting it as far as report merging goes.
  if [[ -n "${SHARD:-}" ]]; then
    if ! [[ "$SHARD" =~ ^[0-9]+/[0-9]+$ ]]; then
      echo "SHARD must match N/M (got: $SHARD)" >&2
      exit 2
    fi
    args+=("--shard=$SHARD")
  fi
  if [[ -n "${PW_GREP:-}" ]]; then
    args+=("--grep" "$PW_GREP")
  fi

  echo "==> running $SUITE_NAME Playwright suite${SHARD:+ (shard $SHARD)}" >&2
  docker run --rm \
    --network "$STAGING_PROJECT_NAME" \
    -v "$ROOT:$ROOT" \
    -w "$SUITE_DIR" \
    -e CI="${CI:-}" \
    -e FORCE_COLOR=1 \
    -e RUN_ID="$RUN_ID" \
    -e PW_REPORT_DIR="$REPORT_DIR" \
    -e PW_BLOB="${PW_BLOB:-}" \
    -e PW_WORKERS="${PW_WORKERS:-}" \
    -e PW_STRICT_FLAKE="${PW_STRICT_FLAKE:-}" \
    "${SUITE_ENV_ARGS[@]}" \
    "$NODE_IMAGE" \
    npx playwright test "${args[@]}"

  echo "==> suite passed. reports: $REPORT_DIR" >&2
}

suite_cleanup() {
  local exit_code=$?

  # Nothing to tear down if the run failed before the slice was rendered.
  if [[ -n "${SLICE:-}" ]]; then
    # Dump per-container logs to stderr BEFORE teardown when the suite failed.
    # A wrapper's own dump task in `always:` races with this trap — by the time
    # it runs the stack is gone and `docker compose logs` finds nothing.
    if [[ "$exit_code" -ne 0 && "${KEEP_STACK:-0}" != "1" ]]; then
      echo "==> dumping $STAGING_PROJECT_NAME container logs (exit=$exit_code)" >&2
      docker compose -f "$SLICE" -p "$STAGING_PROJECT_NAME" \
        logs --no-color --tail=500 2>&1 | redact_secrets >&2 || true
    fi

    if [[ "${KEEP_STACK:-0}" != "1" ]]; then
      echo "==> tearing down $STAGING_PROJECT_NAME stack" >&2
      docker compose -f "$SLICE" -p "$STAGING_PROJECT_NAME" \
        down -v --remove-orphans >/dev/null 2>&1 || true
    fi
  fi

  if [[ -n "$SUITE_PKI_DIR" ]]; then
    if [[ "${KEEP_STACK:-0}" != "1" ]]; then
      rm -rf "$SUITE_PKI_DIR"
    else
      echo "==> KEEP_STACK=1 — leaving $SUITE_PKI_DIR in place (mounted by the containers)" >&2
    fi
  fi

  if [[ "${KEEP_STACK:-0}" == "1" ]]; then
    echo "==> KEEP_STACK=1 — leaving $STAGING_PROJECT_NAME stack up" >&2
  fi

  # $SLICE_DIR is under mktemp -d; always clean up, even under KEEP_STACK=1, so
  # resolved compose config (which bakes in environment values) does not linger.
  rm -rf "${SLICE_DIR:-}"
}
