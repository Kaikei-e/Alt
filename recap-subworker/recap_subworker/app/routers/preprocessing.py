import asyncio
from typing import Any

from fastapi import APIRouter, Depends
from pydantic import BaseModel

from ...port.clusterer import ClustererPort
from ...services.classification import CoarseClassifier
from ...services.embedder import Embedder
from ...services.extraction import ContentExtractor
from ..deps import (
    get_clusterer_gateway_dep,
    get_coarse_classifier_dep,
    get_content_extractor_dep,
    get_embedder_dep,
    get_extract_semaphore_dep,
)

router = APIRouter()

class ExtractRequest(BaseModel):
    html: str
    include_comments: bool = False

class ExtractResponse(BaseModel):
    text: str

class CoarseClassifyRequest(BaseModel):
    text: str

class CoarseClassifyResponse(BaseModel):
    scores: dict[str, float]

class SubClusterOtherRequest(BaseModel):
    texts: list[str]

class SubClusterOtherResponse(BaseModel):
    labels: list[int]
    probabilities: list[float]
    diagnostics: dict[str, Any]

@router.post("/extract", response_model=ExtractResponse)
async def extract_content(
    request: ExtractRequest,
    extractor: ContentExtractor = Depends(get_content_extractor_dep),
    semaphore=Depends(get_extract_semaphore_dep),
) -> ExtractResponse:
    """Extract main content from HTML."""
    async with semaphore:
        # trafilatura extraction is sync CPU work; the semaphore only bounds
        # concurrency, it does not stop this call from blocking the event
        # loop, so offload it to a worker thread.
        text = await asyncio.to_thread(
            extractor.extract_content, request.html, request.include_comments
        )
        return ExtractResponse(text=text)

@router.post("/classify/coarse", response_model=CoarseClassifyResponse)
async def classify_coarse(
    request: CoarseClassifyRequest,
    classifier: CoarseClassifier = Depends(get_coarse_classifier_dep)
) -> CoarseClassifyResponse:
    """Predict coarse genre scores."""
    # predict_coarse() does sync embedding generation (sync httpx + retry
    # backoff on the ollama-remote backend); offload to avoid blocking the
    # event loop.
    scores = await asyncio.to_thread(classifier.predict_coarse, request.text)
    return CoarseClassifyResponse(scores=scores)

@router.post("/cluster/other", response_model=SubClusterOtherResponse)
async def cluster_other(
    request: SubClusterOtherRequest,
    embedder: Embedder = Depends(get_embedder_dep),
    clusterer: ClustererPort = Depends(get_clusterer_gateway_dep),
) -> SubClusterOtherResponse:
    """Sub-cluster 'Other' genre items."""
    if not request.texts:
        return SubClusterOtherResponse(labels=[], probabilities=[], diagnostics={})

    # 1. Embed (sync CPU/IO work; offload to a worker thread)
    embeddings = await asyncio.to_thread(embedder.encode, request.texts)

    # 2. Cluster using the DI-wired clusterer gateway (UMAP/HDBSCAN is
    # CPU-bound; offload to a worker thread rather than blocking the event
    # loop, and reuse the container's clusterer instead of constructing a
    # fresh one per request).
    result = await asyncio.to_thread(clusterer.subcluster_other, embeddings)

    # 3. Return
    return SubClusterOtherResponse(
        labels=result.labels.tolist(),
        probabilities=result.probabilities.tolist(),
        diagnostics={
            "dbcv": result.dbcv_score,
            "min_cluster_size": result.params.min_cluster_size,
            "min_samples": result.params.min_samples
        }
    )
