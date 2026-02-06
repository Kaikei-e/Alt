"""Composition Root: wires all dependencies for the application.

Replaces the module-level singleton pattern in deps.py with an explicit
ServiceContainer that owns the lifecycle of all shared resources.

Usage:
    container = ServiceContainer(settings)
    pipeline = container.cluster_evidence
    run_mgr = container.manage_run
"""

from __future__ import annotations

import sys
from concurrent.futures import ProcessPoolExecutor
from pathlib import Path
from typing import Any

import multiprocessing
import structlog

from ..db.session import get_session_factory
from ..gateway.hdbscan_clusterer import HdbscanClustererGateway
from ..gateway.joblib_classifier import JoblibClassifierGateway
from ..gateway.pg_repository import PgRunRepository
from ..gateway.st_embedder import StEmbedderGateway
from ..infra.config import Settings
from ..services.classification_runner import ClassificationRunner
from ..services.embedder import Embedder, EmbedderConfig
from ..services.pipeline import EvidencePipeline
from ..services.pipeline_runner import PipelineTaskRunner
from ..services.run_manager import RunManager
from ..services.extraction import ContentExtractor
from ..services.classification import CoarseClassifier
from ..services.async_jobs import AdminJobService
from ..services.learning_client import LearningClient
from ..services.learning_scheduler import LearningScheduler
from ..services.classifier import GenreClassifierService
from ..usecase.cluster_evidence import ClusterEvidenceUsecase
from ..usecase.manage_run import ManageRunUsecase

logger = structlog.get_logger(__name__)


class ServiceContainer:
    """Composition Root owning all shared service instances.

    All gateway, usecase, and infrastructure objects are created lazily
    on first access but then cached for the lifetime of the container.
    """

    def __init__(self, settings: Settings) -> None:
        self.settings = settings
        self._process_pool: ProcessPoolExecutor | None = None
        self._embedder: Embedder | None = None
        self._embedder_gateway: StEmbedderGateway | None = None
        self._clusterer_gateway: HdbscanClustererGateway | None = None
        self._pipeline: EvidencePipeline | None = None
        self._pipeline_runner: PipelineTaskRunner | None = None
        self._run_manager: RunManager | None = None
        self._cluster_evidence: ClusterEvidenceUsecase | None = None
        self._manage_run: ManageRunUsecase | None = None
        self._classifier: GenreClassifierService | None = None
        self._classification_runner: ClassificationRunner | None = None
        self._learning_client: LearningClient | None = None
        self._learning_scheduler: LearningScheduler | None = None
        self._content_extractor: ContentExtractor | None = None
        self._coarse_classifier: CoarseClassifier | None = None
        self._admin_job_service: AdminJobService | None = None

    # --- Infrastructure ---

    @property
    def process_pool(self) -> ProcessPoolExecutor:
        if self._process_pool is None:
            pool_kwargs: dict[str, int] = {"max_workers": self.settings.process_pool_size}
            if sys.version_info >= (3, 13):
                pool_kwargs["max_tasks_per_child"] = 100
            mp_context = multiprocessing.get_context("spawn")
            self._process_pool = ProcessPoolExecutor(mp_context=mp_context, **pool_kwargs)
            logger.info(
                "process pool created",
                max_workers=self.settings.process_pool_size,
            )
        return self._process_pool

    # --- Gateway layer ---

    @property
    def embedder(self) -> Embedder:
        if self._embedder is None:
            config = EmbedderConfig(
                model_id=self.settings.model_id,
                distill_model_id=self.settings.distill_model_id,
                backend=self.settings.model_backend,
                device=self.settings.device,
                batch_size=self.settings.batch_size,
                cache_size=self.settings.embed_cache_size,
                onnx_model_path=self.settings.onnx_model_path,
                onnx_tokenizer_name=self.settings.onnx_tokenizer_name,
                onnx_pooling=self.settings.onnx_pooling,
                onnx_max_length=self.settings.onnx_max_length,
                ollama_embed_url=self.settings.ollama_embed_url,
                ollama_embed_model=self.settings.ollama_embed_model,
                ollama_embed_timeout=self.settings.ollama_embed_timeout,
            )
            self._embedder = Embedder(config)
        return self._embedder

    @property
    def clusterer_gateway(self) -> HdbscanClustererGateway:
        if self._clusterer_gateway is None:
            self._clusterer_gateway = HdbscanClustererGateway(self.settings)
        return self._clusterer_gateway

    @property
    def classifier(self) -> GenreClassifierService:
        if self._classifier is None:
            model_path = Path(self.settings.genre_classifier_model_path)
            if not model_path.exists():
                raise FileNotFoundError(
                    f"Classification model not found at {model_path}. "
                    "Please ensure the model file exists."
                )
            self._classifier = GenreClassifierService(
                model_path=self.settings.genre_classifier_model_path,
                embedder=self.embedder,
            )
        return self._classifier

    @property
    def classification_runner(self) -> ClassificationRunner:
        if self._classification_runner is None:
            self._classification_runner = ClassificationRunner(self.settings)
        return self._classification_runner

    # --- Usecase layer ---

    @property
    def pipeline(self) -> EvidencePipeline:
        if self._pipeline is None:
            self._pipeline = EvidencePipeline(
                settings=self.settings,
                embedder=self.embedder,
                process_pool=self.process_pool,
            )
        return self._pipeline

    @property
    def pipeline_runner(self) -> PipelineTaskRunner | None:
        if self.settings.pipeline_mode != "processpool":
            return None
        if self._pipeline_runner is None:
            self._pipeline_runner = PipelineTaskRunner(self.settings)
        return self._pipeline_runner

    @property
    def run_manager(self) -> RunManager:
        if self._run_manager is None:
            session_factory = get_session_factory(self.settings)
            pipeline = None if self.settings.pipeline_mode == "processpool" else self.pipeline
            self._run_manager = RunManager(
                self.settings,
                session_factory,
                pipeline=pipeline,
                pipeline_runner=self.pipeline_runner,
                classifier=self.classifier,
                classification_runner=self.classification_runner,
            )
        return self._run_manager

    @property
    def cluster_evidence(self) -> ClusterEvidenceUsecase:
        if self._cluster_evidence is None:
            self._cluster_evidence = ClusterEvidenceUsecase(
                settings=self.settings,
                embedder=self.embedder,
                clusterer=self.clusterer_gateway,
                pipeline=self.pipeline,
            )
        return self._cluster_evidence

    @property
    def manage_run(self) -> ManageRunUsecase:
        if self._manage_run is None:
            self._manage_run = ManageRunUsecase(
                run_manager=self.run_manager,
                settings=self.settings,
            )
        return self._manage_run

    # --- Supporting services ---

    @property
    def learning_client(self) -> LearningClient:
        if self._learning_client is None:
            self._learning_client = LearningClient.create(
                self.settings.recap_worker_learning_url,
                self.settings.learning_request_timeout_seconds,
            )
        return self._learning_client

    @property
    def learning_scheduler(self) -> LearningScheduler | None:
        if not self.settings.learning_scheduler_enabled:
            return None
        if self._learning_scheduler is None:
            self._learning_scheduler = LearningScheduler(
                self.settings,
                interval_hours=self.settings.learning_scheduler_interval_hours,
            )
        return self._learning_scheduler

    @property
    def content_extractor(self) -> ContentExtractor:
        if self._content_extractor is None:
            self._content_extractor = ContentExtractor()
        return self._content_extractor

    @property
    def coarse_classifier(self) -> CoarseClassifier:
        if self._coarse_classifier is None:
            self._coarse_classifier = CoarseClassifier(embedder=self.embedder)
        return self._coarse_classifier

    @property
    def admin_job_service(self) -> AdminJobService:
        if self._admin_job_service is None:
            session_factory = get_session_factory(self.settings)
            self._admin_job_service = AdminJobService(
                settings=self.settings,
                session_factory=session_factory,
                learning_client=self.learning_client,
            )
        return self._admin_job_service

    # --- Lifecycle ---

    async def shutdown(self) -> None:
        """Graceful shutdown of all managed resources."""
        logger.info("shutting down ServiceContainer")

        if self._run_manager is not None:
            try:
                await self._run_manager.shutdown()
            except Exception as exc:
                logger.warning("error shutting down RunManager", error=str(exc))

        if self._pipeline_runner is not None:
            try:
                self._pipeline_runner.shutdown()
            except Exception as exc:
                logger.warning("error shutting down pipeline runner", error=str(exc))

        if self._classification_runner is not None:
            try:
                self._classification_runner.shutdown()
            except Exception as exc:
                logger.warning("error shutting down classification runner", error=str(exc))

        if self._process_pool is not None:
            try:
                self._process_pool.shutdown(wait=False)
            except Exception as exc:
                logger.warning("error shutting down process pool", error=str(exc))

        if self._embedder is not None:
            try:
                self._embedder.close()
            except Exception as exc:
                logger.warning("error closing embedder", error=str(exc))

        if self._learning_client is not None:
            try:
                await self._learning_client.close()
            except Exception as exc:
                logger.warning("error closing learning client", error=str(exc))

        if self._admin_job_service is not None:
            try:
                await self._admin_job_service.shutdown()
            except Exception as exc:
                logger.warning("error shutting down admin job service", error=str(exc))

        logger.info("ServiceContainer shutdown complete")

    # --- Testing support ---

    @classmethod
    def for_testing(
        cls,
        settings: Settings,
        *,
        embedder: Any | None = None,
        clusterer: Any | None = None,
        pipeline: Any | None = None,
        run_manager: Any | None = None,
    ) -> "ServiceContainer":
        """Factory for test instances with injectable fakes.

        Pass fake/mock objects to override any dependency:
            container = ServiceContainer.for_testing(
                settings=test_settings(),
                embedder=FakeEmbedder(),
                clusterer=FakeClusterer(),
            )
        """
        container = cls(settings)
        if embedder is not None:
            container._embedder = embedder
        if clusterer is not None:
            container._clusterer_gateway = clusterer
        if pipeline is not None:
            container._pipeline = pipeline
        if run_manager is not None:
            container._run_manager = run_manager
        return container
