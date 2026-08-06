#!/usr/bin/env bash
# e2e/playwright/acolyte-orchestrator/run.sh
#
# Brings up the acolyte-orchestrator slice of the alt-staging stack (acolyte-db
# Postgres + Atlas migrator + the Starlette/Connect orchestrator, plus the
# Ollama stub and the Meilisearch/search-indexer pair the gatherer node reads
# evidence from), runs the Playwright API suite inside the staging network so
# the `acolyte-orchestrator` DNS name resolves, and tears the stack down.
#
# ADR-000766 established the `e2e/<framework>/<svc>/run.sh` dispatch contract.
# Everything generic about the lifecycle lives in `_lib/suite.sh` — see that
# file for the ordering contract and for why the install and the run are two
# separate containers.
#
# Environment overrides beyond the shared ones (see _lib/suite.sh):
#   BASE_URL              orchestrator URL as seen from the test container
#   MTLS_SIDECAR_URL      the port pki-agent serves mTLS on in production —
#                         asserted CLOSED here, see tests/topology.spec.ts
#   SEARCH_INDEXER_URL    evidence retrieval backend for the gatherer node
#   MEILI_URL             Meilisearch, seeded by setup/global-setup.ts
#   MEILI_MASTER_KEY_FILE path to the master key (a path, never a value)
#   MEILI_SEED_DOCS       fixture corpus pushed into the `articles` index
#   OLLAMA_STUB_URL       what NEWS_CREATOR_URL points at in the slice
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../../.." && pwd)"
# shellcheck source=../_lib/suite.sh
source "$ROOT/e2e/playwright/_lib/suite.sh"

suite_init acolyte-orchestrator

# No `suite_image_tags` on purpose, and this is a behavioural choice rather than
# an omission: acolyte-orchestrator, acolyte-db-migrator and
# news-creator-ollama-stub are all *local build contexts* in
# compose.staging.yaml (no GHCR image to tag), and the only pulled dependency —
# search-indexer — resolves through `${SEARCH_INDEXER_IMAGE_TAG:-main}`. Forcing
# this suite's IMAGE_TAG onto it would pin search-indexer to a tag that the
# dispatch SHA may never have built, turning an unrelated green build into a
# `manifest unknown` on `compose pull`. The retired Hurl run.sh made the same
# call for the same reason.
#
# No `suite_pki` either: the slice runs MTLS_ENFORCE=false /
# PEER_IDENTITY_TRUSTED=off, so there is no mutual-TLS material to mint. The
# consequences of that configuration are asserted, not assumed — see
# tests/topology.spec.ts and the peer-identity cases in tests/health.spec.ts.

suite_endpoint BASE_URL             "http://acolyte-orchestrator:8090"
suite_endpoint MTLS_SIDECAR_URL     "http://acolyte-orchestrator:9443"
suite_endpoint SEARCH_INDEXER_URL   "http://search-indexer:9300"
suite_endpoint MEILI_URL            "http://meilisearch:7700"
suite_endpoint OLLAMA_STUB_URL      "http://news-creator-ollama-stub:11435"
# Anchored on the same fixture file compose's `secrets:` block mounts, so
# changing one rotates both. A path, not a value: it must never land in
# `docker inspect` output or a CI environment dump.
suite_endpoint MEILI_MASTER_KEY_FILE "$ROOT/e2e/fixtures/staging-secrets/meili_master_key.txt"
# The canonical search-indexer corpus. The `AI infrastructure trends` documents
# in it are what the gatherer node retrieves for the run-lifecycle scenarios;
# without them the pipeline still completes but with empty evidence, which is a
# materially different code path from the one production takes.
suite_endpoint MEILI_SEED_DOCS       "$ROOT/e2e/fixtures/search-indexer/seed-docs.json"

# `--build` is required: acolyte-db-migrator, acolyte-orchestrator and
# news-creator-ollama-stub have no GHCR image at all. The migrator's
# restart=no + the orchestrator's `service_completed_successfully` gate is what
# guarantees `atlas migrate apply` finishes before uvicorn binds.
suite_up --build \
  acolyte-db \
  acolyte-db-migrator \
  acolyte-orchestrator \
  news-creator-ollama-stub \
  meilisearch \
  stub-backend \
  search-indexer

suite_test
