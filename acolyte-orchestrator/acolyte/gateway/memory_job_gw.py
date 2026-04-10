"""In-memory job gateway — for testing and initial development."""

from __future__ import annotations

from datetime import UTC, datetime
from uuid import UUID, uuid4

from acolyte.domain.run import ReportJob, ReportRun


class MemoryJobGateway:
    """In-memory JobQueuePort implementation."""

    def __init__(self) -> None:
        self._runs: dict[UUID, ReportRun] = {}
        self._jobs: dict[UUID, ReportJob] = {}

    async def create_run(self, report_id: UUID, target_version_no: int) -> ReportRun:
        run = ReportRun(
            run_id=uuid4(),
            report_id=report_id,
            target_version_no=target_version_no,
            run_status="pending",
        )
        self._runs[run.run_id] = run
        job = ReportJob(
            job_id=uuid4(),
            run_id=run.run_id,
            job_status="pending",
            created_at=datetime.now(UTC),
        )
        self._jobs[job.job_id] = job
        return run

    async def get_run(self, run_id: UUID) -> ReportRun | None:
        return self._runs.get(run_id)

    async def claim_job(self, worker_id: str) -> ReportJob | None:
        for job in self._jobs.values():
            if job.job_status == "pending":
                claimed = ReportJob(
                    job_id=job.job_id,
                    run_id=job.run_id,
                    job_status="claimed",
                    attempt_no=job.attempt_no + 1,
                    claimed_by=worker_id,
                    claimed_at=datetime.now(UTC),
                    available_at=job.available_at,
                    created_at=job.created_at,
                )
                self._jobs[job.job_id] = claimed
                return claimed
        return None

    async def update_job_status(self, job_id: UUID, status: str) -> None:
        job = self._jobs.get(job_id)
        if job:
            self._jobs[job_id] = ReportJob(
                job_id=job.job_id,
                run_id=job.run_id,
                job_status=status,
                attempt_no=job.attempt_no,
                claimed_by=job.claimed_by,
                claimed_at=job.claimed_at,
                available_at=job.available_at,
                created_at=job.created_at,
            )

    async def complete_job(self, job_id: UUID) -> None:
        await self.update_job_status(job_id, "succeeded")

    async def fail_job(self, job_id: UUID, failure_message: str) -> None:
        await self.update_job_status(job_id, "failed")

    async def complete_run(self, run_id: UUID) -> None:
        run = self._runs.get(run_id)
        if run:
            self._runs[run_id] = ReportRun(
                run_id=run.run_id,
                report_id=run.report_id,
                target_version_no=run.target_version_no,
                run_status="succeeded",
                planner_model=run.planner_model,
                writer_model=run.writer_model,
                critic_model=run.critic_model,
                started_at=run.started_at,
                finished_at=datetime.now(UTC),
            )

    async def fail_run(self, run_id: UUID, failure_code: str, failure_message: str) -> None:
        run = self._runs.get(run_id)
        if run:
            self._runs[run_id] = ReportRun(
                run_id=run.run_id,
                report_id=run.report_id,
                target_version_no=run.target_version_no,
                run_status="failed",
                planner_model=run.planner_model,
                writer_model=run.writer_model,
                critic_model=run.critic_model,
                started_at=run.started_at,
                finished_at=datetime.now(UTC),
                failure_code=failure_code,
                failure_message=failure_message,
            )
