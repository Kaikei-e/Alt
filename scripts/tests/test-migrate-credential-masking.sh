#!/usr/bin/env bash
# The Atlas migrate.sh runners must never print the database password, even
# when atlas echoes the connection URL in its errors or the shell is traced.

set -euo pipefail

ROOT="$(cd "$(dirname "$0")"/../.. && pwd)"
SECRET='s3cr3t-PW'
SCRIPTS=(
  migrations-atlas
  pre-processor-migration-atlas
  acolyte-migration-atlas
  knowledge-sovereign
  rag-migration-atlas
  recap-migration-atlas
)

if ! command -v busybox >/dev/null; then
  echo "SKIP: busybox not installed"
  exit 0
fi

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
mkdir -p "$work/bin" "$work/migrations"
: >"$work/migrations/atlas.sum"

cat >"$work/bin/atlas" <<'ATLAS'
#!/bin/sh
url=""
while [ $# -gt 0 ]; do
  [ "$1" = "--url" ] && url="$2"
  shift
done
[ -z "$url" ] && exit 0
echo "Error: failed to connect using $url" >&2
exit 1
ATLAS
printf '#!/bin/sh\nexit 0\n' >"$work/bin/nc"
chmod +x "$work/bin/atlas" "$work/bin/nc"

fail=0
run_case() {
  local name="$1" cmd="$2"
  shift 2
  local out
  out="$(env PATH="$work/bin:$PATH" MIGRATION_DIR="$work/migrations" "$@" \
    busybox sh -x "$ROOT/$name/docker/scripts/migrate.sh" "$cmd" 2>&1 || true)"
  if grep -qF "$SECRET" <<<"$out"; then
    echo "FAIL: $name $cmd leaked the database password"
    fail=1
  else
    echo "ok: $name $cmd"
  fi
}

for name in "${SCRIPTS[@]}"; do
  for cmd in status apply; do
    run_case "$name" "$cmd" \
      DATABASE_URL="postgres://alt:${SECRET}@db.example:5432/alt?sslmode=disable"
  done
done

run_case migrations-atlas status DB_HOST=db.example DB_USER=alt DB_NAME=alt DB_PORT=5432 DB_PASSWORD="$SECRET"

exit "$fail"
