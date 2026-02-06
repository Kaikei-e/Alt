"""ManageRunUsecase: orchestrates run lifecycle via ports.

Extracted from services/run_manager.py. The original RunManager class
remains as the canonical implementation; this usecase wraps it while
depending on port protocols for testability.
"""

from __future__ import annotations

from typing import Optional

import structlog

from ..db.dao import RunRecord
from ..infra.config import Settings
from ..services.run_manager import (
    ClassificationRunSubmission,
    RunManager,
    RunSubmission,
)

LOGGER = structlog.get_logger(__name__)


class ManageRunUsecase:
    """Usecase wrapping RunManager for Clean Architecture compliance.

    Provides the same API as RunManager but expressed in terms of
    usecase semantics. The actual orchestration logic (idempotency,
    background scheduling, two-phase DB pattern) lives in RunManager.
    """

    def __init__(
        self,
        *,
        run_manager: RunManager,
        settings: Settings,
    ) -> None:
        self._run_manager = run_manager
        self._settings = settings

    async def create_run(self, submission: RunSubmission) -> RunRecord:
        """Create a new clustering run or return existing idempotent run.

        Raises:
            ConcurrentRunError: If a run is already in progress.
            IdempotencyMismatchError: If key reused with different payload.
        """
        return await self._run_manager.create_run(submission)

    async def create_classification_run(
        self, submission: ClassificationRunSubmission
    ) -> RunRecord:
        """Create a new classification run or return existing idempotent run."""
        return await self._run_manager.create_classification_run(submission)

    async def get_run(self, run_id: int) -> Optional[RunRecord]:
        """Fetch a run by its ID."""
        return await self._run_manager.get_run(run_id)

    async def shutdown(self) -> None:
        """Cancel all pending tasks and wait for completion."""
        await self._run_manager.shutdown()
