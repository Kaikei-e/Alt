#!/usr/bin/env bash
set -euo pipefail

readonly GOLANGCI_LINT_VERSION="v2.14.0"
readonly GOSEC_VERSION="v2.30.0"
readonly GOVULNCHECK_VERSION="v1.6.0"

readonly GOLANGCI_LINT_MODULE="github.com/golangci/golangci-lint/v2/cmd/golangci-lint"
readonly GOSEC_MODULE="github.com/securego/gosec/v2/cmd/gosec"
readonly GOVULNCHECK_MODULE="golang.org/x/vuln/cmd/govulncheck"

if [ "$#" -ne 1 ] || [ -z "$1" ] || [[ "$1" = /* ]]; then
  echo "Usage: $0 <project-dir-relative-to-repo-root>" >&2
  exit 2
fi

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
PROJECT_REL="${1#./}"
PROJECT_REL="${PROJECT_REL%/}"
PROJECT_DIR="$(cd "${REPO_ROOT}/${PROJECT_REL:-.}" && pwd)"

if [ ! -f "${PROJECT_DIR}/go.mod" ]; then
  echo "ERROR: go.mod not found in ${PROJECT_DIR}" >&2
  exit 1
fi

CONFIG_FILE="${REPO_ROOT}/.golangci.yml"
if [ ! -f "${CONFIG_FILE}" ]; then
  echo "ERROR: Missing golangci-lint config: ${CONFIG_FILE}" >&2
  exit 1
fi

for cmd in go jq python3; do
  if ! command -v "${cmd}" >/dev/null 2>&1; then
    echo "ERROR: Required command not found in PATH: ${cmd}" >&2
    exit 1
  fi
done

GO_TOOLCHAIN_VERSION="$(go env GOVERSION)"
if [ -z "${GO_TOOLCHAIN_VERSION}" ]; then
  echo "ERROR: Could not determine Go toolchain version via 'go env GOVERSION'" >&2
  exit 1
fi

CACHE_BASE="${XDG_CACHE_HOME:-${HOME:?HOME must be set}/.cache}/alt-lint/${GO_TOOLCHAIN_VERSION}"

ensure_tool() {
  local name="$1" module="$2" version="$3"
  local bin_dir="${CACHE_BASE}/${name}-${version}"
  if [ ! -x "${bin_dir}/${name}" ]; then
    mkdir -p "${bin_dir}"
    echo "Installing ${name}@${version} into ${bin_dir} (${GO_TOOLCHAIN_VERSION})..." >&2
    (cd "${REPO_ROOT}" && GOBIN="${bin_dir}" go install "${module}@${version}")
  fi
}

ensure_tool "golangci-lint" "${GOLANGCI_LINT_MODULE}" "${GOLANGCI_LINT_VERSION}"
ensure_tool "gosec" "${GOSEC_MODULE}" "${GOSEC_VERSION}"
ensure_tool "govulncheck" "${GOVULNCHECK_MODULE}" "${GOVULNCHECK_VERSION}"

GOLANGCI_LINT_BIN="${CACHE_BASE}/golangci-lint-${GOLANGCI_LINT_VERSION}/golangci-lint"
GOSEC_BIN="${CACHE_BASE}/gosec-${GOSEC_VERSION}/gosec"
GOVULNCHECK_BIN="${CACHE_BASE}/govulncheck-${GOVULNCHECK_VERSION}/govulncheck"

cd "${PROJECT_DIR}"
export GOFLAGS="-buildvcs=false"

overall_rc=0
status_canary="PASS"
status_gofmt="PASS"
status_govet="PASS"
status_golangci="PASS"
status_gosec="PASS"
status_govulncheck="PASS"

CANARY_AUDIT_SCRIPT="${SCRIPT_DIR}/canary_audit.py"
if [ ! -f "${CANARY_AUDIT_SCRIPT}" ]; then
  echo "ERROR: Missing canary audit helper: ${CANARY_AUDIT_SCRIPT}" >&2
  exit 1
fi

canary_dir="$(mktemp -d "${TMPDIR:-/tmp}/alt-go-canary-XXXXXX")"
cleanup_canary() {
  rm -rf "${canary_dir}"
}
trap cleanup_canary EXIT

echo "==> [${PROJECT_REL}] canary audit (golangci-lint, gosec)"
if ! python3 "${CANARY_AUDIT_SCRIPT}" go-canary \
  --project-dir "${PROJECT_DIR}" \
  --config "${CONFIG_FILE}" \
  --golangci-lint-bin "${GOLANGCI_LINT_BIN}" \
  --gosec-bin "${GOSEC_BIN}" \
  --work-dir "${canary_dir}"; then
  exit 1
fi
rm -rf "${canary_dir}"

echo "==> [${PROJECT_REL}] gofmt"
gofmt_out="$(gofmt -l .)"
unformatted=""
if [ -n "${gofmt_out}" ]; then
  unformatted="$(printf '%s\n' "${gofmt_out}" | grep -v '^gen/' || true)"
fi
if [ -n "${unformatted}" ]; then
  echo "ERROR: Run gofmt -w on these files:" >&2
  printf '%s\n' "${unformatted}" >&2
  status_gofmt="FAIL"
  overall_rc=1
fi

echo "==> [${PROJECT_REL}] go vet"
if ! go vet ./...; then
  status_govet="FAIL"
  overall_rc=1
fi

echo "==> [${PROJECT_REL}] golangci-lint"
if "${GOLANGCI_LINT_BIN}" run --config "${CONFIG_FILE}"; then
  golangci_rc=0
else
  golangci_rc=$?
fi
if [ "${golangci_rc}" -eq 1 ]; then
  status_golangci="FAIL"
  overall_rc=1
elif [ "${golangci_rc}" -ne 0 ]; then
  echo "ERROR: golangci-lint failed to run (exit code ${golangci_rc})" >&2
  exit "${golangci_rc}"
fi

mkdir -p ./_reports
rm -f ./_reports/gosec.sarif ./_reports/gosec.sarif.tmp ./_reports/gosec.out \
      ./_reports/govulncheck.sarif ./_reports/govulncheck.sarif.tmp ./_reports/govulncheck.stderr

echo "==> [${PROJECT_REL}] gosec (SARIF)"
if "${GOSEC_BIN}" -severity high -confidence high -exclude-dir='(^|/)gen(/|$)' -fmt sarif -out ./_reports/gosec.sarif -stdout -verbose text ./... > ./_reports/gosec.out 2>&1; then
  gosec_rc=0
else
  gosec_rc=$?
fi
if [ ! -s ./_reports/gosec.sarif ]; then
  [ -s ./_reports/gosec.out ] && cat ./_reports/gosec.out >&2
  rm -f ./_reports/gosec.out
  echo "ERROR: gosec did not produce ./_reports/gosec.sarif (exit code ${gosec_rc})" >&2
  exit 1
fi
jq '(.runs[]?.results[]?) |= del(.fixes)' ./_reports/gosec.sarif > ./_reports/gosec.sarif.tmp
mv ./_reports/gosec.sarif.tmp ./_reports/gosec.sarif

if ! python3 "${CANARY_AUDIT_SCRIPT}" go-verify-scan \
  --project-dir "${PROJECT_DIR}" \
  --gosec-out ./_reports/gosec.out \
  --gosec-sarif ./_reports/gosec.sarif; then
  [ -s ./_reports/gosec.out ] && cat ./_reports/gosec.out >&2
  rm -f ./_reports/gosec.out
  exit 1
fi

gosec_issue_count="$(jq '[.runs[]?.results[]?] | length' ./_reports/gosec.sarif)"
if [ "${gosec_issue_count}" -gt 0 ]; then
  jq -r '.runs[]?.results[]? | "ERROR: gosec \(.ruleId) \(.locations[0].physicalLocation.artifactLocation.uri // "unknown"):\(.locations[0].physicalLocation.region.startLine // 1): \(.message.text)"' ./_reports/gosec.sarif >&2
  status_gosec="FAIL"
  overall_rc=1
elif [ "${gosec_rc}" -ne 0 ]; then
  [ -s ./_reports/gosec.out ] && cat ./_reports/gosec.out >&2
  rm -f ./_reports/gosec.out
  echo "ERROR: gosec exited with code ${gosec_rc} despite zero SARIF findings" >&2
  exit 1
fi
rm -f ./_reports/gosec.out

echo "==> [${PROJECT_REL}] govulncheck (SARIF)"
if ! "${GOVULNCHECK_BIN}" -format=sarif ./... > ./_reports/govulncheck.sarif 2>./_reports/govulncheck.stderr; then
  [ -s ./_reports/govulncheck.stderr ] && cat ./_reports/govulncheck.stderr >&2
  rm -f ./_reports/govulncheck.stderr
  echo "ERROR: govulncheck execution failed" >&2
  exit 1
fi
[ -s ./_reports/govulncheck.stderr ] && cat ./_reports/govulncheck.stderr >&2
rm -f ./_reports/govulncheck.stderr

if [ ! -s ./_reports/govulncheck.sarif ]; then
  echo "ERROR: govulncheck did not produce a non-empty ./_reports/govulncheck.sarif" >&2
  exit 1
fi
jq '(.runs[]?.results[]?.stacks | select(type == "array")) |= unique' ./_reports/govulncheck.sarif > ./_reports/govulncheck.sarif.tmp
mv ./_reports/govulncheck.sarif.tmp ./_reports/govulncheck.sarif

jq -r '
  .runs[]? as $run |
  ($run.tool.driver.rules // [] | map({(.id): .}) | add // {}) as $rules |
  $run.results[]? |
  ($rules[.ruleId] // {}) as $rule |
  (
    [
      .stacks[]?.frames[-1]?.module,
      .codeFlows[]?.threadFlows[]?.locations[-1]?.module
    ]
    | map(select(. != null and . != ""))
    | unique
    | join(", ")
  ) as $mods |
  (
    .properties.fixedVersion // .properties.fixed_version //
    $rule.properties.fixedVersion // $rule.properties.fixed_version // ""
  ) as $fixed |
  (if .level == "error" then "ERROR" else "INFO" end) +
  " govulncheck " + .ruleId +
  (if $mods != "" then " module=" + $mods else "" end) +
  (if $fixed != "" then " fixed=" + $fixed else "" end) +
  ": " + (.message.text // "")
' ./_reports/govulncheck.sarif >&2

govuln_called_count="$(jq '[.runs[]?.results[]? | select(.level == "error")] | length' ./_reports/govulncheck.sarif)"
if [ "${govuln_called_count}" -gt 0 ]; then
  status_govulncheck="FAIL"
  overall_rc=1
fi

echo "==> [${PROJECT_REL}] summary"
printf '  %-15s %s\n' "canary" "${status_canary}"
printf '  %-15s %s\n' "gofmt" "${status_gofmt}"
printf '  %-15s %s\n' "go vet" "${status_govet}"
printf '  %-15s %s\n' "golangci-lint" "${status_golangci}"
printf '  %-15s %s\n' "gosec" "${status_gosec}"
printf '  %-15s %s\n' "govulncheck" "${status_govulncheck}"

exit "${overall_rc}"
