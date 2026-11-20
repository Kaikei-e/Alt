#!/usr/bin/env python3
"""
Optuna Hyperparameter Optimization for Genre Classification.

This script uses Bayesian optimization with Optuna to find optimal
hyperparameters for the genre classification model, using cross-validation
to prevent overfitting.

Usage:
    uv run python scripts/optimize_hyperparams.py [--n-trials 50] [--output data/optimal_params.json]
"""

import argparse
import json
import sys
from collections.abc import Callable
from pathlib import Path
from typing import Any

import numpy as np
import optuna
import pandas as pd
from sklearn.decomposition import TruncatedSVD
from sklearn.feature_extraction.text import TfidfVectorizer
from sklearn.linear_model import LogisticRegression
from sklearn.metrics import f1_score
from sklearn.model_selection import StratifiedKFold, cross_val_score
from sklearn.pipeline import Pipeline
from sklearn.preprocessing import StandardScaler

# Add project root to path
project_root = Path(__file__).parent.parent
sys.path.insert(0, str(project_root))


def load_training_data(data_path: Path, min_samples: int = 20) -> tuple[pd.Series, pd.Series]:
    """Load and filter training data.

    Args:
        data_path: Path to training_data.csv
        min_samples: Minimum samples per genre

    Returns:
        Tuple of (X, y) as pandas Series
    """
    df = pd.read_csv(data_path)
    df = df.dropna(subset=["content", "genre"])

    # Filter out rare classes
    counts = df["genre"].value_counts()
    valid_genres = counts[counts >= min_samples].index
    sys.stdout.write(f"Filtering genres with >= {min_samples} samples. Kept: {len(valid_genres)}\n")
    df = df[df["genre"].isin(valid_genres)]

    return df["content"], df["genre"]


OVERFIT_GAP_WARNING_THRESHOLD = 0.10
TARGET_CV_F1_MIN = 0.70


def create_objective(
    texts: pd.Series,
    y: pd.Series,
    n_folds: int = 5,
) -> Callable[..., Any]:
    """Create Optuna objective function.

    Args:
        texts: Training content
        y: Training labels
        n_folds: Number of CV folds

    Returns:
        Objective function for Optuna
    """

    def objective(trial: optuna.Trial) -> float:
        # Hyperparameters to optimize
        reg_c = trial.suggest_float("C", 0.001, 10.0, log=True)
        max_features = trial.suggest_int("max_features", 500, 3000, step=500)
        svd_components = trial.suggest_int("svd_components", 50, 300, step=50)
        min_df = trial.suggest_int("min_df", 1, 5)
        max_df = trial.suggest_float("max_df", 0.8, 0.98)

        # Build pipeline (TF-IDF only for speed during optimization)
        # Full pipeline with embeddings is too slow for many trials
        tfidf = TfidfVectorizer(
            max_features=max_features,
            sublinear_tf=True,
            min_df=min_df,
            max_df=max_df,
            ngram_range=(1, 2),
        )

        # SVD for dimensionality reduction
        svd = TruncatedSVD(n_components=svd_components, random_state=42)

        # Logistic Regression with regularization
        clf = LogisticRegression(
            C=reg_c,
            max_iter=1000,
            solver="lbfgs",
            class_weight="balanced",
            n_jobs=-1,
        )

        # Create pipeline
        pipeline = Pipeline(
            [
                ("tfidf", tfidf),
                ("svd", svd),
                ("scaler", StandardScaler()),
                ("clf", clf),
            ]
        )

        # Cross-validation
        cv = StratifiedKFold(n_splits=n_folds, shuffle=True, random_state=42)

        try:
            scores = cross_val_score(
                pipeline,
                texts,
                y,
                cv=cv,
                scoring="f1_macro",
                n_jobs=-1,
            )
        except Exception as e:  # noqa: BLE001 - a failed trial scores 0.0 so the study continues
            sys.stdout.write(f"Trial failed: {e}\n")
            return 0.0
        else:
            mean_score = scores.mean()
            std_score = scores.std()

            # Report intermediate values for pruning
            trial.set_user_attr("cv_std", std_score)
            trial.set_user_attr("cv_scores", scores.tolist())

            return mean_score

    return objective


def optimize_hyperparams(
    data_path: Path,
    n_trials: int = 50,
    n_folds: int = 5,
    timeout: int | None = None,
    min_samples: int = 20,
) -> dict:
    """Run hyperparameter optimization.

    Args:
        data_path: Path to training data
        n_trials: Number of Optuna trials
        n_folds: Number of CV folds
        timeout: Optional timeout in seconds
        min_samples: Minimum samples per genre

    Returns:
        Dictionary with best parameters and results
    """
    sys.stdout.write(f"Loading data from {data_path}...\n")
    texts, y = load_training_data(data_path, min_samples)
    sys.stdout.write(f"Loaded {len(texts)} samples with {y.nunique()} genres\n")

    # Create study with TPE sampler (default Bayesian optimization)
    sampler = optuna.samplers.TPESampler(seed=42)
    study = optuna.create_study(
        direction="maximize",
        sampler=sampler,
        study_name="genre_classification_optimization",
    )

    # Create objective
    objective = create_objective(texts, y, n_folds)

    # Optimize
    sys.stdout.write(f"\nStarting optimization with {n_trials} trials...\n")
    study.optimize(
        objective,
        n_trials=n_trials,
        timeout=timeout,
        show_progress_bar=True,
        gc_after_trial=True,
    )

    # Get best results
    best_trial = study.best_trial
    best_params = best_trial.params

    # Calculate train/test gap with best params
    sys.stdout.write("\nValidating best parameters...\n")
    train_score, test_score, gap = validate_train_test_gap(texts, y, best_params, n_folds)

    return {
        "best_params": best_params,
        "best_cv_f1": best_trial.value,
        "cv_std": best_trial.user_attrs.get("cv_std", 0),
        "cv_scores": best_trial.user_attrs.get("cv_scores", []),
        "train_f1": train_score,
        "test_f1": test_score,
        "train_test_gap": gap,
        "n_trials": n_trials,
        "n_folds": n_folds,
        "n_samples": len(texts),
        "n_genres": y.nunique(),
    }


def validate_train_test_gap(
    texts: pd.Series,
    y: pd.Series,
    params: dict,
    n_folds: int = 5,
) -> tuple[float, float, float]:
    """Validate train/test gap to check for overfitting.

    Args:
        texts: Training content
        y: Training labels
        params: Best hyperparameters
        n_folds: Number of CV folds

    Returns:
        Tuple of (train_score, test_score, gap)
    """
    tfidf = TfidfVectorizer(
        max_features=params["max_features"],
        sublinear_tf=True,
        min_df=params["min_df"],
        max_df=params["max_df"],
        ngram_range=(1, 2),
    )

    svd = TruncatedSVD(n_components=params["svd_components"], random_state=42)

    clf = LogisticRegression(
        C=params["C"],
        max_iter=1000,
        solver="lbfgs",
        class_weight="balanced",
        n_jobs=-1,
    )

    pipeline = Pipeline(
        [
            ("tfidf", tfidf),
            ("svd", svd),
            ("scaler", StandardScaler()),
            ("clf", clf),
        ]
    )

    cv = StratifiedKFold(n_splits=n_folds, shuffle=True, random_state=42)

    train_scores = []
    test_scores = []

    for train_idx, test_idx in cv.split(texts, y):
        x_train, x_test = texts.iloc[train_idx], texts.iloc[test_idx]
        y_train, y_test = y.iloc[train_idx], y.iloc[test_idx]

        pipeline.fit(x_train, y_train)

        # Train score
        y_train_pred = pipeline.predict(x_train)
        train_scores.append(f1_score(y_train, y_train_pred, average="macro"))

        # Test score
        y_test_pred = pipeline.predict(x_test)
        test_scores.append(f1_score(y_test, y_test_pred, average="macro"))

    train_mean = float(np.mean(train_scores))
    test_mean = float(np.mean(test_scores))
    gap = train_mean - test_mean

    return train_mean, test_mean, gap


def print_results(results: dict) -> None:
    """Print optimization results."""
    sys.stdout.write("\n" + "=" * 60 + "\n")
    sys.stdout.write("OPTIMIZATION RESULTS\n")
    sys.stdout.write("=" * 60 + "\n")

    sys.stdout.write("\nBest Hyperparameters:\n")
    for param, value in results["best_params"].items():
        if isinstance(value, float):
            sys.stdout.write(f"  {param}: {value:.6f}\n")
        else:
            sys.stdout.write(f"  {param}: {value}\n")

    sys.stdout.write("\nCross-Validation Results:\n")
    sys.stdout.write(f"  Mean Macro F1: {results['best_cv_f1']:.4f}\n")
    sys.stdout.write(f"  Std: {results['cv_std']:.4f}\n")
    sys.stdout.write(f"  Fold Scores: {[f'{s:.4f}' for s in results['cv_scores']]}\n")

    sys.stdout.write("\nOverfitting Analysis:\n")
    sys.stdout.write(f"  Train F1: {results['train_f1']:.4f}\n")
    sys.stdout.write(f"  Test F1: {results['test_f1']:.4f}\n")
    sys.stdout.write(f"  Train/Test Gap: {results['train_test_gap']:.4f}\n")

    if results["train_test_gap"] < OVERFIT_GAP_WARNING_THRESHOLD:
        sys.stdout.write("  Status: OK (gap < 0.10)\n")
    else:
        sys.stdout.write("  Status: WARNING - potential overfitting (gap >= 0.10)\n")

    sys.stdout.write("\nDataset Info:\n")
    sys.stdout.write(f"  Samples: {results['n_samples']}\n")
    sys.stdout.write(f"  Genres: {results['n_genres']}\n")
    sys.stdout.write("=" * 60 + "\n")


def main() -> None:
    parser = argparse.ArgumentParser(
        description="Optimize genre classification hyperparameters with Optuna"
    )
    parser.add_argument(
        "--data",
        type=Path,
        default=Path("data/training_data.csv"),
        help="Path to training data CSV",
    )
    parser.add_argument(
        "--output",
        type=Path,
        default=Path("data/optimal_params.json"),
        help="Output path for optimal parameters",
    )
    parser.add_argument(
        "--n-trials",
        type=int,
        default=50,
        help="Number of Optuna trials (default: 50)",
    )
    parser.add_argument(
        "--n-folds",
        type=int,
        default=5,
        help="Number of CV folds (default: 5)",
    )
    parser.add_argument(
        "--timeout",
        type=int,
        default=None,
        help="Optimization timeout in seconds",
    )
    parser.add_argument(
        "--min-samples",
        type=int,
        default=20,
        help="Minimum samples per genre (default: 20)",
    )

    args = parser.parse_args()

    # Check data file exists
    if not args.data.exists():
        sys.stdout.write(f"Error: Training data not found at {args.data}\n")
        sys.exit(1)

    # Run optimization
    results = optimize_hyperparams(
        data_path=args.data,
        n_trials=args.n_trials,
        n_folds=args.n_folds,
        timeout=args.timeout,
        min_samples=args.min_samples,
    )

    # Print results
    print_results(results)

    # Save results
    with args.output.open("w") as f:
        json.dump(results, f, indent=2)
    sys.stdout.write(f"\nResults saved to: {args.output}\n")

    # Return success/failure based on targets
    if (
        results["best_cv_f1"] >= TARGET_CV_F1_MIN
        and results["train_test_gap"] < OVERFIT_GAP_WARNING_THRESHOLD
    ):
        sys.stdout.write("\nSUCCESS: Targets met (CV F1 >= 0.70, gap < 0.10)\n")
        sys.exit(0)
    else:
        sys.stdout.write(
            "\nNOTE: Targets not yet met. Consider more trials or data augmentation.\n"
        )
        sys.exit(0)  # Still exit 0 since optimization completed


if __name__ == "__main__":
    main()
