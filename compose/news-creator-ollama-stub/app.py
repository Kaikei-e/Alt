"""Ollama-compatible stub for news-creator Hurl staging.

Surface implemented (current = Phase 4):

- `GET  /api/tags`     → fixed model list (Phase 1)
- `POST /api/generate` → fixed Ollama-style response, `stream=true` emits NDJSON (Phase 2-4)
- `POST /api/chat`     → fixed non-streaming response, `stream=true` emits NDJSON (Phase 2-4)

Response content selection:

- `format` field present (Ollama Structured Outputs):
  - schema with `sections` + `lead`            → `MorningLetterContent` JSON
  - schema with `bullets` and no `sections`    → `RecapSummary` JSON
  - otherwise                                  → `RecapSummary` JSON (default)
- `/api/chat` without `format`                 → `QueryPlan` JSON
- `/api/generate` without `format`             → multi-line ASCII (works for
  both summarize and expand-query)

Streaming (`stream: true` in body):

- `/api/generate` → NDJSON of 3 chunks. The last chunk has `done: true`.
  Each chunk's `response` field is one ASCII token.
- `/api/chat`     → NDJSON of 3 chunks. The last chunk has `done: true`.
  Each chunk's `message.content` is one ASCII token.

Out of scope of this Phase:

- `/v1/rerank` cross-encoder model load (HuggingFace download blocked
  by the staging network's `internal: true` flag).
- Queue saturation control endpoint (`/admin/set-delay`). Triggering
  the HybridPrioritySemaphore's QueueFullError reliably from a serial
  Hurl suite would need either parallel client orchestration or per-
  invocation env overrides; both add machinery the present scope can't
  justify yet.
"""

from __future__ import annotations

import asyncio
import json
from typing import AsyncIterator

from fastapi import FastAPI, Request
from fastapi.responses import StreamingResponse

app = FastAPI(title="news-creator-ollama-stub", version="0.4.0")

STUB_MODEL_NAME = "gemma3:4b-it-qat"

PLAIN_GENERATE_RESPONSE = (
    "stub expansion alpha keyword\n"
    "stub expansion beta keyword\n"
    "stub expansion gamma keyword"
)

RECAP_PAYLOAD = {
    "title": "stub recap summary title",
    "bullets": [
        "Stub bullet one describing a fictional event for E2E shape tests.",
        "Stub bullet two providing additional varied wording to avoid the repetition guard.",
        "Stub bullet three closing the structured payload with distinct vocabulary.",
    ],
    "language": "ja",
}

MORNING_LETTER_PAYLOAD = {
    "schema_version": 1,
    "lead": "Stub morning letter lead summarizing the day in a single sentence.",
    "sections": [
        {
            "key": "top3",
            "title": "Top 3 Stories",
            "bullets": [
                "Stub top story alpha with distinct vocabulary.",
                "Stub top story beta with distinct vocabulary.",
                "Stub top story gamma with distinct vocabulary.",
            ],
            "genre": None,
            "narrative": None,
        }
    ],
    "generated_at": "2026-04-18T06:00:00+09:00",
    "source_recap_window_days": 3,
}

QUERY_PLAN_PAYLOAD = {
    "reasoning": "Stub planner: the query is treated as a generic topic deep dive for E2E shape tests.",
    "resolved_query": "stub resolved query",
    "search_queries": [
        "stub search query alpha",
        "stub search query beta",
        "stub search query gamma",
    ],
    "intent": "general",
    "retrieval_policy": "global_only",
    "answer_format": "summary",
    "should_clarify": False,
    "topic_entities": ["stub-entity"],
}

# Streaming token sequence — chosen to be ASCII, distinct enough to
# avoid summarize_usecase's repetition guard, and short enough that
# the full NDJSON body fits in a single Hurl body assertion. Three
# tokens give two `done: false` lines + one `done: true` final line.
STREAM_TOKENS = ("stub-token-alpha ", "stub-token-beta ", "stub-token-gamma")


def _has_format(payload: dict) -> bool:
    return payload.get("format") is not None


def _is_morning_letter_schema(format_value: object) -> bool:
    if not isinstance(format_value, dict):
        return False
    properties = format_value.get("properties")
    if not isinstance(properties, dict):
        return False
    return "sections" in properties and "lead" in properties


def _structured_payload_for(format_value: object) -> dict:
    if _is_morning_letter_schema(format_value):
        return MORNING_LETTER_PAYLOAD
    return RECAP_PAYLOAD


def _resolve_chat_content(payload: dict) -> str:
    if _has_format(payload):
        return json.dumps(
            _structured_payload_for(payload.get("format")), ensure_ascii=False
        )
    return json.dumps(QUERY_PLAN_PAYLOAD, ensure_ascii=False)


def _resolve_generate_content(payload: dict) -> str:
    if _has_format(payload):
        return json.dumps(
            _structured_payload_for(payload.get("format")), ensure_ascii=False
        )
    return PLAIN_GENERATE_RESPONSE


def _resolve_model(payload: dict) -> str:
    requested = payload.get("model")
    if isinstance(requested, str) and requested:
        return requested
    return STUB_MODEL_NAME


def _is_streaming(payload: dict) -> bool:
    return bool(payload.get("stream"))


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


def _make_generate_chunk(model: str, token: str, done: bool) -> dict:
    chunk = {
        "model": model,
        "created_at": "2026-04-18T00:00:00Z",
        "response": token,
        "done": done,
    }
    if done:
        chunk["done_reason"] = "stop"
        chunk["context"] = []
        chunk["total_duration"] = 1_000_000
        chunk["load_duration"] = 100_000
        chunk["prompt_eval_count"] = 16
        chunk["prompt_eval_duration"] = 200_000
        chunk["eval_count"] = 32
        chunk["eval_duration"] = 700_000
    return chunk


def _make_chat_chunk(model: str, token: str, done: bool) -> dict:
    chunk = {
        "model": model,
        "created_at": "2026-04-18T00:00:00Z",
        "message": {"role": "assistant", "content": token},
        "done": done,
    }
    if done:
        chunk["done_reason"] = "stop"
        chunk["total_duration"] = 1_000_000
        chunk["load_duration"] = 100_000
        chunk["prompt_eval_count"] = 16
        chunk["prompt_eval_duration"] = 200_000
        chunk["eval_count"] = 32
        chunk["eval_duration"] = 700_000
    return chunk


async def _ndjson_stream(chunks: list[dict]) -> AsyncIterator[bytes]:
    for chunk in chunks:
        yield (json.dumps(chunk, ensure_ascii=False) + "\n").encode("utf-8")
        # Tiny sleep so chunks actually flush separately rather than
        # being coalesced into one TCP packet — matches real Ollama's
        # token-by-token cadence closely enough for shape testing.
        await asyncio.sleep(0)


@app.post("/api/generate")
async def generate(request: Request):
    payload = await request.json()
    model = _resolve_model(payload)
    if _is_streaming(payload):
        chunks = [
            _make_generate_chunk(model, token, done=(i == len(STREAM_TOKENS) - 1))
            for i, token in enumerate(STREAM_TOKENS)
        ]
        return StreamingResponse(
            _ndjson_stream(chunks),
            media_type="application/x-ndjson",
        )
    response_text = _resolve_generate_content(payload)
    return {
        "model": model,
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
async def chat(request: Request):
    payload = await request.json()
    model = _resolve_model(payload)
    if _is_streaming(payload):
        chunks = [
            _make_chat_chunk(model, token, done=(i == len(STREAM_TOKENS) - 1))
            for i, token in enumerate(STREAM_TOKENS)
        ]
        return StreamingResponse(
            _ndjson_stream(chunks),
            media_type="application/x-ndjson",
        )
    content = _resolve_chat_content(payload)
    return {
        "model": model,
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
