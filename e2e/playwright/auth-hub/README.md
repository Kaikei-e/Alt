# auth-hub Playwright E2E suite

Black-box HTTP coverage for the Identity-Aware Proxy that bridges nginx's
`auth_request` to Ory Kratos (Go 1.26 / Echo v4, port 8888). Replaces
`e2e/hurl/auth-hub/`.

Every spec is HTTP-only — no spec touches `page`, `browser` or `context` — so
Playwright never launches a browser and the runner needs no browser binaries.

## Run

```bash
bash e2e/playwright/auth-hub/run.sh              # stack up, tests, teardown
KEEP_STACK=1 bash e2e/playwright/auth-hub/run.sh # leave the stack up to poke at
PW_GREP='@smoke' bash e2e/playwright/auth-hub/run.sh
```

`run.sh` is the only supported entry point. The slice needs
`ghcr.io/$GHCR_OWNER/alt-auth-hub:$IMAGE_TAG` to exist (CI builds it and tags
it `ci`). Reports land in `e2e/reports/auth-hub-<run_id>/`.

Checking your work without a Docker daemon:

```bash
cd e2e/playwright && npx tsc --noEmit -p auth-hub/tsconfig.json
cd auth-hub && BASE_URL=… KRATOS_PUBLIC_URL=… … npx playwright test --list
```

## Stack

`run.sh` brings up the `auth-hub` compose profile from
`compose/compose.staging.yaml`, unchanged from what the Hurl suite used:

| Service | What it is |
|---|---|
| `auth-hub-db` | `postgres:16-alpine`, ephemeral. Kratos owns the schema. |
| `auth-hub-db-migrator` | `oryd/kratos:v1.3.0` running `kratos migrate sql`; exits 0 and gates Kratos. |
| `kratos` | FrontendAPI :4433 + AdminAPI :4434, `--dev` so self-service flows accept plain HTTP inside the internal network. |
| `auth-hub` | Echo v4 on :8888. `MTLS_LISTEN=false`. HS256 secret shared with alt-backend. |

The staging network is `internal: true`, which silently drops host port
publishes — the suite runs *inside* the network, which is the only portable way
to reach the SUT. There is no `suite_pki` call: with `MTLS_LISTEN=false` there
is no TLS listener to mint material for, and `tests/topology.spec.ts` asserts
that absence rather than assuming it.

## What this suite covers

| Spec | Surface |
|---|---|
| `health.spec.ts` | `/health` body, no-auth, no-rate-limit, verb. |
| `validate.spec.ts` | `/validate` — the nginx `auth_request` target: identity headers, empty body, cache-hit path, and five rejection branches. |
| `backend-token.spec.ts` | The `X-Alt-Backend-Token` HS256 JWT: signature, every claim, TTL, session binding. |
| `session.spec.ts` | `/session` — the SPA's bootstrap envelope, dates, role, cross-session isolation. |
| `csrf.spec.ts` | `POST /csrf` — token format, MAC width, session binding, three rejection branches. |
| `internal.spec.ts` | `/internal/system-user` — the shared-secret boundary, 401-vs-403, identity provenance. |
| `rate-limit.spec.ts` | The four per-IP limiters: burst, `Retry-After`, per-IP keying, `/health` exemption. |
| `security-headers.spec.ts` | The seven headers `middleware/SecurityHeaders()` sets, on 200 / 401 / 404. |
| `topology.spec.ts` | The route table, the Kratos surfaces auth-hub must not proxy, the unbound mTLS listener. |

Tags follow the fleet convention: `@smoke` (would catch a broken deployment),
`@contract` (a shape another service or the SPA depends on), `@authz` (a
negative). No spec here is slow enough to warrant `@slow`.

## Retired Hurl scenarios

| Hurl file | Where it landed |
|---|---|
| `00-setup.hurl` | Split in two. The two readiness waits became `setup/global-setup.ts`; the identity seed + login became the `session` / `mintSession` fixtures in `src/fixtures.ts`, which run per worker instead of once for the whole suite. |
| `01-health.hurl` | `health.spec.ts` › *GET /health reports healthy*. The comment about the legacy handler's extra `service` field is now enforced: `healthSchema` is `strict()`. |
| `02-validate-happy.hurl` | `validate.spec.ts` › *returns the four identity headers*. All four asserts ported; the JWS regex assert is superseded by `backend-token.spec.ts`. |
| `03-validate-missing-cookie.hurl` | `validate.spec.ts` › */validate without a cookie → 401* (+ the new *leaks no identity headers*). |
| `04-validate-bad-cookie.hurl` | `validate.spec.ts` › */validate with an unknown session value → 401*. |
| `05-session-happy.hurl` | `session.spec.ts` › *returns the full session envelope* + *identifies the session it was given* + *sets the backend token header*. `jsonpath "$.session.id" exists` is replaced by `sessionResponseSchema`. |
| `06-session-unauth.hurl` | `session.spec.ts` › */session without a cookie → 401*. |
| `07-csrf-happy.hurl` | `csrf.spec.ts` › *POST /csrf mints a token* + *the token is a fresh timestamp and a full-width MAC*. The `{16,}` character-class regex became the real format plus a digest-length check. |
| `08-csrf-unauth.hurl` | `csrf.spec.ts` › */csrf with no Cookie header at all → 401 with the bare error envelope*. |
| `09-internal-system-user-happy.hurl` | `internal.spec.ts` › *returns a resolvable identity*. The `== {{user_id}}` equality is **replaced**, not dropped — see below. |
| `10-internal-system-user-missing-auth.hurl` | `internal.spec.ts` › *no X-Internal-Auth header → 401*. |
| `11-internal-system-user-bad-token.hurl` | `internal.spec.ts` › *a wrong secret of the same length → 403*. |
| `12-validate-jwt-shape.hurl` | `backend-token.spec.ts`, all six tests. |

### The one assertion that changed meaning

`09-internal-system-user-happy.hurl` asserted `$.user_id == "{{user_id}}"`.
That held only because the whole Hurl run had exactly one Kratos identity, so
"the first identity Kratos returns" and "the identity we seeded" were
necessarily the same row. The handler itself does not promise that — it asks
Kratos for `/admin/identities?page_size=1` and returns whatever comes back.

This suite seeds one identity per worker, so the old equality would be an
assertion about test ordering. The replacement asserts the order-independent
fact instead: the id auth-hub reports resolves to a real Kratos identity via
`GET /admin/identities/{id}`. A stale cache entry, a config default or a
truncated string all fail that; none of them would have failed a UUID regex.

## How the ordering was broken

The Hurl suite ran with `--jobs 1` and a `run.sh` that parsed
`kratos_session_cookie` / `user_id` / `session_id` out of a `--report-json`
and injected them into every later scenario as `--variable`. Scenario 12 could
not run without scenario 0.

Two changes remove that entirely:

1. **Every test seeds what it reads.** Each worker mints its own Kratos
   identity and session (`local+<runId>-w<n>-<rand>@domain`); any test that
   needs a second one — or that destroys the one it uses, like the revoked-
   session test — calls `mintSession()`. Isolation is by naming, not teardown.
2. **Every test gets its own client address.** All four rate limiters
   (`middleware/rate_limit.go`) key on `c.RealIP()`, and `main.go` configures
   `echo.ExtractIPFromXFFHeader()`. Each test carries a synthetic
   `X-Forwarded-For` in RFC 6598 space (100.64.0.0/10 — outside every range
   Echo trusts, so it survives the forwarded-chain walk), which gives it a
   private token bucket in each limiter. That is what makes the /internal
   limiter — 10 req/min, burst 3, not env-tunable, the original reason for
   `--jobs 1` — a non-issue, and it is why this suite needs no `workers: 1`
   project. `rate-limit.spec.ts` › *the limiters are keyed per client address*
   asserts that isolation directly, so if the assumption ever breaks it fails
   as one named test rather than as scattered 429s.

Nothing sleeps. The Hurl suite's `--delay` / `retry-interval: 6000` waits
existed only to wait out the /internal limiter; with per-test buckets there is
nothing to wait for.

## New coverage, and where it came from

Every addition is grounded in a file that was read, named in the spec's own
comments:

- **The JWT is actually verified** (`backend-token.spec.ts`, from
  `infrastructure/token/jwt.go` + the shared `alt_backend_token_secret`
  mount). Hurl could only substring-match the decoded payload and checked no
  signature at all — a token signed with the wrong key passed every one of its
  asserts. Also new: exact `aud` list equality, `tenant_id`, `sid`, `role`,
  `alg`/`typ`, and `exp - iat == BACKEND_TOKEN_TTL`.
- **The security header set** (`security-headers.spec.ts`, from
  `middleware/security_headers.go` + `main.go:117`). Nothing looked at any of
  the seven. The interesting assertions are that they survive the 401 and 404
  paths, which is what a reordering of the middleware chain breaks first.
- **The rate limiters** (`rate-limit.spec.ts`, from `middleware/rate_limit.go`
  + `main.go:158-187`). Explicitly out of scope for Hurl because a scenario
  file cannot give itself a private client address. The assertions are about
  bucket *capacity*, not elapsed time, so nothing here is timing-fragile.
- **The route table and the unbound mTLS listener** (`topology.spec.ts`, from
  `main.go:176-230` + `MTLS_LISTEN=false` in compose.staging.yaml). The
  `expectConnectionRefused` assertion is CLAUDE.md rule 8's "disabled must be
  observable" at the transport layer; `tlsutil.LoadServerConfig` defaults
  `ClientAuth` to `NoClientCert`, so a listener that came up without its CA
  wired would serve `/internal/system-user` to any TLS client.
- **Rejection branches nobody covered**: `/session` and `/csrf` with a *bad*
  cookie (only the missing-cookie short-circuit was tested), a Cookie header
  carrying unrelated cookies, an empty `X-Internal-Auth`, a length-mismatched
  wrong secret, a genuinely revoked Kratos session, and — the one that matters
  most — that no rejection emits an `X-Alt-*` identity header.
- **Cross-session isolation** (`session.spec.ts`, `csrf.spec.ts`,
  `backend-token.spec.ts`). `infrastructure/cache/session_cache.go` is keyed on
  the cookie value; a cache that returned the wrong user presents as a clean
  200 and was untestable with a single seeded identity.
- **Verb negatives** on all five routes, which distinguish "the path exists and
  refuses this method" (405) from "the path does not exist" (404) — the same
  discrimination `_shared/connect.ts`'s `expectProcedureMounted` makes for
  Connect services. auth-hub mounts no Connect mux, so this is where the
  equivalent claim lives.

## Notes

- The `test-tenant-id.txt` fixture is not read by this suite. The Hurl `run.sh`
  passed it as `--variable tenant_id`, but no `.hurl` file ever referenced it;
  the tenant assertions are all `tenantId == userId`, which is what
  `usecase.GetSession` and `jwt.go` actually do in single-tenant mode.
- Cookie flag assertions (`HttpOnly`, `Secure`, `SameSite`) are still out of
  scope, for the reason the Hurl README gave: staging runs HTTP-only on an
  `internal: true` network and Kratos `--dev` relaxes the `Secure` requirement,
  so matching prod would be misleading. auth-hub sets no cookies of its own.
- Error-body *phrasing* is deliberately not asserted, per the Hurl suite's
  position that it belongs to Kratos and to `mapDomainError`. The error
  *envelope shape* is asserted, because `/csrf`'s missing-cookie path answers
  `{"error": …}` while every other 401 answers `{"message": …}`, and the SPA
  branches on that.
