"""Dependency wiring for FastAPI routes.

Phase 1: the ServiceContainer is owned by the app's lifespan and stored on
``request.app.state.container``. All dependencies route through
``get_container`` — no module-level singletons, no hidden caches.
"""

from __future__ import annotations

import asyncio
from collections.abc import AsyncIterator

from fastapi import Depends, Request
from sqlalchemy.ext.asyncio import AsyncSession

from ..infra.config import Settings, get_settings
from ..services.async_jobs import AdminJobService
from ..services.classification import CoarseClassifier
from ..services.classification_runner import ClassificationRunner
from ..services.classifier import GenreClassifierService
from ..services.embedder import Embedder
from ..services.extraction import ContentExtractor
from ..services.genre_learning import GenreLearningService
from ..services.learning_client import LearningClient
from ..services.pipeline import EvidencePipeline
from ..services.pipeline_runner import PipelineTaskRunner
from ..services.run_manager import RunManager
from ..usecase.submit_run import (
    GetClassificationRunUsecase,
    GetRunUsecase,
    SubmitClassificationRunUsecase,
    SubmitRunUsecase,
)
from .container import ServiceContainer


def get_container(request: Request) -> ServiceContainer:
    """Return the ServiceContainer bound to the running FastAPI app.

    Raises:
        RuntimeError: if the lifespan never populated ``app.state.container``.
    """
    container = getattr(request.app.state, "container", None)
    if container is None:
        raise RuntimeError(
            "ServiceContainer is not initialized on app.state. "
            "Ensure create_app()'s lifespan is active."
        )
    return container


def get_settings_dep() -> Settings:
    return get_settings()


def get_pipeline_dep(
    container: ServiceContainer = Depends(get_container),
) -> EvidencePipeline:
    return container.pipeline


def get_embedder_dep(
    container: ServiceContainer = Depends(get_container),
) -> Embedder:
    return container.embedder


def get_run_manager_dep(
    container: ServiceContainer = Depends(get_container),
) -> RunManager:
    return container.run_manager


def get_submit_run_usecase_dep(
    container: ServiceContainer = Depends(get_container),
) -> SubmitRunUsecase:
    return container.submit_run_usecase


def get_get_run_usecase_dep(
    container: ServiceContainer = Depends(get_container),
) -> GetRunUsecase:
    return container.get_run_usecase


def get_submit_classification_run_usecase_dep(
    container: ServiceContainer = Depends(get_container),
) -> SubmitClassificationRunUsecase:
    return container.submit_classification_run_usecase


def get_get_classification_run_usecase_dep(
    container: ServiceContainer = Depends(get_container),
) -> GetClassificationRunUsecase:
    return container.get_classification_run_usecase


def get_pipeline_runner_dep(
    container: ServiceContainer = Depends(get_container),
) -> PipelineTaskRunner | None:
    return container.pipeline_runner


def get_classifier_dep(
    container: ServiceContainer = Depends(get_container),
) -> GenreClassifierService:
    return container.classifier


def get_classification_runner_dep(
    container: ServiceContainer = Depends(get_container),
) -> ClassificationRunner:
    return container.classification_runner


async def get_session(
    container: ServiceContainer = Depends(get_container),
) -> AsyncIterator[AsyncSession]:
    """FastAPI dependency that yields an AsyncSession owned by the container."""
    async with container.db.session_factory() as session:
        yield session


def get_learning_service(
    container: ServiceContainer = Depends(get_container),
    session: AsyncSession = Depends(get_session),
) -> GenreLearningService:
    import structlog

    logger = structlog.get_logger(__name__)
    settings = container.settings
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


def get_learning_client(
    container: ServiceContainer = Depends(get_container),
) -> LearningClient:
    return container.learning_client


def get_admin_job_service_dep(
    container: ServiceContainer = Depends(get_container),
) -> AdminJobService:
    return container.admin_job_service


def get_extract_semaphore_dep(
    container: ServiceContainer = Depends(get_container),
) -> asyncio.Semaphore:
    return container.extract_semaphore


def get_content_extractor_dep(
    container: ServiceContainer = Depends(get_container),
) -> ContentExtractor:
    return container.content_extractor


def get_coarse_classifier_dep(
    container: ServiceContainer = Depends(get_container),
) -> CoarseClassifier:
    return container.coarse_classifier
