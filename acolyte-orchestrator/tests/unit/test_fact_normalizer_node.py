"""Unit tests for FactNormalizerNode — normalizes quotes into atomic facts."""

from __future__ import annotations

import json
from dataclasses import dataclass

import pytest

from acolyte.domain.quote_selection import FactNormalizerOutput
from acolyte.port.llm_provider import LLMResponse
from acolyte.usecase.graph.nodes.fact_normalizer_node import FactNormalizerNode


@dataclass
class FakeSettings:
    """Minimal settings stub for FactNormalizerNode."""

    fact_num_predict: int = 512
    max_facts_total: int = 20


class FakeLLM:
    def __init__(self, response_text: str = "") -> None:
        self._response_text = response_text
        self.calls: list[dict] = []

    async def generate(self, prompt: str, **kwargs: object) -> LLMResponse:
        self.calls.append({"prompt": prompt, **kwargs})
        return LLMResponse(text=self._response_text, model="fake")


def _normalize_response(claim: str, confidence: float = 0.9, data_type: str = "statistic") -> str:
    return json.dumps(
        {
            "reasoning": "normalized",
            "claim": claim,
            "confidence": confidence,
            "data_type": data_type,
        }
    )


# ---------------------------------------------------------------------------
# Existing tests (updated for is_fallback + config injection)
# ---------------------------------------------------------------------------


@pytest.mark.asyncio
async def test_converts_quote_to_fact() -> None:
    """1 quote → extracted_facts has 1 entry with all required fields."""
    llm = FakeLLM(_normalize_response("AI market grew 20%", 0.9, "statistic"))
    node = FactNormalizerNode(llm, FakeSettings())

    state = {
        "selected_quotes": [
            {
                "text": "The AI market expanded by 20%",
                "source_id": "art-1",
                "source_title": "Report",
                "section_key": "analysis",
                "start_offset": 0,
                "end_offset": 28,
            }
        ],
    }
    result = await node(state)

    facts = result["extracted_facts"]
    assert len(facts) == 1
    fact = facts[0]
    assert fact["claim"] == "AI market grew 20%"
    assert fact["source_id"] == "art-1"
    assert fact["source_title"] == "Report"
    assert fact["verbatim_quote"] == "The AI market expanded by 20%"
    assert fact["confidence"] == 0.9
    assert fact["data_type"] == "statistic"
    assert fact["is_fallback"] is False


@pytest.mark.asyncio
async def test_failure_preserves_quote_as_fact() -> None:
    """LLM fails → quote text becomes claim with confidence=0.3."""
    llm = FakeLLM("invalid json")
    node = FactNormalizerNode(llm, FakeSettings())

    state = {
        "selected_quotes": [
            {
                "text": "NVIDIA dominates the market",
                "source_id": "art-1",
                "source_title": "GPU Report",
                "section_key": "analysis",
            }
        ],
    }
    result = await node(state)

    facts = result["extracted_facts"]
    assert len(facts) == 1
    assert facts[0]["claim"] == "NVIDIA dominates the market"
    assert facts[0]["confidence"] == 0.3
    assert facts[0]["data_type"] == "quote"
    assert facts[0]["source_id"] == "art-1"
    assert facts[0]["is_fallback"] is True


@pytest.mark.asyncio
async def test_processes_all_quotes() -> None:
    """3 quotes → 3 facts."""
    llm = FakeLLM(_normalize_response("fact", 0.8, "quote"))
    node = FactNormalizerNode(llm, FakeSettings())

    state = {
        "selected_quotes": [{"text": f"quote {i}", "source_id": f"art-{i}", "source_title": f"T{i}"} for i in range(3)],
    }
    result = await node(state)
    assert len(result["extracted_facts"]) == 3


@pytest.mark.asyncio
async def test_empty_quotes_produces_empty() -> None:
    """selected_quotes=[] → extracted_facts=[]."""
    node = FactNormalizerNode(FakeLLM(), FakeSettings())
    result = await node({"selected_quotes": []})
    assert result["extracted_facts"] == []


@pytest.mark.asyncio
async def test_num_predict_from_node_config() -> None:
    """FactNormalizer must use fact_num_predict from settings."""
    llm = FakeLLM(_normalize_response("fact"))
    settings = FakeSettings(fact_num_predict=512)
    node = FactNormalizerNode(llm, settings)

    state = {
        "selected_quotes": [{"text": "q", "source_id": "art-1", "source_title": "T"}],
    }
    await node(state)

    assert llm.calls[0]["num_predict"] == 512


@pytest.mark.asyncio
async def test_partial_failure_preserves_successes() -> None:
    """3 quotes, LLM always fails on quote 2 → 2 success + 1 fallback."""

    class FailForArt2(FakeLLM):
        async def generate(self, prompt: str, **kwargs: object) -> LLMResponse:
            if "quote 2" in prompt:
                raise TimeoutError("ReadTimeout")
            return LLMResponse(
                text=_normalize_response("normalized claim", 0.85, "statistic"),
                model="fake",
            )

    node = FactNormalizerNode(FailForArt2(), FakeSettings())
    state = {
        "selected_quotes": [
            {"text": "quote 1", "source_id": "art-1", "source_title": "T1"},
            {"text": "quote 2", "source_id": "art-2", "source_title": "T2"},
            {"text": "quote 3", "source_id": "art-3", "source_title": "T3"},
        ],
    }
    result = await node(state)

    facts = result["extracted_facts"]
    assert len(facts) == 3
    fallback_facts = [f for f in facts if f["is_fallback"] is True]
    assert len(fallback_facts) == 1
    assert fallback_facts[0]["claim"] == "quote 2"


# ---------------------------------------------------------------------------
# New tests (Phase 1 RED)
# ---------------------------------------------------------------------------


@pytest.mark.asyncio
async def test_llm_success_sets_is_fallback_false() -> None:
    """LLM success → is_fallback=False on the returned fact."""
    llm = FakeLLM(_normalize_response("GDP grew 3%", 0.95, "statistic"))
    node = FactNormalizerNode(llm, FakeSettings())

    state = {
        "selected_quotes": [{"text": "GDP grew 3%", "source_id": "art-1", "source_title": "Economy"}],
    }
    result = await node(state)
    assert result["extracted_facts"][0]["is_fallback"] is False


@pytest.mark.asyncio
async def test_llm_failure_sets_is_fallback_true() -> None:
    """LLM failure → is_fallback=True, confidence=0.3."""
    llm = FakeLLM("not json at all")
    node = FactNormalizerNode(llm, FakeSettings())

    state = {
        "selected_quotes": [{"text": "Some quote", "source_id": "art-1", "source_title": "Src"}],
    }
    result = await node(state)
    fact = result["extracted_facts"][0]
    assert fact["is_fallback"] is True
    assert fact["confidence"] == 0.3


@pytest.mark.asyncio
async def test_llm_empty_response_produces_fallback() -> None:
    """LLM returns empty string → fallback fact."""
    llm = FakeLLM("")
    node = FactNormalizerNode(llm, FakeSettings())

    state = {
        "selected_quotes": [{"text": "Important finding", "source_id": "art-1", "source_title": "Src"}],
    }
    result = await node(state)
    fact = result["extracted_facts"][0]
    assert fact["is_fallback"] is True
    assert fact["claim"] == "Important finding"


@pytest.mark.asyncio
async def test_llm_malformed_json_produces_fallback() -> None:
    """LLM returns truncated / malformed JSON → fallback fact."""
    llm = FakeLLM('{"claim": "partial json')  # truncated
    node = FactNormalizerNode(llm, FakeSettings())

    state = {
        "selected_quotes": [{"text": "Market data point", "source_id": "art-1", "source_title": "Src"}],
    }
    result = await node(state)
    fact = result["extracted_facts"][0]
    assert fact["is_fallback"] is True
    assert fact["claim"] == "Market data point"


@pytest.mark.asyncio
async def test_fallback_fact_same_shape_as_llm_fact() -> None:
    """Fallback fact and LLM fact must have the same key set."""
    success_llm = FakeLLM(_normalize_response("normalized", 0.9, "statistic"))
    fail_llm = FakeLLM("bad")

    quote = {"text": "some quote", "source_id": "art-1", "source_title": "T"}

    success_result = await FactNormalizerNode(success_llm, FakeSettings())({"selected_quotes": [quote]})
    fail_result = await FactNormalizerNode(fail_llm, FakeSettings())({"selected_quotes": [quote]})

    success_keys = set(success_result["extracted_facts"][0].keys())
    fallback_keys = set(fail_result["extracted_facts"][0].keys())
    assert success_keys == fallback_keys


@pytest.mark.asyncio
async def test_total_cap_uses_round_robin_across_sections() -> None:
    """When quotes exceed max_facts_total, cap with section round-robin, not raw slice."""
    llm = FakeLLM(_normalize_response("fact", 0.8, "quote"))
    settings = FakeSettings(max_facts_total=4)
    node = FactNormalizerNode(llm, settings)

    # 3 quotes from section A, 3 from section B
    state = {
        "selected_quotes": [
            {"text": "A1", "source_id": "art-1", "source_title": "T1", "section_key": "sec_a"},
            {"text": "A2", "source_id": "art-2", "source_title": "T2", "section_key": "sec_a"},
            {"text": "A3", "source_id": "art-3", "source_title": "T3", "section_key": "sec_a"},
            {"text": "B1", "source_id": "art-4", "source_title": "T4", "section_key": "sec_b"},
            {"text": "B2", "source_id": "art-5", "source_title": "T5", "section_key": "sec_b"},
            {"text": "B3", "source_id": "art-6", "source_title": "T6", "section_key": "sec_b"},
        ],
    }
    result = await node(state)

    facts = result["extracted_facts"]
    assert len(facts) == 4
    # Round-robin: should have 2 from sec_a and 2 from sec_b
    section_keys_in_prompts = []
    for call in llm.calls:
        prompt = call["prompt"]
        for q in state["selected_quotes"]:
            if q["text"] in prompt:
                section_keys_in_prompts.append(q["section_key"])
                break
    sec_a_count = section_keys_in_prompts.count("sec_a")
    sec_b_count = section_keys_in_prompts.count("sec_b")
    assert sec_a_count == 2
    assert sec_b_count == 2


@pytest.mark.asyncio
async def test_schema_constrains_data_type_enum() -> None:
    """FactNormalizerOutput JSON schema must constrain data_type to enum values."""
    schema = FactNormalizerOutput.model_json_schema()
    data_type_schema = schema["properties"]["data_type"]
    assert "enum" in data_type_schema, "data_type must be constrained by enum in JSON schema"
    expected = {"statistic", "date", "quote", "trend", "comparison"}
    assert set(data_type_schema["enum"]) == expected


@pytest.mark.asyncio
async def test_selected_quotes_nonempty_never_returns_empty_facts() -> None:
    """If selected_quotes is non-empty, extracted_facts must be non-empty (fallback guarantees)."""
    # Even with complete LLM failure, every quote produces a fallback fact
    llm = FakeLLM("totally broken")
    node = FactNormalizerNode(llm, FakeSettings())

    state = {
        "selected_quotes": [
            {"text": "q1", "source_id": "art-1", "source_title": "T1"},
            {"text": "q2", "source_id": "art-2", "source_title": "T2"},
        ],
    }
    result = await node(state)
    assert len(result["extracted_facts"]) > 0
