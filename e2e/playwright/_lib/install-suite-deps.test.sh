#!/usr/bin/env bash
# e2e/playwright/_lib/install-suite-deps.test.sh
#
# `npm ci` reaches the public registry, so it fails the way networks fail.
# On 2026-08-10 a read timeout (`npm error code ETIMEDOUT`, errno -110) took
# out the alt-harvester and news-creator legs; the next day it took out
# alt-backend and alt-harvester. Neither had anything to do with the code
# under test. Because the deploy gate reads the e2e job as a whole, each of
# those timeouts withheld the entire production rollout — a Web Push fix sat
# built-but-unshipped for 36 hours behind one of them.
#
# So the install has to survive a transient registry failure, and it has to
# stop rather than loop forever when the registry is genuinely down.
#
# `docker` and `sleep` are stubbed: this must not pull an image, and it must
# not spend the backoff for real.
#
# Run:
#   bash e2e/playwright/_lib/install-suite-deps.test.sh
# Exit 0 on green, non-zero on red.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

PASS=0
FAIL=0

assert() {
  local name="$1"; shift
  if "$@"; then
    echo "  PASS  $name"
    PASS=$((PASS + 1))
  else
    echo "  FAIL  $name"
    FAIL=$((FAIL + 1))
  fi
}

DOCKER_LOG=$(mktemp)
trap 'rm -f "$DOCKER_LOG"' EXIT

# The helper reads these; no real path or image is touched because docker is
# stubbed out below.
ROOT="/nonexistent/alt"
NODE_IMAGE="node:test-stub"
export ROOT NODE_IMAGE

# Fail the first $DOCKER_FAILURES invocations, then succeed. A file rather
# than a variable so the count survives however the helper invokes docker.
docker() {
  echo "$*" >> "$DOCKER_LOG"
  local calls
  calls=$(wc -l < "$DOCKER_LOG")
  if (( calls <= DOCKER_FAILURES )); then
    return 1
  fi
  return 0
}
export -f docker

# The backoff is real time in CI and dead time here.
sleep() { :; }
export -f sleep

# shellcheck source-path=SCRIPTDIR
# shellcheck source=install-suite-deps.sh
source "$SCRIPT_DIR/install-suite-deps.sh"

echo "[behavioural] a healthy registry installs once"
: > "$DOCKER_LOG"
DOCKER_FAILURES=0
install_suite_deps
assert "returns success" test $? -eq 0
assert "does not retry when the first attempt works" \
  test "$(wc -l < "$DOCKER_LOG")" -eq 1

echo
echo "[behavioural] a transient registry timeout is retried, not surfaced"
: > "$DOCKER_LOG"
DOCKER_FAILURES=1
if install_suite_deps; then
  echo "  PASS  returns success after a retry"
  PASS=$((PASS + 1))
else
  echo "  FAIL  returns success after a retry"
  FAIL=$((FAIL + 1))
fi
assert "invoked npm ci more than once" \
  test "$(wc -l < "$DOCKER_LOG")" -ge 2

echo
echo "[behavioural] a registry that stays down fails, bounded"
: > "$DOCKER_LOG"
DOCKER_FAILURES=999
if install_suite_deps; then
  echo "  FAIL  must surface the failure when every attempt fails"
  FAIL=$((FAIL + 1))
else
  echo "  PASS  surfaces the failure when every attempt fails"
  PASS=$((PASS + 1))
fi
ATTEMPTS=$(wc -l < "$DOCKER_LOG")
assert "retries a bounded number of times (got $ATTEMPTS)" \
  bash -c "(( $ATTEMPTS > 1 && $ATTEMPTS <= 10 ))"

echo
echo "[structural] the install still runs on the default bridge"
: > "$DOCKER_LOG"
DOCKER_FAILURES=0
install_suite_deps
INVOCATION=$(cat "$DOCKER_LOG")
# The staging network is `internal: true`. Joining it here would make every
# attempt fail identically, and a retry loop would only make that slower.
assert "does not attach the install container to a compose network" \
  bash -c "[[ '$INVOCATION' != *'--network'* ]]"
assert "still runs npm ci" \
  bash -c "[[ '$INVOCATION' == *'npm ci'* ]]"

echo
echo "Total: PASS=$PASS FAIL=$FAIL"
[[ $FAIL -eq 0 ]]
