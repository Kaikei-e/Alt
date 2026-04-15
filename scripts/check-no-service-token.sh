#!/usr/bin/env bash
# Post-cutover guard: fail if any X-Service-Token / SERVICE_SECRET /
# ServiceAuthMiddleware references remain in source.
#
# After the mTLS hard-cutover lands, wire this into CI (proto-contract.yaml
# or similar) so reintroduction of the shared-secret layer fails the build.
#
# Before the cutover this script is expected to FAIL — it surfaces every
# residue that must be deleted when flipping the switch.
#
# Usage:
#   ./scripts/check-no-service-token.sh           # report only
#   ./scripts/check-no-service-token.sh --strict  # exit 1 on any finding
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$REPO_ROOT"

STRICT="false"
if [[ "${1:-}" == "--strict" ]]; then
  STRICT="true"
fi

# Allowlist: docs that describe the legacy mechanism, CDC test suites that
# pin the mechanism during the migration window, and postmortems are
# expected to mention it. Everything else in production source must be
# free of the references post-cutover.
ALLOWED_PATHS=(
  "docs/"
  "CLAUDE.md"
  "scripts/check-no-service-token.sh"
  ".claude/"
  # Test files retain `assert.Empty(...)` regression guards that ensure the
  # header is NEVER sent. Allowlisting *_test.go / tests/ keeps those guards
  # in source while the strict mode focuses on production code paths.
  "_test.go"
  "/tests/"
  "test_unit.py"
  # pact JSON files (historical record)
  "pacts/"
  ".json"
  # Archived prototype configs
  "x-prototype/"
  # Project READMEs (historical references)
  "README.md"
)

build_exclude_args() {
  local out=()
  for p in "${ALLOWED_PATHS[@]}"; do
    # Path containing a slash is treated as a path prefix; otherwise as
    # a filename suffix glob.
    if [[ "$p" == */* ]]; then
      out+=(--glob "!${p}")
      out+=(--glob "!${p}**")
    else
      out+=(--glob "!**${p}")
      out+=(--glob "!**${p}/**")
    fi
  done
  printf '%s\n' "${out[@]}"
}

mapfile -t EXCLUDES < <(build_exclude_args)

# Patterns that constitute the shared-secret legacy layer.
PATTERNS=(
  'X-Service-Token'
  'SERVICE_SECRET'
  'SERVICE_TOKEN'
  'service_secret\b'
  'service_token\b'
  'resolve_service_secret'
  'ServiceAuthMiddleware'
  'service_auth_middleware'
)

TOTAL=0
for pat in "${PATTERNS[@]}"; do
  count=$(rg -c "$pat" "${EXCLUDES[@]}" 2>/dev/null | awk -F: '{s+=$2} END{print s+0}')
  if [[ "${count:-0}" -gt 0 ]]; then
    echo "[found $count] $pat"
    TOTAL=$((TOTAL + count))
  fi
done

echo ""
if [[ "$TOTAL" -eq 0 ]]; then
  echo "PASS: no X-Service-Token / SERVICE_SECRET residues outside the allowlist."
  exit 0
fi

echo "FOUND ${TOTAL} residue references outside the allowlist."
echo ""
echo "This is expected before the mTLS hard-cutover (documented in"
echo "docs/runbooks/mtls-cutover.md). After the cutover, this script must"
echo "exit 0 and be wired into CI as a required status check."

if [[ "$STRICT" == "true" ]]; then
  exit 1
fi
exit 0
