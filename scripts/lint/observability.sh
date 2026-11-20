#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$repo_root"

PROM_VERSION="3.1.0"
AM_VERSION="0.28.1"

case "$(uname -s)" in
  Linux) os="linux" ;;
  Darwin) os="darwin" ;;
  *) echo "unsupported OS: $(uname -s)" >&2; exit 1 ;;
esac

case "$(uname -m)" in
  x86_64|amd64) arch="amd64" ;;
  aarch64|arm64) arch="arm64" ;;
  *) echo "unsupported arch: $(uname -m)" >&2; exit 1 ;;
esac

verify_sha256() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum -c -
  else
    shasum -a 256 -c -
  fi
}

ensure_tool() {
  local tool="$1" repo="$2" version="$3" dest_dir="$4"
  if [ -x "$dest_dir/$tool" ]; then
    return 0
  fi
  local tmp_dir=""
  tmp_dir="$(mktemp -d)"
  trap 'rm -rf "$tmp_dir"' RETURN

  local tarball="${tool%tool}-${version}.${os}-${arch}.tar.gz"
  if [ "$tool" = "promtool" ]; then
    tarball="prometheus-${version}.${os}-${arch}.tar.gz"
  else
    tarball="alertmanager-${version}.${os}-${arch}.tar.gz"
  fi
  local base_url="https://github.com/${repo}/releases/download/v${version}"

  curl -fsSL --proto '=https' --tlsv1.2 --retry 3 -o "$tmp_dir/$tarball" "$base_url/$tarball"
  curl -fsSL --proto '=https' --tlsv1.2 --retry 3 -o "$tmp_dir/sha256sums.txt" "$base_url/sha256sums.txt"
  (cd "$tmp_dir" && grep "  ${tarball}$" sha256sums.txt | verify_sha256)

  mkdir -p "$dest_dir"
  local prefix="${tarball%.tar.gz}"
  tar -xzf "$tmp_dir/$tarball" --strip-components=1 -C "$dest_dir" "${prefix}/${tool}"
  chmod +x "$dest_dir/$tool"
}

cache_root="${XDG_CACHE_HOME:-$HOME/.cache}/alt-observability-tools"
prom_dir="$cache_root/prometheus-${PROM_VERSION}-${os}-${arch}"
am_dir="$cache_root/alertmanager-${AM_VERSION}-${os}-${arch}"

ensure_tool "promtool" "prometheus/prometheus" "$PROM_VERSION" "$prom_dir"
ensure_tool "amtool" "prometheus/alertmanager" "$AM_VERSION" "$am_dir"

fail=0
summary=()

run_step() {
  local label="$1"
  shift
  local rc=0
  set +e
  "$@"
  rc=$?
  set -e
  if [ "$rc" -eq 0 ]; then
    summary+=("PASS: $label")
  else
    summary+=("FAIL: $label")
    fail=1
  fi
}

run_step "compose-tool-images" python3 scripts/observability-compose-tool-images.py --compose compose/observability.yaml
run_step "observability-validate" env PROMTOOL="$prom_dir/promtool" AMTOOL="$am_dir/amtool" bash scripts/observability-validate.sh

printf '%s\n' "${summary[@]}"
exit "$fail"
