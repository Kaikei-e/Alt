"""Unit tests for ClusterEvidenceUsecase."""

from __future__ import annotations

from unittest.mock import MagicMock

from recap_subworker.domain.models import (
    EvidenceBudget,
    EvidenceResponse,
    WarmupResponse,
)
from recap_subworker.infra.config import Settings
from recap_subworker.domain.models import (
    ClusterDocument,
    EvidenceConstraints,
    EvidenceRequest,
)
from recap_subworker.usecase.cluster_evidence import ClusterEvidenceUsecase


def _make_evidence_request() -> EvidenceRequest:
    docs = [
        ClusterDocument(
            article_id=f"art-{i}",
            title="Test",
            paragraphs=["x" * 80],
        )
        for i in range(5)
    ]
    return EvidenceRequest(
        job_id="test-job",
        genre="tech",
        documents=docs,
        constraints=EvidenceConstraints(),
    )


class TestClusterEvidenceUsecase:
    def _make_usecase(self, fake_embedder, fake_clusterer):
        settings = Settings(model_id="fake")
        mock_pipeline = MagicMock()
        mock_pipeline.run.return_value = EvidenceResponse(
            job_id="test-job",
            genre="tech",
            clusters=[],
            evidence_budget=EvidenceBudget(sentences=0, tokens_estimated=0),
        )
        mock_pipeline.warmup.return_value = WarmupResponse(
            warmed=True, batches=1, backend="hash"
        )

        return ClusterEvidenceUsecase(
            settings=settings,
            embedder=fake_embedder,
            clusterer=fake_clusterer,
            pipeline=mock_pipeline,
        ), mock_pipeline

    def test_execute_delegates_to_pipeline(self, fake_embedder, fake_clusterer):
        usecase, mock_pipeline = self._make_usecase(fake_embedder, fake_clusterer)
        request = _make_evidence_request()

        result = usecase.execute(request)

        mock_pipeline.run.assert_called_once_with(request)
        assert result.job_id == "test-job"
        assert result.genre == "tech"

    def test_warmup_delegates_to_pipeline(self, fake_embedder, fake_clusterer):
        usecase, mock_pipeline = self._make_usecase(fake_embedder, fake_clusterer)

        result = usecase.warmup(["sample text"])

        mock_pipeline.warmup.assert_called_once_with(["sample text"])
        assert result.warmed is True

    def test_embedder_property(self, fake_embedder, fake_clusterer):
        usecase, _ = self._make_usecase(fake_embedder, fake_clusterer)
        assert usecase.embedder is fake_embedder

    def test_clusterer_property(self, fake_embedder, fake_clusterer):
        usecase, _ = self._make_usecase(fake_embedder, fake_clusterer)
        assert usecase.clusterer is fake_clusterer
