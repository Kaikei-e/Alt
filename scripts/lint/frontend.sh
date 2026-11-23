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

if ! command -v python3 >/dev/null 2>&1; then
  echo "error: required command not found in PATH: python3" >&2
  exit 1
fi

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

bun_version_of() {
  local bin="$1"
  local out
  if ! out="$("$bin" --version 2>/dev/null)"; then
    return 1
  fi
  out="$(printf '%s' "$out" | tr -d '[:space:]')"
  [ -n "$out" ] || return 1
  printf '%s\n' "$out"
}

bun_bin=""
if [ -n "${ALT_LINT_BUN_BIN:-}" ]; then
  if [ ! -x "$ALT_LINT_BUN_BIN" ]; then
    echo "error: ALT_LINT_BUN_BIN is not executable: ${ALT_LINT_BUN_BIN}" >&2
    exit 1
  fi
  override_ver="$(bun_version_of "$ALT_LINT_BUN_BIN" || true)"
  if [ "$override_ver" != "$expected_bun_version" ]; then
    echo "error: ALT_LINT_BUN_BIN (${ALT_LINT_BUN_BIN}) bun version mismatch: expected ${expected_bun_version}, got '${override_ver:-unknown}'" >&2
    exit 1
  fi
  bun_bin="$ALT_LINT_BUN_BIN"
else
  if path_bun="$(command -v bun 2>/dev/null || true)" && [ -n "$path_bun" ]; then
    path_ver="$(bun_version_of "$path_bun" || true)"
    if [ "$path_ver" = "$expected_bun_version" ]; then
      bun_bin="$path_bun"
    fi
  fi

  if [ -z "$bun_bin" ]; then
    cache_dir="${XDG_CACHE_HOME:-${HOME}/.cache}/alt-lint/bun/${expected_bun_version}"
    cached_bun="${cache_dir}/bun"
    if [ -x "$cached_bun" ] && cached_ver="$(bun_version_of "$cached_bun" || true)" && [ "$cached_ver" = "$expected_bun_version" ]; then
      bun_bin="$cached_bun"
    else
      os_name="$(uname -s)"
      arch_name="$(uname -m)"
      case "${os_name}:${arch_name}" in
        Linux:x86_64) target_zip="bun-linux-x64" ;;
        Linux:aarch64|Linux:arm64) target_zip="bun-linux-aarch64" ;;
        Darwin:arm64|Darwin:aarch64) target_zip="bun-darwin-aarch64" ;;
        Darwin:x86_64) target_zip="bun-darwin-x64" ;;
        *)
          echo "error: unsupported platform for automatic Bun ${expected_bun_version} install: ${os_name}/${arch_name}" >&2
          exit 1
          ;;
      esac

      if ! command -v curl >/dev/null 2>&1; then
        echo "error: required command not found in PATH for Bun ${expected_bun_version} download: curl" >&2
        exit 1
      fi

      download_base_url="${ALT_LINT_BUN_DOWNLOAD_BASE_URL:-https://github.com/oven-sh/bun/releases/download}"
      download_url="${download_base_url%/}/bun-v${expected_bun_version}/${target_zip}.zip"
      tmp_dir="$(mktemp -d "${TMPDIR:-/tmp}/alt-bun-install-XXXXXX")"
      zip_path="${tmp_dir}/${target_zip}.zip"
      extract_dir="${tmp_dir}/extract"
      mkdir -p "$extract_dir"

      echo "==> [${project_rel}] installing pinned bun ${expected_bun_version} into ${cache_dir}" >&2
      if ! curl -fsSL --retry 3 "$download_url" -o "$zip_path"; then
        rm -rf "$tmp_dir"
        echo "error: failed to download bun ${expected_bun_version} from ${download_url}" >&2
        exit 1
      fi

      if ! python3 -m zipfile -e "$zip_path" "$extract_dir"; then
        rm -rf "$tmp_dir"
        echo "error: failed to extract bun archive downloaded from ${download_url}" >&2
        exit 1
      fi

      extracted_bun="${extract_dir}/${target_zip}/bun"
      if [ ! -f "$extracted_bun" ]; then
        extracted_bun="$(find "$extract_dir" -maxdepth 2 -type f -name bun | head -n 1 || true)"
      fi
      if [ -z "$extracted_bun" ] || [ ! -f "$extracted_bun" ]; then
        rm -rf "$tmp_dir"
        echo "error: bun binary not found in extracted archive ${download_url}" >&2
        exit 1
      fi

      mkdir -p "$cache_dir"
      tmp_cached="${cache_dir}/bun.tmp.$$"
      rm -f "$tmp_cached"
      cp "$extracted_bun" "$tmp_cached"
      chmod +x "$tmp_cached"
      downloaded_ver="$(bun_version_of "$tmp_cached" || true)"
      if [ "$downloaded_ver" != "$expected_bun_version" ]; then
        rm -f "$tmp_cached"
        rm -rf "$tmp_dir"
        echo "error: downloaded bun version mismatch: expected ${expected_bun_version}, got '${downloaded_ver:-unknown}'" >&2
        exit 1
      fi
      mv -f "$tmp_cached" "$cached_bun"
      rm -rf "$tmp_dir"
      bun_bin="$cached_bun"
    fi
  fi
fi

actual_bun_version="$(bun_version_of "$bun_bin" || true)"
if [ "$actual_bun_version" != "$expected_bun_version" ]; then
  echo "error: bun version mismatch: expected ${expected_bun_version}, got '${actual_bun_version:-unknown}'" >&2
  exit 1
fi

bun_bin_dir="$(cd "$(dirname "$bun_bin")" && pwd)"
bun_bin="${bun_bin_dir}/$(basename "$bun_bin")"
export PATH="${bun_bin_dir}:${PATH}"

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
  if "$bun_bin" run lint:check; then
    lint_status="pass"
  else
    lint_rc=$?
    lint_status="fail"
  fi
fi

check_rc=0
if "$bun_bin" run check; then
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
