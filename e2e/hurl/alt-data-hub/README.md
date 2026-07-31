# alt-data-hub Hurl E2E suite

End-to-end scenarios for the data plane of the alt-backend 3-binary split.
alt-data-hub serves `BackendInternalService` and the `/v1/internal/*` REST
routes — the surfaces that authenticate a *service* rather than a user —
over mTLS on `:9443`, with no host port published at all.

The admin Connect services (`KnowledgeHomeAdminService`,
`AdminMonitorService`) did **not** move here; they stay on alt-backend's
loopback-bound operator listener. See `di/container_datahub.go`: "It builds
no crawler, no search indexer, no image pipeline and no admin surface."

**Status: wired.** The `alt-data-hub` profile exists in
`compose/compose.staging.yaml` and the sibling job exists in
`.github/workflows/e2e-hurl.yml`. The suite was written outside-in ahead of
the split; the list under "What has to exist for this to go green" is now a
description of what is there rather than a to-do.

## The contract this suite exists to hold

| Claim | Where it is asserted |
|---|---|
| `BackendInternalService` answers here, over mTLS | `01-backend-internal-service.hurl` |
| `DataHubService` answers here, over mTLS | `04-datahub-service.hurl` |
| …and *only* here | `../alt-backend/03-topology-internal-surface-absent.hurl`, `../alt-harvester/01-operator-surface-only.hurl` |
| `/v1/internal/*` answers here, over mTLS | `02-internal-rest-routes.hurl` |
| `:9110` serves health + metrics in plaintext, and nothing else | `03-ops-listener.hurl` |
| An anonymous TLS client is refused | `run.sh` → `assert_transport_refused` |
| A valid-chain / wrong-identity peer is refused | `run.sh` → `assert_transport_refused` |
| No listener reachable on `:9000` / `:9101` / `:9102` | `run.sh` → `assert_transport_refused` |

The positive and negative halves have to be read together. On its own,
`01-backend-internal-service.hurl` is satisfied by a service that serves
everything to everyone, and the 404s in the sibling suites are satisfied by
a service that serves nothing at all. The pair is the claim.

### Two namespaces, on purpose

`01` and `04` assert the same procedures under two different paths, and both
must pass. ADR-000954 D7 replaces `services.backend.v1.BackendInternalService`
with `alt.datahub.v1.DataHubService`, but Wave 2-B moves the seven peers one
PR at a time, so between the first and last of those PRs some callers are on
each path. Serving one and not the other would break whichever half had not
been migrated yet.

The duplication is scheduled to end, not to persist. Wave 2-C deletes the
legacy proto, and at that point `01` inverts — from "these procedures answer"
to "these procedures 404" — in the same commit. Until then, a `04` that passes
while `01` fails means a peer that has not been migrated has just lost its
data plane.

`:9102` is in the refused list on purpose. It is the port alt-backend's
operator listener uses, and the tempting move during the split was to give
alt-data-hub one too — "it already has a plaintext port for the probe". It
does not: `cmd/datahub` opens exactly two sockets, the mTLS `:9443` and the
ops `:9110`, and `di/container_datahub.go` builds no admin surface for a
third to serve. The probe is what makes that a tested claim rather than an
intention, and `03-ops-listener.hurl` covers the other half — that `:9110`
answers nothing but `/health` and `/metrics`.

## Running

```bash
# Full suite (brings up the slice, mints a throwaway PKI, runs Hurl, tears down)
bash e2e/hurl/alt-data-hub/run.sh

# Debug: leave the stack (and its certificates) up for inspection
KEEP_STACK=1 bash e2e/hurl/alt-data-hub/run.sh

# Reports (JUnit + HTML) land under:
ls e2e/reports/alt-data-hub-*/
```

The script expects the alt-data-hub image to already exist as
`ghcr.io/${GHCR_OWNER:-kaikei-e}/alt-alt-data-hub:${IMAGE_TAG:-ci}`, built
from `alt-backend/Dockerfile.backend` with `--build-arg BINARY=datahub`
(see `services.yaml`).

## Authentication

There is no JWT here. The client certificate *is* the caller's identity —
that is the whole reason these surfaces had to leave a listener the public
NIC can reach. `run.sh` mints a throwaway PKI per run under
`e2e/fixtures/alt-data-hub/pki/`. It lives under `fixtures/` because compose
has to bind-mount it; it is gitignored because that puts private keys one
`git add e2e/` away from a commit. The exit trap wipes it, except under
`KEEP_STACK=1` — the files are mounted into a container that is still
running, and deleting them would break the stack the flag exists to inspect.

| File | Role |
|---|---|
| `ca.pem` / `ca-key.pem` | one-day root, trusted by both ends |
| `ca-bundle.pem` | copy of `ca.pem` under the production trust-bundle name |
| `alt-data-hub.pem` / `-key.pem` | server leaf, `CN`/`DNS SAN` = the compose service name |
| `svc-cert.pem` / `svc-key.pem` | copies of the server leaf under the production names |
| `pre-processor.pem` | client leaf on the allowlist (`$ALLOWED_PEER`) |
| `rogue-peer.pem` | client leaf *not* on the allowlist (`$DENIED_PEER`) |

The duplicate names exist so the staging slice can mount this one directory
at `/certs` and `/trust` and reuse the production environment values
verbatim (`DATAHUB_TLS_CERT_FILE=/certs/svc-cert.pem`,
`DATAHUB_TLS_KEY_FILE=/certs/svc-key.pem`,
`DATAHUB_TLS_CA_FILE=/trust/ca-bundle.pem`). Staging TLS wiring that differs
from production is TLS wiring that can rot without anyone noticing.

`--cacert` / `--cert` / `--key` are passed on the Hurl command line rather
than in per-entry `[Options]`, so a scenario file cannot silently fall back
to an anonymous connection and still report green.

## Why the negatives are shell and not Hurl

Hurl cannot express "the connection must not be established": an entry that
fails to reach the server is a run failure, not a passing assertion. The
polarity is inverted in `_lib/probe-transport-refused.hurl` (which passes on
*any* response) plus `_lib/assert-transport-refused.sh` (which requires Hurl
exit code **3** — runtime error — so a parse error or an assert failure
cannot masquerade as a refusal). Both live under `_lib/` because
`alt-harvester/run.sh` needs the same trick.

Note that `require_and_verify` and the peer allowlist both reject during the
TLS handshake — `tlsutil.WithAllowedPeers` installs a `VerifyConnection`
callback, not an interceptor — so neither negative produces a status code to
assert on.

## What this suite does not cover

- **Zero published ports.** That is a compose fact, not a runtime one:
  `render-slice.sh` strips every `ports:` key before the staging slice
  boots, so an E2E assertion here would pass vacuously. It is enforced
  instead by `ZERO_PUBLISH_SERVICES` in `scripts/compose-port-audit.py`.
- **Response field shapes.** The internal handler uses Connect's stock
  protojson codec, so zero-valued scalars are absent from the wire and
  asserting on them would encode a codec detail. Field contracts belong to
  the Pact consumer tests under `alt-backend/pacts/`.

## What the suite depends on (all present)

1. An `alt-data-hub` service on the `alt-data-hub` profile in
   `compose/compose.staging.yaml`, sharing `alt-backend-db` /
   `alt-backend-db-migrator` / `alt-backend-deps-stub` with the
   `alt-backend` profile, with:
   - `build.args.BINARY: datahub`
   - the mTLS listener on `:9443`, client auth `require_and_verify`, and an
     allowlist containing `pre-processor` but **not** `rogue-peer`
   - `../e2e/fixtures/alt-data-hub/pki` mounted at both `/certs` and
     `/trust` (read-only)
   - the ops listener on `:9110` (`OPS_LISTEN`)
   - **no** listener on `:9000` / `:9101` / `:9102`
2. A healthcheck that works without a shell — the runtime image has none;
   `compose/core.yaml` uses `["CMD", "/app-entry", "healthcheck"]`.
3. A sibling job in `.github/workflows/e2e-hurl.yml` following the existing
   per-service job shape: build with `--build-arg BINARY=datahub`, then run
   this script with `STAGING_PROJECT_NAME: alt-staging-alt-data-hub`.
