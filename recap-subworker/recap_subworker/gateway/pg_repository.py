"""PostgreSQL repository implementing RunRepositoryPort.

Thin wrapper around SubworkerDAO that conforms to the port protocol.
The DAO retains the actual SQLAlchemy logic; this gateway adapts it
to the RunRepositoryPort interface for dependency inversion.
"""

from __future__ import annotations

from typing import Any, Optional
from uuid import UUID

from sqlalchemy.ext.asyncio import AsyncSession

from ..db.dao import (
    AdminJobRecord,
    DiagnosticEntry,
    NewRun,
    PersistedCluster,
    RunRecord,
    SubworkerDAO,
)


class PgRunRepository:
    """Gateway implementing RunRepositoryPort via SubworkerDAO."""

    def __init__(self, session: AsyncSession) -> None:
        self._dao = SubworkerDAO(session)

    async def insert_run(self, run: NewRun) -> int:
        return await self._dao.insert_run(run)

    async def find_run_by_idempotency(
        self, job_id: UUID, genre: str, idempotency_key: str
    ) -> Optional[RunRecord]:
        return await self._dao.find_run_by_idempotency(job_id, genre, idempotency_key)

    async def has_running_run(self, job_id: UUID, genre: str) -> bool:
        return await self._dao.has_running_run(job_id, genre)

    async def mark_run_success(
        self,
        run_id: int,
        cluster_count: int,
        response_payload: dict[str, Any],
        status: str = "succeeded",
    ) -> None:
        await self._dao.mark_run_success(run_id, cluster_count, response_payload, status)

    async def mark_run_failure(
        self, run_id: int, status: str, error_message: str
    ) -> None:
        await self._dao.mark_run_failure(run_id, status, error_message)

    async def insert_clusters(
        self, run_id: int, clusters: list[PersistedCluster]
    ) -> None:
        await self._dao.insert_clusters(run_id, clusters)

    async def upsert_diagnostics(
        self, run_id: int, entries: list[DiagnosticEntry]
    ) -> None:
        await self._dao.upsert_diagnostics(run_id, entries)

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
        await self._dao.upsert_run_diagnostics(
            run_id,
            cluster_avg_similarity_mean=cluster_avg_similarity_mean,
            cluster_avg_similarity_variance=cluster_avg_similarity_variance,
            cluster_avg_similarity_p95=cluster_avg_similarity_p95,
            cluster_avg_similarity_max=cluster_avg_similarity_max,
            cluster_count=cluster_count,
        )

    async def fetch_run(self, run_id: int) -> Optional[RunRecord]:
        return await self._dao.fetch_run(run_id)

    async def insert_system_metrics(
        self,
        metric_type: str,
        metrics: dict[str, Any],
        job_id: Optional[UUID] = None,
    ) -> None:
        await self._dao.insert_system_metrics(metric_type, metrics, job_id)

    async def has_running_admin_job(self, kind: str) -> bool:
        return await self._dao.has_running_admin_job(kind)

    async def fetch_admin_job(self, job_id: UUID) -> Optional[AdminJobRecord]:
        return await self._dao.fetch_admin_job(job_id)
