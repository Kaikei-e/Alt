"""Unit tests for scope propagation through the pipeline."""

from __future__ import annotations

import json
from datetime import UTC, datetime
from typing import cast
from uuid import UUID, uuid4

import pytest

from acolyte.domain.brief import ReportBrief
from acolyte.domain.report import ChangeItem, Report, ReportSection, ReportVersion, SectionVersion
from acolyte.port.evidence_provider import ArticleHit, ArticleMetadata, RecapHit
from acolyte.port.llm_provider import LLMResponse
from acolyte.usecase.graph.report_graph import build_report_graph


class FakeLLM:
    async def generate(self, prompt: str, **kwargs: object) -> LLMResponse:
        if "planner" in prompt.lower() or "plan" in prompt.lower() or "outline" in prompt.lower():
            return LLMResponse(
                text=json.dumps(
                    {
                        "reasoning": "Summary needed",
                        "sections": [{"key": "summary", "title": "Summary"}],
                    }
                ),
                model="fake",
            )
        if "critic" in prompt.lower() or "evaluate" in prompt.lower():
            return LLMResponse(
                text=json.dumps(
                    {
                        "reasoning": "ok",
                        "verdict": "accept",
                        "revise_sections": [],
                        "feedback": {},
                    }
                ),
                model="fake",
            )
        return LLMResponse(text="Generated content about the topic.", model="fake")


class FakeEvidence:
    async def search_articles(
        self,
        query: str,
        *,
        limit: int = 20,
        published_after: datetime | None = None,
        published_before: datetime | None = None,
    ) -> list[ArticleHit]:
        return [ArticleHit(article_id="art-1", title="Test", tags=["AI"], score=0.9)]

    async def fetch_article_metadata(self, article_ids: list[str]) -> list[ArticleMetadata]:
        return []

    async def fetch_article_body(self, article_id: str) -> str:
        return "Body."

    async def search_recaps(self, query: str, *, limit: int = 10) -> list[RecapHit]:
        return []


class FakeReportRepo:
    def __init__(self) -> None:
        self.reports: dict[UUID, Report] = {}
        self.briefs: dict[UUID, ReportBrief] = {}
        self.sections: dict[UUID, list[ReportSection]] = {}
        self.last_scope_snapshot: dict | None = None

    async def create_report(self, title: str, report_type: str) -> Report:
        rid = uuid4()
        report = Report(
            report_id=rid,
            title=title,
            report_type=report_type,
            current_version=0,
            latest_successful_run_id=None,
            created_at=datetime.now(UTC),
        )
        self.reports[rid] = report
        self.sections[rid] = []
        return report

    async def create_brief(self, report_id: UUID, brief: ReportBrief) -> None:
        self.briefs[report_id] = brief

    async def get_brief(self, report_id: UUID) -> ReportBrief | None:
        return self.briefs.get(report_id)

    async def get_report(self, report_id: UUID) -> Report | None:
        return self.reports.get(report_id)

    async def get_sections(self, report_id: UUID) -> list[ReportSection]:
        return self.sections.get(report_id, [])

    async def bump_version(
        self,
        report_id: UUID,
        expected_version: int,
        change_reason: str,
        change_items: list[ChangeItem],
        **kwargs: object,
    ) -> int:
        self.last_scope_snapshot = cast("dict | None", kwargs.get("scope_snapshot"))
        report = self.reports[report_id]
        new_v = expected_version + 1
        self.reports[report_id] = Report(
            report_id=report.report_id,
            title=report.title,
            report_type=report.report_type,
            current_version=new_v,
            latest_successful_run_id=report.latest_successful_run_id,
            created_at=report.created_at,
        )
        return new_v

    async def create_section(self, report_id: UUID, section_key: str, display_order: int) -> ReportSection:
        sec = ReportSection(
            report_id=report_id, section_key=section_key, current_version=0, display_order=display_order
        )
        self.sections.setdefault(report_id, []).append(sec)
        return sec

    async def bump_section_version(
        self,
        report_id: UUID,
        section_key: str,
        expected_version: int,
        body: str,
        citations: list[dict] | None = None,
    ) -> int:
        sections = self.sections.get(report_id, [])
        for i, s in enumerate(sections):
            if s.section_key == section_key:
                sections[i] = ReportSection(
                    report_id=report_id,
                    section_key=section_key,
                    current_version=expected_version + 1,
                    display_order=s.display_order,
                )
                break
        return expected_version + 1

    async def list_reports(self, cursor: str | None, limit: int) -> tuple[list[Report], str | None]:
        return list(self.reports.values()), None

    async def get_report_version(self, report_id: UUID, version_no: int) -> ReportVersion | None:
        return None

    async def list_report_versions(
        self, report_id: UUID, cursor: str | None, limit: int
    ) -> tuple[list[ReportVersion], str | None]:
        return [], None

    async def get_change_items(self, report_id: UUID, version_no: int) -> list[ChangeItem]:
        return []

    async def get_section_version(self, report_id: UUID, section_key: str, version_no: int) -> SectionVersion | None:
        return None

    async def has_active_run(self, report_id: UUID) -> bool:
        return False

    async def delete_report(self, report_id: UUID) -> None:
        self.reports.pop(report_id, None)
        self.briefs.pop(report_id, None)
        self.sections.pop(report_id, None)


@pytest.mark.asyncio
async def test_pipeline_with_valid_brief_succeeds() -> None:
    """Pipeline should complete when brief has valid topic."""
    repo = FakeReportRepo()
    report = await repo.create_report("AI Report", "weekly_briefing")
    brief = ReportBrief.from_scope({"topic": "AI semiconductor supply chain"}, "weekly_briefing")
    await repo.create_brief(report.report_id, brief)

    graph = build_report_graph(FakeLLM(), FakeEvidence(), repo)
    result = await graph.ainvoke(
        {
            "report_id": str(report.report_id),
            "run_id": str(uuid4()),
            "brief": brief.to_dict(),
            "revision_count": 0,
        }
    )

    assert result.get("final_version_no") == 1
    assert result.get("error") is None


@pytest.mark.asyncio
async def test_pipeline_scope_snapshot_preserved() -> None:
    """Finalizer should persist brief as scope_snapshot in report_versions."""
    repo = FakeReportRepo()
    report = await repo.create_report("AI Report", "weekly_briefing")
    brief = ReportBrief.from_scope({"topic": "AI semiconductor"}, "weekly_briefing")
    await repo.create_brief(report.report_id, brief)

    graph = build_report_graph(FakeLLM(), FakeEvidence(), repo)
    await graph.ainvoke(
        {
            "report_id": str(report.report_id),
            "run_id": str(uuid4()),
            "brief": brief.to_dict(),
            "revision_count": 0,
        }
    )

    assert repo.last_scope_snapshot is not None
    assert repo.last_scope_snapshot["topic"] == "AI semiconductor"
