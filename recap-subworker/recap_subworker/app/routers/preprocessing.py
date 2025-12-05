from typing import List, Dict, Any
from fastapi import APIRouter, Depends, HTTPException
from pydantic import BaseModel, ConfigDict
import numpy as np

from ...services.extraction import ContentExtractor
from ...services.classification import CoarseClassifier
from ...services.clusterer import Clusterer, ClusterResult
from ...services.embedder import Embedder
from ..deps import (
    get_content_extractor_dep,
    get_coarse_classifier_dep,
    get_embedder_dep,
    get_settings_dep
)
from ...infra.config import Settings

router = APIRouter()

class ExtractRequest(BaseModel):
    html: str
    include_comments: bool = False

class ExtractResponse(BaseModel):
    text: str

class CoarseClassifyRequest(BaseModel):
    text: str

class CoarseClassifyResponse(BaseModel):
    scores: Dict[str, float]

class SubClusterOtherRequest(BaseModel):
    texts: List[str]

class SubClusterOtherResponse(BaseModel):
    labels: List[int]
    probabilities: List[float]
    diagnostics: Dict[str, Any]

@router.post("/extract", response_model=ExtractResponse)
async def extract_content(
    request: ExtractRequest,
    extractor: ContentExtractor = Depends(get_content_extractor_dep)
) -> ExtractResponse:
    """Extract main content from HTML."""
    text = extractor.extract_content(request.html, request.include_comments)
    return ExtractResponse(text=text)

@router.post("/classify/coarse", response_model=CoarseClassifyResponse)
async def classify_coarse(
    request: CoarseClassifyRequest,
    classifier: CoarseClassifier = Depends(get_coarse_classifier_dep)
) -> CoarseClassifyResponse:
    """Predict coarse genre scores."""
    scores = classifier.predict_coarse(request.text)
    return CoarseClassifyResponse(scores=scores)

@router.post("/cluster/other", response_model=SubClusterOtherResponse)
async def cluster_other(
    request: SubClusterOtherRequest,
    embedder: Embedder = Depends(get_embedder_dep),
    settings: Settings = Depends(get_settings_dep),
) -> SubClusterOtherResponse:
    """Sub-cluster 'Other' genre items."""
    if not request.texts:
        return SubClusterOtherResponse(labels=[], probabilities=[], diagnostics={})

    # 1. Embed
    embeddings = embedder.encode(request.texts)

    # 2. Cluster using specialized method
    clusterer = Clusterer(settings)
    result = clusterer.subcluster_other(embeddings)

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
