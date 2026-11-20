"""Characterization tests for evaluate_genres path validation branches."""

from __future__ import annotations

from pathlib import Path
from unittest.mock import MagicMock

import pytest
from fastapi import HTTPException

from recap_subworker.app.routers import evaluation as evaluation_router
from recap_subworker.infra.config import Settings

HTTP_BAD_REQUEST = 400
HTTP_NOT_FOUND = 404


def _fake_settings() -> Settings:
    return Settings(model_id="fake", allow_embedding_drift=True)


@pytest.mark.asyncio
async def test_evaluate_genres_rejects_non_json_golden_path(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    monkeypatch.setattr(evaluation_router, "require_existing_path", lambda p, _dirs: Path(p))
    request = evaluation_router.EvaluateRequest(
        golden_data_path="/app/data/golden.txt",
        save_to_db=False,
    )
    with pytest.raises(HTTPException) as exc_info:
        await evaluation_router.evaluate_genres(
            request=request,
            settings=_fake_settings(),
            container=MagicMock(),
            session=MagicMock(),
        )
    assert exc_info.value.status_code == HTTP_BAD_REQUEST
    assert ".json or .jsonl" in str(exc_info.value.detail)


@pytest.mark.asyncio
async def test_evaluate_genres_returns_404_when_golden_missing(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    def _raise_missing(_p: str, _dirs: object) -> Path:
        raise FileNotFoundError

    monkeypatch.setattr(evaluation_router, "require_existing_path", _raise_missing)
    request = evaluation_router.EvaluateRequest(
        golden_data_path="/app/data/missing.json",
        save_to_db=False,
    )
    with pytest.raises(HTTPException) as exc_info:
        await evaluation_router.evaluate_genres(
            request=request,
            settings=_fake_settings(),
            container=MagicMock(),
            session=MagicMock(),
        )
    assert exc_info.value.status_code == HTTP_NOT_FOUND
    assert "Golden dataset file not found" in str(exc_info.value.detail)


@pytest.mark.asyncio
async def test_evaluate_genres_rejects_non_joblib_weights_path(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    monkeypatch.setattr(evaluation_router, "require_existing_path", lambda p, _dirs: Path(p))
    request = evaluation_router.EvaluateRequest(
        golden_data_path="/app/data/golden.json",
        weights_path="/app/data/weights.pkl",
        save_to_db=False,
    )
    with pytest.raises(HTTPException) as exc_info:
        await evaluation_router.evaluate_genres(
            request=request,
            settings=_fake_settings(),
            container=MagicMock(),
            session=MagicMock(),
        )
    assert exc_info.value.status_code == HTTP_BAD_REQUEST
    assert ".joblib" in str(exc_info.value.detail)


@pytest.mark.asyncio
async def test_evaluate_genres_returns_404_when_weights_missing(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    def _golden_ok_weights_missing(raw: str, _dirs: object) -> Path:
        if raw.endswith(".joblib"):
            raise FileNotFoundError
        return Path(raw)

    monkeypatch.setattr(evaluation_router, "require_existing_path", _golden_ok_weights_missing)
    request = evaluation_router.EvaluateRequest(
        golden_data_path="/app/data/golden.json",
        weights_path="/app/data/missing.joblib",
        save_to_db=False,
    )
    with pytest.raises(HTTPException) as exc_info:
        await evaluation_router.evaluate_genres(
            request=request,
            settings=_fake_settings(),
            container=MagicMock(),
            session=MagicMock(),
        )
    assert exc_info.value.status_code == HTTP_NOT_FOUND
    assert "Weights file not found" in str(exc_info.value.detail)
