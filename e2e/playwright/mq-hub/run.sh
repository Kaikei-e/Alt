#!/usr/bin/env bash
# e2e/playwright/mq-hub/run.sh
#
# Brings up the mq-hub slice of the alt-staging stack (redis-streams +
# mq-hub), runs the Playwright API suite inside the staging network so the
# `mq-hub` DNS name resolves, and tears the stack down.
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
#   * `python3 e2e/fixtures/mq-hub/gen-batch-oversize.py` — the 1001-event
#     MAX_BATCH_SIZE fixture is now built in the spec that uses it
#     (src/events.ts), so there is no generated, gitignored JSON file and no
#     Python dependency on the runner.
#   * `--jobs 1` — that was load-bearing for Hurl because 12-stream-info read
#     a stream 04/07/10 had written. Every spec here seeds the stream it
#     reads, under a key nothing else touches, so the suite runs
#     `fullyParallel`.
#
# Environment overrides beyond the shared ones (see _lib/suite.sh):
#   BASE_URL        mq-hub's one listener, as seen from the test container
#   OPS_ABSENT_URL  a port mq-hub must NOT answer on (tests/topology.spec.ts)
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../../.." && pwd)"
# shellcheck source=../_lib/suite.sh
source "$ROOT/e2e/playwright/_lib/suite.sh"

suite_init mq-hub

# compose.staging.yaml keys the image off MQ_HUB_IMAGE_TAG (default `main`) so
# unrelated services stay on the last successful main build; this forwards the
# suite's own IMAGE_TAG into it. redis-streams is an upstream `redis:8.4.5-alpine`
# and has no tag of ours.
suite_image_tags MQ_HUB_IMAGE_TAG

# No suite_pki: mq-hub speaks plaintext HTTP only. main.go calls
# ListenAndServe, and middleware.PeerIdentityMiddleware — the one component
# that would need client certificates — is unwired dead code the service
# announces at startup with a `peer_identity_disabled` warning.
# tests/topology.spec.ts asserts that posture rather than assuming it.
suite_endpoint BASE_URL       "http://mq-hub:9500"
suite_endpoint OPS_ABSENT_URL "http://mq-hub:9110"

# The whole slice. mq-hub `depends_on: redis-streams (service_healthy)`, so
# compose orders them; naming both keeps the teardown symmetric and makes the
# dependency visible here rather than only in compose.staging.yaml.
suite_up redis-streams mq-hub

suite_test
