import time
from typing import List, Dict, Any
from fastapi import APIRouter, Depends
from pydantic import BaseModel

import structlog

from ..deps import get_classifier_dep
from ...services.classifier import GenreClassifierService

logger = structlog.get_logger(__name__)
router = APIRouter()

class ClassificationRequest(BaseModel):
    texts: List[str]
    multi_label: bool = False
    top_k: int = 5

class ValidationCandidate(BaseModel):
    genre: str
    score: float
    threshold: float

class ClassificationResult(BaseModel):
    top_genre: str
    confidence: float
    scores: Dict[str, float]
    candidates: List[ValidationCandidate] = []

class ClassificationResponse(BaseModel):
    results: List[ClassificationResult]

@router.post("/classify", response_model=ClassificationResponse)
async def classify_texts(
    request: ClassificationRequest,
    classifier: GenreClassifierService = Depends(get_classifier_dep),
):
    start_time = time.time()
    total_texts = len(request.texts)

    logger.info(
        "Starting classification",
        total_texts=total_texts,
        batch_size=classifier.embedder.config.batch_size if hasattr(classifier, 'embedder') else None,
        multi_label=request.multi_label,
    )

    # In a real app, we might get overrides from settings here if we want per-request overrides?
    # Or just use the global settings injected into the service.
    # The service updates its thresholds from internal state, but we need to pass overrides if we had any.
    # For Step 8, we want *runtime* adjustability via config.
    # So we should get the settings and check for overrides.

    # We can retrieve settings from the dependency injection if we change the signature slightly
    # But `classifier` is already injected.
    # Wait, `get_classifier_dep` uses `_get_classifier(settings)`.
    # `Settings` are loaded at startup/cached.
    # If we want *runtime* without restart, we might need to reload settings or check env every time?
    # Usually "runtime adjustability" means without code changes, i.e., env vars + restart is fine.
    # OR dynamic config.
    # The user manual said "configure thresholds ... ensure runtime adjustability".

    # Let's assume standard config (Settings) is sufficient, but we need to pass the parsed overrides to the predict method
    # if `classifier` service doesn't hold them automatically.
    # `GenreClassifierService` now accepts `threshold_overrides` in `predict_batch` (via `_ensure_model`).

    # However, `GenreClassifierService` instance is a singleton in `deps.py`.
    # If we want to support dynamic updates from env vars *per request* or *checked periodically*, we need access to the config.
    # The `classifier` fixture is created with the *startup* settings.
    # If `Settings` are proper Pydantic BaseSettings, they read env at instantiation.
    # `get_settings` is `@lru_cache`, so it won't re-read env unless we clear cache.

    # For now, let's proceed with passing the overrides explicitly if we can get them.
    # But simpler: just rely on the service to have them if initialized with them.
    # Wait, I didn't update `_get_classifier` in `deps.py` to pass overrides during init.
    # I should update `deps.py` or `GenreClassifierService.__init__`?
    # `GenreClassifierService` doesn't take overrides in `__init__`.
    # It takes them in `_ensure_model` or `predict_batch`.

    # Let's assume for this step we will extract overrides from the CURRENT settings object and pass them.
    from ..deps import get_settings
    settings = get_settings()

    # Parse json overrides
    import json
    overrides = {}
    if settings.genre_subworker_threshold_overrides:
        try:
            overrides = json.loads(settings.genre_subworker_threshold_overrides)
        except Exception as e:
            logger.warning("Failed to parse threshold overrides", error=str(e))

    results = classifier.predict_batch(
        request.texts,
        multi_label=request.multi_label,
        top_k=request.top_k,
        threshold_overrides=overrides
    )

    elapsed_time = time.time() - start_time
    logger.info(
        "Classification completed",
        total_texts=total_texts,
        results_count=len(results),
        elapsed_seconds=round(elapsed_time, 2),
        throughput_per_sec=round(total_texts / elapsed_time, 2) if elapsed_time > 0 else 0,
    )

    return ClassificationResponse(results=results)
