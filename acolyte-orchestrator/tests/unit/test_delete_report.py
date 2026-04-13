"""Unit tests for DeleteReport Connect handler."""

from __future__ import annotations

from datetime import UTC, datetime
from unittest.mock import MagicMock
from uuid import UUID, uuid4

import pytest
from connectrpc.code import Code
from connectrpc.errors import ConnectError

from acolyte.domain.brief import ReportBrief
from acolyte.domain.report import ChangeItem, Report, ReportSection, ReportVersion, SectionVersion
from acolyte.gen.proto.alt.acolyte.v1 import acolyte_pb2
from acolyte.handler.connect_service import AcolyteConnectService


class FakeReportRepo:
    def __init__(self) -> None:
        self.reports: dict[UUID, Report] = {}
        self.briefs: dict[UUID, ReportBrief] = {}
        self.active_runs: dict[UUID, bool] = {}
        self.deleted: list[UUID] = []

    async def create_report(self, title: str, report_type: str) -> Report:
        raise NotImplementedError

    async def create_brief(self, report_id: UUID, brief: ReportBrief) -> None:
        self.briefs[report_id] = brief

    async def get_brief(self, report_id: UUID) -> ReportBrief | None:
        return self.briefs.get(report_id)

    async def get_report(self, report_id: UUID) -> Report | None:
        return self.reports.get(report_id)

    async def list_reports(self, cursor: str | None, limit: int) -> tuple[list[Report], str | None]:
        return list(self.reports.values()), None

    async def bump_version(self, *args: object, **kwargs: object) -> int:
        raise NotImplementedError

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

    async def get_sections(self, report_id: UUID) -> list[ReportSection]:
        return []

    async def bump_section_version(self, *args: object, **kwargs: object) -> int:
        raise NotImplementedError

    async def get_section_version(self, report_id: UUID, section_key: str, version_no: int) -> SectionVersion | None:
        return None

    async def has_active_run(self, report_id: UUID) -> bool:
        return self.active_runs.get(report_id, False)

    async def delete_report(self, report_id: UUID) -> None:
        self.reports.pop(report_id, None)
        self.briefs.pop(report_id, None)
        self.deleted.append(report_id)


def _service_with(repo: FakeReportRepo) -> AcolyteConnectService:
    fake_settings = MagicMock()
    fake_settings.resolve_service_secret.return_value = ""
    return AcolyteConnectService(fake_settings, repo)


def _seed_report(repo: FakeReportRepo) -> UUID:
    rid = uuid4()
    repo.reports[rid] = Report(
        report_id=rid,
        title="Disposable",
        report_type="custom",
        current_version=1,
        latest_successful_run_id=None,
        created_at=datetime.now(UTC),
    )
    return rid


@pytest.mark.asyncio
async def test_delete_report_happy_path() -> None:
    repo = FakeReportRepo()
    rid = _seed_report(repo)

    service = _service_with(repo)
    resp = await service.delete_report(acolyte_pb2.DeleteReportRequest(report_id=str(rid)), ctx=None)  # type: ignore[bad-argument-type]

    assert isinstance(resp, acolyte_pb2.DeleteReportResponse)
    assert rid not in repo.reports
    assert repo.deleted == [rid]


@pytest.mark.asyncio
async def test_delete_report_refuses_when_active_run_exists() -> None:
    repo = FakeReportRepo()
    rid = _seed_report(repo)
    repo.active_runs[rid] = True

    service = _service_with(repo)
    with pytest.raises(ConnectError) as exc:
        await service.delete_report(acolyte_pb2.DeleteReportRequest(report_id=str(rid)), ctx=None)  # type: ignore[bad-argument-type]

    assert exc.value.code == Code.FAILED_PRECONDITION
    assert rid in repo.reports
    assert repo.deleted == []


@pytest.mark.asyncio
async def test_delete_report_returns_not_found() -> None:
    repo = FakeReportRepo()

    service = _service_with(repo)
    with pytest.raises(ConnectError) as exc:
        await service.delete_report(acolyte_pb2.DeleteReportRequest(report_id=str(uuid4())), ctx=None)  # type: ignore[bad-argument-type]

    assert exc.value.code == Code.NOT_FOUND
    assert repo.deleted == []
