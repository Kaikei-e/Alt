#!/usr/bin/env python3
"""
Tag Quality Evaluation Framework

Evaluates tag extraction quality using gold standard datasets.
Supports multiple extractors and provides comprehensive metrics.

Usage:
    uv run python scripts/evaluate_tag_quality.py --help
    uv run python scripts/evaluate_tag_quality.py --dataset evaluation/golden_dataset_ja.json --extractor hybrid
    uv run python scripts/evaluate_tag_quality.py --create-dataset --output evaluation/golden_dataset_ja.json --limit 200
"""

import argparse
import json
import os
import sys
from collections.abc import Callable
from dataclasses import dataclass, field
from pathlib import Path
from typing import Any

# Add tag-generator/app to path for imports
sys.path.insert(0, str(Path(__file__).parent.parent / "tag-generator" / "app"))


@dataclass
class EvaluationMetrics:
    """Container for evaluation metrics."""

    precision_at_5: float = 0.0
    precision_at_10: float = 0.0
    recall_at_5: float = 0.0
    recall_at_10: float = 0.0
    f1_at_5: float = 0.0
    f1_at_10: float = 0.0
    diversity_score: float = 0.0
    avg_tag_count: float = 0.0
    empty_tag_rate: float = 0.0
    avg_inference_ms: float = 0.0
    total_samples: int = 0
    per_sample_results: list[dict[str, Any]] = field(default_factory=list)


@dataclass
class GoldenSample:
    """A single sample from the golden dataset."""

    id: str
    title: str
    content: str
    expected_tags: list[str]
    language: str = "ja"
    source: str = "manual"


def load_golden_dataset(path: str) -> list[GoldenSample]:
    """Load golden dataset from JSON file."""
    with open(path, encoding="utf-8") as f:
        data = json.load(f)

    samples = []
    for item in data.get("samples", []):
        samples.append(
            GoldenSample(
                id=item["id"],
                title=item["title"],
                content=item["content"],
                expected_tags=item["expected_tags"],
                language=item.get("language", "ja"),
                source=item.get("source", "manual"),
            )
        )
    return samples


def save_golden_dataset(samples: list[GoldenSample], path: str) -> None:
    """Save golden dataset to JSON file."""
    data = {
        "version": "1.0",
        "description": "Golden dataset for tag extraction quality evaluation",
        "samples": [
            {
                "id": s.id,
                "title": s.title,
                "content": s.content,
                "expected_tags": s.expected_tags,
                "language": s.language,
                "source": s.source,
            }
            for s in samples
        ],
    }
    os.makedirs(os.path.dirname(path), exist_ok=True)
    with open(path, "w", encoding="utf-8") as f:
        json.dump(data, f, ensure_ascii=False, indent=2)


def compute_precision_at_k(predicted: list[str], expected: set[str], k: int) -> float:
    """Compute precision@k metric."""
    if not predicted or k == 0:
        return 0.0

    top_k = predicted[:k]
    # Normalize for case-insensitive comparison
    top_k_normalized = {t.lower() for t in top_k}
    expected_normalized = {t.lower() for t in expected}

    matches = len(top_k_normalized & expected_normalized)
    return matches / min(len(top_k), k)


def compute_recall_at_k(predicted: list[str], expected: set[str], k: int) -> float:
    """Compute recall@k metric."""
    if not expected or k == 0:
        return 0.0

    top_k = predicted[:k]
    top_k_normalized = {t.lower() for t in top_k}
    expected_normalized = {t.lower() for t in expected}

    matches = len(top_k_normalized & expected_normalized)
    return matches / len(expected_normalized)


def compute_f1(precision: float, recall: float) -> float:
    """Compute F1 score from precision and recall."""
    if precision + recall == 0:
        return 0.0
    return 2 * (precision * recall) / (precision + recall)


def compute_diversity_score(tags: list[str]) -> float:
    """
    Compute diversity score based on tag uniqueness and variety.

    A score of 1.0 means all tags are unique and diverse.
    Lower scores indicate redundant or very similar tags.
    """
    if not tags:
        return 0.0

    # Check for unique tags
    normalized = [t.lower() for t in tags]
    unique_count = len(set(normalized))
    uniqueness = unique_count / len(normalized)

    # Check for substring overlap (penalize if one tag is substring of another)
    overlap_penalty = 0.0
    for i, tag1 in enumerate(normalized):
        for tag2 in normalized[i + 1 :]:
            if tag1 in tag2 or tag2 in tag1:
                overlap_penalty += 0.1

    overlap_penalty = min(overlap_penalty, 0.5)  # Cap at 50% penalty

    return max(0.0, uniqueness - overlap_penalty)


def evaluate_extractor(
    samples: list[GoldenSample],
    extract_fn: Callable[[str, str], tuple[list[str], float]],
) -> EvaluationMetrics:
    """
    Evaluate an extractor against the golden dataset.

    Args:
        samples: List of golden samples
        extract_fn: Function that takes (title, content) and returns (tags, inference_ms)

    Returns:
        EvaluationMetrics with aggregated results
    """
    metrics = EvaluationMetrics(total_samples=len(samples))

    total_precision_5 = 0.0
    total_precision_10 = 0.0
    total_recall_5 = 0.0
    total_recall_10 = 0.0
    total_diversity = 0.0
    total_inference_ms = 0.0
    total_tag_count = 0
    empty_count = 0

    for sample in samples:
        tags, inference_ms = extract_fn(sample.title, sample.content)
        expected_set = set(sample.expected_tags)

        p5 = compute_precision_at_k(tags, expected_set, 5)
        p10 = compute_precision_at_k(tags, expected_set, 10)
        r5 = compute_recall_at_k(tags, expected_set, 5)
        r10 = compute_recall_at_k(tags, expected_set, 10)
        diversity = compute_diversity_score(tags)

        total_precision_5 += p5
        total_precision_10 += p10
        total_recall_5 += r5
        total_recall_10 += r10
        total_diversity += diversity
        total_inference_ms += inference_ms
        total_tag_count += len(tags)

        if not tags:
            empty_count += 1

        metrics.per_sample_results.append(
            {
                "id": sample.id,
                "predicted_tags": tags,
                "expected_tags": sample.expected_tags,
                "precision_at_5": p5,
                "precision_at_10": p10,
                "recall_at_5": r5,
                "recall_at_10": r10,
                "diversity": diversity,
                "inference_ms": inference_ms,
            }
        )

    n = len(samples) if samples else 1

    metrics.precision_at_5 = total_precision_5 / n
    metrics.precision_at_10 = total_precision_10 / n
    metrics.recall_at_5 = total_recall_5 / n
    metrics.recall_at_10 = total_recall_10 / n
    metrics.f1_at_5 = compute_f1(metrics.precision_at_5, metrics.recall_at_5)
    metrics.f1_at_10 = compute_f1(metrics.precision_at_10, metrics.recall_at_10)
    metrics.diversity_score = total_diversity / n
    metrics.avg_tag_count = total_tag_count / n
    metrics.empty_tag_rate = empty_count / n
    metrics.avg_inference_ms = total_inference_ms / n

    return metrics


def print_metrics(metrics: EvaluationMetrics, extractor_name: str) -> None:
    """Print evaluation metrics in a formatted table."""
    print(f"\n{'=' * 60}")
    print(f"Evaluation Results: {extractor_name}")
    print(f"{'=' * 60}")
    print(f"Total Samples: {metrics.total_samples}")
    print(f"\n{'Metric':<25} {'Value':<15}")
    print("-" * 40)
    print(f"{'Precision@5':<25} {metrics.precision_at_5:.3f}")
    print(f"{'Precision@10':<25} {metrics.precision_at_10:.3f}")
    print(f"{'Recall@5':<25} {metrics.recall_at_5:.3f}")
    print(f"{'Recall@10':<25} {metrics.recall_at_10:.3f}")
    print(f"{'F1@5':<25} {metrics.f1_at_5:.3f}")
    print(f"{'F1@10':<25} {metrics.f1_at_10:.3f}")
    print(f"{'Diversity Score':<25} {metrics.diversity_score:.3f}")
    print(f"{'Avg Tag Count':<25} {metrics.avg_tag_count:.1f}")
    print(f"{'Empty Tag Rate':<25} {metrics.empty_tag_rate:.1%}")
    print(f"{'Avg Inference (ms)':<25} {metrics.avg_inference_ms:.1f}")
    print("=" * 60)


def create_extractor_fn(
    extractor_type: str,
) -> Callable[[str, str], tuple[list[str], float]]:
    """
    Create an extractor function for the specified type.

    Args:
        extractor_type: One of 'current', 'hybrid', 'ginza'

    Returns:
        Function that takes (title, content) and returns (tags, inference_ms)
    """
    import time

    if extractor_type == "current":
        from tag_extractor.extract import TagExtractor

        extractor = TagExtractor()

        def extract_current(title: str, content: str) -> tuple[list[str], float]:
            result = extractor.extract_tags_with_metrics(title, content)
            return result.tags, result.inference_ms

        return extract_current

    elif extractor_type == "hybrid":
        from tag_extractor.hybrid_extractor import HybridExtractor

        extractor = HybridExtractor()

        def extract_hybrid(title: str, content: str) -> tuple[list[str], float]:
            start = time.perf_counter()
            tags = extractor.extract_tags(title, content)
            inference_ms = (time.perf_counter() - start) * 1000
            return tags, inference_ms

        return extract_hybrid

    elif extractor_type == "ginza":
        from tag_extractor.ginza_extractor import GinzaExtractor

        extractor = GinzaExtractor()

        def extract_ginza(title: str, content: str) -> tuple[list[str], float]:
            start = time.perf_counter()
            text = f"{title}\n{content}"
            noun_phrases = extractor.extract_noun_phrases(text)
            entities = extractor.extract_named_entities(text)
            # Combine and deduplicate
            tags = list(dict.fromkeys(entities + noun_phrases))[:10]
            inference_ms = (time.perf_counter() - start) * 1000
            return tags, inference_ms

        return extract_ginza

    else:
        raise ValueError(f"Unknown extractor type: {extractor_type}")


def create_dataset_from_db(
    db_url: str,
    output_path: str,
    limit: int = 200,
    min_confidence: float = 0.7,
    min_tags: int = 5,
    language: str = "ja",
) -> None:
    """
    Create golden dataset from database by extracting high-quality tagged articles.

    Args:
        db_url: PostgreSQL connection URL
        output_path: Path to save the golden dataset
        limit: Maximum number of samples to extract
        min_confidence: Minimum tag confidence threshold
        min_tags: Minimum number of tags per article
        language: Target language ('ja' or 'en')
    """
    import psycopg2

    print(f"Connecting to database...")

    conn = psycopg2.connect(db_url)
    cur = conn.cursor()

    # Query to get high-quality tagged articles
    query = """
        SELECT
            a.id,
            a.title,
            COALESCE(a.text_content, a.description, '') as content,
            array_agg(t.name ORDER BY at.confidence DESC) as tags
        FROM articles a
        JOIN article_tags at ON a.id = at.article_id
        JOIN tags t ON at.tag_id = t.id
        WHERE at.confidence >= %s
          AND a.language = %s
        GROUP BY a.id, a.title, a.text_content, a.description
        HAVING count(t.id) >= %s
        ORDER BY a.created_at DESC
        LIMIT %s;
    """

    cur.execute(query, (min_confidence, language, min_tags, limit))
    rows = cur.fetchall()

    samples = []
    for row in rows:
        article_id, title, content, tags = row
        if title and content and tags:
            samples.append(
                GoldenSample(
                    id=str(article_id),
                    title=title,
                    content=content[:10000],  # Limit content length
                    expected_tags=tags[:15],  # Limit tags
                    language=language,
                    source="database",
                )
            )

    cur.close()
    conn.close()

    print(f"Extracted {len(samples)} samples from database")
    save_golden_dataset(samples, output_path)
    print(f"Saved golden dataset to {output_path}")


def main() -> None:
    parser = argparse.ArgumentParser(
        description="Evaluate tag extraction quality",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
Examples:
  # Evaluate current extractor
  uv run python scripts/evaluate_tag_quality.py --dataset evaluation/golden_dataset_ja.json --extractor current

  # Evaluate hybrid extractor
  uv run python scripts/evaluate_tag_quality.py --dataset evaluation/golden_dataset_ja.json --extractor hybrid

  # Create dataset from database
  uv run python scripts/evaluate_tag_quality.py --create-dataset --output evaluation/golden_dataset_ja.json --db-url "postgresql://..." --limit 200

  # Save evaluation results to JSON
  uv run python scripts/evaluate_tag_quality.py --dataset evaluation/golden_dataset_ja.json --extractor current --output-json results.json
        """,
    )

    parser.add_argument(
        "--dataset",
        type=str,
        help="Path to golden dataset JSON file",
    )
    parser.add_argument(
        "--extractor",
        type=str,
        choices=["current", "hybrid", "ginza"],
        default="current",
        help="Extractor type to evaluate",
    )
    parser.add_argument(
        "--output-json",
        type=str,
        help="Path to save detailed evaluation results as JSON",
    )
    parser.add_argument(
        "--create-dataset",
        action="store_true",
        help="Create golden dataset from database",
    )
    parser.add_argument(
        "--output",
        type=str,
        default="evaluation/golden_dataset_ja.json",
        help="Output path for created dataset",
    )
    parser.add_argument(
        "--db-url",
        type=str,
        default=os.getenv("DATABASE_URL"),
        help="Database URL for creating dataset",
    )
    parser.add_argument(
        "--limit",
        type=int,
        default=200,
        help="Maximum samples to extract from database",
    )
    parser.add_argument(
        "--language",
        type=str,
        default="ja",
        choices=["ja", "en"],
        help="Language for dataset creation",
    )

    args = parser.parse_args()

    if args.create_dataset:
        if not args.db_url:
            print("Error: --db-url is required for dataset creation")
            sys.exit(1)
        create_dataset_from_db(
            db_url=args.db_url,
            output_path=args.output,
            limit=args.limit,
            language=args.language,
        )
        return

    if not args.dataset:
        print("Error: --dataset is required for evaluation")
        parser.print_help()
        sys.exit(1)

    if not os.path.exists(args.dataset):
        print(f"Error: Dataset file not found: {args.dataset}")
        sys.exit(1)

    print(f"Loading golden dataset from {args.dataset}...")
    samples = load_golden_dataset(args.dataset)
    print(f"Loaded {len(samples)} samples")

    print(f"Creating {args.extractor} extractor...")
    extract_fn = create_extractor_fn(args.extractor)

    print(f"Evaluating {args.extractor} extractor...")
    metrics = evaluate_extractor(samples, extract_fn)

    print_metrics(metrics, args.extractor)

    if args.output_json:
        result = {
            "extractor": args.extractor,
            "dataset": args.dataset,
            "metrics": {
                "precision_at_5": metrics.precision_at_5,
                "precision_at_10": metrics.precision_at_10,
                "recall_at_5": metrics.recall_at_5,
                "recall_at_10": metrics.recall_at_10,
                "f1_at_5": metrics.f1_at_5,
                "f1_at_10": metrics.f1_at_10,
                "diversity_score": metrics.diversity_score,
                "avg_tag_count": metrics.avg_tag_count,
                "empty_tag_rate": metrics.empty_tag_rate,
                "avg_inference_ms": metrics.avg_inference_ms,
                "total_samples": metrics.total_samples,
            },
            "per_sample_results": metrics.per_sample_results,
        }
        with open(args.output_json, "w", encoding="utf-8") as f:
            json.dump(result, f, ensure_ascii=False, indent=2)
        print(f"\nDetailed results saved to {args.output_json}")


if __name__ == "__main__":
    main()
