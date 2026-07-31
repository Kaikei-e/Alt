# Transport-refusal probe, shared by the suites that have to prove a port is
# closed rather than merely guarded (alt-data-hub, alt-harvester).
#
# It lives under _lib/ and not in a suite directory so no run.sh glob
# (`0[1-9]-*.hurl`) can pick it up: it is invoked directly, by
# assert_transport_refused() in _lib/assert-transport-refused.sh.
#
# Hurl has no vocabulary for "the connection must not be established": an
# entry that cannot reach the server is a run failure, not an assertion that
# can succeed. So the polarity is inverted in the helper instead. This file
# asserts `HTTP *` — literally any response passes — which leaves exactly one
# way for the run to end non-zero: a connect or TLS-handshake error. The
# helper then requires Hurl exit code 3 (runtime error), not merely non-zero,
# so a parse error (2) or an assert error (4) can never be mistaken for the
# refusal we are trying to prove.
#
# Callers vary {{probe_url}} and the --cacert / --cert / --key flags to cover:
#   - https://alt-data-hub:9443 with no client certificate  → handshake refused
#   - https://alt-data-hub:9443 with an unlisted peer name  → handshake refused
#   - http://alt-data-hub:{9000,9101,9102}                  → nothing bound
#   - http://alt-harvester:{9101,9102,9443}                 → nothing bound
#
# One entry per file on purpose: with several entries Hurl stops at the first
# failure, so a file where entry 1 connects and entry 2 does not would still
# exit 3 and read as a pass.
GET {{probe_url}}
HTTP *
