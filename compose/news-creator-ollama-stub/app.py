"""Ollama-compatible stub for news-creator Hurl staging.

Phase 1 surface (this file): only `/api/tags` is meaningful. Returns a
fixed model list so OllamaGateway.list_models() succeeds and `/health`
on news-creator reports `models: [...]`.

Phase 2+ will add `/api/generate`, `/api/chat`, `/api/embed`, and an
`/admin/set-delay` control endpoint. Kept intentionally tiny — this is
not a real LLM, just a contract-shape responder.
"""

from __future__ import annotations

from fastapi import FastAPI

app = FastAPI(title="news-creator-ollama-stub", version="0.1.0")

STUB_MODEL_NAME = "gemma3:4b-it-qat"


@app.get("/api/tags")
async def list_tags() -> dict:
    return {
        "models": [
            {
                "name": STUB_MODEL_NAME,
                "model": STUB_MODEL_NAME,
                "modified_at": "2026-04-18T00:00:00Z",
                "size": 0,
                "digest": "stub-digest",
                "details": {
                    "format": "gguf",
                    "family": "gemma3",
                    "parameter_size": "4B",
                    "quantization_level": "Q4_K_M",
                },
            }
        ]
    }
