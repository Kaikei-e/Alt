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

class ClassificationResult(BaseModel):
    top_genre: str
    confidence: float
    scores: Dict[str, float]

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
    )

    results = classifier.predict_batch(request.texts)

    elapsed_time = time.time() - start_time
    logger.info(
        "Classification completed",
        total_texts=total_texts,
        results_count=len(results),
        elapsed_seconds=round(elapsed_time, 2),
        throughput_per_sec=round(total_texts / elapsed_time, 2) if elapsed_time > 0 else 0,
    )

    return ClassificationResponse(results=results)
