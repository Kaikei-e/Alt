#!/usr/bin/env bash
# Assert that a port refuses the connection outright, rather than answering
# with a status code.
#
# Usage
# -----
# Source this file from a run.sh that already defines a `hurl_run` wrapper
# (docker run --network <staging net> ... "$HURL_IMAGE" "$@"), then call:
#
#   assert_transport_refused "no plaintext listener on :9101" \
#     --file-root "$ROOT" \
#     --variable "probe_url=http://alt-harvester:9101/health" \
#     e2e/hurl/_lib/probe-transport-refused.hurl
#
# Why this is not a Hurl assertion
# --------------------------------
# "Nothing is listening" and "the TLS handshake was rejected" are the two
# facts a split-topology suite most needs to state, and Hurl can express
# neither: an entry that fails to reach the server is a run failure, full
# stop. The polarity therefore gets inverted out here — the probe file
# passes on *any* response, so the only route to a non-zero exit is the
# transport error we want.
#
# Exit code 3 specifically, not "non-zero"
# ----------------------------------------
# Hurl exits 2 on a parse error and 4 on an assertion failure. Accepting any
# non-zero code would turn a probe file that no longer parses into a silent
# pass — the same silent-fallback failure mode Rule 8 exists to forbid, just
# expressed in test infrastructure instead of DI.

set -euo pipefail

assert_transport_refused() {
  local what="$1"; shift
  local code=0
  hurl_run --test "$@" >/dev/null 2>&1 || code=$?
  if [[ "$code" -eq 3 ]]; then
    echo "  ok: $what — refused at the transport layer" >&2
    return 0
  fi
  if [[ "$code" -eq 0 ]]; then
    echo "FAIL: $what — the server answered; this boundary is not closed" >&2
  else
    echo "FAIL: $what — expected Hurl exit 3 (transport error), got $code" >&2
  fi
  return 1
}
