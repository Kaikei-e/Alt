#!/usr/bin/env bash
# e2e/playwright/alt-backend/run.sh
#
# Brings up the alt-backend slice of the alt-staging stack (Postgres + Atlas
# migrator + alt-backend-deps-stub + alt-data-hub + alt-backend), runs the
# Playwright API suite inside the staging network so the `alt-backend` DNS name
# resolves, and tears the stack down.
#
# ADR-000766 established the `e2e/<framework>/<svc>/run.sh` dispatch contract;
# alt-deploy's release-deploy.yaml probes this path. Everything generic about
# the lifecycle lives in `_lib/suite.sh` — see that file for the ordering
# contract and for why the install and the run are two separate containers.
#
# Environment overrides beyond the shared ones (see _lib/suite.sh):
#   BASE_URL      alt-backend REST URL as seen from the test container
#   CONNECT_URL   user-facing Connect-RPC URL
#   INTERNAL_URL  loopback operator Connect-RPC URL
#   OPS_URL       operator listener: /health + /metrics. The three split
#                 binaries share it; Prometheus scrapes all of them there.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../../.." && pwd)"
# shellcheck source=../_lib/suite.sh
source "$ROOT/e2e/playwright/_lib/suite.sh"

suite_init alt-backend

# backend-deps-stub and alt-data-hub are co-versioned with alt-backend: same
# build stream, same Go module (ADR-000954's three-binary split).
suite_image_tags ALT_BACKEND_IMAGE_TAG ALT_BACKEND_DEPS_STUB_IMAGE_TAG ALT_DATA_HUB_IMAGE_TAG

# One CA, one server leaf for alt-data-hub and one client leaf named
# `alt-backend` — which is what DATAHUB_ALLOWED_PEERS admits.
suite_pki alt-data-hub alt-backend

suite_endpoint BASE_URL     "http://alt-backend:9000"
suite_endpoint CONNECT_URL  "http://alt-backend:9101"
suite_endpoint INTERNAL_URL "http://alt-backend:9102"
suite_endpoint OPS_URL      "http://alt-backend:9110"
suite_endpoint JWT_FILE     "$ROOT/e2e/fixtures/alt-backend/test-jwt.txt"
suite_endpoint ARTICLE_ID_SEED "00000000-0000-0000-0000-000000000001"

# The slice runs the real data plane, not a stub: ADR-000954 Wave 3 made
# alt-data-hub the only owner of alt_db, so cmd/backend reaches every row
# through it over mutual TLS.
suite_up \
  alt-backend-db \
  alt-backend-db-migrator \
  alt-backend-deps-stub \
  alt-data-hub \
  alt-backend

suite_test
