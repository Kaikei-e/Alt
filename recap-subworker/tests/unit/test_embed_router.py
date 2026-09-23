"""Tests for POST /v1/embed router."""

from __future__ import annotations

import numpy as np
import pytest
from fastapi import FastAPI
from starlette.testclient import TestClient

from recap_subworker.app import deps
from recap_subworker.app.routers import embed
from recap_subworker.services.embed_service import EmbedService
from tests.conftest import HashEmbedder


def _build_app(embedder: HashEmbedder | None = None) -> FastAPI:
    """Minimal FastAPI app hosting only the embed router with DI overrides."""
    app = FastAPI()
    app.include_router(embed.router, prefix="/v1")
    fake = embedder or HashEmbedder(dim=1024)
    service = EmbedService(embedder=fake)
    app.dependency_overrides[deps.get_embed_service_dep] = lambda: service
    return app


def test_embed_success():
    """Verify POST /v1/embed returns 200 with valid model, dim, and normalized embeddings."""
    app = _build_app(HashEmbedder(dim=1024))
    client = TestClient(app)

    response = client.post(
        "/v1/embed",
        json={
            "texts": ["Example headline 1", "Example headline 2"],
            "normalize": True,
        },
    )

    assert response.status_code == 200
    data = response.json()
    assert data["model"] == "hash-fake"
    assert data["dim"] == 1024
    assert len(data["embeddings"]) == 2
    assert len(data["embeddings"][0]) == 1024
    assert len(data["embeddings"][1]) == 1024

    # Verify L2 normalization
    v0 = np.array(data["embeddings"][0])
    v1 = np.array(data["embeddings"][1])
    assert np.linalg.norm(v0) == pytest.approx(1.0, rel=1e-4)
    assert np.linalg.norm(v1) == pytest.approx(1.0, rel=1e-4)


def test_embed_empty_texts_422():
    """Verify POST /v1/embed returns 422 when texts list is empty."""
    app = _build_app()
    client = TestClient(app)

    response = client.post("/v1/embed", json={"texts": [], "normalize": True})
    assert response.status_code == 422


def test_embed_empty_text_element_422():
    """Verify POST /v1/embed returns 422 when any text is empty."""
    app = _build_app()
    client = TestClient(app)

    response = client.post(
        "/v1/embed",
        json={"texts": ["Example headline 1", ""], "normalize": True},
    )
    assert response.status_code == 422


def test_embed_whitespace_text_element_422():
    """Verify POST /v1/embed returns 422 when any text is only whitespace."""
    app = _build_app()
    client = TestClient(app)

    response = client.post(
        "/v1/embed",
        json={"texts": ["   \t\n  "], "normalize": True},
    )
    assert response.status_code == 422


def test_embed_exceeds_256_texts_422():
    """Verify POST /v1/embed returns 422 when texts exceeds 256 items."""
    app = _build_app()
    client = TestClient(app)

    texts = [f"Example headline {i}" for i in range(257)]
    response = client.post("/v1/embed", json={"texts": texts, "normalize": True})
    assert response.status_code == 422


def test_embed_boundary_256_texts_success():
    """Verify POST /v1/embed accepts boundary condition of exactly 256 texts."""
    app = _build_app(HashEmbedder(dim=64))
    client = TestClient(app)

    texts = [f"Example headline {i}" for i in range(256)]
    response = client.post("/v1/embed", json={"texts": texts, "normalize": True})
    assert response.status_code == 200
    data = response.json()
    assert len(data["embeddings"]) == 256
    assert data["dim"] == 64


def test_embed_missing_body_422():
    """Verify POST /v1/embed returns 422 on empty or invalid request body."""
    app = _build_app()
    client = TestClient(app)

    response = client.post("/v1/embed", json={})
    assert response.status_code == 422


def test_embed_default_normalize():
    """Verify POST /v1/embed defaults normalize to True when omitted."""
    app = _build_app(HashEmbedder(dim=64))
    client = TestClient(app)

    response = client.post("/v1/embed", json={"texts": ["Example headline 1"]})
    assert response.status_code == 200
    data = response.json()
    v = np.array(data["embeddings"][0])
    assert np.linalg.norm(v) == pytest.approx(1.0, rel=1e-4)


def test_embed_502_on_unresolvable_identity():
    """Verify POST /v1/embed returns 502 when embedder identity cannot be resolved."""

    class _AnonymousEmbedder:
        def encode(self, texts):
            return np.ones((len(texts), 8), dtype=np.float32)

    app = _build_app(_AnonymousEmbedder())  # type: ignore[arg-type]
    client = TestClient(app)

    response = client.post("/v1/embed", json={"texts": ["Example headline 1"]})
    assert response.status_code == 502
    data = response.json()
    assert "detail" in data
    assert "reason" in data["detail"]


def test_embed_502_on_backend_failure():
    """Verify POST /v1/embed returns 502 when backend embedder fails during encode."""

    class _FailingEmbedder:
        model_id = "test-model"

        def encode(self, texts):
            raise RuntimeError("Backend connection timed out")

    app = _build_app(_FailingEmbedder())  # type: ignore[arg-type]
    client = TestClient(app)

    response = client.post("/v1/embed", json={"texts": ["Example headline 1"]})
    assert response.status_code == 502
    data = response.json()
    assert "detail" in data
    assert "reason" in data["detail"]
