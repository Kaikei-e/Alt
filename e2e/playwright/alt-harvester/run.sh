#!/usr/bin/env bash
# e2e/playwright/alt-harvester/run.sh
#
# Brings up the alt-harvester slice of the alt-staging stack (Postgres + Atlas
# migrator + alt-backend-deps-stub + alt-data-hub + alt-harvester), runs the
# Playwright API suite inside the staging network so the `alt-harvester` DNS
# name resolves, and tears the stack down.
#
# alt-harvester is the job-runner third of the alt-backend split (ADR-000954).
# It owns the schedules `job.RegisterHarvesterJobs` wires up and serves no API;
# the only socket it opens is the operator listener on :9110, and that is not
# published to the host. The suite is therefore mostly negative — see
# tests/absent-surfaces.spec.ts and tests/topology.spec.ts for what that buys.
#
# ADR-000766 established the `e2e/<framework>/<svc>/run.sh` dispatch contract;
# alt-deploy's release-deploy.yaml probes this path. Everything generic about
# the lifecycle lives in `_lib/suite.sh` — see that file for the ordering
# contract and for why the install and the run are two separate containers.
#
# Environment overrides beyond the shared ones (see _lib/suite.sh):
#   OPS_URL         alt-harvester operator listener (/health + /metrics)
#   HARVESTER_HOST  DNS name the closed-port probes dial. It is a bare host,
#                   not a URL, because tests/topology.spec.ts builds one URL
#                   per port that must have nothing bound to it.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../../.." && pwd)"
# shellcheck source=../_lib/suite.sh
source "$ROOT/e2e/playwright/_lib/suite.sh"

suite_init alt-harvester

# alt-data-hub and backend-deps-stub are co-versioned with alt-harvester: same
# Go module, same Dockerfile, only `--build-arg BINARY=` differs
# (ADR-000954's three-binary split).
suite_image_tags ALT_HARVESTER_IMAGE_TAG ALT_BACKEND_DEPS_STUB_IMAGE_TAG ALT_DATA_HUB_IMAGE_TAG

# One CA, one server leaf for alt-data-hub and one client leaf named
# `alt-harvester` — which is what DATAHUB_ALLOWED_PEERS admits. Must run before
# suite_up: di.newDataHubClient reads the material at construction and panics
# without it, so certificates minted after the stack is up are certificates the
# harvester never saw.
suite_pki alt-data-hub alt-harvester

suite_endpoint OPS_URL        "http://alt-harvester:9110"
suite_endpoint HARVESTER_HOST "alt-harvester"

# The slice runs the real data plane, not a stub: after ADR-000954 Wave 3
# cmd/harvester holds no DB pool, so every one of its jobs reads and writes
# through DataHubService over mutual TLS. A slice without alt-data-hub is a
# harvester that panics at boot rather than one under test.
suite_up \
  alt-backend-db \
  alt-backend-db-migrator \
  alt-backend-deps-stub \
  alt-data-hub \
  alt-harvester

suite_test
