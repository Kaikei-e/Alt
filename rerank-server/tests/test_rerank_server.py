"""Unit tests for rerank_server.py.

These tests never enter the app's lifespan context manager, so the real
CrossEncoder is never downloaded/loaded. `app.state.model` is set directly
to a lightweight fake before each test that needs a "loaded" model.
"""

from __future__ import annotations

import asyncio
import os
import threading
import time
from pathlib import Path
from unittest.mock import MagicMock

import pytest
from fastapi import HTTPException
from fastapi.testclient import TestClient

import rerank_server
from rerank_server import (
    DEFAULT_MODEL,
    MAX_CANDIDATE_LENGTH,
    MAX_CANDIDATES,
    InferenceDeadlineExceeded,
    app,
)

JAPANESE_MODEL = "hotchpotch/japanese-bge-reranker-v2-m3-v1"
RURI_MODEL = "cl-nagoya/ruri-v3-reranker-310m"

# Far enough ahead that _predict_sync's deadline check never fires.
NO_DEADLINE = float("inf")


def scoring_model(scores_by_candidate: dict[str, float]) -> MagicMock:
    """A CrossEncoder-shaped fake that scores by candidate text, not position.

    Scoring by position would silently pass even if the server mixed up which
    score belongs to which candidate, which is exactly what the chunked and
    length-reordered predict path has to get right.
    """
    model = MagicMock()
    model.predict.side_effect = lambda pairs, **_: [
        scores_by_candidate[candidate] for _, candidate in pairs
    ]
    return model


@pytest.fixture
def client() -> TestClient:
    # Plain (non-context-manager) TestClient never triggers `lifespan`,
    # so the heavy real model load is skipped entirely.
    return TestClient(app)


@pytest.fixture
def fake_model() -> MagicMock:
    return scoring_model({"a": 1.0, "b": 2.0, "c": 3.0, "low": 0.1, "mid": 0.5, "high": 0.9})


# --------------------------------------------------------------------------
# Model selection
# --------------------------------------------------------------------------


def test_resolve_model_defaults_to_bge(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.delenv("RERANK_MODEL", raising=False)

    assert rerank_server._resolve_model() == DEFAULT_MODEL


def test_resolve_model_reads_env(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setenv("RERANK_MODEL", JAPANESE_MODEL)

    assert rerank_server._resolve_model() == JAPANESE_MODEL


@pytest.mark.parametrize("value", ["", "no-slash", "../../etc/passwd", "org/na me", "a/b/c"])
def test_resolve_model_rejects_malformed_repo_id(
    monkeypatch: pytest.MonkeyPatch, value: str
) -> None:
    monkeypatch.setenv("RERANK_MODEL", value)

    with pytest.raises(ValueError, match="RERANK_MODEL"):
        rerank_server._resolve_model()


@pytest.mark.parametrize(
    "model",
    [
        DEFAULT_MODEL,
        JAPANESE_MODEL,
        "hotchpotch/japanese-reranker-base-v2",
        "hotchpotch/japanese-reranker-xsmall-v2",
        RURI_MODEL,
    ],
)
def test_known_good_models_are_valid_repo_ids(
    monkeypatch: pytest.MonkeyPatch, model: str
) -> None:
    assert model in rerank_server.SUPPORTED_MODELS
    monkeypatch.setenv("RERANK_MODEL", model)

    assert rerank_server._resolve_model() == model


def test_ruri_model_is_selectable_and_cached_separately(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    """The highest-JQaRA candidate must not collide with the default's export dir."""
    monkeypatch.delenv("RERANK_MODEL_DIR", raising=False)
    monkeypatch.delenv("RERANK_MODEL_CACHE_ROOT", raising=False)

    assert rerank_server._resolve_model_dir(RURI_MODEL) == "/models/ruri-v3-reranker-310m-onnx"


def test_ruri_model_is_accepted_end_to_end(
    client: TestClient, fake_model: MagicMock, monkeypatch: pytest.MonkeyPatch
) -> None:
    monkeypatch.setattr(rerank_server, "RERANK_MODEL", RURI_MODEL)
    app.state.model = fake_model

    resp = client.post(
        "/v1/rerank", json={"query": "q", "candidates": ["a"], "model": RURI_MODEL}
    )

    assert resp.status_code == 200
    assert resp.json()["model"] == RURI_MODEL


def test_resolve_model_dir_derives_the_default_cache_path(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    """First boot after the ruri adoption exports into its own directory."""
    monkeypatch.delenv("RERANK_MODEL_DIR", raising=False)
    monkeypatch.delenv("RERANK_MODEL_CACHE_ROOT", raising=False)

    assert rerank_server._resolve_model_dir(DEFAULT_MODEL) == "/models/ruri-v3-reranker-310m-onnx"


def test_resolve_model_dir_differs_per_model(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.delenv("RERANK_MODEL_DIR", raising=False)
    monkeypatch.delenv("RERANK_MODEL_CACHE_ROOT", raising=False)

    assert rerank_server._resolve_model_dir(JAPANESE_MODEL) != rerank_server._resolve_model_dir(
        DEFAULT_MODEL
    )


def test_resolve_model_dir_honours_explicit_env(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setenv("RERANK_MODEL_DIR", "/elsewhere/model")

    assert rerank_server._resolve_model_dir(JAPANESE_MODEL) == "/elsewhere/model"


def test_rerank_accepts_the_configured_model(
    client: TestClient, fake_model: MagicMock, monkeypatch: pytest.MonkeyPatch
) -> None:
    monkeypatch.setattr(rerank_server, "RERANK_MODEL", JAPANESE_MODEL)
    app.state.model = fake_model

    resp = client.post(
        "/v1/rerank",
        json={"query": "q", "candidates": ["a"], "model": JAPANESE_MODEL},
    )

    assert resp.status_code == 200
    assert resp.json()["model"] == JAPANESE_MODEL


def test_rerank_rejects_a_model_other_than_the_configured_one(
    client: TestClient, fake_model: MagicMock, monkeypatch: pytest.MonkeyPatch
) -> None:
    monkeypatch.setattr(rerank_server, "RERANK_MODEL", JAPANESE_MODEL)
    app.state.model = fake_model

    resp = client.post(
        "/v1/rerank",
        json={"query": "q", "candidates": ["a"], "model": DEFAULT_MODEL},
    )

    assert resp.status_code == 422
    assert JAPANESE_MODEL in resp.text
    fake_model.predict.assert_not_called()


def test_rerank_accepts_a_request_without_a_model_field(
    client: TestClient, fake_model: MagicMock
) -> None:
    app.state.model = fake_model

    resp = client.post("/v1/rerank", json={"query": "q", "candidates": ["a"]})

    assert resp.status_code == 200


def test_health_reports_the_configured_model(
    client: TestClient, fake_model: MagicMock, monkeypatch: pytest.MonkeyPatch
) -> None:
    monkeypatch.setattr(rerank_server, "RERANK_MODEL", JAPANESE_MODEL)
    app.state.model = fake_model

    resp = client.get("/health")

    assert resp.status_code == 200
    assert resp.json()["model"] == JAPANESE_MODEL


# --------------------------------------------------------------------------
# Request handling
# --------------------------------------------------------------------------


def test_rerank_without_loaded_model_returns_503(client: TestClient) -> None:
    app.state.model = None

    resp = client.post("/v1/rerank", json={"query": "q", "candidates": ["a", "b"]})

    assert resp.status_code == 503


def test_rerank_returns_results_sorted_by_score_desc(
    client: TestClient, fake_model: MagicMock
) -> None:
    app.state.model = fake_model

    resp = client.post("/v1/rerank", json={"query": "q", "candidates": ["low", "mid", "high"]})

    assert resp.status_code == 200
    body = resp.json()
    scores = [r["score"] for r in body["results"]]
    assert scores == sorted(scores, reverse=True)
    # "high" scores 0.9 and sits at original index 2
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


def test_rerank_rejects_too_many_candidates(client: TestClient, fake_model: MagicMock) -> None:
    app.state.model = fake_model

    resp = client.post(
        "/v1/rerank",
        json={"query": "q", "candidates": ["x"] * (MAX_CANDIDATES + 1)},
    )

    assert resp.status_code == 422
    # The rejection has to name the knob and the reason, not just "too long".
    assert "RERANK_MAX_CANDIDATES" in resp.text
    assert "RERANK_SERVER_TIMEOUT" in resp.text
    fake_model.predict.assert_not_called()


def test_rerank_accepts_a_candidate_list_at_the_limit(
    client: TestClient, monkeypatch: pytest.MonkeyPatch
) -> None:
    monkeypatch.setattr(rerank_server, "MAX_CANDIDATES", 40)
    candidates = [f"c{i}" for i in range(40)]
    app.state.model = scoring_model({c: float(i) for i, c in enumerate(candidates)})

    resp = client.post("/v1/rerank", json={"query": "q", "candidates": candidates})

    assert resp.status_code == 200
    assert len(resp.json()["results"]) == 40


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


def test_health_returns_200_when_model_loaded(client: TestClient, fake_model: MagicMock) -> None:
    app.state.model = fake_model

    resp = client.get("/health")

    assert resp.status_code == 200
    assert resp.json()["status"] == "ok"


# --------------------------------------------------------------------------
# Chunked inference
# --------------------------------------------------------------------------


def test_predict_sync_splits_pairs_into_chunks(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setattr(rerank_server, "RERANK_BACKEND", "torch")
    monkeypatch.setattr(rerank_server, "RERANK_CHUNK_SIZE", 2)
    candidates = [f"c{i}" for i in range(5)]
    model = scoring_model({c: float(i) for i, c in enumerate(candidates)})
    pairs = [("q", c) for c in candidates]

    rerank_server._predict_sync(model, pairs, NO_DEADLINE)

    assert model.predict.call_count == 3


def test_predict_sync_maps_scores_back_to_original_indices(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    """Chunking reorders pairs by length; the scores must land back where they started."""
    monkeypatch.setattr(rerank_server, "RERANK_BACKEND", "torch")
    monkeypatch.setattr(rerank_server, "RERANK_CHUNK_SIZE", 2)
    # Deliberately unsorted lengths so the internal reordering is non-trivial.
    candidates = ["short", "the longest candidate here", "mid length", "x", "medium size!"]
    expected = {c: float(i) for i, c in enumerate(candidates)}
    model = scoring_model(expected)
    pairs = [("q", c) for c in candidates]

    scores = rerank_server._predict_sync(model, pairs, NO_DEADLINE)

    assert scores == [expected[c] for c in candidates]


def test_predict_sync_scores_the_longest_candidates_first(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    """Longest-first keeps padding waste inside one chunk instead of every batch."""
    monkeypatch.setattr(rerank_server, "RERANK_BACKEND", "torch")
    monkeypatch.setattr(rerank_server, "RERANK_CHUNK_SIZE", 2)
    candidates = ["x", "xxxx", "xx", "xxxxxx"]
    model = scoring_model(dict.fromkeys(candidates, 0.0))
    pairs = [("q", c) for c in candidates]

    rerank_server._predict_sync(model, pairs, NO_DEADLINE)

    first_chunk = model.predict.call_args_list[0].args[0]
    assert [c for _, c in first_chunk] == ["xxxxxx", "xxxx"]


def test_predict_sync_passes_batch_size_on_onnx_backend(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setattr(rerank_server, "RERANK_BACKEND", "onnx")
    monkeypatch.setattr(rerank_server, "RERANK_BATCH_SIZE", 4)
    monkeypatch.setattr(rerank_server, "RERANK_CHUNK_SIZE", 4)
    model = scoring_model({"a": 1.0, "b": 2.0})
    pairs = [("q", "a"), ("q", "b")]

    rerank_server._predict_sync(model, pairs, NO_DEADLINE)

    model.predict.assert_called_once_with(pairs, batch_size=4)


def test_predict_sync_omits_batch_size_on_torch_backend(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setattr(rerank_server, "RERANK_BACKEND", "torch")
    monkeypatch.setattr(rerank_server, "RERANK_CHUNK_SIZE", 4)
    model = scoring_model({"a": 1.0, "b": 2.0})
    pairs = [("q", "a"), ("q", "b")]

    rerank_server._predict_sync(model, pairs, NO_DEADLINE)

    model.predict.assert_called_once_with(pairs)


def test_predict_sync_refuses_to_start_after_the_deadline(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    monkeypatch.setattr(rerank_server, "RERANK_BACKEND", "torch")
    model = scoring_model({"a": 1.0})

    with pytest.raises(InferenceDeadlineExceeded):
        rerank_server._predict_sync(model, [("q", "a")], time.monotonic() - 1)

    model.predict.assert_not_called()


def test_predict_sync_stops_at_the_next_chunk_boundary_after_the_deadline(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    """A request the client already abandoned must not keep scoring every chunk."""
    monkeypatch.setattr(rerank_server, "RERANK_BACKEND", "torch")
    monkeypatch.setattr(rerank_server, "RERANK_CHUNK_SIZE", 1)
    candidates = [f"c{i}" for i in range(10)]
    model = MagicMock()

    def slow_predict(pairs: list[tuple[str, str]]) -> list[float]:
        time.sleep(0.05)
        return [0.0] * len(pairs)

    model.predict.side_effect = slow_predict
    pairs = [("q", c) for c in candidates]

    with pytest.raises(InferenceDeadlineExceeded):
        rerank_server._predict_sync(model, pairs, time.monotonic() + 0.12)

    assert model.predict.call_count < len(candidates)


def test_rerank_returns_504_when_the_deadline_passes_mid_request(
    client: TestClient, monkeypatch: pytest.MonkeyPatch
) -> None:
    monkeypatch.setattr(rerank_server, "RERANK_CHUNK_SIZE", 1)
    monkeypatch.setattr(rerank_server, "SERVER_TIMEOUT_SECONDS", 0.12)

    def slow_predict(pairs: list[tuple[str, str]]) -> list[float]:
        time.sleep(0.05)
        return [0.0] * len(pairs)

    model = MagicMock()
    model.predict.side_effect = slow_predict
    app.state.model = model

    resp = client.post(
        "/v1/rerank", json={"query": "q", "candidates": [f"c{i}" for i in range(10)]}
    )

    assert resp.status_code == 504


# --------------------------------------------------------------------------
# Timeout coherence
# --------------------------------------------------------------------------


def test_default_server_timeout_stays_under_the_client_timeout() -> None:
    """rag-orchestrator's RERANK_TIMEOUT is 12s (compose/rag.yaml).

    The server must give up first, otherwise it keeps computing a result the
    client has already stopped waiting for.
    """
    client_timeout_seconds = 12.0

    assert rerank_server.SERVER_TIMEOUT_SECONDS < client_timeout_seconds


def test_default_max_candidates_fits_the_server_budget() -> None:
    """The early-reject bound and the budget must not contradict each other.

    ADR-000951's long-passage rate (~250ms/candidate at ~500 tokens) over the
    default 10s budget is where the 40 comes from.
    """
    assert rerank_server._default_max_candidates() == 40
    assert MAX_CANDIDATES == 40


def test_default_max_candidates_admits_the_client_side_cap() -> None:
    """rag-orchestrator caps its own list at 40 (defaultRerankMaxCandidates)."""
    client_side_cap = 40

    assert rerank_server._default_max_candidates() >= client_side_cap


@pytest.mark.parametrize(
    ("timeout_seconds", "budget_ms", "expected"),
    [
        (10.0, 250.0, 40),
        (20.0, 250.0, 80),  # a longer budget must raise the bound with it
        (10.0, 52.0, 192),  # production-traffic rate, per ADR-000951
        (0.01, 250.0, 1),  # never degenerates to zero
    ],
)
def test_default_max_candidates_is_derived_from_timeout_and_budget(
    monkeypatch: pytest.MonkeyPatch, timeout_seconds: float, budget_ms: float, expected: int
) -> None:
    monkeypatch.setattr(rerank_server, "SERVER_TIMEOUT_SECONDS", timeout_seconds)
    monkeypatch.setattr(rerank_server, "CANDIDATE_BUDGET_MS", budget_ms)

    assert rerank_server._default_max_candidates() == expected


def test_rerank_timeout_returns_504(client: TestClient, monkeypatch: pytest.MonkeyPatch) -> None:
    def slow_predict(pairs: list[tuple[str, str]]) -> list[float]:
        time.sleep(0.3)
        return [0.0] * len(pairs)

    model = MagicMock()
    model.predict.side_effect = slow_predict
    app.state.model = model
    monkeypatch.setattr(rerank_server, "SERVER_TIMEOUT_SECONDS", 0.05)

    resp = client.post("/v1/rerank", json={"query": "q", "candidates": ["a"]})

    assert resp.status_code == 504


def test_rerank_timeout_releases_semaphore(
    client: TestClient, monkeypatch: pytest.MonkeyPatch
) -> None:
    def slow_predict(pairs: list[tuple[str, str]]) -> list[float]:
        time.sleep(0.3)
        return [0.0] * len(pairs)

    model = MagicMock()
    model.predict.side_effect = slow_predict
    app.state.model = model
    monkeypatch.setattr(rerank_server, "SERVER_TIMEOUT_SECONDS", 0.05)

    client.post("/v1/rerank", json={"query": "q", "candidates": ["a"]})

    assert rerank_server._inference_semaphore.locked() is False


def test_rerank_timeout_does_not_allow_concurrent_model_access(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    """Regression test for the timeout/thread-leak race (rerank_server.py).

    asyncio.timeout only cancels the *awaiting* coroutine, not the
    underlying thread pool job -- concurrent.futures.Future.cancel() is a
    no-op once execution has started. If inference is offloaded via
    asyncio.to_thread (the shared default executor), a request that times
    out leaves an orphaned thread still running model.predict() even after
    the semaphore has been released, so a subsequent request's predict()
    call can start concurrently on the same (non-thread-safe) CrossEncoder
    instance. This must never happen.
    """
    state = {"active": 0, "observed_concurrent": False}
    state_lock = threading.Lock()

    def slow_predict(pairs: list[tuple[str, str]]) -> list[float]:
        with state_lock:
            state["active"] += 1
            if state["active"] > 1:
                state["observed_concurrent"] = True
        time.sleep(0.3)
        with state_lock:
            state["active"] -= 1
        return [0.0] * len(pairs)

    model = MagicMock()
    model.predict.side_effect = slow_predict
    fake_request = MagicMock()
    fake_request.app.state.model = model
    monkeypatch.setattr(rerank_server, "SERVER_TIMEOUT_SECONDS", 0.05)
    req = rerank_server.RerankRequest(query="q", candidates=["a"])

    async def _call() -> int:
        try:
            await rerank_server.rerank(req, fake_request)
        except HTTPException as exc:
            return exc.status_code
        return 200

    async def _run() -> tuple[int, int]:
        task1 = asyncio.create_task(_call())
        await asyncio.sleep(0.15)
        task2 = asyncio.create_task(_call())
        return await asyncio.gather(task1, task2)

    statuses = asyncio.run(_run())

    assert 504 in statuses
    assert state["observed_concurrent"] is False


# --------------------------------------------------------------------------
# Model loading
# --------------------------------------------------------------------------


def test_load_onnx_model_skips_export_when_quantized_file_present(
    monkeypatch: pytest.MonkeyPatch, tmp_path: Path
) -> None:
    onnx_dir = tmp_path / "onnx"
    onnx_dir.mkdir()
    (onnx_dir / rerank_server.ONNX_QUANTIZED_FILE_NAME).touch()
    monkeypatch.setattr(rerank_server, "RERANK_MODEL_DIR", str(tmp_path))
    monkeypatch.setattr(rerank_server, "_purge_stray_ort_temp_dirs", lambda: None)

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
    monkeypatch.setattr(rerank_server, "_purge_stray_ort_temp_dirs", lambda: None)

    export_mock = MagicMock()
    monkeypatch.setattr(rerank_server, "_export_quantized_onnx_model", export_mock)
    cross_encoder_mock = MagicMock()
    monkeypatch.setattr(rerank_server, "CrossEncoder", cross_encoder_mock)

    rerank_server._load_onnx_model()

    export_mock.assert_called_once_with(str(tmp_path))
    cross_encoder_mock.assert_called_once()


def test_load_onnx_model_exports_when_quantized_path_is_a_directory(
    monkeypatch: pytest.MonkeyPatch, tmp_path: Path
) -> None:
    """A directory of that name must not read as a usable export."""
    (tmp_path / "onnx" / rerank_server.ONNX_QUANTIZED_FILE_NAME).mkdir(parents=True)
    monkeypatch.setattr(rerank_server, "RERANK_MODEL_DIR", str(tmp_path))
    monkeypatch.setattr(rerank_server, "_purge_stray_ort_temp_dirs", lambda: None)

    export_mock = MagicMock()
    monkeypatch.setattr(rerank_server, "_export_quantized_onnx_model", export_mock)
    monkeypatch.setattr(rerank_server, "CrossEncoder", MagicMock())

    rerank_server._load_onnx_model()

    export_mock.assert_called_once_with(str(tmp_path))


def test_reset_export_tmp_wipes_previous_scratch(
    monkeypatch: pytest.MonkeyPatch, tmp_path: Path
) -> None:
    monkeypatch.setattr(rerank_server, "RERANK_MODEL_DIR", str(tmp_path))
    leftover = tmp_path / ".export-tmp" / "ort.quant.old"
    leftover.mkdir(parents=True)
    (leftover / "blob").write_bytes(b"x")

    root = rerank_server._reset_export_tmp()

    assert root == tmp_path / ".export-tmp"
    assert root.is_dir()
    assert list(root.iterdir()) == []


def test_purge_stray_ort_temp_dirs_only_removes_ort_quant(
    monkeypatch: pytest.MonkeyPatch, tmp_path: Path
) -> None:
    monkeypatch.setattr(rerank_server.tempfile, "gettempdir", lambda: str(tmp_path))
    stray = tmp_path / "ort.quant.leftover"
    stray.mkdir()
    (stray / "blob").write_bytes(b"x")
    keep = tmp_path / "unrelated"
    keep.mkdir()

    rerank_server._purge_stray_ort_temp_dirs()

    assert not stray.exists()
    assert keep.is_dir()


def test_load_onnx_model_purges_export_scratch_even_when_file_present(
    monkeypatch: pytest.MonkeyPatch, tmp_path: Path
) -> None:
    onnx_dir = tmp_path / "onnx"
    onnx_dir.mkdir()
    (onnx_dir / rerank_server.ONNX_QUANTIZED_FILE_NAME).touch()
    leftover = tmp_path / ".export-tmp" / "ort.quant.old"
    leftover.mkdir(parents=True)
    proc_tmp = tmp_path / "proc-tmp"
    proc_tmp.mkdir()
    monkeypatch.setattr(rerank_server, "RERANK_MODEL_DIR", str(tmp_path))
    monkeypatch.setattr(rerank_server.tempfile, "gettempdir", lambda: str(proc_tmp))
    monkeypatch.setattr(rerank_server, "_export_quantized_onnx_model", MagicMock())
    monkeypatch.setattr(rerank_server, "CrossEncoder", MagicMock())

    rerank_server._load_onnx_model()

    assert not leftover.exists()
    assert (tmp_path / ".export-tmp").is_dir()


def test_onnx_export_tmpdir_sets_and_restores(
    monkeypatch: pytest.MonkeyPatch, tmp_path: Path
) -> None:
    monkeypatch.setattr(rerank_server, "RERANK_MODEL_DIR", str(tmp_path))
    monkeypatch.delenv("TMPDIR", raising=False)

    with rerank_server._onnx_export_tmpdir():
        assert os.environ["TMPDIR"] == str(tmp_path / ".export-tmp")
        (Path(os.environ["TMPDIR"]) / "ort.quant.live").mkdir()

    assert "TMPDIR" not in os.environ
    assert list((tmp_path / ".export-tmp").iterdir()) == []


def test_load_model_sets_app_state_on_success(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setattr(rerank_server, "RERANK_BACKEND", "torch")
    model = MagicMock()
    monkeypatch.setattr(rerank_server, "_load_torch_model", lambda: model)
    fake_app = MagicMock()
    fake_app.state.model = None

    asyncio.run(rerank_server._load_model(fake_app))

    assert fake_app.state.model is model


def test_load_model_leaves_state_none_on_failure(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setattr(rerank_server, "RERANK_BACKEND", "torch")

    def _boom() -> MagicMock:
        raise RuntimeError("model load failed")

    monkeypatch.setattr(rerank_server, "_load_torch_model", _boom)
    fake_app = MagicMock()
    fake_app.state.model = None

    asyncio.run(rerank_server._load_model(fake_app))

    assert fake_app.state.model is None
