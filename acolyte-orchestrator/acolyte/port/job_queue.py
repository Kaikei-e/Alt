"""Job queue port — interface for job claim and lifecycle."""

from __future__ import annotations

from typing import Protocol
from uuid import UUID

from acolyte.domain.run import ReportJob, ReportRun


class JobQueuePort(Protocol):
    async def create_run(self, report_id: UUID, target_version_no: int) -> ReportRun: ...

    async def get_run(self, run_id: UUID) -> ReportRun | None: ...

    async def get_active_run_for_report(self, report_id: UUID) -> ReportRun | None: ...

    async def get_latest_run_for_report(self, report_id: UUID) -> ReportRun | None: ...

    async def list_running_runs(self) -> list[ReportRun]:
        """Return every unfinished run — run_status 'pending' or 'running' — across
        all reports, for startup reconciliation.

        'pending' is in scope because create_run persists the run before the
        pipeline reaches mark_running, so a restart in that window strands a run
        that never became 'running'. An implementation scoped to 'running' alone
        leaves such a run wedging get_active_run_for_report for its report forever.
        """
        ...

    async def claim_job(self, worker_id: str) -> ReportJob | None: ...

    async def update_job_status(self, job_id: UUID, status: str) -> None: ...

    async def complete_job(self, job_id: UUID) -> None: ...

    async def fail_job(self, job_id: UUID, failure_message: str) -> None: ...

    async def mark_running(self, run_id: UUID, planner_model: str, writer_model: str, critic_model: str) -> None: ...

    async def complete_run(self, run_id: UUID) -> None: ...

    async def fail_run(self, run_id: UUID, failure_code: str, failure_message: str) -> None: ...
