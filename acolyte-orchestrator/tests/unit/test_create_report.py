"""Unit tests for create report usecase."""

from __future__ import annotations

from datetime import UTC, datetime
from uuid import uuid4

import pytest

from acolyte.domain.brief import ReportBrief
from acolyte.domain.report import ChangeItem, Report, ReportSection, ReportVersion, SectionVersion
from acolyte.usecase.create_report_uc import CreateReportUsecase


class FakeReportRepo:
    def __init__(self) -> None:
        self.reports: list[Report] = []
        self.briefs: dict = {}

    async def create_brief(self, report_id: object, brief: ReportBrief) -> None:
        self.briefs[report_id] = brief

    async def get_brief(self, report_id: object) -> ReportBrief | None:
        return self.briefs.get(report_id)

    async def create_report(self, title: str, report_type: str) -> Report:
        report = Report(
            report_id=uuid4(),
            title=title,
            report_type=report_type,
            current_version=0,
            latest_successful_run_id=None,
            created_at=datetime.now(UTC),
        )
        self.reports.append(report)
        return report

    # --- Stubs required by ReportRepositoryPort ---

    async def get_report(self, report_id: object) -> Report | None:
        return None

    async def list_reports(self, cursor: str | None, limit: int) -> tuple[list[Report], str | None]:
        return self.reports, None

    async def bump_version(
        self,
        report_id: object,
        expected_version: int,
        change_reason: str,
        change_items: list[ChangeItem],
        **kwargs: object,
    ) -> int:
        return expected_version + 1

    async def get_report_version(self, report_id: object, version_no: int) -> ReportVersion | None:
        return None

    async def list_report_versions(
        self, report_id: object, cursor: str | None, limit: int
    ) -> tuple[list[ReportVersion], str | None]:
        return [], None

    async def get_change_items(self, report_id: object, version_no: int) -> list[ChangeItem]:
        return []

    async def create_section(self, report_id: object, section_key: str, display_order: int) -> ReportSection:
        raise NotImplementedError

    async def get_sections(self, report_id: object) -> list[ReportSection]:
        return []

    async def bump_section_version(
        self, report_id: object, section_key: str, expected_version: int, body: str, citations: list[dict] | None = None
    ) -> int:
        return expected_version + 1

    async def get_section_version(self, report_id: object, section_key: str, version_no: int) -> SectionVersion | None:
        return None


@pytest.mark.asyncio
async def test_create_report_returns_new_report() -> None:
    repo = FakeReportRepo()
    uc = CreateReportUsecase(repo)

    report = await uc.execute("Weekly AI Briefing", "weekly_briefing")

    assert report.title == "Weekly AI Briefing"
    assert report.report_type == "weekly_briefing"
    assert report.current_version == 0
    assert len(repo.reports) == 1


@pytest.mark.asyncio
async def test_create_multiple_reports() -> None:
    repo = FakeReportRepo()
    uc = CreateReportUsecase(repo)

    r1 = await uc.execute("Report 1", "weekly_briefing")
    r2 = await uc.execute("Report 2", "market_analysis")

    assert r1.report_id != r2.report_id
    assert len(repo.reports) == 2
