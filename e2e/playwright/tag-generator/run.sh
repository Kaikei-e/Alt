#!/usr/bin/env bash
# e2e/playwright/tag-generator/run.sh
#
# Brings up the tag-generator slice of the alt-staging stack (redis-streams +
# stub-backend + mq-hub + tag-generator), runs the Playwright API suite inside
# the staging network so the `tag-generator` and `mq-hub` DNS names resolve,
# and tears the stack down.
#
# ADR-000766 established the `e2e/<framework>/<svc>/run.sh` dispatch contract;
# alt-deploy's release-deploy.yaml probes this path. Everything generic about
# the lifecycle lives in `_lib/suite.sh` — see that file for the ordering
# contract and for why the dependency install and the test run are two
# separate containers.
#
# Why mq-hub is in the slice
# --------------------------
# Half of what tag-generator does is consume `alt:events:tags` and reply on a
# Redis Stream. No HTTP client can XREAD, so the round trip is driven through
# mq-hub's synchronous `GenerateTagsForArticle` wrapper — exactly as the Hurl
# suite did. mq-hub therefore has to sit in the same slice, and redis-streams
# under it.
#
# stub-backend is nginx answering 200 to everything; tag-generator only probes
# BACKEND_API_URL for reachability and this suite never drives the article
# fetch path that would need real Connect-RPC responses.
#
# Native inbound TLS uses generated leaves and retains strict peer verification.
#
# Environment overrides beyond the shared ones (see _lib/suite.sh):
#   BASE_URL         tag-generator's FastAPI listener
#   MQHUB_BASE_URL   mq-hub's Connect-RPC listener
#   TLS_SIDECAR_URL  the port the production mTLS sidecar would bind, asserted closed
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../../.." && pwd)"
# shellcheck source=../_lib/suite.sh
source "$ROOT/e2e/playwright/_lib/suite.sh"

suite_init tag-generator
suite_pki tag-generator alt-backend denied-peer
suite_endpoint TLS_CLIENT_CERT_FILE "$SUITE_PKI_DIR/alt-backend.pem"
suite_endpoint TLS_CLIENT_KEY_FILE "$SUITE_PKI_DIR/alt-backend-key.pem"
suite_endpoint TLS_DENIED_CERT_FILE "$SUITE_PKI_DIR/denied-peer.pem"
suite_endpoint TLS_DENIED_KEY_FILE "$SUITE_PKI_DIR/denied-peer-key.pem"
suite_endpoint PLAINTEXT_URL "http://tag-generator:9400"
suite_endpoint MQHUB_AUTH_TOKEN_FILE "$ROOT/e2e/playwright/_fixtures/test-credentials/mqhub_auth_token.txt"

# Both images are built for this suite's CI job, so both follow IMAGE_TAG.
# The Hurl script forwarded only TAG_GENERATOR_IMAGE_TAG, which left mq-hub
# resolving to `${MQ_HUB_IMAGE_TAG:-main}` — i.e. the CI job built
# alt-mq-hub:ci and then ran the stack against alt-mq-hub:main. Forwarding
# both is what the CI job always meant.
suite_image_tags TAG_GENERATOR_IMAGE_TAG MQ_HUB_IMAGE_TAG

suite_endpoint BASE_URL        "https://tag-generator:9443"
suite_endpoint MQHUB_BASE_URL  "http://mq-hub:9500"
suite_endpoint TLS_SIDECAR_URL "https://tag-generator:9443"

suite_up \
  redis-streams \
  stub-backend \
  mq-hub \
  tag-generator

suite_test
