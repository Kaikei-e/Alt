"""Tests for extractive compression domain model and CompressorNode."""

from __future__ import annotations

import pytest

from acolyte.domain.compressed_evidence import (
    CompressedSpan,
    _extract_query_terms,
    compress_article,
    score_sentence,
    select_top_sentences,
    split_sentences,
)

# --- CompressedSpan ---


def test_compressed_span_is_frozen_dataclass():
    span = CompressedSpan(text="AI is growing", char_offset=0, relevance_score=0.9)
    assert span.text == "AI is growing"
    assert span.char_offset == 0
    assert span.relevance_score == 0.9
    with pytest.raises(AttributeError):
        span.text = "x"  # type: ignore[misc]


# --- split_sentences ---


def test_split_sentences_english():
    sents = split_sentences("First sentence. Second one. Third!")
    assert len(sents) == 3
    assert sents[0][0] == "First sentence."
    assert sents[0][1] == 0


def test_split_sentences_japanese():
    sents = split_sentences("最初の文。次の文。")
    assert len(sents) == 2
    assert sents[0][0] == "最初の文。"


def test_split_sentences_mixed():
    sents = split_sentences("English first. 日本語の文。More English.")
    assert len(sents) == 3


def test_split_sentences_conservative_on_abbreviation_and_decimal():
    """U.S. and 3.14% must NOT be split mid-token."""
    sents = split_sentences("U.S. chip exports rose 3.14%. 次の文。")
    assert len(sents) == 2


def test_split_sentences_empty():
    assert split_sentences("") == []


def test_split_sentences_single_no_delimiter():
    sents = split_sentences("No delimiter here")
    assert len(sents) == 1
    assert sents[0][0] == "No delimiter here"


def test_split_sentences_newline_separated():
    """Single \\n should split lines (for RSS bullet points, headlines)."""
    sents = split_sentences("Key Findings\nAI chip market grows 45%\nNVIDIA leads")
    assert len(sents) == 3


def test_split_sentences_bullet_points():
    sents = split_sentences("• Item one\n• Item two\n• Item three")
    assert len(sents) == 3


# --- score_sentence ---


def test_score_sentence_keyword_overlap():
    score = score_sentence("AI trends are accelerating in 2026", {"ai", "trends"})
    assert score > 0.0


def test_score_sentence_no_match():
    score = score_sentence("Weather is sunny today", {"ai", "trends"})
    assert score == 0.0


def test_score_sentence_case_insensitive():
    s1 = score_sentence("AI trends rising", {"ai", "trends"})
    s2 = score_sentence("ai trends rising", {"ai", "trends"})
    assert s1 == s2


def test_score_sentence_more_terms_higher():
    s1 = score_sentence("AI trends", {"ai", "trends"})
    s2 = score_sentence("AI only", {"ai", "trends"})
    assert s1 > s2


def test_score_sentence_japanese_content():
    """Japanese sentences must score > 0 when matching Japanese query terms."""
    score = score_sentence(
        "NVIDIAは2026年第1四半期にBlackwell Ultra GPUの量産を開始した。",
        {"チップ", "nvidia", "2026"},
    )
    assert score > 0.0


def test_score_sentence_cjk_bigram_fuzzy():
    """CJK bi-gram overlap should match partial Japanese terms."""
    score = score_sentence(
        "AIチップ市場は急成長している。",
        {"チップ", "市場"},
    )
    assert score > 0.0


# --- _extract_query_terms ---


def test_extract_query_terms_japanese_splits_on_punctuation():
    """Japanese punctuation (。、) must be token separators, not part of tokens."""
    terms = _extract_query_terms(["AIやLLMのチップに関して、各種メーカーの動向を分析して。"])
    # Should NOT have a single 50-char token
    assert all(len(t) < 20 for t in terms), f"Giant token found: {terms}"
    # Should contain individual terms
    assert "ai" in terms or any("チップ" in t for t in terms)


def test_extract_query_terms_mixed_language():
    terms = _extract_query_terms(["AI chip market 2026", "半導体トレンド"])
    assert "ai" in terms
    assert "chip" in terms
    assert "2026" in terms
    assert any("半導体" in t or "トレンド" in t for t in terms)


# --- compress_article ---


def test_compress_article_selects_relevant_sentences():
    body = "AI adoption is growing rapidly. The weather is nice. AI spending hit $100B."
    spans = compress_article(body, ["AI trends"], char_budget=80)
    texts = [s.text for s in spans]
    assert any("AI" in t for t in texts)
    assert not any("weather" in t.lower() for t in texts)


def test_compress_article_japanese_content_not_empty():
    """Japanese article with Japanese queries must NOT return empty (the original bug)."""
    body = (
        "NVIDIAは2026年第1四半期にBlackwell Ultra GPUの量産を開始した。\n"
        "この新チップは、前世代のH100と比較して推論性能が約3倍に向上している。\n"
        "一方、AMDはMI400シリーズを発表し、LLMトレーニング市場での競争が激化している。\n"
        "中国政府は国産チップの開発を加速させており、Huaweiの Ascend 910Cが注目を集めている。\n"
        "IntelはGaudi 3を2026年に投入し、AIアクセラレータ市場への本格参入を表明した。"
    )
    topic = "AIやLLMに用いられるチップに関して、各種メーカーや国家の動向から2026年のトレンドを分析して。"
    spans = compress_article(body, [topic], char_budget=1000)
    assert len(spans) > 0, "Japanese article must produce spans, got empty"


def test_compress_article_preserves_score_descending_order():
    """Packing order: strongest evidence first (Lost-in-the-Middle mitigation)."""
    body = "Weak filler sentence here. Strong AI trend data point. AI spending hit $100B in 2026."
    spans = compress_article(body, ["AI spending trends"], char_budget=200)
    if len(spans) > 1:
        scores = [s.relevance_score for s in spans]
        assert scores == sorted(scores, reverse=True)


def test_compress_article_passthrough_short_body():
    body = "Short article about AI."
    spans = compress_article(body, ["AI"], char_budget=1000)
    assert len(spans) == 1
    assert spans[0].text == body


def test_compress_article_can_return_empty_when_nothing_is_relevant():
    """Selective augmentation: no relevant span → empty list."""
    body = "Weather is sunny today. Markets are calm."
    spans = compress_article(body, ["AI chips"], char_budget=80)
    assert spans == []


def test_compress_article_empty_body():
    spans = compress_article("", ["AI"], char_budget=1000)
    assert spans == []


def test_compress_article_respects_char_budget():
    body = "AI trend one. " * 100  # ~1400 chars
    spans = compress_article(body, ["AI"], char_budget=200)
    total = sum(len(s.text) for s in spans)
    assert total <= 250  # budget + one sentence overflow tolerance


# --- select_top_sentences ---


def test_select_top_sentences_returns_scored_spans():
    """select_top_sentences returns CompressedSpan list with relevance_score."""
    body = "AI chip market grew 20%. Weather is nice. NVIDIA leads the GPU race."
    spans = select_top_sentences(body, ["AI chip market"])
    assert len(spans) >= 1
    assert all(isinstance(s, CompressedSpan) for s in spans)
    assert spans[0].relevance_score > 0


def test_select_top_sentences_respects_max_sentences():
    """Never returns more than max_sentences."""
    body = "Sent one about AI. Sent two about AI. Sent three about AI. Sent four about AI."
    spans = select_top_sentences(body, ["AI"], max_sentences=2)
    assert len(spans) <= 2


def test_select_top_sentences_respects_max_len():
    """Each returned span text is capped at max_len characters."""
    body = "A" * 300 + " about AI chips. Short AI fact."
    spans = select_top_sentences(body, ["AI"], max_len=200)
    assert all(len(s.text) <= 200 for s in spans)


def test_select_top_sentences_japanese_article():
    """Japanese article with JP queries produces scored spans."""
    body = (
        "NVIDIAは2026年第1四半期にBlackwell Ultra GPUの量産を開始した。\n"
        "この新チップは推論性能が約3倍に向上している。\n"
        "天気は晴れです。\n"
    )
    spans = select_top_sentences(body, ["NVIDIA チップ GPUの量産"])
    assert len(spans) >= 1
    assert spans[0].relevance_score > 0


def test_select_top_sentences_empty_body():
    """Empty body returns empty list."""
    assert select_top_sentences("", ["AI"]) == []


def test_select_top_sentences_returns_empty_without_position_fallback():
    """score=0 and position_fallback=False returns [] (for 3-tier degradation)."""
    body = "Completely unrelated sentence one. Another unrelated sentence."
    spans = select_top_sentences(body, ["quantum computing blockchain"], position_fallback=False)
    assert spans == []


def test_select_top_sentences_position_fallback_returns_head_sentences():
    """score=0 and position_fallback=True returns first N sentences."""
    body = "Completely unrelated sentence one. Another unrelated sentence."
    spans = select_top_sentences(body, ["quantum computing blockchain"], position_fallback=True)
    assert len(spans) >= 1


def test_select_top_sentences_offset_correct_against_raw_body():
    """Returned char_offset matches actual position of text in body."""
    body = "First sentence here. Second about AI chips. Third sentence."
    spans = select_top_sentences(body, ["AI chips"], max_sentences=1)
    for span in spans:
        assert body[span.char_offset : span.char_offset + len(span.text)] == span.text


# --- CompressorNode ---


@pytest.mark.asyncio
async def test_compressor_node_produces_compressed_evidence():
    from acolyte.usecase.graph.nodes.compressor_node import CompressorNode

    node = CompressorNode(char_budget=80)
    state = {
        "hydrated_evidence": {"art-1": "Important AI fact about trends. Irrelevant filler here. Another AI trend."},
        "curated_by_section": {"analysis": [{"id": "art-1"}]},
        "outline": [{"key": "analysis", "search_queries": ["AI trends"]}],
        "brief": {"topic": "AI"},
    }
    result = await node(state)
    assert "compressed_evidence" in result
    assert "art-1" in result["compressed_evidence"]
    spans = result["compressed_evidence"]["art-1"]
    assert len(spans) >= 1


@pytest.mark.asyncio
async def test_compressor_node_uses_query_facets_over_search_queries():
    """query_facets (ADR-667/669) should be preferred over legacy search_queries."""
    from acolyte.usecase.graph.nodes.compressor_node import CompressorNode

    node = CompressorNode(char_budget=200)
    state = {
        "hydrated_evidence": {
            "art-1": "Semiconductor chip exports grew 15%. Weather is nice today. AI chip market booming."
        },
        "curated_by_section": {"analysis": [{"id": "art-1"}]},
        "outline": [
            {
                "key": "analysis",
                "search_queries": ["general overview"],
                "query_facets": [
                    {"intent": "market_data", "raw_query": "semiconductor chip exports"},
                ],
            }
        ],
        "brief": {"topic": "chip market"},
    }
    result = await node(state)
    spans = result["compressed_evidence"]["art-1"]
    texts = [s["text"] for s in spans]
    # Should find chip/semiconductor content (from facets), not just "general overview"
    assert any("chip" in t.lower() or "semiconductor" in t.lower() for t in texts)


@pytest.mark.asyncio
async def test_compressor_node_merges_queries_from_multiple_sections():
    from acolyte.usecase.graph.nodes.compressor_node import CompressorNode

    node = CompressorNode()
    state = {
        "hydrated_evidence": {"art-1": "Text about markets and technology trends."},
        "curated_by_section": {
            "market": [{"id": "art-1"}],
            "tech": [{"id": "art-1"}],
        },
        "outline": [
            {"key": "market", "search_queries": ["market trends"]},
            {"key": "tech", "search_queries": ["technology advances"]},
        ],
        "brief": {"topic": "industry overview"},
    }
    result = await node(state)
    assert "art-1" in result["compressed_evidence"]


@pytest.mark.asyncio
async def test_compressor_node_empty_hydrated():
    from acolyte.usecase.graph.nodes.compressor_node import CompressorNode

    node = CompressorNode()
    state = {
        "hydrated_evidence": {},
        "curated_by_section": {"analysis": [{"id": "art-1"}]},
        "outline": [{"key": "analysis", "search_queries": ["AI"]}],
        "brief": {"topic": "AI"},
    }
    result = await node(state)
    assert result["compressed_evidence"] == {}


# --- ExtractorNode with compressed evidence ---


@pytest.mark.asyncio
async def test_extractor_uses_compressed_evidence_when_available():
    """Extractor should use compressed spans instead of raw body."""
    import json

    from acolyte.port.llm_provider import LLMResponse
    from acolyte.usecase.graph.nodes.extractor_node import ExtractorNode

    class CaptureLLM:
        def __init__(self):
            self._calls: list[dict] = []

        async def generate(self, prompt, **kwargs):
            self._calls.append({"prompt": prompt})
            return LLMResponse(
                text=json.dumps(
                    {
                        "reasoning": "test",
                        "facts": [
                            {
                                "claim": "AI growing",
                                "source_id": "art-1",
                                "source_title": "Test",
                                "verbatim_quote": "Compressed span",
                                "confidence": 0.9,
                                "data_type": "quote",
                            }
                        ],
                    }
                ),
                model="fake",
            )

    llm = CaptureLLM()
    node = ExtractorNode(llm)
    state = {
        "curated_by_section": {"analysis": [{"id": "art-1", "title": "Test"}]},
        "hydrated_evidence": {"art-1": "Full body that should NOT be used"},
        "compressed_evidence": {
            "art-1": [{"text": "Compressed relevant span about AI.", "char_offset": 0, "relevance_score": 0.9}]
        },
    }
    result = await node(state)
    prompt_used = llm._calls[0]["prompt"]
    assert "Compressed relevant span" in prompt_used
    assert "should NOT be used" not in prompt_used
    assert len(result["extracted_facts"]) >= 1


@pytest.mark.asyncio
async def test_extractor_falls_back_to_hydrated_without_compressed():
    """Without compressed_evidence, extractor uses hydrated_evidence (backward compat)."""
    import json

    from acolyte.port.llm_provider import LLMResponse
    from acolyte.usecase.graph.nodes.extractor_node import ExtractorNode

    class CaptureLLM:
        def __init__(self):
            self._calls: list[dict] = []

        async def generate(self, prompt, **kwargs):
            self._calls.append({"prompt": prompt})
            return LLMResponse(
                text=json.dumps(
                    {
                        "reasoning": "test",
                        "facts": [],
                    }
                ),
                model="fake",
            )

    llm = CaptureLLM()
    node = ExtractorNode(llm)
    state = {
        "curated_by_section": {"analysis": [{"id": "art-1", "title": "Test"}]},
        "hydrated_evidence": {"art-1": "Fallback body text about AI."},
    }
    await node(state)
    prompt_used = llm._calls[0]["prompt"]
    assert "Fallback body text" in prompt_used


@pytest.mark.asyncio
async def test_extractor_falls_back_to_hydrated_when_compressed_empty():
    """compressed[id] == [] triggers tiered fallback to hydrated body."""
    import json

    from acolyte.port.llm_provider import LLMResponse
    from acolyte.usecase.graph.nodes.extractor_node import ExtractorNode

    class CaptureLLM:
        def __init__(self):
            self._calls: list[dict] = []

        async def generate(self, prompt, **kwargs):
            self._calls.append({"prompt": prompt})
            return LLMResponse(text=json.dumps({"reasoning": "t", "facts": []}), model="fake")

    llm = CaptureLLM()
    node = ExtractorNode(llm)
    state = {
        "curated_by_section": {"analysis": [{"id": "art-1", "title": "Test"}]},
        "hydrated_evidence": {"art-1": "Fallback body from hydrated evidence"},
        "compressed_evidence": {"art-1": []},  # selective augmentation: nothing relevant
    }
    await node(state)
    # Tier 2: should fall back to hydrated body, NOT skip
    assert len(llm._calls) >= 1
    assert "Fallback body from hydrated" in llm._calls[0]["prompt"]


@pytest.mark.asyncio
async def test_extractor_degrades_to_quote_only_on_all_failures():
    """When LLM returns empty facts on all passes, quote-only fallback produces output."""
    import json

    from acolyte.port.llm_provider import LLMResponse
    from acolyte.usecase.graph.nodes.extractor_node import ExtractorNode

    class AlwaysEmptyLLM:
        def __init__(self):
            self._calls: list[dict] = []

        async def generate(self, prompt, **kwargs):
            self._calls.append({"prompt": prompt})
            return LLMResponse(text=json.dumps({"reasoning": "t", "facts": []}), model="fake")

    llm = AlwaysEmptyLLM()
    node = ExtractorNode(llm)
    state = {
        "curated_by_section": {"analysis": [{"id": "art-1", "title": "Test"}]},
        "hydrated_evidence": {"art-1": "Important fact about AI chips.\nAnother key finding."},
    }
    result = await node(state)
    # Should have quote-only facts (Pass 3 fallback)
    assert len(result["extracted_facts"]) >= 1
    assert result["extracted_facts"][0]["confidence"] == 0.3  # quote-only marker


@pytest.mark.asyncio
async def test_extractor_always_produces_some_output():
    """Extractor must never return empty facts for an article with body text."""
    from acolyte.usecase.graph.nodes.extractor_node import ExtractorNode

    class FailingLLM:
        async def generate(self, prompt, **kwargs):
            raise TimeoutError("ReadTimeout on every call")

    llm = FailingLLM()
    node = ExtractorNode(llm)
    state = {
        "curated_by_section": {"analysis": [{"id": "art-1", "title": "Test"}]},
        "hydrated_evidence": {"art-1": "Content that must produce at least one fact."},
    }
    result = await node(state)
    # Quote-only fallback should fire
    assert len(result["extracted_facts"]) >= 1
