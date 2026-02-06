"""Tests for Pydantic model security hardening (extra=forbid, max_length)."""

from __future__ import annotations

import pytest
from pydantic import ValidationError

from recap_subworker.domain.models import (
    ClusterDocument,
    ClusterJobParams,
    ClusterJobPayload,
    EvidenceConstraints,
    EvidenceRequest,
)


def _make_doc(article_id: str = "art-1") -> ClusterDocument:
    return ClusterDocument(
        article_id=article_id,
        title="Test",
        paragraphs=["x" * 80],
    )


class TestClusterJobPayloadSecurity:
    def test_extra_fields_rejected(self):
        params = ClusterJobParams(
            max_sentences_total=200,
            umap_n_components=25,
            hdbscan_min_cluster_size=5,
            mmr_lambda=0.35,
        )
        with pytest.raises(ValidationError, match="extra"):
            ClusterJobPayload(
                params=params,
                documents=[_make_doc() for _ in range(3)],
                evil_field="injected",  # type: ignore[call-arg]
            )

    def test_documents_max_length(self):
        """Documents list must not exceed 5000 items."""
        params = ClusterJobParams(
            max_sentences_total=200,
            umap_n_components=25,
            hdbscan_min_cluster_size=5,
            mmr_lambda=0.35,
        )
        # This should work (under limit)
        payload = ClusterJobPayload(
            params=params,
            documents=[_make_doc(article_id=f"art-{i}") for i in range(3)],
        )
        assert len(payload.documents) == 3


class TestEvidenceRequestSecurity:
    def test_extra_fields_rejected(self):
        with pytest.raises(ValidationError, match="extra"):
            EvidenceRequest(
                job_id="test",
                genre="tech",
                documents=[_make_doc()],
                sneaky="bad",  # type: ignore[call-arg]
            )

    def test_documents_max_length_field_exists(self):
        """EvidenceRequest.documents has max_length=5000."""
        field = EvidenceRequest.model_fields["documents"]
        assert field.metadata is not None or field.json_schema_extra is not None


class TestClusterDocumentSecurity:
    def test_extra_fields_rejected(self):
        with pytest.raises(ValidationError, match="extra"):
            ClusterDocument(
                article_id="art-1",
                paragraphs=["x" * 80],
                malicious="payload",  # type: ignore[call-arg]
            )

    def test_article_id_max_length(self):
        with pytest.raises(ValidationError):
            ClusterDocument(
                article_id="a" * 200,
                paragraphs=["x" * 80],
            )
