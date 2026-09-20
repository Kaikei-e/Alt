"""Backfill report owners usecase — automated startup gate & legacy migration."""

from __future__ import annotations

from typing import TYPE_CHECKING
from uuid import UUID

from acolyte.domain.exceptions import UnmappedLegacyReportsError

if TYPE_CHECKING:
    from acolyte.port.report_repository import ReportOwnerBackfillPort

__all__ = ["BackfillReportOwnersUsecase", "UnmappedLegacyReportsError"]


class BackfillReportOwnersUsecase:
    """Safely backfill owner user_id for legacy unowned reports.

    Acts as an automated startup gate ensuring no reports remain with user_id=NULL,
    preventing legacy data from being silently hidden by per-user ownership queries.
    """

    def __init__(self, report_repo: ReportOwnerBackfillPort) -> None:
        self._report_repo = report_repo

    async def execute(
        self,
        *,
        single_owner_id: UUID | None = None,
        mapping: dict[UUID, UUID] | None = None,
    ) -> int:
        """Backfill legacy reports with either a single owner UUID or an explicit mapping.

        Raises:
            ValueError: If both single_owner_id and mapping are provided.
            UnmappedLegacyReportsError: If unowned legacy reports exist and cannot be mapped.
        """
        if single_owner_id is not None and mapping is not None:
            msg = "Cannot backfill with both single_owner_id and mapping simultaneously"
            raise ValueError(msg)

        if mapping is not None:
            for k, v in mapping.items():
                if not isinstance(k, UUID) or not isinstance(v, UUID):
                    msg = (
                        f"Mapping keys and values must be UUID instances, got key {k!r} "
                        f"({type(k).__name__}) and value {v!r} ({type(v).__name__})"
                    )
                    raise TypeError(msg)

        return await self._report_repo.backfill_owners(
            single_owner_id=single_owner_id,
            mapping=mapping,
        )
