"""List reports usecase."""

from __future__ import annotations

from typing import TYPE_CHECKING

if TYPE_CHECKING:
    from acolyte.domain.report import Report
    from acolyte.port.report_repository import ReportRepositoryPort


class ListReportsUsecase:
    def __init__(self, report_repo: ReportRepositoryPort) -> None:
        self._report_repo = report_repo

    async def execute(self, cursor: str | None, limit: int) -> tuple[list[Report], str | None]:
        return await self._report_repo.list_reports(cursor, limit)
