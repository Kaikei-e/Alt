import pytest
from unittest.mock import MagicMock, patch
import numpy as np
from recap_subworker.services.classifier import GenreClassifierService

@pytest.fixture
def mock_embedder():
    embedder = MagicMock()
    # Mock return for encode: 1 text -> 1 embedding of size 2
    embedder.encode.return_value = np.array([[0.1, 0.2]])
    embedder.config.batch_size = 32
    return embedder

@pytest.fixture
def mock_model():
    model = MagicMock()
    model.classes_ = np.array(["tech", "politics", "other"])
    # Mock predict_proba: 1 sample, 3 classes
    # [tech=0.8, politics=0.6, other=0.1]
    # Note: sum > 1 is possible if these were independent classifiers, but for softmax they sum to 1.
    # However, for multi-label simulation or just generic confidence testing, we can define arbitrary probs if we mock it.
    # Let's assume standard probabilities for now, but we want to test multi-label "threshold passing".
    # If we use OneVsRest or independent logits, they don't sum to 1.
    # If using Softmax (LogisticRegression standard), they sum to 1.
    # But our code logic just checks `if score >= threshold`.
    # Let's use values that sum to 1 for realism if checking standard LR, but for multi-label logic,
    # we might want to check if multiple can pass. With Softmax, it's hard for multiple to pass 0.5.
    # But we can set low thresholds.
    model.predict_proba.return_value = np.array([[0.45, 0.45, 0.1]])
    return model

@pytest.fixture
def genre_classifier(mock_embedder, mock_model, tmp_path):
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
    # But we want _ensure_model to update thresholds if overrides passed.
    # We should probably mock joblib.load instead if we want to test _ensure_model logic,
    # but here we want to test predict_batch logic mostly.

    # Let's manually reimplement the partial logic of _ensure_model we care about in the test
    # or just trust the mock injection.

    return service

def test_predict_batch_single_label(genre_classifier):
    # With thresholds 0.4, both tech(0.45) and politics(0.45) pass.
    # Single label should return the highest score.
    # Since they are equal, it might depend on sort stability or order.
    # Let's adjust mock to distinguish.
    genre_classifier.model.predict_proba.return_value = np.array([[0.5, 0.4, 0.1]])

    results = genre_classifier.predict_batch(["test text"], multi_label=False)

    assert len(results) == 1
    res = results[0]
    assert res["top_genre"] == "tech"
    assert res["confidence"] == 0.5
    # candidates should be present but cropped or full? Code says candidates[:top_k]
    assert len(res["candidates"]) >= 1
    assert res["candidates"][0]["genre"] == "tech"

def test_predict_batch_multi_label(genre_classifier):
    # tech=0.5 (thresh=0.4) -> PASS
    # politics=0.45 (thresh=0.4) -> PASS
    # other=0.05 (thresh=0.5) -> FAIL
    genre_classifier.model.predict_proba.return_value = np.array([[0.5, 0.45, 0.05]])

    results = genre_classifier.predict_batch(["test text"], multi_label=True, top_k=5)

    assert len(results) == 1
    res = results[0]
    assert res["top_genre"] == "tech" # First sorted

    # Check candidates structure for multi-label
    candidates = res["candidates"]
    assert len(candidates) == 2
    genres = {c["genre"] for c in candidates}
    assert "tech" in genres
    assert "politics" in genres
    assert "other" not in genres

def test_predict_batch_threshold_overrides_passed_to_ensure(genre_classifier):
    # This test verifies that we can pass thresholds to predict_batch
    # In our mocked service, we mocked _ensure_model, so we verify it was called with overrides
    overrides = {"tech": 0.9}
    genre_classifier.predict_batch(["test"], threshold_overrides=overrides)
    genre_classifier._ensure_model.assert_called_with(overrides)

# We should also test that _ensure_model actually updates current_thresholds.
# We need a fresh service without mocked _ensure_model for that.
@patch("joblib.load")
@patch("pathlib.Path.exists")
@patch("builtins.open", new_callable=MagicMock)
@patch("json.load")
def test_ensure_model_updates_thresholds(mock_json_load, mock_open, mock_exists, mock_load, mock_embedder, tmp_path):
    mock_exists.return_value = True # model and thresholds exist
    mock_json_load.return_value = {"tech": 0.5} # Base thresholds

    service = GenreClassifierService("dummy/path", mock_embedder)

    # Override
    service._ensure_model(threshold_overrides={"tech": 0.8, "new": 0.3})

    assert service.current_thresholds["tech"] == 0.8
    assert service.current_thresholds["new"] == 0.3
    # Base should remain unchanged in the underlying dict if separate, but here we copied it.
    assert service.thresholds["tech"] == 0.5


def test_predict_batch_below_threshold_flag_single_label(genre_classifier):
    """Test that below_threshold flag is set when no genre passes threshold."""
    # All scores below their thresholds
    # tech=0.3 (thresh=0.4) -> FAIL
    # politics=0.3 (thresh=0.4) -> FAIL
    # other=0.4 (thresh=0.5) -> FAIL
    genre_classifier.model.predict_proba.return_value = np.array([[0.3, 0.3, 0.4]])

    results = genre_classifier.predict_batch(["test text"], multi_label=False)

    assert len(results) == 1
    res = results[0]
    # Should use highest score (other=0.4)
    assert res["top_genre"] == "other"
    assert res["confidence"] == 0.4
    assert res["below_threshold"] is True
    assert res["candidates"] == []  # No candidates passed threshold


def test_predict_batch_below_threshold_flag_multi_label(genre_classifier):
    """Test below_threshold in multi-label mode when nothing passes."""
    # All scores below their thresholds
    genre_classifier.model.predict_proba.return_value = np.array([[0.3, 0.3, 0.4]])

    results = genre_classifier.predict_batch(["test text"], multi_label=True)

    assert len(results) == 1
    res = results[0]
    # Should use highest score (other=0.4)
    assert res["top_genre"] == "other"
    assert res["confidence"] == 0.4
    assert res["below_threshold"] is True
    # In multi-label mode, should include fallback as a candidate
    assert len(res["candidates"]) == 1
    assert res["candidates"][0]["genre"] == "other"


def test_predict_batch_above_threshold_flag(genre_classifier):
    """Test that below_threshold is False when a genre passes threshold."""
    # tech passes threshold
    genre_classifier.model.predict_proba.return_value = np.array([[0.5, 0.3, 0.2]])

    results = genre_classifier.predict_batch(["test text"], multi_label=False)

    assert len(results) == 1
    res = results[0]
    assert res["top_genre"] == "tech"
    assert res["confidence"] == 0.5
    assert res["below_threshold"] is False
