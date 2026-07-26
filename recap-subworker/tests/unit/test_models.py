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
    params = ClusterJobParams(
        max_sentences_total=2000, umap_n_components=25, hdbscan_min_cluster_size=5, mmr_lambda=0.35
    )
    # min_length=3 on ClusterJobPayload.documents: pin both the failing field
    # and the "too few" reason so this doesn't silently start passing for an
    # unrelated validation error (e.g. a paragraph-length regression).
    with pytest.raises(ValidationError, match=r"documents\s+List should have at least 3 items"):
        ClusterJobPayload(params=params, documents=[_make_document()])


def test_cluster_document_enforces_paragraph_length():
    params = ClusterJobParams(
        max_sentences_total=2000, umap_n_components=25, hdbscan_min_cluster_size=5, mmr_lambda=0.35
    )
    # _validate_paragraphs raises this exact message for paragraphs under the
    # 30-character floor; match on it so a change to the *documents* count
    # constraint (or an unrelated field error) can't masquerade as this case.
    with pytest.raises(ValidationError, match=r"paragraph text must be at least 30 characters"):
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
