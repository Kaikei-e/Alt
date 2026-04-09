"""Unit tests for create report usecase."""

from __future__ import annotations

from datetime import UTC, datetime
from uuid import uuid4

import pytest

from acolyte.domain.report import Report
from acolyte.usecase.create_report_uc import CreateReportUsecase


class FakeReportRepo:
    def __init__(self) -> None:
        self.reports: list[Report] = []

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
