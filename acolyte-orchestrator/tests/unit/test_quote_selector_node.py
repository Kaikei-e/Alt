"""Unit tests for QuoteSelectorNode — selects verbatim quotes per article per section."""

from __future__ import annotations

import json

import pytest

from acolyte.port.llm_provider import LLMResponse
from acolyte.usecase.graph.nodes.quote_selector_node import QuoteSelectorNode
from acolyte.usecase.graph.state import ReportGenerationState


class FakeLLM:
    def __init__(self, response_text: str = "") -> None:
        self._response_text = response_text
        self.calls: list[dict] = []

    async def generate(self, prompt: str, **kwargs: object) -> LLMResponse:
        self.calls.append({"prompt": prompt, **kwargs})
        return LLMResponse(text=self._response_text, model="fake")


def _make_quote_response(quotes: list[dict]) -> str:
    return json.dumps({"reasoning": "selected", "quotes": quotes})


def _base_state(
    curated: dict | None = None,
    hydrated: dict | None = None,
    compressed: dict | None = None,
    outline: list | None = None,
) -> ReportGenerationState:
    return {
        "curated_by_section": curated or {},
        "hydrated_evidence": hydrated or {},
        "compressed_evidence": compressed or {},
        "outline": outline or [{"key": "analysis", "search_queries": ["AI trends"]}],
    }


@pytest.mark.asyncio
async def test_produces_quotes_from_article() -> None:
    """1 article × 1 section → selected_quotes has entries with correct source_id."""
    llm = FakeLLM("")
    node = QuoteSelectorNode(llm)

    state = _base_state(
        curated={"analysis": [{"id": "art-1", "title": "Report"}]},
        hydrated={"art-1": "AI market grew 20%. NVIDIA leads GPU."},
    )
    result = await node(state)

    quotes = result["selected_quotes"]
    assert len(quotes) >= 1
    assert all(q["source_id"] == "art-1" for q in quotes)


@pytest.mark.asyncio
async def test_fallback_extracts_sentences() -> None:
    """When LLM fails, sentence extraction produces quotes."""
    llm = FakeLLM("invalid json")
    node = QuoteSelectorNode(llm)

    state = _base_state(
        curated={"analysis": [{"id": "art-1", "title": "Report"}]},
        hydrated={"art-1": "First sentence about AI.\nSecond sentence about GPUs."},
    )
    result = await node(state)

    quotes = result["selected_quotes"]
    assert len(quotes) >= 1
    assert quotes[0]["source_id"] == "art-1"


@pytest.mark.asyncio
async def test_one_article_failure_doesnt_stop_others() -> None:
    """3 articles, LLM fails on art-2 → other articles still produce quotes."""
    call_count = 0

    class FailForArt2(FakeLLM):
        async def generate(self, prompt: str, **kwargs: object) -> LLMResponse:
            nonlocal call_count
            call_count += 1
            if "art-2" in prompt:
                raise TimeoutError("ReadTimeout")
            return LLMResponse(
                text=_make_quote_response([{"text": "fact", "source_id": "art-1" if "art-1" in prompt else "art-3"}]),
                model="fake",
            )

    node = QuoteSelectorNode(FailForArt2())
    state = _base_state(
        curated={
            "analysis": [
                {"id": "art-1", "title": "A1"},
                {"id": "art-2", "title": "A2"},
                {"id": "art-3", "title": "A3"},
            ]
        },
        hydrated={
            "art-1": "Body 1.",
            "art-2": "Body 2.",
            "art-3": "Body 3.",
        },
    )
    result = await node(state)

    # art-1 and art-3 should have quotes; art-2 uses fallback
    assert len(result["selected_quotes"]) >= 2


@pytest.mark.asyncio
async def test_empty_evidence_produces_empty() -> None:
    """No curated articles → selected_quotes=[]."""
    node = QuoteSelectorNode(FakeLLM())
    result = await node(_base_state())
    assert result["selected_quotes"] == []


# --- Heuristic primary path tests ---


@pytest.mark.asyncio
async def test_heuristic_primary_no_llm_call() -> None:
    """Primary heuristic path should NOT call LLM."""
    llm = FakeLLM("should not be called")
    node = QuoteSelectorNode(llm)
    state = _base_state(
        curated={"analysis": [{"id": "art-1", "title": "Report"}]},
        hydrated={"art-1": "AI market grew 20%. NVIDIA leads GPU. Other stuff."},
    )
    result = await node(state)
    assert len(result["selected_quotes"]) >= 1
    assert len(llm.calls) == 0


@pytest.mark.asyncio
async def test_heuristic_japanese_article() -> None:
    """Japanese article produces quotes via heuristic without LLM."""
    llm = FakeLLM("")
    node = QuoteSelectorNode(llm)
    body = (
        "NVIDIAは2026年第1四半期にBlackwell Ultra GPUの量産を開始した。\n"
        "この新チップは推論性能が約3倍に向上している。\n"
        "AMDはMI400シリーズを発表した。"
    )
    state = _base_state(
        curated={"analysis": [{"id": "art-1", "title": "GPU Report"}]},
        hydrated={"art-1": body},
        outline=[{"key": "analysis", "search_queries": ["NVIDIA GPU チップ"]}],
    )
    result = await node(state)
    quotes = result["selected_quotes"]
    assert len(quotes) >= 1
    assert all(q["source_id"] == "art-1" for q in quotes)
    assert len(llm.calls) == 0


@pytest.mark.asyncio
async def test_heuristic_newline_heavy_article() -> None:
    """Newline-heavy article (bullet points) produces quotes."""
    llm = FakeLLM("")
    node = QuoteSelectorNode(llm)
    body = (
        "Key Findings\n• AI chip market grows 45%\n• NVIDIA market share 80%\n• AMD catching up\n• Intel enters market"
    )
    state = _base_state(
        curated={"analysis": [{"id": "art-1", "title": "Bullets"}]},
        hydrated={"art-1": body},
        outline=[{"key": "analysis", "search_queries": ["AI chip market"]}],
    )
    result = await node(state)
    quotes = result["selected_quotes"]
    assert 1 <= len(quotes) <= 3
    assert len(llm.calls) == 0


@pytest.mark.asyncio
async def test_heuristic_punctuation_heavy_article() -> None:
    """Punctuation-heavy article (abbreviations, decimals) produces quotes."""
    llm = FakeLLM("")
    node = QuoteSelectorNode(llm)
    body = "U.S. chip exports rose 3.14%. The A.I. market hit $100B. TSMC's N3E process yields 92.5%."
    state = _base_state(
        curated={"analysis": [{"id": "art-1", "title": "Data"}]},
        hydrated={"art-1": body},
        outline=[{"key": "analysis", "search_queries": ["chip exports market"]}],
    )
    result = await node(state)
    quotes = result["selected_quotes"]
    assert len(quotes) >= 1
    assert len(llm.calls) == 0


@pytest.mark.asyncio
async def test_heuristic_quote_count_capped() -> None:
    """Heuristic returns at most 3 quotes per article."""
    llm = FakeLLM("")
    node = QuoteSelectorNode(llm)
    body = "\n".join([f"Sentence {i} about AI chips." for i in range(20)])
    state = _base_state(
        curated={"analysis": [{"id": "art-1", "title": "Long"}]},
        hydrated={"art-1": body},
    )
    result = await node(state)
    quotes = result["selected_quotes"]
    assert len(quotes) <= 3


@pytest.mark.asyncio
async def test_heuristic_quote_length_capped() -> None:
    """Each heuristic quote text is at most 200 chars."""
    llm = FakeLLM("")
    node = QuoteSelectorNode(llm)
    body = "A" * 500 + " about AI.\nShort AI fact."
    state = _base_state(
        curated={"analysis": [{"id": "art-1", "title": "Long sent"}]},
        hydrated={"art-1": body},
    )
    result = await node(state)
    for q in result["selected_quotes"]:
        assert len(q["text"]) <= 200


@pytest.mark.asyncio
async def test_offsets_verified_against_raw_hydrated_body() -> None:
    """Offsets must be verified against raw hydrated body, not compressed."""
    llm = FakeLLM("")
    node = QuoteSelectorNode(llm)
    raw_body = "Prefix text. AI chip market is booming in 2026. Suffix text."
    state = _base_state(
        curated={"analysis": [{"id": "art-1", "title": "Report"}]},
        hydrated={"art-1": raw_body},
        compressed={"art-1": [{"text": "AI chip market is booming", "char_offset": 14, "relevance_score": 0.8}]},
        outline=[{"key": "analysis", "search_queries": ["AI chip"]}],
    )
    result = await node(state)
    for q in result["selected_quotes"]:
        assert q["start_offset"] >= 0
        # Verify the offset is against raw_body
        assert raw_body[q["start_offset"] : q["end_offset"]] == q["text"]


@pytest.mark.asyncio
async def test_query_facets_take_priority_over_search_queries() -> None:
    """query_facets should be preferred over search_queries."""
    llm = FakeLLM("")
    node = QuoteSelectorNode(llm)
    body = "Semiconductor exports grew 15%. The weather is nice. AI chip demand surges."
    state = _base_state(
        curated={"analysis": [{"id": "art-1", "title": "Report"}]},
        hydrated={"art-1": body},
        outline=[
            {
                "key": "analysis",
                "search_queries": ["general overview"],
                "query_facets": [
                    {
                        "intent": "investigate",
                        "raw_query": "semiconductor exports",
                        "must_have_terms": ["semiconductor", "exports"],
                        "entities": [],
                        "optional_terms": [],
                    },
                ],
            }
        ],
    )
    result = await node(state)
    quotes = result["selected_quotes"]
    assert len(quotes) >= 1
    # Should find semiconductor content (from facets), not generic "overview"
    assert any("semiconductor" in q["text"].lower() or "export" in q["text"].lower() for q in quotes)


@pytest.mark.asyncio
async def test_same_article_in_two_sections_uses_section_conditioned_queries() -> None:
    """Same article in two sections should produce different section-conditioned quotes."""
    llm = FakeLLM("")
    node = QuoteSelectorNode(llm)
    body = "AI chip exports grew 20%. Cloud computing revenue increased. Weather is nice."
    state = _base_state(
        curated={
            "market": [{"id": "art-1", "title": "Report"}],
            "tech": [{"id": "art-1", "title": "Report"}],
        },
        hydrated={"art-1": body},
        outline=[
            {"key": "market", "search_queries": ["chip exports"]},
            {"key": "tech", "search_queries": ["cloud computing"]},
        ],
    )
    result = await node(state)
    quotes = result["selected_quotes"]
    # Should have quotes for both sections
    section_keys = {q["section_key"] for q in quotes}
    assert "market" in section_keys
    assert "tech" in section_keys


@pytest.mark.asyncio
async def test_llm_secondary_used_only_when_heuristic_returns_empty() -> None:
    """LLM is only called when heuristic returns nothing (score=0 for all sentences)."""
    response = _make_quote_response([{"text": "LLM selected quote", "source_id": "art-1"}])
    llm = FakeLLM(response)
    node = QuoteSelectorNode(llm)
    # Body has no terms matching query → heuristic returns [] → LLM called
    state = _base_state(
        curated={"analysis": [{"id": "art-1", "title": "Report"}]},
        hydrated={"art-1": "Completely unrelated weather forecast content."},
        outline=[{"key": "analysis", "search_queries": ["quantum computing blockchain"]}],
    )
    await node(state)
    assert len(llm.calls) >= 1  # LLM was called as secondary


@pytest.mark.asyncio
async def test_sentence_fallback_when_heuristic_and_llm_both_fail() -> None:
    """When heuristic returns empty AND LLM fails, deterministic fallback produces quotes."""
    llm = FakeLLM("invalid json")
    node = QuoteSelectorNode(llm)
    state = _base_state(
        curated={"analysis": [{"id": "art-1", "title": "Report"}]},
        hydrated={"art-1": "Unrelated content one. Unrelated content two."},
        outline=[{"key": "analysis", "search_queries": ["quantum computing blockchain"]}],
    )
    result = await node(state)
    # Should still produce quotes via tertiary fallback
    assert len(result["selected_quotes"]) >= 1
    assert result["selected_quotes"][0]["source_id"] == "art-1"


@pytest.mark.asyncio
async def test_selected_quote_shape() -> None:
    """Heuristic quotes have the same shape as LLM quotes (backward compat)."""
    llm = FakeLLM("")
    node = QuoteSelectorNode(llm)
    state = _base_state(
        curated={"analysis": [{"id": "art-1", "title": "Report"}]},
        hydrated={"art-1": "AI market grew 20%."},
    )
    result = await node(state)
    q = result["selected_quotes"][0]
    required_keys = {"text", "source_id", "source_title", "section_key", "start_offset", "end_offset"}
    assert required_keys.issubset(set(q.keys()))


# --- Existing tests (updated for 3-tier ordering) ---


@pytest.mark.asyncio
async def test_uses_compressed_body() -> None:
    """When compressed_evidence has spans, use those instead of raw hydrated body."""
    response = _make_quote_response(
        [
            {"text": "compressed fact", "source_id": "art-1"},
        ]
    )
    llm = FakeLLM(response)
    node = QuoteSelectorNode(llm)

    state = _base_state(
        curated={"analysis": [{"id": "art-1", "title": "Report"}]},
        hydrated={"art-1": "Full long body that should not be used."},
        compressed={"art-1": [{"text": "compressed fact", "char_offset": 0, "relevance_score": 0.8}]},
    )
    await node(state)

    # Verify prompt used compressed body
    assert len(llm.calls) >= 1
    assert "compressed fact" in llm.calls[0]["prompt"]


@pytest.mark.asyncio
async def test_num_predict_is_small() -> None:
    """QuoteSelector must use num_predict <= 1024."""
    response = _make_quote_response([{"text": "q", "source_id": "art-1"}])
    llm = FakeLLM(response)
    node = QuoteSelectorNode(llm)

    state = _base_state(
        curated={"analysis": [{"id": "art-1", "title": "R"}]},
        hydrated={"art-1": "Body."},
    )
    await node(state)

    assert llm.calls[0]["num_predict"] <= 1024


@pytest.mark.asyncio
async def test_quote_offset_verified_against_body() -> None:
    """Heuristic quotes have valid start_offset verified against raw body."""
    body = "The AI market expanded by 20% year-over-year in 2026."
    llm = FakeLLM("")
    node = QuoteSelectorNode(llm)

    state = _base_state(
        curated={"analysis": [{"id": "art-1", "title": "Report"}]},
        hydrated={"art-1": body},
        outline=[{"key": "analysis", "search_queries": ["AI market"]}],
    )
    result = await node(state)

    quotes = result["selected_quotes"]
    assert len(quotes) >= 1
    assert quotes[0]["start_offset"] >= 0
    assert quotes[0]["end_offset"] > quotes[0]["start_offset"]
    # Verify offset points to the actual text in raw body
    q = quotes[0]
    assert body[q["start_offset"] : q["end_offset"]] == q["text"]


@pytest.mark.asyncio
async def test_quote_not_in_body_gets_offset_minus_one() -> None:
    """When LLM quote is NOT a substring of body, start_offset=-1."""
    response = _make_quote_response(
        [
            {"text": "this quote does not exist in body", "source_id": "art-1"},
        ]
    )
    llm = FakeLLM(response)
    node = QuoteSelectorNode(llm)

    state = _base_state(
        curated={"analysis": [{"id": "art-1", "title": "Report"}]},
        hydrated={"art-1": "The actual article body is different."},
    )
    result = await node(state)

    quotes = result["selected_quotes"]
    assert len(quotes) >= 1
    assert quotes[0]["start_offset"] == -1


@pytest.mark.asyncio
async def test_section_context_in_prompt() -> None:
    """Prompt must include section search_queries for section-conditioned recall."""
    response = _make_quote_response([{"text": "q", "source_id": "art-1"}])
    llm = FakeLLM(response)
    node = QuoteSelectorNode(llm)

    state = _base_state(
        curated={"analysis": [{"id": "art-1", "title": "R"}]},
        hydrated={"art-1": "Body."},
        outline=[{"key": "analysis", "search_queries": ["NVIDIA GPU performance"]}],
    )
    await node(state)

    assert "NVIDIA GPU performance" in llm.calls[0]["prompt"]
