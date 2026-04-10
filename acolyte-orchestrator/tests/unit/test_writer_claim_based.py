"""Unit tests for WriterNode claim-based generation path."""

from __future__ import annotations

import pytest

from acolyte.port.llm_provider import LLMResponse
from acolyte.usecase.graph.nodes.writer_node import WriterNode


class FakeLLM:
    def __init__(self, text: str = "Generated claim-based section.") -> None:
        self._text = text
        self.prompts: list[str] = []

    async def generate(self, prompt: str, **kwargs: object) -> LLMResponse:
        self.prompts.append(prompt)
        return LLMResponse(text=self._text, model="fake")


def _make_claim_plan(
    claims: list[dict] | None = None,
) -> list[dict]:
    if claims is None:
        claims = [
            {
                "claim": "AI market grew 20%",
                "claim_type": "statistical",
                "evidence_ids": ["art-1"],
                "supporting_quotes": ["The AI market expanded by 20%"],
                "numeric_facts": ["20%"],
                "novelty_against": [],
                "must_cite": True,
            },
        ]
    return claims


@pytest.mark.asyncio
async def test_writer_uses_claim_plans_when_present() -> None:
    """When claim_plans exist, writer uses claim-based prompt."""
    llm = FakeLLM()
    node = WriterNode(llm)

    state = {
        "outline": [{"key": "analysis", "title": "Analysis"}],
        "curated": [],
        "curated_by_section": {"analysis": [{"id": "art-1", "title": "Test"}]},
        "claim_plans": {"analysis": _make_claim_plan()},
        "brief": {"topic": "AI trends"},
        "sections": {},
    }

    result = await node(state)
    assert "sections" in result
    assert "analysis" in result["sections"]
    assert result["sections"]["analysis"] == "Generated claim-based section."
    # Verify paragraph-based prompt was used (contains claim content in XML tags)
    assert len(llm.prompts) == 1
    assert "AI market grew 20%" in llm.prompts[0]
    assert "<claim>" in llm.prompts[0]


@pytest.mark.asyncio
async def test_writer_empty_claims_produces_empty_body() -> None:
    """Section with empty claims → empty body (no hallucinated content)."""
    llm = FakeLLM("Should not be called")
    node = WriterNode(llm)

    state = {
        "outline": [{"key": "analysis", "title": "Analysis"}],
        "curated": [],
        "curated_by_section": {"analysis": []},
        "claim_plans": {"analysis": []},
        "brief": {"topic": "AI trends"},
        "sections": {},
    }

    result = await node(state)
    assert result["sections"]["analysis"] == ""


@pytest.mark.asyncio
async def test_writer_falls_back_to_evidence_without_claim_plans() -> None:
    """Without claim_plans in state, writer uses legacy evidence path."""
    llm = FakeLLM("Legacy evidence-based output.")
    node = WriterNode(llm)

    state = {
        "outline": [{"key": "summary", "title": "Summary"}],
        "curated": [{"type": "article", "id": "art-1", "title": "Test", "score": 0.9}],
        "brief": {"topic": "AI trends"},
        "sections": {},
    }

    result = await node(state)
    assert result["sections"]["summary"] == "Legacy evidence-based output."
    # Legacy prompt contains evidence references
    assert len(llm.prompts) == 1
    assert "参考記事" in llm.prompts[0]


@pytest.mark.asyncio
async def test_writer_revision_with_claim_plans() -> None:
    """Claim-based writer handles revision feedback from critic."""
    llm = FakeLLM("Revised claim-based section.")
    node = WriterNode(llm)

    state = {
        "outline": [{"key": "analysis", "title": "Analysis"}],
        "curated": [],
        "curated_by_section": {"analysis": [{"id": "art-1", "title": "Test"}]},
        "claim_plans": {"analysis": _make_claim_plan()},
        "brief": {"topic": "AI trends"},
        "sections": {},
        "critique": {
            "verdict": "revise",
            "revise_sections": ["analysis"],
            "feedback": {"analysis": "Add more statistical context"},
        },
        "revision_count": 1,
    }

    result = await node(state)
    assert "analysis" in result["sections"]
    # Paragraph-level revision: since there are no existing section_paragraphs,
    # all paragraphs are generated fresh. The section-level feedback is not
    # injected into paragraph prompts (claim_feedbacks is used instead).
    assert len(llm.prompts) == 1


@pytest.mark.asyncio
async def test_writer_claim_plan_missing_section_uses_empty() -> None:
    """If claim_plans exists but a section is missing from it, treat as empty."""
    llm = FakeLLM("Should not be called for missing section")
    node = WriterNode(llm)

    state = {
        "outline": [
            {"key": "analysis", "title": "Analysis"},
            {"key": "conclusion", "title": "Conclusion"},
        ],
        "curated": [],
        "curated_by_section": {
            "analysis": [{"id": "art-1", "title": "Test"}],
            "conclusion": [],
        },
        "claim_plans": {
            "analysis": _make_claim_plan(),
            # conclusion missing from claim_plans
        },
        "brief": {"topic": "AI trends"},
        "sections": {},
    }

    result = await node(state)
    # analysis should be generated
    assert result["sections"]["analysis"]
    # conclusion should be empty (no claims planned)
    assert result["sections"]["conclusion"] == ""


@pytest.mark.asyncio
async def test_writer_produces_section_citations() -> None:
    """Claim-based writer assembles section_citations from claim_plans."""
    llm = FakeLLM("Section body with AI market grew 20% inline.")
    node = WriterNode(llm)

    state = {
        "outline": [{"key": "analysis", "title": "Analysis"}],
        "curated": [],
        "curated_by_section": {"analysis": [{"id": "art-1", "title": "Test"}]},
        "claim_plans": {
            "analysis": [
                {
                    "claim_id": "analysis-1",
                    "claim": "AI market grew 20%",
                    "claim_type": "statistical",
                    "evidence_ids": ["art-1"],
                    "supporting_quotes": ["The AI market expanded by 20%"],
                    "numeric_facts": ["20%"],
                    "novelty_against": [],
                    "must_cite": True,
                },
            ],
        },
        "brief": {"topic": "AI trends"},
        "sections": {},
    }

    result = await node(state)
    assert "section_citations" in result
    assert "analysis" in result["section_citations"]
    citations = result["section_citations"]["analysis"]
    assert len(citations) == 1
    assert citations[0]["claim_id"] == "analysis-1"
    assert citations[0]["source_id"] == "art-1"
    assert citations[0]["source_type"] == "article"
    assert citations[0]["quote"] == "The AI market expanded by 20%"


@pytest.mark.asyncio
async def test_writer_citation_offset_mapping() -> None:
    """When a supporting_quote substring is found in body, offsets are set."""
    body_text = "Start. The AI market expanded by 20% this year. End."
    llm = FakeLLM(body_text)
    node = WriterNode(llm)

    state = {
        "outline": [{"key": "analysis", "title": "Analysis"}],
        "curated": [],
        "curated_by_section": {"analysis": [{"id": "art-1", "title": "Test"}]},
        "claim_plans": {
            "analysis": [
                {
                    "claim_id": "analysis-1",
                    "claim": "AI market grew",
                    "claim_type": "statistical",
                    "evidence_ids": ["art-1"],
                    "supporting_quotes": ["The AI market expanded by 20%"],
                    "numeric_facts": [],
                    "novelty_against": [],
                    "must_cite": True,
                },
            ],
        },
        "brief": {"topic": "AI trends"},
        "sections": {},
    }

    result = await node(state)
    citations = result["section_citations"]["analysis"]
    assert citations[0]["offset_start"] >= 0
    assert citations[0]["offset_end"] > citations[0]["offset_start"]


@pytest.mark.asyncio
async def test_writer_multiple_evidence_ids_produce_multiple_citations() -> None:
    """A claim with 2 evidence_ids produces 2 citation entries."""
    llm = FakeLLM("Generated content.")
    node = WriterNode(llm)

    state = {
        "outline": [{"key": "analysis", "title": "Analysis"}],
        "curated": [],
        "curated_by_section": {"analysis": [{"id": "art-1"}, {"id": "art-2"}]},
        "claim_plans": {
            "analysis": [
                {
                    "claim_id": "analysis-1",
                    "claim": "Market data",
                    "claim_type": "factual",
                    "evidence_ids": ["art-1", "art-2"],
                    "supporting_quotes": ["quote from art-1"],
                    "numeric_facts": [],
                    "novelty_against": [],
                    "must_cite": True,
                },
            ],
        },
        "brief": {"topic": "test"},
        "sections": {},
    }

    result = await node(state)
    citations = result["section_citations"]["analysis"]
    assert len(citations) == 2
    assert citations[0]["source_id"] == "art-1"
    assert citations[1]["source_id"] == "art-2"


@pytest.mark.asyncio
async def test_writer_legacy_path_returns_empty_citations() -> None:
    """Without claim_plans, section_citations should be empty dict."""
    llm = FakeLLM("Legacy output.")
    node = WriterNode(llm)

    state = {
        "outline": [{"key": "summary", "title": "Summary"}],
        "curated": [{"type": "article", "id": "art-1", "title": "Test", "score": 0.9}],
        "brief": {"topic": "AI trends"},
        "sections": {},
    }

    result = await node(state)
    assert result.get("section_citations", {}) == {}


@pytest.mark.asyncio
async def test_writer_paragraph_with_no_evidence_still_generates() -> None:
    """Paragraph with no evidence_ids still generates body (no section-level rejection)."""
    llm = FakeLLM("Generated despite no evidence.")
    node = WriterNode(llm)

    state = {
        "outline": [{"key": "analysis", "title": "Analysis"}],
        "curated": [],
        "curated_by_section": {"analysis": []},
        "claim_plans": {
            "analysis": [
                {
                    "claim_id": "analysis-1",
                    "claim": "Unsupported claim",
                    "claim_type": "factual",
                    "evidence_ids": [],
                    "supporting_quotes": [],
                    "numeric_facts": [],
                    "novelty_against": [],
                    "must_cite": True,
                },
            ],
        },
        "brief": {"topic": "AI trends"},
        "sections": {},
    }

    result = await node(state)
    # Paragraph generates body; citation validation is now in Critic
    assert result["sections"]["analysis"] == "Generated despite no evidence."
    assert result["section_citations"]["analysis"] == []


@pytest.mark.asyncio
async def test_writer_keeps_section_when_some_claims_cited() -> None:
    """Section with mix of cited and uncited claims: keep body, only cited claims in citations."""
    llm = FakeLLM("Body with cited content.")
    node = WriterNode(llm)

    state = {
        "outline": [{"key": "analysis", "title": "Analysis"}],
        "curated": [],
        "curated_by_section": {"analysis": [{"id": "art-1"}]},
        "claim_plans": {
            "analysis": [
                {
                    "claim_id": "analysis-1",
                    "claim": "Supported claim",
                    "claim_type": "factual",
                    "evidence_ids": ["art-1"],
                    "supporting_quotes": ["quote"],
                    "numeric_facts": [],
                    "novelty_against": [],
                    "must_cite": True,
                },
                {
                    "claim_id": "analysis-2",
                    "claim": "Unsupported claim",
                    "claim_type": "factual",
                    "evidence_ids": [],
                    "supporting_quotes": [],
                    "numeric_facts": [],
                    "novelty_against": [],
                    "must_cite": True,
                },
            ],
        },
        "brief": {"topic": "AI trends"},
        "sections": {},
    }

    result = await node(state)
    # Both paragraphs generated (paragraph-level, no section-level rejection)
    body = result["sections"]["analysis"]
    assert "Body with cited content." in body
    # Cited claim should have citations
    citations = result["section_citations"]["analysis"]
    cited_claim_ids = {c["claim_id"] for c in citations}
    assert "analysis-1" in cited_claim_ids
    # analysis-2 has no evidence_ids, so no citation for it


# --- Conclusion-specific tests ---


@pytest.mark.asyncio
async def test_writer_uses_conclusion_prompt_for_conclusion_role() -> None:
    """Section with section_role='conclusion' uses the conclusion-specific prompt."""
    llm = FakeLLM("Conclusion synthesis content.")
    node = WriterNode(llm)

    state = {
        "outline": [{"key": "conclusion", "title": "Conclusion", "section_role": "conclusion"}],
        "curated": [],
        "curated_by_section": {"conclusion": []},
        "claim_plans": {
            "conclusion": [
                {
                    "claim_id": "conclusion-1",
                    "claim": "AI trends indicate consolidation",
                    "claim_type": "synthesis",
                    "evidence_ids": ["art-1"],
                    "supporting_quotes": ["market consolidation"],
                    "numeric_facts": [],
                    "novelty_against": ["analysis"],
                    "must_cite": True,
                },
            ],
        },
        "brief": {"topic": "AI trends"},
        "sections": {},
    }

    result = await node(state)
    assert result["sections"]["conclusion"] == "Conclusion synthesis content."
    # Conclusion prompt should contain synthesis-specific instructions
    assert len(llm.prompts) == 1
    prompt = llm.prompts[0]
    assert "統合判断" in prompt or "意味づけ" in prompt
    # Should NOT contain the standard claim-based instructions
    assert "計画済みクレーム" not in prompt


@pytest.mark.asyncio
async def test_writer_conclusion_prompt_excludes_raw_evidence() -> None:
    """Conclusion prompt should not reference raw evidence articles."""
    llm = FakeLLM("Conclusion text.")
    node = WriterNode(llm)

    state = {
        "outline": [{"key": "conclusion", "title": "Conclusion", "section_role": "conclusion"}],
        "curated": [{"type": "article", "id": "art-1", "title": "Raw Article"}],
        "curated_by_section": {"conclusion": [{"id": "art-1", "title": "Raw Article"}]},
        "claim_plans": {
            "conclusion": [
                {
                    "claim_id": "conclusion-1",
                    "claim": "Synthesis point",
                    "claim_type": "synthesis",
                    "evidence_ids": ["art-1"],
                    "supporting_quotes": ["quote"],
                    "numeric_facts": [],
                    "novelty_against": ["analysis"],
                    "must_cite": True,
                },
            ],
        },
        "brief": {"topic": "AI trends"},
        "sections": {},
    }

    await node(state)
    prompt = llm.prompts[0]
    assert "参考記事" not in prompt


# --- Executive Summary-specific tests ---


@pytest.mark.asyncio
async def test_writer_uses_es_prompt_for_executive_summary_role() -> None:
    """Section with section_role='executive_summary' uses the ES-specific prompt."""
    llm = FakeLLM("Executive summary content.")
    node = WriterNode(llm)

    state = {
        "outline": [{"key": "executive_summary", "title": "Executive Summary", "section_role": "executive_summary"}],
        "curated": [],
        "curated_by_section": {"executive_summary": []},
        "claim_plans": {
            "executive_summary": [
                {
                    "claim_id": "executive_summary-1",
                    "claim": "AI market is consolidating rapidly",
                    "claim_type": "synthesis",
                    "evidence_ids": ["art-1"],
                    "supporting_quotes": ["market consolidation"],
                    "numeric_facts": ["20%"],
                    "novelty_against": ["analysis"],
                    "must_cite": True,
                },
            ],
        },
        "brief": {"topic": "AI trends"},
        "sections": {},
    }

    result = await node(state)
    assert result["sections"]["executive_summary"] == "Executive summary content."
    assert len(llm.prompts) == 1
    prompt = llm.prompts[0]
    # ES prompt should contain ES-specific instructions
    assert "要旨" in prompt or "主要な発見" in prompt
    # Should NOT contain the standard claim-based header
    assert "計画済みクレーム" not in prompt


@pytest.mark.asyncio
async def test_writer_es_prompt_excludes_raw_evidence() -> None:
    """ES prompt should not reference raw evidence articles."""
    llm = FakeLLM("ES text.")
    node = WriterNode(llm)

    state = {
        "outline": [{"key": "executive_summary", "title": "Executive Summary", "section_role": "executive_summary"}],
        "curated": [{"type": "article", "id": "art-1", "title": "Raw Article"}],
        "curated_by_section": {"executive_summary": [{"id": "art-1", "title": "Raw Article"}]},
        "claim_plans": {
            "executive_summary": [
                {
                    "claim_id": "executive_summary-1",
                    "claim": "Summary point",
                    "claim_type": "synthesis",
                    "evidence_ids": ["art-1"],
                    "supporting_quotes": ["quote"],
                    "numeric_facts": [],
                    "novelty_against": ["analysis"],
                    "must_cite": True,
                },
            ],
        },
        "brief": {"topic": "AI trends"},
        "sections": {},
    }

    await node(state)
    prompt = llm.prompts[0]
    assert "参考記事" not in prompt


# --- Contract field injection tests (Issue 5) ---


@pytest.mark.asyncio
async def test_writer_conclusion_paragraph_contains_synthesis_language() -> None:
    """Conclusion paragraph prompt contains synthesis-specific language."""
    llm = FakeLLM("Contract-driven content.")
    node = WriterNode(llm)

    state = {
        "outline": [
            {
                "key": "conclusion",
                "title": "Conclusion",
                "section_role": "conclusion",
                "novelty_against": ["market_analysis", "tech_analysis"],
            },
        ],
        "curated": [],
        "curated_by_section": {"conclusion": []},
        "claim_plans": {
            "conclusion": [
                {
                    "claim_id": "conclusion-1",
                    "claim": "Synthesis point",
                    "claim_type": "synthesis",
                    "evidence_ids": ["art-1"],
                    "supporting_quotes": ["quote"],
                    "numeric_facts": [],
                    "novelty_against": [],
                    "must_cite": True,
                },
            ],
        },
        "brief": {"topic": "AI trends"},
        "sections": {},
    }

    await node(state)
    prompt = llm.prompts[0]
    # Paragraph prompt uses XML tags and conclusion-specific language
    assert "<claim>" in prompt
    assert "意味づけ" in prompt or "統合" in prompt


@pytest.mark.asyncio
async def test_writer_paragraph_prompt_uses_xml_tags() -> None:
    """Paragraph prompt uses XML-style tags for input boundaries."""
    llm = FakeLLM("Content with stats.")
    node = WriterNode(llm)

    state = {
        "outline": [
            {
                "key": "analysis",
                "title": "Analysis",
                "section_role": "analysis",
            },
        ],
        "curated": [],
        "curated_by_section": {"analysis": []},
        "claim_plans": {
            "analysis": [
                {
                    "claim_id": "analysis-1",
                    "claim": "Market data",
                    "claim_type": "factual",
                    "evidence_ids": ["art-1"],
                    "supporting_quotes": ["quote"],
                    "numeric_facts": [],
                    "novelty_against": [],
                    "must_cite": True,
                },
            ],
        },
        "brief": {"topic": "test"},
        "sections": {},
    }

    await node(state)
    prompt = llm.prompts[0]
    assert "<topic>" in prompt
    assert "<section>" in prompt
    assert "<claim>" in prompt
    assert "<supporting_quotes>" in prompt


@pytest.mark.asyncio
async def test_writer_conclusion_prompt_contains_no_new_facts() -> None:
    """Conclusion paragraph prompt must contain no-new-facts instruction."""
    llm = FakeLLM("Synthesis content.")
    node = WriterNode(llm)

    state = {
        "outline": [
            {
                "key": "conclusion",
                "title": "Conclusion",
                "section_role": "conclusion",
                "synthesis_only": True,
            },
        ],
        "curated": [],
        "curated_by_section": {"conclusion": []},
        "claim_plans": {
            "conclusion": [
                {
                    "claim_id": "conclusion-1",
                    "claim": "Synthesis",
                    "claim_type": "synthesis",
                    "evidence_ids": ["art-1"],
                    "supporting_quotes": ["quote"],
                    "numeric_facts": [],
                    "novelty_against": [],
                    "must_cite": True,
                },
            ],
        },
        "brief": {"topic": "test"},
        "sections": {},
    }

    await node(state)
    prompt = llm.prompts[0]
    assert "新事実" in prompt


@pytest.mark.asyncio
async def test_writer_uses_section_title_in_prompt() -> None:
    """Writer puts section title in XML tag regardless of section_role."""
    llm = FakeLLM("Content.")
    node = WriterNode(llm)

    state = {
        "outline": [
            {
                "key": "wrap_up",
                "title": "Wrap Up",
                "section_role": "conclusion",
                "novelty_against": ["deep_dive_1", "deep_dive_2"],
            },
        ],
        "curated": [],
        "curated_by_section": {"wrap_up": []},
        "claim_plans": {
            "wrap_up": [
                {
                    "claim_id": "wrap_up-1",
                    "claim": "Final point",
                    "claim_type": "synthesis",
                    "evidence_ids": ["art-1"],
                    "supporting_quotes": ["quote"],
                    "numeric_facts": [],
                    "novelty_against": [],
                    "must_cite": True,
                },
            ],
        },
        "brief": {"topic": "test"},
        "sections": {},
    }

    await node(state)
    prompt = llm.prompts[0]
    assert "Wrap Up" in prompt
