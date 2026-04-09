"""Unit tests for get report usecase."""

from __future__ import annotations

from datetime import UTC, datetime
from uuid import UUID, uuid4

import pytest

from acolyte.domain.report import Report, ReportSection
from acolyte.usecase.get_report_uc import GetReportUsecase


class FakeReportRepo:
    def __init__(self) -> None:
        self.reports: dict[UUID, Report] = {}
        self.sections: dict[UUID, list[ReportSection]] = {}

    async def get_report(self, report_id: UUID) -> Report | None:
        return self.reports.get(report_id)

    async def get_sections(self, report_id: UUID) -> list[ReportSection]:
        return self.sections.get(report_id, [])


@pytest.mark.asyncio
async def test_get_existing_report() -> None:
    repo = FakeReportRepo()
    rid = uuid4()
    report = Report(
        report_id=rid,
        title="Test Report",
        report_type="weekly_briefing",
        current_version=2,
        latest_successful_run_id=None,
        created_at=datetime.now(UTC),
    )
    repo.reports[rid] = report
    repo.sections[rid] = [
        ReportSection(report_id=rid, section_key="executive_summary", current_version=1, display_order=0),
        ReportSection(report_id=rid, section_key="market_trends", current_version=1, display_order=1),
    ]

    uc = GetReportUsecase(repo)
    result_report, result_sections = await uc.execute(rid)

    assert result_report is not None
    assert result_report.title == "Test Report"
    assert len(result_sections) == 2


@pytest.mark.asyncio
async def test_get_nonexistent_report() -> None:
    repo = FakeReportRepo()
    uc = GetReportUsecase(repo)

    result_report, result_sections = await uc.execute(uuid4())

    assert result_report is None
    assert result_sections == []
