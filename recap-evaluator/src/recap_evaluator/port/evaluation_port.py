"""Evaluation port — abstract interface for evaluators."""

from typing import Any, Protocol
from uuid import UUID


class EvaluatorPort(Protocol):
    """Protocol for evaluation components."""

    async def evaluate_batch(self, job_ids: list[UUID], **kwargs: Any) -> Any: ...
