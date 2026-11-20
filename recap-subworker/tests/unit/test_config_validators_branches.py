"""Characterization tests for edge return paths of Settings validators."""

from __future__ import annotations

import importlib.metadata
import json
from pathlib import Path

import joblib
import numpy as np
import pytest
from sklearn.linear_model import LogisticRegression

from recap_subworker.infra.config import Settings

THIRTY_CLASSES = [f"genre_{i}" for i in range(30)]
TRAIN_SAMPLES_PER_CLASS = 5
FEATURE_DIM = 8
MAX_ITER = 200
RNG_SEED = 7
DEFAULT_THRESHOLD = 0.5
HEX_SHA256_LEN = 64
PARTIAL_THRESHOLD_COUNT = 20


def _runtime_sklearn_minor() -> str:
    full = importlib.metadata.version("scikit-learn")
    major, minor, *_ = full.split(".")
    return f"{major}.{minor}"


def _write_model(path: Path) -> Path:
    rng = np.random.default_rng(RNG_SEED)
    x = rng.normal(size=(len(THIRTY_CLASSES) * TRAIN_SAMPLES_PER_CLASS, FEATURE_DIM))
    y = np.array([c for c in THIRTY_CLASSES for _ in range(TRAIN_SAMPLES_PER_CLASS)])
    model = LogisticRegression(max_iter=MAX_ITER).fit(x, y)
    joblib.dump(model, path)
    return path


def _write_thresholds(path: Path, classes: list[str]) -> Path:
    path.write_text(json.dumps(dict.fromkeys(classes, DEFAULT_THRESHOLD)), encoding="utf-8")
    return path


def _write_sidecar(model_path: Path, *, classes: list[str], embedding_model_id: str) -> Path:
    meta_path = model_path.with_suffix(".meta.json")
    payload = {
        "sklearn_version": _runtime_sklearn_minor() + ".0",
        "transformers_version": "5.6.0",
        "sentence_transformers_version": "5.4.0",
        "trained_at": "2026-04-23T12:00:00+00:00",
        "language": "ja",
        "classes": classes,
        "feature_dim": FEATURE_DIM,
        "source_data_sha256": "0" * HEX_SHA256_LEN,
        "embedding_model_id": embedding_model_id,
        "device": "cpu",
    }
    meta_path.write_text(json.dumps(payload), encoding="utf-8")
    return meta_path


@pytest.fixture(autouse=True)
def _strict_drift(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.delenv("RECAP_SUBWORKER_ALLOW_EMBEDDING_DRIFT", raising=False)


def test_empty_ja_model_or_thresholds_path_skips_validators() -> None:
    settings = Settings(  # type: ignore[arg-type]
        classification_backend="joblib",
        genre_classifier_model_path_ja="",
        genre_classifier_model_path_en="",
        genre_thresholds_path_ja="",
        genre_thresholds_path_en="",
        tfidf_vectorizer_path_ja="",
        tfidf_vectorizer_path_en="",
    )
    assert settings.classification_backend == "joblib"


def test_corrupt_model_file_skips_threshold_coverage(tmp_path: Path) -> None:
    bad_model = tmp_path / "bad.joblib"
    bad_model.write_bytes(b"not-a-pickle")
    thresholds = _write_thresholds(tmp_path / "thresholds.json", THIRTY_CLASSES)
    settings = Settings(  # type: ignore[arg-type]
        classification_backend="joblib",
        genre_classifier_model_path=str(bad_model),
        genre_classifier_model_path_ja=str(bad_model),
        genre_classifier_model_path_en="",
        genre_thresholds_path_ja=str(thresholds),
        genre_thresholds_path_en="",
        tfidf_vectorizer_path_ja="",
        tfidf_vectorizer_path_en="",
    )
    assert settings.genre_classifier_model_path_ja == str(bad_model)


def test_corrupt_or_nondict_thresholds_file_skips_coverage(tmp_path: Path) -> None:
    model = _write_model(tmp_path / "model.joblib")
    bad_thresholds = tmp_path / "thresholds.json"
    bad_thresholds.write_text("{bad-json", encoding="utf-8")
    settings = Settings(  # type: ignore[arg-type]
        classification_backend="joblib",
        genre_classifier_model_path=str(model),
        genre_classifier_model_path_ja=str(model),
        genre_classifier_model_path_en="",
        genre_thresholds_path_ja=str(bad_thresholds),
        genre_thresholds_path_en="",
        tfidf_vectorizer_path_ja="",
        tfidf_vectorizer_path_en="",
    )
    assert settings.genre_thresholds_path_ja == str(bad_thresholds)

    bad_thresholds.write_text('["not", "a", "dict"]', encoding="utf-8")
    settings_list = Settings(  # type: ignore[arg-type]
        classification_backend="joblib",
        genre_classifier_model_path=str(model),
        genre_classifier_model_path_ja=str(model),
        genre_classifier_model_path_en="",
        genre_thresholds_path_ja=str(bad_thresholds),
        genre_thresholds_path_en="",
        tfidf_vectorizer_path_ja="",
        tfidf_vectorizer_path_en="",
    )
    assert settings_list.genre_thresholds_path_ja == str(bad_thresholds)


def test_meta_sidecar_classes_used_for_threshold_coverage(tmp_path: Path) -> None:
    model = _write_model(tmp_path / "model.joblib")
    _write_sidecar(model, classes=THIRTY_CLASSES, embedding_model_id="BAAI/bge-m3")
    thresholds = _write_thresholds(
        tmp_path / "thresholds.json", THIRTY_CLASSES[:PARTIAL_THRESHOLD_COUNT]
    )
    with pytest.raises(ValueError, match="threshold coverage gap"):
        Settings(  # type: ignore[arg-type]
            classification_backend="joblib",
            genre_classifier_model_path=str(model),
            genre_classifier_model_path_ja=str(model),
            genre_classifier_model_path_en="",
            genre_thresholds_path_ja=str(thresholds),
            genre_thresholds_path_en="",
            tfidf_vectorizer_path_ja="",
            tfidf_vectorizer_path_en="",
            model_backend="sentence-transformers",
            model_id="BAAI/bge-m3",
        )


def test_corrupt_or_nondict_sidecar_skips_embedding_consistency(tmp_path: Path) -> None:
    model = _write_model(tmp_path / "model.joblib")
    thresholds = _write_thresholds(tmp_path / "thresholds.json", THIRTY_CLASSES)
    meta_path = model.with_suffix(".meta.json")
    meta_path.write_text("{not-json", encoding="utf-8")
    Settings(  # type: ignore[arg-type]
        classification_backend="joblib",
        genre_classifier_model_path=str(model),
        genre_classifier_model_path_ja=str(model),
        genre_classifier_model_path_en="",
        genre_thresholds_path_ja=str(thresholds),
        genre_thresholds_path_en="",
        tfidf_vectorizer_path_ja="",
        tfidf_vectorizer_path_en="",
        model_backend="sentence-transformers",
        model_id="intfloat/multilingual-e5-large",
    )
    meta_path.write_text('["not", "dict"]', encoding="utf-8")
    Settings(  # type: ignore[arg-type]
        classification_backend="joblib",
        genre_classifier_model_path=str(model),
        genre_classifier_model_path_ja=str(model),
        genre_classifier_model_path_en="",
        genre_thresholds_path_ja=str(thresholds),
        genre_thresholds_path_en="",
        tfidf_vectorizer_path_ja="",
        tfidf_vectorizer_path_en="",
        model_backend="sentence-transformers",
        model_id="intfloat/multilingual-e5-large",
    )


def test_hash_backend_skips_embedding_consistency(tmp_path: Path) -> None:
    model = _write_model(tmp_path / "model.joblib")
    thresholds = _write_thresholds(tmp_path / "thresholds.json", THIRTY_CLASSES)
    _write_sidecar(model, classes=THIRTY_CLASSES, embedding_model_id="BAAI/bge-m3")
    settings = Settings(  # type: ignore[arg-type]
        classification_backend="joblib",
        genre_classifier_model_path=str(model),
        genre_classifier_model_path_ja=str(model),
        genre_classifier_model_path_en="",
        genre_thresholds_path_ja=str(thresholds),
        genre_thresholds_path_en="",
        tfidf_vectorizer_path_ja="",
        tfidf_vectorizer_path_en="",
        model_backend="hash",
    )
    assert settings.model_backend == "hash"
