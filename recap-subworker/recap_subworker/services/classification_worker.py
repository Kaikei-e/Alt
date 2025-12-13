"""Worker-side helpers for executing classification in isolation."""

from __future__ import annotations

import sys
from typing import Any, Union

from ..infra.config import Settings
from .classifier import GenreClassifierService
from .embedder import Embedder, EmbedderConfig
from .learning_machine_classifier import LearningMachineStudentClassifier


_CLASSIFIER: Union[GenreClassifierService, LearningMachineStudentClassifier, None] = None


def initialize(settings_payload: dict[str, Any]) -> None:
    """Initializer invoked inside worker processes to build the classifier.

    This function is called when a worker process starts. It initializes
    the classifier and embedder. If initialization fails, the error is
    logged and re-raised to prevent the worker from starting in a bad state.
    """
    import structlog

    logger = structlog.get_logger(__name__)

    try:
        logger.info("initializing classification worker process")
        settings = Settings(**settings_payload)

        global _CLASSIFIER

        # Check which backend to use
        backend = getattr(settings, "classification_backend", "joblib")

        if backend == "learning_machine":
            logger.info("using learning_machine student classifier backend")
            # Load learning machine student models
            student_ja_dir = getattr(settings, "learning_machine_student_ja_dir", None)
            student_en_dir = getattr(settings, "learning_machine_student_en_dir", None)
            taxonomy_path = getattr(settings, "learning_machine_taxonomy_path", None)
            device = getattr(settings, "device", "cpu")

            _CLASSIFIER = LearningMachineStudentClassifier(
                student_ja_dir=student_ja_dir,
                student_en_dir=student_en_dir,
                taxonomy_path=taxonomy_path,
                device=device,
            )
        else:
            # Default: joblib backend (backward compatibility)
            logger.info("using joblib classifier backend")
            logger.debug(
                "creating embedder",
                model_id=settings.model_id,
                backend=settings.model_backend,
                device=settings.device,
            )

            config = EmbedderConfig(
                model_id=settings.model_id,
                distill_model_id=settings.distill_model_id,
                backend=settings.model_backend,
                device=settings.device,
                batch_size=settings.batch_size,
                cache_size=settings.embed_cache_size,
            )
            embedder = Embedder(config)

            logger.debug("creating genre classifier service")
            _CLASSIFIER = GenreClassifierService(
                model_path=settings.genre_classifier_model_path,
                embedder=embedder,
            )

        logger.info("classification worker process initialized successfully")
    except Exception as exc:
        logger.exception(
            "classification worker initialization failed",
            error=str(exc),
            error_type=type(exc).__name__,
            exc_info=True,
        )
        # Re-raise to prevent worker from starting in a bad state
        # This will cause the pool to fail initialization and be cleaned up
        sys.exit(1)


def _require_classifier() -> Union[GenreClassifierService, LearningMachineStudentClassifier]:
    if _CLASSIFIER is None:  # pragma: no cover - runtime safeguard
        raise RuntimeError("Classification worker not initialized")
    return _CLASSIFIER


def predict_batch(texts: list[str]) -> list[dict[str, Any]]:
    """Execute classification and return a JSON-serializable response."""
    classifier = _require_classifier()
    results = classifier.predict_batch(texts)
    return results

