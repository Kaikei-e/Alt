"""Concurrency regression tests for /v1 preprocessing endpoints.

These endpoints wrap synchronous CPU-bound work (trafilatura extraction,
coarse-genre embedding, embedding + HDBSCAN clustering). Calling that work
directly inside an ``async def`` handler blocks the single-threaded asyncio
event loop for the full call duration, serializing every other in-flight
request on the same worker. The fix offloads each blocking call via
``asyncio.to_thread``.

Reference: docs/plan/20260706largerepocodereview.md
(recap-subworker/recap_subworker/app/routers/preprocessing.py HIGH findings).
"""

from __future__ import annotations

import asyncio
import time

import httpx
import numpy as np
import pytest
from fastapi import FastAPI

from recap_subworker.app import deps
from recap_subworker.app.routers import preprocessing

_BLOCK_SECONDS = 0.3


def _build_app() -> FastAPI:
    """Minimal app hosting only the preprocessing router.

    Deliberately avoids ``create_app()``: its ``setup_metrics()`` wires
    ``prometheus_fastapi_instrumentator`` middleware that currently raises
    ``AttributeError: '_IncludedRouter' object has no attribute 'path'`` on
    every request in this environment (pre-existing, unrelated to these
    endpoints — confirmed by the same failure on ``tests/unit/test_api_runs.py``
    on a clean checkout). These tests only need the router + DI overrides.
    """
    app = FastAPI()
    app.include_router(preprocessing.router, prefix="/v1")
    return app


class _BlockingExtractor:
    """Fake extractor whose extract_content() blocks synchronously."""

    def extract_content(self, html: str, include_comments: bool = False) -> str:
        time.sleep(_BLOCK_SECONDS)
        return "extracted"


class _BlockingCoarseClassifier:
    """Fake coarse classifier whose predict_coarse() blocks synchronously."""

    def predict_coarse(self, text: str) -> dict[str, float]:
        time.sleep(_BLOCK_SECONDS)
        return {"tech": 0.9}


class _BlockingEmbedder:
    """Fake embedder whose encode() blocks synchronously."""

    def encode(self, texts):
        time.sleep(_BLOCK_SECONDS)
        return np.zeros((len(texts), 4), dtype=np.float32)


class _FakeParams:
    min_cluster_size = 3
    min_samples = 1


class _FakeClusterResult:
    def __init__(self, n: int) -> None:
        self.labels = np.zeros(n, dtype=int)
        self.probabilities = np.ones(n, dtype=float)
        self.dbcv_score = 0.0
        self.params = _FakeParams()


class _BlockingClustererGateway:
    """Fake clusterer gateway whose subcluster_other() blocks synchronously."""

    def subcluster_other(self, embeddings, token_counts=None):
        time.sleep(_BLOCK_SECONDS)
        return _FakeClusterResult(embeddings.shape[0])


async def _concurrent_elapsed(app, requests: list[tuple[str, dict]]) -> float:
    transport = httpx.ASGITransport(app=app)
    async with httpx.AsyncClient(transport=transport, base_url="http://testserver") as client:
        start = time.monotonic()
        responses = await asyncio.gather(
            *(client.post(path, json=payload) for path, payload in requests)
        )
        elapsed = time.monotonic() - start
    for response in responses:
        assert response.status_code == 200, response.text
    return elapsed


@pytest.mark.asyncio
async def test_extract_offloads_blocking_call_to_thread() -> None:
    """Two concurrent /v1/extract calls must overlap, not serialize."""
    app = _build_app()
    app.dependency_overrides[deps.get_content_extractor_dep] = lambda: _BlockingExtractor()
    app.dependency_overrides[deps.get_extract_semaphore_dep] = lambda: asyncio.Semaphore(2)

    elapsed = await _concurrent_elapsed(
        app,
        [
            ("/v1/extract", {"html": "<p>" + "x" * 20 + "</p>"}),
            ("/v1/extract", {"html": "<p>" + "y" * 20 + "</p>"}),
        ],
    )

    # Serialized (blocking) execution would take >= 2 * _BLOCK_SECONDS.
    # Offloaded (asyncio.to_thread) execution overlaps and stays close to
    # a single _BLOCK_SECONDS.
    assert elapsed < _BLOCK_SECONDS * 1.5, (
        f"requests appear serialized (elapsed={elapsed:.3f}s); "
        "extractor.extract_content() is blocking the event loop"
    )


@pytest.mark.asyncio
async def test_classify_coarse_offloads_blocking_call_to_thread() -> None:
    """Two concurrent /v1/classify/coarse calls must overlap, not serialize."""
    app = _build_app()
    app.dependency_overrides[deps.get_coarse_classifier_dep] = lambda: _BlockingCoarseClassifier()

    elapsed = await _concurrent_elapsed(
        app,
        [
            ("/v1/classify/coarse", {"text": "hello world"}),
            ("/v1/classify/coarse", {"text": "goodbye world"}),
        ],
    )

    assert elapsed < _BLOCK_SECONDS * 1.5, (
        f"requests appear serialized (elapsed={elapsed:.3f}s); "
        "classifier.predict_coarse() is blocking the event loop"
    )


@pytest.mark.asyncio
async def test_cluster_other_offloads_blocking_calls_to_thread() -> None:
    """Two concurrent /v1/cluster/other calls must overlap, not serialize."""
    app = _build_app()
    app.dependency_overrides[deps.get_embedder_dep] = lambda: _BlockingEmbedder()
    app.dependency_overrides[deps.get_clusterer_gateway_dep] = lambda: _BlockingClustererGateway()

    elapsed = await _concurrent_elapsed(
        app,
        [
            ("/v1/cluster/other", {"texts": ["a", "b", "c"]}),
            ("/v1/cluster/other", {"texts": ["d", "e", "f"]}),
        ],
    )

    # Each request runs two blocking stages (encode + subcluster_other)
    # sequentially; a fully serialized pair of requests would take
    # >= 4 * _BLOCK_SECONDS. Offloaded, the two requests' matching stages
    # overlap so the total stays well under that.
    assert elapsed < _BLOCK_SECONDS * 3, (
        f"requests appear serialized (elapsed={elapsed:.3f}s); "
        "embedder.encode()/clusterer.subcluster_other() is blocking the event loop"
    )


def test_cluster_other_uses_di_wired_clusterer_gateway() -> None:
    """`/v1/cluster/other` must resolve its clusterer via the DI-wired
    container gateway (deps.get_clusterer_gateway_dep), not construct a
    fresh `Clusterer(settings)` per request."""
    import inspect

    from recap_subworker.app.routers import preprocessing

    source = inspect.getsource(preprocessing)
    assert "Clusterer(settings)" not in source
    assert "get_clusterer_gateway_dep" in source
