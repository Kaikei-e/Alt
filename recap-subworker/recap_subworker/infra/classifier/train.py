"""
Genre Classification Training Script.

Trains a genre classifier with configurable hyperparameters.
Can load optimal parameters from a JSON file (output of optimize_hyperparams.py).
Supports loading augmented training data alongside original data.

Usage:
    uv run python -m recap_subworker.infra.classifier.train [--params data/optimal_params.json]
    uv run python -m recap_subworker.infra.classifier.train --data data/training_data_augmented.csv
"""

import argparse
import asyncio
import joblib
import json
import numpy as np
import pandas as pd
from pathlib import Path
from sklearn.calibration import CalibratedClassifierCV
from sklearn.decomposition import TruncatedSVD
from sklearn.feature_extraction.text import TfidfVectorizer
from sklearn.linear_model import LogisticRegression
from sklearn.metrics import classification_report, f1_score
from sklearn.model_selection import train_test_split
from sklearn.preprocessing import StandardScaler
from sklearn.base import BaseEstimator, TransformerMixin

from recap_subworker.services.embedder import Embedder
from recap_subworker.infra.config import get_settings


# Default hyperparameters (can be overridden by --params)
DEFAULT_HYPERPARAMS = {
    "C": 0.1,
    "max_features": 1000,
    "svd_components": 200,
    "min_df": 2,
    "max_df": 0.95,
}


def load_hyperparams(params_path: Path | None) -> dict:
    """Load hyperparameters from JSON file or use defaults.

    Args:
        params_path: Path to optimal_params.json (from optimize_hyperparams.py)

    Returns:
        Dictionary of hyperparameters
    """
    if params_path is None or not params_path.exists():
        print("Using default hyperparameters")
        return DEFAULT_HYPERPARAMS.copy()

    with open(params_path) as f:
        data = json.load(f)

    # Extract best_params if it's the output of optimize_hyperparams.py
    if "best_params" in data:
        params = data["best_params"]
        print(f"Loaded optimal hyperparameters from {params_path}")
        print(f"  CV F1: {data.get('best_cv_f1', 'N/A')}")
        print(f"  Train/Test gap: {data.get('train_test_gap', 'N/A')}")
    else:
        params = data

    # Merge with defaults for any missing keys
    result = DEFAULT_HYPERPARAMS.copy()
    result.update(params)
    return result

# --- Custom Transformer for Embeddings ---
class EmbeddingTransformer(BaseEstimator, TransformerMixin):
    def __init__(self, embedder: Embedder):
        self.embedder = embedder

    def fit(self, X, y=None):
        return self

    def transform(self, X):
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

async def main(
    params_path: Path | None = None,
    data_path: Path | None = None,
    augmented_paths: list[Path] | None = None,
):
    """Main training function.

    Args:
        params_path: Optional path to hyperparameters JSON file
        data_path: Path to training data CSV (default: data/training_data.csv)
        augmented_paths: Optional list of augmented data CSV files to merge
    """
    settings = get_settings()

    # Determine data path
    if data_path is None:
        data_path = Path("data/training_data.csv")

    if not data_path.exists():
        print(f"Data not found: {data_path}")
        return

    # Load hyperparameters
    hyperparams = load_hyperparams(params_path)
    print("\nHyperparameters:")
    for k, v in hyperparams.items():
        print(f"  {k}: {v}")
    print()

    print(f"Loading data from {data_path}...")
    df = pd.read_csv(data_path)
    df = df.dropna(subset=['content', 'genre'])
    print(f"  Loaded {len(df)} samples")

    # Load and merge augmented data if provided
    if augmented_paths:
        aug_dfs = []
        for aug_path in augmented_paths:
            if aug_path.exists():
                aug_df = pd.read_csv(aug_path)
                aug_df = aug_df.dropna(subset=['content', 'genre'])
                aug_dfs.append(aug_df)
                print(f"  Loaded {len(aug_df)} augmented samples from {aug_path}")
            else:
                print(f"  WARNING: Augmented file not found: {aug_path}")

        if aug_dfs:
            # Combine all dataframes
            all_dfs = [df] + aug_dfs
            df = pd.concat(all_dfs, ignore_index=True)
            # Keep only essential columns
            df = df[['content', 'genre']]
            print(f"  Total samples after merge: {len(df)}")

    # Filter out rare classes if any (less than 10 samples?)
    # genre distribution earlier showed many with 4 samples.
    # We should probably group them into 'other' or drop them?
    # For now, let's keep them and see (LR might fail with few samples).
    # Or maybe drop classes with < 50 samples?
    counts = df['genre'].value_counts()
    valid_genres = counts[counts >= 20].index
    print(f"Filtering genres with >= 20 samples. Kept: {len(valid_genres)}")
    df = df[df['genre'].isin(valid_genres)]

    # Verification limit removed


    X = df['content']
    y = df['genre']

    print("Splitting data...")
    X_train, X_test, y_train, y_test = train_test_split(X, y, test_size=0.2, random_state=42, stratify=y)

    print("Initializing Embedder...")
    # Initialize embedder (this might be heavy)
    print("Initializing Embedder...")
    from recap_subworker.services.embedder import EmbedderConfig

    # Construct config from settings
    embedder_config = EmbedderConfig(
        model_id=settings.model_id,
        backend=settings.model_backend,
        device="cuda", # Force CUDA per user authorization
        distill_model_id=settings.distill_model_id,
        batch_size=4, # Reduced to 4 for extreme safety
        cache_size=1000 # Default cache size
    )
    embedder = Embedder(embedder_config)

    # Transform embeddings first to separate them (optional, but cleaner for pipeline if mixed)
    # Actually, let's just compute embeddings once for Train and Test to save time during tuning.
    print("Generating embeddings for training set...")
    emb_transformer = EmbeddingTransformer(embedder)
    X_train_emb = emb_transformer.transform(X_train)
    X_test_emb = emb_transformer.transform(X_test)

    print("Generating TF-IDF features...")
    tfidf = TfidfVectorizer(
        max_features=hyperparams["max_features"],
        sublinear_tf=True,
        min_df=hyperparams["min_df"],
        max_df=hyperparams["max_df"],
        ngram_range=(1, 2),
    )
    X_train_tfidf = tfidf.fit_transform(X_train)
    X_test_tfidf = tfidf.transform(X_test)

    # Apply TruncatedSVD to reduce TF-IDF dimensionality
    print("Applying TruncatedSVD to TF-IDF features...")
    svd = TruncatedSVD(n_components=hyperparams["svd_components"], random_state=42)
    X_train_tfidf_svd = svd.fit_transform(X_train_tfidf)
    X_test_tfidf_svd = svd.transform(X_test_tfidf)
    print(f"SVD explained variance ratio: {svd.explained_variance_ratio_.sum():.4f}")

    # Concatenate embeddings with SVD-reduced TF-IDF
    X_train_combined_raw = np.hstack((X_train_emb, X_train_tfidf_svd))
    X_test_combined_raw = np.hstack((X_test_emb, X_test_tfidf_svd))

    # Apply StandardScaler to normalize combined features
    print("Normalizing combined features...")
    scaler = StandardScaler()
    X_train_combined = scaler.fit_transform(X_train_combined_raw)
    X_test_combined = scaler.transform(X_test_combined_raw)

    print(f"Features shape: {X_train_combined.shape}")

    # --- Cleanlab Integration ---
    print("Running Cleanlab to identify label issues...")
    from cleanlab.filter import find_label_issues
    from sklearn.model_selection import cross_val_predict

    # We need a temporary classifier for cross-validation
    # Use a simpler/faster one or the same one? LR is fast enough.
    # Note: multi_class parameter was removed in sklearn 1.8
    clf_cv = LogisticRegression(max_iter=1000, solver='lbfgs', class_weight='balanced')

    try:
        # Get out-of-sample predicted probabilities
        # This might fail if some classes have < 2 samples in a fold, but we filtered min 20.
        pred_probs = cross_val_predict(clf_cv, X_train_combined, y_train, cv=3, method='predict_proba', n_jobs=-1)

        # Find label issues
        issue_indices = find_label_issues(
            labels=y_train,
            pred_probs=pred_probs,
            return_indices_ranked_by='self_confidence'
        )

        print(f"Cleanlab identified {len(issue_indices)} label issues.")

        if len(issue_indices) > 0:
            # Create a mask to keep non-issues
            # X_train is a Series, X_train_combined is ndarray
            # We need to filter X_train_combined and y_train

            # Identify indices to keep
            keep_mask = np.ones(len(y_train), dtype=bool)
            # issue_indices are relative to X_train positions (0 to len(X_train)-1)
            keep_mask[issue_indices] = False

            X_train_cleaned = X_train_combined[keep_mask]
            y_train_cleaned = y_train.iloc[keep_mask]

            print(f"Removed {len(issue_indices)} samples. Training size: {len(y_train)} -> {len(y_train_cleaned)}")

            # Update for final training
            X_train_final = X_train_cleaned
            y_train_final = y_train_cleaned
        else:
            X_train_final = X_train_combined
            y_train_final = y_train

    except Exception as e:
        print(f"Cleanlab failed: {e}")
        print("Falling back to original training data.")
        X_train_final = X_train_combined
        y_train_final = y_train

    # Train Model with Probability Calibration (Platt Scaling)
    print("Training Logistic Regression with Platt Scaling calibration...")
    # Note: multi_class parameter was removed in sklearn 1.8
    base_clf = LogisticRegression(
        max_iter=1000,
        solver='lbfgs',
        class_weight='balanced',
        C=hyperparams["C"],
    )
    # Wrap with CalibratedClassifierCV for better probability estimates
    clf = CalibratedClassifierCV(
        estimator=base_clf,
        method='sigmoid',  # Platt scaling
        cv=3,
    )
    clf.fit(X_train_final, y_train_final)

    # Evaluate on test set
    y_pred = clf.predict(X_test_combined)
    print("Test Set Classification Report:")
    print(classification_report(y_test, y_pred))

    # Calculate Train/Test gap (overfitting indicator)
    y_train_pred = clf.predict(X_train_final)
    train_f1 = f1_score(y_train_final, y_train_pred, average='macro')
    test_f1 = f1_score(y_test, y_pred, average='macro')
    train_test_gap = train_f1 - test_f1

    print("\n" + "=" * 50)
    print("OVERFITTING ANALYSIS")
    print("=" * 50)
    print(f"Train Macro F1: {train_f1:.4f}")
    print(f"Test Macro F1:  {test_f1:.4f}")
    print(f"Train/Test Gap: {train_test_gap:.4f}")
    if train_test_gap < 0.10:
        print("Status: OK (gap < 0.10)")
    else:
        print("Status: WARNING - potential overfitting (gap >= 0.10)")
    print("=" * 50 + "\n")

    # Validate/Optimize Thresholds
    print("Optimizing thresholds...")
    probs = clf.predict_proba(X_test_combined)
    classes = clf.classes_

    thresholds = {}
    best_f1s = {}

    # For each class, find best binary threshold vs rest
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
        print(f"Class {cls}: Best Threshold={best_t:.2f}, F1={best_f1:.2f}")

    # Save artifacts
    print("Saving model and artifacts...")
    output_dir = Path("data")

    # We need to save the components to reconstruct the pipeline/predictor
    # The GenericClassifierService will need:
    # 1. Embedder (already has)
    # 2. TfidfVectorizer (fit)
    # 3. TruncatedSVD (fit) - for dimensionality reduction
    # 4. StandardScaler (fit) - for feature normalization
    # 5. CalibratedClassifierCV (fit) - calibrated classifier
    # 6. Thresholds

    joblib.dump(tfidf, output_dir / "tfidf_vectorizer.joblib")
    joblib.dump(svd, output_dir / "tfidf_svd.joblib")
    joblib.dump(scaler, output_dir / "feature_scaler.joblib")
    joblib.dump(clf, output_dir / "genre_classifier.joblib")

    with open(output_dir / "genre_thresholds.json", "w") as f:
        json.dump(thresholds, f, indent=2)

    print("Saved artifacts:")
    print(f"  - {output_dir / 'tfidf_vectorizer.joblib'}")
    print(f"  - {output_dir / 'tfidf_svd.joblib'}")
    print(f"  - {output_dir / 'feature_scaler.joblib'}")
    print(f"  - {output_dir / 'genre_classifier.joblib'}")
    print(f"  - {output_dir / 'genre_thresholds.json'}")
    print("Done!")

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
    args = parser.parse_args()
    asyncio.run(main(
        params_path=args.params,
        data_path=args.data,
        augmented_paths=args.augmented,
    ))
