#!/usr/bin/env bash
# Unit and integration tests for check_layers.sh
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CHECKER="$SCRIPT_DIR/check_layers.sh"

TMP_ROOT="$(mktemp -d)"
trap 'rm -rf "$TMP_ROOT"' EXIT

IN_FILE="$TMP_ROOT/_stdin"
OUT_FILE="$TMP_ROOT/_stdout"
ERR_FILE="$TMP_ROOT/_stderr"
EMPTY_KNOWN="$TMP_ROOT/empty_known.tsv"
: >"$EMPTY_KNOWN"

OUT=""
ERR=""
RC=0
FAILURES=0

pass() {
  echo "ok - $1"
}

fail() {
  echo "not ok - $1 ($2)"
  FAILURES=$((FAILURES + 1))
}

run_checker() {
  local root="$1" known="$2"
  shift 2
  : >"$OUT_FILE"
  : >"$ERR_FILE"
  RC=0
  CHECK_LAYERS_ROOT="$root" CHECK_LAYERS_KNOWN="$known" bash "$CHECKER" "$@" >"$OUT_FILE" 2>"$ERR_FILE" || RC=$?
  OUT="$(cat "$OUT_FILE")"
  ERR="$(cat "$ERR_FILE")"
}

run_checker_stdin() {
  local root="$1" known="$2" input="$3"
  shift 3
  printf '%s' "$input" >"$IN_FILE"
  : >"$OUT_FILE"
  : >"$ERR_FILE"
  RC=0
  CHECK_LAYERS_ROOT="$root" CHECK_LAYERS_KNOWN="$known" bash "$CHECKER" "$@" <"$IN_FILE" >"$OUT_FILE" 2>"$ERR_FILE" || RC=$?
  OUT="$(cat "$OUT_FILE")"
  ERR="$(cat "$ERR_FILE")"
}

count_prefix_lines() {
  local haystack="$1" prefix="$2" count=0 line
  while IFS= read -r line; do
    if [[ "$line" == "$prefix"* ]]; then
      count=$((count + 1))
    fi
  done <<< "$haystack"
  echo "$count"
}

# --- Case 1: clean Go service ---
c1="$TMP_ROOT/case1"
mkdir -p "$c1/svc/domain" "$c1/svc/port" "$c1/svc/usecase"
cat >"$c1/svc/go.mod" <<'EOF'
module example.com/svc

go 1.24
EOF
cat >"$c1/svc/domain/item.go" <<'EOF'
package domain

type Item struct {
	ID string
}
EOF
cat >"$c1/svc/port/finder.go" <<'EOF'
package port

import (
	"example.com/svc/domain"
)

type Finder interface {
	Find(id string) (domain.Item, error)
}
EOF
cat >"$c1/svc/usecase/get_item.go" <<'EOF'
package usecase

import (
	"example.com/svc/domain"
	"example.com/svc/port"
)

type GetItem struct {
	finder port.Finder
}

func (u *GetItem) Execute(id string) (domain.Item, error) {
	return u.finder.Find(id)
}
EOF

run_checker "$c1" "$EMPTY_KNOWN" "$c1/svc"
if [ "$RC" -eq 0 ] && [[ "$OUT" == *"[clean] Go usecase: clean"* ]] && [[ "$OUT" != *"VIOLATION"* ]]; then
  pass "clean Go service exits 0 and reports clean usecase"
else
  fail "clean Go service exits 0 and reports clean usecase" "rc=$RC out=$OUT err=$ERR"
fi

# --- Case 2: Go usecase importing driver (two violating files, one FIX line) ---
c2="$TMP_ROOT/case2"
mkdir -p "$c2/svc/usecase"
cat >"$c2/svc/go.mod" <<'EOF'
module example.com/svc

go 1.24
EOF
cat >"$c2/svc/usecase/first.go" <<'EOF'
package usecase

import (
	"example.com/svc/driver/postgres"
)
EOF
cat >"$c2/svc/usecase/second.go" <<'EOF'
package usecase

import (
	"example.com/svc/driver/redis"
)
EOF

run_checker "$c2" "$EMPTY_KNOWN" "$c2/svc"
fix_count="$(count_prefix_lines "$OUT" "FIX usecase-imports-forbidden-layers-or-sql: ")"
if [ "$RC" -eq 1 ] \
  && [[ "$OUT" == *"VIOLATION usecase-imports-forbidden-layers-or-sql"*"first.go"* ]] \
  && [[ "$OUT" == *"VIOLATION usecase-imports-forbidden-layers-or-sql"*"second.go"* ]] \
  && [ "$fix_count" -eq 1 ]; then
  pass "Go usecase importing driver exits 1 with deduplicated FIX line"
else
  fail "Go usecase importing driver exits 1 with deduplicated FIX line" "rc=$RC fix_count=$fix_count out=$OUT"
fi

# --- Case 3: Python usecase importing httpx ---
c3="$TMP_ROOT/case3"
mkdir -p "$c3/svc/usecase"
cat >"$c3/svc/pyproject.toml" <<'EOF'
[project]
name = "svc"
version = "0.1.0"
EOF
cat >"$c3/svc/usecase/fetch.py" <<'EOF'
import httpx
EOF

run_checker "$c3" "$EMPTY_KNOWN" "$c3/svc"
if [ "$RC" -eq 1 ] \
  && [[ "$OUT" == *"VIOLATION py-usecase-imports-infra-or-outer"* ]] \
  && [[ "$OUT" == *"FIX py-usecase-imports-infra-or-outer: "* ]]; then
  pass "Python usecase importing httpx exits 1 with VIOLATION and FIX"
else
  fail "Python usecase importing httpx exits 1 with VIOLATION and FIX" "rc=$RC out=$OUT"
fi

# --- Case 4: ledger entry matching rule + prefix + * ---
c4="$TMP_ROOT/case4"
mkdir -p "$c4/svc/driver"
cat >"$c4/svc/go.mod" <<'EOF'
module example.com/svc

go 1.24
EOF
cat >"$c4/svc/driver/repo.go" <<'EOF'
package driver

import (
	"example.com/svc/domain"
)
EOF
printf '# comment\n\ndriver-imports-domain\tsvc/driver/\t*\tlegacy driver returns domain\n' >"$c4/known.tsv"

run_checker "$c4" "$c4/known.tsv" "$c4/svc"
if [ "$RC" -eq 0 ] \
  && [[ "$OUT" == *"KNOWN driver-imports-domain svc/driver/ (1 lines): legacy driver returns domain"* ]] \
  && [[ "$OUT" != *"VIOLATION"* ]] \
  && [[ "$OUT" != *"FIX "* ]] \
  && [[ "$OUT" != *"[clean] Go driver: clean"* ]]; then
  pass "wildcard ledger entry marks match KNOWN, suppresses [clean], and exits 0"
else
  fail "wildcard ledger entry marks match KNOWN, suppresses [clean], and exits 0" "rc=$RC out=$OUT"
fi

# --- Case 5: ledger entry with specific line-substring ---
c5="$TMP_ROOT/case5"
mkdir -p "$c5/svc/usecase"
cat >"$c5/svc/go.mod" <<'EOF'
module example.com/svc

go 1.24
EOF
cat >"$c5/svc/usecase/mixed.go" <<'EOF'
package usecase

import (
	"example.com/svc/driver/sovereign_db"
	"example.com/svc/handler"
)
EOF
printf 'usecase-imports-forbidden-layers-or-sql\tsvc/usecase/\texample.com/svc/driver/sovereign_db\tdocumented reduced shape\n' >"$c5/known.tsv"

run_checker "$c5" "$c5/known.tsv" "$c5/svc"
if [ "$RC" -eq 1 ] \
  && [[ "$OUT" == *"KNOWN usecase-imports-forbidden-layers-or-sql svc/usecase/ (1 lines): documented reduced shape"* ]] \
  && [[ "$OUT" != *"VIOLATION"*"sovereign_db"* ]] \
  && [[ "$OUT" == *"VIOLATION usecase-imports-forbidden-layers-or-sql"*"example.com/svc/handler"* ]]; then
  pass "substring ledger entry keeps matching import KNOWN and other forbidden import VIOLATION"
else
  fail "substring ledger entry keeps matching import KNOWN and other forbidden import VIOLATION" "rc=$RC out=$OUT"
fi

# --- Case 6: --file on a violating file ignores sibling violations, [clean], WARN, KNOWN ---
c6="$TMP_ROOT/case6"
mkdir -p "$c6/svc/usecase" "$c6/svc/domain" "$c6/svc/driver"
cat >"$c6/svc/go.mod" <<'EOF'
module example.com/svc

go 1.24
EOF
cat >"$c6/svc/usecase/target.go" <<'EOF'
package usecase

import (
	"example.com/svc/driver/db"
	"go.opentelemetry.io/otel/trace"
)
EOF
cat >"$c6/svc/usecase/other_bad.go" <<'EOF'
package usecase

import (
	"example.com/svc/gateway/repo"
)
EOF
cat >"$c6/svc/domain/clean.go" <<'EOF'
package domain

type Entity struct{}
EOF
cat >"$c6/svc/driver/known_drv.go" <<'EOF'
package driver

import (
	"example.com/svc/domain"
)
EOF
printf 'driver-imports-domain\tsvc/driver/\t*\tdocumented debt\n' >"$c6/known.tsv"

run_checker "$c6" "$c6/known.tsv" --file "$c6/svc/usecase/target.go"
if [ "$RC" -eq 1 ] \
  && [[ "$OUT" == *"VIOLATION usecase-imports-forbidden-layers-or-sql"*"target.go"* ]] \
  && [[ "$OUT" == *"FIX usecase-imports-forbidden-layers-or-sql: "* ]] \
  && [[ "$OUT" != *"other_bad.go"* ]] \
  && [[ "$OUT" != *"[clean]"* ]] \
  && [[ "$OUT" != *"WARN"* ]] \
  && [[ "$OUT" != *"KNOWN"* ]]; then
  pass "--file reports only target file VIOLATION and FIX lines"
else
  fail "--file reports only target file VIOLATION and FIX lines" "rc=$RC out=$OUT err=$ERR"
fi

# --- Case 7: --file on clean file, _test.go, .md, and nonexistent path ---
c7="$TMP_ROOT/case7"
mkdir -p "$c7/svc/usecase"
cat >"$c7/svc/go.mod" <<'EOF'
module example.com/svc

go 1.24
EOF
cat >"$c7/svc/usecase/clean.go" <<'EOF'
package usecase

import (
	"example.com/svc/port"
)
EOF
cat >"$c7/svc/usecase/violating_test.go" <<'EOF'
package usecase

import (
	"example.com/svc/driver/db"
)
EOF
cat >"$c7/svc/README.md" <<'EOF'
# Service
EOF

c7_ok=1
for f in \
  "$c7/svc/usecase/clean.go" \
  "$c7/svc/usecase/violating_test.go" \
  "$c7/svc/README.md" \
  "$c7/svc/usecase/missing.go"; do
  run_checker "$c7" "$EMPTY_KNOWN" --file "$f"
  if [ "$RC" -ne 0 ] || [ -n "$OUT" ] || [ -n "$ERR" ]; then
    c7_ok=0
    break
  fi
done
if [ "$c7_ok" -eq 1 ]; then
  pass "--file exits 0 silently on clean, _test.go, .md, and nonexistent paths"
else
  fail "--file exits 0 silently on clean, _test.go, .md, and nonexistent paths" "file=$f rc=$RC out=$OUT err=$ERR"
fi

# --- Case 8: --changed in git repo with committed and uncommitted violating files ---
c8="$TMP_ROOT/case8"
mkdir -p "$c8/svc/usecase"
git -C "$c8" init -q
git -C "$c8" config user.email "test@example.com"
git -C "$c8" config user.name "Test"
cat >"$c8/svc/go.mod" <<'EOF'
module example.com/svc

go 1.24
EOF
cat >"$c8/svc/usecase/tracked_bad.go" <<'EOF'
package usecase

import (
	"example.com/svc/driver/db"
)
EOF
git -C "$c8" add .
git -C "$c8" commit -q -m "initial"

cat >"$c8/svc/usecase/fresh_bad.go" <<'EOF'
package usecase

import (
	"example.com/svc/driver/db"
)
EOF

: >"$OUT_FILE"
: >"$ERR_FILE"
RC=0
(cd "$c8" && CHECK_LAYERS_KNOWN="$EMPTY_KNOWN" bash "$CHECKER" --changed) >"$OUT_FILE" 2>"$ERR_FILE" || RC=$?
OUT="$(cat "$OUT_FILE")"
ERR="$(cat "$ERR_FILE")"
c8_fix="$(count_prefix_lines "$OUT" "FIX usecase-imports-forbidden-layers-or-sql: ")"
if [ "$RC" -eq 1 ] \
  && [[ "$OUT" == *"fresh_bad.go"* ]] \
  && [[ "$OUT" != *"tracked_bad.go"* ]] \
  && [ "$c8_fix" -eq 1 ]; then
  pass "--changed checks only uncommitted files and exits 1 on violation"
else
  fail "--changed checks only uncommitted files and exits 1 on violation" "rc=$RC out=$OUT err=$ERR"
fi

# --- Case 9: --changed with a clean working tree ---
rm -f "$c8/svc/usecase/fresh_bad.go"
: >"$OUT_FILE"
: >"$ERR_FILE"
RC=0
(cd "$c8" && CHECK_LAYERS_KNOWN="$EMPTY_KNOWN" bash "$CHECKER" --changed) >"$OUT_FILE" 2>"$ERR_FILE" || RC=$?
OUT="$(cat "$OUT_FILE")"
ERR="$(cat "$ERR_FILE")"
if [ "$RC" -eq 0 ] && [ -z "$OUT" ] && [ -z "$ERR" ]; then
  pass "--changed on clean working tree exits 0 with empty output"
else
  fail "--changed on clean working tree exits 0 with empty output" "rc=$RC out=$OUT err=$ERR"
fi

# --- Case 10: --hook with violating file ---
hook_json="{\"tool_input\":{\"file_path\":\"$c6/svc/usecase/target.go\"}}"
run_checker_stdin "$c6" "$c6/known.tsv" "$hook_json" --hook
if [ "$RC" -eq 2 ] \
  && [ -z "$OUT" ] \
  && [[ "$ERR" == *"VIOLATION usecase-imports-forbidden-layers-or-sql"*"target.go"* ]] \
  && [[ "$ERR" == *"FIX usecase-imports-forbidden-layers-or-sql: "* ]] \
  && [[ "$ERR" == *"See .claude/skills/clean-architecture/SKILL.md section 9."* ]]; then
  pass "--hook exits 2 and writes VIOLATION, FIX, and SKILL.md pointer to stderr"
else
  fail "--hook exits 2 and writes VIOLATION, FIX, and SKILL.md pointer to stderr" "rc=$RC out=$OUT err=$ERR"
fi

# --- Case 11: --hook with clean file, {}, and non-JSON stdin ---
c11_ok=1
for payload in \
  "{\"tool_input\":{\"file_path\":\"$c7/svc/usecase/clean.go\"}}" \
  "{}" \
  "not-valid-json{{{"; do
  run_checker_stdin "$c7" "$EMPTY_KNOWN" "$payload" --hook
  if [ "$RC" -ne 0 ] || [ -n "$OUT" ] || [ -n "$ERR" ]; then
    c11_ok=0
    break
  fi
done
if [ "$c11_ok" -eq 1 ]; then
  pass "--hook exits 0 silently on clean file, empty object, and invalid JSON"
else
  fail "--hook exits 0 silently on clean file, empty object, and invalid JSON" "payload=$payload rc=$RC out=$OUT err=$ERR"
fi

# --- Case 12: no arguments -> exit 2 ---
run_checker "$TMP_ROOT" "$EMPTY_KNOWN"
if [ "$RC" -eq 2 ] && [ -z "$OUT" ] && [ -n "$ERR" ]; then
  pass "no arguments prints usage to stderr and exits 2"
else
  fail "no arguments prints usage to stderr and exits 2" "rc=$RC out=$OUT err=$ERR"
fi

# --- Case 13: ledger rule mismatch does not exempt different rule in same file ---
c13="$TMP_ROOT/case13"
mkdir -p "$c13/svc/driver"
cat >"$c13/svc/go.mod" <<'EOF'
module example.com/svc

go 1.24
EOF
cat >"$c13/svc/driver/adapter.go" <<'EOF'
package driver

import (
	"example.com/svc/port"
)
EOF
printf 'driver-imports-domain\tsvc/driver/\t*\tonly domain import exempted\n' >"$c13/known.tsv"

run_checker "$c13" "$c13/known.tsv" "$c13/svc"
if [ "$RC" -eq 1 ] \
  && [[ "$OUT" == *"VIOLATION driver-implements-port-skips-gateway"*"adapter.go"* ]] \
  && [[ "$OUT" != *"KNOWN"* ]]; then
  pass "ledger rule mismatch keeps other rule hit as VIOLATION"
else
  fail "ledger rule mismatch keeps other rule hit as VIOLATION" "rc=$RC out=$OUT err=$ERR"
fi

# --- Case 14: ledger prefix anchored at start of repo-relative path ---
c14="$TMP_ROOT/case14"
mkdir -p "$c14/svc/driver" "$c14/other/svc/driver"
cat >"$c14/svc/go.mod" <<'EOF'
module example.com/svc

go 1.24
EOF
cat >"$c14/svc/driver/alpha.go" <<'EOF'
package driver

import (
	"example.com/svc/domain"
)
EOF
cat >"$c14/other/svc/go.mod" <<'EOF'
module example.com/other/svc

go 1.24
EOF
cat >"$c14/other/svc/driver/bravo.go" <<'EOF'
package driver

import (
	"example.com/other/svc/domain"
)
EOF
printf 'driver-imports-domain\tsvc/driver/\t*\tanchored at repo root\n' >"$c14/known.tsv"

run_checker "$c14" "$c14/known.tsv" "$c14/svc"
c14_rc1="$RC"
c14_out1="$OUT"
run_checker "$c14" "$c14/known.tsv" "$c14/other/svc"
c14_rc2="$RC"
c14_out2="$OUT"
if [ "$c14_rc1" -eq 0 ] \
  && [[ "$c14_out1" == *"KNOWN driver-imports-domain svc/driver/ (1 lines): anchored at repo root"* ]] \
  && [[ "$c14_out1" != *"VIOLATION"* ]] \
  && [ "$c14_rc2" -eq 1 ] \
  && [[ "$c14_out2" == *"VIOLATION driver-imports-domain"*"bravo.go"* ]] \
  && [[ "$c14_out2" != *"KNOWN"* ]]; then
  pass "ledger prefix is anchored at start of repo-relative path"
else
  fail "ledger prefix is anchored at start of repo-relative path" "rc1=$c14_rc1 out1=$c14_out1 rc2=$c14_rc2 out2=$c14_out2"
fi

# --- Case 15: --file and --hook on file whose only hit is covered by ledger ---
c15="$TMP_ROOT/case15"
mkdir -p "$c15/svc/driver"
cat >"$c15/svc/go.mod" <<'EOF'
module example.com/svc

go 1.24
EOF
cat >"$c15/svc/driver/exempted.go" <<'EOF'
package driver

import (
	"example.com/svc/domain"
)
EOF
printf 'driver-imports-domain\tsvc/driver/\t*\trecorded legacy\n' >"$c15/known.tsv"

run_checker "$c15" "$c15/known.tsv" --file "$c15/svc/driver/exempted.go"
c15_file_rc="$RC"
c15_file_out="$OUT"
c15_file_err="$ERR"
run_checker_stdin "$c15" "$c15/known.tsv" "{\"tool_input\":{\"file_path\":\"$c15/svc/driver/exempted.go\"}}" --hook
if [ "$c15_file_rc" -eq 0 ] && [ -z "$c15_file_out" ] && [ -z "$c15_file_err" ] \
  && [ "$RC" -eq 0 ] && [ -z "$OUT" ] && [ -z "$ERR" ]; then
  pass "--file and --hook exit 0 silently when only hit is KNOWN"
else
  fail "--file and --hook exit 0 silently when only hit is KNOWN" "file_rc=$c15_file_rc file_out=$c15_file_out hook_rc=$RC hook_err=$ERR"
fi

# --- Case 16: --file with working directory inside the file's own layer directory ---
c16="$TMP_ROOT/case16"
mkdir -p "$c16/svc/orchestrator/driver/client"
cat >"$c16/svc/go.mod" <<'EOF'
module example.com/svc

go 1.24
EOF
cat >"$c16/svc/orchestrator/driver/client/http_client.go" <<'EOF'
package client

import (
	"example.com/svc/domain"
)
EOF

: >"$OUT_FILE"
: >"$ERR_FILE"
RC=0
(cd "$c16/svc/orchestrator/driver" && CHECK_LAYERS_ROOT="$c16" CHECK_LAYERS_KNOWN="$EMPTY_KNOWN" bash "$CHECKER" --file "$c16/svc/orchestrator/driver/client/http_client.go") >"$OUT_FILE" 2>"$ERR_FILE" || RC=$?
OUT="$(cat "$OUT_FILE")"
ERR="$(cat "$ERR_FILE")"
if [ "$RC" -eq 1 ] && [[ "$OUT" == *"VIOLATION driver-imports-domain"*"http_client.go"* ]]; then
  pass "--file invoked from inside a layer directory still reports VIOLATION"
else
  fail "--file invoked from inside a layer directory still reports VIOLATION" "rc=$RC out=$OUT err=$ERR"
fi

# --- Case 17: --changed from subdirectory with service directory containing space ---
c17="$TMP_ROOT/case17"
mkdir -p "$c17/spaced svc/usecase" "$c17/work_subdir"
git -C "$c17" init -q
git -C "$c17" config user.email "test@example.com"
git -C "$c17" config user.name "Test"
cat >"$c17/spaced svc/go.mod" <<'EOF'
module example.com/spaced

go 1.24
EOF
: >"$c17/work_subdir/.keep"
git -C "$c17" add .
git -C "$c17" commit -q -m "initial"

cat >"$c17/spaced svc/usecase/leak_uc.go" <<'EOF'
package usecase

import (
	"example.com/spaced/driver/db"
)
EOF

: >"$OUT_FILE"
: >"$ERR_FILE"
RC=0
(cd "$c17/work_subdir" && CHECK_LAYERS_KNOWN="$EMPTY_KNOWN" bash "$CHECKER" --changed) >"$OUT_FILE" 2>"$ERR_FILE" || RC=$?
OUT="$(cat "$OUT_FILE")"
ERR="$(cat "$ERR_FILE")"
if [ "$RC" -eq 1 ] && [[ "$OUT" == *"VIOLATION usecase-imports-forbidden-layers-or-sql"*"spaced svc/usecase/leak_uc.go"* ]]; then
  pass "--changed from subdirectory handles service directory name with space"
else
  fail "--changed from subdirectory handles service directory name with space" "rc=$RC out=$OUT err=$ERR"
fi

# --- Case 18: --changed after git mv reports new path and not old path ---
c18="$TMP_ROOT/case18"
mkdir -p "$c18/svc/usecase"
git -C "$c18" init -q
git -C "$c18" config user.email "test@example.com"
git -C "$c18" config user.name "Test"
cat >"$c18/svc/go.mod" <<'EOF'
module example.com/svc

go 1.24
EOF
cat >"$c18/svc/usecase/legacy_name.go" <<'EOF'
package usecase

import (
	"example.com/svc/driver/db"
)
EOF
git -C "$c18" add .
git -C "$c18" commit -q -m "initial"
git -C "$c18" mv svc/usecase/legacy_name.go svc/usecase/renamed_target.go

: >"$OUT_FILE"
: >"$ERR_FILE"
RC=0
(cd "$c18" && CHECK_LAYERS_KNOWN="$EMPTY_KNOWN" bash "$CHECKER" --changed) >"$OUT_FILE" 2>"$ERR_FILE" || RC=$?
OUT="$(cat "$OUT_FILE")"
ERR="$(cat "$ERR_FILE")"
if [ "$RC" -eq 1 ] && [[ "$OUT" == *"renamed_target.go"* ]] && [[ "$OUT" != *"legacy_name.go"* ]]; then
  pass "--changed after git mv reports renamed path and not old path"
else
  fail "--changed after git mv reports renamed path and not old path" "rc=$RC out=$OUT err=$ERR"
fi

# --- Case 19: malformed ledger line exits 2 with error on stderr ---
c19="$TMP_ROOT/case19"
mkdir -p "$c19/svc/domain"
cat >"$c19/svc/go.mod" <<'EOF'
module example.com/svc

go 1.24
EOF
cat >"$c19/svc/domain/model.go" <<'EOF'
package domain

type Model struct{}
EOF
printf 'driver-imports-domain\n' >"$c19/bad_known.tsv"

run_checker "$c19" "$c19/bad_known.tsv" "$c19/svc"
if [ "$RC" -eq 2 ] && [[ "$ERR" == *"malformed ledger line"* ]]; then
  pass "malformed ledger line exits 2 with diagnostic on stderr"
else
  fail "malformed ledger line exits 2 with diagnostic on stderr" "rc=$RC out=$OUT err=$ERR"
fi

# --- Case 20: shipped known_violations.tsv loads cleanly when CHECK_LAYERS_KNOWN is unset ---
c20="$TMP_ROOT/case20"
mkdir -p "$c20/svc/usecase"
cat >"$c20/svc/go.mod" <<'EOF'
module example.com/svc

go 1.24
EOF
cat >"$c20/svc/usecase/clean_flow.go" <<'EOF'
package usecase

import (
	"example.com/svc/port"
)
EOF

: >"$OUT_FILE"
: >"$ERR_FILE"
RC=0
(unset CHECK_LAYERS_KNOWN; CHECK_LAYERS_ROOT="$c20" bash "$CHECKER" "$c20/svc") >"$OUT_FILE" 2>"$ERR_FILE" || RC=$?
OUT="$(cat "$OUT_FILE")"
ERR="$(cat "$ERR_FILE")"
if [ "$RC" -eq 0 ] && [ -z "$ERR" ] && [[ "$OUT" == *"[clean] Go usecase: clean"* ]]; then
  pass "shipped known_violations.tsv loads cleanly by default"
else
  fail "shipped known_violations.tsv loads cleanly by default" "rc=$RC out=$OUT err=$ERR"
fi

if [ "$FAILURES" -gt 0 ]; then
  exit 1
fi
exit 0
