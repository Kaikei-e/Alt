"""Run orchestration service for recap-subworker."""

from __future__ import annotations

import asyncio
import hashlib
from dataclasses import dataclass
from typing import Any, Callable, Optional
from uuid import UUID

import orjson
import structlog

from ..db.dao import (
    DiagnosticEntry,
    NewRun,
    PersistedCluster,
    PersistedEvidence,
    PersistedSentence,
    RunRecord,
    SubworkerDAO,
)
from ..domain.models import (
    ClassificationJobPayload,
    ClassificationJobResponse,
    ClassificationResult,
    ClusterInfo,
    ClusterJobPayload,
    ClusterJobResponse,
    ClusterSentencePayload,
    EvidenceCluster,
    EvidenceConstraints,
    EvidenceRequest,
    EvidenceResponse,
)
from ..infra.config import Settings
from .pipeline_runner import PipelineTaskRunner
from .pipeline import EvidencePipeline


LOGGER = structlog.get_logger(__name__)


class ConcurrentRunError(Exception):
    """Raised when a job+genre pair already has a running run."""


class IdempotencyMismatchError(Exception):
    """Raised when an idempotency key was reused with different payload."""


DaoFactory = Callable[[Any], SubworkerDAO]
SessionFactory = Callable[[], Any]


def _hash_payload(payload: dict[str, Any]) -> str:
    serialized = orjson.dumps(payload, option=orjson.OPT_SORT_KEYS)
    return hashlib.sha256(serialized).hexdigest()


@dataclass(slots=True)
class RunSubmission:
    job_id: UUID
    genre: str
    payload: ClusterJobPayload
    idempotency_key: Optional[str]


@dataclass(slots=True)
class ClassificationRunSubmission:
    job_id: UUID
    payload: ClassificationJobPayload
    idempotency_key: Optional[str]


class RunManager:
    """Coordinates run creation, idempotency checks, and background execution."""

    def __init__(
        self,
        settings: Settings,
        session_factory: SessionFactory,
        dao_factory: DaoFactory = SubworkerDAO,
        pipeline: EvidencePipeline | None = None,
        pipeline_runner: PipelineTaskRunner | None = None,
        classifier: Any | None = None,  # GenreClassifierService
    ) -> None:
        self.settings = settings
        self._session_factory = session_factory
        self._dao_factory = dao_factory
        self._tasks: set[asyncio.Task] = set()
        self._pipeline = pipeline
        self._pipeline_runner = pipeline_runner
        self._classifier = classifier
        self._background_slots = asyncio.Semaphore(settings.max_background_runs)
        self._run_timeout = settings.run_execution_timeout_seconds
        self._queue_warning_threshold = settings.queue_warning_threshold

    async def create_run(self, submission: RunSubmission) -> RunRecord:
        """Insert a new run or reuse an existing idempotent run."""

        payload_dict = submission.payload.model_dump(mode="json")
        request_hash = _hash_payload(payload_dict)
        request_envelope = {
            "payload": payload_dict,
            "idempotency_key": submission.idempotency_key,
            "request_hash": request_hash,
        }

        async with self._session_factory() as session:
            dao = self._dao_factory(session)
            if submission.idempotency_key:
                existing = await dao.find_run_by_idempotency(
                    submission.job_id, submission.genre, submission.idempotency_key
                )
                if existing:
                    stored_hash = existing.request_payload.get("request_hash")
                    if stored_hash and stored_hash != request_hash:
                        await session.rollback()
                        raise IdempotencyMismatchError(
                            "request payload differs for idempotency key"
                        )
                    await session.rollback()
                    return existing

            if await dao.has_running_run(submission.job_id, submission.genre):
                await session.rollback()
                raise ConcurrentRunError("run already in progress for job/genre")

            run = NewRun(
                job_id=submission.job_id,
                genre=submission.genre,
                status="running",
                request_payload=request_envelope,
            )
            run_id = await dao.insert_run(run)
            await session.commit()

        record = RunRecord(
            run_id=run_id,
            job_id=submission.job_id,
            genre=submission.genre,
            status="running",
            cluster_count=0,
            request_payload=request_envelope,
            response_payload=None,
            error_message=None,
        )

        self._schedule_background(record)
        return record

    def _schedule_background(self, record: RunRecord) -> None:
        loop = asyncio.get_running_loop()
        queue_depth = len(self._tasks) + 1
        if queue_depth >= self._queue_warning_threshold:
            LOGGER.warning("run.queue.backlog", queue_depth=queue_depth)
        task = loop.create_task(self._guarded_process_run(record.run_id))
        self._tasks.add(task)
        task.add_done_callback(self._tasks.discard)

    async def _guarded_process_run(self, run_id: int) -> None:
        try:
            await self._process_run(run_id)
        except Exception:  # pragma: no cover - logged upstream
            LOGGER.exception("run.process.failed", run_id=run_id)

    async def _process_run(self, run_id: int) -> None:
        if self._pipeline is None and self._pipeline_runner is None:
            LOGGER.warning("pipeline unavailable; skipping run", run_id=run_id)
            return

        async with self._background_slots:
            LOGGER.info("run.process.started", run_id=run_id)
            try:
                await asyncio.wait_for(self._process_run_inner(run_id), timeout=self._run_timeout)
            except asyncio.TimeoutError:
                await self._handle_failure(run_id, f"pipeline timed out after {self._run_timeout}s")
                LOGGER.error("run.process.timeout", run_id=run_id, timeout_s=self._run_timeout)
            except Exception as exc:
                await self._handle_failure(run_id, str(exc))
                raise

    async def _process_run_inner(self, run_id: int) -> None:
        if self._pipeline is None and self._pipeline_runner is None:
            LOGGER.warning("pipeline unavailable; skipping run", run_id=run_id)
            return

        # Session 1: Fetch record and validate payload (close session before long-running operation)
        record = None
        pipeline_request = None
        try:
            async with self._session_factory() as session:
                dao = self._dao_factory(session)
                record = await dao.fetch_run(run_id)
                if not record:
                    await session.rollback()
                    return
                payload_container = record.request_payload or {}
                payload_dict = payload_container.get("payload")
                if payload_dict is None:
                    await dao.mark_run_failure(run_id, "failed", "request payload missing")
                    await session.commit()
                    return
                # Validate payload before closing session
                job_payload = ClusterJobPayload.model_validate(payload_dict)
                pipeline_request = self._build_pipeline_request(record, job_payload)
        except Exception as exc:
            error_str = str(exc)
            error_type = type(exc).__name__

            # Check if this is a connection closed error during rollback
            if "connection is closed" in error_str or "underlying connection is closed" in error_str:
                LOGGER.warning(
                    "run.process.connection_closed",
                    run_id=run_id,
                    error=error_str,
                    error_type=error_type,
                    message="Connection was closed during record fetch",
                )
                return
            raise

        # Long-running operation: Run pipeline (embedding generation and clustering, no database connection needed)
        try:
            LOGGER.info("run.pipeline.executing", run_id=run_id, stage="clustering")
            response = await self._execute_pipeline(pipeline_request)
            LOGGER.info("run.pipeline.completed", run_id=run_id, stage="clustering")
        except Exception as exc:
            # If pipeline fails, mark as failed in a new session
            try:
                async with self._session_factory() as session:
                    dao = self._dao_factory(session)
                    await dao.mark_run_failure(run_id, "failed", str(exc))
                    await session.commit()
            except Exception as db_exc:
                LOGGER.error(
                    "run.process.failure_mark_failed",
                    run_id=run_id,
                    pipeline_error=str(exc),
                    db_error=str(db_exc),
                    message="Failed to mark run as failed after pipeline error",
                )
            raise

        # Session 2: Save results (open new session after long-running operation)
        # record is guaranteed to be non-None at this point (checked in Session 1)
        assert record is not None, "record should not be None after Session 1"
        try:
            async with self._session_factory() as session:
                dao = self._dao_factory(session)
                persisted_clusters = self._persisted_clusters_from_response(response)
                await dao.insert_clusters(run_id, persisted_clusters)
                diagnostics = self._build_diagnostics_entries(response)
                await dao.upsert_diagnostics(run_id, diagnostics)

                # Log system metrics for dashboard
                clustering_metrics = {
                    "dbcv_score": response.diagnostics.dbcv_score,
                    "silhouette_score": response.diagnostics.silhouette_score,
                    "noise_ratio": response.diagnostics.noise_ratio,
                    "num_clusters": len(response.clusters),
                    "cluster_sizes": [c.size for c in response.clusters],
                    "processing_time_ms": response.diagnostics.hdbscan_ms,  # Approximate
                }
                if hasattr(dao, "insert_system_metrics"):
                    await dao.insert_system_metrics(
                        metric_type="clustering",
                        metrics=clustering_metrics,
                        job_id=record.job_id,
                    )

                api_response = self._build_api_response(run_id, record, response)
                status = "partial" if response.diagnostics.partial else "succeeded"
                await dao.mark_run_success(
                    run_id,
                    api_response.cluster_count,
                    api_response.model_dump(mode="json"),
                    status,
                )
                await session.commit()
                LOGGER.info(
                    "run.process.completed",
                    run_id=run_id,
                    cluster_count=api_response.cluster_count,
                    status=status,
                )
        except Exception as exc:
            error_str = str(exc)
            error_type = type(exc).__name__

            # Check if this is a connection closed error during rollback
            if "connection is closed" in error_str or "underlying connection is closed" in error_str:
                LOGGER.warning(
                    "run.process.connection_closed",
                    run_id=run_id,
                    error=error_str,
                    error_type=error_type,
                    message="Connection was closed during result save",
                )
                # Don't re-raise connection closed errors during cleanup
                # The failure will be handled by _process_run
                return
            # For other exceptions, re-raise to be handled by the caller
            raise

    async def create_classification_run(
        self, submission: ClassificationRunSubmission
    ) -> RunRecord:
        """Insert a new classification run or reuse an existing idempotent run."""

        payload_dict = submission.payload.model_dump(mode="json")
        request_hash = _hash_payload(payload_dict)
        request_envelope = {
            "payload": payload_dict,
            "idempotency_key": submission.idempotency_key,
            "request_hash": request_hash,
            "type": "classification",  # Mark as classification run
        }

        async with self._session_factory() as session:
            dao = self._dao_factory(session)
            if submission.idempotency_key:
                existing = await dao.find_run_by_idempotency(
                    submission.job_id, "classification", submission.idempotency_key
                )
                if existing:
                    if existing.request_payload.get("request_hash") != request_hash:
                        raise IdempotencyMismatchError(
                            f"idempotency key reused with different payload"
                        )
                    LOGGER.info(
                        "classification.run.idempotent",
                        run_id=existing.run_id,
                        job_id=str(submission.job_id),
                    )
                    await session.rollback()
                    return existing

            if await dao.has_running_run(submission.job_id, "classification"):
                raise ConcurrentRunError(
                    f"classification run already in progress for job {submission.job_id}"
                )

            new_run = NewRun(
                job_id=submission.job_id,
                genre="classification",  # Use fixed genre for classification
                status="running",
                request_payload=request_envelope,
            )
            run_id = await dao.insert_run(new_run)
            await session.commit()
            LOGGER.info(
                "classification.run.created",
                run_id=run_id,
                job_id=str(submission.job_id),
                text_count=len(submission.payload.texts),
            )

        # Try to fetch the record, but if it fails, construct it from the inserted data
        try:
            record = await self.get_run(run_id)
            if record is None:
                # Fallback: construct record from inserted data
                LOGGER.warning(
                    "classification.run.get_failed_fallback",
                    run_id=run_id,
                    job_id=str(submission.job_id),
                    message="Failed to fetch run after insert, constructing from inserted data",
                )
                record = RunRecord(
                    run_id=run_id,
                    job_id=submission.job_id,
                    genre="classification",
                    status="running",
                    cluster_count=0,
                    request_payload=request_envelope,
                    response_payload=None,
                    error_message=None,
                )
        except Exception as exc:
            # If get_run fails (e.g., database connection error), construct record from inserted data
            LOGGER.warning(
                "classification.run.get_failed_fallback",
                run_id=run_id,
                job_id=str(submission.job_id),
                error_type=type(exc).__name__,
                error=str(exc),
                message="Failed to fetch run after insert, constructing from inserted data",
            )
            record = RunRecord(
                run_id=run_id,
                job_id=submission.job_id,
                genre="classification",
                status="running",
                cluster_count=0,
                request_payload=request_envelope,
                response_payload=None,
                error_message=None,
            )

        task = asyncio.create_task(self._guarded_process_classification_run(run_id))
        self._tasks.add(task)
        task.add_done_callback(self._tasks.discard)
        return record

    async def _guarded_process_classification_run(self, run_id: int) -> None:
        try:
            await self._process_classification_run(run_id)
        except Exception:  # pragma: no cover - logged upstream
            LOGGER.exception("classification.run.process.failed", run_id=run_id)

    async def _process_classification_run(self, run_id: int) -> None:
        if self._classifier is None:
            LOGGER.warning("classifier unavailable; skipping run", run_id=run_id)
            return

        async with self._background_slots:
            LOGGER.info("classification.run.process.started", run_id=run_id)
            try:
                await asyncio.wait_for(
                    self._process_classification_run_inner(run_id), timeout=self._run_timeout
                )
            except asyncio.TimeoutError:
                await self._handle_failure(
                    run_id, f"classification timed out after {self._run_timeout}s"
                )
                LOGGER.error(
                    "classification.run.process.timeout",
                    run_id=run_id,
                    timeout_s=self._run_timeout,
                )
            except Exception as exc:
                await self._handle_failure(run_id, str(exc))
                raise

    async def _process_classification_run_inner(self, run_id: int) -> None:
        if self._classifier is None:
            LOGGER.warning("classifier unavailable; skipping run", run_id=run_id)
            return

        # Session 1: Fetch record and validate payload (close session before long-running operation)
        try:
            async with self._session_factory() as session:
                dao = self._dao_factory(session)
                record = await dao.fetch_run(run_id)
                if not record:
                    await session.rollback()
                    return
                payload_container = record.request_payload or {}
                payload_dict = payload_container.get("payload")
                if payload_dict is None:
                    await dao.mark_run_failure(run_id, "failed", "request payload missing")
                    await session.commit()
                    return
                # Validate payload before closing session
                classification_payload = ClassificationJobPayload.model_validate(payload_dict)
        except Exception as exc:
            error_str = str(exc)
            error_type = type(exc).__name__

            # Check if this is a connection closed error during rollback
            if "connection is closed" in error_str or "underlying connection is closed" in error_str:
                LOGGER.warning(
                    "classification.run.connection_closed",
                    run_id=run_id,
                    error=error_str,
                    error_type=error_type,
                    message="Connection was closed during record fetch",
                )
                return
            raise

        # Long-running operation: Run classification (no database connection needed)
        try:
            LOGGER.info(
                "classification.run.executing",
                run_id=run_id,
                text_count=len(classification_payload.texts)
            )
            loop = asyncio.get_running_loop()
            results = await loop.run_in_executor(
                None, self._classifier.predict_batch, classification_payload.texts
            )
            LOGGER.info("classification.run.completed_inference", run_id=run_id)

            # Convert results to domain models
            classification_results = [
                ClassificationResult(
                    top_genre=r["top_genre"],
                    confidence=r["confidence"],
                    scores=r["scores"],
                )
                for r in results
            ]

            response_payload = {
                "results": [r.model_dump() for r in classification_results],
            }
        except Exception as exc:
            # If classification fails, mark as failed in a new session
            try:
                async with self._session_factory() as session:
                    dao = self._dao_factory(session)
                    await dao.mark_run_failure(run_id, "failed", str(exc))
                    await session.commit()
            except Exception as db_exc:
                LOGGER.error(
                    "classification.run.failure_mark_failed",
                    run_id=run_id,
                    classification_error=str(exc),
                    db_error=str(db_exc),
                    message="Failed to mark run as failed after classification error",
                )
            raise

        # Session 2: Save results (open new session after long-running operation)
        try:
            async with self._session_factory() as session:
                dao = self._dao_factory(session)
                status = "succeeded"
                await dao.mark_run_success(
                    run_id,
                    len(classification_results),  # Use result_count instead of cluster_count
                    response_payload,
                    status,
                )
                await session.commit()
                LOGGER.info(
                    "classification.run.completed",
                    run_id=run_id,
                    result_count=len(classification_results),
                )
        except Exception as exc:
            error_str = str(exc)
            error_type = type(exc).__name__

            # Check if this is a connection closed error during rollback
            if "connection is closed" in error_str or "underlying connection is closed" in error_str:
                LOGGER.warning(
                    "classification.run.connection_closed",
                    run_id=run_id,
                    error=error_str,
                    error_type=error_type,
                    message="Connection was closed during result save",
                )
                # Don't re-raise connection closed errors during cleanup
                # The failure will be handled by _process_classification_run
                return
            # For other exceptions, re-raise to be handled by the caller
            raise

    async def get_run(self, run_id: int) -> Optional[RunRecord]:
        async with self._session_factory() as session:
            dao = self._dao_factory(session)
            record = await dao.fetch_run(run_id)
            await session.rollback()
            return record

    async def _execute_pipeline(self, request: EvidenceRequest) -> EvidenceResponse:
        if self._pipeline_runner is not None:
            return await self._pipeline_runner.run(request)
        loop = asyncio.get_running_loop()
        assert self._pipeline is not None
        return await loop.run_in_executor(None, self._pipeline.run, request)

    def _build_pipeline_request(
        self, record: RunRecord, payload: ClusterJobPayload
    ) -> EvidenceRequest:
        constraints = EvidenceConstraints(
            max_sentences_per_cluster=payload.params.max_sentences_per_cluster,
            max_total_sentences=min(
                payload.params.max_sentences_total, self.settings.max_total_sentences
            ),
            max_tokens_budget=self.settings.max_tokens_budget,
            dedup_threshold=self.settings.dedup_threshold,
            mmr_lambda=payload.params.mmr_lambda,
            hdbscan_min_cluster_size=payload.params.hdbscan_min_cluster_size
            or self.settings.default_hdbscan_min_cluster_size,
            hdbscan_min_samples=self.settings.default_hdbscan_min_samples,
            umap_n_components=payload.params.umap_n_components
            or self.settings.default_umap_n_components,
        )
        return EvidenceRequest(
            job_id=str(record.job_id),
            genre=record.genre,
            documents=payload.documents,
            constraints=constraints,
            metadata=payload.metadata,
        )

    def _persisted_clusters_from_response(
        self, response: EvidenceResponse
    ) -> list[PersistedCluster]:
        persisted: list[PersistedCluster] = []
        for cluster in response.clusters:
            sentences: list[PersistedSentence] = []
            evidence_rows: list[PersistedEvidence] = []
            for idx, sentence in enumerate(cluster.representatives):
                sentences.append(
                    PersistedSentence(
                        article_id=sentence.source.source_id,
                        paragraph_idx=sentence.source.paragraph_idx,
                        sentence_id=idx,
                        sentence_text=sentence.text,
                        lang=sentence.lang or "unknown",
                        score=max(0.0, 1.0 - idx * 0.05),
                    )
                )
                evidence_rows.append(
                    PersistedEvidence(
                        article_id=sentence.source.source_id,
                        title=None,
                        source_url=sentence.source.url,
                        published_at=None,
                        lang=sentence.lang,
                        rank=idx,
                    )
                )
            stats: dict[str, Any] = {}
            if cluster.stats.avg_sim is not None:
                stats["avg_sim"] = cluster.stats.avg_sim
            if cluster.stats.token_count is not None:
                stats["token_count"] = cluster.stats.token_count
            persisted.append(
                PersistedCluster(
                    cluster_id=cluster.cluster_id,
                    size=cluster.size,
                    label=cluster.label.top_terms[0] if cluster.label.top_terms else None,
                    top_terms=cluster.label.top_terms,
                    stats=stats,
                    sentences=sentences,
                    evidence=evidence_rows,
                )
            )
        return persisted

    def _build_api_response(
        self,
        run_id: int,
        record: RunRecord,
        response: EvidenceResponse,
    ) -> ClusterJobResponse:
        clusters = [self._convert_cluster_to_api(cluster) for cluster in response.clusters]
        diagnostics = self._serialize_diagnostics(response)
        status = "partial" if response.diagnostics.partial else "succeeded"
        return ClusterJobResponse(
            run_id=run_id,
            job_id=str(record.job_id),
            genre=record.genre,
            status=status,
            cluster_count=len(clusters),
            clusters=clusters,
            diagnostics=diagnostics,
        )

    def _convert_cluster_to_api(self, cluster: EvidenceCluster) -> ClusterInfo:
        stats: dict[str, Any] = {}
        if cluster.stats.avg_sim is not None:
            stats["avg_sim"] = cluster.stats.avg_sim
        if cluster.stats.token_count is not None:
            stats["token_count"] = cluster.stats.token_count

        representatives = []
        for idx, sentence in enumerate(cluster.representatives):
            representatives.append(
                ClusterSentencePayload(
                    article_id=sentence.source.source_id,
                    paragraph_idx=sentence.source.paragraph_idx,
                    sentence_text=sentence.text,
                    lang=sentence.lang,
                    score=max(0.0, 1.0 - idx * 0.05),
                )
            )

        return ClusterInfo(
            cluster_id=cluster.cluster_id,
            size=cluster.size,
            label=cluster.label.top_terms[0] if cluster.label.top_terms else None,
            top_terms=cluster.label.top_terms,
            stats=stats,
            representatives=representatives,
        )

    def _serialize_diagnostics(self, response: EvidenceResponse) -> dict[str, Any]:
        diag = response.diagnostics
        payload: dict[str, Any] = {
            "dedup_pairs": diag.dedup_pairs,
            "umap_used": diag.umap_used,
            "partial": diag.partial,
            "total_sentences": diag.total_sentences,
        }
        if diag.embedding_ms is not None:
            payload["embedding_ms"] = diag.embedding_ms
        if diag.hdbscan_ms is not None:
            payload["hdbscan_ms"] = diag.hdbscan_ms
        if diag.noise_ratio is not None:
            payload["noise_ratio"] = diag.noise_ratio
        if diag.hdbscan is not None:
            payload["hdbscan"] = diag.hdbscan.model_dump()
        if diag.dbcv_score is not None:
            payload["dbcv_score"] = diag.dbcv_score
        if diag.silhouette_score is not None:
            payload["silhouette_score"] = diag.silhouette_score
        return payload

    def _build_diagnostics_entries(self, response: EvidenceResponse) -> list[DiagnosticEntry]:
        diag = response.diagnostics
        entries: list[DiagnosticEntry] = []
        mapping = {
            "dedup_pairs": diag.dedup_pairs,
            "umap_used": diag.umap_used,
            "partial": diag.partial,
            "total_sentences": diag.total_sentences,
            "embedding_ms": diag.embedding_ms,
            "hdbscan_ms": diag.hdbscan_ms,
            "noise_ratio": diag.noise_ratio,
            "dbcv_score": diag.dbcv_score,
            "silhouette_score": diag.silhouette_score,
        }
        for key, value in mapping.items():
            if value is None:
                continue
            entries.append(DiagnosticEntry(metric=key, value=value))
        if diag.hdbscan is not None:
            entries.append(
                DiagnosticEntry(
                    metric="hdbscan_min_cluster_size", value=diag.hdbscan.min_cluster_size
                )
            )
            entries.append(
                DiagnosticEntry(metric="hdbscan_min_samples", value=diag.hdbscan.min_samples)
            )
        return entries

    async def _handle_failure(self, run_id: int, message: str) -> None:
        async with self._session_factory() as session:
            dao = self._dao_factory(session)
            await dao.mark_run_failure(run_id, "failed", message)
            await session.commit()

    async def shutdown(self) -> None:
        """Cancel all pending tasks and wait for them to complete.

        This prevents memory leaks from orphaned asyncio tasks.
        """
        if not self._tasks:
            return

        LOGGER.info("shutting down RunManager", pending_tasks=len(self._tasks))

        # Cancel all pending tasks
        for task in list(self._tasks):
            if not task.done():
                task.cancel()

        # Wait for tasks to complete (with timeout to prevent hanging)
        if self._tasks:
            try:
                await asyncio.wait_for(
                    asyncio.gather(*self._tasks, return_exceptions=True),
                    timeout=30.0,
                )
            except asyncio.TimeoutError:
                LOGGER.warning(
                    "some tasks did not complete within timeout during shutdown",
                    pending_count=len([t for t in self._tasks if not t.done()]),
                )

        self._tasks.clear()
        LOGGER.info("RunManager shutdown complete")


__all__ = [
    "RunManager",
    "RunSubmission",
    "ConcurrentRunError",
    "IdempotencyMismatchError",
]
