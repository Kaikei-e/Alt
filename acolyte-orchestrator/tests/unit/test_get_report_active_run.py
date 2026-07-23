"""GetReport must surface the latest pending/running ReportRun via active_run.

Lets the FE resume polling after navigation/reload without remembering the
run_id client-side. The backend is the single source of truth for in-flight
runs.
"""

from __future__ import annotations

from datetime import UTC, datetime
from unittest.mock import MagicMock
from uuid import UUID, uuid4

import pytest

from acolyte.domain.brief import ReportBrief
from acolyte.domain.report import (
    ChangeItem,
    Report,
    ReportSection,
    ReportVersion,
    SectionVersion,
)
from acolyte.domain.run import ReportJob, ReportRun
from acolyte.gen.proto.alt.acolyte.v1 import acolyte_pb2
from acolyte.handler.connect_service import AcolyteConnectService


class _FakeRepo:
    def __init__(self) -> None:
        self.reports: dict[UUID, Report] = {}
        self.briefs: dict[UUID, ReportBrief] = {}

    async def get_report(self, report_id: UUID) -> Report | None:
        return self.reports.get(report_id)

    async def get_brief(self, report_id: UUID) -> ReportBrief | None:
        return self.briefs.get(report_id)

    async def get_sections(self, report_id: UUID) -> list[ReportSection]:
        return []

    async def get_section_version(self, report_id: UUID, section_key: str, version_no: int) -> SectionVersion | None:
        return None

    # Stubs for the rest of the port (not exercised here).
    async def create_report(self, title: str, report_type: str) -> Report:
        raise NotImplementedError

    async def create_brief(self, report_id: UUID, brief: ReportBrief) -> None:
        raise NotImplementedError

    async def list_reports(self, cursor: str | None, limit: int) -> tuple[list[Report], str | None]:
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

    async def create_section(self, report_id: UUID, section_key: str, display_order: int) -> ReportSection:
        raise NotImplementedError

    async def bump_section_version(
        self,
        report_id: UUID,
        section_key: str,
        expected_version: int,
        body: str,
        citations: list[dict] | None = None,
    ) -> int:
        return expected_version + 1

    async def has_active_run(self, report_id: UUID) -> bool:
        return False

    async def delete_report(self, report_id: UUID) -> None:
        return None


class _FakeJobQueue:
    def __init__(self, active: ReportRun | None) -> None:
        self._active = active
        self.calls: list[UUID] = []

    async def get_active_run_for_report(self, report_id: UUID) -> ReportRun | None:
        self.calls.append(report_id)
        return self._active

    # Unused stubs for the rest of the JobQueuePort.
    async def create_run(self, report_id: UUID, target_version_no: int) -> ReportRun:
        raise NotImplementedError

    async def get_run(self, run_id: UUID) -> ReportRun | None:
        return None

    async def get_latest_run_for_report(self, report_id: UUID) -> ReportRun | None:
        return None

    async def claim_job(self, worker_id: str) -> ReportJob | None:
        return None

    async def update_job_status(self, job_id: UUID, status: str) -> None:
        return None

    async def complete_job(self, job_id: UUID) -> None:
        return None

    async def fail_job(self, job_id: UUID, failure_message: str) -> None:
        return None

    async def mark_running(self, run_id: UUID, planner_model: str, writer_model: str, critic_model: str) -> None:
        return None

    async def complete_run(self, run_id: UUID) -> None:
        return None

    async def fail_run(self, run_id: UUID, failure_code: str, failure_message: str) -> None:
        return None


def _seed_report(repo: _FakeRepo) -> UUID:
    rid = uuid4()
    repo.reports[rid] = Report(
        report_id=rid,
        title="Iran Outlook 2026",
        report_type="weekly_briefing",
        current_version=0,
        latest_successful_run_id=None,
        created_at=datetime.now(UTC),
    )
    return rid


@pytest.mark.asyncio
async def test_get_report_includes_active_run_when_queue_has_one() -> None:
    repo = _FakeRepo()
    rid = _seed_report(repo)

    run = ReportRun(
        run_id=uuid4(),
        report_id=rid,
        target_version_no=1,
        run_status="running",
        started_at=datetime(2026, 5, 5, 6, 6, 0, tzinfo=UTC),
    )
    queue = _FakeJobQueue(active=run)

    service = AcolyteConnectService(MagicMock(), repo, job_queue=queue)
    response = await service.get_report(
        acolyte_pb2.GetReportRequest(report_id=str(rid)),
        ctx=None,  # type: ignore[bad-argument-type]
    )

    assert queue.calls == [rid]
    assert response.HasField("active_run")
    assert response.active_run.run_id == str(run.run_id)
    assert response.active_run.report_id == str(rid)
    assert response.active_run.run_status == "running"
    assert response.active_run.target_version_no == 1
    assert response.active_run.started_at == "2026-05-05T06:06:00+00:00"


@pytest.mark.asyncio
async def test_get_report_omits_active_run_when_queue_returns_none() -> None:
    repo = _FakeRepo()
    rid = _seed_report(repo)
    queue = _FakeJobQueue(active=None)

    service = AcolyteConnectService(MagicMock(), repo, job_queue=queue)
    response = await service.get_report(
        acolyte_pb2.GetReportRequest(report_id=str(rid)),
        ctx=None,  # type: ignore[bad-argument-type]
    )

    assert queue.calls == [rid]
    assert not response.HasField("active_run")


@pytest.mark.asyncio
async def test_get_report_skips_queue_lookup_when_job_queue_is_none() -> None:
    """Backward-compat: handler must not crash when no JobQueuePort is wired."""
    repo = _FakeRepo()
    rid = _seed_report(repo)

    service = AcolyteConnectService(MagicMock(), repo, job_queue=None)
    response = await service.get_report(
        acolyte_pb2.GetReportRequest(report_id=str(rid)),
        ctx=None,  # type: ignore[bad-argument-type]
    )

    assert not response.HasField("active_run")
