"""Unit tests for get report usecase and handler citation serialization."""

from __future__ import annotations

import json
from datetime import UTC, datetime
from uuid import UUID, uuid4

import pytest

from acolyte.domain.brief import ReportBrief
from acolyte.domain.report import ChangeItem, Report, ReportSection, ReportVersion, SectionVersion
from acolyte.usecase.get_report_uc import GetReportUsecase


class FakeReportRepo:
    def __init__(self) -> None:
        self.reports: dict[UUID, Report] = {}
        self.briefs: dict[UUID, ReportBrief] = {}
        self.sections: dict[UUID, list[ReportSection]] = {}
        self.section_versions: dict[tuple[UUID, str, int], SectionVersion] = {}

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
        return report

    async def create_brief(self, report_id: UUID, brief: ReportBrief) -> None:
        self.briefs[report_id] = brief

    async def get_brief(self, report_id: UUID) -> ReportBrief | None:
        return self.briefs.get(report_id)

    async def get_report(self, report_id: UUID) -> Report | None:
        return self.reports.get(report_id)

    async def get_sections(self, report_id: UUID) -> list[ReportSection]:
        return self.sections.get(report_id, [])

    async def list_reports(self, cursor: str | None, limit: int) -> tuple[list[Report], str | None]:
        return list(self.reports.values()), None

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

    async def create_section(self, report_id: UUID, section_key: str, display_order: int) -> ReportSection:
        raise NotImplementedError

    async def bump_section_version(
        self, report_id: UUID, section_key: str, expected_version: int, body: str, citations: list[dict] | None = None
    ) -> int:
        return expected_version + 1

    async def get_section_version(self, report_id: UUID, section_key: str, version_no: int) -> SectionVersion | None:
        return self.section_versions.get((report_id, section_key, version_no))

    async def has_active_run(self, report_id: UUID) -> bool:
        return False

    async def delete_report(self, report_id: UUID) -> None:
        self.reports.pop(report_id, None)


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
    response = await service.get_report(request, ctx=None)  # type: ignore[bad-argument-type]

    # Citations should be the actual stored data, not "[]"
    sec = response.sections[0]
    parsed = json.loads(sec.citations_json)
    assert len(parsed) == 1
    assert parsed[0]["claim_id"] == "analysis-1"
    assert parsed[0]["source_id"] == "art-1"


@pytest.mark.asyncio
async def test_handler_returns_brief_scope() -> None:
    """GetReport handler should expose the persisted ReportBrief as Report.scope."""
    from unittest.mock import MagicMock

    from acolyte.gen.proto.alt.acolyte.v1 import acolyte_pb2
    from acolyte.handler.connect_service import AcolyteConnectService

    repo = FakeReportRepo()
    rid = uuid4()
    report = Report(
        report_id=rid,
        title="LLM Safety",
        report_type="weekly_briefing",
        current_version=1,
        latest_successful_run_id=None,
        created_at=datetime.now(UTC),
    )
    repo.reports[rid] = report
    repo.briefs[rid] = ReportBrief(
        topic="LLM safety 2026",
        report_type="weekly_briefing",
        time_range="last_7_days",
        entities=["openai", "anthropic"],
        exclude_topics=["regulation"],
        constraints={"audience": "researchers"},
    )

    fake_settings = MagicMock()
    fake_settings.resolve_service_secret.return_value = ""
    service = AcolyteConnectService(fake_settings, repo)
    request = acolyte_pb2.GetReportRequest(report_id=str(rid))
    response = await service.get_report(request, ctx=None)  # type: ignore[bad-argument-type]

    scope = dict(response.report.scope)
    assert scope["topic"] == "LLM safety 2026"
    assert scope["time_range"] == "last_7_days"
    assert scope["entities"] == "openai,anthropic"
    assert scope["exclude"] == "regulation"
    assert scope["audience"] == "researchers"


@pytest.mark.asyncio
async def test_handler_returns_empty_scope_when_no_brief() -> None:
    """When no brief is persisted, Report.scope must be present and empty (not crash)."""
    from unittest.mock import MagicMock

    from acolyte.gen.proto.alt.acolyte.v1 import acolyte_pb2
    from acolyte.handler.connect_service import AcolyteConnectService

    repo = FakeReportRepo()
    rid = uuid4()
    repo.reports[rid] = Report(
        report_id=rid,
        title="Untitled",
        report_type="custom",
        current_version=0,
        latest_successful_run_id=None,
        created_at=datetime.now(UTC),
    )

    fake_settings = MagicMock()
    fake_settings.resolve_service_secret.return_value = ""
    service = AcolyteConnectService(fake_settings, repo)
    request = acolyte_pb2.GetReportRequest(report_id=str(rid))
    response = await service.get_report(request, ctx=None)  # type: ignore[bad-argument-type]

    assert dict(response.report.scope) == {}
