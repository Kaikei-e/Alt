"""Domain-level exceptions shared across gateways and usecases."""

from __future__ import annotations

from typing import TYPE_CHECKING

if TYPE_CHECKING:
    from uuid import UUID


class StaleVersionError(Exception):
    """Raised when an optimistic lock fails due to version mismatch."""

    def __init__(self, report_id: UUID, expected_version: int) -> None:
        self.report_id = report_id
        self.expected_version = expected_version
        super().__init__(f"Stale version: report {report_id} expected v{expected_version}")
