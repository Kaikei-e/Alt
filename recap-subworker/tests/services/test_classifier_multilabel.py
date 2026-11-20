import json
from pathlib import Path
from unittest.mock import MagicMock

import joblib
import numpy as np
import pytest

from recap_subworker.services.classifier import GenreClassifierService


@pytest.fixture
def mock_embedder() -> MagicMock:
    embedder = MagicMock()
    # Mock return for encode: 1 text -> 1 embedding of size 2
    embedder.encode.return_value = np.array([[0.1, 0.2]])
    embedder.config.batch_size = 32
    return embedder


@pytest.fixture
def mock_model() -> MagicMock:
    model = MagicMock()
    model.classes_ = np.array(["tech", "politics", "other"])
    model.predict_proba.return_value = np.array([[0.45, 0.45, 0.1]])
    return model


@pytest.fixture
def genre_classifier(
    mock_embedder: MagicMock, mock_model: MagicMock, tmp_path: Path
) -> GenreClassifierService:
    # Create fake model file
    model_path = tmp_path / "model.joblib"
    model_path.touch()

    service = GenreClassifierService(str(model_path), mock_embedder)
    service.model = mock_model
    service.tfidf = None
    service.thresholds = {"tech": 0.4, "politics": 0.4, "other": 0.5}
    service.current_thresholds = service.thresholds.copy()

    # Mock _ensure_model to do nothing (since we set up manually)
    service._ensure_model = MagicMock()

    return service


def test_predict_batch_single_label(genre_classifier: GenreClassifierService) -> None:
    probs = [[0.5, 0.4, 0.1]]
    assert isinstance(genre_classifier.model, MagicMock)
    genre_classifier.model.predict_proba.return_value = np.array(probs)

    results = genre_classifier.predict_batch(["test text"], multi_label=False)

    assert len(results) == 1
    res = results[0]
    assert res["top_genre"] == "tech"
    assert res["confidence"] == probs[0][0]
    assert len(res["candidates"]) >= 1
    assert res["candidates"][0]["genre"] == "tech"


def test_predict_batch_multi_label(genre_classifier: GenreClassifierService) -> None:
    # tech=0.5 (thresh=0.4) -> PASS
    # politics=0.45 (thresh=0.4) -> PASS
    # other=0.05 (thresh=0.5) -> FAIL
    assert isinstance(genre_classifier.model, MagicMock)
    genre_classifier.model.predict_proba.return_value = np.array([[0.5, 0.45, 0.05]])

    results = genre_classifier.predict_batch(["test text"], multi_label=True, top_k=5)

    assert len(results) == 1
    res = results[0]
    assert res["top_genre"] == "tech"  # First sorted

    # Check candidates structure for multi-label
    candidates = res["candidates"]
    expected_passing = 2
    assert len(candidates) == expected_passing
    genres = {c["genre"] for c in candidates}
    assert "tech" in genres
    assert "politics" in genres
    assert "other" not in genres


def test_predict_batch_threshold_overrides_passed_to_ensure(
    genre_classifier: GenreClassifierService,
) -> None:
    # This test verifies that we can pass thresholds to predict_batch
    # In our mocked service, we mocked _ensure_model, so we verify it was called with overrides
    overrides = {"tech": 0.9}
    genre_classifier.predict_batch(["test"], threshold_overrides=overrides)
    assert isinstance(genre_classifier._ensure_model, MagicMock)
    genre_classifier._ensure_model.assert_called_with(overrides)


# We should also test that _ensure_model actually updates current_thresholds.
# Uses a real joblib round-trip on tmp_path because mocking Path.open /
# io.open is fragile across Python versions (see ADR-000835 stage 3 rewrite).
def test_ensure_model_updates_thresholds(mock_embedder: MagicMock, tmp_path: Path) -> None:
    from sklearn.linear_model import (  # noqa: PLC0415 - defer heavy sklearn import to test
        LogisticRegression,
    )

    rng = np.random.default_rng(0)
    x = rng.normal(size=(30, 4))
    y = np.array([f"c{i}" for i in range(3) for _ in range(10)])
    clf = LogisticRegression(max_iter=200).fit(x, y)

    model_path = tmp_path / "genre_classifier_ja.joblib"
    joblib.dump(clf, model_path)
    thresholds_path = tmp_path / "genre_thresholds_ja.json"
    initial_tech = 0.5
    thresholds_path.write_text(json.dumps({"tech": initial_tech}))

    service = GenreClassifierService(
        str(model_path),
        mock_embedder,
        thresholds_path=str(thresholds_path),
    )

    override_tech = 0.8
    override_new = 0.3
    service._ensure_model(threshold_overrides={"tech": override_tech, "new": override_new})

    assert service.current_thresholds["tech"] == override_tech
    assert service.current_thresholds["new"] == override_new
    assert service.thresholds is not None
    assert service.thresholds["tech"] == initial_tech


def test_predict_batch_below_threshold_flag_single_label(
    genre_classifier: GenreClassifierService,
) -> None:
    """Test that below_threshold flag is set when no genre passes threshold."""
    # All scores below their thresholds
    probs = [[0.3, 0.3, 0.4]]
    assert isinstance(genre_classifier.model, MagicMock)
    genre_classifier.model.predict_proba.return_value = np.array(probs)

    results = genre_classifier.predict_batch(["test text"], multi_label=False)

    assert len(results) == 1
    res = results[0]
    # Should use highest score (other=0.4)
    assert res["top_genre"] == "other"
    assert res["confidence"] == probs[0][2]
    assert res["below_threshold"] is True
    assert res["candidates"] == []  # No candidates passed threshold


def test_predict_batch_below_threshold_flag_multi_label(
    genre_classifier: GenreClassifierService,
) -> None:
    """Test below_threshold in multi-label mode when nothing passes."""
    probs = [[0.3, 0.3, 0.4]]
    assert isinstance(genre_classifier.model, MagicMock)
    genre_classifier.model.predict_proba.return_value = np.array(probs)

    results = genre_classifier.predict_batch(["test text"], multi_label=True)

    assert len(results) == 1
    res = results[0]
    # Should use highest score (other=0.4)
    assert res["top_genre"] == "other"
    assert res["confidence"] == probs[0][2]
    assert res["below_threshold"] is True
    # In multi-label mode, should include fallback as a candidate
    assert len(res["candidates"]) == 1
    assert res["candidates"][0]["genre"] == "other"


def test_predict_batch_above_threshold_flag(genre_classifier: GenreClassifierService) -> None:
    """Test that below_threshold is False when a genre passes threshold."""
    probs = [[0.5, 0.3, 0.2]]
    assert isinstance(genre_classifier.model, MagicMock)
    genre_classifier.model.predict_proba.return_value = np.array(probs)

    results = genre_classifier.predict_batch(["test text"], multi_label=False)

    assert len(results) == 1
    res = results[0]
    assert res["top_genre"] == "tech"
    assert res["confidence"] == probs[0][0]
    assert res["below_threshold"] is False
