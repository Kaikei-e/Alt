# rag-orchestrator Hurl suite

Hurl-based E2E for the `rag-orchestrator` service, driven against a real `rag-db` Postgres (with `pgvector` + Atlas migrations) brought up through the `rag-orchestrator` profile of `compose/compose.staging.yaml`.

## Scope (Phase 1)

This suite covers the surface that does **not** require news-creator, search-indexer, rerank-external, or alt-backend:

- `/healthz`, `/readyz`, `/connect/health`
- `AugurService` auth enforcement on `X-Alt-User-Id`
- `AugurService` read-only RPCs against an empty database (`ListConversations`, `GetConversation`, `DeleteConversation`)
- Seeded-data read-through (`ListConversations` / `GetConversation` after `psql` seeds two rows)

Out of scope for Phase 1 (require stubs or Go clients):
- Streaming RPCs (`AugurService.StreamChat`, `MorningLetterService.StreamChat`, `/v1/rag/answer/stream`)
- `/v1/rag/retrieve`, `/v1/rag/answer` — need news-creator (Ollama), search-indexer, rerank-external, alt-backend
- `/internal/rag/index/upsert`, `/internal/rag/backfill` — need an embeddings backend

## Running

```bash
IMAGE_TAG=ci GHCR_OWNER=kaikei-e \
  HURL_IMAGE=ghcr.io/orange-opensource/hurl:7.1.0 \
  bash e2e/hurl/rag-orchestrator/run.sh

# debug: keep the stack up on exit
KEEP_STACK=1 bash e2e/hurl/rag-orchestrator/run.sh
ls e2e/reports/rag-orchestrator-*/html/index.html
```

Reports land under `e2e/reports/rag-orchestrator-<run_id>/` (JUnit XML + HTML).
