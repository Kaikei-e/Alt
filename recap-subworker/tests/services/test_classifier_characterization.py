"""Characterization tests for GenreClassifierService._ensure_model and predict_batch."""

from __future__ import annotations

from pathlib import Path
from unittest.mock import MagicMock, patch

import joblib
import numpy as np
import pytest
from sklearn.decomposition import TruncatedSVD
from sklearn.feature_extraction.text import TfidfVectorizer
from sklearn.linear_model import LogisticRegression
from sklearn.preprocessing import StandardScaler

from recap_subworker.services.classifier import GenreClassifierService, SudachiTokenizer

EMBED_DIM = 2
SVD_COMPONENTS = 2
SAMPLE_CONFIDENCE_THRESHOLD = 0.5


def _make_embedder(embed_dim: int = EMBED_DIM) -> MagicMock:
    embedder = MagicMock()
    embedder.config.batch_size = 16
    embedder.config.backend = "hash"
    embedder.encode.side_effect = lambda texts: np.ones((len(texts), embed_dim), dtype=np.float32)
    return embedder


def test_ensure_model_loads_tfidf_svd_scaler_and_skips_reload_on_second_call(
    tmp_path: Path,
) -> None:
    rng = np.random.default_rng(7)
    texts = ["ai news article one", "politics news article two", "sports news article three"] * 4
    tfidf = TfidfVectorizer(max_features=6).fit(texts)
    tfidf_mat = tfidf.transform(texts)
    svd = TruncatedSVD(n_components=SVD_COMPONENTS, random_state=7).fit(tfidf_mat)
    tfidf_reduced = svd.transform(tfidf_mat)
    emb = rng.normal(size=(len(texts), EMBED_DIM)).astype(np.float32)
    combined = np.hstack((emb, tfidf_reduced))
    scaler = StandardScaler().fit(combined)
    scaled = scaler.transform(combined)
    y = np.array(["tech", "politics", "sports"] * 4)
    clf = LogisticRegression(max_iter=200).fit(scaled, y)

    model_path = tmp_path / "genre_classifier.joblib"
    joblib.dump(clf, model_path)
    joblib.dump(tfidf, tmp_path / "tfidf_vectorizer.joblib")
    joblib.dump(svd, tmp_path / "tfidf_svd.joblib")
    joblib.dump(scaler, tmp_path / "feature_scaler.joblib")

    embedder = _make_embedder()
    service = GenreClassifierService(str(model_path), embedder)
    service._ensure_model()

    assert service.model is not None
    assert service.tfidf is not None
    assert isinstance(service.tfidf.tokenizer, SudachiTokenizer)
    assert service.svd is not None
    assert service.scaler is not None
    assert service.thresholds == {}

    first_model = service.model
    service._ensure_model(threshold_overrides={"tech": 0.75})
    assert service.model is first_model
    assert service.current_thresholds == {"tech": 0.75}


def test_ensure_model_initializes_thresholds_when_preloaded_model_has_none_thresholds(
    tmp_path: Path,
) -> None:
    model_path = tmp_path / "model.joblib"
    model_path.touch()
    service = GenreClassifierService(str(model_path), _make_embedder())
    service.model = MagicMock()
    service.thresholds = None

    service._ensure_model(threshold_overrides={"tech": 0.6})

    assert service.thresholds == {}
    assert service.current_thresholds == {"tech": 0.6}


def test_predict_batch_returns_empty_for_empty_texts_and_empty_embeddings(
    tmp_path: Path,
) -> None:
    model_path = tmp_path / "model.joblib"
    model_path.touch()
    embedder = _make_embedder()
    service = GenreClassifierService(str(model_path), embedder)
    service.model = MagicMock()
    service.thresholds = {}

    assert service.predict_batch([]) == []

    embedder.encode.side_effect = lambda _texts: np.empty((0, EMBED_DIM), dtype=np.float32)
    assert service.predict_batch(["hello"]) == []


def test_predict_batch_hybrid_tfidf_svd_scaler_path(tmp_path: Path) -> None:
    model_path = tmp_path / "model.joblib"
    model_path.touch()
    service = GenreClassifierService(str(model_path), _make_embedder())

    tfidf = MagicMock()
    tfidf.transform.return_value = MagicMock()
    svd = MagicMock()
    svd.transform.return_value = np.array([[0.2, 0.3]], dtype=np.float32)
    scaler = MagicMock()
    scaler.transform.side_effect = lambda x: x * 2.0

    model = MagicMock()
    model.n_features_in_ = EMBED_DIM + SVD_COMPONENTS
    model.classes_ = np.array(["tech", "other"])
    model.predict_proba.return_value = np.array([[0.8, 0.2]])

    service.model = model
    service.tfidf = tfidf
    service.svd = svd
    service.scaler = scaler
    service.thresholds = {"tech": SAMPLE_CONFIDENCE_THRESHOLD, "other": SAMPLE_CONFIDENCE_THRESHOLD}

    results = service.predict_batch(["some article"])

    assert len(results) == 1
    assert results[0]["top_genre"] == "tech"
    passed_features = model.predict_proba.call_args[0][0]
    assert passed_features.shape == (1, EMBED_DIM + SVD_COMPONENTS)
    scaler.transform.assert_called_once()


def test_predict_batch_tfidf_dense_fallback_when_model_expects_embeddings_only(
    tmp_path: Path,
) -> None:
    model_path = tmp_path / "model.joblib"
    model_path.touch()
    service = GenreClassifierService(str(model_path), _make_embedder())

    sparse_mock = MagicMock()
    sparse_mock.toarray.return_value = np.array([[0.5, 0.5]], dtype=np.float32)
    tfidf = MagicMock()
    tfidf.transform.return_value = sparse_mock

    model = MagicMock()
    model.n_features_in_ = EMBED_DIM
    model.classes_ = np.array(["tech", "other"])
    model.predict_proba.return_value = np.array([[0.7, 0.3]])

    service.model = model
    service.tfidf = tfidf
    service.svd = None
    service.scaler = None
    service.thresholds = {}

    results = service.predict_batch(["some article"])

    assert results[0]["top_genre"] == "tech"
    passed_features = model.predict_proba.call_args[0][0]
    assert passed_features.shape == (1, EMBED_DIM)


def test_predict_batch_retries_with_embeddings_on_feature_mismatch_value_error(
    tmp_path: Path,
) -> None:
    model_path = tmp_path / "model.joblib"
    model_path.touch()
    service = GenreClassifierService(str(model_path), _make_embedder())

    sparse_mock = MagicMock()
    sparse_mock.toarray.return_value = np.array([[0.5, 0.5]], dtype=np.float32)
    tfidf = MagicMock()
    tfidf.transform.return_value = sparse_mock

    model = MagicMock()
    model.n_features_in_ = 99
    model.classes_ = np.array(["tech", "other"])
    model.predict_proba.side_effect = [
        ValueError(f"X has 4 features, but expecting {EMBED_DIM} features"),
        np.array([[0.65, 0.35]]),
    ]

    service.model = model
    service.tfidf = tfidf
    service.thresholds = {}

    results = service.predict_batch(["some article"])
    expected_calls = 2
    assert model.predict_proba.call_count == expected_calls
    assert results[0]["top_genre"] == "tech"


def test_predict_batch_reraises_unrelated_value_error_and_raises_if_model_none(
    tmp_path: Path,
) -> None:
    model_path = tmp_path / "model.joblib"
    model_path.touch()
    service = GenreClassifierService(str(model_path), _make_embedder())
    service.thresholds = {}

    with (
        patch.object(service, "_ensure_model"),
        pytest.raises(RuntimeError, match="Model must be loaded before prediction"),
    ):
        service.predict_batch(["some article"])

    model = MagicMock()
    model.predict_proba.side_effect = ValueError("unrelated error")
    service.model = model
    with pytest.raises(ValueError, match="unrelated error"):
        service.predict_batch(["some article"])
