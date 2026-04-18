"""Ollama-compatible stub for news-creator Hurl staging.

Surface implemented (current = Phase 2):

- `GET  /api/tags`     → fixed model list (Phase 1)
- `POST /api/generate` → fixed Ollama-style response (Phase 2)
- `POST /api/chat`     → fixed non-streaming chat response (Phase 2)

Heuristics:

- If the request body carries a `format` field (Ollama Structured Outputs,
  used by recap-summary's `_call_llm_for_recap` for `RecapSummary` JSON
  schema), the stub returns a content/response that is itself a JSON
  string matching `{title, bullets[], language: "ja"}`. That keeps
  `_parse_summary_json` happy.
- Otherwise the stub returns plain Japanese text long enough (>= 100
  chars) to pass `summarize_usecase`'s `len(stripped) < 10` guard and
  the `min_length: 10` repetition guard.

Phase 3+ will add `/api/embed` (rerank, if it ever proxies through the
Ollama side rather than the local cross-encoder), the streaming
variants (`stream=true` for SSE on `/api/generate` and NDJSON on
`/api/chat`), and an `/admin/set-delay` control endpoint for the
queue-saturation scenario.
"""

from __future__ import annotations

import json

from fastapi import FastAPI, Request

app = FastAPI(title="news-creator-ollama-stub", version="0.2.0")

STUB_MODEL_NAME = "gemma3:4b-it-qat"

# Plain-text answer for non-structured `generate`/`chat` calls. Long
# enough to satisfy the summarize usecase's >= 10 char guard with a
# comfortable margin, and varied enough to avoid the repetition
# detector. Pure ASCII keeps debugging readable while still standing in
# for a Japanese summary.
PLAIN_TEXT_RESPONSE = (
    "stub summary line one. stub summary line two with different words. "
    "stub summary line three closing the response."
)

# Structured response used when a request includes a `format` field —
# recap-summary's chat path or its `generate()` fallback. Shape matches
# `RecapSummary` (`title`, `bullets[]`, `language: "ja"`).
STRUCTURED_PAYLOAD = {
    "title": "stub recap summary title",
    "bullets": [
        "Stub bullet one describing a fictional event for E2E shape tests.",
        "Stub bullet two providing additional varied wording to avoid the repetition guard.",
        "Stub bullet three closing the structured payload with distinct vocabulary.",
    ],
    "language": "ja",
}


def _has_format(payload: dict) -> bool:
    """`format` is set by the recap path (Structured Outputs)."""
    return payload.get("format") is not None


def _resolve_content(payload: dict) -> str:
    """Choose between structured JSON content and plain Japanese text."""
    if _has_format(payload):
        return json.dumps(STRUCTURED_PAYLOAD, ensure_ascii=False)
    return PLAIN_TEXT_RESPONSE


def _resolve_model(payload: dict) -> str:
    requested = payload.get("model")
    if isinstance(requested, str) and requested:
        return requested
    return STUB_MODEL_NAME


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


@app.post("/api/generate")
async def generate(request: Request) -> dict:
    payload = await request.json()
    response_text = _resolve_content(payload)
    return {
        "model": _resolve_model(payload),
        "created_at": "2026-04-18T00:00:00Z",
        "response": response_text,
        "done": True,
        "done_reason": "stop",
        "context": [],
        "total_duration": 1_000_000,
        "load_duration": 100_000,
        "prompt_eval_count": 16,
        "prompt_eval_duration": 200_000,
        "eval_count": 32,
        "eval_duration": 700_000,
    }


@app.post("/api/chat")
async def chat(request: Request) -> dict:
    payload = await request.json()
    content = _resolve_content(payload)
    return {
        "model": _resolve_model(payload),
        "created_at": "2026-04-18T00:00:00Z",
        "message": {
            "role": "assistant",
            "content": content,
        },
        "done": True,
        "done_reason": "stop",
        "total_duration": 1_000_000,
        "load_duration": 100_000,
        "prompt_eval_count": 16,
        "prompt_eval_duration": 200_000,
        "eval_count": 32,
        "eval_duration": 700_000,
    }
