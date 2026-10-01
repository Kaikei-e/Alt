#!/usr/bin/env bash
# e2e/playwright/tts-speaker/run.sh
#
# Brings up tts-speaker against the deterministic fake upstream (irodori_stub.py),
# runs the Playwright contract suite, and tears down.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../../.." && pwd)"
# shellcheck source=../_lib/suite.sh
source "$ROOT/e2e/playwright/_lib/suite.sh"

suite_init tts-speaker

suite_endpoint BASE_URL "http://tts-speaker:9700"

suite_up --build \
  irodori-stub \
  tts-speaker

suite_test
