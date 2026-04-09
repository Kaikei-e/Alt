"""Create report usecase."""

from __future__ import annotations

from typing import TYPE_CHECKING

if TYPE_CHECKING:
    from acolyte.domain.report import Report
    from acolyte.port.report_repository import ReportRepositoryPort


class CreateReportUsecase:
    def __init__(self, report_repo: ReportRepositoryPort) -> None:
        self._report_repo = report_repo

    async def execute(self, title: str, report_type: str) -> Report:
        return await self._report_repo.create_report(title, report_type)
