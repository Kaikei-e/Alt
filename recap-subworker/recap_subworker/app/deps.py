"""Dependency wiring for FastAPI routes.

This module delegates to ServiceContainer for actual instance creation,
while preserving the existing FastAPI Depends() API surface so that
routers do not need changes.
"""

from __future__ import annotations

import asyncio
from typing import AsyncIterator

from fastapi import Depends
from sqlalchemy.ext.asyncio import AsyncSession

from ..db.session import get_session, get_session_factory
from ..infra.config import Settings, get_settings
from ..services.async_jobs import AdminJobService
from ..services.classification import CoarseClassifier
from ..services.classification_runner import ClassificationRunner
from ..services.classifier import GenreClassifierService
from ..services.embedder import Embedder
from ..services.extraction import ContentExtractor
from ..services.genre_learning import GenreLearningService
from ..services.learning_client import LearningClient
from ..services.learning_scheduler import LearningScheduler
from ..services.pipeline import EvidencePipeline
from ..services.pipeline_runner import PipelineTaskRunner
from ..services.run_manager import RunManager
from .container import ServiceContainer

# Module-level container singleton
_container: ServiceContainer | None = None
_extract_semaphore: asyncio.Semaphore | None = None


def _get_container() -> ServiceContainer:
    global _container
    if _container is None:
        settings = get_settings()
        _container = ServiceContainer(settings)
    return _container


def get_settings_dep() -> Settings:
    return get_settings()


def get_pipeline_dep(settings: Settings = Depends(get_settings_dep)) -> EvidencePipeline:
    return _get_container().pipeline


def get_embedder_dep(settings: Settings = Depends(get_settings_dep)) -> Embedder:
    return _get_container().embedder


def get_run_manager_dep(settings: Settings = Depends(get_settings_dep)) -> RunManager:
    return _get_container().run_manager


def get_pipeline_runner_dep(
    settings: Settings = Depends(get_settings_dep),
) -> PipelineTaskRunner | None:
    return _get_container().pipeline_runner


def get_classifier_dep(settings: Settings = Depends(get_settings_dep)) -> GenreClassifierService:
    return _get_container().classifier


def get_classification_runner_dep(settings: Settings = Depends(get_settings_dep)) -> ClassificationRunner:
    return _get_container().classification_runner


def get_learning_service(
    settings: Settings = Depends(get_settings_dep),
    session: AsyncSession = Depends(get_session),
) -> GenreLearningService:
    import structlog

    logger = structlog.get_logger(__name__)
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
    )
    return GenreLearningService(
        session=session,
        graph_margin=settings.learning_graph_margin,
        cluster_genres=genres if genres else None,
        auto_detect_genres=should_auto_detect,
        bayes_enabled=settings.learning_bayes_enabled,
        bayes_iterations=settings.learning_bayes_iterations,
        bayes_seed=settings.learning_bayes_seed,
        bayes_min_samples=settings.learning_bayes_min_samples,
        tag_label_graph_window="7d",
    )


def get_learning_client(settings: Settings = Depends(get_settings_dep)) -> LearningClient:
    return _get_container().learning_client


def get_admin_job_service_dep(
    settings: Settings = Depends(get_settings_dep),
) -> AdminJobService:
    return _get_container().admin_job_service


def get_extract_semaphore_dep(
    settings: Settings = Depends(get_settings_dep),
) -> asyncio.Semaphore:
    global _extract_semaphore
    if _extract_semaphore is None:
        _extract_semaphore = asyncio.Semaphore(settings.extract_concurrency_max)
    return _extract_semaphore


def get_content_extractor_dep() -> ContentExtractor:
    return _get_container().content_extractor


def get_coarse_classifier_dep(settings: Settings = Depends(get_settings_dep)) -> CoarseClassifier:
    return _get_container().coarse_classifier


def register_lifecycle(app) -> None:
    """Attach startup/shutdown hooks for globally shared resources."""

    @app.on_event("startup")
    async def startup_event() -> None:  # pragma: no cover
        pass

    @app.on_event("shutdown")
    async def shutdown_event() -> None:  # pragma: no cover
        container = _get_container()
        await container.shutdown()
