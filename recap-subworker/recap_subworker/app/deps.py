"""Dependency wiring for FastAPI routes."""

from __future__ import annotations

from concurrent.futures import ProcessPoolExecutor
from typing import AsyncIterator

from fastapi import Depends

from ..db.session import get_session, get_session_factory
from ..infra.config import Settings, get_settings
from ..services.embedder import Embedder, EmbedderConfig
from ..services.genre_learning import GenreLearningService
from ..services.learning_client import LearningClient
from ..services.learning_scheduler import LearningScheduler
from ..services.pipeline import EvidencePipeline
from ..services.pipeline_runner import PipelineTaskRunner
from ..services.run_manager import RunManager
from ..services.classifier import GenreClassifierService
from ..services.extraction import ContentExtractor
from ..services.classification import CoarseClassifier
from sqlalchemy.ext.asyncio import AsyncSession

# Module-level singletons to avoid lru_cache issues with unhashable Settings
_process_pool: ProcessPoolExecutor | None = None
_embedder: Embedder | None = None
_pipeline: EvidencePipeline | None = None
_pipeline_runner: PipelineTaskRunner | None = None
_run_manager: RunManager | None = None
_learning_client: LearningClient | None = None
_learning_scheduler: LearningScheduler | None = None
_classifier: GenreClassifierService | None = None
_content_extractor: ContentExtractor | None = None
_coarse_classifier: CoarseClassifier | None = None


def _get_process_pool(settings: Settings) -> ProcessPoolExecutor:
    global _process_pool
    if _process_pool is None:
        _process_pool = ProcessPoolExecutor(max_workers=settings.process_pool_size)
    return _process_pool


def _get_embedder(settings: Settings) -> Embedder:
    global _embedder
    if _embedder is None:
        config = EmbedderConfig(
            model_id=settings.model_id,
            distill_model_id=settings.distill_model_id,
            backend=settings.model_backend,
            device=settings.device,
            batch_size=settings.batch_size,
            cache_size=settings.embed_cache_size,
        )
        _embedder = Embedder(config)
    return _embedder


def _get_pipeline(settings: Settings) -> EvidencePipeline:
    global _pipeline
    if _pipeline is None:
        _pipeline = EvidencePipeline(
            settings=settings,
            embedder=_get_embedder(settings),
            process_pool=_get_process_pool(settings),
        )
    return _pipeline


def _get_pipeline_runner(settings: Settings) -> PipelineTaskRunner | None:
    global _pipeline_runner
    if settings.pipeline_mode != "processpool":
        return None
    if _pipeline_runner is None:
        _pipeline_runner = PipelineTaskRunner(settings)
    return _pipeline_runner


def _get_run_manager(settings: Settings) -> RunManager:
    global _run_manager
    if _run_manager is None:
        session_factory = get_session_factory(settings)
        pipeline = None if settings.pipeline_mode == "processpool" else _get_pipeline(settings)
        _run_manager = RunManager(
            settings,
            session_factory,
            pipeline=pipeline,
            pipeline_runner=_get_pipeline_runner(settings),
            classifier=_get_classifier(settings),
        )
    return _run_manager


def get_settings_dep() -> Settings:
    return get_settings()


def get_learning_service(
    settings: Settings = Depends(get_settings_dep),
    session: AsyncSession = Depends(get_session),
) -> GenreLearningService:
    import structlog

    logger = structlog.get_logger(__name__)
    # Check if auto-detect is enabled or if cluster_genres is empty/"*"
    should_auto_detect = (
        settings.learning_auto_detect_genres
        or not settings.learning_cluster_genres.strip()
        or settings.learning_cluster_genres.strip() == "*"
    )
    genres = (
        []
        if should_auto_detect
        else [
            genre.strip()
            for genre in settings.learning_cluster_genres.split(",")
            if genre.strip()
        ]
    )
    logger.debug(
        "creating learning service",
        cluster_genres=genres if not should_auto_detect else "auto-detect",
        auto_detect=should_auto_detect,
        graph_margin=settings.learning_graph_margin,
    )
    service = GenreLearningService(
        session=session,
        graph_margin=settings.learning_graph_margin,
        cluster_genres=genres if genres else None,
        auto_detect_genres=should_auto_detect,
        bayes_enabled=settings.learning_bayes_enabled,
        bayes_iterations=settings.learning_bayes_iterations,
        bayes_seed=settings.learning_bayes_seed,
        bayes_min_samples=settings.learning_bayes_min_samples,
        tag_label_graph_window="7d",  # Use 7d window for learning
    )
    logger.debug(
        "learning service created",
        genres=genres if not should_auto_detect else "auto-detect",
        auto_detect=should_auto_detect,
    )
    return service


def get_learning_client(settings: Settings = Depends(get_settings_dep)) -> LearningClient:
    global _learning_client
    if _learning_client is None:
        _learning_client = LearningClient.create(
            settings.recap_worker_learning_url,
            settings.learning_request_timeout_seconds,
        )
    return _learning_client


def get_pipeline_dep(settings: Settings = Depends(get_settings_dep)) -> EvidencePipeline:
    return _get_pipeline(settings)


def get_embedder_dep(settings: Settings = Depends(get_settings_dep)) -> Embedder:
    return _get_embedder(settings)


def get_run_manager_dep(settings: Settings = Depends(get_settings_dep)) -> RunManager:
    return _get_run_manager(settings)


def get_pipeline_runner_dep(
    settings: Settings = Depends(get_settings_dep),
) -> PipelineTaskRunner | None:
    return _get_pipeline_runner(settings)


def _get_learning_scheduler(settings: Settings) -> LearningScheduler | None:
    global _learning_scheduler
    if not settings.learning_scheduler_enabled:
        return None
    if _learning_scheduler is None:
        _learning_scheduler = LearningScheduler(
            settings,
            interval_hours=settings.learning_scheduler_interval_hours,
        )
    return _learning_scheduler


def _get_classifier(settings: Settings) -> GenreClassifierService:
    global _classifier
    if _classifier is None:
        _classifier = GenreClassifierService(
            model_path=settings.genre_classifier_model_path,
            embedder=_get_embedder(settings),
        )
    return _classifier


def get_classifier_dep(settings: Settings = Depends(get_settings_dep)) -> GenreClassifierService:
    return _get_classifier(settings)


def get_content_extractor_dep() -> ContentExtractor:
    global _content_extractor
    if _content_extractor is None:
        _content_extractor = ContentExtractor()
    return _content_extractor


def get_coarse_classifier_dep(settings: Settings = Depends(get_settings_dep)) -> CoarseClassifier:
    global _coarse_classifier
    if _coarse_classifier is None:
        _coarse_classifier = CoarseClassifier(embedder=_get_embedder(settings))
    return _coarse_classifier


def register_lifecycle(app) -> None:
    """Attach startup/shutdown hooks for globally shared resources."""

    @app.on_event("startup")
    async def startup_event() -> None:  # pragma: no cover - FastAPI runtime hook
        # Note: Learning scheduler is now started in Gunicorn master process
        # (see recap_subworker/infra/gunicorn_conf.py on_starting hook)
        # Workers should not start the scheduler to avoid duplicate execution
        pass

    @app.on_event("shutdown")
    async def shutdown_event() -> None:  # pragma: no cover - FastAPI runtime hook
        # Note: Learning scheduler is stopped in Gunicorn master process
        # (see recap_subworker/infra/gunicorn_conf.py on_exit hook)
        if _process_pool is not None:
            _process_pool.shutdown(wait=False)
        if _embedder is not None:
            _embedder.close()
        if _pipeline_runner is not None:
            _pipeline_runner.shutdown()
        if _learning_client is not None:
            await _learning_client.close()
