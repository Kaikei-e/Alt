---
name: tdd-workflow
description: Drive Alt changes test-first in outside-in order — E2E (Playwright) → CDC (Pact) → Unit (RED-GREEN-REFACTOR) — and close with a local CI parity sweep (format/lint/type/security) for every touched microservice. Use when implementing a feature, fixing a bug, or refactoring, when deciding what test to write first, or when the user says "TDDで". Also use before declaring work complete, since "tests pass" is not "CI will pass".
allowed-tools: Bash, Read, Glob, Grep, Edit, Write
argument-hint: <feature-description> [--service=<dir>]
---

# TDD Workflow

Write tests outside-in: E2E → CDC → Unit. That is the order of *writing*; quantity still follows the
pyramid (few E2E, many unit). Both axes apply at once.

- **E2E** — does the user journey / cross-service flow work? Browser journeys → Playwright
  (`alt-frontend-sv/tests/e2e/`); HTTP or Connect-RPC scenarios → Playwright API suites
  (`e2e/playwright/<service>/`)
- **CDC** — do consumer and provider still understand each other? → Pact at every boundary the change crosses
- **Unit** — does each component work? → per-layer tests (Handler / Usecase / Gateway / Driver)

A pure refactor inside one service's inner layers (no UI, no boundary change) starts at Phase 2.

`$ARGUMENTS` carries the feature description; `--service=<dir>` pins the target service, which is
otherwise auto-detected.

Bundled material, read when the phase calls for it:

- [references/e2e-playbooks.md](references/e2e-playbooks.md) — Phase 0: where specs live, the runner
  commands, and the Playwright idioms this repo relies on
- [references/cdc-map.md](references/cdc-map.md) — Phase 1: which consumer-provider pairs exist,
  where each test lives, and the exact commands
- [references/ci-parity.md](references/ci-parity.md) — Phase 5: the per-service local gates that
  match CI
- `templates/go_usecase_test.go.tmpl`, `templates/python_handler_test.py.tmpl`,
  `templates/typescript_component_test.ts.tmpl`, `templates/deno_unit_test.ts.tmpl` — Phase 2
  starting shapes, for when the target has no neighbouring test worth copying

## Phase 0: E2E first

Write the outermost failing test — the acceptance test that expresses the user-visible or
cross-service behavior the change is supposed to deliver. It drives everything below it.

Pick the scope:

- Browser UI (Svelte component, page, user flow) → **Playwright**
- HTTP endpoint, Connect-RPC method, or service-to-service flow → **a Playwright API suite**
- Full-stack (FE calls a new BE endpoint) → **both**: one browser journey + one API spec
- Inner-layer refactor with no external behavior change → skip to Phase 2

Then:

1. Write the failing E2E, following a neighboring spec as the template. Read
   `references/e2e-playbooks.md` and `e2e/playwright/README.md` first — the latter's "rules that are
   not negotiable" section is binding on any new API spec.
2. Run it and confirm RED for the *right* reason — missing behavior, not a 404 from an absent route
   stub, a syntax error, or a compose service that is not up.
3. Commit the failing E2E on its own:
   ```bash
   git commit -m "test(e2e): add failing <feature> scenario"
   ```
4. Boundary crossed → Phase 1. Otherwise → Phase 2.

## Phase 1: CDC contract check

Run this once Phase 0's outer E2E is RED. CDC covers the request/response shape at each boundary,
not the journey. The change crosses a boundary if it:

- modifies a proto file (then run `buf lint` + `buf breaking`)
- modifies a request/response format between services
- adds or modifies an HTTP endpoint another service consumes
- modifies Ollama options or LLM parameters
- introduces or changes a **required header** (`X-Service-Token`, `Authorization`, `X-Api-Key`, a
  tracing header treated as required)
- promotes or demotes an **auth requirement** (optional → required, basic → JWT, JWT → mTLS)
- flips **mTLS** from opt-in to enforced (`MTLS_ENFORCE` default, peer allowlist, CA bundle path)

If none apply, skip to Phase 2. Otherwise:

1. **Consumer side first** — write/update the Pact consumer test in the calling service.
2. **Run the consumer test** → generates the pact JSON under `<service>/pacts/`.
3. **Provider side** — run provider verification against that pact file.
4. **Proto changes** — `cd proto && buf lint && buf breaking --against '.git#branch=main'`.

Pairs, locations and commands live in `references/cdc-map.md`.

## Phase 1b: Provider adds a requirement

When the change is on the **provider** side and tightens what consumers must send (new required
header, new required field, stricter auth, mTLS promotion), consumer-driven contracts protect you
only if every consumer has a pact *and* this change verifies it. In April 2026 `search-indexer`
promoted `X-Service-Token` to required (ADR-000722) while neither `alt-backend` nor
`rag-orchestrator` had a pact carrying it, so provider verification stayed green and the 401 cascade
surfaced in production instead.

Work the "provider tightens a requirement" playbook in `references/cdc-map.md`: enumerate every
consumer, audit `pacts/` for each one, pin the new requirement in each pact, make provider
verification list the union of them, run `./scripts/pact-check.sh`, then smoke the rebuilt stack for
401 / TLS handshake / 403 / 500. A consumer with no pact is contract-unprotected — the change does
not merge until it has one. The same file's boundary-change checklist covers the wiring (mTLS peer
allowlist, `SERVICE_TOKEN` plumbing, cert paths in the container) that unit tests cannot see.

## Phase 2: RED

Define expected behavior through unit tests before implementing. For feature work, enter this phase
only once Phase 0 — and Phase 1 if a boundary is crossed — are RED.

1. **Detect language and service** from `go.mod` / `pyproject.toml` / `Cargo.toml` / `package.json` /
   `deno.json`, and identify the Clean Architecture layer from the feature description.
2. **Create the test file** (see the conventions table below, or a bundled `templates/*.tmpl`).
   Test behavior — success, error and edge cases — not file or symbol existence.
3. **Write the implementation stub first**: declare the signature with explicit argument and return
   types, then fill the body with an unimplemented marker, so the test fails on behavior rather than
   a missing symbol — Go `panic("not implemented")`, Python `raise NotImplementedError`,
   TypeScript `throw new Error("not implemented")`, Rust `unimplemented!()`.
4. **Verify the test fails for the right reason** — not a syntax or import error. If it passes with
   no implementation, rewrite it.
5. **Commit the tests:**
   ```bash
   git commit -m "test(<service>): add failing tests for <feature>"
   ```

## Phase 3: GREEN

Write only enough code to pass the tests.

- Do not edit a test to make it pass. A test changed to fit the implementation has stopped being a
  specification.
- Do not add behavior no test covers.
- Check layer direction: Handler imports Usecase and Port; Usecase imports Port; Gateway imports
  Port and Driver.
- **GREEN includes wiring.** Unit tests construct the component themselves, so they pass even when
  `main`/`di` never wires it into the production pipeline. Grep the constructor in the composition
  root and confirm the `*_enabled` startup log exists (CLAUDE.md Rule 8, `.claude/rules/di-wiring.md`).
  The 2026-07 full-repo review found this gap in 6+ services.

## Phase 4: REFACTOR

Improve naming, remove duplication, simplify — rerunning tests after each change. If Phase 1
detected a boundary change, rerun the CDC consumer tests and provider verification, or
`./scripts/pact-check.sh` for a full sweep.

```bash
git commit -m "feat(<service>): implement <feature>"
```

## Phase 5: Local CI parity

Reproduce each touched service's CI gates locally as the last step before reporting the work
complete. Phases 0-4 prove the tests pass; Phase 5 proves the **formatters, linters, static
analyzers and security scanners** pass too — those are what actually block PRs. Skipping this is the
most common cause of "green locally, red in CI": a stray unused import, format drift, or a
golangci-lint rule that only runs in CI.

1. **Enumerate every service directory touched** (`git diff --name-only` against the branch point).
2. **Run each touched service's gate** from `references/ci-parity.md`.
3. **If `proto/**` changed**, run the proto gate regardless of which services were touched.
4. **If any contract test or consumer-provider interaction changed**, run `./scripts/pact-check.sh`.
5. **Never suppress a failing gate to unblock the task.** A `// nolint`, a loosened ruff config or a
   skipped test greens the gate while leaving the defect in place, and the next reader has no signal
   that anything was traded away. Fix the cause or escalate.

## Per-language map

| Language | Detection | Unit test | Unit test file | CDC contract test |
|---|---|---|---|---|
| Go | `go.mod` | `go test ./...` | `*_test.go`, same package | `driver/contract/` or `internal/adapter/contract/` |
| Python | `pyproject.toml` | `uv run pytest` | `tests/test_*.py` | `tests/contract/` |
| Rust | `Cargo.toml` | `cargo test` | `#[cfg(test)]` or `tests/*.rs` | `src/clients/*_contract.rs` (`#[ignore]`) |
| TypeScript | `bun.lockb` | `bun test` | `*.test.ts` / `*.spec.ts` | `src/test/contracts/` |
| Deno | `deno.json` | `deno test` | `tests/*_test.ts` | — |

Go CDC tests need `CGO_ENABLED=1 go test -tags=contract`; Rust needs `-- --ignored`. E2E runner
commands are in `references/e2e-playbooks.md`, per-pair CDC commands in `references/cdc-map.md`.

## Gotchas

Failure modes this repo has actually hit; the generic TDD rules are assumed.

1. **Writing unit tests first and backfilling E2E at the end.** The outer test drives the design of
   the inner layers; added last, it only ratifies whatever was built.
2. **Skipping Phase 0 because "CDC already covers it".** CDC verifies per-boundary request/response
   shape; Phase 0 verifies the journey. Neither substitutes for the other.
3. **Treating mTLS / auth / required-header changes as "infra".** They change the request contract,
   so they start with a failing consumer test like any other contract change.
4. **Leaving a consumer without a pact for a protected provider.** Provider verification can only
   reject regressions for contracts it can see (Phase 1b).
5. **Using RED to prove a symbol is missing** rather than that behavior is wrong — write the stub
   first (Phase 2 step 3).
6. **Sending different LLM options from different request paths**, or bypassing the semaphore for
   GPU shared resources.
7. **Writing a Playwright spec that depends on another spec having run.** `fullyParallel: true`
   distributes individual tests across workers and shards, so seed what you read under a
   `workerToken` / `testToken` name nothing else uses. The Hurl suites needed `--jobs 1` for exactly
   this reason; the port was the occasion to remove the dependency, not carry it over
   (ADR-000765, `knowledge-sovereign`).
8. **Declaring work complete without Phase 5**, or suppressing a Phase 5 failure to finish.
