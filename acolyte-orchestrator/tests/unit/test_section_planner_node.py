"""Unit tests for SectionPlannerNode — claim planning from extracted facts."""

from __future__ import annotations

import pytest

from acolyte.port.llm_provider import LLMResponse
from acolyte.usecase.graph.nodes.section_planner_node import SectionPlannerNode


def _make_claim(
    claim: str = "test claim",
    claim_type: str = "factual",
    evidence_ids: list[str] | None = None,
    supporting_quotes: list[str] | None = None,
    numeric_facts: list[str] | None = None,
) -> dict:
    return {
        "claim": claim,
        "claim_type": claim_type,
        "evidence_ids": evidence_ids or ["art-1"],
        "supporting_quotes": supporting_quotes or ["test quote"],
        "numeric_facts": numeric_facts or [],
        "novelty_against": [],
        "must_cite": True,
    }


def _make_planner_response(claims: list[dict]) -> str:
    """Build XML section_plan response for testing."""
    claim_blocks = []
    for c in claims:
        eids = "".join(f"<evidence_id>{e}</evidence_id>" for e in c.get("evidence_ids", []))
        quotes = "".join(f"<supporting_quote>{q}</supporting_quote>" for q in c.get("supporting_quotes", []))
        nfacts = "".join(f"<numeric_fact>{n}</numeric_fact>" for n in c.get("numeric_facts", []))
        must_cite = str(c.get("must_cite", True)).lower()
        claim_blocks.append(
            f"<claim><text>{c['claim']}</text><claim_type>{c.get('claim_type', 'factual')}</claim_type>"
            f"{eids}{quotes}{nfacts}<must_cite>{must_cite}</must_cite></claim>"
        )
    return f"<section_plan><reasoning>test reasoning</reasoning>{''.join(claim_blocks)}</section_plan>"


class FakeLLM:
    def __init__(self, response_text: str | None = None) -> None:
        self._response = response_text or _make_planner_response([_make_claim()])
        self.prompts: list[str] = []

    async def generate(self, prompt: str, **kwargs: object) -> LLMResponse:
        self.prompts.append(prompt)
        return LLMResponse(text=self._response, model="fake")


class FakeLLMPerSection:
    """Returns different responses based on section key found in prompt."""

    def __init__(self, responses: dict[str, str]) -> None:
        self._responses = responses
        self.prompts: list[str] = []

    async def generate(self, prompt: str, **kwargs: object) -> LLMResponse:
        self.prompts.append(prompt)
        for key, response in self._responses.items():
            if key in prompt:
                return LLMResponse(text=response, model="fake")
        return LLMResponse(text=_make_planner_response([]), model="fake")


@pytest.mark.asyncio
async def test_planner_produces_claim_plans() -> None:
    """Facts present → section gets claims."""
    claims = [
        _make_claim("AI market grew 20%", "statistical", ["art-1"], ["expanded by 20%"], ["20%"]),
        _make_claim("NVIDIA dominates", "factual", ["art-1"], ["controls 80%"]),
    ]
    llm = FakeLLM(_make_planner_response(claims))
    node = SectionPlannerNode(llm)

    state = {
        "outline": [{"key": "analysis", "title": "Analysis"}],
        "curated_by_section": {
            "analysis": [{"id": "art-1", "title": "AI Market Report"}],
        },
        "extracted_facts": [
            {
                "claim": "AI market grew 20%",
                "source_id": "art-1",
                "source_title": "AI Market Report",
                "verbatim_quote": "expanded by 20%",
                "confidence": 0.9,
                "data_type": "statistic",
            },
        ],
        "brief": {"topic": "AI trends"},
    }

    result = await node(state)
    assert "claim_plans" in result
    assert "analysis" in result["claim_plans"]
    assert len(result["claim_plans"]["analysis"]) == 2
    assert result["claim_plans"]["analysis"][0]["claim"] == "AI market grew 20%"


@pytest.mark.asyncio
async def test_planner_empty_facts_returns_empty_claims() -> None:
    """No extracted facts → empty claims list for section."""
    llm = FakeLLM(_make_planner_response([]))
    node = SectionPlannerNode(llm)

    state = {
        "outline": [{"key": "summary", "title": "Summary"}],
        "curated_by_section": {"summary": []},
        "extracted_facts": [],
        "brief": {"topic": "AI trends"},
    }

    result = await node(state)
    assert result["claim_plans"]["summary"] == []


@pytest.mark.asyncio
async def test_planner_filters_facts_by_section_evidence() -> None:
    """Only facts from section's curated evidence appear in prompt."""
    llm = FakeLLM()
    node = SectionPlannerNode(llm)

    state = {
        "outline": [{"key": "analysis", "title": "Analysis"}],
        "curated_by_section": {
            "analysis": [{"id": "art-1", "title": "Article 1"}],
            # art-2 is NOT curated for analysis
        },
        "extracted_facts": [
            {
                "claim": "Fact from art-1",
                "source_id": "art-1",
                "source_title": "Article 1",
                "verbatim_quote": "q1",
                "confidence": 0.9,
                "data_type": "quote",
            },
            {
                "claim": "Fact from art-2",
                "source_id": "art-2",
                "source_title": "Article 2",
                "verbatim_quote": "q2",
                "confidence": 0.8,
                "data_type": "quote",
            },
        ],
        "brief": {"topic": "AI trends"},
    }

    await node(state)
    # The prompt sent to LLM should only contain art-1 facts
    assert len(llm.prompts) == 1
    assert "Fact from art-1" in llm.prompts[0]
    assert "Fact from art-2" not in llm.prompts[0]


@pytest.mark.asyncio
async def test_planner_no_format_in_llm_kwargs() -> None:
    """Verify the LLM is called WITHOUT format parameter (XML DSL mode)."""

    class CaptureLLM:
        def __init__(self) -> None:
            self.kwargs_list: list[dict] = []

        async def generate(self, prompt: str, **kwargs: object) -> LLMResponse:
            self.kwargs_list.append(kwargs)
            return LLMResponse(text=_make_planner_response([_make_claim()]), model="fake")

    llm = CaptureLLM()
    node = SectionPlannerNode(llm)

    state = {
        "outline": [{"key": "summary", "title": "Summary"}],
        "curated_by_section": {"summary": [{"id": "art-1", "title": "Test"}]},
        "extracted_facts": [
            {
                "claim": "test",
                "source_id": "art-1",
                "source_title": "Test",
                "verbatim_quote": "q",
                "confidence": 0.5,
                "data_type": "quote",
            },
        ],
        "brief": {"topic": "test"},
    }

    await node(state)
    assert len(llm.kwargs_list) >= 1
    assert "format" not in llm.kwargs_list[0]


@pytest.mark.asyncio
async def test_planner_fallback_on_parse_failure_uses_facts() -> None:
    """Malformed LLM output → deterministic fallback from extracted_facts.

    Previously this path returned ``[]`` which silently produced an empty
    section body downstream. Production bug 2026-04-19: Gemma4's
    ``section_plan`` XML can be malformed and the analysis section was the
    only one without a post-LLM safety net.
    """

    class BrokenLLM:
        async def generate(self, prompt: str, **kwargs: object) -> LLMResponse:
            return LLMResponse(text="not valid json at all", model="fake")

    node = SectionPlannerNode(BrokenLLM())

    state = {
        "outline": [{"key": "analysis", "title": "Analysis", "section_role": "analysis"}],
        "curated_by_section": {"analysis": [{"id": "art-1", "title": "Test"}]},
        "extracted_facts": [
            {
                "claim": "test",
                "source_id": "art-1",
                "source_title": "Test",
                "verbatim_quote": "q",
                "confidence": 0.5,
                "data_type": "quote",
            },
        ],
        "brief": {"topic": "test"},
    }

    result = await node(state)
    # Writer must never see an empty analysis claim_plan when facts exist.
    assert "claim_plans" in result
    claims = result["claim_plans"]["analysis"]
    assert len(claims) >= 1
    assert claims[0]["claim"]
    assert claims[0]["evidence_ids"] == ["art-1"]


@pytest.mark.asyncio
async def test_planner_uses_larger_num_predict_so_section_plan_does_not_truncate() -> None:
    """``num_predict=2048`` was truncating long Japanese ``section_plan``
    XML (max_claims=7 × supporting_quote runs) right before the closing
    ``</section_plan>`` tag, producing ``not well-formed`` errors in
    ``_repair_xml``. Bump the budget so the full block fits.
    """

    class CaptureLLM:
        def __init__(self) -> None:
            self.kwargs_list: list[dict] = []

        async def generate(self, prompt: str, **kwargs: object) -> LLMResponse:
            self.kwargs_list.append(kwargs)
            return LLMResponse(text=_make_planner_response([_make_claim()]), model="fake")

    llm = CaptureLLM()
    node = SectionPlannerNode(llm)

    state = {
        "outline": [{"key": "analysis", "title": "Analysis", "section_role": "analysis"}],
        "curated_by_section": {"analysis": [{"id": "art-1", "title": "Test"}]},
        "extracted_facts": [
            {
                "claim": "test",
                "source_id": "art-1",
                "source_title": "Test",
                "verbatim_quote": "q",
                "confidence": 0.9,
                "data_type": "quote",
            },
        ],
        "brief": {"topic": "test"},
    }

    await node(state)
    assert llm.kwargs_list
    assert llm.kwargs_list[0].get("num_predict", 0) >= 4096


@pytest.mark.asyncio
async def test_planner_analysis_empty_when_no_facts_at_all() -> None:
    """Fallback only activates when facts exist; no facts = empty plan
    (the Writer will still render a deterministic body for ES/Conclusion).
    """

    class BrokenLLM:
        async def generate(self, prompt: str, **kwargs: object) -> LLMResponse:
            return LLMResponse(text="not valid json at all", model="fake")

    node = SectionPlannerNode(BrokenLLM())

    state = {
        "outline": [{"key": "analysis", "title": "Analysis", "section_role": "analysis"}],
        "curated_by_section": {"analysis": []},
        "extracted_facts": [],
        "brief": {"topic": "test"},
    }

    result = await node(state)
    assert result["claim_plans"]["analysis"] == []


@pytest.mark.asyncio
async def test_planner_multiple_sections() -> None:
    """Each section gets its own claim plan."""
    analysis_claims = [_make_claim("Analysis claim", "factual", ["art-1"])]
    conclusion_claims = [_make_claim("Conclusion claim", "synthesis", ["art-1"])]

    llm = FakeLLMPerSection(
        {
            "Analysis": _make_planner_response(analysis_claims),
            "Conclusion": _make_planner_response(conclusion_claims),
        }
    )
    node = SectionPlannerNode(llm)

    state = {
        "outline": [
            {"key": "analysis", "title": "Analysis"},
            {"key": "conclusion", "title": "Conclusion"},
        ],
        "curated_by_section": {
            "analysis": [{"id": "art-1", "title": "Test"}],
            "conclusion": [{"id": "art-1", "title": "Test"}],
        },
        "extracted_facts": [
            {
                "claim": "test",
                "source_id": "art-1",
                "source_title": "Test",
                "verbatim_quote": "q",
                "confidence": 0.9,
                "data_type": "quote",
            },
        ],
        "brief": {"topic": "AI trends"},
    }

    result = await node(state)
    assert "analysis" in result["claim_plans"]
    assert "conclusion" in result["claim_plans"]


@pytest.mark.asyncio
async def test_planner_assigns_claim_ids() -> None:
    """Each claim gets a claim_id in '{section_key}-{N}' format."""
    claims = [
        _make_claim("Claim one", "factual", ["art-1"]),
        _make_claim("Claim two", "statistical", ["art-1"]),
    ]
    llm = FakeLLM(_make_planner_response(claims))
    node = SectionPlannerNode(llm)

    state = {
        "outline": [{"key": "analysis", "title": "Analysis"}],
        "curated_by_section": {
            "analysis": [{"id": "art-1", "title": "Test"}],
        },
        "extracted_facts": [
            {
                "claim": "test",
                "source_id": "art-1",
                "source_title": "Test",
                "verbatim_quote": "q",
                "confidence": 0.9,
                "data_type": "quote",
            },
        ],
        "brief": {"topic": "AI trends"},
    }

    result = await node(state)
    claims_out = result["claim_plans"]["analysis"]
    assert claims_out[0]["claim_id"] == "analysis-1"
    assert claims_out[1]["claim_id"] == "analysis-2"


@pytest.mark.asyncio
async def test_planner_claim_ids_unique_across_sections() -> None:
    """claim_ids don't collide across sections."""
    llm = FakeLLMPerSection(
        {
            "Analysis": _make_planner_response([_make_claim("A1", "factual", ["art-1"])]),
            "Conclusion": _make_planner_response([_make_claim("C1", "synthesis", ["art-1"])]),
        }
    )
    node = SectionPlannerNode(llm)

    state = {
        "outline": [
            {"key": "analysis", "title": "Analysis"},
            {"key": "conclusion", "title": "Conclusion"},
        ],
        "curated_by_section": {
            "analysis": [{"id": "art-1", "title": "Test"}],
            "conclusion": [{"id": "art-1", "title": "Test"}],
        },
        "extracted_facts": [
            {
                "claim": "test",
                "source_id": "art-1",
                "source_title": "Test",
                "verbatim_quote": "q",
                "confidence": 0.9,
                "data_type": "quote",
            },
        ],
        "brief": {"topic": "AI trends"},
    }

    result = await node(state)
    a_ids = [c["claim_id"] for c in result["claim_plans"]["analysis"]]
    c_ids = [c["claim_id"] for c in result["claim_plans"]["conclusion"]]
    assert a_ids == ["analysis-1"]
    assert c_ids == ["conclusion-1"]
    # No overlap
    assert set(a_ids).isdisjoint(set(c_ids))


# --- Conclusion-specific tests ---


@pytest.mark.asyncio
async def test_conclusion_planner_receives_analysis_claims_not_facts() -> None:
    """Conclusion with section_role='conclusion' gets analysis claims as input, not extracted_facts."""

    class SectionAwareLLM:
        def __init__(self) -> None:
            self.prompts: list[str] = []

        async def generate(self, prompt: str, **kwargs: object) -> LLMResponse:
            self.prompts.append(prompt)
            if "claim planner" in prompt.lower():
                # Analysis section → return factual claims
                return LLMResponse(
                    text=_make_planner_response(
                        [
                            _make_claim("AI adoption is accelerating", "factual", ["art-1"], ["grew by 40%"]),
                        ]
                    ),
                    model="fake",
                )
            # Conclusion section → return synthesis claims
            return LLMResponse(
                text=_make_planner_response([_make_claim("Synthesis claim", "synthesis", ["art-1"])]),
                model="fake",
            )

    llm = SectionAwareLLM()
    node = SectionPlannerNode(llm)

    state = {
        "outline": [
            {"key": "analysis", "title": "Analysis", "section_role": "analysis"},
            {"key": "conclusion", "title": "Conclusion", "section_role": "conclusion"},
        ],
        "curated_by_section": {
            "analysis": [{"id": "art-1", "title": "Test"}],
            "conclusion": [{"id": "art-1", "title": "Test"}],
        },
        "extracted_facts": [
            {
                "claim": "Raw fact from extractor",
                "source_id": "art-1",
                "source_title": "Test",
                "verbatim_quote": "raw quote",
                "confidence": 0.9,
                "data_type": "quote",
            },
        ],
        "brief": {"topic": "AI trends"},
    }

    await node(state)
    # Conclusion prompt should reference analysis claims, not raw extracted facts
    assert len(llm.prompts) == 2
    conclusion_prompt = llm.prompts[1]  # second call is for conclusion
    assert "AI adoption is accelerating" in conclusion_prompt
    assert "Raw fact from extractor" not in conclusion_prompt


@pytest.mark.asyncio
async def test_conclusion_planner_uses_different_prompt() -> None:
    """Conclusion section uses a synthesis-focused prompt, not the standard fact planner."""

    class CapturePromptLLM:
        def __init__(self) -> None:
            self.prompts: list[str] = []

        async def generate(self, prompt: str, **kwargs: object) -> LLMResponse:
            self.prompts.append(prompt)
            return LLMResponse(
                text=_make_planner_response([_make_claim("test", "synthesis", ["art-1"])]),
                model="fake",
            )

    llm = CapturePromptLLM()
    node = SectionPlannerNode(llm)

    state = {
        "outline": [
            {"key": "analysis", "title": "Analysis", "section_role": "analysis"},
            {"key": "conclusion", "title": "Conclusion", "section_role": "conclusion"},
        ],
        "curated_by_section": {
            "analysis": [{"id": "art-1", "title": "Test"}],
            "conclusion": [{"id": "art-1", "title": "Test"}],
        },
        "extracted_facts": [
            {
                "claim": "test fact",
                "source_id": "art-1",
                "source_title": "Test",
                "verbatim_quote": "q",
                "confidence": 0.9,
                "data_type": "quote",
            },
        ],
        "brief": {"topic": "AI trends"},
    }

    await node(state)
    analysis_prompt = llm.prompts[0]
    conclusion_prompt = llm.prompts[1]
    # Conclusion prompt should contain synthesis-related instructions
    assert "synthesis" in conclusion_prompt.lower() or "統合" in conclusion_prompt
    # Analysis prompt should be the standard claim planner prompt
    assert "claim planner" in analysis_prompt.lower()


@pytest.mark.asyncio
async def test_conclusion_planner_without_analysis_claims_uses_topic_fallback() -> None:
    """If analysis has no claims and no facts, conclusion gets topic-based fallback claim."""
    llm = FakeLLM(_make_planner_response([]))
    node = SectionPlannerNode(llm)

    state = {
        "outline": [
            {"key": "analysis", "title": "Analysis", "section_role": "analysis"},
            {"key": "conclusion", "title": "Conclusion", "section_role": "conclusion"},
        ],
        "curated_by_section": {
            "analysis": [],
            "conclusion": [],
        },
        "extracted_facts": [],
        "brief": {"topic": "AI trends"},
    }

    result = await node(state)
    conclusion_claims = result["claim_plans"]["conclusion"]
    assert len(conclusion_claims) >= 1
    assert conclusion_claims[0]["claim_type"] == "synthesis"


# --- Executive Summary-specific tests ---


@pytest.mark.asyncio
async def test_es_planner_uses_all_section_claims() -> None:
    """ES with section_role='executive_summary' uses claims from all sections, not raw facts."""

    class SectionAwareLLM:
        def __init__(self) -> None:
            self.prompts: list[str] = []

        async def generate(self, prompt: str, **kwargs: object) -> LLMResponse:
            self.prompts.append(prompt)
            if "claim planner" in prompt.lower():
                return LLMResponse(
                    text=_make_planner_response(
                        [_make_claim("AI adoption is accelerating", "factual", ["art-1"], ["grew by 40%"])],
                    ),
                    model="fake",
                )
            if "synthesis planner" in prompt.lower():
                return LLMResponse(
                    text=_make_planner_response(
                        [_make_claim("Overall consolidation", "synthesis", ["art-1"])],
                    ),
                    model="fake",
                )
            # ES planner — return summary claims
            return LLMResponse(
                text=_make_planner_response(
                    [_make_claim("Key finding summary", "synthesis", ["art-1"])],
                ),
                model="fake",
            )

    llm = SectionAwareLLM()
    node = SectionPlannerNode(llm)

    state = {
        "outline": [
            {"key": "executive_summary", "title": "Executive Summary", "section_role": "executive_summary"},
            {"key": "analysis", "title": "Analysis", "section_role": "analysis"},
            {"key": "conclusion", "title": "Conclusion", "section_role": "conclusion"},
        ],
        "curated_by_section": {
            "executive_summary": [{"id": "art-1", "title": "Test"}],
            "analysis": [{"id": "art-1", "title": "Test"}],
            "conclusion": [{"id": "art-1", "title": "Test"}],
        },
        "extracted_facts": [
            {
                "claim": "Raw fact from extractor",
                "source_id": "art-1",
                "source_title": "Test",
                "verbatim_quote": "raw quote",
                "confidence": 0.9,
                "data_type": "quote",
            },
        ],
        "brief": {"topic": "AI trends"},
    }

    result = await node(state)
    # ES prompt should reference accepted claims from analysis/conclusion, not raw facts
    es_prompt = [p for p in llm.prompts if "summary planner" in p.lower() or "executive" in p.lower()]
    assert len(es_prompt) == 1
    assert "AI adoption is accelerating" in es_prompt[0]
    assert "Raw fact from extractor" not in es_prompt[0]
    # ES should have claims
    assert len(result["claim_plans"]["executive_summary"]) > 0


@pytest.mark.asyncio
async def test_es_planner_deferred_after_all_sections() -> None:
    """Even when ES is first in outline, it's processed last (after analysis+conclusion)."""

    class OrderTrackingLLM:
        def __init__(self) -> None:
            self.call_order: list[str] = []

        async def generate(self, prompt: str, **kwargs: object) -> LLMResponse:
            if "summary planner" in prompt.lower() or "executive" in prompt.lower():
                self.call_order.append("executive_summary")
            elif "synthesis planner" in prompt.lower():
                self.call_order.append("conclusion")
            elif "claim planner" in prompt.lower():
                self.call_order.append("analysis")
            return LLMResponse(
                text=_make_planner_response(
                    [_make_claim("test claim", "factual", ["art-1"])],
                ),
                model="fake",
            )

    llm = OrderTrackingLLM()
    node = SectionPlannerNode(llm)

    state = {
        "outline": [
            {"key": "executive_summary", "title": "Executive Summary", "section_role": "executive_summary"},
            {"key": "analysis", "title": "Analysis", "section_role": "analysis"},
            {"key": "conclusion", "title": "Conclusion", "section_role": "conclusion"},
        ],
        "curated_by_section": {
            "executive_summary": [{"id": "art-1", "title": "Test"}],
            "analysis": [{"id": "art-1", "title": "Test"}],
            "conclusion": [{"id": "art-1", "title": "Test"}],
        },
        "extracted_facts": [
            {
                "claim": "test fact",
                "source_id": "art-1",
                "source_title": "Test",
                "verbatim_quote": "q",
                "confidence": 0.9,
                "data_type": "quote",
            },
        ],
        "brief": {"topic": "AI trends"},
    }

    await node(state)
    # ES must be processed LAST, even though it's first in the outline
    assert llm.call_order[-1] == "executive_summary"
    assert "analysis" in llm.call_order
    assert llm.call_order.index("analysis") < llm.call_order.index("executive_summary")


# --- Contract field injection tests (Issue 5) ---


@pytest.mark.asyncio
async def test_section_planner_respects_max_claims_from_contract() -> None:
    """Outline has max_claims=3. Prompt must reflect this limit, not hardcoded '3-7'."""
    llm = FakeLLM()
    node = SectionPlannerNode(llm)

    state = {
        "outline": [{"key": "analysis", "title": "Analysis", "section_role": "analysis", "max_claims": 3}],
        "curated_by_section": {"analysis": [{"id": "art-1", "title": "Test"}]},
        "extracted_facts": [
            {
                "claim": "fact",
                "source_id": "art-1",
                "source_title": "Test",
                "verbatim_quote": "q",
                "confidence": 0.9,
                "data_type": "quote",
            },
        ],
        "brief": {"topic": "test"},
    }

    await node(state)
    assert len(llm.prompts) == 1
    assert "3" in llm.prompts[0]
    # Should NOT contain the old hardcoded range
    assert "3-7" not in llm.prompts[0]


@pytest.mark.asyncio
async def test_section_planner_injects_novelty_against_from_contract() -> None:
    """Outline has novelty_against=['analysis']. Prompt must reference these keys."""
    llm = FakeLLM()
    node = SectionPlannerNode(llm)

    state = {
        "outline": [
            {"key": "analysis", "title": "Analysis", "section_role": "analysis"},
            {"key": "conclusion", "title": "Conclusion", "section_role": "conclusion", "novelty_against": ["analysis"]},
        ],
        "curated_by_section": {
            "analysis": [{"id": "art-1", "title": "Test"}],
            "conclusion": [{"id": "art-1", "title": "Test"}],
        },
        "extracted_facts": [
            {
                "claim": "fact",
                "source_id": "art-1",
                "source_title": "Test",
                "verbatim_quote": "q",
                "confidence": 0.9,
                "data_type": "quote",
            },
        ],
        "brief": {"topic": "test"},
    }

    await node(state)
    conclusion_prompt = llm.prompts[-1]  # conclusion is processed after analysis (non-ES first)
    assert "analysis" in conclusion_prompt.lower()


@pytest.mark.asyncio
async def test_section_planner_injects_synthesis_only_from_contract() -> None:
    """Outline has synthesis_only=True. Prompt must enforce synthesis claim type."""
    llm = FakeLLM()
    node = SectionPlannerNode(llm)

    state = {
        "outline": [
            {"key": "analysis", "title": "Analysis", "section_role": "analysis"},
            {"key": "conclusion", "title": "Conclusion", "section_role": "conclusion", "synthesis_only": True},
        ],
        "curated_by_section": {
            "analysis": [{"id": "art-1", "title": "Test"}],
            "conclusion": [{"id": "art-1", "title": "Test"}],
        },
        "extracted_facts": [
            {
                "claim": "fact",
                "source_id": "art-1",
                "source_title": "Test",
                "verbatim_quote": "q",
                "confidence": 0.9,
                "data_type": "quote",
            },
        ],
        "brief": {"topic": "test"},
    }

    await node(state)
    conclusion_prompt = llm.prompts[-1]
    assert "synthesis" in conclusion_prompt.lower()


@pytest.mark.asyncio
async def test_section_planner_injects_must_include_data_types() -> None:
    """Outline has must_include_data_types=['statistic']. Prompt must mention it."""
    llm = FakeLLM()
    node = SectionPlannerNode(llm)

    state = {
        "outline": [
            {
                "key": "analysis",
                "title": "Analysis",
                "section_role": "analysis",
                "must_include_data_types": ["statistic"],
            },
        ],
        "curated_by_section": {"analysis": [{"id": "art-1", "title": "Test"}]},
        "extracted_facts": [
            {
                "claim": "fact",
                "source_id": "art-1",
                "source_title": "Test",
                "verbatim_quote": "q",
                "confidence": 0.9,
                "data_type": "quote",
            },
        ],
        "brief": {"topic": "test"},
    }

    await node(state)
    assert "statistic" in llm.prompts[0].lower()


@pytest.mark.asyncio
async def test_es_planner_uses_topic_fallback_when_no_other_claims() -> None:
    """ES-only outline (no other sections) → topic-based fallback claim, LLM not called."""

    class TrackingLLM:
        def __init__(self) -> None:
            self.call_count = 0

        async def generate(self, prompt: str, **kwargs: object) -> LLMResponse:
            self.call_count += 1
            return LLMResponse(text=_make_planner_response([]), model="fake")

    llm = TrackingLLM()
    node = SectionPlannerNode(llm)

    state = {
        "outline": [
            {"key": "executive_summary", "title": "Executive Summary", "section_role": "executive_summary"},
        ],
        "curated_by_section": {
            "executive_summary": [],
        },
        "extracted_facts": [],
        "brief": {"topic": "AI trends"},
    }

    result = await node(state)
    es_claims = result["claim_plans"]["executive_summary"]
    assert len(es_claims) >= 1
    assert es_claims[0]["claim_type"] == "synthesis"
    # ES should skip LLM call when there are no accepted claims from other sections
    assert llm.call_count == 0
