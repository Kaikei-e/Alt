---
name: tdd-workflow
description: Test-Driven Development workflow for all languages. Use when implementing new features, fixing bugs, or refactoring code, or when the user says "TDDで". Enforces outside-in order E2E (Playwright/Hurl) → CDC (Pact) → Unit (RED-GREEN-REFACTOR) with concrete stubs, and mandates a local CI parity sweep (format/lint/type/security) for every touched microservice before handoff.
allowed-tools: Bash, Read, Glob, Grep, Edit, Write
argument-hint: <feature-description> [--service=<dir>]
---

# TDD Workflow

Test-Driven Development workflow following Claude Code best practices.
Use this skill in both Plan mode and implementation mode whenever the task may change code or tests.
When the work is implementation-oriented, read this skill before setting the test order and again before editing code.

**Use outside-in order for feature work: E2E → CDC → Unit.** This governs the *order of writing tests*.
The test pyramid still governs *quantity*: few E2E, more CDC, many unit tests
([Fowler — Practical Test Pyramid](https://martinfowler.com/articles/practical-test-pyramid.html)).
These are two different axes — order vs. quantity — and both apply simultaneously.

**Finish every task with Phase 5 — local CI parity.** After Phases 0–4 are green, run the same formatters / linters / static analyzers / security scanners the touched microservices' CI pipelines run, locally, before declaring the work complete. "Tests pass" ≠ "CI will pass"; Phase 5 closes that gap. See §Phase 5 for per-service commands.

Three layers, with designated tools in this repo:

- **E2E (outermost)** — answers *"does the user journey / cross-service flow work?"*
  - Browser user journeys → **Playwright** (`alt-frontend-sv/tests/e2e/`)
  - HTTP / Connect-RPC service scenarios → **Hurl** (`e2e/hurl/<service>/`)
- **CDC** — answers *"do consumer and provider understand each other?"* → **Pact** at every service boundary the change crosses.
- **Unit** — answers *"does each component work?"* → per-layer tests under each service (Handler / Usecase / Gateway / Driver).

For a pure refactor inside one service's inner layers (no UI, no boundary change), skip Phase 0 and Phase 1 and jump to Phase 2.

## Arguments

- `$ARGUMENTS` - Feature description and optional flags
- `--service=<dir>` - Target service directory (auto-detected if omitted)

## Phase 0: E2E FIRST (Playwright / Hurl)

**Goal:** Write the outermost failing test — the one that expresses the user-visible / cross-service behavior the change is supposed to deliver. This is the acceptance test Dave Farley and Martin Fowler call the "executable specification." It drives everything else.

### Decision tree

- Change touches browser UI (Svelte component, page, or user flow) → **Playwright**
- Change touches an HTTP endpoint, Connect-RPC method, or service-to-service flow → **Hurl**
- Change is full-stack (FE calls a new BE endpoint) → **both**: one Playwright journey + one Hurl scenario
- Change is a pure inner-layer refactor with no external behavior change → skip Phase 0 (go to Phase 2)

### Playwright — where & how (frontend E2E)

- Config: `alt-frontend-sv/playwright.config.ts`
- Specs: `alt-frontend-sv/tests/e2e/{auth,desktop,mobile,visual,integration,a11y}/*.spec.ts`
- Page Object base: `alt-frontend-sv/tests/e2e/pages/BasePage.ts` — extend it, don't reinvent
- Fixtures / factories: `alt-frontend-sv/tests/e2e/fixtures/`
- Global setup (MSW, Kratos session): `alt-frontend-sv/tests/e2e/global-setup.ts`

**Commands:**
```bash
cd alt-frontend-sv && bun run test:e2e:integration   # integration project (default)
cd alt-frontend-sv && bun run test:e2e:ui            # UI mode for debugging a single spec
cd alt-frontend-sv && bun run test:e2e               # full suite (build + all projects)
```

**Playwright best practices** (from [playwright.dev best practices](https://playwright.dev/docs/best-practices)):

- **Locators**: `getByRole` / `getByLabel` / `getByText` / `getByTestId` — avoid CSS / XPath
- **Web-first async assertions**: `await expect(locator).toBeVisible()` — **never** `expect(await locator.isVisible()).toBe(true)`
- Trust auto-waiting — no manual `waitForTimeout`, no manual retry loops
- One `test()` = one user journey; isolation via fresh browser context per test
- Group with `test.describe()`; share setup with `beforeEach`, not global mutable state
- Mock third-party deps via the MSW server wired in `global-setup.ts`
- Keep specs deterministic — seed Kratos sessions and backend fixtures, don't rely on "whatever is in the DB"

### Hurl — where & how (API / service-boundary E2E)

- Specs: `e2e/hurl/<service>/*.hurl` (one `.hurl` file per scenario)
- Runner: `e2e/hurl/<service>/run.sh` — boots the right `compose/compose.staging.yaml` profile, runs Hurl **inside the alt-staging Docker network**, writes reports to `e2e/reports/<service>-<run_id>/`
- Staging profiles: `search-indexer` / `mq-hub` / `knowledge-sovereign` in `compose/compose.staging.yaml`
- CI: `.github/workflows/e2e-hurl.yml` (path-gated per service)

**Commands:**
```bash
bash e2e/hurl/search-indexer/run.sh
bash e2e/hurl/mq-hub/run.sh
bash e2e/hurl/knowledge-sovereign/run.sh
```

**Hurl best practices** (from [hurl.dev asserting-response](https://hurl.dev/docs/asserting-response.html), [hurl.dev CI/CD](https://hurl.dev/docs/tutorial/ci-cd-integration.html), and ADRs 000763 / 000764 / 000765):

- **Parameterize** hosts / tokens with `--variable host=...` — never hardcode `http://localhost:...`
- **Health-gate** scenarios with `--retry` before exercising business endpoints
- **CI flags**: `--test --report-junit <dir>/junit.xml --report-html <dir>/html`
- **DB-backed scenarios MUST use `--jobs 1`** — FK / sequence ordering breaks under parallel execution (precedent: ADR-000765)
- Pass `--file-root` whenever scenarios reference fixtures via `file,e2e/fixtures/...;`
- **Assertions**: implicit version/status/headers first, then explicit `jsonpath` / `xpath` with predicates (`contains`, `matches /regex/`, `isIsoDate`, `isUuid`, `isInteger`, `not exists`)
- **Connect-RPC idiom** (ADR-000764): `POST /services.<package>.v1.<Service>/<Method>` with `Content-Type: application/json`; int64 fields are JSON strings; empty repeated fields are omitted
- **Chain requests** with `[Captures]` and reference via `{{var}}` in subsequent entries — don't duplicate setup across files
- Order sections inside an entry: request → `[Captures]` → implicit response (status/headers) → `[Asserts]` → body

### Steps

1. **Detect scope** using the decision tree above (UI / API / both / skip).
2. **Write the failing E2E first**:
   - Playwright: new `*.spec.ts` under the matching `tests/e2e/<area>/` directory, extending the correct Page Object.
   - Hurl: new `*.hurl` under `e2e/hurl/<service>/`, following a neighboring file as the template.
3. **Run it** — confirm RED for the *right reason* (missing behavior), not for the wrong reason (404 / connection refused from a missing route stub, syntax error, or compose service not up).
4. **Commit the failing E2E** on its own:
   ```bash
   git add <spec-or-hurl-file>
   git commit -m "test(e2e): add failing <feature> scenario"
   ```
5. **Proceed**: if the change crosses a service boundary → Phase 1 (CDC). Otherwise → Phase 2 (Unit RED).

## Phase 1: CDC CONTRACT CHECK (Pact)

**Goal:** Determine if the change touches a service boundary. If yes, write/update Pact CDC tests so every boundary the flow crosses has a contract.

Run Phase 1 **after** Phase 0's outer E2E is RED. CDC focuses on the request/response shape at each boundary — not the journey itself.

### Steps

1. **Detect if change crosses service boundaries**
   - Does the change modify a proto file? → Run `buf lint` + `buf breaking`
   - Does the change modify a request/response format between services?
   - Does the change add/modify an HTTP endpoint consumed by another service?
   - Does the change modify Ollama options or LLM parameters?
   - **Does the change introduce or change a required header?** (e.g. `X-Service-Token`, `Authorization`, `X-Api-Key`, tracing headers treated as required)
   - **Does the change promote or demote an authentication/authorization requirement?** (e.g. optional → required, basic → JWT, JWT → mTLS)
   - **Does the change flip mTLS from opt-in to enforced?** (e.g. `MTLS_ENFORCE` default change, peer allowlist edit, CA bundle path change)

2. **If service boundary is touched:**
   a. **Consumer side first** — Write/update Pact consumer test in the calling service as the first end-to-end contract test
      - Go services: `pact-go v2` in `<service>/app/driver/contract/` or `<service>/internal/adapter/contract/`
      - TS services: `@pact-foundation/pact` in `tests/contract/`
   b. **Run consumer test** → Generates pact JSON in `<service>/pacts/`
   c. **Provider side** — Run provider verification against the pact file
      - Python: `pact-python` in `tests/contract/test_provider_verification.py`
      - Go: provider verification test in the providing service
   d. **Proto changes** — Run `cd proto && buf lint && buf breaking --against '.git#branch=main'`

3. **If no service boundary:** Skip to Phase 2 (Unit RED).

## Phase 1b: PROVIDER-ADDS-REQUIREMENT PLAYBOOK

**When the change is on the provider side and tightens what consumers must send** (new required header, new required field, stricter auth, mTLS promotion), the consumer-driven contract pipeline cannot protect you unless every consumer has a pact and that pact is verified by this change. Follow this playbook before merging:

1. **Enumerate all consumers** of the affected endpoint / RPC / service.
   ```bash
   # Grep for URL / env var / generated client import
   grep -rn "<provider-service-name>" --include="*.go" --include="*.py" --include="*.rs" --include="*.ts"
   grep -rn "<PROVIDER_URL_ENV_VAR>" .
   grep -rn "<generated-client-package>" .
   ```
   Record each caller: service name, file path, whether the call uses REST or Connect-RPC.

2. **Audit `pacts/` for each caller** — A pact file is named `<consumer>-<provider>.json`. Check all three common locations:
   - `pacts/` (root)
   - `<consumer>/pacts/`
   - `<consumer>/app/pacts/` (Go services that chdir into `app/`)

   For every enumerated caller, confirm a pact file exists. **If it is missing, stop and treat that consumer as contract-unprotected** — the change must not merge until a consumer contract test is added.

3. **Update each existing pact** — Each consumer's Pact test must pin the new requirement explicitly (e.g. `matchers.Like("token")` on the `X-Service-Token` header). Run the consumer test so the pact file is regenerated, then verify the pact still reflects real consumer behaviour.

4. **Provider verifies the union of pacts** — The provider's verification test (Go `provider_test.go` with `pact-go/v2/provider`, or Python `pact-python`) must list every consumer pact. When you add a new consumer contract, add it to the provider's pact file list too.

5. **Run the full contract regression gate:**
   ```bash
   ./scripts/pact-check.sh          # file-based; fails closed if any step fails
   ./scripts/pact-check.sh --broker # broker mode with can-i-deploy semantics
   ```
   **Do not ship a provider-side requirement change if this fails.** If a single consumer's pact cannot yet satisfy the new requirement, use the Pact [pending pacts](https://docs.pact.io/pact_broker/advanced_topics/pending_pacts) / WIP pacts workflow to stage the rollout rather than disabling the consumer's test.

6. **Runtime smoke** — Rebuild the containers (`docker compose up --build -d <provider> <consumers...>`) and tail the logs for 401 / TLS handshake / 403 / 500 across the consumers to confirm no silent failures.

### Why this exists

Missing this playbook caused the April 2026 "RAG dead / Augur falls over" incident: `search-indexer` promoted `X-Service-Token` to required (ADR-000722), but neither `alt-backend` nor `rag-orchestrator` had a consumer pact with `search-indexer`. Pact CDC was installed but the provider verification could not see those consumers, so the 401 cascade only surfaced in production.

### Existing CDC Tests

Direction reads `A → B` as "A consumes B" (so `A`'s `pacts/A-B.json` is the contract `B` must satisfy).

| A (consumer) → B (provider) | Language | Consumer test location |
|-----------------------------|----------|------------------------|
| alt-backend → pre-processor | Go | `alt-backend/app/driver/preprocessor_connect/contract/` |
| alt-backend → search-indexer | Go | `alt-backend/app/driver/search_indexer_connect/contract/` |
| pre-processor → news-creator | Go | `pre-processor/app/driver/contract/` |
| rag-orchestrator → news-creator | Go | `rag-orchestrator/internal/adapter/contract/` |
| rag-orchestrator → search-indexer | Go | `rag-orchestrator/internal/adapter/contract/` |
| search-indexer → alt-backend, recap-worker, mq-hub | Go | `search-indexer/app/driver/contract/` |
| mq-hub → search-indexer, tag-generator | Go | `mq-hub/app/driver/contract/` |
| recap-worker → news-creator, recap-subworker, alt-backend, tag-generator | Rust | `recap-worker/recap-worker/src/clients/*_contract.rs` |
| recap-evaluator → recap-worker | Python | `recap-evaluator/tests/contract/` |
| alt-butterfly-facade → alt-backend, tts-speaker | Go | `alt-butterfly-facade/internal/handler/contract/` |
| auth-hub → kratos | Go | `auth-hub/internal/adapter/gateway/contract/` |
| acolyte-orchestrator → search-indexer | Python | `acolyte-orchestrator/tests/contract/` |

### Providers and the consumers they verify (reverse lookup)

Use this table when planning a **provider-side change** (Phase 1b) to confirm every consumer is under contract.

| Provider | Consumers whose pacts the provider verifies | Provider verification location |
|----------|---------------------------------------------|--------------------------------|
| alt-backend | recap-worker | `alt-backend/app/driver/contract/provider_test.go` |
| search-indexer | rag-orchestrator, alt-backend (⚠ acolyte-orchestrator pact exists but is not yet verified — missing `X-Service-Token` assertion) | `search-indexer/app/driver/contract/provider_test.go` |
| news-creator | pre-processor, rag-orchestrator, recap-worker, acolyte-orchestrator | `news-creator/app/tests/contract/` |
| recap-subworker | recap-worker | `recap-subworker/tests/contract/` |
| tag-generator | recap-worker, mq-hub | `tag-generator/app/tests/contract/` |
| tts-speaker | alt-butterfly-facade | `tts-speaker/tests/contract/` |
| kratos | auth-hub | (external — consumer-only) |

### CDC Test Commands

```bash
# Go consumer tests (generates pact files)
cd alt-backend/app && CGO_ENABLED=1 go test -tags=contract ./driver/preprocessor_connect/contract/ -v
cd pre-processor/app && CGO_ENABLED=1 go test -tags=contract ./driver/contract/ -v
cd rag-orchestrator && CGO_ENABLED=1 go test -tags=contract ./internal/adapter/contract/ -v
cd search-indexer/app && CGO_ENABLED=1 go test -tags=contract ./driver/contract/ -v
cd mq-hub/app && CGO_ENABLED=1 go test -tags=contract ./driver/contract/ -v

# Rust consumer tests (generates pact files)
cd recap-worker/recap-worker && cargo test --lib contract -- --ignored

# Python consumer tests (generates pact files)
cd recap-evaluator && uv run pytest tests/contract/ -v --no-cov

# Python provider verification (validates against pact files or Broker)
cd news-creator/app && SERVICE_SECRET=test-secret uv run pytest tests/contract/ -v
cd recap-subworker && SERVICE_SECRET=test-secret uv run pytest tests/contract/ -v
cd tag-generator/app && SERVICE_SECRET=test-secret uv run pytest tests/contract/ -v

# Go provider verification
cd alt-backend/app && CGO_ENABLED=1 go test -tags=contract ./driver/contract/ -v

# Full contract regression check (all consumers + all providers)
./scripts/pact-check.sh            # File-based mode (no Broker, fast)
./scripts/pact-check.sh --broker   # Broker mode (starts local Pact Broker)

# Proto breaking change check
cd proto && buf lint && buf breaking --against '.git#branch=main'
```

## Phase 2: RED (Write Failing Unit Test)

**Goal:** Define expected behavior through unit tests BEFORE implementation.
For feature work, enter this phase only after Phase 0 (E2E) and — if a boundary is crossed — Phase 1 (CDC) tests have been written and are RED.

### Steps

1. **Detect Language & Service**
   - Check for `go.mod`, `pyproject.toml`, `Cargo.toml`, `package.json`, `deno.json`
   - Identify Clean Architecture layer from feature description

2. **Create Test File**
   - Use language-specific naming convention
   - Write tests that define expected behavior, not file existence or symbol existence
   - Include success cases, error cases, and edge cases

3. **Create the implementation stub first when needed**
   - Write the function or method signature with explicit argument types and return types before relying on the test failure
   - Fill the body with a temporary unimplemented stub so the test fails for the right reason, not because the symbol is missing
   - Go: `panic("not implemented")`
   - Python: `raise NotImplementedError`
   - TypeScript: `throw new Error("not implemented")`
   - Rust: `unimplemented!()`

4. **Verify Test Fails**
   - Run test command for the language
   - Confirm failure is for the RIGHT reason (not syntax/import errors)
   - If test passes without implementation, rewrite it

5. **Commit Tests**
   ```bash
   git add <test-file>
   git commit -m "test(<service>): add failing tests for <feature>"
   ```

## Phase 3: GREEN (Minimal Implementation)

**Goal:** Write ONLY enough code to pass the tests.

### Steps

1. **Create Implementation**
   - Write minimal code to pass tests
   - DO NOT modify tests to make them pass
   - DO NOT add features not covered by tests

2. **Verify Tests Pass**
   - Run test command
   - All tests must pass before proceeding

3. **Check Layer Violations**
   - Handler can only import Usecase and Port
   - Usecase can only import Port
   - Gateway can import Port and Driver

## Phase 4: REFACTOR (Clean Up)

**Goal:** Improve code quality while keeping tests green.

### Steps

1. **Improve Code**
   - Remove duplication
   - Improve naming
   - Simplify logic

2. **Verify Tests Still Pass**
   - Run tests after each change

3. **Contract Regression Check** (if Phase 1 detected a boundary change)
   - Re-run CDC consumer tests → Verify pact files are still valid
   - Re-run provider verification → Verify provider still satisfies contracts
   - Or run `./scripts/pact-check.sh` for a full consumer + provider sweep

4. **Final Commit**
   ```bash
   git add <implementation-file>
   git commit -m "feat(<service>): implement <feature>"
   ```

## Phase 5: LOCAL CI PARITY (MANDATORY before handoff)

**Goal:** Reproduce the same gates each touched microservice's CI would run, locally, as the last step before reporting the work complete. Phases 0-4 guarantee tests pass; Phase 5 guarantees **formatters, linters, static analyzers, and security scanners** also pass — these are what block PRs in GitHub Actions (`reusable-test-*.yaml`, `reusable-go-quality-gates.yaml`, `proto-contract.yaml`).

Skipping this phase is the most common cause of "green locally, red in CI" — typically a stray unused import, format drift, or a golangci-lint rule that only runs in CI.

### Steps

1. **Enumerate every service directory touched** by the change (grep `git status` / `git diff --name-only` against the branch point).
2. **For each touched service**, run the language-specific gate from the table below. All must pass before handoff.
3. **If a proto file changed** under `proto/`, also run the proto gate regardless of which services are touched.
4. **If a pact / contract test or any consumer-provider interaction changed**, run `./scripts/pact-check.sh` so the full contract regression gate is green.
5. **Never suppress a failing gate to unblock the task** — fix the underlying issue or escalate. Feedback memory `feedback_tdd_strict.md` + `feedback_plan_audit.md` apply.

### Per-service CI parity commands

Match these to what the reusable CI workflows run (`reusable-test-go.yaml`, `reusable-go-quality-gates.yaml`, `reusable-test-python.yaml`, `reusable-test-rust.yaml`, `alt-frontend-sv-unit-test.yaml`, `proto-contract.yaml`). Update this table when CI changes.

**Go service** (alt-backend, search-indexer, pre-processor, mq-hub, auth-hub, rag-orchestrator, alt-butterfly-facade, etc.)

```bash
cd <service>/app      # or service root for rag-orchestrator / auth-hub
gofmt -l . | grep -v '^gen/' | grep -v '^$'   # must print nothing
go vet ./...
# golangci-lint v2.1 (CI uses golangci-lint-action@v8); install locally via:
#   go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.1.0
golangci-lint run ./...
go test ./... -race                            # CI uses CGO_ENABLED=1, add for race tests
CGO_ENABLED=1 go test -tags=contract ./driver/contract/ -v    # if any CDC changed
```

**Python service** (acolyte-orchestrator, news-creator, tag-generator, recap-subworker, metrics, recap-evaluator)

```bash
cd <service>/app      # or service root
uv sync --all-extras --dev
uv run ruff check .
uv run ruff format --check .
uv run pyrefly check
uv run pytest          # CI adds --cov=. --cov-report=xml --junit-xml=tests/results.xml
uv run pytest tests/contract/ -v --no-cov     # if any CDC changed
# news-creator / recap-subworker / tag-generator contract tests require:
#   SERVICE_SECRET=test-secret uv run pytest tests/contract/ -v
```

**Rust service** (rask-log-aggregator, rask-log-forwarder, recap-worker)

```bash
cd <service>
cargo fmt --all -- --check
cargo clippy --all-targets --all-features -- -D warnings
cargo build --release
cargo test --all
cargo test --lib contract -- --ignored        # if any CDC changed (recap-worker)
```

**TypeScript / Svelte** (alt-frontend-sv)

```bash
cd alt-frontend-sv
bun install --frozen-lockfile
bun run check                                  # svelte-check + tsc
bun run lint                                   # eslint / prettier --check
bun test                                       # vitest unit + contract tests
bun test src/test/contracts/                   # if any CDC changed
bun run test:e2e:integration                   # Playwright if UI touched (Phase 0 scope)
```

**Deno service** (auth-token-manager, alt-perf)

```bash
cd <service>
deno fmt --check
deno lint
deno check <entrypoint>.ts
deno test --allow-all
```

**Proto changes** (always, if `proto/**` was touched)

```bash
cd proto
buf lint
buf breaking --against '.git#branch=main'
# Regenerate stubs for every consumer of the changed proto file and commit:
buf generate --template buf.gen.backend-internal.yaml        # alt-backend
buf generate --template buf.gen.pre-processor-services.yaml  # pre-processor
buf generate --template buf.gen.search-indexer.yaml          # search-indexer
# ... and the other buf.gen.<service>.yaml templates as needed
```

**Database migration** (if `migrations-atlas/migrations/**` was touched)

```bash
cd migrations-atlas
atlas migrate hash                # refresh atlas.sum after new .sql files
atlas migrate validate            # schema is consistent
atlas migrate lint --latest 1     # CI-equivalent linter check
```

**Full contract regression** (if any CDC interaction — consumer or provider — was touched)

```bash
./scripts/pact-check.sh           # file-based mode, fails closed
```

### Reporting

At handoff, state explicitly which services' CI-parity gates you ran and their exit status. A truthful summary is:

> Ran local CI parity for: acolyte-orchestrator (ruff/pyrefly/pytest all green, 591 tests), alt-backend (gofmt/vet/golangci-lint/go test all green), pre-processor (same), search-indexer (same). Proto gate green. No CDC regression.

If any gate is skipped, say so explicitly (e.g. "skipped golangci-lint locally — not installed, rely on CI"). Do **not** silently skip.

## Test Commands

### E2E (Phase 0)

| Layer | Tool | Command |
|-------|------|---------|
| E2E (UI) | Playwright | `cd alt-frontend-sv && bun run test:e2e:integration` |
| E2E (UI debug) | Playwright | `cd alt-frontend-sv && bun run test:e2e:ui` |
| E2E (API) | Hurl | `bash e2e/hurl/<service>/run.sh` |

### CDC + Unit (Phase 1 / Phase 2) by Language

| Language | Detection | Unit Test | CDC Consumer Test |
|----------|-----------|-----------|-------------------|
| Go | `go.mod` | `go test ./...` | `CGO_ENABLED=1 go test -tags=contract ./driver/contract/ -v` |
| Python | `pyproject.toml` | `uv run pytest` | `uv run pytest tests/contract/ -v` |
| Rust | `Cargo.toml` | `cargo test` | `cargo test --lib contract -- --ignored` |
| TypeScript (bun) | `bun.lockb` | `bun test` | `bun test src/test/contracts/` |
| Deno | `deno.json` | `deno test` | — |

## Test File Conventions

| Language | Unit Test | CDC Contract Test |
|----------|-----------|-------------------|
| Go | `*_test.go` in same package | `driver/contract/*_test.go` or `internal/adapter/contract/*_test.go` |
| Python | `tests/test_*.py` | `tests/contract/test_provider_verification.py` or `tests/contract/test_*_consumer.py` |
| Rust | `#[cfg(test)]` module or `tests/*.rs` | `src/clients/*_contract.rs` (`#[ignore = "CDC contract test"]`) |
| TypeScript | `*.test.ts` or `*.spec.ts` | `src/test/contracts/*.test.ts` |
| Deno | `tests/*_test.ts` | — |

## Clean Architecture Integration

When implementing features:
1. Identify the target layer (Handler, Usecase, Gateway, Driver)
2. Mock dependencies from outer layers
3. Test only the layer's responsibility

## Service Boundary Checklist (from Postmortems)

When modifying service-to-service communication, verify:

- [ ] **Proto compatibility**: `buf breaking` passes
- [ ] **Options consistency**: LLM parameters match across all request paths
- [ ] **Semaphore routing**: GPU requests go through HybridPrioritySemaphore
- [ ] **Content-type handling**: Proxy layers detect all Connect-RPC serialization formats
- [ ] **CDC tests updated**: Consumer expectations match provider implementation
- [ ] **Required headers sent by every consumer**: for each consumer of the affected provider, the Pact request includes every required header (e.g. `X-Service-Token`, `Authorization`)
- [ ] **mTLS peer allowlist includes every new caller**: the provider's `VerifyConnection` or equivalent peer allowlist lists the new caller CN/SAN
- [ ] **Service token env wired end-to-end**: `SERVICE_TOKEN` / `SERVICE_TOKEN_FILE` / `SERVICE_SECRET_FILE` is set in the compose unit **and** read by the service's config loader **and** passed to the outbound client constructor
- [ ] **CA bundle and cert paths exist in the container**: check `filepath.Clean` on any env-driven cert path; confirm the file exists in the compose `secrets:` or bind-mount list
- [ ] **Provider-side pact verification lists every consumer pact**: the provider's verification test file includes each caller's pact file and is wired into `./scripts/pact-check.sh`

## Anti-Patterns (AVOID)

1. Writing implementation before tests
2. Modifying tests to make them pass
3. Adding features not covered by tests
4. Skipping error case tests
5. Testing implementation details instead of behavior
6. Changing service API without updating CDC consumer tests
7. Sending different LLM options from different request paths
8. Bypassing the semaphore for GPU shared resources
9. Writing unit tests that only fail because a file or function does not exist yet
10. Using RED to validate missing symbols instead of behavior through a concrete stub
11. **Tightening a provider's requirements (new required header, new required field, auth promotion) without updating every consumer's Pact to pin the new requirement** — see Phase 1b
12. **Treating mTLS / auth / required-header changes as "infra" and skipping Phase 0** — they change the request contract and must start with a failing consumer test
13. **Leaving a consumer without a pact for a protected provider** — if a provider enforces auth, every caller must have a pact that asserts the auth is present, so provider verification can reject regressions
14. **Writing `expect(await locator.isVisible()).toBe(true)` in Playwright** — use the web-first async form `await expect(locator).toBeVisible()` so auto-waiting applies
15. **Using CSS / XPath selectors in Playwright** when `getByRole` / `getByLabel` / `getByText` / `getByTestId` work — user-facing locators survive DOM refactors
16. **Hardcoding `http://localhost:...` in `.hurl` files** — always parameterize with `--variable host=...` so the same scenario runs against local / staging / CI
17. **Running DB-backed Hurl scenarios with `--jobs >1`** — FK / sequence ordering breaks under parallelism (precedent: ADR-000765 `knowledge-sovereign`)
18. **Skipping Phase 0 because "CDC already covers it"** — CDC verifies per-boundary request/response shape; Phase 0 verifies the user journey / cross-service flow. They are not substitutes
19. **Writing unit tests first and backfilling E2E at the end** — this violates the outside-in order. The outer test is what drives the design of the inner layers
20. **Declaring work complete without running Phase 5 (local CI parity)** — "tests pass" ≠ "CI will pass". Formatters, linters, static analyzers, and security scanners block PRs in the same commit you thought was done. Every touched microservice gets its CI-equivalent gate run locally before handoff
21. **Suppressing a Phase 5 failure to finish the task** — disabling a lint rule, adding `// nolint`, loosening ruff config, or skipping a test to green the gate is a red flag. Fix the underlying code or escalate; never silence the gate as a shortcut

## References

- Pact: Handling authentication and authorization — https://docs.pact.io/provider/handling_auth
- Pact: Pending pacts — https://docs.pact.io/pact_broker/advanced_topics/pending_pacts
- Pact: Webhooks (`contract_requiring_verification_published`) — https://docs.pact.io/pact_broker/webhooks
- Pact: Can I Deploy — https://docs.pact.io/pact_broker/can_i_deploy
- Pact: Contract Tests vs Functional Tests — https://docs.pact.io/consumer/contract_tests_not_functional_tests
- PactFlow: Compatibility Checks — https://docs.pactflow.io/docs/bi-directional-contract-testing/compatibility-checks/
- Playwright: Best Practices — https://playwright.dev/docs/best-practices
- Playwright: Writing tests — https://playwright.dev/docs/writing-tests
- Playwright: Continuous Integration — https://playwright.dev/docs/ci
- Hurl: Asserting Response — https://hurl.dev/docs/asserting-response.html
- Hurl: CI/CD Integration — https://hurl.dev/docs/tutorial/ci-cd-integration.html
- Hurl: Chaining Requests — https://hurl.dev/docs/tutorial/chaining-requests.html
- Martin Fowler: The Practical Test Pyramid — https://martinfowler.com/articles/practical-test-pyramid.html
- Martin Fowler: TestPyramid bliki — https://martinfowler.com/bliki/TestPyramid.html
- ADR-000763 (Hurl framework inception — search-indexer phase 1)
- ADR-000764 (Hurl mq-hub phase 2 — Connect-RPC over HTTP/1.1+JSON)
- ADR-000765 (Hurl knowledge-sovereign phase 1 — DB state machine, `--jobs 1`)
