# CDC Contract Map (Pact)

Which consumer-provider pairs exist, where their tests live, and how to run them.
Load this when Phase 1 determines the change crosses a service boundary.

- [Consumers and the providers they contract](#consumers-and-the-providers-they-contract)
- [Providers and the consumers they verify](#providers-and-the-consumers-they-verify)
- [Commands](#commands)
- [Provider tightens a requirement (Phase 1b)](#provider-tightens-a-requirement-phase-1b)
- [Boundary change checklist](#boundary-change-checklist)
- [Pact references](#pact-references)

## Consumers and the providers they contract

Direction reads `A → B` as "A consumes B" (so `A`'s `pacts/A-B.json` is the contract `B` must satisfy).

| A (consumer) → B (provider) | Language | Consumer test location |
|-----------------------------|----------|------------------------|
| alt-backend → pre-processor | Go | `alt-backend/app/orchestrator/driver/preprocessor_connect/contract/` |
| alt-backend → search-indexer | Go | `alt-backend/app/orchestrator/driver/search_indexer_connect/contract/` |
| alt-backend → knowledge-sovereign | Go | `alt-backend/app/shared/driver/sovereign_client/contract/` |
| alt-backend → rag-orchestrator | Go | `alt-backend/app/orchestrator/gateway/rag_gateway/contract/` |
| alt-backend → recap-worker | Go | `alt-backend/app/orchestrator/gateway/recap_gateway/contract/` |
| alt-backend → alt-data-hub | Go | `alt-backend/app/shared/gateway/datahub_gateway/contract/` |
| alt-harvester → alt-data-hub | Go | `alt-backend/app/shared/gateway/datahub_gateway/contract/` |
| pre-processor → news-creator, alt-backend, mq-hub | Go | `pre-processor/app/driver/contract/` |
| rag-orchestrator → alt-data-hub, knowledge-sovereign, recap-worker, news-creator, search-indexer | Go | `rag-orchestrator/internal/adapter/contract/` |
| search-indexer → alt-backend, recap-worker, mq-hub | Go | `search-indexer/app/driver/contract/` |
| alt-butterfly-facade → alt-backend | Go | `alt-butterfly-facade/internal/handler/contract/` |
| altctl → knowledge-sovereign | Go | `altctl/internal/sovereignclient/contract/` |
| recap-worker → news-creator, recap-subworker, alt-backend, tag-generator, knowledge-sovereign, alt-data-hub | Rust | `recap-worker/recap-worker/src/clients/*_contract.rs`, `src/clients/<name>/contract.rs` (datahub, knowledge_sovereign, news_creator) |
| recap-evaluator → recap-worker | Python | `recap-evaluator/tests/contract/` |
| tag-generator → alt-backend, mq-hub | Python | `tag-generator/app/tests/contract/` |
| acolyte-orchestrator → news-creator, search-indexer | Python | `acolyte-orchestrator/tests/contract/` |

## Providers and the consumers they verify

Use this for a **provider-side** change to confirm every consumer is under contract.

| Provider | Consumers whose pacts the provider verifies | Provider verification location |
|----------|---------------------------------------------|--------------------------------|
| alt-backend | recap-worker, tag-generator, search-indexer, alt-butterfly-facade, pre-processor | `alt-backend/app/dataplane/driver/contract/` |
| alt-data-hub | alt-backend, alt-harvester, rag-orchestrator, recap-worker | `alt-backend/app/dataplane/driver/contract/` |
| knowledge-sovereign | alt-backend, rag-orchestrator, recap-worker, altctl | `knowledge-sovereign/app/driver/contract/` |
| search-indexer | rag-orchestrator, alt-backend, acolyte-orchestrator | `search-indexer/app/driver/contract/` |
| news-creator | pre-processor, rag-orchestrator, recap-worker, acolyte-orchestrator | `news-creator/app/tests/contract/` |
| recap-subworker | recap-worker | `recap-subworker/tests/contract/` |
| tag-generator | recap-worker, mq-hub | `tag-generator/app/tests/contract/` |
| pre-processor | alt-backend | `pre-processor/app/driver/contract/` |
| mq-hub | pre-processor, search-indexer, tag-generator | `mq-hub/app/driver/contract/` |
| rag-orchestrator | alt-backend | `rag-orchestrator/internal/adapter/contract/` |
| recap-worker | alt-backend, recap-evaluator, search-indexer, rag-orchestrator | `recap-worker/recap-worker/tests/provider_verification.rs` |

## Commands

```bash
# Go consumer tests (generates pact files)
cd alt-backend/app && CGO_ENABLED=1 go test -tags=contract ./orchestrator/driver/preprocessor_connect/contract/ -v
cd alt-backend/app && CGO_ENABLED=1 go test -tags=contract ./shared/driver/sovereign_client/contract/ -v
cd alt-backend/app && CGO_ENABLED=1 go test -tags=contract ./orchestrator/gateway/rag_gateway/contract/ -v
cd alt-backend/app && CGO_ENABLED=1 go test -tags=contract ./orchestrator/gateway/recap_gateway/contract/ -v
cd alt-backend/app && CGO_ENABLED=1 go test -tags=contract ./shared/gateway/datahub_gateway/contract/ -v
cd pre-processor/app && CGO_ENABLED=1 go test -tags=contract ./driver/contract/ -v
cd rag-orchestrator && CGO_ENABLED=1 go test -tags=contract ./internal/adapter/contract/ -v
cd search-indexer/app && CGO_ENABLED=1 go test -tags=contract ./driver/contract/ -v
cd alt-butterfly-facade && CGO_ENABLED=1 go test -tags=contract ./internal/handler/contract/ -v
cd altctl && CGO_ENABLED=1 go test -tags=contract ./internal/sovereignclient/contract/ -v

# Rust consumer tests
cd recap-worker/recap-worker && cargo test --lib contract -- --ignored

# Python consumer tests
cd recap-evaluator && uv run pytest tests/contract/ -v --no-cov
cd tag-generator/app && uv run pytest tests/contract/test_mqhub_tags_consumer.py tests/contract/test_datahub_consumer.py -v --no-cov
cd acolyte-orchestrator && uv run pytest tests/contract/ -v --no-cov

# Python provider verification (validates against pact files or Broker)
cd news-creator/app && uv run pytest tests/contract/ -v
cd recap-subworker && SERVICE_SECRET=test-secret uv run pytest tests/contract/ -v
cd tag-generator/app && SERVICE_SECRET=test-secret uv run pytest tests/contract/ -v

# Go provider verification
cd alt-backend/app && CGO_ENABLED=1 go test -tags=contract ./dataplane/driver/contract/ -v
cd alt-backend/app && CGO_ENABLED=1 go test -tags=contract -run DataHubContract ./dataplane/driver/contract/ -v
cd search-indexer/app && CGO_ENABLED=1 go test -tags=contract -run TestVerifySearchIndexerProviderContracts ./driver/contract/ -v
cd pre-processor/app && CGO_ENABLED=1 go test -tags=contract -run TestVerifyAltBackendContract ./driver/contract/ -v
cd mq-hub/app && CGO_ENABLED=1 go test -tags=contract ./driver/contract/ -v
cd knowledge-sovereign/app && CGO_ENABLED=1 go test -tags=contract ./driver/contract/ -v
cd rag-orchestrator && CGO_ENABLED=1 go test -tags=contract -run TestVerifyAltBackendRagProviderContracts ./internal/adapter/contract/ -v

# Rust provider verification
cd recap-worker/recap-worker && cargo test --test provider_verification -- --ignored

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
