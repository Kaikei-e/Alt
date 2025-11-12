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


class RunManager:
    """Coordinates run creation, idempotency checks, and background execution."""

    def __init__(
        self,
        settings: Settings,
        session_factory: SessionFactory,
        dao_factory: DaoFactory = SubworkerDAO,
        pipeline: EvidencePipeline | None = None,
        pipeline_runner: PipelineTaskRunner | None = None,
    ) -> None:
        self.settings = settings
        self._session_factory = session_factory
        self._dao_factory = dao_factory
        self._tasks: set[asyncio.Task] = set()
        self._pipeline = pipeline
        self._pipeline_runner = pipeline_runner
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

                job_payload = ClusterJobPayload.model_validate(payload_dict)
                pipeline_request = self._build_pipeline_request(record, job_payload)
                response = await self._execute_pipeline(pipeline_request)

                persisted_clusters = self._persisted_clusters_from_response(response)
                await dao.insert_clusters(run_id, persisted_clusters)
                diagnostics = self._build_diagnostics_entries(response)
                await dao.upsert_diagnostics(run_id, diagnostics)

                api_response = self._build_api_response(run_id, record, response)
                status = "partial" if response.diagnostics.partial else "succeeded"
                await dao.mark_run_success(
                    run_id,
                    api_response.cluster_count,
                    api_response.model_dump(mode="json"),
                    status,
                )
                await session.commit()
        except Exception:
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
            max_sentences_per_cluster=self.settings.max_sentences_per_cluster,
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


__all__ = [
    "RunManager",
    "RunSubmission",
    "ConcurrentRunError",
    "IdempotencyMismatchError",
]
