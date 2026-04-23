"""RED test for ADR-000835 stage 3: classifier must not patch ``multi_class``.

The pre-retrain workaround in ``GenreClassifierService._ensure_model`` forced
``self.model.multi_class = 'ovr'`` because sklearn 1.7.x-trained models needed
the attribute when loaded under 1.8.0. After retraining on sklearn 1.8.0 this
patch is both unnecessary and actively harmful (it silently mutates a loaded
estimator and emits a ``DEBUG`` print to stdout).

This test pins the expectation that the patch is gone.
"""

from __future__ import annotations

from pathlib import Path
from unittest.mock import MagicMock

import joblib
import numpy as np
import pytest
from sklearn.linear_model import LogisticRegression


@pytest.fixture
def sklearn_1_8_model(tmp_path: Path) -> Path:
    """Fit a tiny LR on sklearn 1.8.0 and dump it; this model has no
    ``multi_class`` attribute (the 1.8.0 default path removed it)."""
    rng = np.random.default_rng(0)
    x = rng.normal(size=(50, 8))
    y = np.array([f"c{i}" for i in range(5) for _ in range(10)])
    clf = LogisticRegression(max_iter=200).fit(x, y)
    # Make sure the baseline assumption holds so the test asserts what we mean
    assert not hasattr(clf, "multi_class"), (
        "sklearn 1.8 LR still exposes multi_class; test is stale"
    )
    path = tmp_path / "genre_classifier.joblib"
    joblib.dump(clf, path)
    return path


@pytest.fixture
def mock_embedder() -> MagicMock:
    embedder = MagicMock()
    embedder.config.batch_size = 32
    return embedder


def test_ensure_model_does_not_force_multi_class_attribute(
    sklearn_1_8_model: Path, mock_embedder: MagicMock
) -> None:
    from recap_subworker.services.classifier import GenreClassifierService

    service = GenreClassifierService(str(sklearn_1_8_model), mock_embedder)
    service._ensure_model()

    assert service.model is not None
    assert not hasattr(service.model, "multi_class"), (
        "GenreClassifierService must not force-patch multi_class='ovr' onto "
        "sklearn 1.8-trained models. The workaround was only valid for "
        "pickles from 1.7.x; ADR-000835 stage 3 requires removal."
    )


def test_ensure_model_does_not_print_debug_to_stdout(
    sklearn_1_8_model: Path, mock_embedder: MagicMock, capsys: pytest.CaptureFixture
) -> None:
    from recap_subworker.services.classifier import GenreClassifierService

    service = GenreClassifierService(str(sklearn_1_8_model), mock_embedder)
    service._ensure_model()

    captured = capsys.readouterr()
    assert "DEBUG: Force patching" not in captured.out, (
        "The ad-hoc DEBUG stdout print must be removed — structured logs only."
    )
