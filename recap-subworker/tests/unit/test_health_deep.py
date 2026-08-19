"""Deep health — classifier artefact readiness (PM-2026-036 class)."""

from __future__ import annotations

from pathlib import Path
from types import SimpleNamespace

import pytest
from fastapi import FastAPI
from fastapi.testclient import TestClient

from recap_subworker.app.deps import get_embedder_dep, get_settings_dep
from recap_subworker.app.routers.health import router
from recap_subworker.infra.artefact_health import assert_classifier_artefacts, assert_paths_ready


def test_assert_paths_ready_accepts_files(tmp_path: Path) -> None:
    artefact = tmp_path / "genre_classifier_ja.joblib"
    artefact.write_bytes(b"ok")
    assert_paths_ready([str(artefact)], must_be_file=True)


def test_assert_paths_ready_rejects_directory(tmp_path: Path) -> None:
    stub = tmp_path / "genre_classifier.joblib"
    stub.mkdir()
    try:
        assert_paths_ready([str(stub)], must_be_file=True)
    except RuntimeError as exc:
        assert "unavailable" in str(exc)
        assert str(stub) not in str(exc)
    else:
        raise AssertionError("expected RuntimeError")


def test_assert_paths_ready_rejects_missing(tmp_path: Path) -> None:
    missing = tmp_path / "never.joblib"
    try:
        assert_paths_ready([str(missing)], must_be_file=True)
    except RuntimeError:
        return
    raise AssertionError("expected RuntimeError")


def test_assert_classifier_artefacts_joblib_ok(tmp_path: Path) -> None:
    model = tmp_path / "model.joblib"
    model.write_bytes(b"ok")
    settings = SimpleNamespace(
        classification_backend="joblib",
        genre_classifier_model_path=str(model),
        genre_classifier_model_path_ja="",
        genre_classifier_model_path_en="",
        tfidf_vectorizer_path_ja="",
        tfidf_vectorizer_path_en="",
        genre_thresholds_path_ja="",
        genre_thresholds_path_en="",
    )
    assert_classifier_artefacts(settings)  # type: ignore[arg-type]


def _app_with_settings(settings: object) -> FastAPI:
    app = FastAPI()
    app.include_router(router)
    embedder = SimpleNamespace(config=SimpleNamespace(model_id="hash-fake", backend="hash"))
    app.dependency_overrides[get_settings_dep] = lambda: settings
    app.dependency_overrides[get_embedder_dep] = lambda: embedder
    return app


def _client_with_settings(settings: object) -> TestClient:
    return TestClient(_app_with_settings(settings))


def test_deep_health_fails_when_artefact_is_directory(tmp_path: Path) -> None:
    stub = tmp_path / "genre_classifier_ja.joblib"
    stub.mkdir()
    settings = SimpleNamespace(
        classification_backend="joblib",
        genre_classifier_model_path=str(stub),
        genre_classifier_model_path_ja="",
        genre_classifier_model_path_en="",
        tfidf_vectorizer_path_ja="",
        tfidf_vectorizer_path_en="",
        genre_thresholds_path_ja="",
        genre_thresholds_path_en="",
    )
    client = _client_with_settings(settings)
    response = client.get("/health/deep")
    assert response.status_code == 503
    body = response.json()
    assert body["status"] == "fail"
    assert body["service"] == "recap-subworker"
    assert body["checks"][0]["name"] == "classifier_artefacts"
    assert body["checks"][0]["critical"] is True
    assert str(stub) not in response.text


def test_deep_health_passes_when_artefact_is_file(tmp_path: Path) -> None:
    artefact = tmp_path / "genre_classifier_ja.joblib"
    artefact.write_bytes(b"ok")
    settings = SimpleNamespace(
        classification_backend="joblib",
        genre_classifier_model_path=str(artefact),
        genre_classifier_model_path_ja="",
        genre_classifier_model_path_en="",
        tfidf_vectorizer_path_ja="",
        tfidf_vectorizer_path_en="",
        genre_thresholds_path_ja="",
        genre_thresholds_path_en="",
    )
    client = _client_with_settings(settings)
    response = client.get("/health/deep")
    assert response.status_code == 200
    body = response.json()
    assert body["status"] == "pass"


def test_cheap_health_stays_ok_when_deep_fails(tmp_path: Path) -> None:
    stub = tmp_path / "genre_classifier_ja.joblib"
    stub.mkdir()
    settings = SimpleNamespace(
        classification_backend="joblib",
        genre_classifier_model_path=str(stub),
        genre_classifier_model_path_ja="",
        genre_classifier_model_path_en="",
        tfidf_vectorizer_path_ja="",
        tfidf_vectorizer_path_en="",
        genre_thresholds_path_ja="",
        genre_thresholds_path_en="",
    )
    client = _client_with_settings(settings)
    cheap = client.get("/health")
    deep = client.get("/health/deep")
    assert cheap.status_code == 200
    assert cheap.json()["status"] == "ok"
    assert deep.status_code == 503


def test_deep_health_runner_is_app_scoped_not_per_request(tmp_path: Path) -> None:
    artefact = tmp_path / "genre_classifier_ja.joblib"
    artefact.write_bytes(b"ok")
    settings = SimpleNamespace(
        classification_backend="joblib",
        genre_classifier_model_path=str(artefact),
        genre_classifier_model_path_ja="",
        genre_classifier_model_path_en="",
        tfidf_vectorizer_path_ja="",
        tfidf_vectorizer_path_en="",
        genre_thresholds_path_ja="",
        genre_thresholds_path_en="",
    )
    app = _app_with_settings(settings)
    client = TestClient(app)
    first = client.get("/health/deep")
    second = client.get("/health/deep")
    assert first.status_code == 200
    assert second.status_code == 200
    runner = getattr(app.state, "deep_health_runner", None)
    assert runner is not None
    assert second.json()["cached"] is True


@pytest.mark.asyncio
async def test_concurrent_deep_health_shares_one_probe(tmp_path: Path) -> None:
    import asyncio

    import httpx

    from recap_subworker.infra.health_deep import Check, DeepHealthRunner

    artefact = tmp_path / "genre_classifier_ja.joblib"
    artefact.write_bytes(b"ok")
    settings = SimpleNamespace(
        classification_backend="joblib",
        genre_classifier_model_path=str(artefact),
        genre_classifier_model_path_ja="",
        genre_classifier_model_path_en="",
        tfidf_vectorizer_path_ja="",
        tfidf_vectorizer_path_en="",
        genre_thresholds_path_ja="",
        genre_thresholds_path_en="",
    )
    calls = 0
    started = asyncio.Event()
    release = asyncio.Event()

    async def probe() -> None:
        nonlocal calls
        calls += 1
        started.set()
        await release.wait()

    app = _app_with_settings(settings)
    app.state.deep_health_runner = DeepHealthRunner(
        "recap-subworker",
        [Check(name="classifier_artefacts", critical=True, probe=probe)],
        cache_ttl_s=2.0,
        per_check_s=0.5,
        budget_s=0.8,
    )
    transport = httpx.ASGITransport(app=app)
    async with httpx.AsyncClient(transport=transport, base_url="http://test") as ac:
        first = asyncio.create_task(ac.get("/health/deep"))
        await asyncio.wait_for(started.wait(), timeout=1.0)
        rest = [asyncio.create_task(ac.get("/health/deep")) for _ in range(5)]
        release.set()
        responses = [await first, *(await asyncio.gather(*rest))]
    assert calls == 1
    assert all(r.status_code == 200 for r in responses)
    assert all(r.json()["status"] == "pass" for r in responses)


@pytest.mark.asyncio
async def test_cancelled_deep_health_request_does_not_poison_runner(
    tmp_path: Path,
) -> None:
    import asyncio

    import httpx

    from recap_subworker.infra.health_deep import Check, DeepHealthRunner

    artefact = tmp_path / "genre_classifier_ja.joblib"
    artefact.write_bytes(b"ok")
    settings = SimpleNamespace(
        classification_backend="joblib",
        genre_classifier_model_path=str(artefact),
        genre_classifier_model_path_ja="",
        genre_classifier_model_path_en="",
        tfidf_vectorizer_path_ja="",
        tfidf_vectorizer_path_en="",
        genre_thresholds_path_ja="",
        genre_thresholds_path_en="",
    )
    started = asyncio.Event()
    release = asyncio.Event()
    calls = 0

    async def probe() -> None:
        nonlocal calls
        calls += 1
        started.set()
        await release.wait()

    app = _app_with_settings(settings)
    app.state.deep_health_runner = DeepHealthRunner(
        "recap-subworker",
        [Check(name="classifier_artefacts", critical=True, probe=probe)],
        cache_ttl_s=2.0,
        per_check_s=0.5,
        budget_s=0.8,
    )
    client = TestClient(app)
    transport = httpx.ASGITransport(app=app)
    async with httpx.AsyncClient(transport=transport, base_url="http://test") as ac:
        waiter = asyncio.create_task(ac.get("/health/deep"))
        await asyncio.wait_for(started.wait(), timeout=1.0)
        survivor = asyncio.create_task(ac.get("/health/deep"))
        waiter.cancel()
        with pytest.raises(asyncio.CancelledError):
            await waiter
        release.set()
        response = await survivor
    assert calls == 1
    assert response.status_code == 200
    assert response.json()["status"] == "pass"
    follow = client.get("/health/deep")
    assert follow.status_code == 200
    assert follow.json()["cached"] is True
