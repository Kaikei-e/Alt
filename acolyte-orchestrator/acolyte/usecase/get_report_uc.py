"""Get report usecase."""

from __future__ import annotations

from typing import TYPE_CHECKING
from uuid import UUID

if TYPE_CHECKING:
    from acolyte.domain.report import Report, ReportSection
    from acolyte.port.report_repository import ReportRepositoryPort


class GetReportUsecase:
    def __init__(self, report_repo: ReportRepositoryPort) -> None:
        self._report_repo = report_repo

    async def execute(self, report_id: UUID) -> tuple[Report | None, list[ReportSection]]:
        report = await self._report_repo.get_report(report_id)
        if report is None:
            return None, []
        sections = await self._report_repo.get_sections(report_id)
        return report, sections
