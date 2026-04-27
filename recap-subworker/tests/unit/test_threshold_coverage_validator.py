"""RED test: Settings must fail-closed when classifier classes_ are not covered by the JA thresholds file.

Context: When the 30-class classifier was trained on 2026-04-23 the JA thresholds
file was already updated, but the legacy unsuffixed ``data/genre_thresholds.json``
sidecar (loaded as a fallback inside ``GenreClassifierService`` when the
language-specific file is absent) only had 17 keys. ``predict_batch`` falls
back to a hard-coded threshold of 0.5 for any class missing from the file —
which interacts badly with the v5 ``transformers`` buffer-junk regression that
skews ``predict_proba`` toward two dominant classes. The downstream effect is
``no_evidence`` for the missing classes and a collapsed recap genre space.

The fix is twofold:

1. Production code points at ``genre_thresholds_path_ja`` (already wired —
   see ``test_classifier_thresholds_wiring.py``).
2. The Settings boot validator now refuses to start when the loaded
   ``classifier.classes_`` includes any class that has no entry in the JA
   thresholds file. This eliminates the silent 0.5 fallback and forces every
   retrain step to update the thresholds artefact in lockstep.
"""

from __future__ import annotations

import json
from pathlib import Path

import joblib
import numpy as np
import pytest
from sklearn.linear_model import LogisticRegression


def _fit_tiny_model_with_classes(class_names: list[str]) -> LogisticRegression:
    rng = np.random.default_rng(11)
    x = rng.normal(size=(len(class_names) * 5, 8))
    y = np.array([c for c in class_names for _ in range(5)])
    return LogisticRegression(max_iter=200).fit(x, y)


THIRTY_CANONICAL_CLASSES = [
    "ai_data",
    "climate_environment",
    "consumer_products",
    "consumer_tech",
    "culture_arts",
    "cybersecurity",
    "diplomacy_security",
    "economics_macro",
    "education",
    "energy_transition",
    "film_tv",
    "food_cuisine",
    "games_esports",
    "health_medicine",
    "home_living",
    "industry_logistics",
    "internet_platforms",
    "labor_workplace",
    "law_crime",
    "life_science",
    "markets_finance",
    "mobility_automotive",
    "music_audio",
    "politics_government",
    "society_demographics",
    "software_dev",
    "space_astronomy",
    "sports",
    "startups_innovation",
    "travel_places",
]


@pytest.fixture
def ja_model_path_30(tmp_path: Path) -> Path:
    path = tmp_path / "genre_classifier_ja.joblib"
    joblib.dump(_fit_tiny_model_with_classes(THIRTY_CANONICAL_CLASSES), path)
    return path


@pytest.fixture
def ja_thresholds_full(tmp_path: Path) -> Path:
    path = tmp_path / "genre_thresholds_ja.json"
    path.write_text(json.dumps(dict.fromkeys(THIRTY_CANONICAL_CLASSES, 0.5)))
    return path


@pytest.fixture
def ja_thresholds_missing_13(tmp_path: Path) -> Path:
    """The 17-key file as it shipped before 2026-04-27."""
    path = tmp_path / "genre_thresholds_ja.json"
    seventeen = THIRTY_CANONICAL_CLASSES[:17]
    path.write_text(json.dumps(dict.fromkeys(seventeen, 0.5)))
    return path


def _build_settings(
    *,
    ja_model: Path,
    ja_thresholds: Path,
    baseline: int = 30,
):
    from recap_subworker.infra.config import Settings

    return Settings(  # type: ignore[arg-type]
        classification_backend="joblib",
        genre_classifier_model_path=str(ja_model),
        genre_classifier_model_path_ja=str(ja_model),
        genre_classifier_model_path_en="",
        genre_thresholds_path_ja=str(ja_thresholds),
        genre_thresholds_path_en="",
        tfidf_vectorizer_path_ja="",
        tfidf_vectorizer_path_en="",
        genre_baseline_cardinality=baseline,
    )


class TestThresholdCoverageValidator:
    def test_full_30_coverage_passes(
        self, ja_model_path_30: Path, ja_thresholds_full: Path
    ) -> None:
        settings = _build_settings(
            ja_model=ja_model_path_30,
            ja_thresholds=ja_thresholds_full,
        )
        assert settings.genre_baseline_cardinality == 30

    def test_partial_coverage_raises(
        self, ja_model_path_30: Path, ja_thresholds_missing_13: Path
    ) -> None:
        with pytest.raises(ValueError, match="threshold coverage"):
            _build_settings(
                ja_model=ja_model_path_30,
                ja_thresholds=ja_thresholds_missing_13,
            )

    def test_partial_coverage_lists_missing_classes(
        self, ja_model_path_30: Path, ja_thresholds_missing_13: Path
    ) -> None:
        with pytest.raises(ValueError) as exc:
            _build_settings(
                ja_model=ja_model_path_30,
                ja_thresholds=ja_thresholds_missing_13,
            )
        # Operators must see exactly which classes were skipped.
        msg = str(exc.value)
        for missing in THIRTY_CANONICAL_CLASSES[17:]:
            assert missing in msg, (
                f"validator must enumerate missing class {missing!r} in error message; got {msg!r}"
            )

    def test_thresholds_file_missing_skips_coverage_check(
        self, ja_model_path_30: Path, tmp_path: Path
    ) -> None:
        """Missing thresholds file is handled by the existing ``_validate_joblib_artifacts``
        guard for directory-shaped binds. The coverage check must not double-fire when
        the file simply does not exist (e.g. dev workstation without artefact)."""
        from recap_subworker.infra.config import Settings

        non_existent = tmp_path / "missing_thresholds.json"
        # Should not raise from the coverage validator. The ``_ensure_model`` runtime
        # path will still warn, but boot stays green so dev iteration is not blocked.
        settings = Settings(  # type: ignore[arg-type]
            classification_backend="joblib",
            genre_classifier_model_path=str(ja_model_path_30),
            genre_classifier_model_path_ja=str(ja_model_path_30),
            genre_classifier_model_path_en="",
            genre_thresholds_path_ja=str(non_existent),
            genre_thresholds_path_en="",
            tfidf_vectorizer_path_ja="",
            tfidf_vectorizer_path_en="",
            genre_baseline_cardinality=30,
        )
        assert settings.genre_thresholds_path_ja == str(non_existent)

    def test_learning_machine_backend_bypasses_coverage_check(
        self, ja_model_path_30: Path, ja_thresholds_missing_13: Path, tmp_path: Path
    ) -> None:
        """Joblib-only validator must short-circuit for learning_machine backend."""
        from recap_subworker.infra.config import Settings

        lm_dir = tmp_path / "lm"
        lm_dir.mkdir()
        settings = Settings(  # type: ignore[arg-type]
            classification_backend="learning_machine",
            learning_machine_student_ja_dir=str(lm_dir),
            learning_machine_student_en_dir=str(lm_dir),
            genre_classifier_model_path=str(ja_model_path_30),
            genre_classifier_model_path_ja=str(ja_model_path_30),
            genre_thresholds_path_ja=str(ja_thresholds_missing_13),
            genre_baseline_cardinality=30,
        )
        assert settings.classification_backend == "learning_machine"
