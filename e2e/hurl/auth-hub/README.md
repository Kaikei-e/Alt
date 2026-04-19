# auth-hub Hurl E2E suite

Black-box HTTP coverage for the Identity-Aware Proxy that fronts Ory
Kratos (Go 1.26 / Echo v4, port 8888). Boots a dedicated kratos-db
Postgres + Kratos v1.3.0 + auth-hub, seeds a deterministic identity via
Kratos Admin API, acquires a session via the self-service api-flow
login, and exercises every public and internal HTTP surface from inside
the `alt-staging` Docker network.

Convention follows ADR-000763 (Hurl framework), ADR-000766 (run.sh
dispatch), ADR-000781 (DB init via `/docker-entrypoint-initdb.d`), plus
ADR-000785–000788 that record the auth-hub-specific decisions.

## Run

```bash
docker build -t ghcr.io/kaikei-e/alt-auth-hub:ci -f auth-hub/Dockerfile auth-hub
docker pull ghcr.io/orange-opensource/hurl:7.1.0
IMAGE_TAG=ci GHCR_OWNER=kaikei-e \
  HURL_IMAGE=ghcr.io/orange-opensource/hurl:7.1.0 \
  bash e2e/hurl/auth-hub/run.sh
```

Reports land in `e2e/reports/auth-hub-<run_id>/{junit.xml,html/}`.

`KEEP_STACK=1` skips teardown for log inspection.

## Stack

`run.sh` activates the `auth-hub` Compose profile, which brings up:

- `auth-hub-db` — Postgres 16-alpine, ephemeral, fresh per run. Kratos
  owns the schema; `e2e/fixtures/auth-hub/db-init/01-no-op.sql` holds
  the `/docker-entrypoint-initdb.d/` slot for future seed SQL.
- `auth-hub-db-migrator` — `oryd/kratos:v1.3.0` running
  `kratos migrate sql -e --yes`, exits on success; Kratos proper's
  `service_completed_successfully` gate blocks startup until the
  schema is in place.
- `kratos` — FrontendAPI :4433 and AdminAPI :4434, launched with
  `--dev` so self-service flows accept plain HTTP inside the internal
  network. Config forked at `e2e/fixtures/auth-hub/kratos/kratos.yml`
  (the prod config at `/home/koko/Documents/dev/Alt/kratos/kratos.yml`
  hardcodes `curionoah.com` URLs + `.curionoah.com` cookie domain
  which break container-local flows).
- `auth-hub` — Echo v4 on :8888, mTLS off, HS256 secret shared with
  alt-backend via `e2e/fixtures/staging-secrets/alt_backend_token_secret.txt`.

Kratos's prod `entrypoint.sh` is reused (mounted read-only) so
`${KRATOS_COOKIE_SECRET}` / `${KRATOS_CIPHER_SECRET}` in the staging
`kratos.yml` are substituted from the mounted secret files before
Kratos boots.

Hurl runs inside `alt-staging` (the network is `internal: true`, which
silently drops host port publishes — joining the network is the only
portable way to reach the SUT).

## Session seeding

Kratos v1.3 does not expose an admin `POST /admin/sessions` endpoint to
mint cookies for a given identity, so 00-setup performs:

1. `POST /admin/identities` — create (or ensure) an identity with the
   deterministic email from `e2e/fixtures/auth-hub/test-identity-email.txt`.
2. `GET /self-service/login/api` — init a stateless api-flow (no CSRF).
3. `POST <flow.ui.action>` with method=`password` and the seeded
   credentials — Kratos returns a JSON body containing `session_token`.

Downstream scenarios send that token as
`Cookie: ory_kratos_session={{session_token}}`. Kratos whoami accepts
the token via cookie, `X-Session-Token`, or `Authorization: Bearer`
interchangeably, so the cookie form works without any browser-flow
handshake.

## Scenarios

Captures (`session_token`, `user_id`, `session_id`) live in the single
Hurl invocation that spans all files, so `--jobs 1` is load-bearing: a
parallel or split run would lose the captures.

| File | What it proves |
|---|---|
| `00-setup.hurl` | Health, Kratos readiness, identity seed, api-flow login, capture session. |
| `01-health.hurl` | `/health` → 200 `{"status":"healthy"}`. |
| `02-validate-happy.hurl` | `/validate` with cookie → 200 + 4 `X-Alt-*` headers (incl. HS256 JWT). |
| `03-validate-missing-cookie.hurl` | `/validate` without cookie → 401 (short-circuit). |
| `04-validate-bad-cookie.hurl` | `/validate` with invalid cookie → 401 (Kratos negative path). |
| `05-session-happy.hurl` | `/session` with cookie → 200 JSON (`ok`, `user`, `session`). |
| `06-session-unauth.hurl` | `/session` without cookie → 401. |
| `07-csrf-happy.hurl` | `POST /csrf` with cookie → 200 `{data:{csrf_token}}`. |
| `08-csrf-unauth.hurl` | `POST /csrf` without cookie → 401. |
| `09-internal-system-user-happy.hurl` | `/internal/system-user` with `X-Internal-Auth` → 200 `{user_id}`. |
| `10-internal-system-user-missing-auth.hurl` | No header → 401 ("missing internal auth header"). |
| `11-internal-system-user-bad-token.hurl` | Wrong value → 403 ("invalid internal auth"). |
| `12-validate-jwt-shape.hurl` | Decodes the HS256 JWT and asserts `iss`, `aud`, `sub`, `exp`. |

Rate-limit coverage (429 / `Retry-After`) is intentionally **out of the
Hurl suite** — it lives in `auth-hub/middleware/rate_limit_test.go`
where wall-clock timing isn't at the mercy of container cold-start.

## Security notes

- Secrets flow through `--secret` (`password`, `backend_token_secret`)
  so JUnit + HTML reports have them redacted (audit F-002).
- Cookie flag assertions (`HttpOnly`, `Secure`, `SameSite`) are
  deferred to the production Playwright suite; staging runs HTTP-only
  on an `internal: true` network and `--dev` relaxes Kratos's `Secure`
  requirement, so matching prod asserts here would be misleading.
- 401-only asserts on negative scenarios are deliberate: body phrasing
  belongs to Kratos and to `mapDomainError`, both of which change
  without notice, and account-enumeration coverage isn't what HTTP-level
  status-code checks actually prove.

## CI

`.github/workflows/e2e-hurl.yml` contains an `auth-hub` job mirroring
`acolyte-orchestrator`. It builds the auth-hub image locally as
`ghcr.io/kaikei-e/alt-auth-hub:ci`, pre-pulls the Hurl image, runs this
`run.sh` with `KEEP_STACK=1` so post-run log dumps can reach the
containers, uploads reports as the `auth-hub-hurl-reports` artifact,
and always tears the slice down.
