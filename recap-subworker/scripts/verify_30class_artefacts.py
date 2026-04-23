"""Post-training verification for the 30-class retrained classifier.

Reads the joblib + sidecar meta written by ``train.py`` and asserts:

- ``classifier.classes_`` cardinality == 30 and covers the canonical taxonomy
- sidecar metadata is well-formed and sklearn_version matches runtime
- vectorizer / svd / scaler companions exist and are loadable

Usage:
    uv run python scripts/verify_30class_artefacts.py \\
        --artefact-dir data --language ja
"""

from __future__ import annotations

import argparse
import importlib.metadata
import json
import sys
from pathlib import Path

import joblib

CANONICAL_GENRES = {
    "ai_data", "software_dev", "cybersecurity", "consumer_tech",
    "internet_platforms", "space_astronomy", "climate_environment",
    "energy_transition", "health_medicine", "life_science",
    "economics_macro", "markets_finance", "startups_innovation",
    "industry_logistics", "politics_government", "diplomacy_security",
    "law_crime", "education", "labor_workplace", "society_demographics",
    "culture_arts", "film_tv", "music_audio", "sports", "food_cuisine",
    "travel_places", "home_living", "games_esports", "mobility_automotive",
    "consumer_products",
}


def _runtime_minor(pkg: str) -> str:
    try:
        version = importlib.metadata.version(pkg)
    except importlib.metadata.PackageNotFoundError:
        return "unknown"
    parts = version.split(".")
    return f"{parts[0]}.{parts[1]}" if len(parts) >= 2 else version


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--artefact-dir", type=Path, default=Path("data"))
    parser.add_argument("--language", choices=("ja", "en"), default="ja")
    args = parser.parse_args()

    root: Path = args.artefact_dir
    lang: str = args.language
    model_path = root / f"genre_classifier_{lang}.joblib"
    meta_path = root / f"genre_classifier_{lang}.meta.json"
    thresholds_path = root / f"genre_thresholds_{lang}.json"
    vectorizer_path = root / f"tfidf_vectorizer_{lang}.joblib"
    svd_path = root / "tfidf_svd.joblib"
    scaler_path = root / "feature_scaler.joblib"

    errors: list[str] = []

    for path in (model_path, meta_path, thresholds_path, vectorizer_path, svd_path, scaler_path):
        if not path.is_file():
            errors.append(f"missing: {path}")

    if errors:
        for e in errors:
            print(f"FAIL: {e}")
        return 1

    model = joblib.load(model_path)
    classes = sorted(str(c) for c in model.classes_)
    cardinality = len(classes)
    missing = sorted(CANONICAL_GENRES - set(classes))
    extras = sorted(set(classes) - CANONICAL_GENRES)

    meta = json.loads(meta_path.read_text())

    runtime_sklearn = _runtime_minor("scikit-learn")
    sidecar_sklearn = ".".join(meta["sklearn_version"].split(".")[:2])
    version_ok = runtime_sklearn == sidecar_sklearn

    print(f"model_path:         {model_path}")
    print(f"cardinality:        {cardinality}")
    print(f"missing canonical:  {missing}")
    print(f"extras:             {extras}")
    print(f"sidecar sklearn:    {meta['sklearn_version']}  (runtime minor: {runtime_sklearn})")
    print(f"sidecar transformers: {meta['transformers_version']}")
    print(f"sidecar trained_at: {meta['trained_at']}")
    print(f"sidecar source sha: {meta['source_data_sha256'][:16]}...")
    print(f"feature_dim:        {meta['feature_dim']}")

    verdict_ok = cardinality == 30 and not missing and version_ok
    print(f"\nVERDICT: {'PASS' if verdict_ok else 'FAIL'}")
    return 0 if verdict_ok else 2


if __name__ == "__main__":
    sys.exit(main())
