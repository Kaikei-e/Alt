"""Shared test fixtures for recap-subworker tests."""

from __future__ import annotations

import hashlib
from collections.abc import Sequence

import numpy as np
import pytest

from recap_subworker.domain.models import (
    ClusterDocument,
    EvidenceConstraints,
    EvidenceRequest,
)
from recap_subworker.infra.config import Settings
from recap_subworker.services.clusterer import ClusterResult, HDBSCANSettings


class HashEmbedder:
    """Deterministic fake embedder using xxhash-like hashing.

    Produces reproducible embeddings based on text content,
    satisfying EmbedderPort protocol.
    """

    def __init__(self, dim: int = 64) -> None:
        self.dim = dim
        self.config = type("Cfg", (), {"backend": "hash", "model_id": "hash-fake"})()

    def encode(self, sentences: Sequence[str]) -> np.ndarray:
        result = np.zeros((len(sentences), self.dim), dtype=np.float32)
        for i, text in enumerate(sentences):
            digest = hashlib.sha256(text.encode()).digest()
            for j in range(min(self.dim, 32)):
                result[i, j] = (digest[j] - 128) / 128.0
            # L2-normalize
            norm = np.linalg.norm(result[i])
            if norm > 0:
                result[i] /= norm
        return result

    def warmup(self, samples: Sequence[str]) -> int:
        return len(list(samples))

    def close(self) -> None:
        pass


class FakeClusterer:
    """Fake clusterer that assigns all items to n_clusters groups."""

    def __init__(self, n_clusters: int = 3) -> None:
        self.n_clusters = n_clusters

    def cluster(self, embeddings, *, min_cluster_size, min_samples):
        n = embeddings.shape[0]
        labels = np.array([i % self.n_clusters for i in range(n)], dtype=int)
        probs = np.ones(n, dtype=float)
        return ClusterResult(
            labels,
            probs,
            False,
            HDBSCANSettings(
                min_cluster_size=min_cluster_size,
                min_samples=min_samples,
            ),
        )

    def optimize_clustering(
        self, embeddings, *, min_cluster_size_range, min_samples_range, **kwargs
    ):
        return self.cluster(
            embeddings,
            min_cluster_size=min_cluster_size_range[0],
            min_samples=min_samples_range[0],
        )

    def subcluster_other(self, embeddings, token_counts=None):
        return self.cluster(
            embeddings, min_cluster_size=3, min_samples=2
        )

    def recursive_cluster(self, embeddings, labels, probabilities, token_counts):
        return labels, probabilities


@pytest.fixture
def fake_embedder() -> HashEmbedder:
    """Deterministic fake embedder for unit tests."""
    return HashEmbedder(dim=64)


@pytest.fixture
def fake_clusterer() -> FakeClusterer:
    """Fake clusterer returning n_clusters groups."""
    return FakeClusterer(n_clusters=3)


@pytest.fixture
def test_settings() -> Settings:
    """Minimal Settings instance for unit tests."""
    return Settings(model_id="fake")


def make_cluster_document(
    article_id: str = "art-1",
    paragraph_text: str = "x" * 80,
    n_paragraphs: int = 1,
) -> ClusterDocument:
    """Factory for ClusterDocument with valid defaults."""
    return ClusterDocument(
        article_id=article_id,
        title="Test Article",
        paragraphs=[paragraph_text] * n_paragraphs,
    )


def make_evidence_request(
    n_docs: int = 5,
    job_id: str = "test-job",
    genre: str = "tech",
) -> EvidenceRequest:
    """Factory for EvidenceRequest with valid defaults."""
    return EvidenceRequest(
        job_id=job_id,
        genre=genre,
        documents=[
            make_cluster_document(article_id=f"art-{i}")
            for i in range(n_docs)
        ],
        constraints=EvidenceConstraints(),
    )


# ---------------------------------------------------------------------------
# Phase 1 unblock: prevent unit tests from triggering real ML / subprocess
# initialization through the ServiceContainer lazy properties.
#
# Root cause (see docs/ADR/000726 → 000727): once create_app() moved the
# ServiceContainer ownership into the FastAPI lifespan, every fresh app
# instance resolves container.run_manager on first request. That access
# transitively boots a spawn ProcessPoolExecutor (re-importing torch /
# sentence-transformers / numpy 2.x in the child) AND a real
# GenreClassifierService / Embedder. Dispatched through a single anyio
# worker thread, this stalls indefinitely — not a Starlette/anyio bug but
# a CPython spawn + torch import-lock interaction (cpython#105829).
#
# The fix for unit tests is to never let request handling reach those
# lazy properties. We install an autouse override that swaps every heavy
# FastAPI dependency used in the /v1/* routers with an in-memory fake.
# Individual tests can still override back to a purpose-built fake via
# app.dependency_overrides[...] inside the test itself.
# ---------------------------------------------------------------------------


class _StubRunManager:
    async def create_run(self, submission):
        raise NotImplementedError(
            "Install a purpose-built RunManager stub via "
            "app.dependency_overrides[deps.get_run_manager_dep]"
        )

    async def get_run(self, run_id: int):
        return None


class _StubClassifier:
    def predict_batch(self, *args, **kwargs):
        return []


class _StubClassificationRunner:
    async def submit_run(self, *args, **kwargs):
        return None

    async def get_run(self, run_id):
        return None


class _StubContentExtractor:
    async def extract(self, *args, **kwargs):
        return ""


class _StubCoarseClassifier:
    def classify(self, *args, **kwargs):
        return {"top_genres": [], "confidence": 0.0}


class _StubAdminJobService:
    async def shutdown(self) -> None:
        return None

    async def enqueue_graph_job(self):
        return "stub-job"

    async def enqueue_learning_job(self):
        return "stub-job"

    async def get_job(self, job_id):
        return None


class _StubLearningClient:
    async def send_learning_payload(self, payload):
        return type("R", (), {"status_code": 200, "headers": {}, "json": lambda self: {}})()

    async def close(self) -> None:
        return None


@pytest.fixture(autouse=True)
def _override_heavy_dependencies(request):
    """Default unit-test dependency overrides.

    Applied to every test that instantiates ``create_app()`` unless the test
    explicitly opts out with the ``no_heavy_override`` marker. Prevents the
    TestClient from resolving ``container.run_manager`` / ``.classifier`` /
    ``.process_pool`` on the request path — which is what locks up the
    event loop worker thread on Python 3.14 + spawn PPE bootstrap.
    """
    if "no_heavy_override" in request.keywords:
        yield
        return

    # Lazy import inside the fixture so test files that never spin up the
    # app do not pay the import cost.
    import asyncio as _asyncio

    from fastapi.testclient import TestClient as _TestClient

    from recap_subworker.app import deps as _deps
    from recap_subworker.app.main import create_app as _create_app

    # Patch TestClient to inject the overrides the moment an app is built.
    original_create_app = _deps.__dict__.get("__create_app_sentinel__")
    if original_create_app is None:
        original_create_app = _create_app

    def _install(app) -> None:
        overrides = {
            _deps.get_run_manager_dep: lambda: _StubRunManager(),
            _deps.get_classifier_dep: lambda: _StubClassifier(),
            _deps.get_classification_runner_dep: lambda: _StubClassificationRunner(),
            _deps.get_content_extractor_dep: lambda: _StubContentExtractor(),
            _deps.get_coarse_classifier_dep: lambda: _StubCoarseClassifier(),
            _deps.get_admin_job_service_dep: lambda: _StubAdminJobService(),
            _deps.get_learning_client: lambda: _StubLearningClient(),
            _deps.get_extract_semaphore_dep: lambda: _asyncio.Semaphore(1),
        }
        for key, value in overrides.items():
            app.dependency_overrides.setdefault(key, value)

    # Hook TestClient so any app it wraps receives the overrides
    # before lifespan startup proceeds.
    original_enter = _TestClient.__enter__

    def _patched_enter(self, *args, **kwargs):
        _install(self.app)
        return original_enter(self, *args, **kwargs)

    _TestClient.__enter__ = _patched_enter
    try:
        yield
    finally:
        _TestClient.__enter__ = original_enter
