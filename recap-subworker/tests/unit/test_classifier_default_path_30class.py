"""RED test: the production default classifier path must point at the 30-class JA artefact.

Context: ``ServiceContainer.classifier`` and ``classification_worker._init_worker_state``
both read ``settings.genre_classifier_model_path``. Until 2026-04-27 that field
defaulted to ``data/genre_classifier.joblib`` — the legacy 17-class artefact —
even though the 30-class JA artefact ``data/genre_classifier_ja.joblib`` had
already been retrained on 2026-04-23 (sklearn 1.8.0). Production therefore kept
classifying into a collapsed 17-class space, starving the recap pipeline of
genre evidence and triggering ``no_evidence`` / ``Missing from batch response``
failures from 2026-04-22 onward.

This test pins the corrected default: the path must end in ``_ja.joblib`` so
both the container and the multiprocess classification worker resolve to the
30-class artefact in absence of an explicit env override. The default must
also satisfy the ADR-000835 stage-3 cardinality validator at baseline 30.
"""

from __future__ import annotations

from pathlib import Path

import joblib
import numpy as np
import pytest
from sklearn.linear_model import LogisticRegression


def _fit_tiny_model(n_classes: int) -> LogisticRegression:
    rng = np.random.default_rng(7)
    x = rng.normal(size=(n_classes * 5, 8))
    y = np.array([f"class_{i}" for i in range(n_classes) for _ in range(5)])
    return LogisticRegression(max_iter=200).fit(x, y)


def test_default_genre_classifier_model_path_targets_ja_30class_artefact() -> None:
    """The default points at the 30-class JA joblib, not the legacy unsuffixed one."""
    from recap_subworker.infra.config import Settings

    settings = Settings.model_construct()
    default = Settings.model_fields["genre_classifier_model_path"].default
    assert default.endswith("_ja.joblib"), (
        f"genre_classifier_model_path default must point at the 30-class JA artefact, "
        f"got {default!r}. The legacy ``data/genre_classifier.joblib`` is 17-class."
    )
    # Sanity: the field instance also reports the same default.
    assert settings.genre_classifier_model_path == default


def test_default_classifier_path_exists_and_has_30_classes() -> None:
    """The default must resolve to a real, loadable, 30-class joblib in the repo tree."""
    from recap_subworker.infra.config import Settings

    default = Settings.model_fields["genre_classifier_model_path"].default
    repo_root = Path(__file__).resolve().parents[2]
    artefact = repo_root / default
    if not artefact.exists():
        pytest.skip(
            f"default classifier artefact not present in dev tree: {artefact}. "
            "Production hosts mount the artefact at /var/lib/alt-recap-subworker-data; "
            "this file-path test requires a dev copy."
        )

    model = joblib.load(artefact)
    classes = getattr(model, "classes_", None)
    assert classes is not None, f"loaded artefact has no classes_ attribute: {artefact}"
    assert len(classes) == 30, (
        f"expected 30-class classifier at default path {artefact}, got {len(classes)}"
    )


def test_default_path_clears_baseline_cardinality_30(tmp_path: Path) -> None:
    """A 30-class default joblib must satisfy the bumped baseline of 30."""
    import json

    from recap_subworker.infra.config import Settings

    model_path = tmp_path / "genre_classifier_ja.joblib"
    joblib.dump(_fit_tiny_model(30), model_path)
    thresholds = tmp_path / "genre_thresholds_ja.json"
    thresholds.write_text(json.dumps({f"class_{i}": 0.5 for i in range(30)}))

    settings = Settings(  # type: ignore[arg-type]
        classification_backend="joblib",
        genre_classifier_model_path=str(model_path),
        genre_classifier_model_path_ja=str(model_path),
        genre_classifier_model_path_en="",
        genre_thresholds_path_ja=str(thresholds),
        genre_thresholds_path_en="",
        tfidf_vectorizer_path_ja="",
        tfidf_vectorizer_path_en="",
        genre_baseline_cardinality=30,
    )
    assert settings.genre_baseline_cardinality == 30


def test_default_baseline_cardinality_is_30() -> None:
    """The ``genre_baseline_cardinality`` field default must be 30 once the 30-class model is canonical."""
    from recap_subworker.infra.config import Settings

    default = Settings.model_fields["genre_baseline_cardinality"].default
    assert default == 30, (
        f"genre_baseline_cardinality default must be 30 — see ADR-000835 stage 3. got {default}"
    )
