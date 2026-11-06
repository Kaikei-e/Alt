# TDD Workflow Gotchas (Alt Repo History)

Failure modes this repository has actually hit in production or CI. Load this reference before writing a new test suite or when a test passes/fails for an unexpected reason.

1. **Writing unit tests first and backfilling E2E at the end.** The outer test drives the design of
   the inner layers; added last, it only ratifies whatever was built.
2. **Skipping Phase 0 because "CDC already covers it".** CDC verifies per-boundary request/response
   shape; Phase 0 verifies the journey. Neither substitutes for the other.
3. **Treating mTLS / auth / required-header changes as "infra".** They change the request contract,
   so they start with a failing consumer test like any other contract change.
4. **Leaving a consumer without a pact for a protected provider.** Provider verification can only
   reject regressions for contracts it can see (Phase 1b). In April 2026 `search-indexer` promoted
   `X-Service-Token` to required (ADR-000722) while neither `alt-backend` nor `rag-orchestrator` had
   a pact carrying it, so provider verification stayed green and the 401 cascade surfaced in
   production instead.
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
9. **Committing per step during implementation.** Do not create separate commits for each sub-step
   (e.g., a standalone Phase 0 E2E commit before Phase 1/2). Only RED (all failing tests) and GREEN
   (implementation + refactor after local CI parity) are separated into two commits.
