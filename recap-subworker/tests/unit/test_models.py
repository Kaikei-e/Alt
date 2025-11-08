"""Unit tests for ClusterJob API models."""

from __future__ import annotations

import pytest
from pydantic import ValidationError

from recap_subworker.domain.models import (
    ClusterDocument,
    ClusterJobParams,
    ClusterJobPayload,
    ClusterJobResponse,
)


def _make_document(paragraph: str = "x" * 64) -> ClusterDocument:
    return ClusterDocument(
        article_id="art-1",
        title="Sample",
        paragraphs=[paragraph],
    )


def test_cluster_job_payload_requires_minimum_documents():
    params = ClusterJobParams(max_sentences_total=2000, umap_n_components=25, hdbscan_min_cluster_size=5, mmr_lambda=0.35)
    with pytest.raises(ValidationError):
        ClusterJobPayload(params=params, documents=[_make_document()])


def test_cluster_document_enforces_paragraph_length():
    params = ClusterJobParams(max_sentences_total=2000, umap_n_components=25, hdbscan_min_cluster_size=5, mmr_lambda=0.35)
    with pytest.raises(ValidationError):
        ClusterJobPayload(
            params=params,
            documents=[_make_document(paragraph="short") for _ in range(10)],
        )


def test_cluster_job_response_serialization_includes_status():
    response = ClusterJobResponse(
        run_id=1,
        job_id="job",
        genre="ai",
        status="running",
        cluster_count=0,
        clusters=[],
    )
    assert response.status == "running"
