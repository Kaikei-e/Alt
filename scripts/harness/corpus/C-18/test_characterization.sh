#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
TARGET="$SCRIPT_DIR/check-compose-variables.sh"

TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

LOG_FILE="$TMP_DIR/docker-args.log"
cat >"$TMP_DIR/docker" <<EOF
#!/usr/bin/env bash
printf '%s\n' "\$*" >>"$LOG_FILE"
if [[ -n "\${STUB_STDERR:-}" ]]; then
  printf '%s\n' "\$STUB_STDERR" >&2
fi
exit 0
EOF
chmod +x "$TMP_DIR/docker"

# (a) deploy.sh invokes check-compose-variables.sh without --env-file; must exit 0.
DOCKER_BIN="$TMP_DIR/docker" COMPOSE_FILE="$TMP_DIR/compose.yaml" "$TARGET"

# (b) docker compose config must run without --no-env-resolution so env_file: is validated.
grep -q 'compose .*config' "$LOG_FILE"
if grep -q -- '--no-env-resolution' "$LOG_FILE"; then
  echo "FAIL: --no-env-resolution must not be passed to docker compose config" >&2
  exit 1
fi

# (c) Unset interpolation variable warning on stderr must fail with exit 1.
rc=0
DOCKER_BIN="$TMP_DIR/docker" COMPOSE_FILE="$TMP_DIR/compose.yaml" \
  STUB_STDERR='WARN[0000] The "FOO" variable is not set. Defaulting to a blank string.' \
  "$TARGET" 2>/dev/null || rc=$?
if [[ "$rc" -ne 1 ]]; then
  echo "FAIL: expected exit 1 on unset compose variable warning, got $rc" >&2
  exit 1
fi
