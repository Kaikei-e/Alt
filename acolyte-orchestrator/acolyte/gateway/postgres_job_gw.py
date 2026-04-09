"""PostgreSQL job gateway — JobQueuePort implementation."""

from __future__ import annotations

from datetime import datetime
from typing import TYPE_CHECKING
from uuid import UUID

import structlog

from acolyte.domain.run import ReportJob, ReportRun

if TYPE_CHECKING:
    from psycopg_pool import AsyncConnectionPool

logger = structlog.get_logger(__name__)


class PostgresJobGateway:
    """Job queue and run lifecycle backed by PostgreSQL."""

    def __init__(self, pool: AsyncConnectionPool) -> None:
        self._pool = pool

    async def create_run(self, report_id: UUID, target_version_no: int) -> ReportRun:
        async with self._pool.connection() as conn:
            async with conn.transaction():
                cur = await conn.execute(
                    "INSERT INTO report_runs (report_id, target_version_no) "
                    "VALUES (%s, %s) "
                    "RETURNING run_id, report_id, target_version_no, run_status, "
                    "planner_model, writer_model, critic_model, "
                    "started_at, finished_at, failure_code, failure_message",
                    [report_id, target_version_no],
                )
                r = await cur.fetchone()
                run = ReportRun(
                    run_id=r[0],
                    report_id=r[1],
                    target_version_no=r[2],
                    run_status=r[3],
                    planner_model=r[4],
                    writer_model=r[5],
                    critic_model=r[6],
                    started_at=r[7],
                    finished_at=r[8],
                    failure_code=r[9],
                    failure_message=r[10],
                )

                await conn.execute(
                    "INSERT INTO report_jobs (run_id) VALUES (%s)",
                    [run.run_id],
                )

                return run

    async def get_run(self, run_id: UUID) -> ReportRun | None:
        async with self._pool.connection() as conn:
            cur = await conn.execute(
                "SELECT run_id, report_id, target_version_no, run_status, "
                "planner_model, writer_model, critic_model, "
                "started_at, finished_at, failure_code, failure_message "
                "FROM report_runs WHERE run_id = %s",
                [run_id],
            )
            r = await cur.fetchone()
            if r is None:
                return None
            return ReportRun(
                run_id=r[0],
                report_id=r[1],
                target_version_no=r[2],
                run_status=r[3],
                planner_model=r[4],
                writer_model=r[5],
                critic_model=r[6],
                started_at=r[7],
                finished_at=r[8],
                failure_code=r[9],
                failure_message=r[10],
            )

    async def claim_job(self, worker_id: str) -> ReportJob | None:
        """Claim a pending job using SELECT ... FOR UPDATE SKIP LOCKED."""
        async with self._pool.connection() as conn:
            async with conn.transaction():
                cur = await conn.execute(
                    "SELECT job_id, run_id, job_status, attempt_no, claimed_by, claimed_at, "
                    "available_at, created_at "
                    "FROM report_jobs "
                    "WHERE job_status = 'pending' AND available_at <= NOW() "
                    "ORDER BY created_at "
                    "LIMIT 1 "
                    "FOR UPDATE SKIP LOCKED",
                )
                r = await cur.fetchone()
                if r is None:
                    return None

                job_id = r[0]
                await conn.execute(
                    "UPDATE report_jobs SET job_status = 'claimed', claimed_by = %s, "
                    "claimed_at = NOW(), attempt_no = attempt_no + 1 "
                    "WHERE job_id = %s",
                    [worker_id, job_id],
                )

                return ReportJob(
                    job_id=r[0],
                    run_id=r[1],
                    job_status="claimed",
                    attempt_no=r[3] + 1,
                    claimed_by=worker_id,
                    claimed_at=datetime.now(),
                    available_at=r[6],
                    created_at=r[7],
                )

    async def update_job_status(self, job_id: UUID, status: str) -> None:
        async with self._pool.connection() as conn:
            await conn.execute(
                "UPDATE report_jobs SET job_status = %s WHERE job_id = %s",
                [status, job_id],
            )

    async def complete_job(self, job_id: UUID) -> None:
        await self.update_job_status(job_id, "succeeded")

    async def fail_job(self, job_id: UUID, failure_message: str) -> None:
        await self.update_job_status(job_id, "failed")

    async def complete_run(self, run_id: UUID) -> None:
        async with self._pool.connection() as conn:
            await conn.execute(
                "UPDATE report_runs SET run_status = 'succeeded', finished_at = NOW() WHERE run_id = %s",
                [run_id],
            )

    async def fail_run(self, run_id: UUID, failure_code: str, failure_message: str) -> None:
        async with self._pool.connection() as conn:
            await conn.execute(
                "UPDATE report_runs SET run_status = 'failed', finished_at = NOW(), "
                "failure_code = %s, failure_message = %s WHERE run_id = %s",
                [failure_code, failure_message, run_id],
            )
