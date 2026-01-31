#!/usr/bin/env python3
"""
Threshold Tuning Script for Genre Classification.

This script analyzes the golden dataset to find optimal thresholds
that balance precision and recall for each genre.

Usage:
    uv run python scripts/tune_thresholds.py --golden data/golden_classification.json
"""

import argparse
import json
import sys
from collections import defaultdict
from dataclasses import dataclass
from pathlib import Path

import numpy as np


@dataclass
class ThresholdResult:
    """Result of threshold analysis for a genre."""

    genre: str
    optimal_threshold: float
    precision_at_optimal: float
    recall_at_optimal: float
    f1_at_optimal: float
    current_threshold: float
    precision_at_current: float
    recall_at_current: float
    f1_at_current: float


def load_golden_data(golden_path: Path) -> list[dict]:
    """Load golden dataset."""
    with open(golden_path) as f:
        data = json.load(f)
    return data


def load_current_thresholds(thresholds_path: Path) -> dict[str, float]:
    """Load current thresholds."""
    if not thresholds_path.exists():
        return {}
    with open(thresholds_path) as f:
        return json.load(f)


def compute_metrics_at_threshold(
    scores: list[float],
    labels: list[bool],
    threshold: float,
) -> tuple[float, float, float]:
    """Compute precision, recall, F1 at a given threshold."""
    predictions = [s >= threshold for s in scores]

    tp = sum(1 for p, l in zip(predictions, labels) if p and l)
    fp = sum(1 for p, l in zip(predictions, labels) if p and not l)
    fn = sum(1 for p, l in zip(predictions, labels) if not p and l)

    precision = tp / (tp + fp) if (tp + fp) > 0 else 0.0
    recall = tp / (tp + fn) if (tp + fn) > 0 else 0.0
    f1 = 2 * precision * recall / (precision + recall) if (precision + recall) > 0 else 0.0

    return precision, recall, f1


def find_optimal_threshold(
    scores: list[float],
    labels: list[bool],
    metric: str = "f1",
    min_recall: float = 0.3,
) -> tuple[float, float, float, float]:
    """Find optimal threshold for a genre.

    Args:
        scores: List of prediction scores for this genre.
        labels: List of boolean labels (True if this is the correct genre).
        metric: Optimization target ("f1", "recall", "precision").
        min_recall: Minimum recall constraint.

    Returns:
        Tuple of (optimal_threshold, precision, recall, f1).
    """
    if not scores or not labels:
        return 0.5, 0.0, 0.0, 0.0

    # Try thresholds from 0.01 to 0.99
    thresholds = np.arange(0.01, 0.99, 0.01)
    best_threshold = 0.5
    best_metric_value = -1.0
    best_precision = 0.0
    best_recall = 0.0
    best_f1 = 0.0

    for t in thresholds:
        precision, recall, f1 = compute_metrics_at_threshold(scores, labels, t)

        # Skip if recall is below minimum (unless we're optimizing for precision)
        if metric != "precision" and recall < min_recall:
            continue

        if metric == "f1":
            metric_value = f1
        elif metric == "recall":
            metric_value = recall
        else:
            metric_value = precision

        if metric_value > best_metric_value:
            best_metric_value = metric_value
            best_threshold = t
            best_precision = precision
            best_recall = recall
            best_f1 = f1

    return best_threshold, best_precision, best_recall, best_f1


def analyze_thresholds(
    golden_data: list[dict],
    current_thresholds: dict[str, float],
    metric: str = "f1",
    min_recall: float = 0.3,
) -> list[ThresholdResult]:
    """Analyze and find optimal thresholds for all genres.

    Args:
        golden_data: List of golden dataset entries with 'genre' and 'scores'.
        current_thresholds: Current threshold values.
        metric: Optimization target.
        min_recall: Minimum recall constraint.

    Returns:
        List of ThresholdResult for each genre.
    """
    # Collect scores and labels per genre
    genre_data: dict[str, dict] = defaultdict(lambda: {"scores": [], "labels": []})

    for entry in golden_data:
        true_genre = entry.get("genre") or entry.get("expected_genre") or entry.get("label")
        scores = entry.get("scores", {})

        if not true_genre or not scores:
            continue

        for genre, score in scores.items():
            is_correct = genre == true_genre
            genre_data[genre]["scores"].append(score)
            genre_data[genre]["labels"].append(is_correct)

    results = []
    for genre, data in genre_data.items():
        scores = data["scores"]
        labels = data["labels"]

        # Find optimal threshold
        opt_threshold, opt_precision, opt_recall, opt_f1 = find_optimal_threshold(
            scores, labels, metric, min_recall
        )

        # Compute metrics at current threshold
        current_threshold = current_thresholds.get(genre, 0.5)
        cur_precision, cur_recall, cur_f1 = compute_metrics_at_threshold(
            scores, labels, current_threshold
        )

        results.append(
            ThresholdResult(
                genre=genre,
                optimal_threshold=opt_threshold,
                precision_at_optimal=opt_precision,
                recall_at_optimal=opt_recall,
                f1_at_optimal=opt_f1,
                current_threshold=current_threshold,
                precision_at_current=cur_precision,
                recall_at_current=cur_recall,
                f1_at_current=cur_f1,
            )
        )

    # Sort by improvement potential (F1 difference)
    results.sort(key=lambda r: r.f1_at_optimal - r.f1_at_current, reverse=True)
    return results


def print_analysis(results: list[ThresholdResult]) -> None:
    """Print analysis results in a formatted table."""
    print("\n" + "=" * 100)
    print("THRESHOLD ANALYSIS RESULTS")
    print("=" * 100)
    print(
        f"{'Genre':<25} {'Curr Th':>8} {'Opt Th':>8} "
        f"{'Curr P':>7} {'Opt P':>7} "
        f"{'Curr R':>7} {'Opt R':>7} "
        f"{'Curr F1':>8} {'Opt F1':>8} {'Δ F1':>7}"
    )
    print("-" * 100)

    for r in results:
        delta_f1 = r.f1_at_optimal - r.f1_at_current
        print(
            f"{r.genre:<25} {r.current_threshold:>8.3f} {r.optimal_threshold:>8.3f} "
            f"{r.precision_at_current:>7.3f} {r.precision_at_optimal:>7.3f} "
            f"{r.recall_at_current:>7.3f} {r.recall_at_optimal:>7.3f} "
            f"{r.f1_at_current:>8.3f} {r.f1_at_optimal:>8.3f} {delta_f1:>+7.3f}"
        )

    print("=" * 100)


def generate_new_thresholds(
    results: list[ThresholdResult],
    output_path: Path,
) -> dict[str, float]:
    """Generate and save new thresholds file."""
    new_thresholds = {r.genre: r.optimal_threshold for r in results}

    with open(output_path, "w") as f:
        json.dump(new_thresholds, f, indent=2, ensure_ascii=False)

    print(f"\nNew thresholds saved to: {output_path}")
    return new_thresholds


def main():
    parser = argparse.ArgumentParser(description="Tune genre classification thresholds")
    parser.add_argument(
        "--golden",
        type=Path,
        default=Path("data/golden_classification.json"),
        help="Path to golden dataset",
    )
    parser.add_argument(
        "--thresholds",
        type=Path,
        default=Path("data/genre_thresholds_ja.json"),
        help="Path to current thresholds",
    )
    parser.add_argument(
        "--output",
        type=Path,
        default=Path("data/genre_thresholds_ja_tuned.json"),
        help="Output path for tuned thresholds",
    )
    parser.add_argument(
        "--metric",
        choices=["f1", "recall", "precision"],
        default="f1",
        help="Optimization metric",
    )
    parser.add_argument(
        "--min-recall",
        type=float,
        default=0.4,
        help="Minimum recall constraint (default: 0.4 for more conservative thresholds)",
    )
    parser.add_argument(
        "--apply",
        action="store_true",
        help="Apply tuned thresholds (overwrite current)",
    )

    args = parser.parse_args()

    # Check golden dataset exists
    if not args.golden.exists():
        print(f"Error: Golden dataset not found at {args.golden}")
        print("Run evaluation first to generate predictions with scores.")
        sys.exit(1)

    # Load data
    print(f"Loading golden dataset from: {args.golden}")
    golden_data = load_golden_data(args.golden)
    print(f"Loaded {len(golden_data)} entries")

    print(f"Loading current thresholds from: {args.thresholds}")
    current_thresholds = load_current_thresholds(args.thresholds)
    print(f"Loaded thresholds for {len(current_thresholds)} genres")

    # Analyze
    print(f"\nOptimizing for: {args.metric} (min recall: {args.min_recall})")
    results = analyze_thresholds(golden_data, current_thresholds, args.metric, args.min_recall)

    # Print results
    print_analysis(results)

    # Generate new thresholds
    if args.apply:
        output_path = args.thresholds  # Overwrite current
    else:
        output_path = args.output

    generate_new_thresholds(results, output_path)

    # Summary statistics
    avg_current_f1 = sum(r.f1_at_current for r in results) / len(results) if results else 0
    avg_optimal_f1 = sum(r.f1_at_optimal for r in results) / len(results) if results else 0
    avg_current_recall = sum(r.recall_at_current for r in results) / len(results) if results else 0
    avg_optimal_recall = sum(r.recall_at_optimal for r in results) / len(results) if results else 0

    print("\nSUMMARY:")
    print(f"  Average Current F1:  {avg_current_f1:.3f}")
    print(f"  Average Optimal F1:  {avg_optimal_f1:.3f}")
    print(f"  Average Current Recall: {avg_current_recall:.3f}")
    print(f"  Average Optimal Recall: {avg_optimal_recall:.3f}")
    print(f"  Expected Improvement: {avg_optimal_f1 - avg_current_f1:+.3f} F1")


if __name__ == "__main__":
    main()
