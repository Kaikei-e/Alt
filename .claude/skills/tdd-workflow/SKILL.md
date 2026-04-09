---
name: tdd-workflow
description: Test-Driven Development workflow for all languages. Use when implementing new features, fixing bugs, or refactoring code, or when the user says "TDDで". Enforces RED-GREEN-REFACTOR discipline with Pact CDCT-first boundary checks and concrete unit-test stubs.
allowed-tools: Bash, Read, Glob, Grep, Edit, Write
argument-hint: <feature-description> [--service=<dir>]
---

# TDD Workflow

Test-Driven Development workflow following Claude Code best practices.
Use this skill in both Plan mode and implementation mode whenever the task may change code or tests.
When the work is implementation-oriented, read this skill before setting the test order and again before editing code.
It includes Pact CDC (Consumer-Driven Contract) testing for service boundary changes.
For boundary changes, treat the Pact scenario as the first end-to-end test you write.
Use an outside-in order for feature work: `E2E` → `CDCT` → `Unit tests`.

## Arguments

- `$ARGUMENTS` - Feature description and optional flags
- `--service=<dir>` - Target service directory (auto-detected if omitted)

## Phase 0: CONTRACT CHECK (Pact CDCT / E2E First)

**Goal:** Determine if the change touches a service boundary. If yes, write Pact CDCT/E2E tests first.

### Steps

1. **Detect if change crosses service boundaries**
   - Does the change modify a proto file? → Run `buf lint` + `buf breaking`
   - Does the change modify a request/response format between services?
   - Does the change add/modify an HTTP endpoint consumed by another service?
   - Does the change modify Ollama options or LLM parameters?

2. **If service boundary is touched:**
   a. **Consumer side first** — Write/update Pact consumer test in the calling service as the first end-to-end contract test
      - Go services: `pact-go v2` in `<service>/app/driver/contract/` or `<service>/internal/adapter/contract/`
      - TS services: `@pact-foundation/pact` in `tests/contract/`
   b. **Run consumer test** → Generates pact JSON in `<service>/pacts/`
   c. **Provider side** — Run provider verification against the pact file
      - Python: `pact-python` in `tests/contract/test_provider_verification.py`
      - Go: provider verification test in the providing service
   d. **Proto changes** — Run `cd proto && buf lint && buf breaking --against '.git#branch=main'`

3. **If no service boundary:** Skip to Phase 1.

### Existing CDC Tests

| Consumer | Provider | Language | Location |
|----------|----------|----------|----------|
| alt-backend | pre-processor | Go | `alt-backend/app/driver/preprocessor_connect/contract/` |
| pre-processor | news-creator | Go | `pre-processor/app/driver/contract/` |
| rag-orchestrator | news-creator | Go | `rag-orchestrator/internal/adapter/contract/` |
| search-indexer | alt-backend, recap-worker, mq-hub | Go | `search-indexer/app/driver/contract/` |
| mq-hub | search-indexer, tag-generator | Go | `mq-hub/app/driver/contract/` |
| recap-worker | news-creator, recap-subworker, alt-backend, tag-generator | Rust | `recap-worker/recap-worker/src/clients/*_contract.rs` |
| recap-evaluator | recap-worker | Python | `recap-evaluator/tests/contract/` |
| news-creator (provider) | — | Python | `news-creator/app/tests/contract/` |
| recap-subworker (provider) | — | Python | `recap-subworker/tests/contract/` |
| tag-generator (provider) | — | Python | `tag-generator/app/tests/contract/` |
| alt-butterfly-facade | alt-backend, tts-speaker | Go | `alt-butterfly-facade/internal/handler/contract/` |
| auth-hub | kratos | Go | `auth-hub/internal/adapter/gateway/contract/` |
| alt-backend (provider) | — | Go | `alt-backend/app/driver/contract/provider_test.go` |
| tts-speaker (provider) | — | Python | `tts-speaker/tests/contract/` |

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

## Phase 1: RED (Write Failing Test)

**Goal:** Define expected behavior through tests BEFORE implementation.
For feature work, use this phase only after the outer E2E and Pact contract checks have been written or updated.

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

## Phase 2: GREEN (Minimal Implementation)

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

## Phase 3: REFACTOR (Clean Up)

**Goal:** Improve code quality while keeping tests green.

### Steps

1. **Improve Code**
   - Remove duplication
   - Improve naming
   - Simplify logic

2. **Verify Tests Still Pass**
   - Run tests after each change

3. **Contract Regression Check** (if Phase 0 detected boundary change)
   - Re-run CDC consumer tests → Verify pact files are still valid
   - Re-run provider verification → Verify provider still satisfies contracts
   - Or run `./scripts/pact-check.sh` for a full consumer + provider sweep

4. **Final Commit**
   ```bash
   git add <implementation-file>
   git commit -m "feat(<service>): implement <feature>"
   ```

## Test Commands by Language

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
