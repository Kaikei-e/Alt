import asyncio
import time

import structlog
from fastapi import APIRouter, Depends
from pydantic import BaseModel

from ...services.classifier import GenreClassifierService
from ..deps import get_classifier_dep

logger = structlog.get_logger(__name__)
router = APIRouter()

class ClassificationRequest(BaseModel):
    texts: list[str]
    multi_label: bool = False
    top_k: int = 5

class ValidationCandidate(BaseModel):
    genre: str
    score: float
    threshold: float

class ClassificationResult(BaseModel):
    top_genre: str
    confidence: float
    scores: dict[str, float]
    candidates: list[ValidationCandidate] = []

class ClassificationResponse(BaseModel):
    results: list[ClassificationResult]

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

    from ..deps import get_settings
    settings = get_settings()
    overrides = settings.genre_threshold_overrides_parsed

    # predict_batch() does embedding + inference (CPU-bound); offload to a
    # worker thread so it doesn't block the event loop.
    results = await asyncio.to_thread(
        classifier.predict_batch,
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
