"""Port protocols for run submission.

Defined as ``typing.Protocol`` so any object (RunManager, a test fake,
or a future per-operation service) that satisfies the shape can be
injected into the submit usecases. The protocols deliberately cover
only the methods the submit usecases need — keeping the boundary thin.
"""

from __future__ import annotations

from typing import Protocol

from ..db.dao import RunRecord
from ..services.run_manager import ClassificationRunSubmission, RunSubmission


class RunSubmitterPort(Protocol):
    """Port that accepts a clustering run submission."""

    async def create_run(self, submission: RunSubmission) -> RunRecord: ...


class ClassificationRunSubmitterPort(Protocol):
    """Port that accepts a classification run submission."""

    async def create_classification_run(
        self, submission: ClassificationRunSubmission
    ) -> RunRecord: ...
