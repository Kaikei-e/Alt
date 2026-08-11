#!/usr/bin/env bash
# Static check across every Playwright API suite: does it typecheck, and does
# every spec actually load?
#
#   bash e2e/playwright/_lib/check-suites.sh [suite...]
#
# With no arguments it checks every suite that has a playwright.config.ts.
#
# Why this is worth a CI job of its own
# -------------------------------------
# The real suites need a Docker daemon, a compose slice and three minutes of
# stack startup before they can report that a spec file has a syntax error, a
# fixture name is misspelled, or two tests share a title. This needs none of
# that and finishes in seconds, so it gates the expensive jobs: a typo costs
# one minute of CI instead of twenty.
#
# `playwright test --list` is the interesting half. It loads the config,
# imports every spec, resolves every fixture, and reports the resulting test
# tree — so it catches a bad import, an invalid project matcher, a fixture that
# does not exist, and a duplicate title. What it cannot tell you is whether the
# assertions are *true*; only a run against a live stack does that.
#
# Environment
# -----------
# Each suite's src/env.ts reads its endpoints with `requiredEnv` and throws when
# one is missing — deliberately, so a misconfigured run fails by name instead of
# by connection error (CLAUDE.md rule 9). That means `--list` needs values too,
# even though it never sends a request. Rather than maintain a list of them here
# (which would drift the moment a suite grew an endpoint), this script reads the
# `requiredEnv(...)` / `requiredSecretFile(...)` calls out of the suite's own
# env.ts and supplies a placeholder for each. A suite that adds an endpoint is
# covered automatically; a suite that reads one *without* declaring it through
# those helpers fails here, which is itself the finding.
set -uo pipefail

ROOT="$(cd "$(dirname "$0")/../../.." && pwd)"
PW_ROOT="$ROOT/e2e/playwright"
cd "$PW_ROOT"

if [[ ! -d node_modules ]]; then
  echo "e2e/playwright/node_modules is missing — run 'npm ci' in e2e/playwright first." >&2
  exit 1
fi

# A placeholder that is non-empty, parses as a URL, and cannot resolve. If a
# spec ever reaches the network during --list, .invalid guarantees a loud
# failure rather than a request to something real (RFC 6761).
#
# Two schemes, tried in turn, because some suites validate the scheme at load:
# alt-data-hub's env.ts rejects a non-https DATA_HUB_URL (the service has no
# plaintext data-plane listener at all), while a suite asserting a plaintext
# port would reject https. Neither is wrong, and hardcoding one scheme here
# would make the gate fail on its own fixture rather than on the suite.
PLACEHOLDER_SCHEMES=("http" "https")

SECRET_FILE="$(mktemp)"
printf 'e2e-static-check-placeholder\n' > "$SECRET_FILE"
trap 'rm -f "$SECRET_FILE"' EXIT

suites=("$@")
if [[ "${#suites[@]}" -eq 0 ]]; then
  mapfile -t suites < <(
    for config in */playwright.config.ts; do
      [[ -e "$config" ]] || continue
      dirname "$config"
    done | sort
  )
fi

if [[ "${#suites[@]}" -eq 0 ]]; then
  echo "no suites found under $PW_ROOT" >&2
  exit 1
fi

failed=()

for suite in "${suites[@]}"; do
  echo "=============================================================="
  echo "== $suite"
  echo "=============================================================="

  if [[ ! -f "$suite/playwright.config.ts" ]]; then
    echo "FAIL: $suite has no playwright.config.ts" >&2
    failed+=("$suite (no config)")
    continue
  fi

  echo "-- tsc --noEmit -p $suite/tsconfig.json"
  if ! npx tsc --noEmit -p "$suite/tsconfig.json"; then
    failed+=("$suite (typecheck)")
    continue
  fi

  # Collect the env var names the suite declares, from its own source.
  url_vars=()
  env_args=()
  env_file="$suite/src/env.ts"
  if [[ -f "$env_file" ]]; then
    # Each helper needs a placeholder of its own shape. `requiredIntEnv` parses
    # its value, so handing it the URL placeholder makes env.ts throw and the
    # whole suite reports "No tests found" — a gate failing on its own fixture
    # rather than on the suite.
    #
    # `requiredEnv(` and `requiredIntEnv(` do not overlap as substrings, so
    # matching each literally keeps the two sets disjoint without a lookahead.
    while IFS= read -r name; do
      url_vars+=("$name")
    done < <(grep -oE 'requiredEnv\("[A-Z0-9_]+"' "$env_file" \
             | grep -oE '"[A-Z0-9_]+"' | tr -d '"' | sort -u)
    while IFS= read -r name; do
      env_args+=("$name=1")
    done < <(grep -oE 'requiredIntEnv\("[A-Z0-9_]+"' "$env_file" \
             | grep -oE '"[A-Z0-9_]+"' | tr -d '"' | sort -u)
    while IFS= read -r name; do
      env_args+=("$name=$SECRET_FILE")
    done < <(grep -oE 'requiredSecretFile\("[A-Z0-9_]+"' "$env_file" \
             | grep -oE '"[A-Z0-9_]+"' | tr -d '"' | sort -u)
  fi

  echo "-- playwright test --list ($(( ${#env_args[@]} + ${#url_vars[@]} )) placeholder env vars)"
  listed=0
  for scheme in "${PLACEHOLDER_SCHEMES[@]}"; do
    url_args=()
    for name in ${url_vars[@]+"${url_vars[@]}"}; do
      url_args+=("$name=$scheme://placeholder.invalid:9/e2e-static-check")
    done
    if ( cd "$suite" && env ${env_args[@]+"${env_args[@]}"} ${url_args[@]+"${url_args[@]}"} \
           npx playwright test --list > /tmp/pw-list-$$.txt 2>&1 ); then
      listed=1
      break
    fi
  done
  if [[ "$listed" -ne 1 ]]; then
    cat /tmp/pw-list-$$.txt >&2
    rm -f /tmp/pw-list-$$.txt
    failed+=("$suite (--list)")
    continue
  fi

  total="$(grep -oE 'Total: [0-9]+ tests? in [0-9]+ files?' /tmp/pw-list-$$.txt | tail -1)"
  rm -f /tmp/pw-list-$$.txt

  if [[ -z "$total" ]]; then
    echo "FAIL: $suite listed no tests at all" >&2
    failed+=("$suite (zero tests)")
    continue
  fi
  echo "ok: $total"
done

echo
echo "=============================================================="
echo "== shared lifecycle self-tests"
echo "=============================================================="
# e2e/_lib/reclaim-network-pool.test.sh was in the tree but in no workflow, so
# nothing ran it — and it had been failing on its first assertion since the
# helper gained its STAGING_PROJECT_NAME guard. A self-test nobody runs is a
# comment. Running it here makes the network-reclaim ordering (PM-2026-046) an
# enforced property again.
if [[ "$#" -eq 0 ]]; then
  if ! bash "$ROOT/e2e/_lib/reclaim-network-pool.test.sh"; then
    failed+=("reclaim-network-pool self-test")
  fi
  if ! bash "$ROOT/e2e/playwright/_lib/install-suite-deps.test.sh"; then
    failed+=("install-suite-deps self-test")
  fi
else
  echo "(skipped: only run for the whole fleet)"
fi

echo
echo "=============================================================="
echo "== suite wiring"
echo "=============================================================="
# Catches the wiring mistakes that leave every test green: an image built but
# not consumed, an image tag never forwarded (so the suite runs against the
# *previous* release of the service it is testing), a suite that exists in
# suites.yaml or on disk but not both. Only run when checking the whole fleet —
# a partial invocation would report the suites the caller did not ask about.
if [[ "$#" -eq 0 ]]; then
  if ! python3 "$ROOT/e2e/playwright/_lib/audit-suite-wiring.py"; then
    failed+=("suite wiring audit")
  fi
else
  echo "(skipped: only run for the whole fleet)"
fi

echo
echo "=============================================================="
if [[ "${#failed[@]}" -gt 0 ]]; then
  echo "FAILED (${#failed[@]}):"
  printf '  - %s\n' "${failed[@]}"
  exit 1
fi
echo "all ${#suites[@]} suites typecheck and load"
