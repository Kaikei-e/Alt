#!/usr/bin/env bash
# Runtime probe for the alt-docker-postgres AppArmor profile.
#
# Runs an isolated, networkless ephemeral container as the `postgres` user under
# `--security-opt apparmor=<profile>`, verifies that the active kernel label is
# in `(enforce)` mode for the expected profile (unstacked or `//&unconfined`
# stacked), and proves real inter-process signal delivery (`SIGUSR1`) from parent
# to a distinct child process.
set -euo pipefail

DOCKER_BIN="${DOCKER_BIN:-docker}"
TIMEOUT_BIN="${TIMEOUT_BIN:-timeout}"
APPARMOR_PROFILE_NAME="${APPARMOR_PROFILE_NAME:-alt-docker-postgres}"
PROBE_IMAGE="${PROBE_IMAGE:-postgres:17.11-alpine}"
PROBE_TIMEOUT_SECONDS="${PROBE_TIMEOUT_SECONDS:-15}"
CONTAINER_NAME="alt-apparmor-probe-$$-${RANDOM:-0}"

cleanup_container() {
  local exit_code=$?
  if [[ -n "${CONTAINER_NAME:-}" ]]; then
    if "$DOCKER_BIN" rm -f "$CONTAINER_NAME" >/dev/null 2>&1; then
      :
    fi
  fi
  exit "$exit_code"
}
trap cleanup_container EXIT

INNER_SCRIPT=$(cat <<'EOS'
set -eu

profile_name="$1"
attr_path="${APPARMOR_ATTR_PATH:-/proc/self/attr/apparmor/current}"
if [ ! -r "$attr_path" ]; then
  attr_path="/proc/self/attr/current"
fi
if [ ! -r "$attr_path" ]; then
  echo "FAIL: cannot read AppArmor attr path ($attr_path)" >&2
  exit 1
fi

label="$(tr -d '\n\r' < "$attr_path")"
case "$label" in
  "${profile_name} (enforce)"|"${profile_name}//&unconfined (enforce)"|"unconfined//&${profile_name} (enforce)")
    ;;
  *)
    echo "FAIL: unexpected runtime AppArmor label: '${label}' (expected '${profile_name}' or '${profile_name}//&unconfined' in enforce mode)" >&2
    exit 1
    ;;
esac

work_dir="$(mktemp -d)"
ready_file="${work_dir}/ready"
sig_file="${work_dir}/received"

(
  trap 'echo RECEIVED_USR1 > "'"$sig_file"'"; exit 0' USR1
  echo READY > "$ready_file"
  i=0
  while [ "$i" -lt 50 ]; do
    sleep 0.1
    i=$((i + 1))
  done
  exit 1
) &
child_pid=$!

j=0
while [ ! -f "$ready_file" ]; do
  if [ "$j" -ge 30 ]; then
    echo "FAIL: child process did not become ready within timeout" >&2
    rm -rf "$work_dir"
    exit 1
  fi
  sleep 0.05
  j=$((j + 1))
done

if ! kill -USR1 "$child_pid"; then
  echo "FAIL: kill -USR1 to child process ($child_pid) was denied" >&2
  rm -rf "$work_dir"
  exit 1
fi

if ! wait "$child_pid"; then
  echo "FAIL: child process exited non-zero without handling USR1" >&2
  rm -rf "$work_dir"
  exit 1
fi

if [ ! -f "$sig_file" ] || [ "$(cat "$sig_file")" != "RECEIVED_USR1" ]; then
  echo "FAIL: child process did not record USR1 receipt" >&2
  rm -rf "$work_dir"
  exit 1
fi

rm -rf "$work_dir"
echo "PASS: runtime AppArmor label '${label}' verified and inter-process SIGUSR1 delivered."
EOS
)

"$TIMEOUT_BIN" "$PROBE_TIMEOUT_SECONDS" "$DOCKER_BIN" run --rm \
  --name "$CONTAINER_NAME" \
  --pull=never \
  --network=none \
  --user postgres \
  --security-opt "apparmor=${APPARMOR_PROFILE_NAME}" \
  --entrypoint sh \
  "$PROBE_IMAGE" -c "$INNER_SCRIPT" -- "$APPARMOR_PROFILE_NAME"
