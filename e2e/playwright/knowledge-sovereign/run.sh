#!/usr/bin/env bash
# e2e/playwright/knowledge-sovereign/run.sh
#
# Brings up the knowledge-sovereign slice of the alt-staging stack (Postgres +
# Atlas migrator + the sovereign service), runs the Playwright API suite inside
# the staging network so the `knowledge-sovereign` DNS name resolves, and tears
# the stack down.
#
# ADR-000766 established the `e2e/<framework>/<svc>/run.sh` dispatch contract;
# alt-deploy's release-deploy.yaml probes this path. Everything generic about
# the lifecycle lives in `_lib/suite.sh` — see that file for the ordering
# contract and for why the install and the run are two separate containers.
#
# Two things the retired Hurl run.sh did are deliberately NOT carried over:
#
#   * The UUID minting (`tenant_id`, `user_id`, `lens_id`, `lens_version_id`,
#     `signal_id`, `event_id`) and the `--variable` plumbing that threaded one
#     identity set through all twenty scenarios. Identities are now per-test
#     (src/fixtures.ts), which is what removes the `--jobs 1` requirement.
#   * `TODAY` / `OCCURRED_AT`. Deriving both from one shell `date` call was the
#     right instinct — they must agree across the UTC midnight boundary — and
#     `instant()` in src/fixtures.ts keeps it, per test rather than per run.
#
# compose.staging.yaml used to carry a `build:` stanza and no `image:` for
# knowledge-sovereign, which meant the image CI had already built with buildx
# and the registry layer cache was ignored and compose rebuilt the service from
# scratch on every run. It now has both, so this suite forwards its tag like
# every other. There is no mutual-TLS hop in this slice, so no `suite_pki`.
#
# Environment overrides beyond the shared ones (see _lib/suite.sh):
#   BASE_URL     Connect-RPC listener (LISTEN_ADDR). Also serves /health.
#   METRICS_URL  operator listener (METRICS_ADDR): /health, /metrics, /admin/*.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../../.." && pwd)"
# shellcheck source=../_lib/suite.sh
source "$ROOT/e2e/playwright/_lib/suite.sh"

suite_init knowledge-sovereign

# compose.staging.yaml carries `build:` *and* `image:` for this service, so the
# image CI pre-builds with the registry layer cache is what runs; without the
# tag forwarded here compose would look for `:main` and rebuild from source.
suite_image_tags KNOWLEDGE_SOVEREIGN_IMAGE_TAG

suite_endpoint BASE_URL    "http://knowledge-sovereign:9500"
suite_endpoint METRICS_URL "http://knowledge-sovereign:9501"

# The migrator is named explicitly rather than left to `depends_on`: it is a
# run-to-completion container, and `up --wait` only waits for a service it was
# asked to bring up. Without it the suite can start against a database whose
# `knowledge_projection_versions` seed row does not exist yet — which
# setup/global-setup.ts would then poll for until it timed out, reporting a
# readiness failure instead of a migration one.
suite_up \
  knowledge-sovereign-db \
  knowledge-sovereign-db-migrator \
  knowledge-sovereign

suite_test
