# News Creator

_Last reviewed: November 10, 2025_

**Location:** `news-creator/app`

## Role
- FastAPI service that produces summaries and recap blurbs via an Ollama-backed LLM.
- Enforces Clean Architecture boundaries so handlers stay thin and LLM prompts remain testable.

## Service Snapshot
| Layer | Key modules |
| --- | --- |
| Handler | `news_creator/handler/summarize_handler.py`, `generate_handler.py`, `recap_summary_handler.py`, `health_handler.py` |
| Usecase | `news_creator/usecase/summarize_usecase.py`, `recap_summary_usecase.py` |
| Port | `news_creator/port/llm_provider_port.py`, `auth_port.py`, `user_preferences_port.py` |
| Gateway | `news_creator/gateway/ollama_gateway.py` |
| Driver | `news_creator/driver/ollama_driver.py` |
| Domain | `news_creator/domain/models.py`, `prompts.py` |

## Code Status
- `main.py` declares a DI container: loads `NewsCreatorConfig`, constructs `OllamaGateway`, `SummarizeUsecase`, and `RecapSummaryUsecase`, then registers routers with FastAPI. Lifespan hooks (`asynccontextmanager`) initialize/cleanup the gateway to reuse HTTP sessions.
- Handlers are generated through factory helpers (e.g., `create_summarize_router(usecase)`), ensuring inversion of control between API layer and business logic.
- Domain models (Pydantic) validate request payloads (`SummarizeRequest`, `RecapSummaryRequest`) and responses, while `prompts.py` centralizes template text + safety filters.
- `gateway/ollama_gateway.py` adapts usecase calls to driver methods, translating config-provided model names, prediction counts, and safety flags before invoking `driver/ollama_driver.py`.

## Integrations & Data
- **Ollama runtime:** Configured via `NewsCreatorConfig` (endpoint, model, `summary_num_predict`, `recap_num_predict`, fallback strategy). Update envs instead of hardcoding.
- **Dependencies:** Optional auth and user-preference ports can be implemented to personalize prompts; stubbed versions exist for now.
- **Sanitization:** Handlers apply redaction helpers before returning JSON; when adding prompts, ensure they pass through `sanitize_summary`.

## Testing & Tooling
- `uv run pytest` (async) mirrors the production structure: tests in `tests/handler`, `tests/usecase`, `tests/gateway`, `tests/driver`. Use `pytest-asyncio` + `AsyncMock` for gateway/driver stubs.
- Golden prompt datasets live under `tests/domain`—update them when changing prompt templates to avoid regressions.
- Lint/type check with `uv run ruff check` + `uv run mypy`.

## Operational Runbook
1. Start with `docker compose --profile ollama up news-creator` to ensure the Ollama runtime is available.
2. Smoke test: `curl -X POST http://localhost:8001/api/v1/summarize -d '{"article_id":"t","content":"..."}'`.
3. Monitor logs for `ollama_gateway` entries; latency spikes usually indicate GPU contention.
4. When rotating models, update `NEWS_CREATOR_MODEL` env, redeploy, then re-run golden tests.

## Observability
- Logging uses `logging` module configured in `main.py`; messages include `operation`, `model`, `prompt_tokens`.
- Add metrics exporters (e.g., Prometheus) by extending the FastAPI app; current plan is to expose generation duration histograms per model.

## LLM Notes
- When generating code, specify which layer to touch. Example prompt: “Update `news_creator/usecase/summarize_usecase.py` to add metadata...”
- Provide expected request/response schemas so generated handlers remain compatible with domain models.
