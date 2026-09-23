"""Story clustering endpoint router."""

from __future__ import annotations

import asyncio

from fastapi import APIRouter, Depends

from ...services.story_clusterer import (
    ClusterStoriesRequest,
    ClusterStoriesResponse,
    StoryClustererService,
)
from ..deps import get_story_clusterer_service_dep

router = APIRouter()


@router.post("/cluster-stories", response_model=ClusterStoriesResponse)
async def cluster_stories_endpoint(
    request: ClusterStoriesRequest,
    service: StoryClustererService = Depends(get_story_clusterer_service_dep),
) -> ClusterStoriesResponse:
    """Cluster story items using agglomerative clustering with cosine distance and time decay."""
    return await asyncio.to_thread(
        service.cluster_stories,
        items=request.items,
        params=request.params,
    )
