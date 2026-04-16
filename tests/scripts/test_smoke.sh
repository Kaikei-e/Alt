#!/usr/bin/env bash
# Tests for scripts/smoke.sh — post-rollout health probe for the 4 edge endpoints.
set -uo pipefail

HERE="$(cd "$(dirname "$0")" && pwd)"
# shellcheck disable=SC1091
source "$HERE/lib.sh"

SUT="$REPO_ROOT/scripts/smoke.sh"

export_sut_env() {
  export CURL_BIN="$STUB_BIN/curl"
  export SMOKE_WAIT_SECONDS="1"
}

tc_passes_when_all_endpoints_ok() {
  export_sut_env
  make_stub curl 0 "ok"

  run_output=$("$SUT" 2>&1); rc=$?
  assert_eq "$rc" "0" "smoke must exit 0 when all endpoints respond" || { echo "$run_output"; return 1; }
  count=$(stub_call_count curl)
  assert_eq "$count" "4" "smoke must curl 4 endpoints (nginx, backend, bff, meilisearch)" || return 1
}

tc_fails_when_any_endpoint_fails() {
  export_sut_env
  # Fail only for the backend health URL.
  make_conditional_stub curl '
    for arg in "$@"; do
      if [[ "$arg" == *"localhost:9000"* ]]; then
        exit 22
      fi
    done
    exit 0'

  run_output=$("$SUT" 2>&1); rc=$?
  assert_ne "$rc" "0" "smoke must exit non-zero when backend health fails" || return 1
  assert_contains "$run_output" "9000" "output must mention the failing endpoint" || return 1
}

tc_curls_all_four_expected_urls() {
  export_sut_env
  make_stub curl 0 "ok"

  "$SUT" >/dev/null 2>&1 || true
  for needle in "localhost/health" "localhost:9000/v1/health" "localhost:9250/health" "localhost:7700/health"; do
    if ! grep -qF "$needle" "$STUB_LOG"; then
      echo "  FAIL: smoke.sh did not curl $needle"
      cat "$STUB_LOG"
      return 1
    fi
  done
}

main() {
  echo "smoke.sh tests"
  run_case "exits 0 when all 4 endpoints succeed" tc_passes_when_all_endpoints_ok
  run_case "exits non-zero when any endpoint fails" tc_fails_when_any_endpoint_fails
  run_case "curls nginx / backend / bff / meilisearch" tc_curls_all_four_expected_urls
  summary
}

main "$@"
