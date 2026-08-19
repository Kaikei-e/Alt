#!/usr/bin/env bash
# Retired. Wave 4 Pattern B cutover moved inbound TLS into the parent
# process. There are zero `network_mode: service:` pki-agent sidecars,
# so cascading recreate of a shared netns is not a valid operation.
#
# If you are calling this script, the caller is stale. Do not add a
# NETNS_SIDECARS array back — the compose-netns-cascade-audit fails
# when that array reappears, and fails when a pki-agent joins a
# parent's netns.
set -euo pipefail
echo "cascade-pki-sidecars.sh is retired: no netns-sharing pki sidecars remain (Wave 4 Pattern B)." >&2
echo "Inbound TLS lives in the parent process; parent-only recreate no longer orphans :9443." >&2
exit 1
