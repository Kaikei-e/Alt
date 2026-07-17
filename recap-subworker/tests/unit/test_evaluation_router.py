"""Regression tests for /v1/evaluation/genres router bugs.

1. NameError masking: `logger` used to be defined only inside the first
   `except` block (save_genre_evaluation failure), so if
   `insert_system_metrics` failed *after* `save_genre_evaluation` succeeded,
   referencing `logger` raised NameError/UnboundLocalError, masking the real
   DB failure behind an opaque 500.
2. Blocking evaluate(): `service.evaluate()` (predict/bootstrap/CV) must be
   offloaded via asyncio.to_thread, and the default (no per-request weights
   override) EvaluationService instance must be reused (container
   singleton) across requests instead of reloading an Embedder + 3
   classifiers from disk on every call.

Reference: docs/plan/20260706largerepocodereview.md
(recap-subworker/recap_subworker/app/routers/evaluation.py HIGH findings).
"""

from __future__ import annotations

from pathlib import Path
from unittest.mock import MagicMock
from uuid import uuid4

import pytest

from recap_subworker.app.routers import evaluation as evaluation_router
from recap_subworker.infra.config import Settings


def _minimal_results() -> dict:
    return {
        "accuracy": 0.9,
        "accuracy_ci": {"point": 0.9, "lower": 0.8, "upper": 1.0, "width": 0.2},
        "macro_precision": 0.8,
        "macro_recall": 0.8,
        "macro_f1": 0.8,
        "micro_precision": 0.8,
        "micro_recall": 0.8,
        "micro_f1": 0.8,
        "per_genre_metrics": {},
        "confusion_matrix": {},
        "total_samples": 10,
        "total_tp": 5,
        "total_fp": 1,
        "total_fn": 1,
    }


class _FakeEvaluationService:
    def evaluate(self, golden_data_path, language=None, **kwargs):
        return _minimal_results()


class _SucceedThenFailDAO:
    """Mirrors the real bug scenario: save_genre_evaluation succeeds,
    insert_system_metrics fails afterwards."""

    def __init__(self, session):
        self.session = session

    async def save_genre_evaluation(self, **kwargs):
        return uuid4()

    async def insert_system_metrics(self, **kwargs):
        raise RuntimeError("db unavailable")


@pytest.fixture
def fake_container():
    container = MagicMock()
    container.evaluation_service = _FakeEvaluationService()
    return container


@pytest.fixture
def fake_settings():
    return Settings(model_id="fake", allow_embedding_drift=True)


@pytest.fixture
def stub_existing_paths(monkeypatch):
    """Bypass filesystem checks: require_existing_path returns a Path as-is."""

    def _stub(user_path: str, base_dirs=None):
        return Path(user_path)

    monkeypatch.setattr(evaluation_router, "require_existing_path", _stub)


@pytest.mark.asyncio
async def test_evaluate_genres_does_not_mask_db_failure_with_nameerror(
    monkeypatch, fake_container, fake_settings, stub_existing_paths
) -> None:
    """insert_system_metrics failing *after* a successful
    save_genre_evaluation must not raise NameError/UnboundLocalError for
    `logger`. It should be logged and the evaluation response still
    returned (matching the code's own stated intent: DB保存に失敗しても
    評価結果は返す)."""
    monkeypatch.setattr(evaluation_router, "SubworkerDAO", _SucceedThenFailDAO)

    request = evaluation_router.EvaluateRequest(golden_data_path=None, save_to_db=True)

    response = await evaluation_router.evaluate_genres(
        request=request,
        language=None,
        settings=fake_settings,
        session=object(),
        container=fake_container,
    )

    assert response.accuracy == pytest.approx(0.9)


@pytest.mark.asyncio
async def test_evaluate_genres_reuses_container_singleton_and_offloads(
    monkeypatch, fake_container, fake_settings, stub_existing_paths
) -> None:
    """`/v1/evaluation/genres` must reuse container.evaluation_service (no
    per-request EvaluationService() construction for the default weights
    path) and must call service.evaluate() via asyncio.to_thread rather than
    synchronously inline, so a CPU-heavy evaluate() run does not block the
    event loop."""
    calls = []

    async def fake_to_thread(func, *args, **kwargs):
        calls.append((func, args, kwargs))
        return func(*args, **kwargs)

    monkeypatch.setattr(evaluation_router.asyncio, "to_thread", fake_to_thread)

    request = evaluation_router.EvaluateRequest(golden_data_path=None, save_to_db=False)

    response = await evaluation_router.evaluate_genres(
        request=request,
        language=None,
        settings=fake_settings,
        session=object(),
        container=fake_container,
    )

    assert response.accuracy == pytest.approx(0.9)
    assert len(calls) == 1
    assert calls[0][0] == fake_container.evaluation_service.evaluate


@pytest.mark.asyncio
async def test_evaluate_genres_builds_adhoc_service_for_custom_weights_path(
    monkeypatch, fake_container, fake_settings, stub_existing_paths
) -> None:
    """A request-supplied weights_path override must not silently reuse the
    settings-derived singleton (it needs different weights loaded)."""
    created_kwargs = {}

    class _FakeCustomService:
        def __init__(self, **kwargs):
            created_kwargs.update(kwargs)

        def evaluate(self, golden_data_path, language=None, **kwargs):
            return _minimal_results()

    monkeypatch.setattr(evaluation_router, "EvaluationService", _FakeCustomService)

    request = evaluation_router.EvaluateRequest(
        golden_data_path=None,
        weights_path="/app/data/custom/model.joblib",
        save_to_db=False,
    )

    response = await evaluation_router.evaluate_genres(
        request=request,
        language=None,
        settings=fake_settings,
        session=object(),
        container=fake_container,
    )

    assert response.accuracy == pytest.approx(0.9)
    assert created_kwargs["weights_path"] == str(Path("/app/data/custom/model.joblib"))
