"""Unit tests for WriterNode paragraph-level micro-generation (Issue 3).

WriterNode generates 1 paragraph per claim via individual LLM calls.
Accepted paragraphs are immutable during revision.
"""

from __future__ import annotations

import pytest

from acolyte.port.llm_provider import LLMResponse
from acolyte.usecase.graph.nodes.writer_node import WriterNode
from acolyte.usecase.graph.state import ReportGenerationState


class FakeLLM:
    """Fake LLM that returns canned responses and tracks calls."""

    def __init__(self, responses: list[str] | None = None, default: str = "Generated paragraph.") -> None:
        self._responses = list(responses) if responses else []
        self._default = default
        self.prompts: list[str] = []

    async def generate(self, prompt: str, **kwargs: object) -> LLMResponse:
        self.prompts.append(prompt)
        text = self._responses.pop(0) if self._responses else self._default
        return LLMResponse(text=text, model="fake")


def _make_state(
    claims: list[dict] | None = None,
    outline: list[dict] | None = None,
    *,
    section_key: str = "analysis",
    section_role: str = "analysis",
    topic: str = "AI trends",
    critique: dict | None = None,
    section_paragraphs: dict | None = None,
    revision_count: int = 0,
) -> ReportGenerationState:
    if claims is None:
        claims = [
            {
                "claim_id": f"{section_key}-1",
                "claim": "AI market grew 20%",
                "claim_type": "statistical",
                "evidence_ids": ["art-1"],
                "supporting_quotes": ["The AI market expanded by 20%"],
                "numeric_facts": ["20%"],
                "novelty_against": [],
                "must_cite": True,
            },
        ]
    if outline is None:
        outline = [{"key": section_key, "title": section_key.replace("_", " ").title(), "section_role": section_role}]
    state: ReportGenerationState = {
        "outline": outline,
        "curated": [],
        "curated_by_section": {section_key: [{"id": "art-1", "title": "Test"}]},
        "claim_plans": {section_key: claims},
        "brief": {"topic": topic},
        "sections": {},
        "revision_count": revision_count,
    }
    if critique is not None:
        state["critique"] = critique
    if section_paragraphs is not None:
        state["section_paragraphs"] = section_paragraphs
    return state


# --- Core paragraph generation ---


@pytest.mark.asyncio
async def test_writer_generates_one_paragraph_per_claim() -> None:
    """3 claims → 3 separate LLM calls, each producing one paragraph."""
    claims = [
        {
            "claim_id": "analysis-1",
            "claim": "Claim A",
            "claim_type": "factual",
            "evidence_ids": ["art-1"],
            "supporting_quotes": ["Quote A"],
            "numeric_facts": [],
            "novelty_against": [],
            "must_cite": True,
        },
        {
            "claim_id": "analysis-2",
            "claim": "Claim B",
            "claim_type": "statistical",
            "evidence_ids": ["art-2"],
            "supporting_quotes": ["Quote B"],
            "numeric_facts": ["50%"],
            "novelty_against": [],
            "must_cite": True,
        },
        {
            "claim_id": "analysis-3",
            "claim": "Claim C",
            "claim_type": "comparative",
            "evidence_ids": ["art-3"],
            "supporting_quotes": ["Quote C"],
            "numeric_facts": [],
            "novelty_against": [],
            "must_cite": True,
        },
    ]
    llm = FakeLLM(responses=["Paragraph A.", "Paragraph B.", "Paragraph C."])
    node = WriterNode(llm)
    state = _make_state(claims=claims)

    result = await node(state)

    # Must have made 3 LLM calls (one per claim)
    assert len(llm.prompts) == 3
    # section_paragraphs should contain 3 paragraphs
    paras = result.get("section_paragraphs", {}).get("analysis", [])
    assert len(paras) == 3
    assert paras[0]["body"] == "Paragraph A."
    assert paras[1]["body"] == "Paragraph B."
    assert paras[2]["body"] == "Paragraph C."
    # sections should be the concatenation
    assert "Paragraph A." in result["sections"]["analysis"]
    assert "Paragraph B." in result["sections"]["analysis"]
    assert "Paragraph C." in result["sections"]["analysis"]


@pytest.mark.asyncio
async def test_writer_empty_paragraph_preserves_others() -> None:
    """If 1 claim gets empty response, other paragraphs are still preserved."""
    claims = [
        {
            "claim_id": "analysis-1",
            "claim": "Claim A",
            "claim_type": "factual",
            "evidence_ids": ["art-1"],
            "supporting_quotes": ["Quote A"],
            "numeric_facts": [],
            "novelty_against": [],
            "must_cite": True,
        },
        {
            "claim_id": "analysis-2",
            "claim": "Claim B",
            "claim_type": "factual",
            "evidence_ids": ["art-2"],
            "supporting_quotes": ["Quote B"],
            "numeric_facts": [],
            "novelty_against": [],
            "must_cite": True,
        },
    ]
    # Second claim returns empty (thinking exhaustion)
    llm = FakeLLM(responses=["Good paragraph.", ""])
    node = WriterNode(llm)
    state = _make_state(claims=claims)

    result = await node(state)

    paras = result["section_paragraphs"]["analysis"]
    assert len(paras) == 2
    assert paras[0]["body"] == "Good paragraph."
    assert paras[0]["status"] == "accepted"
    assert paras[1]["body"] == ""
    assert paras[1]["status"] == "rejected"
    # Section body should contain the good paragraph
    assert "Good paragraph." in result["sections"]["analysis"]


@pytest.mark.asyncio
async def test_writer_revision_only_regenerates_rejected() -> None:
    """On revision, only rejected paragraphs get new LLM calls."""
    claims = [
        {
            "claim_id": "analysis-1",
            "claim": "Claim A",
            "claim_type": "factual",
            "evidence_ids": ["art-1"],
            "supporting_quotes": ["Quote A"],
            "numeric_facts": [],
            "novelty_against": [],
            "must_cite": True,
        },
        {
            "claim_id": "analysis-2",
            "claim": "Claim B",
            "claim_type": "factual",
            "evidence_ids": ["art-2"],
            "supporting_quotes": ["Quote B"],
            "numeric_facts": [],
            "novelty_against": [],
            "must_cite": True,
        },
    ]
    existing_paragraphs = {
        "analysis": [
            {
                "claim_id": "analysis-1",
                "claim_text": "Claim A",
                "body": "Accepted para.",
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
                "revision_feedback": "body empty",
            },
        ],
    }
    llm = FakeLLM(responses=["Regenerated para B."])
    node = WriterNode(llm)
    state = _make_state(
        claims=claims,
        critique={
            "verdict": "revise",
            "revise_sections": ["analysis"],
            "feedback": {"analysis": "Fix empty paragraph"},
            "claim_feedbacks": {
                "analysis": [{"claim_id": "analysis-2", "action": "regenerate", "reason": "body empty"}]
            },
        },
        section_paragraphs=existing_paragraphs,
        revision_count=1,
    )

    result = await node(state)

    # Only 1 LLM call (for rejected paragraph)
    assert len(llm.prompts) == 1
    paras = result["section_paragraphs"]["analysis"]
    # Accepted paragraph preserved
    assert paras[0]["body"] == "Accepted para."
    assert paras[0]["status"] == "accepted"
    # Rejected paragraph regenerated
    assert paras[1]["body"] == "Regenerated para B."


@pytest.mark.asyncio
async def test_writer_best_sections_tracks_best_nonblocking_revision() -> None:
    """best_sections tracks the best non-empty body, not just the latest."""
    claims = [
        {
            "claim_id": "analysis-1",
            "claim": "Claim A",
            "claim_type": "factual",
            "evidence_ids": ["art-1"],
            "supporting_quotes": ["Quote A"],
            "numeric_facts": [],
            "novelty_against": [],
            "must_cite": True,
        },
    ]
    # First call produces body, update best_sections
    llm = FakeLLM(responses=["Good body from rev-1."])
    node = WriterNode(llm)
    state = _make_state(claims=claims)

    result = await node(state)

    assert result["best_sections"]["analysis"] == "Good body from rev-1."
    assert result["best_section_metrics"]["analysis"]["char_len"] > 0
    assert result["best_section_metrics"]["analysis"]["blocking_count"] == 0


@pytest.mark.asyncio
async def test_writer_paragraph_citations_assembled() -> None:
    """Each paragraph assembles its own citations from claim evidence."""
    claims = [
        {
            "claim_id": "analysis-1",
            "claim": "Market grew",
            "claim_type": "statistical",
            "evidence_ids": ["art-1"],
            "supporting_quotes": ["The AI market expanded by 20%"],
            "numeric_facts": [],
            "novelty_against": [],
            "must_cite": True,
        },
    ]
    llm = FakeLLM(responses=["The AI market expanded by 20% this year."])
    node = WriterNode(llm)
    state = _make_state(claims=claims)

    result = await node(state)

    paras = result["section_paragraphs"]["analysis"]
    assert len(paras[0]["citations"]) >= 1
    assert paras[0]["citations"][0]["source_id"] == "art-1"


@pytest.mark.asyncio
async def test_writer_conclusion_paragraph_uses_synthesis_prompt() -> None:
    """Conclusion paragraphs use synthesis-specific prompt language."""
    claims = [
        {
            "claim_id": "conclusion-1",
            "claim": "AI trends consolidating",
            "claim_type": "synthesis",
            "evidence_ids": ["art-1"],
            "supporting_quotes": ["consolidation trend"],
            "numeric_facts": [],
            "novelty_against": ["analysis"],
            "must_cite": True,
        },
    ]
    llm = FakeLLM()
    node = WriterNode(llm)
    state = _make_state(claims=claims, section_key="conclusion", section_role="conclusion")

    await node(state)

    # Conclusion prompt should contain synthesis-specific language
    assert len(llm.prompts) == 1
    prompt = llm.prompts[0]
    assert "意味づけ" in prompt or "統合" in prompt or "synthesis" in prompt.lower()


@pytest.mark.asyncio
async def test_writer_preserves_claim_plan_order() -> None:
    """Paragraphs appear in the same order as claim_plans."""
    claims = [
        {
            "claim_id": "analysis-1",
            "claim": "First claim",
            "claim_type": "factual",
            "evidence_ids": ["art-1"],
            "supporting_quotes": ["Q1"],
            "numeric_facts": [],
            "novelty_against": [],
            "must_cite": True,
        },
        {
            "claim_id": "analysis-2",
            "claim": "Second claim",
            "claim_type": "factual",
            "evidence_ids": ["art-2"],
            "supporting_quotes": ["Q2"],
            "numeric_facts": [],
            "novelty_against": [],
            "must_cite": True,
        },
        {
            "claim_id": "analysis-3",
            "claim": "Third claim",
            "claim_type": "factual",
            "evidence_ids": ["art-3"],
            "supporting_quotes": ["Q3"],
            "numeric_facts": [],
            "novelty_against": [],
            "must_cite": True,
        },
    ]
    llm = FakeLLM(responses=["Para 1.", "Para 2.", "Para 3."])
    node = WriterNode(llm)
    state = _make_state(claims=claims)

    result = await node(state)

    paras = result["section_paragraphs"]["analysis"]
    assert paras[0]["claim_id"] == "analysis-1"
    assert paras[1]["claim_id"] == "analysis-2"
    assert paras[2]["claim_id"] == "analysis-3"
    # Section body preserves order
    body = result["sections"]["analysis"]
    assert body.index("Para 1.") < body.index("Para 2.") < body.index("Para 3.")


# --- SourceMap integration ---


@pytest.mark.asyncio
async def test_writer_uses_short_ids_when_source_map_present() -> None:
    """When source_map is in state, Writer prompt must use S1/S2 IDs, not UUIDs."""
    from acolyte.domain.source_map import SourceMap

    sm = SourceMap()
    sm.register("abc-1234-5678-dead-beef00000001", "Article Alpha")
    sm.register("def-1234-5678-dead-beef00000002", "Article Beta")

    claims = [
        {
            "claim_id": "analysis-1",
            "claim": "AI market grew 20%",
            "claim_type": "statistical",
            "evidence_ids": ["abc-1234-5678-dead-beef00000001", "def-1234-5678-dead-beef00000002"],
            "supporting_quotes": ["quote alpha", "quote beta"],
            "numeric_facts": ["20%"],
            "novelty_against": [],
            "must_cite": True,
        },
    ]
    llm = FakeLLM()
    node = WriterNode(llm)
    state = _make_state(claims=claims)
    state["source_map"] = sm.to_dict()

    await node(state)

    prompt = llm.prompts[0]
    # Short IDs must appear in prompt
    assert "S1" in prompt
    assert "S2" in prompt
    # UUIDs must NOT appear in prompt
    assert "abc-1234-5678-dead-beef00000001" not in prompt
    assert "def-1234-5678-dead-beef00000002" not in prompt
