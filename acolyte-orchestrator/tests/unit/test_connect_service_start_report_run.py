"""Unit tests for start_report_run's ValueError → ConnectError code mapping.

StartRunUsecase can raise a ValueError for two distinct reasons: the report
doesn't exist, or the circuit breaker rejected the run because the most
recent run failed non-transiently within the cooldown window. These must
map to different Connect-RPC codes so a client (or a retry loop) can tell
"this report_id is wrong" (NOT_FOUND) apart from "back off and retry later"
(FAILED_PRECONDITION).
"""

from __future__ import annotations

from datetime import UTC, datetime, timedelta
from typing import TYPE_CHECKING
from unittest.mock import MagicMock
from uuid import UUID, uuid4

import pytest
from connectrpc.code import Code
from connectrpc.errors import ConnectError

from acolyte.domain.report import ChangeItem, Report, ReportSection, ReportVersion, SectionVersion
from acolyte.domain.run import ReportJob, ReportRun
from acolyte.gen.proto.alt.acolyte.v1 import acolyte_pb2
from acolyte.handler.connect_service import AcolyteConnectService

if TYPE_CHECKING:
    from acolyte.domain.brief import ReportBrief


class _FakeRepo:
    def __init__(self) -> None:
        self.reports: dict[UUID, Report] = {}

    async def get_report(self, report_id: UUID) -> Report | None:
        return self.reports.get(report_id)

    # Unused stubs for the rest of ReportRepositoryPort.
    async def create_report(self, title: str, report_type: str) -> Report:
        raise NotImplementedError

    async def create_brief(self, report_id: UUID, brief: ReportBrief) -> None:
        raise NotImplementedError

    async def get_brief(self, report_id: UUID) -> ReportBrief | None:
        raise NotImplementedError

    async def list_reports(self, cursor: str | None, limit: int) -> tuple[list[Report], str | None]:
        raise NotImplementedError

    async def bump_version(
        self,
        report_id: UUID,
        expected_version: int,
        change_reason: str,
        change_items: list[ChangeItem],
        **kwargs: object,
    ) -> int:
        raise NotImplementedError

    async def get_report_version(self, report_id: UUID, version_no: int) -> ReportVersion | None:
        raise NotImplementedError

    async def list_report_versions(
        self, report_id: UUID, cursor: str | None, limit: int
    ) -> tuple[list[ReportVersion], str | None]:
        raise NotImplementedError

    async def get_change_items(self, report_id: UUID, version_no: int) -> list[ChangeItem]:
        raise NotImplementedError

    async def create_section(self, report_id: UUID, section_key: str, display_order: int) -> ReportSection:
        raise NotImplementedError

    async def get_sections(self, report_id: UUID) -> list[ReportSection]:
        raise NotImplementedError

    async def bump_section_version(
        self,
        report_id: UUID,
        section_key: str,
        expected_version: int,
        body: str,
        citations: list[dict] | None = None,
    ) -> int:
        raise NotImplementedError

    async def get_section_version(self, report_id: UUID, section_key: str, version_no: int) -> SectionVersion | None:
        raise NotImplementedError

    async def has_active_run(self, report_id: UUID) -> bool:
        raise NotImplementedError

    async def delete_report(self, report_id: UUID) -> None:
        raise NotImplementedError


class _FakeJobQueue:
    def __init__(self) -> None:
        self.latest_run: ReportRun | None = None

    async def get_latest_run_for_report(self, report_id: UUID) -> ReportRun | None:
        return self.latest_run

    async def create_run(self, report_id: UUID, target_version_no: int) -> ReportRun:
        return ReportRun(
            run_id=uuid4(),
            report_id=report_id,
            target_version_no=target_version_no,
            run_status="pending",
        )

    # Unused stubs for the rest of JobQueuePort.
    async def get_run(self, run_id: UUID) -> ReportRun | None:
        raise NotImplementedError

    async def get_active_run_for_report(self, report_id: UUID) -> ReportRun | None:
        raise NotImplementedError

    async def claim_job(self, worker_id: str) -> ReportJob | None:
        raise NotImplementedError

    async def update_job_status(self, job_id: UUID, status: str) -> None:
        raise NotImplementedError

    async def complete_job(self, job_id: UUID) -> None:
        raise NotImplementedError

    async def fail_job(self, job_id: UUID, failure_message: str) -> None:
        raise NotImplementedError

    async def mark_running(self, run_id: UUID, planner_model: str, writer_model: str, critic_model: str) -> None:
        raise NotImplementedError

    async def complete_run(self, run_id: UUID) -> None:
        raise NotImplementedError

    async def fail_run(self, run_id: UUID, failure_code: str, failure_message: str) -> None:
        raise NotImplementedError


def _report(report_id: UUID) -> Report:
    return Report(
        report_id=report_id,
        title="Weekly briefing",
        report_type="weekly_briefing",
        current_version=0,
        latest_successful_run_id=None,
        created_at=datetime.now(UTC),
    )


@pytest.mark.asyncio
async def test_start_report_run_maps_report_not_found_to_not_found() -> None:
    repo = _FakeRepo()
    jobs = _FakeJobQueue()
    service = AcolyteConnectService(MagicMock(), repo, job_queue=jobs)

    with pytest.raises(ConnectError) as exc_info:
        await service.start_report_run(
            acolyte_pb2.StartReportRunRequest(report_id=str(uuid4())),
            ctx=None,  # type: ignore[bad-argument-type]
        )

    assert exc_info.value.code == Code.NOT_FOUND


@pytest.mark.asyncio
async def test_start_report_run_maps_breaker_rejection_to_failed_precondition() -> None:
    report_id = uuid4()
    repo = _FakeRepo()
    repo.reports[report_id] = _report(report_id)
    jobs = _FakeJobQueue()
    jobs.latest_run = ReportRun(
        run_id=uuid4(),
        report_id=report_id,
        target_version_no=1,
        run_status="failed",
        finished_at=datetime.now(UTC) - timedelta(minutes=1),
        failure_code="no_content",
        failure_message="content-store pipeline produced zero groundable content",
    )
    service = AcolyteConnectService(MagicMock(), repo, job_queue=jobs)

    with pytest.raises(ConnectError) as exc_info:
        await service.start_report_run(
            acolyte_pb2.StartReportRunRequest(report_id=str(report_id)),
            ctx=None,  # type: ignore[bad-argument-type]
        )

    assert exc_info.value.code == Code.FAILED_PRECONDITION


@pytest.mark.asyncio
async def test_start_report_run_succeeds_when_no_prior_failure() -> None:
    report_id = uuid4()
    repo = _FakeRepo()
    repo.reports[report_id] = _report(report_id)
    jobs = _FakeJobQueue()

    service = AcolyteConnectService(MagicMock(), repo, job_queue=jobs)
    response = await service.start_report_run(
        acolyte_pb2.StartReportRunRequest(report_id=str(report_id)),
        ctx=None,  # type: ignore[bad-argument-type]
    )

    assert response.run_id
