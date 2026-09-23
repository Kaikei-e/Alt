#!/usr/bin/env bash
# Heuristic Clean Architecture layer boundary and dependency checker for Alt services
set -euo pipefail

if [ "$#" -ne 1 ]; then
  echo "Usage: $0 <service-dir>" >&2
  exit 2
fi

SERVICE_DIR="$1"
[ -d "$SERVICE_DIR" ] || { echo "Error: '$SERVICE_DIR' not found" >&2; exit 2; }

VIOLATIONS=0
WARNINGS=0

run_check() {
  local kind="$1" rule="$2" lang="$3" inc="$4" exc="$5" regex="$6"
  local matches status=0
  matches=$(rg --type "$lang" -g "$inc" -g "$exc" -n -H "$regex" "$SERVICE_DIR") || status=$?
  if [ "$status" -eq 2 ]; then
    echo "rg error: $matches" >&2
    exit 2
  fi
  if [ "$status" -eq 0 ] && [ -n "$matches" ]; then
    while IFS= read -r line; do
      [ -z "$line" ] && continue
      [ "$kind" = "VIOLATION" ] && { echo "VIOLATION $rule $line"; VIOLATIONS=$((VIOLATIONS + 1)); } \
                                || { echo "WARN $rule $line"; WARNINGS=$((WARNINGS + 1)); }
    done <<< "$matches"
    return 1
  fi
  return 0
}

has_files() {
  rg --files --type "$1" -g "$2" -g "$3" "$SERVICE_DIR" 2>/dev/null | head -n 1 | grep -q .
}

# --- Go checks ---
if rg --files --type go "$SERVICE_DIR" 2>/dev/null | head -n 1 | grep -q .; then
  # REST / Handler
  hv=0
  run_check "VIOLATION" "rest-imports-driver-gateway-or-pgx" "go" '**/handler/**' '!*_test.go' '"([^"]+/(driver|gateway)(/[^"]*)?|github\.com/jackc/pgx[^"]*)"' || hv=1
  run_check "VIOLATION" "rest-imports-driver-gateway-or-pgx" "go" '**/rest/**' '!*_test.go' '"([^"]+/(driver|gateway)(/[^"]*)?|github\.com/jackc/pgx[^"]*)"' || hv=1
  [ "$hv" -eq 0 ] && (has_files "go" '**/handler/**' '!*_test.go' || has_files "go" '**/rest/**' '!*_test.go') && echo "[clean] Go rest/handler: clean"

  # Usecase
  uv=0
  run_check "VIOLATION" "usecase-imports-forbidden-layers-or-sql" "go" '**/usecase/**' '!*_test.go' '"([^"]+/(driver|gateway|rest|handler)(/[^"]*)?|database/sql|github\.com/jackc/pgx[^"]*)"' || uv=1
  run_check "WARN" "usecase-imports-otel" "go" '**/usecase/**' '!*_test.go' '"go\.opentelemetry\.io[^"]*"' || true
  [ "$uv" -eq 0 ] && has_files "go" '**/usecase/**' '!*_test.go' && echo "[clean] Go usecase: clean"

  # Port
  pv=0
  run_check "VIOLATION" "port-imports-forbidden-layers-or-sql" "go" '**/port/**' '!*_test.go' '"([^"]+/(driver|gateway|rest|handler|usecase)(/[^"]*)?|database/sql|github\.com/jackc/pgx[^"]*)"' || pv=1
  run_check "VIOLATION" "port-imports-net-http" "go" '**/port/**' '!*_test.go' '"net/http"' || pv=1
  run_check "VIOLATION" "port-imports-otel" "go" '**/port/**' '!*_test.go' '"go\.opentelemetry\.io[^"]*"' || pv=1
  [ "$pv" -eq 0 ] && has_files "go" '**/port/**' '!*_test.go' && echo "[clean] Go port: clean"

  # Gateway
  gv=0
  run_check "VIOLATION" "gateway-imports-usecase-or-handler" "go" '**/gateway/**' '!*_test.go' '"([^"]+/(usecase|rest|handler)(/[^"]*)?)"' || gv=1
  [ "$gv" -eq 0 ] && has_files "go" '**/gateway/**' '!*_test.go' && echo "[clean] Go gateway: clean"

  # Driver
  dv=0
  run_check "VIOLATION" "driver-imports-upward" "go" '**/driver/**' '!*_test.go' '"([^"]+/(usecase|gateway|rest|handler)(/[^"]*)?)"' || dv=1
  run_check "VIOLATION" "driver-implements-port-skips-gateway" "go" '**/driver/**' '!*_test.go' '"([^"]+/port(/[^"]*)?)"' || dv=1
  run_check "VIOLATION" "driver-imports-domain" "go" '**/driver/**' '!*_test.go' '"([^"]+/domain(/[^"]*)?)"' || dv=1
  [ "$dv" -eq 0 ] && has_files "go" '**/driver/**' '!*_test.go' && echo "[clean] Go driver: clean"

  # Domain
  dmv=0
  run_check "VIOLATION" "domain-imports-sibling-layers" "go" '**/domain/**' '!*_test.go' '"([^"]+/(usecase|port|gateway|driver|rest|handler)(/[^"]*)?)"' || dmv=1
  run_check "VIOLATION" "domain-imports-sql" "go" '**/domain/**' '!*_test.go' '"(database/sql|github\.com/jackc/pgx[^"]*)"' || dmv=1
  run_check "VIOLATION" "domain-imports-net-http" "go" '**/domain/**' '!*_test.go' '"net/http"' || dmv=1
  run_check "VIOLATION" "domain-imports-otel" "go" '**/domain/**' '!*_test.go' '"go\.opentelemetry\.io[^"]*"' || dmv=1
  [ "$dmv" -eq 0 ] && has_files "go" '**/domain/**' '!*_test.go' && echo "[clean] Go domain: clean"
fi

# --- Python checks ---
if rg --files --type py "$SERVICE_DIR" 2>/dev/null | head -n 1 | grep -q .; then
  py_exc='!*test*'

  # Handler / Router
  py_hv=0
  run_check "VIOLATION" "py-handler-imports-driver-or-gateway" "py" '**/handler/**' "$py_exc" '^(from|import)\s+.*(\.driver|\.gateway)' || py_hv=1
  run_check "VIOLATION" "py-handler-imports-driver-or-gateway" "py" '**/routers/**' "$py_exc" '^(from|import)\s+.*(\.driver|\.gateway)' || py_hv=1
  [ "$py_hv" -eq 0 ] && (has_files "py" '**/handler/**' "$py_exc" || has_files "py" '**/routers/**' "$py_exc") && echo "[clean] Python handler/routers: clean"

  # Usecase
  py_uv=0
  run_check "VIOLATION" "py-usecase-imports-infra-or-outer" "py" '**/usecase/**' "$py_exc" '^(from|import)\s+.*(httpx|asyncpg|redis|sqlalchemy|fastapi|starlette|psycopg|\.driver|\.gateway|\.handler)' || py_uv=1
  [ "$py_uv" -eq 0 ] && has_files "py" '**/usecase/**' "$py_exc" && echo "[clean] Python usecase: clean"

  # Port
  py_pv=0
  run_check "VIOLATION" "py-port-imports-infra" "py" '**/port/**' "$py_exc" '^(from|import)\s+.*(httpx|asyncpg|redis|sqlalchemy|fastapi|starlette|psycopg|\.driver|\.gateway|\.handler)' || py_pv=1
  [ "$py_pv" -eq 0 ] && has_files "py" '**/port/**' "$py_exc" && echo "[clean] Python port: clean"

  # Driver
  py_dv=0
  run_check "VIOLATION" "py-driver-imports-upward" "py" '**/driver/**' "$py_exc" '^(from|import)\s+.*(\.usecase|\.handler|\.gateway)' || py_dv=1
  run_check "VIOLATION" "py-driver-imports-port-skips-gateway" "py" '**/driver/**' "$py_exc" '^(from|import)\s+.*\.port' || py_dv=1
  run_check "VIOLATION" "py-driver-imports-domain" "py" '**/driver/**' "$py_exc" '^(from|import)\s+.*\.domain' || py_dv=1
  [ "$py_dv" -eq 0 ] && has_files "py" '**/driver/**' "$py_exc" && echo "[clean] Python driver: clean"

  # Domain
  py_dmv=0
  run_check "VIOLATION" "py-domain-imports-infra-or-layers" "py" '**/domain/**' "$py_exc" '^(from|import)\s+.*(httpx|asyncpg|redis|sqlalchemy|fastapi|starlette|psycopg|\.driver|\.gateway|\.handler|\.usecase|\.port)' || py_dmv=1
  [ "$py_dmv" -eq 0 ] && has_files "py" '**/domain/**' "$py_exc" && echo "[clean] Python domain: clean"
fi

[ "$VIOLATIONS" -gt 0 ] && exit 1
exit 0
