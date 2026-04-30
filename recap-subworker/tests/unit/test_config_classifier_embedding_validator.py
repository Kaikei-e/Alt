"""RED test for the Settings-level peer validator that enforces classifier
sidecar ↔ runtime embedder fingerprint agreement at boot time.

The 2026-04-14..30 silent classification collapse landed because the runtime
embedder identity drifted from the encoder used at training. Cardinality and
threshold-coverage validators ([[000835]] / [[000857]]) catch joblib drift
but not embedding-space drift. This validator adds the missing layer:

* When ``classification_backend == 'joblib'`` and a ``<model>.meta.json``
  sidecar exists with a non-empty ``embedding_model_id``, compare the
  canonicalised value to the canonicalised effective runtime embedder
  identity (derived from ``model_backend`` plus its backend-specific id).
* On mismatch, ``Settings(...)`` construction raises ``ValueError`` (boot
  fail-closed).
* The opt-out env ``RECAP_SUBWORKER_ALLOW_EMBEDDING_DRIFT=true`` short-
  circuits the check (mapped to ``Settings.allow_embedding_drift``).
* Empty / missing sidecar fields and the ``hash`` backend are bypassed
  the same way the cardinality validator handles them.
"""

from __future__ import annotations

import importlib.metadata
import json
import os
from pathlib import Path

import joblib
import numpy as np
import pytest
from sklearn.linear_model import LogisticRegression


def _runtime_sklearn_minor() -> str:
    full = importlib.metadata.version("scikit-learn")
    major, minor, *_ = full.split(".")
    return f"{major}.{minor}"


def _fit_tiny_model(n_classes: int) -> LogisticRegression:
    rng = np.random.default_rng(42)
    x = rng.normal(size=(n_classes * 5, 8))
    y = np.array([f"class_{i}" for i in range(n_classes) for _ in range(5)])
    return LogisticRegression(max_iter=200).fit(x, y)


@pytest.fixture
def ja_model_path_30(tmp_path: Path) -> Path:
    path = tmp_path / "genre_classifier_ja.joblib"
    joblib.dump(_fit_tiny_model(30), path)
    return path


@pytest.fixture
def ja_thresholds_path(tmp_path: Path) -> Path:
    path = tmp_path / "genre_thresholds_ja.json"
    path.write_text(json.dumps({f"class_{i}": 0.5 for i in range(30)}))
    return path


def _write_sidecar(model_path: Path, embedding_model_id: str) -> Path:
    meta_path = model_path.with_suffix(".meta.json")
    payload = {
        "sklearn_version": _runtime_sklearn_minor() + ".0",
        "transformers_version": "5.6.0",
        "sentence_transformers_version": "5.4.0",
        "trained_at": "2026-04-23T12:00:00+00:00",
        "language": "ja",
        "classes": [f"class_{i}" for i in range(30)],
        "feature_dim": 8,
        "source_data_sha256": "0" * 64,
        "embedding_model_id": embedding_model_id,
        "device": "cuda",
    }
    meta_path.write_text(json.dumps(payload))
    return meta_path


def _build_settings(
    *,
    ja_model: Path,
    ja_thresholds: Path,
    model_backend: str = "sentence-transformers",
    model_id: str = "BAAI/bge-m3",
    ollama_embed_model: str = "mxbai-embed-large",
    ollama_embed_url: str | None = None,
    onnx_tokenizer_name: str | None = None,
):
    from recap_subworker.infra.config import Settings

    kwargs: dict[str, object] = {
        "classification_backend": "joblib",
        "genre_classifier_model_path_ja": str(ja_model),
        "genre_classifier_model_path_en": "",
        "genre_thresholds_path_ja": str(ja_thresholds),
        "genre_thresholds_path_en": "",
        "tfidf_vectorizer_path_ja": "",
        "tfidf_vectorizer_path_en": "",
        "model_backend": model_backend,
        "model_id": model_id,
        "ollama_embed_model": ollama_embed_model,
        "ollama_embed_url": ollama_embed_url,
        "onnx_tokenizer_name": onnx_tokenizer_name,
    }
    return Settings(**kwargs)  # type: ignore[arg-type]


@pytest.fixture(autouse=True)
def _clear_drift_opt_out(monkeypatch: pytest.MonkeyPatch) -> None:
    """Tests in this file exercise the validator's strict path; ensure the
    session-level test-only opt-out (see ``tests/conftest.py``) does not
    short-circuit the check."""
    monkeypatch.delenv("RECAP_SUBWORKER_ALLOW_EMBEDDING_DRIFT", raising=False)


class TestPeerValidatorMatchPath:
    def test_sentence_transformers_bge_m3_matches_sidecar(
        self, ja_model_path_30: Path, ja_thresholds_path: Path
    ) -> None:
        _write_sidecar(ja_model_path_30, "BAAI/bge-m3")
        settings = _build_settings(
            ja_model=ja_model_path_30,
            ja_thresholds=ja_thresholds_path,
            model_backend="sentence-transformers",
            model_id="BAAI/bge-m3",
        )
        assert settings.classification_backend == "joblib"

    def test_ollama_bge_m3_alias_matches_sidecar(
        self, ja_model_path_30: Path, ja_thresholds_path: Path
    ) -> None:
        _write_sidecar(ja_model_path_30, "BAAI/bge-m3")
        settings = _build_settings(
            ja_model=ja_model_path_30,
            ja_thresholds=ja_thresholds_path,
            model_backend="ollama-remote",
            ollama_embed_model="bge-m3",
            ollama_embed_url="http://example:11436",
        )
        assert settings.model_backend == "ollama-remote"


class TestPeerValidatorDriftPath:
    def test_ollama_mxbai_against_bge_m3_sidecar_raises(
        self, ja_model_path_30: Path, ja_thresholds_path: Path
    ) -> None:
        _write_sidecar(ja_model_path_30, "BAAI/bge-m3")
        with pytest.raises(ValueError, match="embedding_model_id"):
            _build_settings(
                ja_model=ja_model_path_30,
                ja_thresholds=ja_thresholds_path,
                model_backend="ollama-remote",
                ollama_embed_model="mxbai-embed-large",
                ollama_embed_url="http://example:11436",
            )

    def test_sentence_transformers_e5_against_bge_m3_sidecar_raises(
        self, ja_model_path_30: Path, ja_thresholds_path: Path
    ) -> None:
        _write_sidecar(ja_model_path_30, "BAAI/bge-m3")
        with pytest.raises(ValueError, match="embedding_model_id"):
            _build_settings(
                ja_model=ja_model_path_30,
                ja_thresholds=ja_thresholds_path,
                model_backend="sentence-transformers",
                model_id="intfloat/multilingual-e5-large",
            )


class TestPeerValidatorOptOut:
    def test_drift_flag_env_lets_boot_pass(
        self,
        ja_model_path_30: Path,
        ja_thresholds_path: Path,
        monkeypatch: pytest.MonkeyPatch,
    ) -> None:
        _write_sidecar(ja_model_path_30, "BAAI/bge-m3")
        monkeypatch.setenv("RECAP_SUBWORKER_ALLOW_EMBEDDING_DRIFT", "true")
        settings = _build_settings(
            ja_model=ja_model_path_30,
            ja_thresholds=ja_thresholds_path,
            model_backend="ollama-remote",
            ollama_embed_model="mxbai-embed-large",
            ollama_embed_url="http://example:11436",
        )
        assert settings.allow_embedding_drift is True
        monkeypatch.delenv("RECAP_SUBWORKER_ALLOW_EMBEDDING_DRIFT", raising=False)
        os.environ.pop("RECAP_SUBWORKER_ALLOW_EMBEDDING_DRIFT", None)

    def test_default_allow_embedding_drift_is_false(
        self, ja_model_path_30: Path, ja_thresholds_path: Path
    ) -> None:
        _write_sidecar(ja_model_path_30, "BAAI/bge-m3")
        settings = _build_settings(
            ja_model=ja_model_path_30,
            ja_thresholds=ja_thresholds_path,
            model_backend="sentence-transformers",
            model_id="BAAI/bge-m3",
        )
        assert settings.allow_embedding_drift is False


class TestPeerValidatorTolerantPaths:
    def test_legacy_sidecar_without_embedding_id_passes(
        self, ja_model_path_30: Path, ja_thresholds_path: Path
    ) -> None:
        """Sidecars predating ADR-000835 stage 3 omit ``embedding_model_id``."""
        meta_path = ja_model_path_30.with_suffix(".meta.json")
        payload = json.loads(meta_path.read_text()) if meta_path.exists() else {}
        payload = {
            "sklearn_version": _runtime_sklearn_minor() + ".0",
            "transformers_version": "5.6.0",
            "sentence_transformers_version": "5.4.0",
            "trained_at": "2026-04-23T12:00:00+00:00",
            "language": "ja",
            "classes": [f"class_{i}" for i in range(30)],
            "feature_dim": 8,
            "source_data_sha256": "0" * 64,
            "embedding_model_id": "",
            "device": "cuda",
        }
        meta_path.write_text(json.dumps(payload))
        settings = _build_settings(
            ja_model=ja_model_path_30,
            ja_thresholds=ja_thresholds_path,
            model_backend="ollama-remote",
            ollama_embed_model="mxbai-embed-large",
            ollama_embed_url="http://example:11436",
        )
        assert settings.classification_backend == "joblib"

    def test_missing_sidecar_passes(self, ja_model_path_30: Path, ja_thresholds_path: Path) -> None:
        """No sidecar at all → nothing to compare; do not block boot."""
        # Deliberately do NOT call ``_write_sidecar``.
        settings = _build_settings(
            ja_model=ja_model_path_30,
            ja_thresholds=ja_thresholds_path,
            model_backend="ollama-remote",
            ollama_embed_model="mxbai-embed-large",
            ollama_embed_url="http://example:11436",
        )
        assert settings.classification_backend == "joblib"

    def test_learning_machine_backend_bypasses_validator(
        self,
        ja_model_path_30: Path,
        ja_thresholds_path: Path,
        tmp_path: Path,
    ) -> None:
        """Non-joblib classification backend short-circuits."""
        from recap_subworker.infra.config import Settings

        _write_sidecar(ja_model_path_30, "BAAI/bge-m3")
        lm_dir = tmp_path / "lm"
        lm_dir.mkdir()
        settings = Settings(  # type: ignore[arg-type]
            classification_backend="learning_machine",
            learning_machine_student_ja_dir=str(lm_dir),
            learning_machine_student_en_dir=str(lm_dir),
            genre_classifier_model_path_ja=str(ja_model_path_30),
            genre_thresholds_path_ja=str(ja_thresholds_path),
            model_backend="ollama-remote",
            ollama_embed_model="mxbai-embed-large",
            ollama_embed_url="http://example:11436",
        )
        assert settings.classification_backend == "learning_machine"
