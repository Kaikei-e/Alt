"""RED test for the runtime embedder fingerprint guard.

The 30-class classifier shipped on 2026-04-23 was trained against
``BAAI/bge-m3`` embeddings and records that identity in the sidecar
``embedding_model_id`` field. Production runtime, however, has been
serving embeddings via Ollama-remote with ``mxbai-embed-large`` (env
override ``RECAP_SUBWORKER_MODEL_BACKEND=ollama-remote`` +
``OLLAMA_EMBED_MODEL=mxbai-embed-large``). Both encoders happen to emit
1024-dim vectors so neither ``n_features_in_`` nor ``feature_dim`` raises,
but they live in different vector spaces — the LogReg head trained on
BGE-M3 features collapses against mxbai features (observed: 9 of 30
genres covered, 21 silently dropped, see ``recap_outputs`` 2026-04-27..29).

This test pins the missing fail-closed behaviour. The guard must:

* compare ``ClassifierMetadata.embedding_model_id`` against the
  *effective runtime embedder identity* resolved from ``Settings``
* raise ``ConfigValidationError`` on mismatch unless the explicit
  ``allow_embedding_drift`` opt-out is set
* recognise canonical aliases (``BAAI/bge-m3`` ↔ ``bge-m3`` ↔
  ``bge-m3:latest``) so the same artefact can be served via
  sentence-transformers or via Ollama-served BGE-M3
* tolerate an empty ``embedding_model_id`` (legacy artefacts predating
  ADR-000835 stage 3) with a warning, not a crash
* skip entirely for ``model_backend='hash'`` (deterministic dev backend
  that never touches the trained head in production)
"""

from __future__ import annotations

import importlib.metadata
import json
from pathlib import Path
from unittest.mock import MagicMock

import joblib
import numpy as np
import pytest
from sklearn.linear_model import LogisticRegression


def _runtime_sklearn_minor() -> str:
    full = importlib.metadata.version("scikit-learn")
    major, minor, *_ = full.split(".")
    return f"{major}.{minor}"


@pytest.fixture
def tiny_model(tmp_path: Path) -> Path:
    rng = np.random.default_rng(0)
    x = rng.normal(size=(30, 4))
    y = np.array([f"c{i}" for i in range(3) for _ in range(10)])
    clf = LogisticRegression(max_iter=200).fit(x, y)
    path = tmp_path / "genre_classifier_ja.joblib"
    joblib.dump(clf, path)
    return path


def _write_meta(model_path: Path, **overrides: object) -> Path:
    meta_path = model_path.with_suffix(".meta.json")
    payload: dict[str, object] = {
        "sklearn_version": _runtime_sklearn_minor() + ".0",
        "transformers_version": "5.6.0",
        "sentence_transformers_version": "5.4.0",
        "trained_at": "2026-04-23T12:00:00+00:00",
        "language": "ja",
        "classes": [f"c{i}" for i in range(3)],
        "feature_dim": 4,
        "source_data_sha256": "0" * 64,
        "embedding_model_id": "BAAI/bge-m3",
        "device": "cuda",
    }
    payload.update(overrides)
    meta_path.write_text(json.dumps(payload))
    return meta_path


def _make_embedder(
    *,
    backend: str,
    model_id: str = "BAAI/bge-m3",
    ollama_embed_model: str = "mxbai-embed-large",
    onnx_tokenizer_name: str | None = None,
    allow_drift: bool = False,
) -> MagicMock:
    """Build a mock embedder whose ``config`` reflects the runtime backend.

    The guard reads ``embedder.config.backend`` plus the backend-specific
    identity field to compute the *effective* runtime embedder identity.
    """
    embedder = MagicMock()
    embedder.config.batch_size = 32
    embedder.config.backend = backend
    embedder.config.model_id = model_id
    embedder.config.ollama_embed_model = ollama_embed_model
    embedder.config.onnx_tokenizer_name = onnx_tokenizer_name
    embedder.config.allow_embedding_drift = allow_drift
    return embedder


class TestEmbeddingFingerprintMatch:
    def test_sentence_transformers_bge_m3_matches_sidecar(self, tiny_model: Path) -> None:
        """Sidecar BGE-M3 + runtime sentence-transformers BGE-M3 → no raise."""
        from recap_subworker.services.classifier import GenreClassifierService

        _write_meta(tiny_model, embedding_model_id="BAAI/bge-m3")
        embedder = _make_embedder(
            backend="sentence-transformers",
            model_id="BAAI/bge-m3",
        )

        service = GenreClassifierService(str(tiny_model), embedder)
        # Should not raise.
        service._ensure_model()

    def test_ollama_bge_m3_matches_sidecar_via_alias(self, tiny_model: Path) -> None:
        """Sidecar ``BAAI/bge-m3`` ↔ Ollama tag ``bge-m3`` is canonical-equivalent."""
        from recap_subworker.services.classifier import GenreClassifierService

        _write_meta(tiny_model, embedding_model_id="BAAI/bge-m3")
        embedder = _make_embedder(
            backend="ollama-remote",
            ollama_embed_model="bge-m3",
        )

        service = GenreClassifierService(str(tiny_model), embedder)
        service._ensure_model()

    def test_ollama_bge_m3_with_tag_suffix_matches(self, tiny_model: Path) -> None:
        """``bge-m3:latest`` (Ollama tag) is the same artefact as ``BAAI/bge-m3``."""
        from recap_subworker.services.classifier import GenreClassifierService

        _write_meta(tiny_model, embedding_model_id="BAAI/bge-m3")
        embedder = _make_embedder(
            backend="ollama-remote",
            ollama_embed_model="bge-m3:latest",
        )

        service = GenreClassifierService(str(tiny_model), embedder)
        service._ensure_model()


class TestEmbeddingFingerprintMismatch:
    def test_ollama_mxbai_against_bge_m3_sidecar_raises(self, tiny_model: Path) -> None:
        """The exact production drift that caused the 2026-04-14..30 outage."""
        from recap_subworker.domain.errors import ConfigValidationError
        from recap_subworker.services.classifier import GenreClassifierService

        _write_meta(tiny_model, embedding_model_id="BAAI/bge-m3")
        embedder = _make_embedder(
            backend="ollama-remote",
            ollama_embed_model="mxbai-embed-large",
        )

        service = GenreClassifierService(str(tiny_model), embedder)
        with pytest.raises(ConfigValidationError, match="embedding_model_id"):
            service._ensure_model()

    def test_sentence_transformers_e5_against_bge_m3_sidecar_raises(self, tiny_model: Path) -> None:
        """A different sentence-transformers identity than training is also drift."""
        from recap_subworker.domain.errors import ConfigValidationError
        from recap_subworker.services.classifier import GenreClassifierService

        _write_meta(tiny_model, embedding_model_id="BAAI/bge-m3")
        embedder = _make_embedder(
            backend="sentence-transformers",
            model_id="intfloat/multilingual-e5-large",
        )

        service = GenreClassifierService(str(tiny_model), embedder)
        with pytest.raises(ConfigValidationError, match="embedding_model_id"):
            service._ensure_model()

    def test_drift_flag_downgrades_to_warning(self, tiny_model: Path) -> None:
        """``allow_embedding_drift=True`` keeps the boot path green."""
        from recap_subworker.services.classifier import GenreClassifierService

        _write_meta(tiny_model, embedding_model_id="BAAI/bge-m3")
        embedder = _make_embedder(
            backend="ollama-remote",
            ollama_embed_model="mxbai-embed-large",
            allow_drift=True,
        )

        service = GenreClassifierService(str(tiny_model), embedder)
        service._ensure_model()
        # No exception — exact log channel asserted in integration layer.


class TestEmbeddingFingerprintTolerance:
    def test_legacy_sidecar_without_embedding_id_warns(self, tiny_model: Path) -> None:
        """Pre-ADR-000835 sidecars omit ``embedding_model_id``."""
        from recap_subworker.services.classifier import GenreClassifierService

        _write_meta(tiny_model, embedding_model_id="")
        embedder = _make_embedder(
            backend="ollama-remote",
            ollama_embed_model="mxbai-embed-large",
        )

        service = GenreClassifierService(str(tiny_model), embedder)
        # Empty sidecar field → warn-only (no ConfigValidationError).
        service._ensure_model()

    def test_hash_backend_bypasses_guard(self, tiny_model: Path) -> None:
        """``hash`` backend is the deterministic dev embedder; never compare."""
        from recap_subworker.services.classifier import GenreClassifierService

        _write_meta(tiny_model, embedding_model_id="BAAI/bge-m3")
        embedder = _make_embedder(backend="hash")

        service = GenreClassifierService(str(tiny_model), embedder)
        service._ensure_model()
