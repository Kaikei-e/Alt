"""RED test for ADR-000835 stage 3: fail-closed classifier cardinality validator.

The 2-bucket collapse of 2026-04-14 shipped an empty-ish 17-class classifier
against a 30-genre kick. The Settings validator chain already catches empty
bind-mounts (``_validate_joblib_artifacts``), but it does not load the model
and cannot notice ``classes_`` cardinality regressions. This test pins the
expected fail-closed behaviour:

- classifier ``classes_`` length below ``genre_baseline_cardinality`` raises
  ``ValueError`` at ``Settings(...)`` construction
- a healthy 30-class model passes through
- ``classification_backend != 'joblib'`` short-circuits the validator
"""

from __future__ import annotations

import json
import os
from pathlib import Path

import joblib
import numpy as np
import pytest
from sklearn.linear_model import LogisticRegression


def _fit_tiny_model(n_classes: int) -> LogisticRegression:
    """Fit a trivial LogisticRegression so ``classes_`` has length ``n_classes``."""
    rng = np.random.default_rng(42)
    # 5 samples per class so sklearn does not complain about single-sample classes
    x = rng.normal(size=(n_classes * 5, 8))
    y = np.array([f"class_{i}" for i in range(n_classes) for _ in range(5)])
    return LogisticRegression(max_iter=200).fit(x, y)


@pytest.fixture
def ja_model_path_17(tmp_path: Path) -> Path:
    """17-class joblib — the real production artefact pre-retrain."""
    path = tmp_path / "genre_classifier_ja.joblib"
    joblib.dump(_fit_tiny_model(17), path)
    return path


@pytest.fixture
def ja_model_path_30(tmp_path: Path) -> Path:
    """30-class joblib — the post-retrain target artefact."""
    path = tmp_path / "genre_classifier_ja.joblib"
    joblib.dump(_fit_tiny_model(30), path)
    return path


@pytest.fixture
def ja_thresholds_path(tmp_path: Path) -> Path:
    """Sidecar thresholds file covering every fixture class.

    The 17- and 30-class tiny models use ``class_0``..``class_N`` labels.  The
    coverage validator added 2026-04-27 refuses to start when classes_ is
    not fully covered by the thresholds JSON, so this fixture writes an entry
    for each potential label up to the largest fixture (30).
    """
    path = tmp_path / "genre_thresholds_ja.json"
    path.write_text(json.dumps({f"class_{i}": 0.5 for i in range(30)}))
    return path


def _build_settings(
    *,
    ja_model: Path,
    ja_thresholds: Path,
    baseline: int | None = None,
):
    """Construct Settings with only the fields needed by both validators."""
    from recap_subworker.infra.config import Settings

    kwargs: dict[str, object] = {
        "classification_backend": "joblib",
        "genre_classifier_model_path_ja": str(ja_model),
        "genre_classifier_model_path_en": "",  # skip EN model
        "genre_thresholds_path_ja": str(ja_thresholds),
        "genre_thresholds_path_en": "",
        "tfidf_vectorizer_path_ja": "",
        "tfidf_vectorizer_path_en": "",
    }
    if baseline is not None:
        kwargs["genre_baseline_cardinality"] = baseline
    return Settings(**kwargs)  # type: ignore[arg-type]


class TestClassifierCardinalityValidator:
    def test_17_class_model_below_default_baseline_raises(
        self, ja_model_path_17: Path, ja_thresholds_path: Path
    ) -> None:
        # Default baseline 15: 17 >= 15 passes. Use a baseline of 25 to trigger
        # the fail-closed path for the pre-retrain artefact.
        with pytest.raises(ValueError, match="cardinality 17"):
            _build_settings(
                ja_model=ja_model_path_17,
                ja_thresholds=ja_thresholds_path,
                baseline=25,
            )

    def test_30_class_model_meets_30_baseline(
        self, ja_model_path_30: Path, ja_thresholds_path: Path
    ) -> None:
        settings = _build_settings(
            ja_model=ja_model_path_30,
            ja_thresholds=ja_thresholds_path,
            baseline=30,
        )
        assert settings.classification_backend == "joblib"

    def test_learning_machine_backend_bypasses_validator(
        self, ja_model_path_17: Path, ja_thresholds_path: Path, tmp_path: Path
    ) -> None:
        """The joblib cardinality check must short-circuit for other backends."""
        from recap_subworker.infra.config import Settings

        # learning_machine backend needs its own artefact dir
        lm_dir = tmp_path / "lm"
        lm_dir.mkdir()

        settings = Settings(  # type: ignore[arg-type]
            classification_backend="learning_machine",
            learning_machine_student_ja_dir=str(lm_dir),
            learning_machine_student_en_dir=str(lm_dir),
            genre_classifier_model_path_ja=str(ja_model_path_17),
            genre_baseline_cardinality=30,
        )
        assert settings.classification_backend == "learning_machine"

    def test_missing_model_file_does_not_crash(
        self, ja_thresholds_path: Path, tmp_path: Path
    ) -> None:
        """A missing file should let the existing ``is_file()`` guard fire at load,
        not the cardinality validator. The validator must skip silently."""
        missing = tmp_path / "never_existed.joblib"
        settings = _build_settings(
            ja_model=missing,
            ja_thresholds=ja_thresholds_path,
            baseline=30,
        )
        assert settings.genre_classifier_model_path_ja == str(missing)

    def test_baseline_env_override(
        self,
        ja_model_path_17: Path,
        ja_thresholds_path: Path,
        monkeypatch: pytest.MonkeyPatch,
    ) -> None:
        """Environment override of baseline cardinality is honoured."""
        monkeypatch.setenv("RECAP_SUBWORKER_GENRE_BASELINE_CARDINALITY", "20")
        with pytest.raises(ValueError, match="cardinality 17"):
            _build_settings(
                ja_model=ja_model_path_17,
                ja_thresholds=ja_thresholds_path,
            )
        monkeypatch.delenv("RECAP_SUBWORKER_GENRE_BASELINE_CARDINALITY", raising=False)
        # Absence of env leaves default 15 → 17 passes
        os.environ.pop("RECAP_SUBWORKER_GENRE_BASELINE_CARDINALITY", None)
