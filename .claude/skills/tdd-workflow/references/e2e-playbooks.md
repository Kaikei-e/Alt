# E2E Playbooks (Phase 0)

Where the outermost tests live and how to write them. Load the section matching the
scope Phase 0's decision tree selected.

- [Playwright — browser user journeys](#playwright--browser-user-journeys)
- [Hurl — API and service-boundary scenarios](#hurl--api-and-service-boundary-scenarios)

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

## Hurl — API and service-boundary scenarios

- Specs: `e2e/hurl/<service>/*.hurl` (one file per scenario)
- Runner: `e2e/hurl/<service>/run.sh` — boots the right `compose/compose.staging.yaml` profile, runs
  Hurl inside the alt-staging Docker network, writes reports to `e2e/reports/<service>-<run_id>/`
- Staging profiles: `search-indexer` / `mq-hub` / `knowledge-sovereign` in `compose/compose.staging.yaml`
- CI: `.github/workflows/e2e-hurl.yml`

```bash
bash e2e/hurl/search-indexer/run.sh
bash e2e/hurl/mq-hub/run.sh
bash e2e/hurl/knowledge-sovereign/run.sh
```

Per [hurl.dev asserting-response](https://hurl.dev/docs/asserting-response.html),
[hurl.dev CI/CD](https://hurl.dev/docs/tutorial/ci-cd-integration.html) and ADRs 000763 / 000764 / 000765:

- **Parameterize** hosts and tokens with `--variable host=...` so one scenario runs against
  local, staging and CI. A hardcoded `http://localhost:...` pins the scenario to one environment
- **Health-gate** scenarios with `--retry` before exercising business endpoints
- **CI flags**: `--test --report-junit <dir>/junit.xml --report-html <dir>/html`
- **DB-backed scenarios run with `--jobs 1`.** FK and sequence ordering breaks under parallel
  execution — this is what ADR-000765 (`knowledge-sovereign`) established
- Pass `--file-root` whenever scenarios reference fixtures via `file,e2e/fixtures/...;`
- **Assertions**: implicit version/status/headers first, then explicit `jsonpath` / `xpath` with
  predicates (`contains`, `matches /regex/`, `isIsoDate`, `isUuid`, `isInteger`, `not exists`)
- **Connect-RPC idiom** (ADR-000764): `POST /services.<package>.v1.<Service>/<Method>` with
  `Content-Type: application/json`; int64 fields arrive as JSON strings; empty repeated fields are omitted
- **Chain requests** with `[Captures]` and reference them via `{{var}}` in later entries rather than
  duplicating setup across files
- Section order inside an entry: request → `[Captures]` → implicit response (status/headers) → `[Asserts]` → body

## Further reading

- Playwright: Writing tests — https://playwright.dev/docs/writing-tests
- Playwright: Continuous Integration — https://playwright.dev/docs/ci
- Hurl: Chaining Requests — https://hurl.dev/docs/tutorial/chaining-requests.html
- ADR-000763 (Hurl framework inception — search-indexer phase 1)
- ADR-000764 (Hurl mq-hub phase 2 — Connect-RPC over HTTP/1.1+JSON)
- ADR-000765 (Hurl knowledge-sovereign phase 1 — DB state machine, `--jobs 1`)
