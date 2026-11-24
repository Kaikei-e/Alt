#!/usr/bin/env bash
set -euo pipefail

usage() {
  echo "usage: run.sh [--policy <fix-existing|block-new>] <target|--list|all>" >&2
}

policy="${LINT_POLICY:-fix-existing}"
while [ "$#" -gt 0 ]; do
  case "$1" in
    --policy)
      if [ "$#" -lt 2 ] || [ -z "$2" ]; then
        usage
        exit 2
      fi
      policy="$2"
      shift 2
      ;;
    --policy=*)
      policy="${1#--policy=}"
      shift
      ;;
    *)
      break
      ;;
  esac
done

if [ "$#" -ne 1 ] || [ -z "$1" ]; then
  usage
  exit 2
fi

case "$policy" in
  fix-existing|block-new)
    export LINT_POLICY="$policy"
    ;;
  *)
    echo "invalid lint policy '$policy': expected 'fix-existing' or 'block-new'" >&2
    exit 2
    ;;
esac

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

if [ -x "$repo_root/scripts/altharness/alt-harness" ]; then
  exec "$repo_root/scripts/altharness/alt-harness" lint "$@"
fi

check_disk_space() {
  local min_kb="${ALT_LINT_MIN_FREE_KB:-10485760}"
  local avail_kb=""
  if [ -n "${ALT_LINT_FAKE_FREE_KB:-}" ]; then
    avail_kb="$ALT_LINT_FAKE_FREE_KB"
  else
    avail_kb="$(df -Pk "$repo_root" | awk 'NR==2 {print $4}')"
  fi
  if ! [[ "$min_kb" =~ ^[0-9]+$ ]] || ! [[ "$avail_kb" =~ ^[0-9]+$ ]]; then
    echo "ERROR: failed to determine free disk space (available='${avail_kb}', required='${min_kb}')" >&2
    exit 1
  fi
  if [ "$avail_kb" -lt "$min_kb" ]; then
    local avail_mb=$((avail_kb / 1024))
    local req_mb=$((min_kb / 1024))
    echo "ERROR: ディスク空き容量不足 (available: ${avail_mb} MB, required: ${req_mb} MB)" >&2
    exit 1
  fi
}

detect_marker_kind() {
  local dir="$1"
  if [ -f "$dir/go.mod" ]; then
    echo "go"
  elif [ -f "$dir/Cargo.toml" ]; then
    echo "rust"
  elif [ -f "$dir/pyproject.toml" ]; then
    echo "python"
  elif [ -f "$dir/deno.json" ]; then
    echo "deno"
  elif [ -f "$dir/bun.lock" ]; then
    echo "frontend"
  elif [ -f "$dir/buf.yaml" ]; then
    echo "proto"
  fi
}

is_reserved_script() {
  case "$1" in
    run|all|go|python|rust|deno|frontend|check-link-vs-url-naming)
      return 0
      ;;
    *)
      return 1
      ;;
  esac
}

is_generated_go_stub_module() {
  local rel="$1"
  local dir="$repo_root/$rel"
  case "$rel" in
    alt-butterfly-facade/internal/gen/proto|rag-orchestrator/internal/gen/proto)
      return 0
      ;;
  esac
  local any_go=""
  any_go="$(find "$dir" -type f -name '*.go' -print -quit)"
  [ -n "$any_go" ] || return 1
  local non_gen=""
  non_gen="$(find "$dir" -type f -name '*.go' ! -name '*.pb.go' ! -name '*.connect.go' -print -quit)"
  [ -z "$non_gen" ]
}

collect_marker_files() {
  if [ -e "$repo_root/.git" ] && command -v git >/dev/null 2>&1 && git -C "$repo_root" rev-parse --is-inside-work-tree >/dev/null 2>&1; then
    git -C "$repo_root" ls-files --cached --others --exclude-standard \
      | grep -E '(^|/)(go\.mod|Cargo\.toml|pyproject\.toml|deno\.json|bun\.lock|buf\.yaml)$' || true
  else
    find "$repo_root" \
      \( -name .git -o -name node_modules -o -name .venv -o -name target -o -name __pycache__ -o -name .cache -o -name vendor -o -name _reports \) -prune \
      -o -type f \( -name go.mod -o -name Cargo.toml -o -name pyproject.toml -o -name deno.json -o -name bun.lock -o -name buf.yaml \) -print
  fi
}

list_targets() {
  local -a raw_targets=()
  local marker_path="" marker_rel="" rel_dir="" marker_kind=""

  while IFS= read -r marker_path; do
    [ -n "$marker_path" ] || continue
    marker_rel="${marker_path#"$repo_root"/}"
    [ -f "$repo_root/$marker_rel" ] || continue
    rel_dir="$(dirname "$marker_rel")"
    [ "$rel_dir" != "." ] || continue
    case "$rel_dir" in
      stopped-using-k8s|stopped-using-k8s/*|scripts/harness/corpus|scripts/harness/corpus/*|scripts/altharness|scripts/altharness/*)
        continue
        ;;
    esac
    marker_kind="$(detect_marker_kind "$repo_root/$rel_dir")"
    [ -n "$marker_kind" ] || continue
    if [ "$marker_kind" = "go" ] && is_generated_go_stub_module "$rel_dir"; then
      continue
    fi
    if [[ "$rel_dir" != */* ]] && [ -f "$repo_root/scripts/lint/$rel_dir.sh" ] && [ "$marker_kind" != "$rel_dir" ]; then
      continue
    fi
    raw_targets+=("$rel_dir")
  done < <(collect_marker_files)

  local sh_file="" name=""
  for sh_file in "$repo_root"/scripts/lint/*.sh; do
    [ -f "$sh_file" ] || continue
    name="$(basename "$sh_file" .sh)"
    if is_reserved_script "$name"; then
      continue
    fi
    if [ -d "$repo_root/$name" ]; then
      marker_kind="$(detect_marker_kind "$repo_root/$name")"
      if [ -n "$marker_kind" ] && [ "$marker_kind" != "$name" ]; then
        continue
      fi
    fi
    raw_targets+=("$name")
  done

  if [ "${#raw_targets[@]}" -eq 0 ]; then
    return 0
  fi
  printf '%s\n' "${raw_targets[@]}" | LC_ALL=C sort -u
}

run_all() {
  local -a targets=()
  local -a statuses=()
  mapfile -t targets < <(list_targets)
  if [ "${#targets[@]}" -eq 0 ]; then
    echo "no lint targets found" >&2
    return 1
  fi

  local overall_rc=0 pass_count=0 fail_count=0 t="" rc=0
  for t in "${targets[@]}"; do
    echo "==> [all] running: $t"
    rc=0
    set +e
    bash "$repo_root/scripts/lint/run.sh" "$t"
    rc=$?
    set -e
    if [ "$rc" -eq 0 ]; then
      statuses+=("PASS")
      pass_count=$((pass_count + 1))
    else
      statuses+=("FAIL")
      fail_count=$((fail_count + 1))
      overall_rc=1
    fi
  done

  echo ""
  echo "==> [all] summary (${pass_count} passed, ${fail_count} failed, ${#targets[@]} total)"
  local i=0
  for i in "${!targets[@]}"; do
    printf '  %-4s  %s\n' "${statuses[$i]}" "${targets[$i]}"
  done
  return "$overall_rc"
}

if [ "$1" = "--list" ]; then
  list_targets
  exit 0
fi

check_disk_space

if [ "$1" = "all" ]; then
  run_all
  exit $?
fi

target="${1#./}"
target="${target%/}"

case "$target" in
  ""|/*|..|../*|*/..|*/../*)
    echo "invalid lint target: $1" >&2
    exit 2
    ;;
  run|go|python|rust|deno|frontend|check-link-vs-url-naming)
    echo "invalid lint target '$target': must be a project directory or named target (adr, apparmor, compose, e2e, harness, observability, proto, registry, repo)" >&2
    exit 2
    ;;
esac

kind=""
if [[ "$target" != */* ]] && [ -f "$repo_root/scripts/lint/$target.sh" ]; then
  if [ -d "$repo_root/$target" ]; then
    marker_kind="$(detect_marker_kind "$repo_root/$target")"
    if [ -n "$marker_kind" ] && [ "$marker_kind" != "$target" ]; then
      echo "named lint target '$target' shadows directory $target/ ($marker_kind)" >&2
      exit 2
    fi
  fi
  kind="$target"
else
  dir="$repo_root/$target"
  if [ ! -d "$dir" ]; then
    echo "lint target directory not found: $target" >&2
    exit 2
  fi
  kind="$(detect_marker_kind "$dir")"
  if [ -z "$kind" ]; then
    echo "no recognized lint marker in $target" >&2
    exit 2
  fi
fi

script="$repo_root/scripts/lint/$kind.sh"
if [ ! -f "$script" ]; then
  echo "missing lint script: scripts/lint/$kind.sh" >&2
  exit 2
fi

cd "$repo_root"
exec bash "$script" "$target"
