"""Slow integration smoke test: end-to-end train.py on a tiny synthetic CSV.

Runs the real training entry point with a 30-class tiny dataset on CPU so the
sklearn pipeline, sidecar writer, and artefact naming are exercised without
needing a GPU. Marked ``slow`` — excluded from default ``pytest -m 'not slow'``.
"""

from __future__ import annotations

import json
import subprocess
import sys
from pathlib import Path

import joblib
import pandas as pd
import pytest

CANONICAL_GENRES = [
    "ai_data",
    "software_dev",
    "cybersecurity",
    "consumer_tech",
    "internet_platforms",
    "space_astronomy",
    "climate_environment",
    "energy_transition",
    "health_medicine",
    "life_science",
    "economics_macro",
    "markets_finance",
    "startups_innovation",
    "industry_logistics",
    "politics_government",
    "diplomacy_security",
    "law_crime",
    "education",
    "labor_workplace",
    "society_demographics",
    "culture_arts",
    "film_tv",
    "music_audio",
    "sports",
    "food_cuisine",
    "travel_places",
    "home_living",
    "games_esports",
    "mobility_automotive",
    "consumer_products",
]


@pytest.fixture
def tiny_30class_csv(tmp_path: Path) -> Path:
    """25 rows × 30 classes = 750 rows, minimum viable for stratify + cv=3."""
    rows = []
    for genre in CANONICAL_GENRES:
        for i in range(25):
            rows.append(
                {
                    "content": f"{genre} article fragment {i} " * 20,
                    "genre": genre,
                }
            )
    csv = tmp_path / "training_data_30class.csv"
    pd.DataFrame(rows).to_csv(csv, index=False)
    return csv


@pytest.mark.slow
def test_train_smoke_produces_30_class_artefacts(
    tiny_30class_csv: Path, tmp_path: Path, monkeypatch: pytest.MonkeyPatch
) -> None:
    out_dir = tmp_path / "artefacts"
    out_dir.mkdir()

    # Force CPU + hash embedder so the smoke test does not pull BGE-M3
    monkeypatch.setenv("RECAP_SUBWORKER_MODEL_BACKEND", "hash")
    monkeypatch.setenv("RECAP_SUBWORKER_DEVICE", "cpu")

    cmd = [
        sys.executable,
        "-m",
        "recap_subworker.infra.classifier.train",
        "--data",
        str(tiny_30class_csv),
        "--language",
        "ja",
        "--output-dir",
        str(out_dir),
    ]
    result = subprocess.run(cmd, check=False, capture_output=True, text=True, timeout=600)

    assert result.returncode == 0, (
        f"train.py exited {result.returncode}\nSTDOUT:\n{result.stdout}\nSTDERR:\n{result.stderr}"
    )

    model_path = out_dir / "genre_classifier_ja.joblib"
    meta_path = out_dir / "genre_classifier_ja.meta.json"
    thresholds_path = out_dir / "genre_thresholds_ja.json"
    vectorizer_path = out_dir / "tfidf_vectorizer_ja.joblib"
    svd_path = out_dir / "tfidf_svd.joblib"
    scaler_path = out_dir / "feature_scaler.joblib"

    for path in (
        model_path,
        meta_path,
        thresholds_path,
        vectorizer_path,
        svd_path,
        scaler_path,
    ):
        assert path.is_file(), f"missing artefact: {path}"

    model = joblib.load(model_path)
    assert len(model.classes_) == 30

    meta = json.loads(meta_path.read_text())
    for field in (
        "sklearn_version",
        "transformers_version",
        "trained_at",
        "language",
        "classes",
        "feature_dim",
        "source_data_sha256",
    ):
        assert field in meta, f"missing meta field: {field}"
    assert meta["language"] == "ja"
    assert len(meta["classes"]) == 30
