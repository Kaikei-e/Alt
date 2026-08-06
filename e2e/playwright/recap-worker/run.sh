#!/usr/bin/env bash
# e2e/playwright/recap-worker/run.sh
#
# Brings up the recap-worker slice of the alt-staging stack (recap-db + Atlas
# migrator + the multi-aliased recap-pipeline-stub + recap-worker), runs the
# Playwright API suite inside the staging network so the `recap-worker` DNS
# name resolves, and tears the stack down.
#
# ADR-000766 established the `e2e/<framework>/<svc>/run.sh` dispatch contract;
# alt-deploy's release-deploy.yaml probes this path. Everything generic about
# the lifecycle — slice rendering, network-pool reclaim, retrying `up`, the
# failure log dump, secret redaction, teardown ordering — lives in
# `_lib/suite.sh`; see that file for the ordering contract and for why the
# dependency install and the test run are two separate containers.
#
# The alt-staging network is `internal: true`, which silently ignores host port
# publishes, so running the suite inside the network is the only portable way
# to reach the service under test. That constraint is inherited unchanged from
# the Hurl suite this replaces.
#
# Why the stub is one container with four names
# ---------------------------------------------
# The recap pipeline calls four upstreams — recap-subworker, news-creator,
# alt-data-hub and tag-generator. The first two are GPU-bound and impractical
# in CI, so `recap-pipeline-stub` joins the network under four aliases
# (`rw-stub-subworker`, `rw-stub-news-creator`, `rw-stub-alt-backend`,
# `rw-stub-tag-generator`) and serves schema-valid canned responses for every
# RPC the worker issues. The `rw-stub-` prefix keeps those names disjoint from
# the real service names other profiles use, so several profiles can share
# alt-staging at once.
#
# That stub also answers 200 to *every* unmatched path, which is why
# tests/topology.spec.ts asserts `GET /health` is a 404 on BASE_URL: it is the
# one request recap-worker and the stub answer differently.
#
# Before running: the images must exist, or you are testing the previous build
# (CLAUDE.md rule 3, moved into the E2E harness).
#
#   python3 e2e/playwright/_lib/build-images.py recap-worker
#
# recap-worker additionally needs the rust-bert model cache warmed on the
# *host* before the stack starts — compose bind-mounts /opt/rustbert-cache
# read-only and the staging network has no egress, so the container cannot
# reach HuggingFace from inside:
#
#   sudo mkdir -p /opt/rustbert-cache && sudo chown -R 999:999 /opt/rustbert-cache
#   docker run --rm -v /opt/rustbert-cache:/opt/rustbert-cache \
#     ghcr.io/kaikei-e/alt-recap-worker:ci warmup
#
# Environment overrides beyond the shared ones (see _lib/suite.sh):
#   BASE_URL       recap-worker's plaintext axum listener
#   MTLS_URL       the rustls port that must have nothing bound to it
#   DEFAULT_GENRE  RECAP_GENRES as the slice sets it
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../../.." && pwd)"
# shellcheck source=../_lib/suite.sh
source "$ROOT/e2e/playwright/_lib/suite.sh"

suite_init recap-worker

# recap-pipeline-stub is a test fixture built in-tree at the same rhythm as
# recap-worker, so it rides the same IMAGE_TAG. Forwarding both is what stops
# a CI job that built `:ci` from running the stack against `:main`.
suite_image_tags RECAP_WORKER_IMAGE_TAG RECAP_PIPELINE_STUB_IMAGE_TAG

# No suite_pki. The slice runs MTLS_ENFORCE=false / MTLS_LISTEN=false /
# PEER_IDENTITY_TRUSTED=off, matching the news-creator / tag-generator /
# knowledge-sovereign staging convention, so there is no mutual-TLS hop to mint
# material for. tests/topology.spec.ts asserts the consequence — nothing bound
# on MTLS_PORT — rather than leaving it implicit.
suite_endpoint BASE_URL      "http://recap-worker:9005"
suite_endpoint MTLS_URL      "https://recap-worker:9443"

# Kept in step with compose.staging.yaml's RECAP_GENRES for the recap-worker
# service. `trigger_recap` echoes the configured defaults back when the request
# body omits `genres`, and tests/pipeline.spec.ts asserts that echo; reading the
# value from here makes the compose setting and the expectation one fact.
suite_endpoint DEFAULT_GENRE "ai"

# recap-db-migrator is deliberately absent from this list. It is a one-shot job
# (`restart: "no"`) that exits when done, so `compose up --wait` would poll it
# forever; recap-worker's `depends_on: service_completed_successfully` gate is
# what orders it. setup/global-setup.ts probes a DB-backed endpoint so a
# migration that failed surfaces once, by name, rather than as a 500 on
# whichever spec ran first.
suite_up \
  recap-db \
  recap-pipeline-stub \
  recap-worker

suite_test
