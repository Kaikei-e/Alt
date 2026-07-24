"""Unit tests for rerank_server.py.

These tests never enter the app's lifespan context manager, so the real
CrossEncoder is never downloaded/loaded. `app.state.model` is set directly
to a lightweight fake before each test that needs a "loaded" model.
"""

from __future__ import annotations

import asyncio
from pathlib import Path
from unittest.mock import MagicMock

import pytest
from fastapi.testclient import TestClient

import rerank_server
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


def test_predict_sync_passes_batch_size_on_onnx_backend(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setattr(rerank_server, "RERANK_BACKEND", "onnx")
    monkeypatch.setattr(rerank_server, "RERANK_BATCH_SIZE", 4)
    fake_model = MagicMock()
    pairs = [("q", "a"), ("q", "b")]

    rerank_server._predict_sync(fake_model, pairs)

    fake_model.predict.assert_called_once_with(pairs, batch_size=4)


def test_predict_sync_omits_batch_size_on_torch_backend(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setattr(rerank_server, "RERANK_BACKEND", "torch")
    fake_model = MagicMock()
    pairs = [("q", "a"), ("q", "b")]

    rerank_server._predict_sync(fake_model, pairs)

    fake_model.predict.assert_called_once_with(pairs)


def test_load_onnx_model_skips_export_when_quantized_file_present(
    monkeypatch: pytest.MonkeyPatch, tmp_path: Path
) -> None:
    onnx_dir = tmp_path / "onnx"
    onnx_dir.mkdir()
    (onnx_dir / rerank_server.ONNX_QUANTIZED_FILE_NAME).touch()
    monkeypatch.setattr(rerank_server, "RERANK_MODEL_DIR", str(tmp_path))

    export_mock = MagicMock()
    monkeypatch.setattr(rerank_server, "_export_quantized_onnx_model", export_mock)
    cross_encoder_mock = MagicMock()
    monkeypatch.setattr(rerank_server, "CrossEncoder", cross_encoder_mock)

    rerank_server._load_onnx_model()

    export_mock.assert_not_called()
    cross_encoder_mock.assert_called_once()
    _, call_kwargs = cross_encoder_mock.call_args
    assert call_kwargs["backend"] == "onnx"
    assert call_kwargs["model_kwargs"]["file_name"] == rerank_server.ONNX_QUANTIZED_FILE_NAME


def test_load_onnx_model_exports_when_quantized_file_missing(
    monkeypatch: pytest.MonkeyPatch, tmp_path: Path
) -> None:
    monkeypatch.setattr(rerank_server, "RERANK_MODEL_DIR", str(tmp_path))

    export_mock = MagicMock()
    monkeypatch.setattr(rerank_server, "_export_quantized_onnx_model", export_mock)
    cross_encoder_mock = MagicMock()
    monkeypatch.setattr(rerank_server, "CrossEncoder", cross_encoder_mock)

    rerank_server._load_onnx_model()

    export_mock.assert_called_once_with(str(tmp_path))
    cross_encoder_mock.assert_called_once()


def test_load_model_sets_app_state_on_success(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setattr(rerank_server, "RERANK_BACKEND", "torch")
    fake_model = MagicMock()
    monkeypatch.setattr(rerank_server, "_load_torch_model", lambda: fake_model)
    fake_app = MagicMock()
    fake_app.state.model = None

    asyncio.run(rerank_server._load_model(fake_app))

    assert fake_app.state.model is fake_model


def test_load_model_leaves_state_none_on_failure(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setattr(rerank_server, "RERANK_BACKEND", "torch")

    def _boom() -> MagicMock:
        raise RuntimeError("model load failed")

    monkeypatch.setattr(rerank_server, "_load_torch_model", _boom)
    fake_app = MagicMock()
    fake_app.state.model = None

    asyncio.run(rerank_server._load_model(fake_app))

    assert fake_app.state.model is None
