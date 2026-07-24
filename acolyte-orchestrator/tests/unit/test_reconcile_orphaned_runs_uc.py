"""Unit tests for ReconcileOrphanedRunsUsecase — startup crash recovery.

A run left in run_status='running' by a crashed/restarted process is never
revisited by anything else in this service (no poller, no auto-resume), so
it stays 'running' forever and wedges has_active_run / GetReport.active_run
for its report indefinitely. This usecase runs once at lifespan startup and
fails every such run outright, unblocking StartReportRun for that report.
"""

from __future__ import annotations

from uuid import UUID, uuid4

import pytest

from acolyte.domain.run import ReportJob, ReportRun
from acolyte.usecase.reconcile_orphaned_runs_uc import ReconcileOrphanedRunsUsecase


class _FakeJobQueue:
    def __init__(self, running: list[ReportRun]) -> None:
        self.running = running
        self.failed: list[tuple[UUID, str, str]] = []

    async def list_running_runs(self) -> list[ReportRun]:
        return self.running

    async def fail_run(self, run_id: UUID, failure_code: str, failure_message: str) -> None:
        self.failed.append((run_id, failure_code, failure_message))

    # Rest of JobQueuePort — not exercised by this usecase.
    async def create_run(self, report_id: UUID, target_version_no: int) -> ReportRun:
        raise NotImplementedError

    async def get_run(self, run_id: UUID) -> ReportRun | None:
        raise NotImplementedError

    async def get_active_run_for_report(self, report_id: UUID) -> ReportRun | None:
        raise NotImplementedError

    async def get_latest_run_for_report(self, report_id: UUID) -> ReportRun | None:
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


def _running_run() -> ReportRun:
    return ReportRun(
        run_id=uuid4(),
        report_id=uuid4(),
        target_version_no=1,
        run_status="running",
    )


@pytest.mark.asyncio
async def test_execute_fails_every_orphaned_running_run() -> None:
    run_a, run_b = _running_run(), _running_run()
    jobs = _FakeJobQueue([run_a, run_b])
    uc = ReconcileOrphanedRunsUsecase(jobs)

    count = await uc.execute()

    assert count == 2
    failed_ids = {run_id for run_id, _code, _msg in jobs.failed}
    assert failed_ids == {run_a.run_id, run_b.run_id}
    assert all(code == "orphaned_after_restart" for _run_id, code, _msg in jobs.failed)


@pytest.mark.asyncio
async def test_execute_is_a_noop_when_nothing_is_running() -> None:
    jobs = _FakeJobQueue([])
    uc = ReconcileOrphanedRunsUsecase(jobs)

    count = await uc.execute()

    assert count == 0
    assert jobs.failed == []
