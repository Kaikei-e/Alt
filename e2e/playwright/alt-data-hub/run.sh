#!/usr/bin/env bash
# e2e/playwright/alt-data-hub/run.sh
#
# Brings up the alt-data-hub slice of the alt-staging stack (Postgres + Atlas
# migrator + alt-backend-deps-stub + alt-data-hub), runs the Playwright API
# suite inside the staging network, and tears the stack down.
#
# alt-data-hub is the data plane of the alt-backend split: it serves
# services.datahub.v1.DataHubService over mutual TLS on :9443 and a plaintext
# operator listener on :9110, and it publishes no host port at all. That is
# what forces the same shape as the mq-hub / knowledge-sovereign / alt-backend
# suites — the tests run in a container attached to the staging network,
# because that is the only place the service is addressable.
#
# What is different here is the credential. There is no JWT: the client
# certificate *is* the caller's identity, so `suite_pki` mints a throwaway
# PKI before the stack comes up and the suite reads the leaf paths out of the
# environment. Certificates are minted per run and never committed
# (e2e/.gitignore); `_lib/suite.sh`'s EXIT trap wipes them.
#
# ADR-000766 established the `e2e/<framework>/<svc>/run.sh` dispatch contract;
# alt-deploy's release-deploy.yaml probes this path. Everything generic about
# the lifecycle lives in `_lib/suite.sh` — see that file for the ordering
# contract (image tags and PKI before `suite_up`, because rendering the slice
# bakes environment values in) and for why the install and the run are two
# separate containers.
#
# Environment overrides beyond the shared ones (see _lib/suite.sh):
#   DATA_HUB_URL   alt-data-hub's mTLS listener, as seen from the test container
#   OPS_URL        the plaintext operator listener (/health + /metrics)
#   ALLOWED_PEER   CN/SAN of the leaf the positive suite presents; must appear
#                  in alt-data-hub's DATAHUB_ALLOWED_PEERS
#   DENIED_PEER    CN/SAN of the leaf the negative probes present; must NOT
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../../.." && pwd)"
# shellcheck source=../_lib/suite.sh
source "$ROOT/e2e/playwright/_lib/suite.sh"

suite_init alt-data-hub

# alt-backend-deps-stub is co-versioned with the alt-backend module: same build
# stream, same Go module (ADR-000954's three-binary split). It stands in for
# AUTH_HUB_URL / SOVEREIGN_URL / MQHUB_CONNECT_URL, all of which
# config.ValidateDataHubConfig requires before the process will boot.
suite_image_tags ALT_DATA_HUB_IMAGE_TAG ALT_BACKEND_DEPS_STUB_IMAGE_TAG

# The two peer identities, resolved *before* suite_pki and suite_up.
#
# compose.staging.yaml interpolates ALLOWED_PEER into
# `DATAHUB_ALLOWED_PEERS=${ALLOWED_PEER:-pre-processor},alt-backend,alt-harvester`,
# and `docker compose config` bakes that in when the slice is rendered. If the
# name the allowlist gets and the name the leaf is minted under ever drifted,
# every positive scenario would die in the TLS handshake with no assertion to
# point at the cause — so both come from one variable, exported once.
: "${ALLOWED_PEER:=pre-processor}"
: "${DENIED_PEER:=rogue-peer}"
export ALLOWED_PEER DENIED_PEER

# One CA, a server leaf for alt-data-hub (also copied to svc-cert.pem /
# svc-key.pem, the production filenames compose mounts at /certs), and two
# client leaves: one on the allowlist and one deliberately off it. The second
# is the whole point — it proves the boundary rejects a *valid chain* with the
# wrong identity, which is the failure a shared internal CA makes possible.
suite_pki alt-data-hub "$ALLOWED_PEER" "$DENIED_PEER"

suite_endpoint DATA_HUB_URL "https://alt-data-hub:9443"
suite_endpoint OPS_URL      "http://alt-data-hub:9110"

# SUITE_PKI_DIR is set by suite_pki, so this line must follow it. The path is
# absolute and the test container mounts $ROOT at the same path, so the leaf
# files resolve identically inside and out.
suite_endpoint PKI_DIR      "$SUITE_PKI_DIR"
suite_endpoint ALLOWED_PEER "$ALLOWED_PEER"
suite_endpoint DENIED_PEER  "$DENIED_PEER"

# The ports the monolith listened on. cmd/datahub opens neither, and
# tests/topology.spec.ts proves it by asserting the connection is refused —
# the assertion the Hurl suite had to express outside the test framework, with
# a probe file that passed on any response plus a shell wrapper demanding exit
# code 3 (e2e/hurl/_lib/assert-transport-refused.sh).
suite_endpoint ABSENT_REST_URL     "http://alt-data-hub:9000"
suite_endpoint ABSENT_CONNECT_URL  "http://alt-data-hub:9101"
suite_endpoint ABSENT_OPERATOR_URL "http://alt-data-hub:9102"

suite_up \
  alt-backend-db \
  alt-backend-db-migrator \
  alt-backend-deps-stub \
  alt-data-hub

suite_test
