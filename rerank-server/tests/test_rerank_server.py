"""Unit tests for rerank_server.py.

These tests never enter the app's lifespan context manager, so the real
CrossEncoder is never downloaded/loaded. `app.state.model` is set directly
to a lightweight fake before each test that needs a "loaded" model.
"""

from __future__ import annotations

from unittest.mock import MagicMock

import pytest
from fastapi.testclient import TestClient

from rerank_server import DEFAULT_MODEL, MAX_CANDIDATE_LENGTH, MAX_CANDIDATES, app


@pytest.fixture
def client() -> TestClient:
    # Plain (non-context-manager) TestClient never triggers `lifespan`,
    # so the heavy real model load is skipped entirely.
    return TestClient(app)


@pytest.fixture
def fake_model() -> MagicMock:
    """A CrossEncoder-shaped fake: only `.predict()` is exercised."""
    model = MagicMock()
    model.predict.side_effect = lambda pairs: [float(i) for i in range(len(pairs))]
    return model


def test_rerank_without_loaded_model_returns_503(client: TestClient) -> None:
    app.state.model = None

    resp = client.post("/v1/rerank", json={"query": "q", "candidates": ["a", "b"]})

    assert resp.status_code == 503


def test_rerank_returns_results_sorted_by_score_desc(
    client: TestClient, fake_model: MagicMock
) -> None:
    app.state.model = fake_model

    resp = client.post(
        "/v1/rerank", json={"query": "q", "candidates": ["low", "mid", "high"]}
    )

    assert resp.status_code == 200
    body = resp.json()
    scores = [r["score"] for r in body["results"]]
    assert scores == sorted(scores, reverse=True)
    # fake_model assigns score == candidate's original index, so "high" (idx 2) wins
    assert body["results"][0]["index"] == 2


def test_rerank_respects_top_k(client: TestClient, fake_model: MagicMock) -> None:
    app.state.model = fake_model

    resp = client.post(
        "/v1/rerank",
        json={"query": "q", "candidates": ["a", "b", "c"], "top_k": 2},
    )

    assert resp.status_code == 200
    assert len(resp.json()["results"]) == 2


def test_rerank_empty_candidates_returns_empty_results(
    client: TestClient, fake_model: MagicMock
) -> None:
    app.state.model = fake_model

    resp = client.post("/v1/rerank", json={"query": "q", "candidates": []})

    assert resp.status_code == 200
    assert resp.json()["results"] == []
    fake_model.predict.assert_not_called()


def test_rerank_rejects_too_many_candidates(
    client: TestClient, fake_model: MagicMock
) -> None:
    app.state.model = fake_model

    resp = client.post(
        "/v1/rerank",
        json={"query": "q", "candidates": ["x"] * (MAX_CANDIDATES + 1)},
    )

    assert resp.status_code == 422


def test_rerank_rejects_candidate_exceeding_max_length(
    client: TestClient, fake_model: MagicMock
) -> None:
    app.state.model = fake_model

    resp = client.post(
        "/v1/rerank",
        json={"query": "q", "candidates": ["x" * (MAX_CANDIDATE_LENGTH + 1)]},
    )

    assert resp.status_code == 422


def test_health_returns_503_while_model_not_loaded(client: TestClient) -> None:
    app.state.model = None

    resp = client.get("/health")

    assert resp.status_code == 503
    assert resp.json()["status"] == "loading"


def test_health_returns_200_when_model_loaded(
    client: TestClient, fake_model: MagicMock
) -> None:
    app.state.model = fake_model

    resp = client.get("/health")

    assert resp.status_code == 200
    body = resp.json()
    assert body["status"] == "ok"
    assert body["model"] == DEFAULT_MODEL


def test_rerank_rejects_unsupported_model(
    client: TestClient, fake_model: MagicMock
) -> None:
    app.state.model = fake_model

    resp = client.post(
        "/v1/rerank",
        json={
            "query": "q",
            "candidates": ["a"],
            "model": "some-other-model",
        },
    )

    assert resp.status_code == 422
    fake_model.predict.assert_not_called()
