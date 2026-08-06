# E2E Playbooks (Phase 0)

Where the outermost tests live and how to write them. Load the section matching the
scope Phase 0's decision tree selected.

- [Playwright — browser user journeys](#playwright--browser-user-journeys)
- [Playwright — API and service-boundary scenarios](#playwright--api-and-service-boundary-scenarios)

Both layers are Playwright now. The Hurl suites were retired once every service had an
API suite under `e2e/playwright/<service>/`; ADR-000766's dispatch contract
(`bash e2e/<framework>/<svc>/run.sh`) is unchanged, only the framework directory moved.

## Playwright — browser user journeys

- Config: `alt-frontend-sv/playwright.config.ts`
- Specs: `alt-frontend-sv/tests/e2e/{auth,desktop,mobile,visual,integration,a11y}/*.spec.ts`
- Page Object base: `alt-frontend-sv/tests/e2e/pages/BasePage.ts` — extend it rather than reinventing
- Fixtures / factories: `alt-frontend-sv/tests/e2e/fixtures/`
- Global setup (MSW, Kratos session): `alt-frontend-sv/tests/e2e/global-setup.ts`

```bash
cd alt-frontend-sv && bun run test:e2e:integration   # integration project (default)
cd alt-frontend-sv && bun run test:e2e:ui            # UI mode for debugging one spec
cd alt-frontend-sv && bun run test:e2e               # full suite (build + all projects)
```

Per [playwright.dev best practices](https://playwright.dev/docs/best-practices):

- **Locators**: `getByRole` / `getByLabel` / `getByText` / `getByTestId`. User-facing locators survive
  DOM refactors, which is why CSS and XPath selectors are avoided
- **Web-first async assertions**: `await expect(locator).toBeVisible()`. The synchronous form
  `expect(await locator.isVisible()).toBe(true)` reads the state once and skips auto-waiting, so it
  flakes on anything that renders asynchronously
- Trust auto-waiting — a manual `waitForTimeout` or retry loop either slows the suite or hides a race
- One `test()` = one user journey; isolation comes from a fresh browser context per test
- Group with `test.describe()`; share setup with `beforeEach` rather than global mutable state
- Mock third-party dependencies through the MSW server wired in `global-setup.ts`
- Seed Kratos sessions and backend fixtures so specs do not depend on whatever is in the DB

## Playwright — API and service-boundary scenarios

HTTP-only: no spec touches `page`, `browser` or `context`, so Playwright never launches a browser.

- Specs: `e2e/playwright/<service>/tests/*.spec.ts`, grouped by surface
- Config: `e2e/playwright/<service>/playwright.config.ts` — one call to `defineApiSuite`
- Runner: `e2e/playwright/<service>/run.sh` — renders an isolated `compose.staging.yaml` slice, mints
  any mTLS material, brings the stack up with retries, runs the suite *inside* the staging network,
  tears it down. Reports land in `e2e/reports/<service>-<run_id>/`
- Shared helpers: `e2e/playwright/_shared/` — read these before writing a spec
- CI: `.github/workflows/e2e-playwright.yml`

**Read `e2e/playwright/README.md` first.** Its "rules that are not negotiable" section is binding.

```bash
python3 e2e/playwright/_lib/build-images.py <service>   # build first, or you test the old image
bash e2e/playwright/<service>/run.sh
KEEP_STACK=1 bash e2e/playwright/<service>/run.sh       # leave the stack up to poke at it
PW_GREP='@smoke' bash e2e/playwright/<service>/run.sh   # a subset

bash e2e/playwright/_lib/check-suites.sh                # typecheck + load every suite, no Docker
```

The idioms that matter, and the failure each one prevents:

- **Every endpoint comes from `src/env.ts` via `requiredEnv`, which has no defaults.** A suite
  pointed at a host that does not exist must fail by name, not report "connection refused" on
  scenario 1 — and a suite pointed at the *wrong* host must not report green (CLAUDE.md rule 9)
- **Assert the whole envelope with a zod schema**, not one field at a time. The Hurl suites were full
  of `jsonpath "$" exists`, which passes for `null`, `[]`, `{}` and `{"error": …}` alike
- **Every test seeds what it reads**, under a `workerToken` / `testToken` name nothing else uses.
  `fullyParallel: true` distributes individual tests across workers and shards, so no test may assume
  another ran first. Isolation is by naming, not teardown — teardown still races a sibling worker's
  `list`, and the stack is destroyed per dispatch anyway
- **Never sleep for a side effect.** Redis Streams consumers, event projectors and Meilisearch
  indexing are eventually consistent by construction; use `eventually` / `eventuallyValue` from
  `_shared/eventual.ts`, which assert the actual condition. Note `toPass` defaults to an *unbounded*
  timeout, so always pass one (the shared config sets a backstop)
- **A status band needs a reason.** `expectStatusIn(res, [200, 404])` claims both answers are correct;
  without a comment naming why each is in the set it is a test that cannot fail
- **Discriminate 404 from everything else.** `expectProcedureMounted` in `_shared/connect.ts` is
  CLAUDE.md rule 8 at the E2E boundary: a handler whose DI dependency came back nil and skipped
  registration is invisible to unit tests and to any "not 2xx" assertion
- **Connect-RPC**: `POST /<package>.<Service>/<Method>` with `Content-Type: application/json`. On the
  wire the error code is the *string* spelling (`not_found`), not the numeric enum the connect-es
  client exposes. int64 fields arrive as JSON strings; empty repeated fields are omitted by protojson
- **Tags**: `@smoke` / `@contract` / `@authz` / `@slow`, matched by `--grep` (there is no `--tag` flag)

## Further reading

- Playwright: Writing tests — https://playwright.dev/docs/writing-tests
- Playwright: Continuous Integration — https://playwright.dev/docs/ci
- Playwright: API testing — https://playwright.dev/docs/api-testing
- `e2e/playwright/README.md` — the fleet's conventions
- ADR-000766 (the `e2e/<framework>/<svc>/run.sh` dispatch contract)
- ADRs 000763 / 000764 / 000765 — the retired Hurl suites, kept for the design history of the
  staging slice, the Connect-RPC-over-JSON idiom and the DB state-machine scenarios
