"""Per-operation run usecases.

Splits the monolithic ``RunManager`` into four handler-facing usecases:
``SubmitRunUsecase``, ``GetRunUsecase``,
``SubmitClassificationRunUsecase``, ``GetClassificationRunUsecase``.

Each usecase takes a ``Port`` (protocol) collaborator. The current
container wires ``RunManager`` as the concrete implementation, but the
port boundary lets tests (and future refactors in Phase 3-5) swap in
alternative gateways without touching handler code.
"""

from __future__ import annotations

from ..db.dao import RunRecord
from ..port.run_reader import RunReaderPort
from ..port.run_submitter import (
    ClassificationRunSubmitterPort,
    RunSubmitterPort,
)
from ..services.run_manager import ClassificationRunSubmission, RunSubmission


class SubmitRunUsecase:
    """Accept a clustering run submission and return the persisted record."""

    def __init__(self, *, submitter: RunSubmitterPort) -> None:
        self._submitter = submitter

    async def execute(self, submission: RunSubmission) -> RunRecord:
        return await self._submitter.create_run(submission)


class GetRunUsecase:
    """Fetch a clustering run record by ID (returns None if missing)."""

    def __init__(self, *, reader: RunReaderPort) -> None:
        self._reader = reader

    async def execute(self, run_id: int) -> RunRecord | None:
        return await self._reader.get_run(run_id)


class SubmitClassificationRunUsecase:
    """Accept a classification run submission and return the persisted record."""

    def __init__(self, *, submitter: ClassificationRunSubmitterPort) -> None:
        self._submitter = submitter

    async def execute(self, submission: ClassificationRunSubmission) -> RunRecord:
        return await self._submitter.create_classification_run(submission)


class GetClassificationRunUsecase:
    """Fetch a classification run record by ID (returns None if missing)."""

    def __init__(self, *, reader: RunReaderPort) -> None:
        self._reader = reader

    async def execute(self, run_id: int) -> RunRecord | None:
        return await self._reader.get_run(run_id)
