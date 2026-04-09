"""Consumer contract tests for acolyte-orchestrator → news-creator.

Verifies that acolyte-orchestrator's expectations of the news-creator
LLM generation API are documented as Pact contracts.

Run with:
    cd acolyte-orchestrator && uv run pytest tests/contract/ -v --no-cov
"""

import json
from pathlib import Path

import httpx
from pact import Pact

PACT_DIR = Path(__file__).resolve().parent.parent.parent.parent / "pacts"


def _new_pact() -> Pact:
    return Pact("acolyte-orchestrator", "news-creator")


def test_generate_text():
    """Verify contract for POST /api/v1/summarize (text generation)."""
    pact = _new_pact()
    (
        pact.upon_receiving("a text generation request for report writing")
        .given("news-creator is ready with a loaded model")
        .with_request("POST", "/api/v1/summarize")
        .will_respond_with(200)
        .with_body(
            json.dumps(
                {
                    "summary": "This week saw significant developments in AI...",
                    "metadata": {"model": "gemma4-e4b-12k", "tokens": 150},
                }
            ),
            "application/json",
        )
    )

    with pact.serve() as srv:
        resp = httpx.post(
            f"{srv.url}/api/v1/summarize",
            json={
                "content": "Generate executive summary for weekly tech briefing.",
                "article_id": "acolyte-report-gen",
                "priority": "low",
            },
        )
        assert resp.status_code == 200
        data = resp.json()
        assert "summary" in data
        assert "metadata" in data

    pact.write_file(str(PACT_DIR), overwrite=True)


def test_generate_text_with_streaming_disabled():
    """Verify contract for non-streaming summarization."""
    pact = _new_pact()
    (
        pact.upon_receiving("a non-streaming generation request")
        .given("news-creator is ready")
        .with_request("POST", "/api/v1/summarize")
        .will_respond_with(200)
        .with_body(
            json.dumps(
                {
                    "summary": "Market trends indicate...",
                    "metadata": {"model": "gemma4-e4b-12k", "tokens": 200},
                }
            ),
            "application/json",
        )
    )

    with pact.serve() as srv:
        resp = httpx.post(
            f"{srv.url}/api/v1/summarize",
            json={
                "content": "Write a section about market trends.",
                "article_id": "acolyte-section-gen",
                "priority": "low",
                "stream": False,
            },
        )
        assert resp.status_code == 200
        data = resp.json()
        assert "summary" in data

    pact.write_file(str(PACT_DIR), overwrite=True)
