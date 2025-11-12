"""Unit tests for the evidence pipeline."""

from __future__ import annotations

import numpy as np

from recap_subworker.domain.models import ClusterDocument, EvidenceRequest
from recap_subworker.infra.config import Settings
from recap_subworker.services.clusterer import ClusterResult, HDBSCANSettings
from recap_subworker.services.pipeline import EvidencePipeline


class FakeEmbedder:
    def __init__(self) -> None:
        self.config = type("Cfg", (), {"backend": "sentence-transformers", "model_id": "fake"})()

    def encode(self, sentences):
        size = max(1, len(sentences))
        return np.eye(size, dtype=np.float32)

    def warmup(self, samples):
        return len(list(samples))

    def close(self):
        pass


class FakeClusterer:
    def cluster(self, embeddings, *, min_cluster_size, min_samples):
        labels = np.zeros((embeddings.shape[0],), dtype=int)
        probs = np.ones_like(labels, dtype=float)
        return ClusterResult(labels, probs, False, HDBSCANSettings(min_cluster_size=min_cluster_size, min_samples=min_samples))


class SplitClusterer:
    """Clusterer that forces each sentence into its own cluster."""

    def cluster(self, embeddings, *, min_cluster_size, min_samples):
        labels = np.arange(embeddings.shape[0], dtype=int)
        probs = np.ones_like(labels, dtype=float)
        return ClusterResult(
            labels,
            probs,
            False,
            HDBSCANSettings(min_cluster_size=min_cluster_size, min_samples=min_samples),
        )


def test_pipeline_basic_flow():
    settings = Settings(model_id="fake")
    pipeline = EvidencePipeline(settings=settings, embedder=FakeEmbedder(), process_pool=None)
    pipeline.clusterer = FakeClusterer()  # type: ignore[assignment]
    pipeline._compute_topics = lambda corpora: [[] for _ in corpora]  # type: ignore[attr-defined]

    request = EvidenceRequest(
        job_id="job",
        genre="ai",
        documents=[
            ClusterDocument(
                article_id="art1",
                paragraphs=["Paragraph one is sufficiently lengthy to satisfy validation."],
            ),
            ClusterDocument(
                article_id="art2",
                paragraphs=["Another qualifying document ensures topic extraction has enough data."],
            )
        ],
    )

    response = pipeline.run(request)

    assert response.job_id == "job"
    assert response.genre == "ai"
    assert response.clusters
    assert response.evidence_budget.sentences > 0


def test_pipeline_prevents_article_reuse_across_clusters():
    settings = Settings(model_id="fake", max_sentences_per_cluster=1)
    pipeline = EvidencePipeline(settings=settings, embedder=FakeEmbedder(), process_pool=None)
    pipeline.clusterer = SplitClusterer()  # type: ignore[assignment]
    pipeline._compute_topics = lambda corpora: [[] for _ in corpora]  # type: ignore[attr-defined]

    request = EvidenceRequest(
        job_id="job",
        genre="ai",
        documents=[
            ClusterDocument(
                article_id="dup",
                paragraphs=[
                    "First sentence easily exceeds thirty characters. Second sentence also satisfies the minimum length requirement."
                ],
            ),
            ClusterDocument(
                article_id="unique",
                paragraphs=["Another paragraph with adequate length for processing to keep counts realistic."],
            ),
        ],
    )

    response = pipeline.run(request)

    assert len(response.clusters) >= 2
    first_cluster = response.clusters[0]
    second_cluster = response.clusters[1]

    assert first_cluster.representatives, "first cluster should include at least one sentence"
    assert second_cluster.representatives == [], "second cluster using duplicate article should be empty"
    assert first_cluster.supporting_ids == ["dup"]
    assert second_cluster.supporting_ids == [], "supporting ids should not repeat duplicates"
