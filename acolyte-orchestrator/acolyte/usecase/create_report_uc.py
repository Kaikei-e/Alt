"""Create report usecase."""

from __future__ import annotations

from typing import TYPE_CHECKING

if TYPE_CHECKING:
    from uuid import UUID

    from acolyte.domain.report import Report
    from acolyte.port.report_repository import ReportRepositoryPort


class CreateReportUsecase:
    def __init__(self, report_repo: ReportRepositoryPort) -> None:
        self._report_repo = report_repo

    async def execute(self, title: str, report_type: str, user_id: UUID) -> Report:
        return await self._report_repo.create_report(title, report_type, user_id=user_id)
