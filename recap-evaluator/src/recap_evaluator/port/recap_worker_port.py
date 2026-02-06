"""Recap-Worker API port — abstract interface for recap-worker communication."""

from typing import Any, Protocol


class RecapWorkerPort(Protocol):
    """Protocol for recap-worker API operations."""

    async def trigger_genre_evaluation(self) -> dict[str, Any] | None: ...

    async def fetch_latest_genre_evaluation(self) -> dict[str, Any] | None: ...

    async def fetch_genre_evaluation_by_id(
        self, run_id: str
    ) -> dict[str, Any] | None: ...
