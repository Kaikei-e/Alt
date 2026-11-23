#!/usr/bin/env bash
# The project's Dockerfile (`FROM oven/bun:<version>`) is the source of truth for
# the Bun version pin; workflow values (alt-frontend-sv-unit-test.yaml,
# proto-contract.yaml) must match it.
set -euo pipefail

if [ "$#" -ne 1 ] || [ -z "$1" ]; then
  echo "usage: $0 <project-dir>" >&2
  exit 2
fi

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
project_rel="${1#./}"
project_rel="${project_rel%/}"
project_dir="${repo_root}/${project_rel}"

if [ ! -d "$project_dir" ] || [ ! -f "${project_dir}/package.json" ]; then
  echo "error: ${project_rel} is not a frontend project directory (missing package.json)" >&2
  exit 2
fi

if [ ! -d "${project_dir}/node_modules" ]; then
  echo "error: ${project_rel}/node_modules is missing; run \`bun install --frozen-lockfile\` in ${project_rel}" >&2
  exit 1
fi

for cmd in bun python3; do
  if ! command -v "$cmd" >/dev/null 2>&1; then
    echo "error: required command not found in PATH: $cmd" >&2
    exit 1
  fi
done

dockerfile="${project_dir}/Dockerfile"
if [ ! -f "$dockerfile" ]; then
  echo "error: ${project_rel}/Dockerfile is missing (source of truth for Bun version pin)" >&2
  exit 1
fi

bun_pins="$(sed -n 's/^FROM[[:space:]]\{1,\}oven\/bun:\([0-9][0-9.]*\)\(-[^[:space:]]*\)\{0,1\}.*/\1/p' "$dockerfile" | sort -u)"
if [ -z "$bun_pins" ] || [ "$(wc -l <<<"$bun_pins")" -ne 1 ]; then
  echo "error: could not parse a single Bun version pin from ${project_rel}/Dockerfile (expected 'FROM oven/bun:<version>')" >&2
  exit 1
fi
expected_bun_version="$bun_pins"

actual_bun_version="$(bun --version | tr -d '[:space:]')"
if [ "$actual_bun_version" != "$expected_bun_version" ]; then
  echo "error: bun version mismatch: expected ${expected_bun_version}, got '${actual_bun_version}'" >&2
  exit 1
fi

if [ ! -f "${project_dir}/bun.lock" ]; then
  echo "error: ${project_rel}/bun.lock is missing" >&2
  exit 1
fi

python3 - "${project_dir}" "@biomejs/biome" "svelte-check" <<'PY'
import json
import pathlib
import re
import sys

project_dir = pathlib.Path(sys.argv[1])
lock_path = project_dir / "bun.lock"
lock_text = lock_path.read_text(encoding="utf-8")

errors: list[str] = []
for pkg in sys.argv[2:]:
    pattern = re.compile(
        rf'^\s*"{re.escape(pkg)}":\s*\["{re.escape(pkg)}@([^"]+)"',
        re.MULTILINE,
    )
    matches = set(pattern.findall(lock_text))
    if len(matches) != 1:
        errors.append(
            f"error: expected exactly 1 locked version for {pkg} in {lock_path}, found {sorted(matches)}"
        )
        continue
    locked_ver = next(iter(matches))
    installed_pkg_json = project_dir / "node_modules" / pkg / "package.json"
    if not installed_pkg_json.is_file():
        errors.append(f"error: {installed_pkg_json} is missing; run `bun install --frozen-lockfile`")
        continue
    installed_data = json.loads(installed_pkg_json.read_text(encoding="utf-8"))
    installed_ver = installed_data.get("version")
    if installed_ver != locked_ver:
        errors.append(
            f"error: {pkg} version mismatch: bun.lock pins {locked_ver}, node_modules has {installed_ver}"
        )

if errors:
    sys.stderr.write("\n".join(errors) + "\n")
    sys.exit(1)
PY

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
canary_audit_script="${script_dir}/canary_audit.py"
if [ -f "${canary_audit_script}" ]; then
  echo "==> [${project_rel}] canary audit (biome)"
  if ! python3 "${canary_audit_script}" frontend-canary \
    --project-dir "${project_dir}"; then
    exit 1
  fi
elif [ -f "${repo_root}/scripts/lint/run.sh" ]; then
  echo "error: missing canary audit helper: ${canary_audit_script}" >&2
  exit 1
fi

cd "$project_dir"

fmt_rc=0
fmt_status="pass"
lint_rc=0
lint_policy="${LINT_POLICY:-fix-existing}"
if [ "$lint_policy" = "block-new" ]; then
  if ./node_modules/.bin/biome format .; then
    fmt_status="pass"
  else
    fmt_rc=$?
    fmt_status="fail"
  fi
  biome_json="$(mktemp "${TMPDIR:-/tmp}/alt-biome-lint-XXXXXX.json")"
  set +e
  ./node_modules/.bin/biome check --error-on-warnings --reporter=json . > "$biome_json"
  lint_rc=$?
  set -e
  if [ "$lint_rc" -eq 0 ]; then
    lint_status="pass"
  elif [ "$fmt_rc" -ne 0 ]; then
    lint_status="fail"
  elif python3 "${script_dir}/check_ratchet.py" check-fingerprints \
    --repo-root "${repo_root}" \
    --project-dir "${project_dir}" \
    --biome-json "$biome_json" \
    --require-findings; then
    lint_rc=0
    lint_status="pass (block-new)"
  else
    lint_status="fail"
  fi
  rm -f "$biome_json"
else
  if bun run lint:check; then
    lint_status="pass"
  else
    lint_rc=$?
    lint_status="fail"
  fi
fi

check_rc=0
if bun run check; then
  check_status="pass"
else
  check_rc=$?
  check_status="fail"
fi

if [ "$lint_policy" = "block-new" ]; then
  echo "format: ${fmt_status} (exit ${fmt_rc})"
fi
echo "lint:check: ${lint_status} (exit ${lint_rc})"
echo "check: ${check_status} (exit ${check_rc})"

if [ "$fmt_rc" -ne 0 ] || [ "$lint_rc" -ne 0 ] || [ "$check_rc" -ne 0 ]; then
  exit 1
fi
