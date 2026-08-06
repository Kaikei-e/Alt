#!/usr/bin/env bash
# e2e/playwright/rag-orchestrator/run.sh
#
# Brings up the rag-orchestrator slice of the alt-staging stack (rag-db
# Postgres with pgvector + the Atlas migrator + rag-orchestrator), seeds the
# Augur chat fixtures through psql inside the rag-db container, runs the
# Playwright API suite inside the staging network so the `rag-orchestrator` DNS
# name resolves, and tears the stack down.
#
# ADR-000766 established the `e2e/<framework>/<svc>/run.sh` dispatch contract.
# Everything generic about the lifecycle lives in `_lib/suite.sh` — see that
# file for the ordering contract (image tags and PKI must be exported *before*
# `suite_up` renders the slice) and for why the install and the run are two
# separate containers.
#
# Environment overrides beyond the shared ones (see _lib/suite.sh):
#   BASE_URL          Echo REST URL as seen from the test container
#   CONNECT_URL       Connect-RPC URL (plaintext h2c in this slice)
#   MTLS_SIDECAR_URL  the port rag-orchestrator talks to alt-data-hub *on* —
#                     asserted CLOSED here, see tests/topology.spec.ts
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../../.." && pwd)"
# shellcheck source=../_lib/suite.sh
source "$ROOT/e2e/playwright/_lib/suite.sh"

suite_init rag-orchestrator

suite_image_tags RAG_ORCHESTRATOR_IMAGE_TAG

# Throwaway mutual-TLS material. No scenario in this suite reaches
# alt-data-hub, but rag-orchestrator cannot *start* without the client leaf:
# `config.loadDataHub` panics on an unset or non-https DATAHUB_MTLS_URL and
# `httpclient.NewDataHubClient` reads the CA at construction, because after
# ADR-000954 D7 there is no plaintext route to alt_db to degrade onto
# (CLAUDE.md rules 8/9). The server leaf names alt-data-hub — unused here, but
# mint_staging_pki wants one — and the client leaf is `rag-orchestrator`, the
# CN production puts in DATAHUB_ALLOWED_PEERS.
#
# Must run before suite_up: compose.staging.yaml interpolates STAGING_PKI_DIR
# into the bind mounts, and `docker compose config` bakes it in as it renders.
suite_pki alt-data-hub rag-orchestrator

suite_endpoint BASE_URL         "http://rag-orchestrator:9010"
suite_endpoint CONNECT_URL      "http://rag-orchestrator:9011"
suite_endpoint MTLS_SIDECAR_URL "http://rag-orchestrator:9443"

# `--build` is required: CI builds the rag-orchestrator image locally and
# rag-db / rag-db-migrator are local build contexts with no GHCR image at all.
# `--wait` blocks on healthcheck convergence; rag-db-migrator's `restart: "no"`
# plus rag-orchestrator's `service_completed_successfully` gate is what
# guarantees `atlas migrate apply` finishes before the server binds.
#
# rag-orchestrator itself declares no container healthcheck — it ships on
# distroless/static with no shell and no `healthcheck` subcommand — so `--wait`
# only proves the process is running. `setup/global-setup.ts` closes that gap by
# polling /readyz, /connect/health and the seeded rows before any test runs.
suite_up --build \
  rag-db \
  rag-db-migrator \
  rag-orchestrator

# ---------------------------------------------------------------------------
# Seed
# ---------------------------------------------------------------------------
# Augur has no write RPC that a test could seed through: `StreamChat` is the
# only path that creates a conversation, and it drives the embedder, the LLM
# and search-indexer, all of which are parked at 127.0.0.1:9 in this slice. So
# the rows arrive out of band, and the parallel-safety the fleet requires is
# bought with one *owner* per role instead — see src/seed.ts.
#
# `exec -T` disables TTY allocation so the SQL can be piped on stdin.
# `ON_ERROR_STOP=1` makes a broken fixture fail the run here, loudly, instead
# of showing up as four confusing test failures later.
echo "==> seeding augur conversation fixtures via psql" >&2
docker compose -f "$SLICE" -p "$STAGING_PROJECT_NAME" exec -T \
  -e PGPASSWORD=alt-staging-test-rag-password \
  rag-db psql -v ON_ERROR_STOP=1 \
    -U rag_user -d rag_db \
  < "$SUITE_DIR/setup/db-seed.sql"

suite_test
