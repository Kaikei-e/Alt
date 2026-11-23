#!/usr/bin/env bash
# Host-side final verification wrapper combining worktree hygiene, targeted & repo lint,
# optional test command, LOOP-01 verify-handoff, and clean git status check.
set -euo pipefail

usage() {
  cat >&2 <<'EOF'
Usage: host-final-check.sh --target <lint-target> [options]

Options:
  --target <target>     Lint target for ./scripts/lint/run.sh <target> (repeatable)
  --repo-root <dir>     Repository root directory (default: auto-detected)
  --test-cmd <cmd>      Optional host test command to execute synchronously
  --handoff <path>      Optional TaskHandoff file to verify via LOOP-01 verify-handoff
  --packet <path>       TaskPacket file (required when --handoff is specified)
  --base-commit <rev>   Base git commit passed to verify-handoff
  --runs-dir <dir>      Runs directory for verify-handoff evidence JSON
  --no-rerun            Pass --no-rerun to verify-handoff
  --skip-repo-lint      Skip ./scripts/lint/run.sh repo (e.g. for fast unit tests)
  --allow-dirty         Do not fail on non-empty git status --porcelain
  -h, --help            Show this help message
EOF
}

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
default_repo_root="$(cd -- "${script_dir}/../.." && pwd -P)"
repo_root="$default_repo_root"
targets=()
test_cmd=""
handoff_path=""
packet_path=""
base_commit=""
runs_dir=""
no_rerun=0
skip_repo_lint=0
allow_dirty=0

while [ "$#" -gt 0 ]; do
  case "$1" in
    --target)
      [ "$#" -ge 2 ] || { echo "error: --target requires a value" >&2; usage; exit 2; }
      targets+=("$2")
      shift 2
      ;;
    --repo-root)
      [ "$#" -ge 2 ] || { echo "error: --repo-root requires a value" >&2; usage; exit 2; }
      repo_root="$2"
      shift 2
      ;;
    --test-cmd)
      [ "$#" -ge 2 ] || { echo "error: --test-cmd requires a value" >&2; usage; exit 2; }
      test_cmd="$2"
      shift 2
      ;;
    --handoff)
      [ "$#" -ge 2 ] || { echo "error: --handoff requires a value" >&2; usage; exit 2; }
      handoff_path="$2"
      shift 2
      ;;
    --packet)
      [ "$#" -ge 2 ] || { echo "error: --packet requires a value" >&2; usage; exit 2; }
      packet_path="$2"
      shift 2
      ;;
    --base-commit)
      [ "$#" -ge 2 ] || { echo "error: --base-commit requires a value" >&2; usage; exit 2; }
      base_commit="$2"
      shift 2
      ;;
    --runs-dir)
      [ "$#" -ge 2 ] || { echo "error: --runs-dir requires a value" >&2; usage; exit 2; }
      runs_dir="$2"
      shift 2
      ;;
    --no-rerun)
      no_rerun=1
      shift
      ;;
    --skip-repo-lint)
      skip_repo_lint=1
      shift
      ;;
    --allow-dirty)
      allow_dirty=1
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    -*)
      echo "error: unknown option: $1" >&2
      usage
      exit 2
      ;;
    *)
      targets+=("$1")
      shift
      ;;
  esac
done

if [ "${#targets[@]}" -eq 0 ]; then
  echo "error: at least one --target is required" >&2
  usage
  exit 2
fi

for target in "${targets[@]}"; do
  if [ -z "$target" ] || [[ "$target" == -* ]] || [ "$target" = "all" ]; then
    echo "error: invalid --target '${target}': must be a concrete lint target (not flag '-*' or 'all')" >&2
    exit 2
  fi
done

if [ ! -d "$repo_root" ]; then
  echo "error: repo root not found: $repo_root" >&2
  exit 2
fi
repo_root="$(cd -- "$repo_root" && pwd -P)"

if [ -n "$handoff_path" ] && [ -z "$packet_path" ]; then
  echo "error: --packet is required when --handoff is provided" >&2
  exit 2
fi

hygiene_script="${repo_root}/scripts/lint/check_worktree_hygiene.py"
if [ ! -f "$hygiene_script" ]; then
  hygiene_script="${default_repo_root}/scripts/lint/check_worktree_hygiene.py"
fi

task_packet_script="${repo_root}/scripts/agent-harness/task_packet.py"
if [ ! -f "$task_packet_script" ]; then
  task_packet_script="${default_repo_root}/scripts/agent-harness/task_packet.py"
fi

run_sh="${repo_root}/scripts/lint/run.sh"
if [ ! -f "$run_sh" ]; then
  echo "error: missing lint runner: $run_sh" >&2
  exit 2
fi

if [ -n "$test_cmd" ]; then
  if ! python3 -c '
import pathlib, sys
sys.path.insert(0, str(pathlib.Path(sys.argv[1]).parent))
import task_packet as mod
cmd = sys.argv[2]
if mod._has_background_ampersand(cmd):
    sys.stderr.write("error: --test-cmd must run in foreground only (no & or nohup)\n")
    sys.exit(2)
bad = mod._match_forbidden_autofix(cmd)
if bad is not None:
    sys.stderr.write(f"error: --test-cmd contains forbidden GUARD-01 autofix ({bad})\n")
    sys.exit(2)
' "$task_packet_script" "$test_cmd"; then
    exit 2
  fi
fi

packet_policy=""
if [ -n "$packet_path" ]; then
  if [ ! -f "$packet_path" ]; then
    echo "error: packet file not found: $packet_path" >&2
    exit 2
  fi
  packet_policy="$(python3 -c '
import pathlib, sys
sys.path.insert(0, str(pathlib.Path(sys.argv[1]).parent))
import task_packet as mod
doc = mod.load_document(pathlib.Path(sys.argv[2]))
ep = doc.data.get("execution_policy")
pol = ep.get("policy", "") if isinstance(ep, dict) else ""
if pol in ("fix-existing", "block-new"):
    sys.stdout.write(pol)
' "$task_packet_script" "$packet_path" 2>/dev/null || true)"
fi

echo "==> [host-final-check] 1/5 worktree hygiene"
python3 "$hygiene_script" --repo-root "$repo_root"

echo "==> [host-final-check] 2/5 targeted lint"
for target in "${targets[@]}"; do
  if [ -n "$packet_policy" ]; then
    echo "--> ./scripts/lint/run.sh --policy ${packet_policy} ${target}"
    (cd -- "$repo_root" && bash "$run_sh" --policy "$packet_policy" "$target")
  else
    echo "--> ./scripts/lint/run.sh ${target}"
    (cd -- "$repo_root" && bash "$run_sh" "$target")
  fi
done

if [ "$skip_repo_lint" -eq 0 ]; then
  echo "==> [host-final-check] 3/5 repo lint"
  (cd -- "$repo_root" && bash "$run_sh" repo)
else
  echo "==> [host-final-check] 3/5 repo lint (skipped via --skip-repo-lint)"
fi

if [ -n "$test_cmd" ]; then
  echo "==> [host-final-check] 4/5 host test command"
  (cd -- "$repo_root" && bash -c "$test_cmd")
else
  echo "==> [host-final-check] 4/5 host test command (not specified)"
fi

if [ -n "$handoff_path" ]; then
  echo "==> [host-final-check] 5/5 LOOP-01 verify-handoff"
  verify_args=(
    "$task_packet_script"
    verify-handoff
    "$handoff_path"
    --packet "$packet_path"
    --repo-root "$repo_root"
  )
  if [ -n "$base_commit" ]; then
    verify_args+=(--base-commit "$base_commit")
  fi
  if [ -n "$runs_dir" ]; then
    verify_args+=(--runs-dir "$runs_dir")
  fi
  if [ "$no_rerun" -eq 1 ]; then
    verify_args+=(--no-rerun)
  fi
  python3 "${verify_args[@]}"
else
  echo "==> [host-final-check] 5/5 LOOP-01 verify-handoff (not specified)"
fi

if [ "$allow_dirty" -eq 0 ]; then
  echo "==> [host-final-check] final git status --porcelain check"
  dirty_status="$(git -C "$repo_root" status --porcelain -- ':(exclude).harness')"
  if [ -n "$dirty_status" ]; then
    echo "ERROR: dirty worktree detected after final checks:" >&2
    printf '%s\n' "$dirty_status" >&2
    exit 1
  fi
fi

echo "OK: host-final-check passed (${targets[*]})"
