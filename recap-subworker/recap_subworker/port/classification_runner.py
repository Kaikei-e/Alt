"""Classification runner port: Protocol for process-pool-backed batch classification."""

from __future__ import annotations

from typing import Any, Protocol, runtime_checkable


@runtime_checkable
class ClassificationRunnerPort(Protocol):
    """Port for dispatching genre classification to a dedicated worker pool."""

    async def predict_batch(self, texts: list[str]) -> list[dict[str, Any]]:
        """Classify a batch of texts, returning the same shape as ClassifierPort."""
        ...
