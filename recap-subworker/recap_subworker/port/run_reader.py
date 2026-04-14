"""Port protocol for fetching a run record by ID."""

from __future__ import annotations

from typing import Protocol

from ..db.dao import RunRecord


class RunReaderPort(Protocol):
    """Port that resolves ``run_id`` to a stored run record (or None)."""

    async def get_run(self, run_id: int) -> RunRecord | None: ...
