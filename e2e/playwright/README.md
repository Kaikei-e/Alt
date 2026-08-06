# Playwright E2E suites

Every Alt microservice's end-to-end suite lives here, one directory per
service. All of them are **HTTP-only**: no spec touches `page`, `browser` or
`context`, so Playwright never launches a browser and the runner needs no
browser binaries. That is what lets CI use a `node:22-bookworm-slim` image
(~150MB) with `PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD=1` instead of the ~1.7GB
Playwright image.

The browser-driven suite for the SPA is the one exception. Its specs stay where
they are (`alt-frontend-sv/tests/e2e/`) with their own config, because they
need a real browser engine and a self-contained stack rather than a compose
slice; `alt-frontend-sv/` here is only the dispatch shim that satisfies
ADR-000766's `e2e/<framework>/<svc>/run.sh` contract. It is deliberately absent
from `suites.yaml` — it builds no images and runs under the Playwright browser
image, so none of the machinery below applies to it.

## Layout

```
e2e/playwright/
  package.json          one manifest and one lockfile for the whole fleet
  tsconfig.base.json    the compiler settings every suite extends
  _shared/              TypeScript helpers imported by every suite
  _lib/                 the shell lifecycle every run.sh is built from
  <service>/
    playwright.config.ts   defineApiSuite({ service, workers, ... })
    tsconfig.json          extends ../tsconfig.base.json
    run.sh                 the dispatch entry point (ADR-000766)
    setup/global-setup.ts  readiness gate — replaces the old 00-setup scenario
    src/env.ts             the endpoints this suite reads from the environment
    src/fixtures.ts        worker-scoped clients and seeded data
    src/schemas.ts         the response shapes this service promises
    tests/*.spec.ts
```

### Why one install root

`e2e/playwright/package.json` is the fleet's only manifest, and Node resolves
`node_modules` by walking up from `e2e/playwright/<service>/`. One lockfile
means twelve suites cannot drift onto different Playwright or zod versions, and
a CI job that runs more than one suite installs once.

npm **workspaces** were evaluated and rejected: `npm ci` from a workspace
subdirectory installs into the repository root and removes the existing
`node_modules` first. With the repo bind-mounted at a single path, parallel
suites would each delete and rebuild the same tree underneath one another.

`_shared` is imported by relative path (`../../_shared/http.js`) rather than
being a package. A `file:` dependency would need `install-links=true` in an
`.npmrc` to install its transitive dependencies at all — a silent failure mode
for no gain, since `_shared` has no dependencies of its own beyond what every
suite already has.

## Running a suite

```bash
# Everything: stack up, tests, teardown.
bash e2e/playwright/<service>/run.sh

# Keep the stack up to poke at it after a failure.
KEEP_STACK=1 bash e2e/playwright/<service>/run.sh

# A subset.
PW_GREP='@smoke' bash e2e/playwright/<service>/run.sh
```

`run.sh` is the only supported entry point. It renders an isolated compose
slice, mints any mutual-TLS material the slice needs, brings the stack up with
retries, installs dependencies on the default bridge (the staging network is
`internal: true` — no egress), runs the suite *inside* that network so service
DNS names resolve, and tears everything down. See `_lib/suite.sh`.

Running `npx playwright test` by hand works if you export the endpoints
yourself, and fails with a named error if you forget one — the suites have no
default URLs on purpose (CLAUDE.md rule 9).

## Building the images a suite runs against

`run.sh` brings the stack up but assumes the images already exist, so running
it straight after a source change silently tests the *previous* image — the
failure mode CLAUDE.md rule 3 warns about, moved into the E2E harness. Build
first:

```bash
python3 e2e/playwright/_lib/build-images.py <service>            # build
python3 e2e/playwright/_lib/build-images.py <service> --dry-run  # just show me
```

`suites.yaml` says *which* images a suite needs; `services.yaml` — the
repository's existing registry — says *how* to build each one, so the
alt-backend trio's `--build-arg BINARY=` is declared once and CI and your
laptop run the same command.

## CI

`.github/workflows/e2e-playwright.yml` runs every suite on every PR and every
push to main, with no path filtering: skipping by touched path lets a
shared-wiring regression (a compose change, a fixture, an upstream Go package,
proto) land green because it matched no service path.

```
plan ──┬── single    (matrix over the unsharded suites: build + run)   ──┐
       │                                                                │
       └── build ──> sharded (matrix over suite × shard)              ──┴─> merge
```

`plan` derives its matrices from `suites.yaml`, so a suite's shard count lives
next to the suite rather than in the workflow where it would drift. Sharding is
a last resort: eleven suites already give eleven parallel jobs, and each shard
pays the full compose-slice startup cost, so splitting a suite that finishes in
four minutes makes it slower. Add `shards:` only past roughly eight minutes.

Every job writes a **blob** report; the `merge` job folds them all into one
HTML report plus one set of GitHub annotations. Each suite's `tag: '@<service>'`
is what keeps twelve services' blobs from colliding and what labels each test
in the merged report with the service it came from.

## Writing specs

### The rules that are not negotiable

1. **Every test seeds what it reads.** `fullyParallel: true` distributes
   individual tests across workers and shards; no test may assume another ran
   first. Isolation comes from *naming* (`workerToken`, `testToken` in
   `_shared/ids.ts`), not from teardown — the stack is destroyed per dispatch
   anyway, and a teardown still races a sibling worker's `list`.

2. **Assert the whole envelope, not a spot check.** The Hurl suites these
   replaced were full of `jsonpath "$" exists`, which passes for `null`, `[]`,
   `{}` and `{"error": …}` alike. Use a zod schema from `src/schemas.ts` via
   `expectJsonStatus`, so a handler that changes shape fails here instead of
   silently satisfying the check.

3. **Never sleep for a side effect.** Redis Streams consumers, event
   projectors and Meilisearch indexing are eventually consistent by
   construction. Use `eventually` / `eventuallyValue` from `_shared/eventual.ts`,
   which assert the actual condition. A fast stack then finishes in one
   interval and a slow one still passes.

4. **A status band needs a reason.** `expectStatusIn(res, [200, 404])` says
   *both answers are correct*. Every use must carry a comment naming why each
   member is in the set. Without one it is a test that cannot fail, which is
   worse than no test because it reports green.

5. **Distinguish 404 from everything else.** A handler whose DI dependency came
   back nil and skipped registration is invisible to unit tests and to any
   "not 2xx" assertion — the procedure simply stops existing. `expectProcedureMounted`
   in `_shared/connect.ts` is CLAUDE.md rule 8 pushed out to the E2E boundary.

### Tagging

Tags are matched by `--grep` against the full title; there is no `--tag` flag.

| Tag | Meaning |
|-----|---------|
| `@smoke` | Would catch a completely broken deployment. Cheap, no seeding. |
| `@contract` | Asserts a response shape another service or the SPA depends on. |
| `@authz` | A negative: anonymous, wrong tenant, wrong role, wrong listener. |
| `@slow` | Seconds, not milliseconds — a warmup, a model call, a backfill. |

```ts
test("rejects an anonymous caller", { tag: "@authz" }, async ({ anon }) => { … });
```

### Helpers

| Module | What it is for |
|---|---|
| `_shared/env.ts` | `requiredEnv`, `requiredSecretFile` — fail-fast config, no defaults |
| `_shared/http.ts` | Status/schema/header assertions that put the body in the failure message |
| `_shared/schemas.ts` | `uuidSchema`, `timestampSchema`, the error envelopes each stack emits |
| `_shared/connect.ts` | Connect-RPC: `callUnary`, `expectUnaryError`, `expectProcedureMounted` |
| `_shared/eventual.ts` | `eventually`, `eventuallyValue` — convergence, not sleeps |
| `_shared/net.ts` | `expectConnectionRefused`, `expectTlsHandshakeRejected` — proving a port is closed |
| `_shared/ids.ts` | `workerToken`, `testToken`, `uuid` — parallel-safe naming |
| `_shared/readiness.ts` | `waitForReady` probes for `globalSetup` |
| `_shared/config.ts` | `defineApiSuite` — the one config the fleet shares |

## Checking your work without Docker

```bash
cd e2e/playwright && npm ci
npx tsc --noEmit -p <service>/tsconfig.json     # types, including _shared
cd <service> && npx playwright test --list      # every spec loads, no duplicate titles
```

Both run without a Docker daemon and catch most of what goes wrong in a
rewrite. They do not, of course, tell you the assertions are *true* — only CI,
or `run.sh` on a machine with a daemon, does that.
