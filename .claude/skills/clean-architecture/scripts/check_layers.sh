#!/usr/bin/env bash
# Heuristic Clean Architecture layer boundary and dependency checker for Alt services
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

usage() {
  echo "Usage: $0 <service-dir> | --file <path> | --changed | --hook" >&2
  exit 2
}

MODE=""
SERVICE_DIR=""
TARGET_FILE=""

if [ "$#" -eq 0 ]; then
  usage
fi

case "$1" in
  --file)
    [ "$#" -eq 2 ] || usage
    MODE="file"
    TARGET_FILE="$2"
    ;;
  --changed)
    [ "$#" -eq 1 ] || usage
    MODE="changed"
    ;;
  --hook)
    [ "$#" -eq 1 ] || usage
    MODE="hook"
    ;;
  -*)
    usage
    ;;
  *)
    [ "$#" -eq 1 ] || usage
    MODE="dir"
    SERVICE_DIR="$1"
    ;;
esac

VIOLATIONS=0
WARNINGS=0
COLLECTED_VIOLATIONS=()
VIOLATED_RULES=()

K_RULES=()
K_PREFIXES=()
K_SUBSTRS=()
K_REASONS=()
K_COUNTS=()

ABS_SERVICE_DIR=""
REPO_ROOT=""
FILTER_ABS_FILE=""
DISPLAY_FILE=""

load_known_ledger() {
  local known_file="${CHECK_LAYERS_KNOWN:-$SCRIPT_DIR/known_violations.tsv}"
  if [ ! -r "$known_file" ] || [ -d "$known_file" ]; then
    return 0
  fi
  local raw_line rest k_rule k_prefix k_substr k_reason
  while IFS= read -r raw_line || [ -n "$raw_line" ]; do
    raw_line="${raw_line%$'\r'}"
    if [[ -z "${raw_line// /}" || "$raw_line" == \#* ]]; then
      continue
    fi
    rest="$raw_line"
    if [[ "$rest" != *$'\t'* ]]; then
      echo "check_layers.sh: malformed ledger line: $raw_line" >&2
      exit 2
    fi
    k_rule="${rest%%$'\t'*}"
    rest="${rest#*$'\t'}"
    if [[ "$rest" != *$'\t'* ]]; then
      echo "check_layers.sh: malformed ledger line: $raw_line" >&2
      exit 2
    fi
    k_prefix="${rest%%$'\t'*}"
    rest="${rest#*$'\t'}"
    if [[ "$rest" != *$'\t'* ]]; then
      echo "check_layers.sh: malformed ledger line: $raw_line" >&2
      exit 2
    fi
    k_substr="${rest%%$'\t'*}"
    k_reason="${rest#*$'\t'}"
    if [ -z "$k_rule" ] || [ -z "$k_prefix" ] || [ -z "$k_substr" ] || [ -z "$k_reason" ]; then
      echo "check_layers.sh: malformed ledger line: $raw_line" >&2
      exit 2
    fi
    K_RULES+=("$k_rule")
    K_PREFIXES+=("$k_prefix")
    K_SUBSTRS+=("$k_substr")
    K_REASONS+=("$k_reason")
    K_COUNTS+=(0)
  done < "$known_file"
  return 0
}

resolve_repo_root() {
  local ref_dir="$1" git_root
  if [ -n "${CHECK_LAYERS_ROOT:-}" ]; then
    (cd "$CHECK_LAYERS_ROOT" && pwd -P)
  elif git_root=$(git -C "$ref_dir" rev-parse --show-toplevel 2>/dev/null); then
    (cd "$git_root" && pwd -P)
  else
    pwd -P
  fi
  return 0
}

record_violated_rule() {
  local rule="$1" existing
  if [ "${#VIOLATED_RULES[@]}" -gt 0 ]; then
    for existing in "${VIOLATED_RULES[@]}"; do
      if [ "$existing" = "$rule" ]; then
        return 0
      fi
    done
  fi
  VIOLATED_RULES+=("$rule")
  return 0
}

is_known_match() {
  local rule="$1" rel_path="$2" text="$3"
  local i total="${#K_RULES[@]}"
  for ((i = 0; i < total; i++)); do
    if [ "$rule" = "${K_RULES[$i]}" ] && [[ "$rel_path" == "${K_PREFIXES[$i]}"* ]]; then
      if [ "${K_SUBSTRS[$i]}" = "*" ] || [[ "$text" == *"${K_SUBSTRS[$i]}"* ]]; then
        K_COUNTS[$i]=$(( ${K_COUNTS[$i]} + 1 ))
        return 0
      fi
    fi
  done
  return 1
}

remediation_for() {
  case "$1" in
    rest-imports-driver-gateway-or-pgx)
      echo "move database and external calls behind a Usecase and Port, implement that Port in Gateway over Driver, and map Domain errors to transport responses in Handler."
      ;;
    usecase-imports-forbidden-layers-or-sql)
      echo "define a narrow Port interface in port/ for the capability, depend on that Port in Usecase, and move concrete Gateway, Driver, or SQL calls into Gateway and Driver."
      ;;
    port-imports-forbidden-layers-or-sql)
      echo "keep Port method signatures purely in terms of Domain types and standard library primitives, and move concrete adapter or SQL types into Gateway and Driver."
      ;;
    port-imports-net-http)
      echo "replace net/http types in Port signatures with Domain values or plain parameters and keep HTTP transport details inside Handler or Driver."
      ;;
    port-imports-otel)
      echo "remove OpenTelemetry types from Port contracts; propagate trace context via context.Context and keep span instrumentation in adapters or Composition Root."
      ;;
    gateway-imports-usecase-or-handler)
      echo "Gateway may import only Port, Driver, and Domain; move shared interfaces to port/ and shared data types to domain/ instead of importing Usecase or Handler."
      ;;
    driver-imports-upward)
      echo "a Driver performs raw I/O only and must not import Handler, Usecase, or Gateway; move orchestration upward and pass plain parameters into the Driver."
      ;;
    driver-implements-port-skips-gateway)
      echo "do not import Port in Driver; return Driver-owned row/payload types from Driver and implement the Port interface in a Gateway that wraps it."
      ;;
    driver-imports-domain)
      echo "a Driver owns its own row/payload types; return those and move the mapping to Domain types into the Gateway that calls it."
      ;;
    domain-imports-sibling-layers)
      echo "keep Domain restricted to the standard library; move external capability interfaces to port/ and orchestration or adapter logic to Usecase or Gateway."
      ;;
    domain-imports-sql)
      echo "remove database/sql and pgx types from Domain models; define row structs in Driver and map between rows and pure Domain types in Gateway."
      ;;
    domain-imports-net-http)
      echo "remove net/http types from Domain; keep HTTP transport DTOs in Handler or raw HTTP calls in Driver and map to pure Domain values at the boundary."
      ;;
    domain-imports-otel)
      echo "keep Domain free of OpenTelemetry imports; propagate context.Context and put span instrumentation in Handler, Gateway, or Driver."
      ;;
    py-handler-imports-driver-or-gateway)
      echo "call a Usecase from the Handler/Router instead of importing Gateway or Driver directly, and wire concrete adapters in the Composition Root."
      ;;
    py-usecase-imports-infra-or-outer)
      echo "define a Port protocol/ABC in port/ for the external capability, call that Port from Usecase, and move HTTP, DB, or framework imports into Gateway, Driver, or Handler."
      ;;
    py-port-imports-infra)
      echo "keep Port definitions free of infrastructure and web-framework packages, expressing contracts with Domain and standard library types only."
      ;;
    py-driver-imports-upward)
      echo "a Driver performs raw I/O only and must not import Handler, Usecase, or Gateway; move higher-level types or orchestration upward and pass plain parameters into Driver."
      ;;
    py-driver-imports-port-skips-gateway)
      echo "do not import Port in Driver; return Driver-owned payload/row types from Driver and implement the Port in a Gateway that maps them to Domain models."
      ;;
    py-driver-imports-domain)
      echo "a Driver owns its own row/payload types; return those from Driver and move the mapping to Domain models into the Gateway that calls it."
      ;;
    py-domain-imports-infra-or-layers)
      echo "keep Domain pure (standard library only); move capability protocols to port/ and move infrastructure or framework dependencies to Handler, Gateway, or Driver."
      ;;
    *)
      echo "move the forbidden cross-layer or infrastructure dependency to the appropriate outer adapter and depend inward via Port and Domain."
      ;;
  esac
  return 0
}

emit_known_lines() {
  local i total="${#K_RULES[@]}"
  for ((i = 0; i < total; i++)); do
    if [ "${K_COUNTS[$i]}" -gt 0 ]; then
      echo "KNOWN ${K_RULES[$i]} ${K_PREFIXES[$i]} (${K_COUNTS[$i]} lines): ${K_REASONS[$i]}"
    fi
  done
  return 0
}

emit_fix_lines() {
  local rule
  if [ "${#VIOLATED_RULES[@]}" -gt 0 ]; then
    for rule in "${VIOLATED_RULES[@]}"; do
      echo "FIX $rule: $(remediation_for "$rule")"
    done
  fi
  return 0
}

run_check() {
  local kind="$1" rule="$2" lang="$3" inc="$4" exc="$5" regex="$6"
  local matches status=0
  matches=$(rg --type "$lang" -g "$inc" -g "$exc" -n -H "$regex" "$SERVICE_DIR") || status=$?
  if [ "$status" -eq 2 ]; then
    echo "rg error: $matches" >&2
    exit 2
  fi
  if [ "$status" -eq 0 ] && [ -n "$matches" ]; then
    local svc_clean="${SERVICE_DIR%/}"
    [ -z "$svc_clean" ] && svc_clean="/"
    local line sub rel_in_svc rest match_lineno match_text abs_match_path repo_rel_path
    while IFS= read -r line; do
      [ -z "$line" ] && continue
      if [[ "$line" == "$svc_clean/"* ]]; then
        sub="${line#"$svc_clean/"}"
        rel_in_svc="${sub%%:*}"
        rest="${sub#*:}"
      else
        rel_in_svc="${line%%:*}"
        rest="${line#*:}"
        rel_in_svc="${rel_in_svc#./}"
      fi
      rel_in_svc="${rel_in_svc#/}"
      match_lineno="${rest%%:*}"
      match_text="${rest#*:}"
      abs_match_path="$ABS_SERVICE_DIR/$rel_in_svc"

      if [ -n "$FILTER_ABS_FILE" ] && [ "$abs_match_path" != "$FILTER_ABS_FILE" ]; then
        continue
      fi

      if [ "$REPO_ROOT" = "/" ]; then
        repo_rel_path="${abs_match_path#/}"
      elif [[ "$abs_match_path" == "$REPO_ROOT/"* ]]; then
        repo_rel_path="${abs_match_path#"$REPO_ROOT/"}"
      else
        repo_rel_path="$rel_in_svc"
      fi

      if [ "$kind" = "VIOLATION" ]; then
        if is_known_match "$rule" "$repo_rel_path" "$match_text"; then
          continue
        fi
        VIOLATIONS=$((VIOLATIONS + 1))
        record_violated_rule "$rule"
        if [ "$MODE" = "dir" ]; then
          echo "VIOLATION $rule $line"
        else
          COLLECTED_VIOLATIONS+=("VIOLATION $rule $DISPLAY_FILE:$match_lineno:$match_text")
        fi
      else
        WARNINGS=$((WARNINGS + 1))
        if [ "$MODE" = "dir" ]; then
          echo "WARN $rule $line"
        fi
      fi
    done <<< "$matches"
    return 1
  fi
  return 0
}

has_files() {
  { rg --files --type "$1" -g "$2" -g "$3" "$SERVICE_DIR" 2>/dev/null || true; } | head -n 1 | grep -q .
}

run_layer_checks() {
  # --- Go checks ---
  if { rg --files --type go "$SERVICE_DIR" 2>/dev/null || true; } | head -n 1 | grep -q .; then
    # REST / Handler
    local hv=0
    run_check "VIOLATION" "rest-imports-driver-gateway-or-pgx" "go" '**/handler/**' '!*_test.go' '"([^"]+/(driver|gateway)(/[^"]*)?|github\.com/jackc/pgx[^"]*)"' || hv=1
    run_check "VIOLATION" "rest-imports-driver-gateway-or-pgx" "go" '**/rest/**' '!*_test.go' '"([^"]+/(driver|gateway)(/[^"]*)?|github\.com/jackc/pgx[^"]*)"' || hv=1
    [ "$MODE" = "dir" ] && [ "$hv" -eq 0 ] && (has_files "go" '**/handler/**' '!*_test.go' || has_files "go" '**/rest/**' '!*_test.go') && echo "[clean] Go rest/handler: clean"

    # Usecase
    local uv=0
    run_check "VIOLATION" "usecase-imports-forbidden-layers-or-sql" "go" '**/usecase/**' '!*_test.go' '"([^"]+/(driver|gateway|rest|handler)(/[^"]*)?|database/sql|github\.com/jackc/pgx[^"]*)"' || uv=1
    run_check "WARN" "usecase-imports-otel" "go" '**/usecase/**' '!*_test.go' '"go\.opentelemetry\.io[^"]*"' || true
    [ "$MODE" = "dir" ] && [ "$uv" -eq 0 ] && has_files "go" '**/usecase/**' '!*_test.go' && echo "[clean] Go usecase: clean"

    # Port
    local pv=0
    run_check "VIOLATION" "port-imports-forbidden-layers-or-sql" "go" '**/port/**' '!*_test.go' '"([^"]+/(driver|gateway|rest|handler|usecase)(/[^"]*)?|database/sql|github\.com/jackc/pgx[^"]*)"' || pv=1
    run_check "VIOLATION" "port-imports-net-http" "go" '**/port/**' '!*_test.go' '"net/http"' || pv=1
    run_check "VIOLATION" "port-imports-otel" "go" '**/port/**' '!*_test.go' '"go\.opentelemetry\.io[^"]*"' || pv=1
    [ "$MODE" = "dir" ] && [ "$pv" -eq 0 ] && has_files "go" '**/port/**' '!*_test.go' && echo "[clean] Go port: clean"

    # Gateway
    local gv=0
    run_check "VIOLATION" "gateway-imports-usecase-or-handler" "go" '**/gateway/**' '!*_test.go' '"([^"]+/(usecase|rest|handler)(/[^"]*)?)"' || gv=1
    [ "$MODE" = "dir" ] && [ "$gv" -eq 0 ] && has_files "go" '**/gateway/**' '!*_test.go' && echo "[clean] Go gateway: clean"

    # Driver
    local dv=0
    run_check "VIOLATION" "driver-imports-upward" "go" '**/driver/**' '!*_test.go' '"([^"]+/(usecase|gateway|rest|handler)(/[^"]*)?)"' || dv=1
    run_check "VIOLATION" "driver-implements-port-skips-gateway" "go" '**/driver/**' '!*_test.go' '"([^"]+/port(/[^"]*)?)"' || dv=1
    run_check "VIOLATION" "driver-imports-domain" "go" '**/driver/**' '!*_test.go' '"([^"]+/domain(/[^"]*)?)"' || dv=1
    [ "$MODE" = "dir" ] && [ "$dv" -eq 0 ] && has_files "go" '**/driver/**' '!*_test.go' && echo "[clean] Go driver: clean"

    # Domain
    local dmv=0
    run_check "VIOLATION" "domain-imports-sibling-layers" "go" '**/domain/**' '!*_test.go' '"([^"]+/(usecase|port|gateway|driver|rest|handler)(/[^"]*)?)"' || dmv=1
    run_check "VIOLATION" "domain-imports-sql" "go" '**/domain/**' '!*_test.go' '"(database/sql|github\.com/jackc/pgx[^"]*)"' || dmv=1
    run_check "VIOLATION" "domain-imports-net-http" "go" '**/domain/**' '!*_test.go' '"net/http"' || dmv=1
    run_check "VIOLATION" "domain-imports-otel" "go" '**/domain/**' '!*_test.go' '"go\.opentelemetry\.io[^"]*"' || dmv=1
    [ "$MODE" = "dir" ] && [ "$dmv" -eq 0 ] && has_files "go" '**/domain/**' '!*_test.go' && echo "[clean] Go domain: clean"
  fi

  # --- Python checks ---
  if { rg --files --type py "$SERVICE_DIR" 2>/dev/null || true; } | head -n 1 | grep -q .; then
    local py_exc='!*test*'

    # Handler / Router
    local py_hv=0
    run_check "VIOLATION" "py-handler-imports-driver-or-gateway" "py" '**/handler/**' "$py_exc" '^(from|import)\s+.*(\.driver|\.gateway)' || py_hv=1
    run_check "VIOLATION" "py-handler-imports-driver-or-gateway" "py" '**/routers/**' "$py_exc" '^(from|import)\s+.*(\.driver|\.gateway)' || py_hv=1
    [ "$MODE" = "dir" ] && [ "$py_hv" -eq 0 ] && (has_files "py" '**/handler/**' "$py_exc" || has_files "py" '**/routers/**' "$py_exc") && echo "[clean] Python handler/routers: clean"

    # Usecase
    local py_uv=0
    run_check "VIOLATION" "py-usecase-imports-infra-or-outer" "py" '**/usecase/**' "$py_exc" '^(from|import)\s+.*(httpx|asyncpg|redis|sqlalchemy|fastapi|starlette|psycopg|\.driver|\.gateway|\.handler)' || py_uv=1
    [ "$MODE" = "dir" ] && [ "$py_uv" -eq 0 ] && has_files "py" '**/usecase/**' "$py_exc" && echo "[clean] Python usecase: clean"

    # Port
    local py_pv=0
    run_check "VIOLATION" "py-port-imports-infra" "py" '**/port/**' "$py_exc" '^(from|import)\s+.*(httpx|asyncpg|redis|sqlalchemy|fastapi|starlette|psycopg|\.driver|\.gateway|\.handler)' || py_pv=1
    [ "$MODE" = "dir" ] && [ "$py_pv" -eq 0 ] && has_files "py" '**/port/**' "$py_exc" && echo "[clean] Python port: clean"

    # Driver
    local py_dv=0
    run_check "VIOLATION" "py-driver-imports-upward" "py" '**/driver/**' "$py_exc" '^(from|import)\s+.*(\.usecase|\.handler|\.gateway)' || py_dv=1
    run_check "VIOLATION" "py-driver-imports-port-skips-gateway" "py" '**/driver/**' "$py_exc" '^(from|import)\s+.*\.port' || py_dv=1
    run_check "VIOLATION" "py-driver-imports-domain" "py" '**/driver/**' "$py_exc" '^(from|import)\s+.*\.domain' || py_dv=1
    [ "$MODE" = "dir" ] && [ "$py_dv" -eq 0 ] && has_files "py" '**/driver/**' "$py_exc" && echo "[clean] Python driver: clean"

    # Domain
    local py_dmv=0
    run_check "VIOLATION" "py-domain-imports-infra-or-layers" "py" '**/domain/**' "$py_exc" '^(from|import)\s+.*(httpx|asyncpg|redis|sqlalchemy|fastapi|starlette|psycopg|\.driver|\.gateway|\.handler|\.usecase|\.port)' || py_dmv=1
    [ "$MODE" = "dir" ] && [ "$py_dmv" -eq 0 ] && has_files "py" '**/domain/**' "$py_exc" && echo "[clean] Python domain: clean"
  fi
  return 0
}

check_single_file() {
  local file_path="$1"
  if [ ! -f "$file_path" ]; then
    return 0
  fi

  local base marker
  base="$(basename "$file_path")"
  case "$base" in
    *_test.go)
      return 0
      ;;
    *.go)
      marker="go.mod"
      ;;
    *.py)
      if [[ "$base" == *test* ]]; then
        return 0
      fi
      marker="pyproject.toml"
      ;;
    *)
      return 0
      ;;
  esac

  local file_dir abs_file dir service_root=""
  file_dir="$(cd "$(dirname "$file_path")" && pwd -P)"
  abs_file="$file_dir/$base"

  dir="$file_dir"
  while true; do
    if [ -f "$dir/$marker" ]; then
      service_root="$dir"
      break
    fi
    if [ "$dir" = "/" ]; then
      break
    fi
    dir="$(dirname "$dir")"
  done

  if [ -z "$service_root" ]; then
    return 0
  fi

  SERVICE_DIR="."
  ABS_SERVICE_DIR="$service_root"
  REPO_ROOT="$(resolve_repo_root "$service_root")"
  FILTER_ABS_FILE="$abs_file"
  DISPLAY_FILE="$file_path"

  pushd "$service_root" >/dev/null
  run_layer_checks
  popd >/dev/null
  return 0
}

load_known_ledger

case "$MODE" in
  dir)
    [ -d "$SERVICE_DIR" ] || { echo "Error: '$SERVICE_DIR' not found" >&2; exit 2; }
    ABS_SERVICE_DIR="$(cd "$SERVICE_DIR" && pwd -P)"
    REPO_ROOT="$(resolve_repo_root "$SERVICE_DIR")"
    FILTER_ABS_FILE=""
    DISPLAY_FILE=""
    run_layer_checks
    emit_known_lines
    emit_fix_lines
    [ "$VIOLATIONS" -gt 0 ] && exit 1
    exit 0
    ;;
  file)
    check_single_file "$TARGET_FILE"
    if [ "${#COLLECTED_VIOLATIONS[@]}" -gt 0 ]; then
      printf '%s\n' "${COLLECTED_VIOLATIONS[@]}"
    fi
    emit_fix_lines
    [ "$VIOLATIONS" -gt 0 ] && exit 1
    exit 0
    ;;
  changed)
    CHANGED_ROOT="$(resolve_repo_root ".")"
    porcelain_out="$(git -c core.quotePath=false -C "$CHANGED_ROOT" status --porcelain -uall)" || exit 2
    if [ -n "$porcelain_out" ]; then
      cwd_phys="$(pwd -P)"
      while IFS= read -r status_line; do
        [ -z "$status_line" ] && continue
        xy="${status_line:0:2}"
        raw_path="${status_line:3}"
        if [[ "$xy" == R* || "$xy" == *R || "$xy" == C* || "$xy" == *C ]]; then
          raw_path="${raw_path##* -> }"
        fi
        if [[ "$raw_path" == \"*\" ]]; then
          raw_path="${raw_path#\"}"
          raw_path="${raw_path%\"}"
        fi
        if [ "$cwd_phys" = "$CHANGED_ROOT" ]; then
          file_arg="$raw_path"
        else
          file_arg="$CHANGED_ROOT/$raw_path"
        fi
        check_single_file "$file_arg"
      done <<< "$porcelain_out"
    fi
    if [ "${#COLLECTED_VIOLATIONS[@]}" -gt 0 ]; then
      printf '%s\n' "${COLLECTED_VIOLATIONS[@]}"
    fi
    emit_fix_lines
    [ "$VIOLATIONS" -gt 0 ] && exit 1
    exit 0
    ;;
  hook)
    if ! command -v jq >/dev/null 2>&1; then
      echo "check_layers.sh --hook: jq is required" >&2
      exit 2
    fi
    hook_file="$(jq -r 'if (type == "object") and (.tool_input | type) == "object" and (.tool_input.file_path | type) == "string" then .tool_input.file_path else empty end' 2>/dev/null)" || exit 0
    [ -n "$hook_file" ] || exit 0
    check_single_file "$hook_file"
    if [ "$VIOLATIONS" -gt 0 ]; then
      {
        if [ "${#COLLECTED_VIOLATIONS[@]}" -gt 0 ]; then
          printf '%s\n' "${COLLECTED_VIOLATIONS[@]}"
        fi
        emit_fix_lines
        echo "See .claude/skills/clean-architecture/SKILL.md section 9."
      } >&2
      exit 2
    fi
    exit 0
    ;;
esac
