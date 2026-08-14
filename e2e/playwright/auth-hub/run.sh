#!/usr/bin/env bash
# e2e/playwright/auth-hub/run.sh
#
# Brings up the auth-hub slice of the alt-staging stack (kratos-db Postgres +
# Kratos migrator + Kratos + auth-hub), runs the Playwright API suite inside
# the staging network so the `auth-hub` and `kratos` DNS names resolve, and
# tears the stack down.
#
# ADR-000766 established the `e2e/<framework>/<svc>/run.sh` dispatch contract;
# alt-deploy's release-deploy.yaml probes this path. Everything generic about
# the lifecycle lives in `_lib/suite.sh` — see that file for the ordering
# contract and for why the dependency install and the test run are two
# separate containers (the staging network is `internal: true`, so no egress).
#
# The slice needs `ghcr.io/$GHCR_OWNER/alt-auth-hub:$IMAGE_TAG` to exist; CI
# builds it locally and tags it `ci` before calling this. Kratos and Postgres
# come from public images the daemon pulls outside the internal network.
#
# The Hurl suite this replaces needed three extra things that are gone:
#
#   * a standalone `00-setup.hurl` invocation with `--report-json`, plus `jq`
#     on the host to parse `kratos_session_cookie` / `user_id` / `session_id`
#     out of the report and re-inject them as `--variable`. Sessions are now
#     seeded per worker inside the suite (src/fixtures.ts).
#   * `--jobs 1`, which existed because those captures were file-scoped and
#     because the /internal limiter is 10 req/min burst 3. Nothing is shared
#     between tests now, and each test carries its own synthetic client
#     address, so the suite runs `fullyParallel`.
#   * `--secret` redaction for the session cookie and the HS256 signing key.
#     That key arrives here as a *path*, is read inside the test container, and
#     never reaches a compose slice, `docker inspect`, or a CI env dump;
#     `suite.sh`'s `redact_secrets` scrubs the log dump on failure.
#     INTERNAL_AUTH_SECRET below is the one exception: it is exported as a
#     value, so it does reach the container env and a CI env dump, and
#     `redact_secrets` (JWS and PEM forms only) will not scrub it. That is
#     acceptable only because the literal is already committed in plaintext in
#     compose.staging.yaml — a path would buy nothing. Never give a real secret
#     this treatment.
#
# Environment overrides beyond the shared ones (see _lib/suite.sh):
#   BASE_URL           auth-hub REST URL as seen from the test container
#   KRATOS_PUBLIC_URL  Kratos FrontendAPI — where sessions are minted
#   KRATOS_ADMIN_URL   Kratos AdminAPI — identity seed / read / session revoke
#   MTLS_URL           where auth-hub's optional mTLS listener would bind
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../../.." && pwd)"
# shellcheck source=../_lib/suite.sh
source "$ROOT/e2e/playwright/_lib/suite.sh"

suite_init auth-hub

# auth-hub is the only locally-built image in this slice; Kratos, the Kratos
# migrator and Postgres are upstream images pinned in compose.staging.yaml.
suite_image_tags AUTH_HUB_IMAGE_TAG

# No `suite_pki`: compose.staging.yaml sets MTLS_LISTEN=false, so auth-hub
# starts no TLS listener and there is no mutual-TLS material for the slice to
# mount. tests/topology.spec.ts asserts that absence rather than assuming it.

suite_endpoint BASE_URL          "http://auth-hub:8888"
suite_endpoint KRATOS_PUBLIC_URL "http://kratos:4433"
suite_endpoint KRATOS_ADMIN_URL  "http://kratos:4434"
suite_endpoint MTLS_URL          "https://auth-hub:9443"

# Secrets and fixtures arrive as paths. The repo is bind-mounted into the test
# container at the same absolute path, so these resolve unchanged inside it.
#
# alt_backend_token_secret is the HS256 signing key and only that; the suite
# needs it to verify the JWT signature the way alt-backend will. It no longer
# opens /internal — main.go keys that group on INTERNAL_AUTH_SECRET, and
# auth-hub refuses to start when the two are the same value.
suite_endpoint BACKEND_TOKEN_SECRET_FILE \
  "$ROOT/e2e/fixtures/staging-secrets/alt_backend_token_secret.txt"
suite_endpoint IDENTITY_EMAIL_FILE    "$ROOT/e2e/fixtures/auth-hub/test-identity-email.txt"
suite_endpoint IDENTITY_PASSWORD_FILE "$ROOT/e2e/fixtures/auth-hub/test-identity-password.txt"

# The /internal shared bearer — the one secret that arrives as a value rather
# than a path, because compose.staging.yaml sets it inline on both auth-hub and
# alt-data-hub. The default here has to be that same literal, or every
# /internal call in the suite gets 403 instead of 200.
suite_endpoint INTERNAL_AUTH_SECRET \
  "staging-fixture-internal-auth-secret-not-a-real-key"

# Mirrors of the three BACKEND_TOKEN_* values compose.staging.yaml configures
# auth-hub with. They live here rather than as constants in the suite so that
# changing the compose slice and forgetting the suite fails as a named claim
# mismatch instead of passing against last year's contract — these are the
# claims alt-backend verifies, so a drift is a cross-service outage.
suite_endpoint JWT_ISSUER      "auth-hub"
suite_endpoint JWT_AUDIENCE    "alt-backend"
suite_endpoint JWT_TTL_SECONDS "1800"

# The same four services the Hurl run.sh brought up. Ordering is enforced by
# compose itself: the migrator gates on auth-hub-db being healthy, Kratos gates
# on the migrator's `service_completed_successfully`, auth-hub gates on Kratos
# being healthy.
suite_up \
  auth-hub-db \
  auth-hub-db-migrator \
  kratos \
  auth-hub

suite_test
