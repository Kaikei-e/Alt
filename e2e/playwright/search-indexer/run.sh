#!/usr/bin/env bash
# e2e/playwright/search-indexer/run.sh
#
# Brings up the search-indexer slice of the alt-staging stack (meilisearch +
# stub-backend + search-indexer), runs the Playwright API suite inside the
# staging network so the `search-indexer` and `meilisearch` DNS names resolve,
# and tears the stack down.
#
# ADR-000766 established the `e2e/<framework>/<svc>/run.sh` dispatch contract;
# alt-deploy's release-deploy.yaml probes this path. Everything generic about
# the lifecycle lives in `_lib/suite.sh` — see that file for the ordering
# contract (image tags before `suite_up`, because rendering the slice bakes
# environment values in) and for why the install and the run are two separate
# containers.
#
# Two things the Hurl runner needed and this one does not:
#
#   * A separate, serial `hurl_run … 00-seed-meilisearch.hurl` invocation
#     wedged between `compose up` and the suite. The seed is now
#     `setup/global-setup.ts`, which additionally waits for search-indexer to
#     have bootstrapped its index *before* pushing documents — the ordering
#     the old script got right only by accident of `depends_on`.
#   * `--file-root "$ROOT"`. Hurl forbids `..` in body-file paths, so the
#     fixture corpus had to be addressed relative to the repository root and
#     the runner had to mount and chdir accordingly. Node reads the file by
#     absolute path from MEILI_SEED_DOCS.
#
# `--jobs 4` had no ordering caveat in the Hurl runner and needs none here:
# the parity scenarios read a corpus seeded before any worker starts, and
# everything new reads a corpus its own worker seeded.
#
# Environment overrides beyond the shared ones (see _lib/suite.sh):
#   BASE_URL              REST :9300, as seen from the test container
#   CONNECT_URL           Connect-RPC :9301
#   MTLS_ABSENT_URL       :9443 — asserted CLOSED, see tests/topology.spec.ts
#   MEILI_URL             Meilisearch, seeded by setup/global-setup.ts
#   MEILI_MASTER_KEY_FILE path to the master key (a path, never a value)
#   MEILI_SEED_DOCS       the shared fixture corpus
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../../.." && pwd)"
# shellcheck source=../_lib/suite.sh
source "$ROOT/e2e/playwright/_lib/suite.sh"

suite_init search-indexer

# compose.staging.yaml keys the image off SEARCH_INDEXER_IMAGE_TAG (default
# `main`) so unrelated services stay on the last successful main build; this
# forwards the suite's own IMAGE_TAG into it. meilisearch and stub-backend come
# straight from Docker Hub (`getmeili/meilisearch:v1.11`, `nginx:1.27-alpine`)
# and have no tag of ours.
suite_image_tags SEARCH_INDEXER_IMAGE_TAG

# No suite_pki, and that is asserted rather than assumed. The slice sets
# MTLS_LISTEN=false, so bootstrap/app.go never binds :9443 and there is no
# mutual-TLS material for anything to present — tests/topology.spec.ts proves
# the port is closed instead of leaving it an unstated premise.

suite_endpoint BASE_URL        "http://search-indexer:9300"
suite_endpoint CONNECT_URL     "http://search-indexer:9301"
suite_endpoint MTLS_ABSENT_URL "http://search-indexer:9443"
suite_endpoint MEILI_URL       "http://meilisearch:7700"
# Anchored on the same fixture file compose's `secrets:` block mounts, so
# changing one rotates both. A path, not a value: it must never land in
# `docker inspect` output or a CI environment dump.
suite_endpoint MEILI_MASTER_KEY_FILE "$ROOT/e2e/fixtures/staging-secrets/meili_master_key.txt"
# The canonical corpus. The parity assertions carried over from the Hurl suite
# — two "rust" documents, owned by alice and bob — are statements about exactly
# this file.
suite_endpoint MEILI_SEED_DOCS "$ROOT/e2e/fixtures/search-indexer/seed-docs.json"

# stub-backend is nginx answering 200 to everything: search-indexer only probes
# BACKEND_API_URL for reachability at startup and never validates the body. It
# is also what RECAP_WORKER_URL and REDIS_STREAMS_URL point at, which is why
# the recap index stays empty and CONSUMER_ENABLED is false in this slice.
suite_up meilisearch stub-backend search-indexer

suite_test
