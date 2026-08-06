#!/usr/bin/env bash
# e2e/playwright/news-creator/run.sh
#
# Brings up the news-creator slice of the alt-staging stack
# (news-creator-ollama-stub + news-creator), runs the Playwright API suite
# inside the staging network so the `news-creator` DNS name resolves, and
# tears the stack down.
#
# ADR-000766 established the `e2e/<framework>/<svc>/run.sh` dispatch contract.
# Everything generic about the lifecycle — slice rendering, network-pool
# reclaim, retrying `up`, the failure log dump, secret redaction, teardown
# ordering — lives in `_lib/suite.sh`; see that file for the ordering contract
# and for why the dependency install and the test run are two containers.
#
# The staging network is `internal: true`, which silently ignores host port
# publishes, so running the suite inside the network is the only portable way
# to reach the service under test. That constraint is inherited unchanged from
# the Hurl suite this replaces.
#
# The slice has no GPU: news-creator's real backend is Ollama on an RTX 4060,
# and CI swaps it for `compose/news-creator-ollama-stub/app.py`, a FastAPI stub
# that answers /api/tags, /api/generate and /api/chat (sync + NDJSON) with
# fixed payloads keyed off the request's `format` field.
#
# Environment overrides beyond the shared ones (see _lib/suite.sh):
#   BASE_URL         news-creator URL as seen from the test container
#   UNBOUND_URL      a port on the same host that must have nothing bound to it
#   STUB_MODEL       the model name /api/tags advertises (== LLM_MODEL)
#   MAX_QUEUE_DEPTH  the semaphore's queue ceiling (== the compose value)
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../../.." && pwd)"
# shellcheck source=../_lib/suite.sh
source "$ROOT/e2e/playwright/_lib/suite.sh"

suite_init news-creator

# Only one image is built for this slice; news-creator-ollama-stub is built
# from context rather than pulled, so it has no *_IMAGE_TAG of its own.
suite_image_tags NEWS_CREATOR_IMAGE_TAG

# No suite_pki: the slice runs unauthenticated (MTLS_ENFORCE=false,
# PEER_IDENTITY_TRUSTED=off in compose.staging.yaml), matching the
# mq-hub / tag-generator / knowledge-sovereign staging convention. The
# `@authz` spec in tests/health.spec.ts asserts that the peer-identity header
# is inert under exactly that configuration.
suite_endpoint BASE_URL    "http://news-creator:11434"

# main.py's `if __name__ == "__main__"` block would bind 8001; the shipped
# image binds 11434. tests/topology.spec.ts asserts nothing answers here.
suite_endpoint UNBOUND_URL "http://news-creator:8001"

# Kept in step with compose.staging.yaml's LLM_MODEL and MAX_QUEUE_DEPTH for
# the news-creator service. The Hurl suite hard-coded both into its
# assertions, so changing either in compose broke the tests with an opaque
# diff; reading them from here makes the compose value and the expectation one
# fact (src/env.ts reads both with requiredEnv/requiredIntEnv).
suite_endpoint STUB_MODEL      "gemma3:4b-it-qat"
suite_endpoint MAX_QUEUE_DEPTH "10"

suite_up news-creator-ollama-stub news-creator

suite_test
