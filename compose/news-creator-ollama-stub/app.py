"""Ollama-compatible stub for news-creator Hurl staging.

Surface implemented (current = Phase 3):

- `GET  /api/tags`     → fixed model list (Phase 1)
- `POST /api/generate` → fixed Ollama-style response (Phase 2/3)
- `POST /api/chat`     → fixed non-streaming chat response (Phase 2/3)

Response content is chosen by inspecting the request:

- `format` field present (Ollama Structured Outputs, used by
  recap-summary's `_call_llm_for_recap` for `RecapSummary` JSON
  schema) → return a JSON string matching `{title, bullets[],
  language: "ja"}`.
- `/api/chat` without `format` → return a JSON string matching
  `QueryPlan` (`{reasoning, resolved_query, search_queries[], intent,
  retrieval_policy, answer_format, should_clarify, topic_entities[]}`).
  This satisfies `plan_query_usecase`'s `_extract_json` + `QueryPlan(**parsed)`
  parse and is benign for the generic chat-proxy smoke (the test only
  asserts on the response envelope shape, not the content body).
- `/api/generate` without `format` → return a multi-line ASCII payload.
  `summarize_usecase` rolls the entire string into the response
  `summary`; `expand_query_usecase`'s `_parse_expansion_lines` splits
  by newline and yields the same lines as expanded queries. The
  multi-line shape lets one stub serve both endpoints without
  prompt sniffing.

Phase 4 will add `stream=true` paths (SSE on /api/generate, NDJSON on
/api/chat) and an `/admin/set-delay` control endpoint for the
queue-saturation scenario.

Out of scope for Phase 3: `/v1/rerank`. The cross-encoder
(`BAAI/bge-reranker-v2-m3`, ~568 MB) is downloaded from HuggingFace at
first use, which the `alt-staging` network's `internal: true` flag
blocks. Adding it requires either pre-baking the model into the
news-creator image or punching an egress allowlist — both belong in a
later Phase.
"""

from __future__ import annotations

import json

from fastapi import FastAPI, Request

app = FastAPI(title="news-creator-ollama-stub", version="0.3.0")

STUB_MODEL_NAME = "gemma3:4b-it-qat"

# Multi-line ASCII payload returned by `/api/generate` without `format`.
# - summarize_usecase keeps the whole blob as the response `summary`
#   (>= 10 chars after strip → passes the empty-output guard; varied
#   vocabulary → does not trip the repetition detector).
# - expand_query_usecase splits by newline and yields each non-empty
#   line as an expanded query.
PLAIN_GENERATE_RESPONSE = (
    "stub expansion alpha keyword\n"
    "stub expansion beta keyword\n"
    "stub expansion gamma keyword"
)

# Structured response returned when a `format` field is present
# (recap-summary's chat path or its `generate()` fallback). Shape
# matches `RecapSummary` (`title`, `bullets[]`, `language: "ja"`).
RECAP_PAYLOAD = {
    "title": "stub recap summary title",
    "bullets": [
        "Stub bullet one describing a fictional event for E2E shape tests.",
        "Stub bullet two providing additional varied wording to avoid the repetition guard.",
        "Stub bullet three closing the structured payload with distinct vocabulary.",
    ],
    "language": "ja",
}

# Structured response returned by `/api/chat` without `format`.
# Matches the `QueryPlan` Pydantic model; `plan_query_usecase`
# unwraps it via `_extract_json` + `QueryPlan(**parsed)`.
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


def _has_format(payload: dict) -> bool:
    """`format` is set by the recap path (Structured Outputs)."""
    return payload.get("format") is not None


def _resolve_chat_content(payload: dict) -> str:
    if _has_format(payload):
        return json.dumps(RECAP_PAYLOAD, ensure_ascii=False)
    return json.dumps(QUERY_PLAN_PAYLOAD, ensure_ascii=False)


def _resolve_generate_content(payload: dict) -> str:
    if _has_format(payload):
        return json.dumps(RECAP_PAYLOAD, ensure_ascii=False)
    return PLAIN_GENERATE_RESPONSE


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
    response_text = _resolve_generate_content(payload)
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
    content = _resolve_chat_content(payload)
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
