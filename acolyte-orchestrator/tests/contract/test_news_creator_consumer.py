"""Consumer contract tests for acolyte-orchestrator → news-creator.

Verifies that acolyte-orchestrator's expectations of the news-creator
LLM generation API are documented as Pact contracts.

Run with:
    cd acolyte-orchestrator && uv run pytest tests/contract/ -v --no-cov
"""

import json
from pathlib import Path

import httpx
from pact import Pact, match

PACT_DIR = Path(__file__).resolve().parent.parent.parent.parent / "pacts"

# Content must be >=100 chars to pass news-creator's SummarizeRequest guard
# (see news-creator/app/news_creator/handler/summarize_handler.py:68-74).
_REPORT_PROMPT = (
    "Generate an executive summary for the weekly technology briefing. "
    "Cover AI research breakthroughs, notable infrastructure incidents, and security advisories. "
    "Highlight trends readers should watch next quarter."
)
_SECTION_PROMPT = (
    "Write a market-trends section analysing venture funding flows, the latest GPU supply "
    "dynamics, and what that means for inference costs for small operators over the next six months."
)


def _new_pact() -> Pact:
    return Pact("acolyte-orchestrator", "news-creator")


def _news_creator_response(summary: str) -> dict:
    """Mirror news-creator's SummarizeResponse shape using type matchers.

    Keep this in sync with news-creator/app/news_creator/domain/models.py::SummarizeResponse.
    Matchers assert the shape without pinning the provider's canned values so
    the provider can freely return different strings/integers in its test fixtures.
    """
    return {
        "success": match.boolean(True),
        "article_id": match.string("acolyte-gen"),
        "summary": match.string(summary),
        "model": match.string("gemma4-e4b-12k"),
        "prompt_tokens": match.integer(120),
        "completion_tokens": match.integer(200),
        "total_duration_ms": match.decimal(1234.5),
    }


def test_generate_text():
    """Verify contract for POST /api/v1/summarize (text generation)."""
    pact = _new_pact()
    request_body = {
        "content": _REPORT_PROMPT,
        "article_id": "acolyte-gen",
        "priority": "low",
        "stream": False,
    }
    (
        pact.upon_receiving("a text generation request for report writing")
        .given("news-creator is ready with a loaded model")
        .with_request("POST", "/api/v1/summarize")
        .with_body(json.dumps(request_body), "application/json")
        .will_respond_with(200)
        .with_body(
            _news_creator_response("This week saw significant developments in AI..."),
            "application/json",
        )
    )

    with pact.serve() as srv:
        resp = httpx.post(f"{srv.url}/api/v1/summarize", json=request_body)
        assert resp.status_code == 200
        data = resp.json()
        assert isinstance(data["summary"], str) and data["summary"]
        assert isinstance(data["model"], str)
        assert isinstance(data["completion_tokens"], int)

    pact.write_file(str(PACT_DIR), overwrite=True)


def test_generate_text_with_streaming_disabled():
    """Verify contract for non-streaming summarization."""
    pact = _new_pact()
    request_body = {
        "content": _SECTION_PROMPT,
        "article_id": "acolyte-gen",
        "priority": "low",
        "stream": False,
    }
    (
        pact.upon_receiving("a non-streaming generation request")
        .given("news-creator is ready")
        .with_request("POST", "/api/v1/summarize")
        .with_body(json.dumps(request_body), "application/json")
        .will_respond_with(200)
        .with_body(
            _news_creator_response("Market trends indicate..."),
            "application/json",
        )
    )

    with pact.serve() as srv:
        resp = httpx.post(f"{srv.url}/api/v1/summarize", json=request_body)
        assert resp.status_code == 200
        data = resp.json()
        assert isinstance(data["summary"], str) and data["summary"]
        assert data["success"] is True

    pact.write_file(str(PACT_DIR), overwrite=True)
