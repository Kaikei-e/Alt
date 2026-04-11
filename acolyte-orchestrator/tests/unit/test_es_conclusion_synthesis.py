"""Formal acceptance tests for Issue 5: ES/Conclusion deterministic synthesis.

Acceptance criteria (exec3.md):
  1. ES / Conclusion が claims=0 で空本文にならない
  2. ES は accepted claims から最低限生成できる
  3. Conclusion は analysis accepted claims が存在すれば最低限生成できる
  4. Analysis と Conclusion の表現重複が減る
  5. unit test に「analysis claim あり」「analysis claim なし」「ES fallback」「Conclusion fallback」
"""

from __future__ import annotations

import pytest

from acolyte.port.llm_provider import LLMResponse
from acolyte.usecase.graph.nodes.section_planner_node import (
    _deterministic_conclusion_claims,
    _deterministic_es_claims,
)
from acolyte.usecase.graph.nodes.writer_node import WriterNode


class FakeLLM:
    """Returns pre-configured response text and captures prompts."""

    def __init__(self, response_text: str = "Generated paragraph content.") -> None:
        self._response_text = response_text
        self.prompts: list[str] = []

    async def generate(self, prompt: str, **kwargs: object) -> LLMResponse:
        self.prompts.append(prompt)
        return LLMResponse(text=self._response_text, model="fake")


def _make_analysis_claims(n: int = 3) -> list[dict]:
    return [
        {
            "claim_id": f"analysis-{i}",
            "claim": f"Analysis claim {i} about market trends",
            "claim_type": "factual",
            "evidence_ids": [f"art-{i}"],
            "supporting_quotes": [f"Quote from article {i}"],
            "numeric_facts": [f"{i * 10}%"] if i <= 2 else [],
            "novelty_against": [],
            "must_cite": True,
        }
        for i in range(1, n + 1)
    ]


# ---------------------------------------------------------------------------
# AC-1: ES / Conclusion が claims=0 で空本文にならない
# ---------------------------------------------------------------------------


def test_conclusion_claims_never_empty_even_without_input() -> None:
    """_deterministic_conclusion_claims with both inputs empty returns ≥1 claim."""
    result = _deterministic_conclusion_claims([], [], topic="AI semiconductor market")
    assert len(result) >= 1
    assert result[0]["claim_type"] == "synthesis"
    assert result[0]["claim"]  # non-empty string


def test_es_claims_never_empty_even_without_input() -> None:
    """_deterministic_es_claims with both inputs empty returns ≥1 claim."""
    result = _deterministic_es_claims({}, [], topic="AI semiconductor market")
    assert len(result) >= 1
    assert result[0]["claim_type"] == "synthesis"
    assert result[0]["claim"]


@pytest.mark.asyncio
async def test_writer_conclusion_nonempty_when_claims_empty() -> None:
    """WriterNode produces non-empty conclusion even with claims=[] when analysis exists."""
    llm = FakeLLM("Fallback content.")
    node = WriterNode(llm)

    state = {
        "outline": [
            {"key": "analysis", "title": "Analysis", "section_role": "analysis"},
            {"key": "conclusion", "title": "Conclusion", "section_role": "conclusion"},
        ],
        "curated": [],
        "curated_by_section": {},
        "claim_plans": {"analysis": _make_analysis_claims(2), "conclusion": []},
        "brief": {"topic": "AI semiconductor market"},
        "sections": {},
    }

    result = await node(state)
    # Analysis should be generated normally (non-empty)
    assert result["sections"]["analysis"]
    # Conclusion should NOT be empty despite claims=[]
    assert result["sections"]["conclusion"], "Conclusion should not be empty when analysis content exists"


@pytest.mark.asyncio
async def test_writer_es_nonempty_when_claims_empty() -> None:
    """WriterNode produces non-empty ES even with claims=[] when other sections exist."""
    llm = FakeLLM("Generated paragraph.")
    node = WriterNode(llm)

    state = {
        "outline": [
            {"key": "executive_summary", "title": "Executive Summary", "section_role": "executive_summary"},
            {"key": "analysis", "title": "Analysis", "section_role": "analysis"},
            {"key": "conclusion", "title": "Conclusion", "section_role": "conclusion"},
        ],
        "curated": [],
        "curated_by_section": {},
        "claim_plans": {
            "analysis": _make_analysis_claims(2),
            "conclusion": [
                {
                    "claim_id": "conclusion-1",
                    "claim": "Market is consolidating",
                    "claim_type": "synthesis",
                    "evidence_ids": ["art-1"],
                    "supporting_quotes": ["consolidation"],
                    "numeric_facts": [],
                    "novelty_against": ["analysis"],
                    "must_cite": True,
                },
            ],
            "executive_summary": [],
        },
        "brief": {"topic": "AI semiconductor market"},
        "sections": {},
    }

    result = await node(state)
    assert result["sections"]["analysis"]
    assert result["sections"]["conclusion"]
    assert result["sections"]["executive_summary"], "ES should not be empty when other section content exists"


# ---------------------------------------------------------------------------
# AC-2/AC-3: accepted claims ベース生成
# ---------------------------------------------------------------------------


def test_conclusion_from_analysis_claims_produces_synthesis() -> None:
    """When analysis claims exist, conclusion generates synthesis claims."""
    analysis_claims = _make_analysis_claims(3)
    result = _deterministic_conclusion_claims(analysis_claims, [], topic="AI trends")
    assert len(result) > 0
    for claim in result:
        assert claim["claim_type"] == "synthesis"


def test_es_from_all_section_claims_produces_synthesis() -> None:
    """When section claims exist, ES generates synthesis claims."""
    all_claims = {
        "analysis": _make_analysis_claims(3),
        "conclusion": [
            {
                "claim_id": "conclusion-1",
                "claim": "Conclusion synthesis",
                "claim_type": "synthesis",
                "evidence_ids": ["art-1"],
                "supporting_quotes": ["Q"],
                "numeric_facts": [],
                "novelty_against": ["analysis"],
                "must_cite": True,
            },
        ],
    }
    result = _deterministic_es_claims(all_claims, [], topic="AI trends")
    assert len(result) > 0
    for claim in result:
        assert claim["claim_type"] == "synthesis"


# ---------------------------------------------------------------------------
# AC-4: Analysis と Conclusion の表現重複が減る
# ---------------------------------------------------------------------------


@pytest.mark.asyncio
async def test_writer_conclusion_prompt_includes_analysis_context() -> None:
    """Conclusion prompt must include prior analysis body for anti-duplication."""
    llm = FakeLLM("Conclusion content.")
    node = WriterNode(llm)

    state = {
        "outline": [
            {"key": "analysis", "title": "Analysis", "section_role": "analysis"},
            {"key": "conclusion", "title": "Conclusion", "section_role": "conclusion"},
        ],
        "curated": [],
        "curated_by_section": {},
        "claim_plans": {
            "analysis": _make_analysis_claims(1),
            "conclusion": [
                {
                    "claim_id": "conclusion-1",
                    "claim": "Market consolidation",
                    "claim_type": "synthesis",
                    "evidence_ids": ["art-1"],
                    "supporting_quotes": ["consolidation"],
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
    # Find the conclusion prompt (should be the second prompt)
    conclusion_prompts = [p for p in llm.prompts if "統合クレーム" in p or "結論" in p]
    assert len(conclusion_prompts) >= 1
    conclusion_prompt = conclusion_prompts[0]
    assert "<prior_analysis>" in conclusion_prompt, "Conclusion prompt should include prior analysis context"
    assert "重複" in conclusion_prompt, "Conclusion prompt should include anti-duplication instruction"


@pytest.mark.asyncio
async def test_writer_conclusion_prefers_accepted_analysis_claims_over_planner_claims() -> None:
    """Conclusion should synthesize from accepted analysis claims, not stale planner output."""
    llm = FakeLLM("Conclusion content.")
    node = WriterNode(llm)

    state = {
        "outline": [
            {"key": "analysis", "title": "Analysis", "section_role": "analysis"},
            {"key": "conclusion", "title": "Conclusion", "section_role": "conclusion"},
        ],
        "curated": [],
        "curated_by_section": {},
        "claim_plans": {
            "analysis": _make_analysis_claims(1),
            "conclusion": [
                {
                    "claim_id": "conclusion-planner-1",
                    "claim": "Planner-only synthesis that should be ignored",
                    "claim_type": "synthesis",
                    "evidence_ids": ["art-x"],
                    "supporting_quotes": ["planner quote"],
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

    conclusion_prompts = [p for p in llm.prompts if "統合クレーム" in p or "結論" in p]
    assert len(conclusion_prompts) >= 1
    prompt = conclusion_prompts[0]
    assert "Analysis claim 1 about market trends" in prompt
    assert "Planner-only synthesis that should be ignored" not in prompt


@pytest.mark.asyncio
async def test_writer_es_renders_deterministically_no_llm() -> None:
    """ES uses deterministic renderer — no LLM calls, claim content in body."""
    llm = FakeLLM("ES content.")
    node = WriterNode(llm)

    state = {
        "outline": [
            {"key": "executive_summary", "title": "Executive Summary", "section_role": "executive_summary"},
            {"key": "analysis", "title": "Analysis", "section_role": "analysis"},
        ],
        "curated": [],
        "curated_by_section": {},
        "claim_plans": {
            "analysis": _make_analysis_claims(1),
            "executive_summary": [
                {
                    "claim_id": "es-1",
                    "claim": "Market overview",
                    "claim_type": "synthesis",
                    "evidence_ids": ["art-1"],
                    "supporting_quotes": ["overview"],
                    "numeric_facts": ["10%"],
                    "novelty_against": ["analysis"],
                    "must_cite": True,
                },
            ],
        },
        "brief": {"topic": "AI trends"},
        "sections": {},
    }

    result = await node(state)
    # No LLM calls for ES (deterministic renderer — only analysis uses LLM)
    es_prompts = [p for p in llm.prompts if "要旨" in p or "主要な発見" in p]
    assert len(es_prompts) == 0
    # Body should be non-empty
    es_body = result["sections"]["executive_summary"]
    assert es_body


@pytest.mark.asyncio
async def test_writer_es_uses_accepted_claims_for_rendering() -> None:
    """Executive summary renders accepted claims from prior sections via deterministic renderer."""
    llm = FakeLLM("ES content.")
    node = WriterNode(llm)

    state = {
        "outline": [
            {"key": "analysis", "title": "Analysis", "section_role": "analysis"},
            {"key": "executive_summary", "title": "Executive Summary", "section_role": "executive_summary"},
        ],
        "curated": [],
        "curated_by_section": {},
        "claim_plans": {
            "analysis": _make_analysis_claims(1),
            "executive_summary": [
                {
                    "claim_id": "es-planner-1",
                    "claim": "Planner ES claim",
                    "claim_type": "synthesis",
                    "evidence_ids": ["art-x"],
                    "supporting_quotes": ["planner quote"],
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

    # Analysis uses LLM, but ES should not (deterministic renderer)
    es_prompts = [p for p in llm.prompts if "要旨" in p or "主要な発見" in p]
    assert len(es_prompts) == 0
    # ES body should be non-empty
    es_body = result["sections"].get("executive_summary", "")
    assert es_body


@pytest.mark.asyncio
async def test_writer_conclusion_context_is_truncated() -> None:
    """Analysis body injected into conclusion prompt is truncated to 500 chars."""
    llm = FakeLLM("Conclusion.")
    node = WriterNode(llm)

    state = {
        "outline": [
            {"key": "analysis", "title": "Analysis", "section_role": "analysis"},
            {"key": "conclusion", "title": "Conclusion", "section_role": "conclusion"},
        ],
        "curated": [],
        "curated_by_section": {},
        "claim_plans": {
            "analysis": _make_analysis_claims(1),
            "conclusion": [
                {
                    "claim_id": "conclusion-1",
                    "claim": "Synthesis",
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
    conclusion_prompts = [p for p in llm.prompts if "統合クレーム" in p or "結論" in p]
    assert len(conclusion_prompts) >= 1
    prompt = conclusion_prompts[0]
    # Extract the prior_analysis content
    if "<prior_analysis>" in prompt:
        start = prompt.index("<prior_analysis>") + len("<prior_analysis>")
        end = prompt.index("</prior_analysis>")
        analysis_context = prompt[start:end]
        assert len(analysis_context) <= 500, f"Analysis context should be ≤500 chars, got {len(analysis_context)}"


# ---------------------------------------------------------------------------
# AC-5: 正式カバレッジ
# ---------------------------------------------------------------------------


def test_conclusion_fallback_prefers_numeric_analysis_claims() -> None:
    """Deterministic conclusion prefers analysis claims with numeric_facts."""
    claims = _make_analysis_claims(5)  # first 2 have numeric_facts
    result = _deterministic_conclusion_claims(claims, [], topic="test")
    # At least one result claim should carry numeric_facts forward
    has_numeric = any(c.get("numeric_facts") for c in result)
    assert has_numeric, "Conclusion fallback should prefer claims with numeric data"


def test_es_fallback_prefers_diverse_sources() -> None:
    """ES fallback prefers claims from diverse sources."""
    all_claims = {
        "analysis": [
            {
                "claim_id": f"a-{i}",
                "claim": f"Claim {i}",
                "claim_type": "factual",
                "evidence_ids": ["art-1"],  # all same source
                "supporting_quotes": [f"Q{i}"],
                "numeric_facts": [],
                "novelty_against": [],
                "must_cite": True,
            }
            for i in range(1, 5)
        ]
        + [
            {
                "claim_id": "a-diverse",
                "claim": "Diverse claim",
                "claim_type": "statistical",
                "evidence_ids": ["art-2"],  # different source
                "supporting_quotes": ["QD"],
                "numeric_facts": ["42%"],
                "novelty_against": [],
                "must_cite": True,
            }
        ],
    }
    result = _deterministic_es_claims(all_claims, [], topic="test")
    source_ids = set()
    for claim in result:
        source_ids.update(claim.get("evidence_ids", []))
    assert len(source_ids) > 1, "ES fallback should include diverse sources"
