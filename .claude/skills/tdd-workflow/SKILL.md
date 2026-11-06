---
name: tdd-workflow
description: Drives Alt changes test-first in outside-in order — E2E (Playwright) → CDC (Pact) → Unit (RED-GREEN-REFACTOR) — and closes with a local CI parity sweep (format/lint/type/security) for every touched microservice. Use when implementing a feature, fixing a bug, or refactoring, when deciding what test to write first, or when the user says "TDDで", "TDD で実装", or "失敗テストから". Also use before declaring work complete, since "tests pass" is not "CI will pass".
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
`$ARGUMENTS` carries the feature description; `--service=<dir>` pins the target service.

**Commit granularity (`~/.claude/CLAUDE.md`):** Do not commit per step during implementation; commit once after all steps complete. The sole TDD exception is separating **RED** and **GREEN** into two commits: one RED commit after all failing tests (E2E + CDC + Unit) are written, and one GREEN commit after implementation, refactoring, and Phase 5 CI parity succeed.

Bundled material, read when the phase calls for it:

- [references/e2e-playbooks.md](references/e2e-playbooks.md) — Phase 0: where specs live, runner commands, and Playwright idioms
- [references/cdc-map.md](references/cdc-map.md) — Phase 1 & 1b: consumer-provider pairs, test paths, commands, and provider requirement tightening
- [references/ci-parity.md](references/ci-parity.md) — Phase 5: per-service local gates that match CI
- [references/gotchas.md](references/gotchas.md) — Repo-specific TDD failure modes to check before writing specs or diagnosing flaky/false-green tests
- `templates/go_usecase_test.go.tmpl`, `templates/python_handler_test.py.tmpl`,
  `templates/typescript_component_test.ts.tmpl`, `templates/deno_unit_test.ts.tmpl` — Phase 2 starting shapes

## Phase 0: E2E first

Write the outermost failing acceptance test:

- Browser UI (Svelte component, page, user flow) → **Playwright**
- HTTP endpoint, Connect-RPC method, or service-to-service flow → **a Playwright API suite**
- Full-stack (FE calls a new BE endpoint) → **both**: one browser journey + one API spec
- Inner-layer refactor with no external behavior change → skip to Phase 2

1. Read [references/e2e-playbooks.md](references/e2e-playbooks.md) and `e2e/playwright/README.md`, then write the failing E2E following a neighboring spec.
2. Run it and confirm RED for the *right* reason — missing behavior, not a syntax error or stopped container. Do not commit yet; hold until Phase 2 RED completes.
3. Boundary crossed → Phase 1. Otherwise → Phase 2.

## Phase 1: CDC contract check

Run once Phase 0's outer E2E is RED. A change crosses a boundary if it modifies a proto file (`buf lint` + `buf breaking`), a request/response format, an HTTP/Connect-RPC endpoint consumed by another service, Ollama/LLM options, a required header (`X-Service-Token`, `Authorization`, `X-Api-Key`), an auth requirement, or mTLS enforcement (`MTLS_ENFORCE`, peer allowlist, CA bundle).

If none apply, skip to Phase 2. Otherwise read [references/cdc-map.md](references/cdc-map.md) and:

1. **Consumer side first** — write/update the Pact consumer test in the calling service and generate `<service>/pacts/`.
2. **Provider side** — run provider verification against that pact file.
3. **Proto changes** — `cd proto && buf lint && buf breaking --against '.git#branch=main'`.
4. **Provider tightens a requirement (Phase 1b)** — if a provider adds a required header/field, stricter auth, or mTLS promotion, follow the Phase 1b playbook and boundary-change checklist in [references/cdc-map.md](references/cdc-map.md) so every consumer has a pact pinning the new requirement.

## Phase 2: RED

Define expected behavior through unit tests before implementing. Enter this phase only once Phase 0 (and Phase 1 if a boundary is crossed) are RED.

1. **Detect language and service** from `go.mod` / `pyproject.toml` / `Cargo.toml` / `package.json` / `deno.json`, and identify the Clean Architecture layer.
2. **Create the test file** (see table below, or `templates/*.tmpl`). Test behavior — success, error, and edge cases — not symbol existence.
3. **Write the implementation stub first**: declare the signature with explicit types and an unimplemented body (`panic("not implemented")`, `raise NotImplementedError`, `throw new Error("not implemented")`, `unimplemented!()`).
4. **Verify the test fails for the right reason.** If it passes with no implementation, rewrite it.
5. **Commit the RED tests together** (Phase 0 + Phase 1 + Phase 2 tests in a single RED commit):
   ```bash
   git commit -m "test(<service>): add failing tests for <feature>"
   ```

## Phase 3: GREEN

Write only enough code to pass the tests.

- Do not edit a test to make it pass. Do not add behavior no test covers.
- Check layer direction: Handler imports Usecase and Port; Usecase imports Port; Gateway imports Port and Driver.
- **GREEN includes wiring.** Grep the constructor in the composition root (`main`/`di`) and confirm the `*_enabled` startup log exists (`CLAUDE.md` Rule 8, `.claude/rules/di-wiring.md`).

## Phase 4: REFACTOR

Improve naming, remove duplication, and simplify — rerunning tests after each change. If Phase 1 detected a boundary change, rerun CDC consumer tests and provider verification (or `./scripts/pact-check.sh`). Hold the implementation commit until Phase 5 passes.

## Phase 5: Local CI parity & GREEN commit

Reproduce each touched service's CI gates locally before committing implementation or reporting completion:

1. **Enumerate every service directory touched** (`git diff --name-only` against the branch point).
2. **Run each touched service's gate** from [references/ci-parity.md](references/ci-parity.md).
3. **If `proto/**` changed**, run the proto gate. **If any contract changed**, run `./scripts/pact-check.sh`.
4. **Never suppress a failing gate** (`// nolint`, loosened ruff config, skipped test) to unblock the task.
5. **Commit the GREEN + REFACTOR implementation once all steps and gates pass:**
   ```bash
   git commit -m "feat(<service>): implement <feature>"
   ```

## Per-language map

| Language | Detection | Unit test | Unit test file | CDC contract test |
|---|---|---|---|---|
| Go | `go.mod` | `go test ./...` | `*_test.go`, same package | `driver/contract/` or `internal/adapter/contract/` (`CGO_ENABLED=1 go test -tags=contract`) |
| Python | `pyproject.toml` | `uv run pytest` | `tests/test_*.py` | `tests/contract/` |
| Rust | `Cargo.toml` | `cargo test` | `#[cfg(test)]` or `tests/*.rs` | `src/clients/*_contract.rs` (`cargo test -- --ignored`) |
| TypeScript | `bun.lock` | `bun run test` | `*.test.ts` / `*.spec.ts` | `src/test/contracts/` |
| Deno | `deno.json` | `deno test` | `tests/*_test.ts` | — |

Before writing new specs or debugging unexpected test behavior, read [references/gotchas.md](references/gotchas.md).
