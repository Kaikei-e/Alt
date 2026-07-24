#!/usr/bin/env bash
# Unit tests for entrypoint.sh's model-pull fail-fast behavior (CLAUDE.md Rule 9).
#
# Regression covered: `ollama pull` failures used to be logged as
# "non-fatal" and the loop kept going, so the container reached "healthy"
# without the required embedding model ever being present (warn-and-limp).
# ensure_models() must now retry a bounded number of times and return
# non-zero when a required model still cannot be pulled, so main() can
# fail fast and let the compose restart policy surface the crash loop.
#
# Usage: bash knowledge-embedder/tests/test_entrypoint.sh
set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ENTRYPOINT="$SCRIPT_DIR/../entrypoint.sh"

fail=0

# shellcheck disable=SC1090
source "$ENTRYPOINT"

run_case() {
  local name="$1"
  shift
  if "$@"; then
    echo "PASS: $name"
  else
    echo "FAIL: $name"
    fail=1
  fi
}

# Creates a mock `ollama` binary on a fresh PATH dir.
#   mode=always_fail -> every `ollama pull` call fails
#   mode=recovers     -> `ollama pull` fails once, then succeeds
setup_mock_bin() {
  local mode="$1"
  local bin_dir
  bin_dir=$(mktemp -d)
  local counter_file="$bin_dir/.attempts"
  echo 0 >"$counter_file"

  cat >"$bin_dir/ollama" <<EOF
#!/usr/bin/env bash
counter_file="$counter_file"
if [ "\$1" = "list" ]; then
  exit 0
fi
if [ "\$1" = "pull" ]; then
  n=\$(cat "\$counter_file")
  n=\$((n + 1))
  echo "\$n" > "\$counter_file"
  if [ "$mode" = "recovers" ] && [ "\$n" -ge 2 ]; then
    exit 0
  fi
  exit 1
fi
exit 0
EOF
  chmod +x "$bin_dir/ollama"
  echo "$bin_dir"
}

test_permanent_failure_returns_nonzero() {
  local bin_dir out rc
  bin_dir=$(setup_mock_bin "always_fail")
  out=$(mktemp)
  (
    export PATH="$bin_dir:$PATH"
    export EMBEDDING_MODELS="fake-model"
    export MODEL_PULL_MAX_ATTEMPTS=3
    export MODEL_PULL_RETRY_DELAY=0
    ensure_models
  ) >"$out" 2>&1
  rc=$?
  rm -rf "$bin_dir" "$out"
  [ "$rc" -ne 0 ]
}

test_transient_failure_recovers_within_budget() {
  local bin_dir out rc
  bin_dir=$(setup_mock_bin "recovers")
  out=$(mktemp)
  (
    export PATH="$bin_dir:$PATH"
    export EMBEDDING_MODELS="fake-model"
    export MODEL_PULL_MAX_ATTEMPTS=3
    export MODEL_PULL_RETRY_DELAY=0
    ensure_models
  ) >"$out" 2>&1
  rc=$?
  rm -rf "$bin_dir" "$out"
  [ "$rc" -eq 0 ]
}

run_case "permanent pull failure -> ensure_models returns non-zero (fail-fast)" \
  test_permanent_failure_returns_nonzero
run_case "transient pull failure recovers within retry budget -> ensure_models returns 0" \
  test_transient_failure_recovers_within_budget

exit "$fail"
