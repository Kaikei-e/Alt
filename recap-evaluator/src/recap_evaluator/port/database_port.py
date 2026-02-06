"""Database port — abstract interface for database operations."""

from datetime import datetime
from typing import Any, Protocol
from uuid import UUID


class DatabasePort(Protocol):
    """Protocol for database operations against recap-db."""

    async def fetch_recent_jobs(
        self, days: int, status: str = "completed"
    ) -> list[dict[str, Any]]: ...

    async def fetch_job_articles(self, job_id: UUID) -> list[dict[str, Any]]: ...

    async def fetch_outputs(self, job_id: UUID) -> list[dict[str, Any]]: ...

    async def fetch_stage_logs(self, job_id: UUID) -> list[dict[str, Any]]: ...

    async def fetch_stage_logs_batch(
        self, job_ids: list[UUID]
    ) -> dict[UUID, list[dict[str, Any]]]: ...

    async def fetch_preprocess_metrics(self, job_id: UUID) -> dict[str, Any] | None: ...

    async def fetch_preprocess_metrics_batch(
        self, job_ids: list[UUID]
    ) -> dict[UUID, dict[str, Any]]: ...

    async def fetch_subworker_runs(self, job_id: UUID) -> list[dict[str, Any]]: ...

    async def fetch_clusters_for_run(self, run_id: UUID) -> list[dict[str, Any]]: ...

    async def fetch_genre_learning_results(
        self, job_id: UUID
    ) -> list[dict[str, Any]]: ...

    async def fetch_evaluation_by_id(
        self, evaluation_id: UUID
    ) -> dict[str, Any] | None: ...

    async def fetch_evaluation_history(
        self, evaluation_type: str | None = None, limit: int = 30
    ) -> list[dict[str, Any]]: ...

    async def save_evaluation_run(
        self,
        evaluation_id: UUID,
        evaluation_type: str,
        job_ids: list[UUID],
        metrics: dict[str, Any],
        created_at: datetime,
    ) -> None: ...
