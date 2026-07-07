"""Concurrency regression test for /v1/classify.

classifier.predict_batch() (embedding + inference, CPU-bound) was called
directly inside the async handler, blocking the event loop for the full
call duration. Fixed via asyncio.to_thread offload.

Reference: docs/plan/20260706largerepocodereview.md
(recap-subworker/recap_subworker/app/routers/classification.py HIGH finding).
"""

from __future__ import annotations

import asyncio
import time

import httpx
import pytest
from fastapi import FastAPI

from recap_subworker.app import deps
from recap_subworker.app.routers import classification

_BLOCK_SECONDS = 0.3


def _build_app() -> FastAPI:
    """Minimal app hosting only the classification router.

    Deliberately avoids ``create_app()``: its ``setup_metrics()`` wires
    ``prometheus_fastapi_instrumentator`` middleware that currently raises
    ``AttributeError: '_IncludedRouter' object has no attribute 'path'`` on
    every request in this environment (pre-existing, unrelated to this
    endpoint — confirmed by the same failure on ``tests/unit/test_api_runs.py``
    on a clean checkout). This test only needs the router + DI override.
    """
    app = FastAPI()
    app.include_router(classification.router, prefix="/v1")
    return app


class _FakeEmbedderConfig:
    batch_size = 8


class _FakeEmbedder:
    config = _FakeEmbedderConfig()


class _BlockingClassifier:
    """Fake classifier whose predict_batch() blocks synchronously."""

    embedder = _FakeEmbedder()

    def predict_batch(self, texts, multi_label=False, top_k=5, threshold_overrides=None):
        time.sleep(_BLOCK_SECONDS)
        return [
            {
                "top_genre": "tech",
                "confidence": 0.9,
                "scores": {"tech": 0.9},
                "candidates": [],
            }
            for _ in texts
        ]


@pytest.mark.asyncio
async def test_classify_offloads_blocking_call_to_thread() -> None:
    """Two concurrent /v1/classify calls must overlap, not serialize."""
    app = _build_app()
    app.dependency_overrides[deps.get_classifier_dep] = lambda: _BlockingClassifier()

    transport = httpx.ASGITransport(app=app)
    async with httpx.AsyncClient(transport=transport, base_url="http://testserver") as client:
        start = time.monotonic()
        responses = await asyncio.gather(
            client.post("/v1/classify", json={"texts": ["hello"]}),
            client.post("/v1/classify", json={"texts": ["world"]}),
        )
        elapsed = time.monotonic() - start

    for response in responses:
        assert response.status_code == 200, response.text

    # Serialized (blocking) execution would take >= 2 * _BLOCK_SECONDS.
    assert elapsed < _BLOCK_SECONDS * 1.5, (
        f"requests appear serialized (elapsed={elapsed:.3f}s); "
        "classifier.predict_batch() is blocking the event loop"
    )
