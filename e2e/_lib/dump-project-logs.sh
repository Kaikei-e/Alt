#!/usr/bin/env bash
# Dump every container of a compose project, redacted.
#
#   bash e2e/_lib/dump-project-logs.sh <project-name> [tail-lines]
#
# Why not `docker compose logs`
# -----------------------------
# Two reasons, both learned the hard way in this repo's CI.
#
# 1. A profile-gated service is invisible to `docker compose logs` unless the
#    same `--profile` flag is repeated, so a bare `logs` in a failure handler
#    silently prints nothing — which is exactly when the logs are needed
#    (observed on run 24752223343, where the omission hid a SIGSEGV).
# 2. The suites run against a slice rendered under `mktemp -d` that the trap
#    has already deleted by the time a workflow's `if: failure()` step runs, so
#    there is no compose file left to pass to `-f`.
#
# Going through the project label instead needs neither the compose file nor
# the profile list: it finds whatever is actually there, including containers
# that have already exited — which are usually the interesting ones.
#
# Redaction is not optional. `docker logs` copies container stdout verbatim
# into the job output, and any error path that logs a request header can carry
# a staging token; a CI job log is a far more durable place for a secret to
# land than the container it came from.
set -euo pipefail

PROJECT="${1:?usage: dump-project-logs.sh <project-name> [tail-lines]}"
TAIL="${2:-500}"

redact() {
  sed -E \
    -e 's#eyJ[A-Za-z0-9_=-]+\.eyJ[A-Za-z0-9_=-]+\.[A-Za-z0-9_=-]+#[REDACTED_JWT]#g' \
    -e 's#(-----BEGIN [A-Z ]*PRIVATE KEY-----).*#\1[REDACTED_KEY]#g'
}

echo "===== docker ps -a (project=$PROJECT) ====="
docker ps -a --filter "label=com.docker.compose.project=$PROJECT" \
  --format 'table {{.Names}}\t{{.Status}}\t{{.Image}}' || true

mapfile -t containers < <(
  docker ps -aq --filter "label=com.docker.compose.project=$PROJECT" 2>/dev/null || true
)

if [[ "${#containers[@]}" -eq 0 ]]; then
  echo "no containers found for project '$PROJECT' — the stack never came up, or it was" >&2
  echo "already torn down (check that the job sets KEEP_STACK=1)." >&2
  exit 0
fi

for id in "${containers[@]}"; do
  name="$(docker inspect -f '{{.Name}}' "$id" 2>/dev/null | sed 's#^/##')"
  echo
  echo "===== $name (tail=$TAIL) ====="
  docker logs --tail="$TAIL" "$id" 2>&1 | redact || true
done
