"""Unit tests for FinalizerNode best_sections persistence."""

from __future__ import annotations

from datetime import UTC, datetime
from typing import TYPE_CHECKING, cast
from uuid import UUID, uuid4

import pytest

from acolyte.domain.report import ChangeItem, Report, ReportSection, ReportVersion, SectionVersion
from acolyte.port.llm_provider import LLMResponse
from acolyte.usecase.graph.nodes.finalizer_node import FinalizerNode
from acolyte.usecase.graph.nodes.writer_node import WriterNode

if TYPE_CHECKING:
    from acolyte.domain.brief import ReportBrief
    from acolyte.usecase.graph.state import ReportGenerationState, SectionCitationDict


class FakeRepo:
    def __init__(self) -> None:
        self.report_id = uuid4()
        self.report = Report(
            report_id=self.report_id,
            title="Test",
            report_type="weekly_briefing",
            current_version=0,
            latest_successful_run_id=None,
            created_at=datetime.now(UTC),
        )
        self.sections = [
            ReportSection(report_id=self.report_id, section_key="analysis", current_version=0, display_order=0)
        ]
        self.saved_bodies: dict[str, str] = {}
        self.saved_citations: dict[str, list[dict] | None] = {}

    async def create_report(self, title: str, report_type: str, user_id: UUID) -> Report:
        return self.report

    async def create_brief(self, report_id: UUID, brief: ReportBrief) -> None:
        pass

    async def get_brief(self, report_id: UUID) -> ReportBrief | None:
        return None

    async def get_report(self, report_id: UUID) -> Report | None:
        return self.report

    async def list_reports(self, cursor: str | None, limit: int, user_id: UUID) -> tuple[list[Report], str | None]:
        return [], None

    async def bump_version(
        self,
        report_id: UUID,
        expected_version: int,
        change_reason: str,
        change_items: list[ChangeItem],
        **kwargs: object,
    ) -> int:
        return expected_version + 1

    async def get_report_version(self, report_id: UUID, version_no: int) -> ReportVersion | None:
        return None

    async def list_report_versions(
        self, report_id: UUID, cursor: str | None, limit: int
    ) -> tuple[list[ReportVersion], str | None]:
        return [], None

    async def get_change_items(self, report_id: UUID, version_no: int) -> list[ChangeItem]:
        return []

    async def get_sections(self, report_id: UUID) -> list[ReportSection]:
        return self.sections

    async def create_section(self, report_id: UUID, section_key: str, display_order: int) -> ReportSection:
        return ReportSection(
            report_id=report_id, section_key=section_key, current_version=0, display_order=display_order
        )

    async def bump_section_version(
        self,
        report_id: UUID,
        section_key: str,
        expected_version: int,
        body: str,
        citations: list[dict] | None = None,
    ) -> int:
        self.saved_bodies[section_key] = body
        self.saved_citations[section_key] = citations
        return expected_version + 1

    async def get_section_version(self, report_id: UUID, section_key: str, version_no: int) -> SectionVersion | None:
        return None

    async def has_active_run(self, report_id: UUID) -> bool:
        return False

    async def delete_report(self, report_id: UUID) -> None:
        return None


@pytest.mark.asyncio
async def test_finalizer_always_prefers_best_sections_when_available() -> None:
    """Finalizer should persist best_sections even if the latest revision is non-empty."""
    repo = FakeRepo()
    node = FinalizerNode(repo)

    state: ReportGenerationState = {
        "report_id": str(repo.report_id),
        "outline": [{"key": "analysis"}],
        "brief": {"topic": "AI"},
        "sections": {"analysis": "Short latest body."},
        "best_sections": {"analysis": "Better body from earlier revision."},
        "section_citations": {"analysis": []},
    }

    result = await node(state)

    assert result["final_version_no"] == 1
    assert repo.saved_bodies["analysis"] == "Better body from earlier revision."


@pytest.mark.asyncio
async def test_finalizer_persists_best_citations_corresponding_to_best_section_body() -> None:
    """When best_sections overrides sections[key], persisted citations must match the best body."""
    repo = FakeRepo()
    node = FinalizerNode(repo)

    earlier_citations: list[SectionCitationDict] = [
        {
            "claim_id": "analysis-1",
            "source_id": "art-rev1",
            "source_type": "article",
            "quote": "Earlier revision quote",
            "offset_start": 0,
            "offset_end": 22,
        }
    ]
    latest_citations: list[SectionCitationDict] = [
        {
            "claim_id": "analysis-1",
            "source_id": "art-rev2",
            "source_type": "article",
            "quote": "Latest revision quote",
            "offset_start": 0,
            "offset_end": 21,
        }
    ]

    state: ReportGenerationState = {
        "report_id": str(repo.report_id),
        "outline": [{"key": "analysis"}],
        "brief": {"topic": "AI"},
        "sections": {"analysis": "Short latest body."},
        "best_sections": {"analysis": "Better body from earlier revision."},
        "section_citations": {"analysis": latest_citations},
        "best_section_citations": {"analysis": earlier_citations},
    }

    result = await node(state)

    assert result["final_version_no"] == 1
    assert repo.saved_bodies["analysis"] == "Better body from earlier revision."
    assert repo.saved_citations["analysis"] == earlier_citations


@pytest.mark.asyncio
async def test_finalizer_old_checkpoint_without_best_citations_falls_back_only_when_bodies_match() -> None:
    """Missing best_section_citations in an old checkpoint must not crash:
    fall back to latest citations only when best body IS the latest body,
    otherwise persist no stale citations."""
    repo = FakeRepo()
    node = FinalizerNode(repo)

    latest_citations: list[SectionCitationDict] = [
        {"claim_id": "analysis-1", "source_id": "art-rev2", "source_type": "article", "quote": "q"}
    ]

    # Case 1: best body != latest body -> do not persist stale latest citations
    state_mismatch: ReportGenerationState = {
        "report_id": str(repo.report_id),
        "outline": [{"key": "analysis"}],
        "brief": {"topic": "AI"},
        "sections": {"analysis": "Short latest body."},
        "best_sections": {"analysis": "Better body from earlier revision."},
        "section_citations": {"analysis": latest_citations},
    }
    await node(state_mismatch)
    assert repo.saved_citations["analysis"] == []

    # Case 2: best body == latest body -> fall back to latest citations
    state_match: ReportGenerationState = {
        "report_id": str(repo.report_id),
        "outline": [{"key": "analysis"}],
        "brief": {"topic": "AI"},
        "sections": {"analysis": "Same body."},
        "best_sections": {"analysis": "Same body."},
        "section_citations": {"analysis": latest_citations},
    }
    await node(state_match)
    assert repo.saved_citations["analysis"] == latest_citations


@pytest.mark.asyncio
async def test_writer_preserves_best_section_citations_when_later_revision_is_worse() -> None:
    """WriterNode records best_section_citations alongside best_sections so
    FinalizerNode persists revision-1 citations with revision-1 body."""

    class ScriptedLLM:
        def __init__(self, responses: list[str]) -> None:
            self._responses = list(responses)

        async def generate(self, prompt: str, **kwargs: object) -> LLMResponse:
            return LLMResponse(text=self._responses.pop(0), model="fake")

    writer = WriterNode(ScriptedLLM(["Detailed revision one body with grounded evidence.", "Short rev2."]))
    state: dict = {
        "outline": [{"key": "analysis", "title": "Analysis", "section_role": "analysis"}],
        "brief": {"topic": "AI"},
        "claim_plans": {
            "analysis": [
                {
                    "claim_id": "analysis-1",
                    "claim": "Claim 1",
                    "claim_type": "factual",
                    "evidence_ids": ["art-1"],
                    "supporting_quotes": ["Quote 1"],
                    "numeric_facts": [],
                    "novelty_against": [],
                    "must_cite": True,
                }
            ]
        },
    }

    # Revision 1: accepted longer body citing art-1
    rev1 = await writer(cast("ReportGenerationState", state))
    state.update(rev1)

    # Revision 2: critic asks to regenerate analysis-1 with different evidence, producing a shorter body
    state["critique"] = {
        "verdict": "revise",
        "revise_sections": ["analysis"],
        "claim_feedbacks": {"analysis": [{"claim_id": "analysis-1", "action": "regenerate", "reason": "tighten"}]},
    }
    state["claim_plans"] = {
        "analysis": [
            {
                "claim_id": "analysis-1",
                "claim": "Claim 1 revised",
                "claim_type": "factual",
                "evidence_ids": ["art-2"],
                "supporting_quotes": ["Quote 2"],
                "numeric_facts": [],
                "novelty_against": [],
                "must_cite": True,
            }
        ]
    }
    rev2 = await writer(cast("ReportGenerationState", state))
    state.update(rev2)

    assert state["sections"]["analysis"] == "Short rev2."
    assert state["best_sections"]["analysis"] == "Detailed revision one body with grounded evidence."
    assert state["section_citations"]["analysis"][0]["source_id"] == "art-2"
    assert state["best_section_citations"]["analysis"][0]["source_id"] == "art-1"

    repo = FakeRepo()
    state["report_id"] = str(repo.report_id)
    await FinalizerNode(repo)(cast("ReportGenerationState", state))

    assert repo.saved_bodies["analysis"] == "Detailed revision one body with grounded evidence."
    assert repo.saved_citations["analysis"] is not None
    assert repo.saved_citations["analysis"][0]["source_id"] == "art-1"
