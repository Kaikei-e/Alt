#!/usr/bin/env bash
set -euo pipefail

if [ "$#" -ne 1 ] || [ -z "$1" ]; then
  echo "usage: run.sh <target>" >&2
  exit 2
fi

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
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
