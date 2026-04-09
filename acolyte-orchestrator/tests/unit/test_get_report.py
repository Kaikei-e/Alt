"""Unit tests for get report usecase and handler citation serialization."""

from __future__ import annotations

import json
from datetime import UTC, datetime
from uuid import UUID, uuid4

import pytest

from acolyte.domain.report import Report, ReportSection, SectionVersion
from acolyte.usecase.get_report_uc import GetReportUsecase


class FakeReportRepo:
    def __init__(self) -> None:
        self.reports: dict[UUID, Report] = {}
        self.sections: dict[UUID, list[ReportSection]] = {}
        self.section_versions: dict[tuple[UUID, str, int], SectionVersion] = {}

    async def get_report(self, report_id: UUID) -> Report | None:
        return self.reports.get(report_id)

    async def get_sections(self, report_id: UUID) -> list[ReportSection]:
        return self.sections.get(report_id, [])

    async def get_section_version(
        self, report_id: UUID, section_key: str, version_no: int
    ) -> SectionVersion | None:
        return self.section_versions.get((report_id, section_key, version_no))


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


@pytest.mark.asyncio
async def test_handler_returns_stored_citations() -> None:
    """GetReport handler should serialize SectionVersion.citations, not hardcoded '[]'."""
    from unittest.mock import MagicMock

    from acolyte.gen.proto.alt.acolyte.v1 import acolyte_pb2
    from acolyte.handler.connect_service import AcolyteConnectService

    repo = FakeReportRepo()
    rid = uuid4()
    report = Report(
        report_id=rid,
        title="Test",
        report_type="weekly_briefing",
        current_version=1,
        latest_successful_run_id=None,
        created_at=datetime.now(UTC),
    )
    repo.reports[rid] = report
    repo.sections[rid] = [
        ReportSection(report_id=rid, section_key="analysis", current_version=1, display_order=0),
    ]
    citations = [
        {"claim_id": "analysis-1", "source_id": "art-1", "source_type": "article", "quote": "AI grew 20%"},
    ]
    repo.section_versions[(rid, "analysis", 1)] = SectionVersion(
        report_id=rid,
        section_key="analysis",
        version_no=1,
        body="Section body text",
        citations=citations,
        created_at=datetime.now(UTC),
    )

    fake_settings = MagicMock()
    fake_settings.resolve_service_secret.return_value = ""
    service = AcolyteConnectService(fake_settings, repo)
    request = acolyte_pb2.GetReportRequest(report_id=str(rid))
    response = await service.get_report(request, ctx=None)

    # Citations should be the actual stored data, not "[]"
    sec = response.sections[0]
    parsed = json.loads(sec.citations_json)
    assert len(parsed) == 1
    assert parsed[0]["claim_id"] == "analysis-1"
    assert parsed[0]["source_id"] == "art-1"
