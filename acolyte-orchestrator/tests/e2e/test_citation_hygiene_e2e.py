"""End-to-end guard for citation hygiene across Writer + Finalizer.

Covers the full pipeline slice that produces the rendered report body:
1. Writer runs claim-plan paragraphs with a scripted LLM.
2. Rejected paragraphs carry a ``citation_format_violation`` feedback.
3. Finalizer persists body with ``[Sn]`` markers intact and a Sources footer,
   never expanding markers to inline titles.
"""

from __future__ import annotations

from datetime import UTC, datetime
from uuid import uuid4

import pytest

from acolyte.domain.report import Report, ReportSection
from acolyte.domain.source_map import SourceMap
from acolyte.port.llm_provider import LLMResponse
from acolyte.usecase.graph.nodes.finalizer_node import FinalizerNode
from acolyte.usecase.graph.nodes.writer_node import WriterNode


class ScriptedLLM:
    def __init__(self, responses: list[str]) -> None:
        self._responses = list(responses)
        self.prompts: list[str] = []

    async def generate(self, prompt: str, **kwargs: object) -> LLMResponse:
        self.prompts.append(prompt)
        text = self._responses.pop(0) if self._responses else ""
        return LLMResponse(text=text, model="scripted")


class InMemoryReportRepo:
    def __init__(self) -> None:
        self.report_id = uuid4()
        self.report = Report(
            report_id=self.report_id,
            title="Citation hygiene e2e",
            report_type="market_analysis",
            current_version=0,
            latest_successful_run_id=None,
            created_at=datetime.now(UTC),
        )
        self.sections = [
            ReportSection(report_id=self.report_id, section_key="analysis", current_version=0, display_order=0)
        ]
        self.saved_bodies: dict[str, str] = {}
        self.saved_citations: dict[str, list[dict] | None] = {}

    async def get_report(self, report_id):
        return self.report

    async def bump_version(self, report_id, expected_version, change_reason, change_items, **kwargs):
        return expected_version + 1

    async def get_sections(self, report_id):
        return self.sections

    async def create_section(self, report_id, section_key, display_order):
        section = ReportSection(
            report_id=report_id, section_key=section_key, current_version=0, display_order=display_order
        )
        self.sections.append(section)
        return section

    async def bump_section_version(self, report_id, section_key, expected_version, body, citations=None):
        self.saved_bodies[section_key] = body
        self.saved_citations[section_key] = citations
        return expected_version + 1


def _claim() -> dict:
    return {
        "claim_id": "analysis-1",
        "claim": "市場は急速に拡大した",
        "claim_type": "statistical",
        "evidence_ids": ["uuid-alpha", "uuid-beta"],
        "supporting_quotes": ["市場は 30% 拡大"],
        "numeric_facts": ["30%"],
        "novelty_against": [],
        "must_cite": True,
    }


def _state_with_source_map(llm_responses: list[str] | None = None) -> tuple[dict, SourceMap]:
    sm = SourceMap()
    sm.register(
        "uuid-alpha",
        "AI Market Surge 2026",
        publisher="TechDaily",
        url="https://techdaily.example/ai-surge",
    )
    sm.register(
        "uuid-beta",
        "GPU Price Trends",
        publisher="ChipReport",
        url="https://chip.example/gpu",
    )

    state: dict = {
        "outline": [{"key": "analysis", "title": "Analysis", "section_role": "analysis"}],
        "curated": [],
        "curated_by_section": {"analysis": [{"id": "uuid-alpha", "title": "AI Market Surge 2026"}]},
        "claim_plans": {"analysis": [_claim()]},
        "brief": {"topic": "AI market"},
        "sections": {},
        "revision_count": 0,
        "source_map": sm.to_dict(),
    }
    if llm_responses is not None:
        state["_llm_responses"] = llm_responses
    return state, sm


@pytest.mark.asyncio
async def test_writer_rejects_inline_title_output_in_pipeline() -> None:
    """Writer rejects a paragraph that inlines an article title in brackets."""
    bad_output = "市場の拡大は [AI Market Surge 2026 | TechDaily | market, gpu] で確認されている。"
    llm = ScriptedLLM([bad_output])
    writer = WriterNode(llm)
    state, _ = _state_with_source_map()

    result = await writer(state)

    paras = result["section_paragraphs"]["analysis"]
    assert paras[0]["status"] == "rejected"
    assert "citation_format_violation" in paras[0]["revision_feedback"]
    assert paras[0]["body"] == ""


@pytest.mark.asyncio
async def test_clean_paragraph_persists_with_Sn_and_sources_footer() -> None:
    """Clean [Sn] output → persisted body keeps markers AND gets Sources footer."""
    clean = "市場は 30% 拡大したと報告されている [S1][S2]。"
    llm = ScriptedLLM([clean])
    writer = WriterNode(llm)
    state, _ = _state_with_source_map()

    writer_result = await writer(state)
    paras = writer_result["section_paragraphs"]["analysis"]
    assert paras[0]["status"] == "accepted"
    assert "[S1]" in paras[0]["body"]
    assert "[S2]" in paras[0]["body"]

    repo = InMemoryReportRepo()
    finalizer = FinalizerNode(repo)
    finalizer_state: dict = {
        "report_id": str(repo.report_id),
        "outline": state["outline"],
        "brief": state["brief"],
        "sections": writer_result["sections"],
        "best_sections": writer_result.get("best_sections", {}),
        "section_citations": writer_result.get("section_citations", {}),
        "source_map": state["source_map"],
    }

    await finalizer(finalizer_state)

    persisted = repo.saved_bodies["analysis"]
    assert "[S1]" in persisted
    assert "[S2]" in persisted
    assert "AI Market Surge 2026" not in persisted.split("---\nSources:\n")[0]
    assert "\n\n---\nSources:\n" in persisted
    footer = persisted.split("\n\n---\nSources:\n", 1)[1]
    assert "- [S1] AI Market Surge 2026 — TechDaily (https://techdaily.example/ai-surge)" in footer
    assert "- [S2] GPU Price Trends — ChipReport (https://chip.example/gpu)" in footer


@pytest.mark.asyncio
async def test_finalizer_never_expands_Sn_into_inline_titles() -> None:
    """Regression guard: [S1] markers must never be replaced by [title] inline."""
    repo = InMemoryReportRepo()
    finalizer = FinalizerNode(repo)

    sm = SourceMap()
    sm.register("uuid-alpha", "Article Title X", publisher="Pub", url="https://x.example")

    await finalizer(
        {
            "report_id": str(repo.report_id),
            "outline": [{"key": "analysis"}],
            "brief": {"topic": "X"},
            "sections": {"analysis": "Claim [S1] stands."},
            "best_sections": {},
            "section_citations": {"analysis": []},
            "source_map": sm.to_dict(),
        }
    )

    persisted = repo.saved_bodies["analysis"]
    body_before_footer = persisted.split("\n\n---\nSources:\n", 1)[0]
    assert body_before_footer == "Claim [S1] stands."
    assert "[Article Title X]" not in persisted
