---
name: tdd-workflow
description: Drive Alt changes test-first in outside-in order — E2E (Playwright/Hurl) → CDC (Pact) → Unit (RED-GREEN-REFACTOR) — and close with a local CI parity sweep (format/lint/type/security) for every touched microservice. Use when implementing a feature, fixing a bug, or refactoring, when deciding what test to write first, or when the user says "TDDで". Also use before declaring work complete, since "tests pass" is not "CI will pass".
allowed-tools: Bash, Read, Glob, Grep, Edit, Write
argument-hint: <feature-description> [--service=<dir>]
---

# TDD Workflow

**Outside-in order for feature work: E2E → CDC → Unit.** This governs the *order of writing tests*.
The test pyramid separately governs *quantity*: few E2E, more CDC, many unit tests
([Fowler — Practical Test Pyramid](https://martinfowler.com/articles/practical-test-pyramid.html)).
Two different axes; both apply at once.

Three layers, with the designated tool in this repo:

- **E2E (outermost)** — *does the user journey / cross-service flow work?*
  - Browser user journeys → **Playwright** (`alt-frontend-sv/tests/e2e/`)
  - HTTP / Connect-RPC service scenarios → **Hurl** (`e2e/hurl/<service>/`)
- **CDC** — *do consumer and provider still understand each other?* → **Pact** at every boundary the change crosses
- **Unit** — *does each component work?* → per-layer tests under each service (Handler / Usecase / Gateway / Driver)

For a pure refactor inside one service's inner layers (no UI, no boundary change), skip Phase 0 and
Phase 1 and start at Phase 2.

`$ARGUMENTS` carries the feature description; `--service=<dir>` pins the target service, which is
otherwise auto-detected.

## Phase 0: E2E first

Write the outermost failing test — the acceptance test that expresses the user-visible or
cross-service behavior the change is supposed to deliver. It drives everything below it.

### Decision tree

- Touches browser UI (Svelte component, page, user flow) → **Playwright**
- Touches an HTTP endpoint, Connect-RPC method, or service-to-service flow → **Hurl**
- Full-stack (FE calls a new BE endpoint) → **both**: one Playwright journey + one Hurl scenario
- Pure inner-layer refactor with no external behavior change → skip to Phase 2

For where the specs live, the runner commands, and the Playwright/Hurl idioms this repo relies on,
read [references/e2e-playbooks.md](references/e2e-playbooks.md).

### Steps

1. **Detect scope** with the decision tree above.
2. **Write the failing E2E**, following a neighboring spec as the template.
3. **Run it** and confirm RED for the *right* reason — missing behavior, not a 404 from an absent
   route stub, a syntax error, or a compose service that is not up.
4. **Commit the failing E2E on its own:**
   ```bash
   git commit -m "test(e2e): add failing <feature> scenario"
   ```
5. **Proceed**: boundary crossed → Phase 1. Otherwise → Phase 2.

## Phase 1: CDC contract check

Run this after Phase 0's outer E2E is RED. CDC covers the request/response shape at each boundary,
not the journey.

### Does the change cross a service boundary?

- Modifies a proto file → run `buf lint` + `buf breaking`
- Modifies a request/response format between services
- Adds or modifies an HTTP endpoint consumed by another service
- Modifies Ollama options or LLM parameters
- Introduces or changes a **required header** (`X-Service-Token`, `Authorization`, `X-Api-Key`, a
  tracing header treated as required)
- Promotes or demotes an **authentication/authorization requirement** (optional → required,
  basic → JWT, JWT → mTLS)
- Flips **mTLS** from opt-in to enforced (`MTLS_ENFORCE` default, peer allowlist, CA bundle path)

If none apply, skip to Phase 2.

### If a boundary is touched

1. **Consumer side first** — write/update the Pact consumer test in the calling service.
2. **Run the consumer test** → generates the pact JSON under `<service>/pacts/`.
3. **Provider side** — run provider verification against that pact file.
4. **Proto changes** — `cd proto && buf lint && buf breaking --against '.git#branch=main'`.

For which pairs exist, where each test lives, and the exact commands, read
[references/cdc-map.md](references/cdc-map.md).

## Phase 1b: Provider adds a requirement

When the change is on the **provider** side and tightens what consumers must send (new required
header, new required field, stricter auth, mTLS promotion), consumer-driven contracts cannot protect
you unless every consumer has a pact *and* this change verifies it.

1. **Enumerate all consumers** of the affected endpoint / RPC / service.
   ```bash
   grep -rn "<provider-service-name>" --include="*.go" --include="*.py" --include="*.rs" --include="*.ts"
   grep -rn "<PROVIDER_URL_ENV_VAR>" .
   grep -rn "<generated-client-package>" .
   ```
   Record each caller: service name, file path, REST or Connect-RPC.

2. **Audit `pacts/` for each caller.** A pact file is named `<consumer>-<provider>.json`; check
   `pacts/`, `<consumer>/pacts/`, and `<consumer>/app/pacts/` (Go services that chdir into `app/`).
   A missing pact means that consumer is contract-unprotected — the change does not merge until a
   consumer contract test exists.

3. **Update each existing pact** so it pins the new requirement explicitly (e.g.
   `matchers.Like("token")` on `X-Service-Token`), then rerun the consumer test to regenerate it.

4. **Provider verifies the union of pacts** — the provider's verification test must list every
   consumer pact. Adding a new consumer contract means adding it here too.

5. **Run the full contract regression gate:**
   ```bash
   ./scripts/pact-check.sh            # file-based; fails closed
   ./scripts/pact-check.sh --broker   # broker mode with can-i-deploy semantics
   ```
   If one consumer's pact cannot satisfy the new requirement yet, stage the rollout with Pact's
   [pending pacts](https://docs.pact.io/pact_broker/advanced_topics/pending_pacts) rather than
   disabling that consumer's test.

6. **Runtime smoke** — rebuild (`docker compose up --build -d <provider> <consumers...>`) and tail
   the consumer logs for 401 / TLS handshake / 403 / 500.

**Why this phase exists:** in the April 2026 "RAG dead / Augur falls over" incident, `search-indexer`
promoted `X-Service-Token` to required (ADR-000722) while neither `alt-backend` nor
`rag-orchestrator` had a consumer pact with it. Pact was installed, but provider verification could
not see those consumers, so the 401 cascade only surfaced in production.

## Phase 2: RED

Define expected behavior through unit tests before implementing. For feature work, enter this phase
only once Phase 0 — and Phase 1 if a boundary is crossed — are RED.

1. **Detect language and service** from `go.mod` / `pyproject.toml` / `Cargo.toml` / `package.json` /
   `deno.json`, and identify the Clean Architecture layer from the feature description.
2. **Create the test file.** Bundled starting shapes:
   `templates/go_usecase_test.go.tmpl`, `templates/python_handler_test.py.tmpl`,
   `templates/typescript_component_test.ts.tmpl`, `templates/deno_unit_test.ts.tmpl`.
   Test behavior — success cases, error cases, edge cases — not file or symbol existence.
3. **Write the implementation stub first.** Declare the signature with explicit argument and return
   types, then fill the body with an unimplemented marker so the test fails on behavior rather than a
   missing symbol: Go `panic("not implemented")`, Python `raise NotImplementedError`,
   TypeScript `throw new Error("not implemented")`, Rust `unimplemented!()`.
4. **Verify the test fails for the right reason** — not a syntax or import error. If it passes with
   no implementation, rewrite it.
5. **Commit the tests:**
   ```bash
   git commit -m "test(<service>): add failing tests for <feature>"
   ```

## Phase 3: GREEN

Write only enough code to pass the tests.

- Do not edit the tests to make them pass. A test changed to fit the implementation has stopped
  being a specification.
- Do not add behavior no test covers.
- Check layer direction: Handler imports Usecase and Port; Usecase imports Port; Gateway imports
  Port and Driver.
- **GREEN includes wiring.** Unit tests construct the component themselves, so they pass even when
  `main`/`di` never wires it into the production pipeline. Grep the constructor in the composition
  root and confirm the `*_enabled` startup log exists (CLAUDE.md Rule 8, `.claude/rules/di-wiring.md`).
  The 2026-07 full-repo review found this gap in 6+ services.

## Phase 4: REFACTOR

Improve naming, remove duplication, simplify — rerunning tests after each change.

If Phase 1 detected a boundary change, rerun the CDC consumer tests and provider verification, or
`./scripts/pact-check.sh` for a full sweep.

```bash
git commit -m "feat(<service>): implement <feature>"
```

## Phase 5: Local CI parity

Reproduce each touched service's CI gates locally as the last step before reporting the work
complete. Phases 0-4 prove the tests pass; Phase 5 proves the **formatters, linters, static
analyzers and security scanners** pass too — those are what actually block PRs.

Skipping this is the most common cause of "green locally, red in CI": a stray unused import, format
drift, or a golangci-lint rule that only runs in CI.

1. **Enumerate every service directory touched** (`git diff --name-only` against the branch point).
2. **Run each touched service's gate** from [references/ci-parity.md](references/ci-parity.md).
3. **If `proto/**` changed**, run the proto gate regardless of which services were touched.
4. **If any contract test or consumer-provider interaction changed**, run `./scripts/pact-check.sh`.
5. **Never suppress a failing gate to unblock the task.** A `// nolint`, a loosened ruff config, or a
   skipped test greens the gate while leaving the defect in place, and the next reader has no signal
   that anything was traded away. Fix the cause or escalate.

## Test commands

| Layer | Tool | Command |
|-------|------|---------|
| E2E (UI) | Playwright | `cd alt-frontend-sv && bun run test:e2e:integration` |
| E2E (UI debug) | Playwright | `cd alt-frontend-sv && bun run test:e2e:ui` |
| E2E (API) | Hurl | `bash e2e/hurl/<service>/run.sh` |

| Language | Detection | Unit test | CDC consumer test |
|----------|-----------|-----------|-------------------|
| Go | `go.mod` | `go test ./...` | `CGO_ENABLED=1 go test -tags=contract ./driver/contract/ -v` |
| Python | `pyproject.toml` | `uv run pytest` | `uv run pytest tests/contract/ -v` |
| Rust | `Cargo.toml` | `cargo test` | `cargo test --lib contract -- --ignored` |
| TypeScript (bun) | `bun.lockb` | `bun test` | `bun test src/test/contracts/` |
| Deno | `deno.json` | `deno test` | — |

## Test file conventions

| Language | Unit test | CDC contract test |
|----------|-----------|-------------------|
| Go | `*_test.go` in the same package | `driver/contract/*_test.go` or `internal/adapter/contract/*_test.go` |
| Python | `tests/test_*.py` | `tests/contract/test_provider_verification.py` or `tests/contract/test_*_consumer.py` |
| Rust | `#[cfg(test)]` module or `tests/*.rs` | `src/clients/*_contract.rs` (`#[ignore = "CDC contract test"]`) |
| TypeScript | `*.test.ts` or `*.spec.ts` | `src/test/contracts/*.test.ts` |
| Deno | `tests/*_test.ts` | — |

## Service boundary checklist

Verify these when modifying service-to-service communication. Each line is here because it failed in
production at least once.

- [ ] **Proto compatibility**: `buf breaking` passes
- [ ] **Options consistency**: LLM parameters match across all request paths
- [ ] **Semaphore routing**: GPU requests go through HybridPrioritySemaphore
- [ ] **Content-type handling**: proxy layers detect every Connect-RPC serialization format
- [ ] **CDC tests updated**: consumer expectations match provider implementation
- [ ] **Every consumer sends the required headers**: each consumer's Pact request includes every
      required header, so provider verification can reject a regression
- [ ] **mTLS peer allowlist includes every new caller** (the provider's `VerifyConnection` or
      equivalent lists the new caller's CN/SAN)
- [ ] **Service token wired end-to-end**: `SERVICE_TOKEN` / `SERVICE_TOKEN_FILE` /
      `SERVICE_SECRET_FILE` is set in the compose unit **and** read by the config loader **and**
      passed to the outbound client constructor
- [ ] **CA bundle and cert paths exist in the container**: check `filepath.Clean` on env-driven cert
      paths and confirm the file is in the compose `secrets:` or bind-mount list
- [ ] **Provider verification lists every consumer pact** and is wired into `./scripts/pact-check.sh`
- [ ] **New component is wired into the composition root** (see Phase 3)

## Gotchas

Failure modes this repo has actually hit. The generic TDD rules are assumed.

1. **Editing a test to make it pass.** The test stops being a specification and the defect ships.
2. **Writing unit tests first and backfilling E2E at the end.** The outer test is what drives the
   design of the inner layers; added last, it only ratifies whatever was built.
3. **Skipping Phase 0 because "CDC already covers it".** CDC verifies per-boundary request/response
   shape; Phase 0 verifies the journey. Neither substitutes for the other.
4. **Treating mTLS / auth / required-header changes as "infra" and skipping Phase 0.** They change
   the request contract, so they start with a failing consumer test like any other contract change.
5. **Tightening a provider's requirements without updating every consumer's pact** — see Phase 1b.
6. **Leaving a consumer without a pact for a protected provider.** Provider verification can only
   reject regressions for contracts it can see.
7. **Using RED to prove a symbol is missing** rather than that behavior is wrong. Write the stub
   first (Phase 2 step 3) so the failure is about behavior.
8. **Changing a service API without updating the CDC consumer tests.**
9. **Sending different LLM options from different request paths.**
10. **Bypassing the semaphore for GPU shared resources.**
11. **`expect(await locator.isVisible()).toBe(true)` in Playwright** — reads state once and skips
    auto-waiting, so it flakes. Use `await expect(locator).toBeVisible()`.
12. **CSS / XPath selectors in Playwright** where `getByRole` / `getByLabel` / `getByText` /
    `getByTestId` work — user-facing locators survive DOM refactors.
13. **Hardcoding `http://localhost:...` in `.hurl` files** — pins the scenario to one environment.
14. **Running DB-backed Hurl scenarios with `--jobs >1`** — FK / sequence ordering breaks
    (ADR-000765, `knowledge-sovereign`).
15. **Declaring work complete without Phase 5.** Formatters, linters and scanners block the PR in
    the same commit you thought was finished.
16. **Suppressing a Phase 5 failure to finish the task** — see Phase 5 step 5.
17. **Declaring GREEN with an unwired component** — see Phase 3.
