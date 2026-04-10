"""Unit tests for revision feedback delta (Issue 8).

Revision prompts must not grow with revision count. Only rejected paragraphs
are regenerated with short delta feedback. Accepted paragraphs are immutable.
"""

from __future__ import annotations

import pytest

from acolyte.port.llm_provider import LLMResponse
from acolyte.usecase.graph.nodes.writer_node import WriterNode


class FakeLLM:
    def __init__(self, responses: list[str] | None = None, default: str = "Regenerated.") -> None:
        self._responses = list(responses) if responses else []
        self._default = default
        self.prompts: list[str] = []

    async def generate(self, prompt: str, **kwargs: object) -> LLMResponse:
        self.prompts.append(prompt)
        text = self._responses.pop(0) if self._responses else self._default
        return LLMResponse(text=text, model="fake")


def _make_claims(n: int = 2) -> list[dict]:
    return [
        {
            "claim_id": f"analysis-{i}",
            "claim": f"Claim {i}",
            "claim_type": "factual",
            "evidence_ids": [f"art-{i}"],
            "supporting_quotes": [f"Quote {i}"],
            "numeric_facts": [],
            "novelty_against": [],
            "must_cite": True,
        }
        for i in range(1, n + 1)
    ]


def _make_revision_state(
    *,
    accepted_body: str = "Accepted para.",
    rejected_body: str = "",
    feedback_reason: str = "body empty",
    revision_count: int = 1,
) -> dict:
    claims = _make_claims(2)
    return {
        "outline": [{"key": "analysis", "title": "Analysis", "section_role": "analysis"}],
        "curated": [],
        "curated_by_section": {"analysis": [{"id": "art-1"}, {"id": "art-2"}]},
        "claim_plans": {"analysis": claims},
        "brief": {"topic": "AI trends"},
        "sections": {"analysis": accepted_body},
        "section_paragraphs": {
            "analysis": [
                {"claim_id": "analysis-1", "claim_text": "Claim 1", "body": accepted_body, "status": "accepted", "citations": [], "revision_feedback": ""},
                {"claim_id": "analysis-2", "claim_text": "Claim 2", "body": rejected_body, "status": "rejected", "citations": [], "revision_feedback": feedback_reason},
            ],
        },
        "critique": {
            "verdict": "revise",
            "revise_sections": ["analysis"],
            "feedback": {"analysis": "Fix empty paragraph"},
            "claim_feedbacks": {"analysis": [{"claim_id": "analysis-2", "action": "regenerate", "reason": feedback_reason}]},
        },
        "revision_count": revision_count,
    }


@pytest.mark.asyncio
async def test_revision_prompt_length_does_not_grow() -> None:
    """Prompt length should not increase across revisions."""
    # Rev-1
    llm1 = FakeLLM(responses=["Rev1 body."])
    node1 = WriterNode(llm1)
    state1 = _make_revision_state(revision_count=1)
    await node1(state1)
    rev1_prompt_len = len(llm1.prompts[0])

    # Rev-2 with same feedback
    llm2 = FakeLLM(responses=["Rev2 body."])
    node2 = WriterNode(llm2)
    state2 = _make_revision_state(revision_count=2)
    await node2(state2)
    rev2_prompt_len = len(llm2.prompts[0])

    # Prompt length should be approximately the same (not growing)
    assert abs(rev2_prompt_len - rev1_prompt_len) < 50


@pytest.mark.asyncio
async def test_revision_only_regenerates_rejected_paragraphs() -> None:
    """Only rejected paragraphs get LLM calls during revision."""
    llm = FakeLLM(responses=["New para 2."])
    node = WriterNode(llm)
    state = _make_revision_state()

    await node(state)

    # Only 1 LLM call (for rejected paragraph analysis-2)
    assert len(llm.prompts) == 1
    # Prompt should contain Claim 2, not Claim 1
    assert "Claim 2" in llm.prompts[0]
    assert "Claim 1" not in llm.prompts[0]


@pytest.mark.asyncio
async def test_accepted_paragraphs_survive_revision() -> None:
    """Accepted paragraph body must not change during revision."""
    llm = FakeLLM(responses=["Regenerated para 2."])
    node = WriterNode(llm)
    state = _make_revision_state(accepted_body="Original accepted.")

    result = await node(state)

    paras = result["section_paragraphs"]["analysis"]
    # First paragraph (accepted) is unchanged
    assert paras[0]["body"] == "Original accepted."
    assert paras[0]["status"] == "accepted"
    # Second paragraph is regenerated
    assert paras[1]["body"] == "Regenerated para 2."


@pytest.mark.asyncio
async def test_revision_feedback_is_token_bounded() -> None:
    """Delta feedback in prompt is bounded (not the full critique)."""
    llm = FakeLLM(responses=["Fixed."])
    node = WriterNode(llm)
    state = _make_revision_state(feedback_reason="body empty due to thinking exhaustion")

    await node(state)

    prompt = llm.prompts[0]
    # Should contain the delta feedback in XML tag
    assert "<delta_feedback>" in prompt
    # Should NOT contain full critique reasoning
    assert "Heuristic checks" not in prompt


@pytest.mark.asyncio
async def test_past_feedback_not_accumulated() -> None:
    """Rev-2 prompt should NOT contain rev-1's feedback."""
    # Simulate rev-2 where the paragraph was previously rejected with different feedback
    llm = FakeLLM(responses=["Rev2 output."])
    node = WriterNode(llm)
    state = _make_revision_state(
        feedback_reason="new delta only",
        revision_count=2,
    )
    # The existing paragraph has old feedback stored
    state["section_paragraphs"]["analysis"][1]["revision_feedback"] = "old feedback from rev-1"

    await node(state)

    prompt = llm.prompts[0]
    # Should contain the NEW feedback from claim_feedbacks, not old stored feedback
    assert "new delta only" in prompt
    # Should NOT contain old feedback
    assert "old feedback from rev-1" not in prompt
