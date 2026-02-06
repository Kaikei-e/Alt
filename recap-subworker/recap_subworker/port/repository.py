"""Repository port: Protocol for run persistence."""

from __future__ import annotations

from typing import Any, Optional, Protocol, runtime_checkable
from uuid import UUID

from ..db.dao import (
    AdminJobRecord,
    DiagnosticEntry,
    NewRun,
    PersistedCluster,
    RunRecord,
)


@runtime_checkable
class RunRepositoryPort(Protocol):
    """Port for persisting and retrieving subworker runs."""

    async def insert_run(self, run: NewRun) -> int:
        """Insert a new run and return its ID."""
        ...

    async def find_run_by_idempotency(
        self, job_id: UUID, genre: str, idempotency_key: str
    ) -> Optional[RunRecord]:
        """Find an existing run by idempotency key."""
        ...

    async def has_running_run(self, job_id: UUID, genre: str) -> bool:
        """Check if a running run exists for the given job and genre."""
        ...

    async def mark_run_success(
        self,
        run_id: int,
        cluster_count: int,
        response_payload: dict[str, Any],
        status: str = "succeeded",
    ) -> None:
        """Mark a run as successful with response data."""
        ...

    async def mark_run_failure(
        self, run_id: int, status: str, error_message: str
    ) -> None:
        """Mark a run as failed."""
        ...

    async def insert_clusters(
        self, run_id: int, clusters: list[PersistedCluster]
    ) -> None:
        """Persist cluster results for a run."""
        ...

    async def upsert_diagnostics(
        self, run_id: int, entries: list[DiagnosticEntry]
    ) -> None:
        """Upsert diagnostic entries for a run."""
        ...

    async def upsert_run_diagnostics(
        self,
        run_id: int,
        *,
        cluster_avg_similarity_mean: Optional[float] = None,
        cluster_avg_similarity_variance: Optional[float] = None,
        cluster_avg_similarity_p95: Optional[float] = None,
        cluster_avg_similarity_max: Optional[float] = None,
        cluster_count: int = 0,
    ) -> None:
        """Upsert run-level cluster statistics."""
        ...

    async def fetch_run(self, run_id: int) -> Optional[RunRecord]:
        """Fetch a run by its ID."""
        ...

    async def insert_system_metrics(
        self,
        metric_type: str,
        metrics: dict[str, Any],
        job_id: Optional[UUID] = None,
    ) -> None:
        """Insert system-wide metrics for dashboard monitoring."""
        ...

    async def has_running_admin_job(self, kind: str) -> bool:
        """Check if a running admin job of the given kind exists."""
        ...

    async def fetch_admin_job(self, job_id: UUID) -> Optional[AdminJobRecord]:
        """Fetch an admin job by its ID."""
        ...
