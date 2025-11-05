"""Unit tests for the evidence pipeline."""

from __future__ import annotations

import numpy as np

from recap_subworker.domain.models import ArticlePayload, EvidenceRequest
from recap_subworker.infra.config import Settings
from recap_subworker.services.clusterer import ClusterResult, HDBSCANSettings
from recap_subworker.services.pipeline import EvidencePipeline


class FakeEmbedder:
    def __init__(self) -> None:
        self.config = type("Cfg", (), {"backend": "sentence-transformers", "model_id": "fake"})()

    def encode(self, sentences):
        base = np.array([[1.0, 0.0], [0.0, 1.0]], dtype=np.float32)
        vectors = base[: len(sentences)]
        return vectors

    def warmup(self, samples):
        return len(list(samples))

    def close(self):
        pass


class FakeClusterer:
    def cluster(self, embeddings, *, min_cluster_size, min_samples):
        labels = np.zeros((embeddings.shape[0],), dtype=int)
        probs = np.ones_like(labels, dtype=float)
        return ClusterResult(labels, probs, False, HDBSCANSettings(min_cluster_size=min_cluster_size, min_samples=min_samples))


def test_pipeline_basic_flow():
    settings = Settings(model_id="fake")
    pipeline = EvidencePipeline(settings=settings, embedder=FakeEmbedder(), process_pool=None)
    pipeline.clusterer = FakeClusterer()  # type: ignore[assignment]

    request = EvidenceRequest(
        job_id="job",
        genre="ai",
        articles=[
            ArticlePayload(
                source_id="art1",
                paragraphs=["Paragraph one. Second sentence."],
            )
        ],
    )

    response = pipeline.run(request)

    assert response.job_id == "job"
    assert response.genre == "ai"
    assert response.clusters
    assert response.evidence_budget.sentences > 0
