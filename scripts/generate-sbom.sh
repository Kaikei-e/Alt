#!/usr/bin/env bash
# generate-sbom.sh — Generate CycloneDX SBOM for selected Go services (M-001).
# OWASP Top 10:2025 elevates SBOM generation as a baseline requirement; this
# script gives developers a one-shot local equivalent of what we want CI to
# eventually emit alongside container images.
#
# Usage:
#   scripts/generate-sbom.sh                       # SBOM for alt-backend (default)
#   scripts/generate-sbom.sh alt-backend pre-processor auth-hub
#
# Tools used:
#   - syft       (Anchore) — preferred, multi-format CycloneDX/SPDX
#   - cyclonedx-gomod — fallback when syft is unavailable
#
# The generated artefacts are written to ./sbom/<service>-<commit>.cdx.json.

set -euo pipefail

SERVICES=("$@")
if [ "${#SERVICES[@]}" -eq 0 ]; then
  SERVICES=("alt-backend")
fi

OUT_DIR="$(git rev-parse --show-toplevel)/sbom"
mkdir -p "$OUT_DIR"

COMMIT="$(git rev-parse --short HEAD)"

have() { command -v "$1" >/dev/null 2>&1; }

if have syft; then
  TOOL=syft
elif have cyclonedx-gomod; then
  TOOL=cyclonedx
else
  echo "error: neither 'syft' nor 'cyclonedx-gomod' is installed" >&2
  echo "  install syft:           https://github.com/anchore/syft" >&2
  echo "  install cyclonedx-gomod: go install github.com/CycloneDX/cyclonedx-gomod/cmd/cyclonedx-gomod@latest" >&2
  exit 1
fi

for service in "${SERVICES[@]}"; do
  src="$service/app"
  if [ ! -d "$src" ]; then
    src="$service"
  fi
  if [ ! -f "$src/go.mod" ]; then
    echo "skip: $service (no go.mod under $src)" >&2
    continue
  fi
  out="$OUT_DIR/${service}-${COMMIT}.cdx.json"
  echo "==> $service ($TOOL) -> $out"
  case "$TOOL" in
    syft)
      syft "dir:$src" -o cyclonedx-json="$out"
      ;;
    cyclonedx)
      ( cd "$src" && cyclonedx-gomod mod -licenses -json -output "$out" )
      ;;
  esac
done

echo "done. SBOMs written to $OUT_DIR"
