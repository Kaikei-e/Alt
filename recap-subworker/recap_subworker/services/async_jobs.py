"""Async admin job execution for graph build and genre learning."""

from __future__ import annotations

import asyncio
import time
from dataclasses import asdict
from datetime import datetime, timezone
from typing import Any, Callable, Coroutine
from uuid import UUID, uuid4

import structlog

from ..db.dao import AdminJobRecord, SubworkerDAO
from ..infra.config import Settings
from ..infra.telemetry import ADMIN_JOB_DURATION_SECONDS, ADMIN_JOB_STATUS_TOTAL
from ..services.genre_learning import GenreLearningService
from ..services.learning_client import LearningClient
from ..services.tag_label_graph_builder import TagLabelGraphBuilder

LOGGER = structlog.get_logger(__name__)


class ConcurrentAdminJobError(Exception):
    """Raised when a job of the same kind is already running."""


def _utcnow() -> datetime:
    return datetime.now(timezone.utc)


class AdminJobService:
    """Manages admin jobs for graph rebuild and genre learning."""

    def __init__(
        self,
        settings: Settings,
        session_factory,
        learning_client: LearningClient,
    ) -> None:
        self._settings = settings
        self._session_factory = session_factory
        self._learning_client = learning_client
        self._tasks: set[asyncio.Task] = set()
        # Limit concurrent admin jobs (graph / learning)
        self._semaphore = asyncio.Semaphore(settings.graph_build_max_concurrency)

    async def enqueue_graph_job(self) -> UUID:
        """Schedule a graph-build job and return its ID."""
        job_id = uuid4()
        async with self._session_factory() as session:
            dao = SubworkerDAO(session)
            if await dao.has_running_admin_job("graph"):
                await session.rollback()
                raise ConcurrentAdminJobError("graph job already running")
            await dao.insert_admin_job(
                job_id=job_id,
                kind="graph",
                status="running",
                payload=None,
                started_at=_utcnow(),
            )
            await session.commit()

        self._schedule(self._execute_graph_job, job_id)
        return job_id

    async def enqueue_learning_job(self) -> UUID:
        """Schedule a genre learning job and return its ID."""
        job_id = uuid4()
        async with self._session_factory() as session:
            dao = SubworkerDAO(session)
            if await dao.has_running_admin_job("learning"):
                await session.rollback()
                raise ConcurrentAdminJobError("learning job already running")
            await dao.insert_admin_job(
                job_id=job_id,
                kind="learning",
                status="running",
                payload=None,
                started_at=_utcnow(),
            )
            await session.commit()

        self._schedule(self._execute_learning_job, job_id)
        return job_id

    async def get_job(self, job_id: UUID) -> AdminJobRecord | None:
        async with self._session_factory() as session:
            dao = SubworkerDAO(session)
            record = await dao.fetch_admin_job(job_id)
            await session.rollback()
            return record

    def _schedule(
        self,
        fn: Callable[[UUID], Coroutine[Any, Any, None]],
        job_id: UUID,
    ) -> None:
        loop = asyncio.get_running_loop()
        task = loop.create_task(self._guarded(fn, job_id))
        self._tasks.add(task)
        task.add_done_callback(self._tasks.discard)

    async def _guarded(
        self,
        fn: Callable[[UUID], Coroutine[Any, Any, None]],
        job_id: UUID,
    ) -> None:
        try:
            await fn(job_id)
        except Exception:
            LOGGER.exception("admin job execution failed", job_id=str(job_id))

    async def _execute_graph_job(self, job_id: UUID) -> None:
        LOGGER.info("graph job execution started", job_id=str(job_id))
        started = time.monotonic()
        async with self._semaphore:
            try:
                result = await self._run_graph_build()
                await self._mark_success(job_id, "graph", result)
                duration = time.monotonic() - started
                ADMIN_JOB_STATUS_TOTAL.labels(kind="graph", status="succeeded").inc()
                ADMIN_JOB_DURATION_SECONDS.labels(kind="graph").observe(duration)
                LOGGER.info(
                    "graph job completed",
                    job_id=str(job_id),
                    duration_seconds=duration,
                    result=result,
                )
            except Exception as exc:
                await self._mark_failure(job_id, "graph", exc, started)

    async def _execute_learning_job(self, job_id: UUID) -> None:
        started = time.monotonic()
        async with self._semaphore:
            try:
                result = await self._run_learning()
                await self._mark_success(job_id, "learning", result)
                duration = time.monotonic() - started
                ADMIN_JOB_STATUS_TOTAL.labels(kind="learning", status="succeeded").inc()
                ADMIN_JOB_DURATION_SECONDS.labels(kind="learning").observe(duration)
                LOGGER.info(
                    "learning job completed",
                    job_id=str(job_id),
                    duration_seconds=duration,
                    recap_worker_status=result.get("recap_worker_status"),
                )
            except Exception as exc:
                await self._mark_failure(job_id, "learning", exc, started)

    async def _run_graph_build(self) -> dict[str, Any]:
        async with self._session_factory() as session:
            builder = TagLabelGraphBuilder(
                session=session,
                max_tags=self._settings.graph_build_max_tags,
                min_confidence=self._settings.graph_build_min_confidence,
                min_support=self._settings.graph_build_min_support,
            )
            windows = [
                int(w.strip())
                for w in self._settings.graph_build_windows.split(",")
                if w.strip()
            ]
            results: dict[str, int] = {}
            for window_days in windows:
                edge_count = await builder.build_graph(window_days)
                results[f"{window_days}d"] = edge_count
            await session.commit()
            return {
                "edge_counts": results,
                "total_edges": sum(results.values()),
            }

    async def _run_learning(self) -> dict[str, Any]:
        graph_result = {}
        if self._settings.graph_build_enabled:
            try:
                graph_result = await self._run_graph_build()
            except Exception:
                LOGGER.exception("failed to rebuild tag_label_graph before learning")
                # Continue learning even if graph rebuild fails

        async with self._session_factory() as session:
            learning_service = self._build_learning_service(session)
            learning_result = await learning_service.generate_learning_result(
                days=self._settings.learning_snapshot_days
            )
            payload = self._build_learning_payload(learning_result)
            response = await self._learning_client.send_learning_payload(payload)
            result: dict[str, Any] = {
                "recap_worker_status": response.status_code,
                "entries_observed": learning_result.summary.total_records,
            }
            if graph_result:
                result["graph"] = graph_result
            # Attach response body if JSON
            try:
                if response.headers.get("content-type", "").startswith("application/json"):
                    result["recap_worker_response"] = response.json()
            except Exception:
                # ignore parse errors; status_code is sufficient
                pass
            await session.commit()
            return result

    def _build_learning_service(self, session) -> GenreLearningService:
        should_auto_detect = (
            self._settings.learning_auto_detect_genres
            or not self._settings.learning_cluster_genres.strip()
            or self._settings.learning_cluster_genres.strip() == "*"
        )
        genres = (
            []
            if should_auto_detect
            else [
                genre.strip()
                for genre in self._settings.learning_cluster_genres.split(",")
                if genre.strip()
            ]
        )
        return GenreLearningService(
            session=session,
            graph_margin=self._settings.learning_graph_margin,
            cluster_genres=genres if genres else None,
            auto_detect_genres=should_auto_detect,
            bayes_enabled=self._settings.learning_bayes_enabled,
            bayes_iterations=self._settings.learning_bayes_iterations,
            bayes_seed=self._settings.learning_bayes_seed,
            bayes_min_samples=self._settings.learning_bayes_min_samples,
            tag_label_graph_window="7d",
        )

    def _build_learning_payload(self, result) -> dict[str, Any]:
        summary = asdict(result.summary)
        graph_override: dict[str, Any] = {
            "graph_margin": result.summary.graph_margin_reference,
        }
        if result.summary.boost_threshold_reference is not None:
            graph_override["boost_threshold"] = result.summary.boost_threshold_reference
        if result.summary.tag_count_threshold_reference is not None:
            graph_override["tag_count_threshold"] = (
                result.summary.tag_count_threshold_reference
            )

        metadata: dict[str, Any] = {
            "captured_at": _utcnow().isoformat(),
            "entries_observed": result.summary.total_records,
        }
        if result.summary.accuracy_estimate is not None:
            metadata["accuracy_estimate"] = result.summary.accuracy_estimate
        if result.summary.test_accuracy is not None:
            metadata["test_accuracy"] = result.summary.test_accuracy

        payload: dict[str, Any] = {
            "summary": summary,
            "graph_override": graph_override,
            "metadata": metadata,
        }
        if result.cluster_draft:
            payload["cluster_draft"] = result.cluster_draft
        return payload

    async def _mark_success(self, job_id: UUID, kind: str, result: dict[str, Any]) -> None:
        async with self._session_factory() as session:
            dao = SubworkerDAO(session)
            await dao.update_admin_job_status(
                job_id=job_id,
                status="succeeded",
                finished_at=_utcnow(),
                result=result,
                error=None,
            )
            await session.commit()

    async def _mark_failure(
        self,
        job_id: UUID,
        kind: str,
        exc: Exception,
        started: float,
    ) -> None:
        duration = time.monotonic() - started
        ADMIN_JOB_STATUS_TOTAL.labels(kind=kind, status="failed").inc()
        ADMIN_JOB_DURATION_SECONDS.labels(kind=kind).observe(duration)
        async with self._session_factory() as session:
            dao = SubworkerDAO(session)
            await dao.update_admin_job_status(
                job_id=job_id,
                status="failed",
                finished_at=_utcnow(),
                error=str(exc),
                result=None,
            )
            await session.commit()
        LOGGER.error(
            "admin job failed",
            job_id=str(job_id),
            kind=kind,
            error=str(exc),
            error_type=type(exc).__name__,
        )

    async def shutdown(self) -> None:
        """Cancel background tasks (best-effort)."""
        if not self._tasks:
            return
        for task in list(self._tasks):
            if not task.done():
                task.cancel()
        try:
            await asyncio.wait_for(
                asyncio.gather(*self._tasks, return_exceptions=True),
                timeout=10.0,
            )
        except asyncio.TimeoutError:
            LOGGER.warning("timeout while shutting down admin job tasks")
        self._tasks.clear()


__all__ = ["AdminJobService", "ConcurrentAdminJobError"]

