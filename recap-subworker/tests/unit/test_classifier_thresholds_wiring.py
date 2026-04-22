"""RED test for classifier factory wiring language-specific thresholds.

Context: The classify-runs hot path builds `GenreClassifierService` via
`container.py::classifier` (and `classification_worker.py::_init_worker_state`).
Both previously passed only `model_path` + `embedder`, causing the classifier
to default to `<model_path>.parent / "genre_thresholds.json"` — a deprecated
17/30-genre file — which makes 13 canonical genres default to threshold 0.5
and collapse into argmax fallback (2-bucket genre_distribution).

This test pins the expectation that the factory uses the language-specific
Japanese thresholds path, which covers all 30 canonical genres.

Review reference: docs/review/3days-recap-rustbert-cache-recovery-2026-04-22.md
§残課題 (1).
"""

from __future__ import annotations

from unittest.mock import MagicMock, patch

from recap_subworker.app.container import ServiceContainer


def test_container_classifier_passes_language_specific_thresholds(tmp_path) -> None:
    """ServiceContainer.classifier must pass thresholds_path pointing at the
    30-genre Japanese thresholds file, not the deprecated default.
    """
    model_file = tmp_path / "genre_classifier.joblib"
    model_file.write_bytes(b"fake joblib")
    thresholds_file = tmp_path / "genre_thresholds_ja.json"

    settings_stub = MagicMock()
    settings_stub.genre_classifier_model_path = str(model_file)
    settings_stub.genre_thresholds_path_ja = str(thresholds_file)

    captured_kwargs: dict = {}

    def fake_init(self, model_path, embedder, **kwargs):
        captured_kwargs["model_path"] = model_path
        captured_kwargs.update(kwargs)

    with patch(
        "recap_subworker.app.container.GenreClassifierService.__init__",
        fake_init,
    ):
        container = ServiceContainer(settings_stub)
        container._embedder = MagicMock()
        _ = container.classifier

    assert "thresholds_path" in captured_kwargs, (
        "ServiceContainer must pass thresholds_path to GenreClassifierService"
    )
    assert captured_kwargs["thresholds_path"] == str(thresholds_file)
