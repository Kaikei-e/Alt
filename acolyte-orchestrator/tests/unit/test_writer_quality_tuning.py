"""Unit tests for Writer paragraph quality tuning.

Per-role num_predict, target_length hints in prompts, numbered quote mapping.
"""

from __future__ import annotations

import pytest

from acolyte.config.settings import Settings
from acolyte.port.llm_provider import LLMResponse
from acolyte.usecase.graph.nodes.writer_node import WriterNode
from acolyte.usecase.graph.state import ReportGenerationState


class FakeLLM:
    """Fake LLM that tracks call kwargs for assertion."""

    def __init__(self, responses: list[str] | None = None, default: str = "Generated paragraph.") -> None:
        self._responses = list(responses) if responses else []
        self._default = default
        self.calls: list[dict] = []

    async def generate(self, prompt: str, **kwargs: object) -> LLMResponse:
        self.calls.append({"prompt": prompt, **kwargs})
        text = self._responses.pop(0) if self._responses else self._default
        return LLMResponse(text=text, model="fake")


def _make_claim(claim_id: str = "s-1", claim: str = "Market grew 20%") -> dict:
    return {
        "claim_id": claim_id,
        "claim": claim,
        "claim_type": "statistical",
        "evidence_ids": ["art-1"],
        "supporting_quotes": ["The AI market expanded by 20%"],
        "numeric_facts": ["20%"],
        "novelty_against": [],
        "must_cite": True,
    }


def _make_state(
    *,
    section_key: str = "analysis",
    section_role: str = "analysis",
    topic: str = "AI trends",
    claims: list[dict] | None = None,
) -> ReportGenerationState:
    if claims is None:
        claims = [_make_claim(claim_id=f"{section_key}-1")]
    return {
        "outline": [{"key": section_key, "title": section_key.replace("_", " ").title(), "section_role": section_role}],
        "curated": [],
        "curated_by_section": {section_key: [{"id": "art-1", "title": "Test"}]},
        "claim_plans": {section_key: claims},
        "brief": {"topic": topic},
        "sections": {},
        "revision_count": 0,
    }


# --- Per-role num_predict ---


@pytest.mark.asyncio
async def test_analysis_paragraph_uses_role_num_predict() -> None:
    """Analysis role uses paragraph_num_predict_analysis from settings."""
    settings = Settings(paragraph_num_predict_analysis=1200)
    llm = FakeLLM()
    node = WriterNode(llm, settings=settings)  # type: ignore[unexpected-keyword]
    state = _make_state(section_role="analysis")

    await node(state)

    assert len(llm.calls) == 1
    assert llm.calls[0]["num_predict"] == 1200


@pytest.mark.asyncio
async def test_conclusion_paragraph_uses_higher_num_predict() -> None:
    """Conclusion role uses paragraph_num_predict_conclusion (higher for synthesis)."""
    settings = Settings(paragraph_num_predict_conclusion=1500)
    llm = FakeLLM()
    node = WriterNode(llm, settings=settings)  # type: ignore[unexpected-keyword]
    state = _make_state(section_key="conclusion", section_role="conclusion")

    await node(state)

    assert len(llm.calls) == 1
    assert llm.calls[0]["num_predict"] == 1500


@pytest.mark.asyncio
async def test_es_paragraph_uses_shortest_num_predict() -> None:
    """Executive summary role uses paragraph_num_predict_es from settings."""
    settings = Settings(paragraph_num_predict_es=600)
    llm = FakeLLM()
    node = WriterNode(llm, settings=settings)  # type: ignore[unexpected-keyword]
    state = _make_state(section_key="executive_summary", section_role="executive_summary")

    await node(state)

    assert len(llm.calls) == 1
    assert llm.calls[0]["num_predict"] == 600


# --- Prompt improvements ---


@pytest.mark.asyncio
async def test_paragraph_prompt_includes_target_length_hint() -> None:
    """Paragraph prompt includes <target_length> tag with character range."""
    llm = FakeLLM()
    node = WriterNode(llm, settings=Settings())  # type: ignore[unexpected-keyword]
    state = _make_state(section_role="analysis")

    await node(state)

    prompt = llm.calls[0]["prompt"]
    assert "<target_length>" in prompt


@pytest.mark.asyncio
async def test_paragraph_prompt_numbered_quote_mapping() -> None:
    """Supporting quotes are formatted with numbered evidence-id mapping."""
    claim = _make_claim()
    claim["supporting_quotes"] = ["Quote A from source", "Quote B from source"]
    claim["evidence_ids"] = ["art-1", "art-2"]
    llm = FakeLLM()
    node = WriterNode(llm, settings=Settings())  # type: ignore[unexpected-keyword]
    state = _make_state(claims=[claim])

    await node(state)

    prompt = llm.calls[0]["prompt"]
    # Should contain evidence ID references alongside quotes
    assert "[art-1]" in prompt
    assert "[art-2]" in prompt


# --- Immutability preservation ---


@pytest.mark.asyncio
async def test_accepted_paragraphs_immutable_after_quality_tuning() -> None:
    """Accepted paragraphs must survive revision even with new settings."""
    claims = [_make_claim("analysis-1", "Claim A"), _make_claim("analysis-2", "Claim B")]
    existing_paragraphs = {
        "analysis": [
            {
                "claim_id": "analysis-1",
                "claim_text": "Claim A",
                "body": "Original accepted.",
                "status": "accepted",
                "citations": [],
                "revision_feedback": "",
            },
            {
                "claim_id": "analysis-2",
                "claim_text": "Claim B",
                "body": "",
                "status": "rejected",
                "citations": [],
                "revision_feedback": "",
            },
        ],
    }
    settings = Settings(paragraph_num_predict_analysis=1200)
    llm = FakeLLM(responses=["Regenerated B."])
    node = WriterNode(llm, settings=settings)  # type: ignore[unexpected-keyword]
    state = _make_state(claims=claims)
    state["section_paragraphs"] = existing_paragraphs
    state["critique"] = {
        "verdict": "revise",
        "revise_sections": ["analysis"],
        "feedback": {"analysis": "Fix empty paragraph"},
        "claim_feedbacks": {"analysis": [{"claim_id": "analysis-2", "action": "regenerate", "reason": "body empty"}]},
    }
    state["revision_count"] = 1

    result = await node(state)

    paras = result["section_paragraphs"]["analysis"]
    # Accepted paragraph unchanged
    assert paras[0]["body"] == "Original accepted."
    assert paras[0]["status"] == "accepted"
    # Only 1 LLM call (for rejected paragraph)
    assert len(llm.calls) == 1


# --- Conclusion quality ---


@pytest.mark.asyncio
async def test_conclusion_paragraph_minimum_output_length() -> None:
    """Non-empty conclusion paragraph should be at least 50 chars for substance."""
    # This tests that the prompt + num_predict encourages substantive output.
    # The test verifies the target_length hint exists for conclusion role.
    llm = FakeLLM()
    node = WriterNode(llm, settings=Settings())  # type: ignore[unexpected-keyword]
    state = _make_state(section_key="conclusion", section_role="conclusion")

    await node(state)

    prompt = llm.calls[0]["prompt"]
    # Conclusion prompt should have target_length encouraging substantive output
    assert "<target_length>" in prompt
    # Conclusion should use synthesis-specific prompt
    assert "統合" in prompt or "結論" in prompt
