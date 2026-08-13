# CDC Contract Map (Pact)

Which consumer-provider pairs exist, where their tests live, and how to run them.
Load this when Phase 1 determines the change crosses a service boundary.

- [Consumers and the providers they contract](#consumers-and-the-providers-they-contract)
- [Providers and the consumers they verify](#providers-and-the-consumers-they-verify)
- [Commands](#commands)
- [Provider tightens a requirement (Phase 1b)](#provider-tightens-a-requirement-phase-1b)
- [Boundary change checklist](#boundary-change-checklist)

## Consumers and the providers they contract

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
| alt-butterfly-facade → alt-backend | Go | `alt-butterfly-facade/internal/handler/contract/` |
| auth-hub → kratos | Go | `auth-hub/internal/adapter/gateway/contract/` |
| acolyte-orchestrator → search-indexer | Python | `acolyte-orchestrator/tests/contract/` |

## Providers and the consumers they verify

Use this for a **provider-side** change to confirm every consumer is under contract.

| Provider | Consumers whose pacts the provider verifies | Provider verification location |
|----------|---------------------------------------------|--------------------------------|
| alt-backend | recap-worker | `alt-backend/app/driver/contract/provider_test.go` |
| search-indexer | rag-orchestrator, alt-backend (⚠ an acolyte-orchestrator pact exists but is not yet verified — missing `X-Service-Token` assertion) | `search-indexer/app/driver/contract/provider_test.go` |
| news-creator | pre-processor, rag-orchestrator, recap-worker, acolyte-orchestrator | `news-creator/app/tests/contract/` |
| recap-subworker | recap-worker | `recap-subworker/tests/contract/` |
| tag-generator | recap-worker, mq-hub | `tag-generator/app/tests/contract/` |
| kratos | auth-hub | (external — consumer-only) |

## Commands

```bash
# Go consumer tests (generates pact files)
cd alt-backend/app && CGO_ENABLED=1 go test -tags=contract ./driver/preprocessor_connect/contract/ -v
cd pre-processor/app && CGO_ENABLED=1 go test -tags=contract ./driver/contract/ -v
cd rag-orchestrator && CGO_ENABLED=1 go test -tags=contract ./internal/adapter/contract/ -v
cd search-indexer/app && CGO_ENABLED=1 go test -tags=contract ./driver/contract/ -v
cd mq-hub/app && CGO_ENABLED=1 go test -tags=contract ./driver/contract/ -v

# Rust consumer tests
cd recap-worker/recap-worker && cargo test --lib contract -- --ignored

# Python consumer tests
cd recap-evaluator && uv run pytest tests/contract/ -v --no-cov

# Python provider verification (validates against pact files or Broker)
cd news-creator/app && SERVICE_SECRET=test-secret uv run pytest tests/contract/ -v
cd recap-subworker && SERVICE_SECRET=test-secret uv run pytest tests/contract/ -v
cd tag-generator/app && SERVICE_SECRET=test-secret uv run pytest tests/contract/ -v

# Go provider verification
cd alt-backend/app && CGO_ENABLED=1 go test -tags=contract ./driver/contract/ -v

# Full contract regression (all consumers + all providers)
./scripts/pact-check.sh            # file-based mode, no Broker, fast
./scripts/pact-check.sh --broker   # Broker mode with can-i-deploy semantics

# Proto breaking change check
cd proto && buf lint && buf breaking --against '.git#branch=main'
```

## Provider tightens a requirement (Phase 1b)

Run this when the provider starts demanding something new — a required header, a required field,
stricter auth, mTLS promotion.

1. **Enumerate all consumers** of the affected endpoint / RPC / service, recording service name,
   file path and REST-vs-Connect-RPC:
   ```bash
   grep -rn "<provider-service-name>" --include="*.go" --include="*.py" --include="*.rs" --include="*.ts"
   grep -rn "<PROVIDER_URL_ENV_VAR>" .
   grep -rn "<generated-client-package>" .
   ```
2. **Audit `pacts/` for each caller.** A pact file is named `<consumer>-<provider>.json`; check
   `pacts/`, `<consumer>/pacts/`, and `<consumer>/app/pacts/` (Go services that chdir into `app/`).
   A missing pact means that consumer is contract-unprotected — the change does not merge until a
   consumer contract test exists.
3. **Update each existing pact** so it pins the new requirement explicitly (e.g.
   `matchers.Like("token")` on `X-Service-Token`), then rerun the consumer test to regenerate it.
4. **Provider verifies the union of pacts** — the provider's verification test lists every consumer
   pact; a new consumer contract means adding it here too.
5. **Run the contract regression gate** (`./scripts/pact-check.sh`, or `--broker` for can-i-deploy
   semantics). If one consumer cannot satisfy the new requirement yet, stage the rollout with
   pending pacts rather than disabling that consumer's test.
6. **Runtime smoke** — rebuild (`docker compose up --build -d <provider> <consumers...>`) and tail
   the consumer logs for 401 / TLS handshake / 403 / 500.

## Boundary change checklist

Verify these when modifying service-to-service communication. Each line is here because it failed in
production at least once.

- [ ] **Proto compatibility**: `buf breaking` passes
- [ ] **Options consistency**: LLM parameters match across all request paths
- [ ] **Semaphore routing**: GPU requests go through HybridPrioritySemaphore
- [ ] **Content-type handling**: proxy layers detect every Connect-RPC serialization format
- [ ] **CDC tests updated**: consumer expectations match provider implementation
- [ ] **Every consumer sends the required headers**, so provider verification can reject a regression
- [ ] **mTLS peer allowlist includes every new caller** (the provider's `VerifyConnection` or
      equivalent lists the new caller's CN/SAN)
- [ ] **Service token wired end-to-end**: `SERVICE_TOKEN` / `SERVICE_TOKEN_FILE` /
      `SERVICE_SECRET_FILE` is set in the compose unit **and** read by the config loader **and**
      passed to the outbound client constructor
- [ ] **CA bundle and cert paths exist in the container**: check `filepath.Clean` on env-driven cert
      paths and confirm the file is in the compose `secrets:` or bind-mount list
- [ ] **Provider verification lists every consumer pact** and is wired into `./scripts/pact-check.sh`
- [ ] **New component is wired into the composition root** (SKILL.md Phase 3)

## Pact references

- Handling authentication and authorization — https://docs.pact.io/provider/handling_auth
- Pending pacts — https://docs.pact.io/pact_broker/advanced_topics/pending_pacts
- Webhooks (`contract_requiring_verification_published`) — https://docs.pact.io/pact_broker/webhooks
- Can I Deploy — https://docs.pact.io/pact_broker/can_i_deploy
- Contract tests vs functional tests — https://docs.pact.io/consumer/contract_tests_not_functional_tests
- PactFlow compatibility checks — https://docs.pactflow.io/docs/bi-directional-contract-testing/compatibility-checks/
