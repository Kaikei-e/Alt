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

3. **If no service boundary:** Skip to Phase 1.

## Phase 0b: PROVIDER-ADDS-REQUIREMENT PLAYBOOK

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

Missing this playbook caused the April 2026 "RAG dead / Augur falls over" incident: `search-indexer` promoted `X-Service-Token` to required (ADR-000722), but neither `alt-backend` nor `rag-orchestrator` had a consumer pact with `search-indexer`. Pact CDCT was installed but the provider verification could not see those consumers, so the 401 cascade only surfaced in production.

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

Use this table when planning a **provider-side change** (Phase 0b) to confirm every consumer is under contract.

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
11. **Tightening a provider's requirements (new required header, new required field, auth promotion) without updating every consumer's Pact to pin the new requirement** — see Phase 0b
12. **Treating mTLS / auth / required-header changes as "infra" and skipping Phase 0** — they change the request contract and must start with a failing consumer test
13. **Leaving a consumer without a pact for a protected provider** — if a provider enforces auth, every caller must have a pact that asserts the auth is present, so provider verification can reject regressions

## References

- Pact: Handling authentication and authorization — https://docs.pact.io/provider/handling_auth
- Pact: Pending pacts — https://docs.pact.io/pact_broker/advanced_topics/pending_pacts
- Pact: Webhooks (`contract_requiring_verification_published`) — https://docs.pact.io/pact_broker/webhooks
- Pact: Can I Deploy — https://docs.pact.io/pact_broker/can_i_deploy
- PactFlow: Compatibility Checks — https://docs.pactflow.io/docs/bi-directional-contract-testing/compatibility-checks/
