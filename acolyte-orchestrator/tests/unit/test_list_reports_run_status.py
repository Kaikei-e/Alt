"""Unit tests for ListReports' latest_run_status field.

Previously hardcoded to "" regardless of the report's actual run history
(connect_service.py:155). Must now reflect the report's most recent run
status via JobQueuePort.get_latest_run_for_report, regardless of that run's
outcome (pending/running/succeeded/failed/cancelled).
"""

from __future__ import annotations

from datetime import UTC, datetime
from uuid import UUID, uuid4

import pytest

from acolyte.domain.report import Report
from acolyte.domain.run import ReportJob, ReportRun
from acolyte.gen.proto.alt.acolyte.v1 import acolyte_pb2
from acolyte.handler.connect_service import AcolyteConnectService


class _FakeRepo:
    def __init__(self, reports: list[Report]) -> None:
        self._reports = reports

    async def list_reports(self, cursor: str | None, limit: int) -> tuple[list[Report], str | None]:
        return self._reports[:limit], None


class _FakeJobQueue:
    def __init__(self, latest_by_report: dict[UUID, ReportRun | None]) -> None:
        self._latest_by_report = latest_by_report
        self.calls: list[UUID] = []

    async def get_latest_run_for_report(self, report_id: UUID) -> ReportRun | None:
        self.calls.append(report_id)
        return self._latest_by_report.get(report_id)

    # Unused stubs for the rest of JobQueuePort.
    async def create_run(self, report_id: UUID, target_version_no: int) -> ReportRun:
        raise NotImplementedError

    async def get_run(self, run_id: UUID) -> ReportRun | None:
        return None

    async def get_active_run_for_report(self, report_id: UUID) -> ReportRun | None:
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


def _report(rid: UUID, title: str) -> Report:
    return Report(
        report_id=rid,
        title=title,
        report_type="weekly_briefing",
        current_version=1,
        latest_successful_run_id=None,
        created_at=datetime.now(UTC),
    )


@pytest.mark.asyncio
async def test_list_reports_returns_actual_run_status_for_failed_run() -> None:
    rid = uuid4()
    run = ReportRun(run_id=uuid4(), report_id=rid, target_version_no=2, run_status="failed", failure_code="no_evidence")
    repo = _FakeRepo([_report(rid, "Iran Outlook")])
    jobs = _FakeJobQueue({rid: run})

    service = AcolyteConnectService(object(), repo, job_queue=jobs)  # type: ignore[bad-argument-type]
    response = await service.list_reports(acolyte_pb2.ListReportsRequest(limit=10), ctx=None)  # type: ignore[bad-argument-type]

    assert jobs.calls == [rid]
    assert len(response.reports) == 1
    assert response.reports[0].latest_run_status == "failed"


@pytest.mark.asyncio
async def test_list_reports_returns_empty_status_when_no_run_exists() -> None:
    rid = uuid4()
    repo = _FakeRepo([_report(rid, "No runs yet")])
    jobs = _FakeJobQueue({})

    service = AcolyteConnectService(object(), repo, job_queue=jobs)  # type: ignore[bad-argument-type]
    response = await service.list_reports(acolyte_pb2.ListReportsRequest(limit=10), ctx=None)  # type: ignore[bad-argument-type]

    assert response.reports[0].latest_run_status == ""


@pytest.mark.asyncio
async def test_list_reports_returns_empty_status_when_job_queue_not_wired() -> None:
    """Backward-compat: handler must not crash when no JobQueuePort is wired."""
    rid = uuid4()
    repo = _FakeRepo([_report(rid, "No queue wired")])

    service = AcolyteConnectService(object(), repo, job_queue=None)  # type: ignore[bad-argument-type]
    response = await service.list_reports(acolyte_pb2.ListReportsRequest(limit=10), ctx=None)  # type: ignore[bad-argument-type]

    assert response.reports[0].latest_run_status == ""
