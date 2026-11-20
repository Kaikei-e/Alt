"""Characterization tests for EvidencePipeline.run and _build_clusters branches."""

from __future__ import annotations

from collections.abc import Sequence
from typing import Any

import numpy as np

from recap_subworker.domain.models import (
    ClusterDocument,
    EvidenceConstraints,
    EvidenceRequest,
    TelemetryEnvelope,
)
from recap_subworker.infra.config import Settings
from recap_subworker.services.clusterer import ClusterResult, HDBSCANSettings
from recap_subworker.services.pipeline import EvidencePipeline

MEDIUM_CORPUS_SENTENCES = 12
LARGE_CORPUS_SENTENCES = 65
SMALL_CLUSTER_MCS = 5
MEDIUM_CLUSTER_MCS = 5
LARGE_CLUSTER_MCS = 10
PARTIAL_MAX_SENTENCES = 1
EXPECTED_PARTIAL_DIAGNOSTICS = 1


class _FakeEmbedder:
    def __init__(self) -> None:
        self.config = type("Cfg", (), {"backend": "sentence-transformers", "model_id": "fake"})()

    def encode(self, sentences: Sequence[str]) -> np.ndarray:
        dim = 64
        rng = np.random.default_rng(42)
        vectors = rng.normal(size=(len(sentences), dim)).astype(np.float32)
        norms = np.linalg.norm(vectors, axis=1, keepdims=True)
        return vectors / np.where(norms == 0, 1.0, norms)

    def warmup(self, samples: Sequence[str]) -> int:
        return len(list(samples))

    def close(self) -> None:
        pass


class _RecordingClusterer:
    def __init__(self) -> None:
        self.optimize_calls: list[dict[str, Any]] = []
        self.subcluster_calls: int = 0

    def optimize_clustering(
        self,
        embeddings: np.ndarray,
        **kwargs: object,
    ) -> ClusterResult:
        self.optimize_calls.append(dict(kwargs))
        labels = np.zeros((embeddings.shape[0],), dtype=int)
        probs = np.ones_like(labels, dtype=float)
        return ClusterResult(
            labels=labels,
            probabilities=probs,
            used_umap=False,
            params=HDBSCANSettings(min_cluster_size=SMALL_CLUSTER_MCS, min_samples=1),
        )

    def subcluster_other(
        self,
        embeddings: np.ndarray,
        token_counts: np.ndarray | None = None,
    ) -> ClusterResult:
        self.subcluster_calls += 1
        labels = np.zeros((embeddings.shape[0],), dtype=int)
        probs = np.ones_like(labels, dtype=float)
        return ClusterResult(
            labels=labels,
            probabilities=probs,
            used_umap=False,
            params=HDBSCANSettings(min_cluster_size=3, min_samples=1),
        )


def _make_documents(count: int) -> list[ClusterDocument]:
    return [
        ClusterDocument(
            article_id=f"art-{i}",
            paragraphs=[
                f"Unique sentence number {i} with enough characters to pass length checks."
            ],
        )
        for i in range(count)
    ]


def test_pipeline_run_other_genre_calls_subcluster_other_and_truncates_partial() -> None:
    settings = Settings(model_id="fake")
    pipeline = EvidencePipeline(settings=settings, embedder=_FakeEmbedder(), process_pool=None)
    recorder = _RecordingClusterer()
    pipeline.clusterer = recorder  # type: ignore[assignment]
    pipeline._compute_topics = lambda corpora: [["topic"] for _ in corpora]  # type: ignore[assignment]

    request = EvidenceRequest(
        job_id="job-other",
        genre="other",
        documents=_make_documents(3),
        constraints=EvidenceConstraints(max_total_sentences=PARTIAL_MAX_SENTENCES),
        telemetry=TelemetryEnvelope(request_id="req-1", prompt_version="v2"),
    )

    response = pipeline.run(request)

    assert recorder.subcluster_calls == 1
    assert not recorder.optimize_calls
    assert response.diagnostics.partial is True
    assert response.diagnostics.total_sentences == EXPECTED_PARTIAL_DIAGNOSTICS


def test_pipeline_run_selects_medium_and_large_parameter_ranges() -> None:
    settings = Settings(model_id="fake", enable_umap_auto=True)
    pipeline = EvidencePipeline(settings=settings, embedder=_FakeEmbedder(), process_pool=None)
    recorder = _RecordingClusterer()
    pipeline.clusterer = recorder  # type: ignore[assignment]
    pipeline._compute_topics = lambda corpora: [["topic"] for _ in corpora]  # type: ignore[assignment]

    medium_request = EvidenceRequest(
        job_id="job-medium",
        genre="tech",
        documents=_make_documents(MEDIUM_CORPUS_SENTENCES),
    )
    pipeline.run(medium_request)

    large_request = EvidenceRequest(
        job_id="job-large",
        genre="tech",
        documents=_make_documents(LARGE_CORPUS_SENTENCES),
    )
    pipeline.run(large_request)

    expected_calls = 2
    assert len(recorder.optimize_calls) == expected_calls
    assert recorder.optimize_calls[0]["min_cluster_size_range"] == [5, 7, 10]
    assert recorder.optimize_calls[0]["umap_n_neighbors_range"] == [15, 30]
    assert recorder.optimize_calls[1]["min_cluster_size_range"] == [10, 15, 20]
    assert recorder.optimize_calls[1]["umap_n_neighbors_range"] == [30, 50]


def test_build_clusters_characterization_preserves_order_and_budget() -> None:
    settings = Settings(model_id="fake")
    pipeline = EvidencePipeline(settings=settings, embedder=_FakeEmbedder(), process_pool=None)
    recorder = _RecordingClusterer()
    pipeline.clusterer = recorder  # type: ignore[assignment]
    pipeline._compute_topics = lambda corpora: [["term"] for _ in corpora]  # type: ignore[assignment]

    request = EvidenceRequest(
        job_id="job-budget",
        genre="tech",
        documents=[
            ClusterDocument(
                article_id="art-1",
                source_url="https://example.com/1",
                paragraphs=["First valid representative sentence exceeding twenty chars."],
            ),
            ClusterDocument(
                article_id="art-2",
                source_url="https://example.com/2",
                paragraphs=["Second valid representative sentence exceeding twenty chars."],
            ),
        ],
        constraints=EvidenceConstraints(max_sentences_per_cluster=2, mmr_lambda=0.5),
    )

    response = pipeline.run(request)

    expected_reps = 2
    assert len(response.clusters) == 1
    assert response.clusters[0].label.top_terms == ["term"]
    assert len(response.clusters[0].representatives) == expected_reps
    assert response.evidence_budget.sentences == expected_reps
    assert response.evidence_budget.tokens_estimated > 0
