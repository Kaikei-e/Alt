#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
cd "${REPO_ROOT}"

WORK_DIR="$(mktemp -d)"
trap 'rm -rf "${WORK_DIR}"' EXIT

require_cmd() {
  local cmd="$1"
  local hint="$2"
  if ! command -v "${cmd}" >/dev/null 2>&1; then
    echo "ERROR: Required command not found in PATH: ${cmd} (${hint})" >&2
    exit 1
  fi
}

require_cmd docker "install Docker CLI with Compose plugin"
require_cmd lkal "install via 'make install-lkal'"
require_cmd kisage "install via 'make install-kisage'"

installer="${REPO_ROOT}/scripts/ci-install-compose.sh"
if [ ! -f "${installer}" ]; then
  echo "ERROR: Missing compose installer: ${installer}" >&2
  exit 1
fi

expected_compose_version="$(
  sed -n 's/^COMPOSE_VERSION="\${COMPOSE_VERSION:-\([^}]*\)}"/\1/p' "${installer}" | head -n 1
)"
if [ -z "${expected_compose_version}" ]; then
  echo "ERROR: Could not parse COMPOSE_VERSION pin from ${installer}" >&2
  exit 1
fi

if ! actual_compose_output="$(docker compose version 2>&1)"; then
  echo "ERROR: failed to query docker compose version: ${actual_compose_output}" >&2
  exit 1
fi
actual_compose_version="$(awk '{print $NF}' <<<"${actual_compose_output}")"
if [ "${actual_compose_version}" != "${expected_compose_version}" ]; then
  echo "ERROR: docker compose version mismatch: expected ${expected_compose_version}, got '${actual_compose_output}'" >&2
  exit 1
fi

if command -v python3 >/dev/null 2>&1 && python3 -c 'import sys, yaml; ver = getattr(yaml, "__version__", ""); sys.exit(0 if sys.version_info >= (3, 12) and ver.split(".", 1)[0] == "6" else 1)' >/dev/null 2>&1; then
  :
elif command -v uv >/dev/null 2>&1; then
  mkdir -p "${WORK_DIR}/bin"
  cat <<'SH' >"${WORK_DIR}/bin/python3"
#!/usr/bin/env bash
exec uv run --quiet --no-project --python 3.14 --with 'pyyaml==6.*' python "$@"
SH
  chmod +x "${WORK_DIR}/bin/python3"
  export PATH="${WORK_DIR}/bin:${PATH}"
else
  echo "ERROR: Python >= 3.12 with PyYAML 6.x (or uv) is required" >&2
  exit 1
fi

if [ ! -f "${REPO_ROOT}/.env.template" ]; then
  echo "ERROR: Missing .env.template at ${REPO_ROOT}/.env.template" >&2
  exit 1
fi

STAGE_DIR="${WORK_DIR}/stage"
mkdir -p "${STAGE_DIR}/compose"

for entry in "${REPO_ROOT}"/* "${REPO_ROOT}"/.[!.]* "${REPO_ROOT}"/..?*; do
  [ -e "${entry}" ] || [ -L "${entry}" ] || continue
  base="$(basename "${entry}")"
  case "${base}" in
    compose|.env) continue ;;
  esac
  ln -s "${entry}" "${STAGE_DIR}/${base}"
done

for entry in "${REPO_ROOT}/compose"/* "${REPO_ROOT}/compose"/.[!.]* "${REPO_ROOT}/compose"/..?*; do
  [ -e "${entry}" ] || [ -L "${entry}" ] || continue
  base="$(basename "${entry}")"
  [ "${base}" = ".env" ] && continue
  cp -R "${entry}" "${STAGE_DIR}/compose/${base}"
done

DUMMY_ENV="${STAGE_DIR}/.env"
{
  cat "${REPO_ROOT}/.env.template"
  printf '\nDOCKER_GROUP_ID=0\nKRATOS_COOKIE_DOMAIN=\nTRUSTED_PROXY_CIDRS=none\n'
} >"${DUMMY_ENV}"
cp "${DUMMY_ENV}" "${STAGE_DIR}/compose/.env"

declare -a CHECK_NAMES=()
declare -a CHECK_RESULTS=()
overall_rc=0

run_check() {
  local name="$1"
  shift
  echo "==> ${name}"
  local rc=0
  set +e
  "$@"
  rc=$?
  set -e
  CHECK_NAMES+=("${name}")
  if [ "${rc}" -eq 0 ]; then
    CHECK_RESULTS+=("PASS")
  else
    CHECK_RESULTS+=("FAIL (exit ${rc})")
    overall_rc=1
  fi
}

run_check "test_ci_install_compose" \
  python3 -m unittest discover -s tests/scripts -p test_ci_install_compose.py
run_check "compose-healthcheck-audit" \
  python3 scripts/compose-healthcheck-audit.py --repo-root "${STAGE_DIR}" --env-file "${DUMMY_ENV}"
run_check "test-compose-port-audit" \
  python3 scripts/tests/test-compose-port-audit.py
run_check "compose-port-audit" \
  python3 scripts/compose-port-audit.py --repo-root "${STAGE_DIR}" --overlays
run_check "check-compose-variables" \
  env COMPOSE_FILE="${STAGE_DIR}/compose/compose.yaml" bash scripts/check-compose-variables.sh
run_check "test-compose-netns-cascade-audit" \
  python3 scripts/tests/test-compose-netns-cascade-audit.py
run_check "compose-netns-cascade-audit" \
  python3 scripts/compose-netns-cascade-audit.py
run_check "test-pki-wave4-acceptance" \
  python3 scripts/tests/test-pki-wave4-acceptance.py
run_check "test-pki-subject-scoped-provisioners" \
  python3 scripts/tests/test-pki-subject-scoped-provisioners.py
run_check "test-pki-agent-recovery-runbook" \
  python3 scripts/tests/test-pki-agent-recovery-runbook.py
run_check "test-retire-pki-writers" \
  python3 scripts/tests/test-retire-pki-writers.py
run_check "test-health-deep-contract" \
  python3 scripts/tests/test-health-deep-contract.py
run_check "test-p2-doc-contracts" \
  python3 scripts/tests/test-p2-doc-contracts.py
run_check "test_deploy" \
  bash tests/scripts/test_deploy.sh
run_check "test_cascade_pki_sidecars" \
  bash tests/scripts/test_cascade_pki_sidecars.sh
run_check "test-compose-file-bind-audit" \
  python3 scripts/tests/test-compose-file-bind-audit.py
run_check "compose-file-bind-audit" \
  python3 scripts/compose-file-bind-audit.py
run_check "test-compose-init-edge-audit" \
  python3 scripts/tests/test-compose-init-edge-audit.py
run_check "test-compose-pre-start-audit" \
  python3 scripts/tests/test-compose-pre-start-audit.py
run_check "compose-init-edge-audit" \
  python3 scripts/compose-init-edge-audit.py
run_check "compose-pre-start-audit" \
  python3 scripts/compose-pre-start-audit.py
run_check "test-compose-ops-surface-audit" \
  python3 scripts/tests/test-compose-ops-surface-audit.py
run_check "compose-ops-surface-audit" \
  python3 scripts/compose-ops-surface-audit.py
run_check "test-compose-restart-policy-audit" \
  python3 scripts/tests/test-compose-restart-policy-audit.py
run_check "compose-restart-policy-audit" \
  python3 scripts/compose-restart-policy-audit.py
run_check "test-compose-feature-gate" \
  python3 scripts/tests/test-compose-feature-gate.py
run_check "compose-feature-gate" \
  python3 scripts/compose-feature-gate.py --gate-report
run_check "test-compose-prod-env-wiring" \
  python3 scripts/tests/test-compose-prod-env-wiring.py
run_check "test_compose_wiring" \
  env COMPOSE_REPO_ROOT="${STAGE_DIR}" COMPOSE_ENV_FILE="${DUMMY_ENV}" bash scripts/tests/test_compose_wiring.sh
run_check "test-compose-sovereign-knobs" \
  python3 scripts/tests/test-compose-sovereign-knobs.py
run_check "test-recap-evaluator-auth-wiring" \
  python3 scripts/tests/test-recap-evaluator-auth-wiring.py
run_check "test-check-env-overrides" \
  python3 scripts/tests/test-check-env-overrides.py
run_check "test-group1-security" \
  python3 scripts/tests/test-group1-security.py
run_check "test-compose-reachability-audit" \
  python3 scripts/tests/test-compose-reachability-audit.py
run_check "compose-reachability-audit" \
  python3 scripts/compose-reachability-audit.py --repo-root "${STAGE_DIR}" -f "${STAGE_DIR}/compose/compose.yaml" --env-file "${DUMMY_ENV}"
run_check "lkal-check" \
  lkal check --json deploy/desired-env.lkal
run_check "kisage-env-verify" \
  python3 scripts/kisage-env.py verify
run_check "test-kisage-env" \
  python3 scripts/tests/test-kisage-env.py
run_check "test-rag-orchestrator-callers" \
  python3 scripts/tests/test-rag-orchestrator-callers.py
run_check "test_kisage_clean_checkout" \
  python3 -m unittest discover -s tests/scripts -p test_kisage_clean_checkout.py

echo ""
echo "==> Summary"
for i in "${!CHECK_NAMES[@]}"; do
  printf '  %-42s %s\n' "${CHECK_NAMES[$i]}" "${CHECK_RESULTS[$i]}"
done

exit "${overall_rc}"
