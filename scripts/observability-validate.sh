#!/usr/bin/env bash
# Static validation of the observability configuration tree.
#
# Runs, in order:
#   1. promtool check config   — prometheus.yml syntax
#   2. promtool check rules    — every file in observability/prometheus/rules/
#   3. promtool test rules     — every *_test.yml in observability/prometheus/tests/
#   4. amtool check-config     — observability/alertmanager/alertmanager.yml, IF it
#                                exists (the file is introduced separately; a repo
#                                state without it is valid and must stay green)
#   5. observability-config-audit.py — the structural invariants promtool cannot
#                                see (rule glob coverage, Grafana provisioning,
#                                dashboard uids, alertmanager wiring)
#
# Tool resolution, in order of preference:
#   $PROMTOOL / $AMTOOL           — explicit binary path (what CI sets)
#   promtool / amtool on $PATH
#   docker run <image from compose/observability.yaml>
# The docker fallback pins the exact image the production stack runs, so a
# local run validates against the same version that will load the file. It is
# the developer convenience path; CI installs real binaries and never reaches
# it.
#
# Exit non-zero on the first failing stage. Safe to run anywhere: it reads the
# repository and never contacts a running Prometheus.

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$REPO_ROOT"

COMPOSE_OBS="compose/observability.yaml"
PROM_CONFIG="observability/prometheus/prometheus.yml"
PROM_RULES_DIR="observability/prometheus/rules"
PROM_TESTS_DIR="observability/prometheus/tests"
AM_CONFIG="observability/alertmanager/alertmanager.yml"

# Read the image tag straight out of the compose file so the validator can
# never validate against a different version than the one that loads the file.
image_from_compose() {
  local repo="$1" fallback="$2" found
  found="$(grep -oE "image:[[:space:]]*${repo}:[^[:space:]\"']+" "$COMPOSE_OBS" 2>/dev/null \
    | head -1 | sed -E 's/image:[[:space:]]*//')"
  printf '%s' "${found:-$fallback}"
}

PROM_IMAGE="$(image_from_compose 'prom/prometheus' 'prom/prometheus:latest')"
AM_IMAGE="$(image_from_compose 'prom/alertmanager' 'prom/alertmanager:latest')"

resolve_tool() {
  # $1 = tool name, $2 = env override value, $3 = docker image
  local name="$1" override="$2" image="$3"
  if [[ -n "$override" ]]; then
    printf '%s' "$override"
    return 0
  fi
  if command -v "$name" >/dev/null 2>&1; then
    command -v "$name"
    return 0
  fi
  if command -v docker >/dev/null 2>&1; then
    printf 'docker run --rm --entrypoint /bin/%s -v %s:/repo -w /repo %s' \
      "$name" "$REPO_ROOT" "$image"
    return 0
  fi
  return 1
}

if ! PROMTOOL_CMD="$(resolve_tool promtool "${PROMTOOL:-}" "$PROM_IMAGE")"; then
  echo "::error::promtool not found. Install it, set PROMTOOL=/path/to/promtool, or make docker available."
  exit 1
fi

stage() { printf '\n==> %s\n' "$1"; }

stage "promtool check config — $PROM_CONFIG"
# NOTE: rule_files in prometheus.yml is the *container* path
# (/etc/prometheus/rules/*.yml). promtool treats a glob that matches nothing as
# a non-error, so this stage validates scrape/alerting/global syntax only. Rule
# content is stage 2 and rule-glob coverage is stage 5 — neither is implied by
# a green result here.
$PROMTOOL_CMD check config "$PROM_CONFIG"

stage "promtool check rules — $PROM_RULES_DIR"
shopt -s nullglob
rule_files=("$PROM_RULES_DIR"/*.yml "$PROM_RULES_DIR"/*.yaml)
shopt -u nullglob
if [[ ${#rule_files[@]} -eq 0 ]]; then
  echo "::error::no rule files under $PROM_RULES_DIR — the alerting surface is empty"
  exit 1
fi
$PROMTOOL_CMD check rules "${rule_files[@]}"

stage "promtool test rules — $PROM_TESTS_DIR"
shopt -s nullglob
test_files=("$PROM_TESTS_DIR"/*_test.yml "$PROM_TESTS_DIR"/*_test.yaml)
shopt -u nullglob
if [[ ${#test_files[@]} -eq 0 ]]; then
  echo "::error::no promtool unit tests under $PROM_TESTS_DIR — alert expressions are unpinned"
  exit 1
fi
# promtool resolves each test file's `rule_files:` relative to that file's own
# directory, so `../rules/x.yml` works from any working directory.
$PROMTOOL_CMD test rules "${test_files[@]}"

# Report which rule files no unit test covers. Advisory, not a gate: a rule can
# be worth shipping before its test exists, but an uncovered rule should be a
# visible choice rather than an accident.
uncovered=()
for rf in "${rule_files[@]}"; do
  base="$(basename "$rf")"
  if ! grep -qF -- "$base" "${test_files[@]}" 2>/dev/null; then
    uncovered+=("$rf")
  fi
done
if [[ ${#uncovered[@]} -gt 0 ]]; then
  for rf in "${uncovered[@]}"; do
    echo "::warning::$rf is referenced by no promtool unit test"
  done
fi

if [[ -f "$AM_CONFIG" ]]; then
  stage "amtool check-config — $AM_CONFIG"
  if AMTOOL_CMD="$(resolve_tool amtool "${AMTOOL:-}" "$AM_IMAGE")"; then
    # amtool validates structure and receiver wiring; it does not read the
    # `*_file` secret references, so this is safe on a runner that holds none.
    $AMTOOL_CMD check-config "$AM_CONFIG"
  else
    echo "::error::$AM_CONFIG exists but amtool is unavailable. Set AMTOOL=/path/to/amtool."
    exit 1
  fi
else
  echo ""
  echo "==> amtool check-config — skipped ($AM_CONFIG not present)"
fi

stage "structural audit — scripts/observability-config-audit.py"
python3 scripts/observability-config-audit.py --repo-root "$REPO_ROOT"

echo ""
echo "==> observability config validation passed"
