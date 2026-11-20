"""Genre Classification Training Script.

Trains a genre classifier with configurable hyperparameters.
Can load optimal parameters from a JSON file (output of optimize_hyperparams.py).
Supports loading augmented training data alongside original data.

Usage:
    uv run python -m recap_subworker.infra.classifier.train [--params data/optimal_params.json]
    uv run python -m recap_subworker.infra.classifier.train --data data/training_data_augmented.csv

ADR-000835 stage 2/3: writes a sidecar ``genre_classifier_<lang>.meta.json``
capturing sklearn / transformers / sentence-transformers versions, class set,
feature dimension, and source data sha256 so the runtime can fail-closed on
drift.
"""

import argparse
import asyncio
import dataclasses
import hashlib
import importlib.metadata
import json
import logging
from collections.abc import Sequence
from datetime import UTC, datetime
from pathlib import Path
from typing import Literal, Self

import joblib
import numpy as np
import pandas as pd
from sklearn.base import BaseEstimator, TransformerMixin
from sklearn.calibration import CalibratedClassifierCV
from sklearn.decomposition import TruncatedSVD
from sklearn.feature_extraction.text import TfidfVectorizer
from sklearn.linear_model import LogisticRegression
from sklearn.metrics import classification_report, f1_score
from sklearn.model_selection import cross_val_predict, train_test_split
from sklearn.preprocessing import StandardScaler

from recap_subworker.infra.config import Settings, get_settings
from recap_subworker.services.embedder import Embedder, EmbedderConfig

logger = logging.getLogger(__name__)


def _installed_version(pkg: str) -> str:
    try:
        return importlib.metadata.version(pkg)
    except importlib.metadata.PackageNotFoundError:
        return "unknown"


# Default hyperparameters (can be overridden by --params)
DEFAULT_HYPERPARAMS = {
    "C": 0.1,
    "max_features": 1000,
    "svd_components": 200,
    "min_df": 2,
    "max_df": 0.95,
}
_MIN_SVD_FEATURES = 2
_MIN_GENRE_SAMPLES = 20
_OVERFITTING_GAP_THRESHOLD = 0.10


def load_hyperparams(params_path: Path | None) -> dict:
    """Load hyperparameters from JSON file or use defaults.

    Args:
        params_path: Path to optimal_params.json (from optimize_hyperparams.py)

    Returns:
        Dictionary of hyperparameters
    """
    if params_path is None or not params_path.exists():
        logger.info("Using default hyperparameters")
        return DEFAULT_HYPERPARAMS.copy()

    with params_path.open() as f:
        data = json.load(f)

    # Extract best_params if it's the output of optimize_hyperparams.py
    if "best_params" in data:
        params = data["best_params"]
        logger.info("Loaded optimal hyperparameters from %s", params_path)
        logger.info("  CV F1: %s", data.get("best_cv_f1", "N/A"))
        logger.info("  Train/Test gap: %s", data.get("train_test_gap", "N/A"))
    else:
        params = data

    # Merge with defaults for any missing keys
    result = DEFAULT_HYPERPARAMS.copy()
    result.update(params)
    return result


def clamp_svd_components(requested: int, *, n_samples: int, n_features: int) -> int:
    """Cap TruncatedSVD rank to what sklearn 1.8.0 will actually fit.

    Randomized SVD accepts ``n_components <= n_features`` and the matrix rank
    is ``min(n_samples, n_features)``. Passing the hyperparam unchanged crashes
    training when TF-IDF yields a smaller vocabulary than ``svd_components``.
    """
    if n_features < _MIN_SVD_FEATURES:
        msg = f"TruncatedSVD requires at least 2 TF-IDF features, got n_features={n_features}"
        raise ValueError(msg)
    if requested < 1:
        msg = f"svd_components must be >= 1, got {requested}"
        raise ValueError(msg)
    cap = min(int(n_samples), int(n_features))
    if cap < 1:
        msg = f"TruncatedSVD requires at least 1 sample, got n_samples={n_samples}"
        raise ValueError(msg)
    return min(int(requested), cap)


# --- Custom Transformer for Embeddings ---
class EmbeddingTransformer(BaseEstimator, TransformerMixin):
    def __init__(self, embedder: Embedder) -> None:
        self.embedder = embedder

    def fit(self, X: pd.Series | Sequence[str], y: pd.Series | None = None) -> Self:  # noqa: N803  # sklearn TransformerMixin API
        return self

    def transform(self, X: pd.Series | Sequence[str]) -> np.ndarray:  # noqa: N803  # sklearn TransformerMixin API
        # X is list of strings or Series
        texts = X.tolist() if isinstance(X, pd.Series) else list(X)
        # Prepend passage: for E5 models if needed, but Embedder might handle it?
        # Sentences from DB might be raw.
        # Embedder.encode expects "passage: " prefix for E5 if asymmetric?
        # Let's assume Embedder handles it or we do it here.
        # E5 expects "query: " or "passage: ". For classification, usually "passage: ".
        # But wait, existing code in classifier.py does: input_texts = [f"passage: {t}" for t in texts]
        # So we should do the same.
        input_texts = [f"passage: {t}" for t in texts]
        return self.embedder.encode(input_texts)


# --- Training Script ---


@dataclasses.dataclass
class _FeatureArtifacts:
    x_train_combined: np.ndarray
    x_test_combined: np.ndarray
    tfidf: TfidfVectorizer
    svd: TruncatedSVD
    scaler: StandardScaler
    embedder_device: str


@dataclasses.dataclass
class _TrainingSaveBundle:
    clf: CalibratedClassifierCV
    features: _FeatureArtifacts
    thresholds: dict[str, float]
    feature_dim: int
    data_path: Path
    settings: Settings


def _load_training_dataframe(data_path: Path, augmented_paths: list[Path] | None) -> pd.DataFrame:
    logger.info("Loading data from %s...", data_path)
    df = pd.read_csv(data_path)
    df = df.dropna(subset=["content", "genre"])
    logger.info("  Loaded %d samples", len(df))
    # Load and merge augmented data if provided
    if augmented_paths:
        aug_dfs = []
        for aug_path in augmented_paths:
            if aug_path.exists():
                aug_df = pd.read_csv(aug_path)
                aug_df = aug_df.dropna(subset=["content", "genre"])
                aug_dfs.append(aug_df)
                logger.info("  Loaded %d augmented samples from %s", len(aug_df), aug_path)
            else:
                logger.info("  WARNING: Augmented file not found: %s", aug_path)
        if aug_dfs:
            all_dfs = [df, *aug_dfs]
            df = pd.concat(all_dfs, ignore_index=True)
            df = df[["content", "genre"]]
            logger.info("  Total samples after merge: %d", len(df))
    counts = df["genre"].value_counts()
    valid_genres = counts[counts >= _MIN_GENRE_SAMPLES].index
    logger.info(
        "Filtering genres with >= %d samples. Kept: %d", _MIN_GENRE_SAMPLES, len(valid_genres)
    )
    return df[df["genre"].isin(valid_genres)]


def _extract_features(
    x_train: pd.Series,
    x_test: pd.Series,
    hyperparams: dict,
    settings: Settings,
) -> _FeatureArtifacts:
    logger.info("Initializing Embedder...")
    logger.info("Initializing Embedder...")

    embedder_config = dataclasses.replace(
        EmbedderConfig.from_settings(settings),
        device="cuda",  # Force CUDA per user authorization
        batch_size=4,  # Reduced to 4 for extreme safety
        cache_size=1000,  # Default cache size
    )
    embedder = Embedder(embedder_config)

    logger.info("Generating embeddings for training set...")
    emb_transformer = EmbeddingTransformer(embedder)
    X_train_emb = emb_transformer.transform(x_train)  # noqa: N806  # ML matrix convention
    X_test_emb = emb_transformer.transform(x_test)  # noqa: N806  # ML matrix convention

    logger.info("Generating TF-IDF features...")
    tfidf = TfidfVectorizer(
        max_features=hyperparams["max_features"],
        sublinear_tf=True,
        min_df=hyperparams["min_df"],
        max_df=hyperparams["max_df"],
        ngram_range=(1, 2),
    )
    X_train_tfidf = tfidf.fit_transform(x_train)  # noqa: N806  # ML matrix convention
    X_test_tfidf = tfidf.transform(x_test)  # noqa: N806  # ML matrix convention

    n_samples, n_features = X_train_tfidf.shape
    n_components = clamp_svd_components(
        hyperparams["svd_components"],
        n_samples=n_samples,
        n_features=n_features,
    )
    if n_components != hyperparams["svd_components"]:
        logger.info(
            "Clamped svd_components from %s to %s (n_samples=%s n_features=%s)",
            hyperparams["svd_components"],
            n_components,
            n_samples,
            n_features,
        )
    logger.info("Applying TruncatedSVD to TF-IDF features...")
    svd = TruncatedSVD(n_components=n_components, random_state=42)
    X_train_tfidf_svd = svd.fit_transform(X_train_tfidf)  # noqa: N806  # ML matrix convention
    X_test_tfidf_svd = svd.transform(X_test_tfidf)  # noqa: N806  # ML matrix convention
    logger.info("SVD explained variance ratio: %.4f", svd.explained_variance_ratio_.sum())
    X_train_combined_raw = np.hstack((X_train_emb, X_train_tfidf_svd))  # noqa: N806  # ML matrix convention
    X_test_combined_raw = np.hstack((X_test_emb, X_test_tfidf_svd))  # noqa: N806  # ML matrix convention

    logger.info("Normalizing combined features...")
    scaler = StandardScaler()
    X_train_combined = scaler.fit_transform(X_train_combined_raw)  # noqa: N806  # ML matrix convention
    X_test_combined = scaler.transform(X_test_combined_raw)  # noqa: N806  # ML matrix convention
    logger.info("Features shape: %s", X_train_combined.shape)

    return _FeatureArtifacts(
        x_train_combined=X_train_combined,
        x_test_combined=X_test_combined,
        tfidf=tfidf,
        svd=svd,
        scaler=scaler,
        embedder_device=str(embedder_config.device),
    )


def _filter_label_issues(
    x_train_combined: np.ndarray, y_train: pd.Series
) -> tuple[np.ndarray, pd.Series]:
    logger.info("Running Cleanlab to identify label issues...")
    from cleanlab.filter import (  # noqa: PLC0415  # optional train extra; defer import until training runs
        find_label_issues,
    )

    clf_cv = LogisticRegression(max_iter=1000, solver="lbfgs", class_weight="balanced")
    try:
        pred_probs = cross_val_predict(
            clf_cv, x_train_combined, y_train, cv=3, method="predict_proba", n_jobs=-1
        )
        issue_indices = find_label_issues(
            labels=y_train, pred_probs=pred_probs, return_indices_ranked_by="self_confidence"
        )
        logger.info("Cleanlab identified %d label issues.", len(issue_indices))
        if len(issue_indices) > 0:
            keep_mask = np.ones(len(y_train), dtype=bool)
            keep_mask[issue_indices] = False
            X_train_cleaned = x_train_combined[keep_mask]  # noqa: N806  # ML matrix convention
            y_train_cleaned = y_train.iloc[keep_mask]
            logger.info(
                "Removed %d samples. Training size: %d -> %d",
                len(issue_indices),
                len(y_train),
                len(y_train_cleaned),
            )
            result = (X_train_cleaned, y_train_cleaned)
        else:
            result = (x_train_combined, y_train)
    except Exception as e:  # noqa: BLE001  # cleanlab is best-effort; fall back to uncleaned training data
        logger.info("Cleanlab failed: %s", e)
        logger.info("Falling back to original training data.")
        return x_train_combined, y_train
    else:
        return result


def _train_and_evaluate(
    features: _FeatureArtifacts,
    x_train_final: np.ndarray,
    y_train_final: pd.Series,
    y_test: pd.Series,
    hyperparams: dict,
) -> CalibratedClassifierCV:
    logger.info("Training Logistic Regression with Platt Scaling calibration...")
    base_clf = LogisticRegression(
        max_iter=1000,
        solver="lbfgs",
        class_weight="balanced",
        C=hyperparams["C"],
    )
    clf = CalibratedClassifierCV(
        estimator=base_clf,
        method="sigmoid",  # Platt scaling
        cv=3,
    )
    clf.fit(x_train_final, y_train_final)

    y_pred = clf.predict(features.x_test_combined)
    logger.info("Test Set Classification Report:")
    logger.info(classification_report(y_test, y_pred))

    y_train_pred = clf.predict(x_train_final)
    train_f1 = f1_score(y_train_final, y_train_pred, average="macro")
    test_f1 = f1_score(y_test, y_pred, average="macro")
    train_test_gap = train_f1 - test_f1

    logger.info("\n%s", "=" * 50)
    logger.info("OVERFITTING ANALYSIS")
    logger.info("=" * 50)
    logger.info("Train Macro F1: %.4f", train_f1)
    logger.info("Test Macro F1:  %.4f", test_f1)
    logger.info("Train/Test Gap: %.4f", train_test_gap)
    if train_test_gap < _OVERFITTING_GAP_THRESHOLD:
        logger.info("Status: OK (gap < %.2f)", _OVERFITTING_GAP_THRESHOLD)
    else:
        logger.info(
            "Status: WARNING - potential overfitting (gap >= %.2f)", _OVERFITTING_GAP_THRESHOLD
        )
    logger.info("%s\n", "=" * 50)
    return clf


def _optimize_thresholds(
    clf: CalibratedClassifierCV, x_test_combined: np.ndarray, y_test: pd.Series
) -> dict[str, float]:
    logger.info("Optimizing thresholds...")
    probs = clf.predict_proba(x_test_combined)
    classes = clf.classes_

    thresholds: dict[str, float] = {}
    best_f1s = {}
    y_test_dummies = pd.get_dummies(y_test)

    for i, cls in enumerate(classes):
        if cls not in y_test_dummies.columns:
            continue

        y_true_binary = y_test_dummies[cls]
        y_prob = probs[:, i]
        best_t = 0.5
        best_f1 = 0.0

        for t in np.arange(0.1, 0.95, 0.05):
            y_pred_binary = (y_prob >= t).astype(int)
            score = f1_score(y_true_binary, y_pred_binary)
            if score > best_f1:
                best_f1 = score
                best_t = t

        thresholds[cls] = float(best_t)
        best_f1s[cls] = best_f1
        logger.info("Class %s: Best Threshold=%.2f, F1=%.2f", cls, best_t, best_f1)
    return thresholds


async def _save_training_artifacts(
    out_dir: Path, language: Literal["ja", "en"], bundle: _TrainingSaveBundle
) -> None:
    logger.info("Saving model and artifacts...")
    await asyncio.to_thread(out_dir.mkdir, parents=True, exist_ok=True)

    model_path = out_dir / f"genre_classifier_{language}.joblib"
    vectorizer_path = out_dir / f"tfidf_vectorizer_{language}.joblib"
    thresholds_path = out_dir / f"genre_thresholds_{language}.json"
    svd_path = out_dir / "tfidf_svd.joblib"
    scaler_path = out_dir / "feature_scaler.joblib"
    meta_path = out_dir / f"genre_classifier_{language}.meta.json"

    joblib.dump(bundle.features.tfidf, vectorizer_path)
    joblib.dump(bundle.features.svd, svd_path)
    joblib.dump(bundle.features.scaler, scaler_path)
    joblib.dump(bundle.clf, model_path)

    await asyncio.to_thread(thresholds_path.write_text, json.dumps(bundle.thresholds, indent=2))

    # ADR-000835 stage 3: sidecar metadata so the runtime can fail-closed on
    # sklearn / transformers drift without re-loading the joblib to inspect.
    source_bytes = await asyncio.to_thread(bundle.data_path.read_bytes)
    metadata = {
        "sklearn_version": _installed_version("scikit-learn"),
        "transformers_version": _installed_version("transformers"),
        "sentence_transformers_version": _installed_version("sentence-transformers"),
        "trained_at": datetime.now(UTC).isoformat(),
        "language": language,
        "classes": sorted(str(c) for c in bundle.clf.classes_.tolist()),
        "feature_dim": bundle.feature_dim,
        "source_data_sha256": hashlib.sha256(source_bytes).hexdigest(),
        "embedding_model_id": bundle.settings.model_id,
        "device": bundle.features.embedder_device,
    }
    await asyncio.to_thread(meta_path.write_text, json.dumps(metadata, indent=2))

    logger.info("Saved artifacts:")
    logger.info("  - %s", vectorizer_path)
    logger.info("  - %s", svd_path)
    logger.info("  - %s", scaler_path)
    logger.info("  - %s", model_path)
    logger.info("  - %s", thresholds_path)
    logger.info("  - %s", meta_path)
    logger.info("Done!")


async def main(
    params_path: Path | None = None,
    data_path: Path | None = None,
    augmented_paths: list[Path] | None = None,
    language: Literal["ja", "en"] = "ja",
    output_dir: Path | None = None,
) -> None:
    """Main training function.

    Args:
        params_path: Optional path to hyperparameters JSON file
        data_path: Path to training data CSV (default: data/training_data.csv)
        augmented_paths: Optional list of augmented data CSV files to merge
        language: Target language; controls output filename suffix (``_ja``/``_en``)
        output_dir: Directory where artefacts and the sidecar meta are written
    """
    settings = get_settings()

    if data_path is None:
        data_path = Path("data/training_data.csv")

    if not data_path.exists():
        logger.info("Data not found: %s", data_path)
        return

    hyperparams = load_hyperparams(params_path)
    logger.info("\nHyperparameters:")
    for k, v in hyperparams.items():
        logger.info("  %s: %s", k, v)
    logger.info("")

    df = _load_training_dataframe(data_path, augmented_paths)
    X = df["content"]  # noqa: N806  # ML matrix convention
    y = df["genre"]

    logger.info("Splitting data...")
    X_train, X_test, y_train, y_test = train_test_split(  # noqa: N806  # ML matrix convention
        X, y, test_size=0.2, random_state=42, stratify=y
    )

    features = _extract_features(X_train, X_test, hyperparams, settings)
    X_train_final, y_train_final = _filter_label_issues(features.x_train_combined, y_train)  # noqa: N806  # ML matrix convention
    clf = _train_and_evaluate(features, X_train_final, y_train_final, y_test, hyperparams)
    thresholds = _optimize_thresholds(clf, features.x_test_combined, y_test)

    out_dir = output_dir if output_dir is not None else Path("data")
    await _save_training_artifacts(
        out_dir,
        language,
        _TrainingSaveBundle(
            clf=clf,
            features=features,
            thresholds=thresholds,
            feature_dim=int(X_train_final.shape[1]),
            data_path=data_path,
            settings=settings,
        ),
    )


if __name__ == "__main__":
    parser = argparse.ArgumentParser(
        description="Train genre classification model with configurable hyperparameters"
    )
    parser.add_argument(
        "--params",
        type=Path,
        default=None,
        help="Path to hyperparameters JSON (output of optimize_hyperparams.py)",
    )
    parser.add_argument(
        "--data",
        type=Path,
        default=None,
        help="Path to training data CSV (default: data/training_data.csv)",
    )
    parser.add_argument(
        "--augmented",
        type=Path,
        nargs="*",
        default=None,
        help="Additional augmented data CSV files to merge with training data",
    )
    parser.add_argument(
        "--language",
        choices=("ja", "en"),
        default="ja",
        help="Target language — controls output filename suffix (_ja / _en)",
    )
    parser.add_argument(
        "--output-dir",
        type=Path,
        default=None,
        help="Directory to write artefacts (default: data/)",
    )
    args = parser.parse_args()
    asyncio.run(
        main(
            params_path=args.params,
            data_path=args.data,
            augmented_paths=args.augmented,
            language=args.language,
            output_dir=args.output_dir,
        )
    )
