"""Start report run usecase."""

from __future__ import annotations

from typing import TYPE_CHECKING
from uuid import UUID

if TYPE_CHECKING:
    from acolyte.domain.run import ReportRun
    from acolyte.port.job_queue import JobQueuePort
    from acolyte.port.report_repository import ReportRepositoryPort


class StartRunUsecase:
    def __init__(self, report_repo: ReportRepositoryPort, job_queue: JobQueuePort) -> None:
        self._report_repo = report_repo
        self._job_queue = job_queue

    async def execute(self, report_id: UUID) -> ReportRun:
        report = await self._report_repo.get_report(report_id)
        if report is None:
            raise ValueError(f"Report {report_id} not found")
        return await self._job_queue.create_run(report_id, report.current_version + 1)
