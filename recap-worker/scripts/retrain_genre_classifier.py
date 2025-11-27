#!/usr/bin/env python3
"""
Utility script to regenerate genre classifier weights from a labelled dataset.

The script can either:
1. Consume a JSON file with structure identical to `tests/data/golden_classification.json`
2. Fetch articles from the database (past 14 days) and use genre learning results

Emits a weights JSON compatible with `recap_worker::classification::model::HybridModel`.
"""

from __future__ import annotations

import argparse
import json
import math
import os
import re
from collections import Counter, defaultdict
from datetime import datetime, timedelta, timezone
from pathlib import Path
from typing import Optional

try:
    import psycopg2
    from psycopg2.extras import RealDictCursor
    PSYCOPG2_AVAILABLE = True
except ImportError:
    PSYCOPG2_AVAILABLE = False

FEATURE_VOCAB = [
    "人工知能",
    "自動運転",
    "資金調達",
    "投資",
    "決算",
    "政策",
    "政府",
    "遺伝子",
    "医療",
    "量子",
    "サッカー",
    "音楽",
    "confidential computing",
    "cybersecurity",
    "transformer",
    "diplomacy",
    "treaty",
    "economy",
    "business",
]

EMBEDDING_DIM = 6

EMBED_LOOKUP = {
    "人工知能": [1.0, 0.0, 0.0, 0.0, 0.0, 0.0],
    "自動運転": [1.0, 0.0, 0.0, 0.0, 0.0, 0.0],
    "transformer": [1.0, 0.0, 0.0, 0.0, 0.0, 0.0],
    "資金調達": [0.0, 1.0, 0.0, 0.0, 0.0, 0.0],
    "投資": [0.0, 1.0, 0.0, 0.0, 0.0, 0.0],
    "決算": [0.0, 1.0, 0.0, 0.0, 0.0, 0.0],
    "economy": [0.0, 1.0, 0.0, 0.0, 0.0, 0.0],
    "business": [0.0, 1.0, 0.0, 0.0, 0.0, 0.0],
    "政策": [0.0, 0.0, 1.0, 0.0, 0.0, 0.0],
    "政府": [0.0, 0.0, 1.0, 0.0, 0.0, 0.0],
    "diplomacy": [0.0, 0.3, 0.8, 0.0, 0.0, 0.0],
    "treaty": [0.0, 0.3, 0.8, 0.0, 0.0, 0.0],
    "遺伝子": [0.0, 0.0, 0.0, 1.0, 0.0, 0.0],
    "医療": [0.0, 0.0, 0.0, 1.0, 0.0, 0.0],
    "量子": [0.4, 0.1, 0.0, 0.9, 0.0, 0.0],
    "サッカー": [0.0, 0.0, 0.0, 0.0, 1.0, 0.0],
    "音楽": [0.0, 0.0, 0.0, 0.0, 0.0, 1.0],
    "confidential computing": [0.8, 0.3, 0.0, 0.0, 0.0, 0.0],
    "cybersecurity": [0.8, 0.2, 0.0, 0.0, 0.0, 0.0],
}

# 現在のジャンルリスト（genre_classifier_weights.jsonから取得）
GENRES = [
    "ai",
    "tech",
    "business",
    "politics",
    "health",
    "sports",
    "science",
    "entertainment",
    "world",
    "security",
    "product",
    "design",
    "culture",
    "environment",
    "lifestyle",
    "art_culture",
    "developer_insights",
    "pro_it_media",
    "consumer_tech",
    "global_politics",
    "environment_policy",
    "society_justice",
    "travel_lifestyle",
    "security_policy",
    "business_finance",
    "ai_research",
    "ai_policy",
    "games_puzzles",
    "other",
]


def load_samples(path: Path) -> list[dict]:
    """Load samples from a JSON file."""
    data = json.loads(path.read_text(encoding="utf-8"))
    if not isinstance(data, list):
        raise ValueError("dataset must be a JSON array")
    return data


def get_db_connection(dsn: str):
    """Get a PostgreSQL database connection."""
    if not PSYCOPG2_AVAILABLE:
        raise RuntimeError("psycopg2 is required for database operations. Install it with: pip install psycopg2-binary")
    return psycopg2.connect(dsn)


def fetch_distinct_genres_from_db(
    recap_db_dsn: str,
    days: int = 30,
    min_samples: int = 10,
) -> list[str]:
    """
    Fetch distinct genres from recap-db that have sufficient samples.

    Args:
        recap_db_dsn: Database connection string for recap-db
        days: Number of days to look back (default: 30)
        min_samples: Minimum number of samples required for a genre to be included (default: 10)

    Returns:
        List of genre names sorted alphabetically
    """
    if not PSYCOPG2_AVAILABLE:
        raise RuntimeError("psycopg2 is required for database operations. Install it with: pip install psycopg2-binary")

    # Calculate date range
    end_date = datetime.now(timezone.utc)
    start_date = end_date - timedelta(days=days)

    genres = []

    try:
        recap_conn = get_db_connection(recap_db_dsn)
    except Exception as e:
        raise RuntimeError(f"Failed to connect to recap-db: {e}") from e

    try:
        with recap_conn.cursor(cursor_factory=RealDictCursor) as cursor:
            # Fetch distinct genres with sufficient samples
            cursor.execute("""
                SELECT
                    refine_decision->>'final_genre' as genre,
                    COUNT(*) as count
                FROM recap_genre_learning_results
                WHERE created_at >= %s
                  AND refine_decision->>'final_genre' IS NOT NULL
                  AND refine_decision->>'final_genre' != ''
                GROUP BY refine_decision->>'final_genre'
                HAVING COUNT(*) >= %s
                ORDER BY genre
            """, (start_date, min_samples))

            for row in cursor.fetchall():
                genre = row["genre"]
                if genre:
                    genres.append(genre.lower())

    finally:
        recap_conn.close()

    if not genres:
        raise RuntimeError(
            f"No genres found in recap-db (past {days} days, min_samples={min_samples}). "
            "Please check the database connection and data availability."
        )

    print(f"Found {len(genres)} distinct genres from database: {', '.join(genres)}")
    return genres


def fetch_articles_from_db(
    alt_backend_dsn: Optional[str],
    recap_db_dsn: str,
    days: int = 14,
    genres: Optional[list[str]] = None,
) -> list[dict]:
    """
    Fetch articles from the database for the past N days and combine with genre learning results.

    Uses recap-db's recap_job_articles table which contains backed up articles from alt-backend.

    Args:
        alt_backend_dsn: Not used (kept for compatibility)
        recap_db_dsn: Database connection string for recap-db
        days: Number of days to look back (default: 14)
        genres: Optional list of valid genres to filter by. If None, uses GENRES constant.

    Returns:
        List of sample dictionaries with title, body, and expected_genres
    """
    if not PSYCOPG2_AVAILABLE:
        raise RuntimeError("psycopg2 is required for database operations. Install it with: pip install psycopg2-binary")

    # Calculate date range
    end_date = datetime.now(timezone.utc)
    start_date = end_date - timedelta(days=days)

    samples = []

    # Connect to recap-db to get genre learning results and article content
    try:
        recap_conn = get_db_connection(recap_db_dsn)
    except Exception as e:
        raise RuntimeError(f"Failed to connect to recap-db: {e}") from e

    try:
        with recap_conn.cursor(cursor_factory=RealDictCursor) as cursor:
            # Fetch genre learning results from the past N days
            cursor.execute("""
                SELECT
                    article_id,
                    refine_decision->>'final_genre' as final_genre,
                    coarse_candidates,
                    tag_profile->'top_tags' as top_tags,
                    created_at
                FROM recap_genre_learning_results
                WHERE created_at >= %s
                ORDER BY created_at DESC
            """, (start_date,))

            genre_results = {}
            for row in cursor.fetchall():
                article_id = row["article_id"]
                final_genre = row["final_genre"]
                coarse_candidates = row["coarse_candidates"]

                # Extract genres from refine_decision or coarse_candidates
                genres = []
                if final_genre:
                    genres.append(final_genre.lower())
                elif coarse_candidates:
                    # Get top genres from coarse_candidates (top 3 by score)
                    try:
                        candidates = json.loads(coarse_candidates) if isinstance(coarse_candidates, str) else coarse_candidates
                        if isinstance(candidates, list):
                            valid_genres = genres if genres is not None else GENRES
                            for candidate in sorted(candidates, key=lambda x: x.get("score", 0), reverse=True)[:3]:
                                genre = candidate.get("genre", "").lower()
                                if genre and genre in valid_genres:
                                    genres.append(genre)
                    except (json.JSONDecodeError, TypeError):
                        # Skip invalid JSON
                        pass

                if genres:
                    genre_results[article_id] = genres

            print(f"Found {len(genre_results)} articles with genre classifications")

            # Fetch articles from recap_job_articles table (same connection)
            # (This table contains backed up articles from alt-backend)
            if not genre_results:
                print("Warning: No genre classifications found. Cannot fetch articles.")
                return []

            article_ids = list(genre_results.keys())
            # Fetch articles from recap_job_articles table
            # Use ANY operator with array for safe parameterized query
            cursor.execute("""
                SELECT DISTINCT ON (article_id)
                    article_id,
                    title,
                    fulltext_html as content,
                    published_at
                FROM recap_job_articles
                WHERE article_id = ANY(%s)
                AND (published_at >= %s OR published_at IS NULL)
                ORDER BY article_id, published_at DESC NULLS LAST
            """, (article_ids, start_date))

            for row in cursor.fetchall():
                article_id = row["article_id"]
                title = row["title"] or ""
                # Extract text from HTML content (simple approach - remove HTML tags)
                html_content = row["content"] or ""
                # Simple HTML tag removal
                content = re.sub(r'<[^>]+>', ' ', html_content)
                content = re.sub(r'\s+', ' ', content).strip()

                # Only include articles that have genre classifications
                if article_id in genre_results:
                    samples.append({
                        "title": title,
                        "body": content,
                        "expected_genres": genre_results[article_id],
                    })

    finally:
        recap_conn.close()

    print(f"Loaded {len(samples)} samples from database (past {days} days)")
    return samples


def expand_tokens(tokens: list[str]) -> list[str]:
    expanded = []
    for token in tokens:
        lower = token.lower()
        expanded.append(lower)
        # simple stemming for english plural forms
        if lower.endswith("s") and len(lower) > 3:
            expanded.append(lower[:-1])
    return expanded


def build_feature_counts(samples: list[dict], genres: list[str]) -> tuple[dict[str, Counter], Counter, dict[str, int]]:
    """
    Build feature counts per genre and document frequency across entire corpus.

    Args:
        samples: List of sample dictionaries
        genres: List of genre names to consider

    Returns:
        - feature_counts: dict mapping genre to Counter of term frequencies
        - genre_totals: Counter of total documents per genre
        - doc_frequency: dict mapping term to number of documents containing it
    """
    feature_counts: dict[str, Counter] = {genre: Counter() for genre in genres}
    genre_totals: Counter = Counter()
    # Track which documents contain each term (for IDF calculation)
    doc_frequency: dict[str, int] = {term: 0 for term in FEATURE_VOCAB}
    documents_with_term: dict[str, set] = {term: set() for term in FEATURE_VOCAB}

    for sample_idx, sample in enumerate(samples):
        expected = [g.lower() for g in sample.get("expected_genres", [])]
        tokens = expand_tokens(sample.get("tokens", []))
        # Track unique terms in this document
        doc_terms = set()
        for token in tokens:
            if token in FEATURE_VOCAB:
                doc_terms.add(token)

        # Count document frequency (each document counts once per term)
        for term in doc_terms:
            documents_with_term[term].add(sample_idx)

        # Count term frequency per genre
        for genre in expected:
            if genre not in feature_counts:
                continue
            genre_totals[genre] += 1
            for token in tokens:
                if token in FEATURE_VOCAB:
                    feature_counts[genre][token] += 1

    # Convert sets to counts
    total_docs = len(samples)
    for term in FEATURE_VOCAB:
        doc_frequency[term] = len(documents_with_term[term])

    return feature_counts, genre_totals, doc_frequency


def infer_tokens(title: str, body: str) -> list[str]:
    text = f"{title} {body}".lower().replace("　", " ")
    return [tok for tok in text.split() if tok]


def enrich_samples(samples: list[dict]) -> None:
    for sample in samples:
        tokens = sample.get("tokens")
        if not tokens:
            tokens = infer_tokens(sample.get("title", ""), sample.get("body", ""))
        sample["tokens"] = tokens


def compute_weights(samples: list[dict], genres: list[str]) -> dict:
    """
    Compute weights from samples using standard TF-IDF formula.

    Args:
        samples: List of sample dictionaries with title, body, and expected_genres
        genres: List of genre names to compute weights for

    TF-IDF formula (scikit-learn style):
    - TF: term frequency in genre / total terms in genre
    - IDF: log((total_docs + 1) / (docs_with_term + 1)) + 1
    - TF-IDF: TF * IDF
    """
    enrich_samples(samples)
    total_docs = len(samples)
    feature_counts, genre_totals, doc_frequency = build_feature_counts(samples, genres)

    # Compute IDF for each term across entire corpus (scikit-learn style)
    # IDF = log((n_samples + 1) / (df + 1)) + 1
    feature_idf = []
    for term in FEATURE_VOCAB:
        df = doc_frequency[term]  # number of documents containing this term
        # scikit-learn TfidfVectorizer default: smooth_idf=True
        idf = math.log((total_docs + 1) / (df + 1)) + 1.0
        feature_idf.append(round(idf, 3))

    # Compute TF-IDF weights per genre
    tfidf_weights = []
    for genre in genres:
        total_terms_in_genre = sum(feature_counts[genre].values())
        if total_terms_in_genre == 0:
            # No terms in this genre, use zero weights
            tfidf_weights.append([0.0] * len(FEATURE_VOCAB))
            continue

        row = []
        for idx, term in enumerate(FEATURE_VOCAB):
            # TF: term frequency in genre / total terms in genre
            tf = feature_counts[genre][term] / total_terms_in_genre
            # Use the corpus-wide IDF
            idf = feature_idf[idx]
            # TF-IDF weight
            tfidf = tf * idf
            row.append(round(tfidf, 3))
        tfidf_weights.append(row)

    # Compute embedding weights (weighted average by term frequency)
    embedding_weights = []
    for genre in genres:
        agg = [0.0] * EMBEDDING_DIM
        total_weight = 0.0
        for term in FEATURE_VOCAB:
            vec = EMBED_LOOKUP.get(term)
            if vec and feature_counts[genre][term] > 0:
                # Weight by term frequency in this genre
                weight = feature_counts[genre][term]
                total_weight += weight
                for idx, value in enumerate(vec):
                    agg[idx] += value * weight
        if total_weight > 0:
            agg = [round(v / total_weight, 3) for v in agg]
        embedding_weights.append(agg)

    # Compute bias using inverse frequency (class imbalance aware)
    # Use log to prevent extreme values for very rare genres
    total_samples = sum(genre_totals.values())
    bias = []
    for genre in genres:
        genre_count = genre_totals[genre]
        if genre_count == 0:
            # Very high bias for genres with no samples (will be filtered out anyway)
            bias_value = 0.0
        else:
            # Inverse frequency with smoothing: log(total / count)
            # This gives higher bias to rarer genres
            bias_value = math.log(total_samples / genre_count) if total_samples > 0 else 0.0
        bias.append(round(bias_value, 3))

    return {
        "_comment": "統計的に計算されたジャンル分類器の重み付け。recap_genre_learning_resultsテーブルから取得したデータに基づく。標準的なTF-IDF計算式を使用（scikit-learn互換）。",
        "feature_dim": len(FEATURE_VOCAB),
        "embedding_dim": EMBEDDING_DIM,
        "feature_vocab": FEATURE_VOCAB,
        "feature_idf": feature_idf,
        "genres": genres,
        "tfidf_weights": tfidf_weights,
        "embedding_weights": embedding_weights,
        "bias": bias,
    }


def main() -> None:
    parser = argparse.ArgumentParser(
        description="Regenerate genre classifier weights from database or JSON file.",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
Examples:
  # Use database (past 14 days, updates genre_classifier_weights.json by default)
  python retrain_genre_classifier.py --from-db

  # Use database with custom output path
  python retrain_genre_classifier.py --from-db --output weights.json

  # Use database with custom days
  python retrain_genre_classifier.py --from-db --days 7

  # Use JSON file
  python retrain_genre_classifier.py dataset.json weights.json
        """,
    )
    parser.add_argument(
        "dataset",
        type=Path,
        nargs="?",
        help="Path to labelled dataset JSON (required if --from-db is not used)",
    )
    parser.add_argument(
        "output",
        type=Path,
        nargs="?",
        help="Output weights JSON path (required if --from-db is not used)",
    )
    parser.add_argument(
        "--from-db",
        action="store_true",
        help="Fetch articles from database instead of JSON file",
    )
    parser.add_argument(
        "--days",
        type=int,
        default=14,
        help="Number of days to look back when using --from-db (default: 14)",
    )
    parser.add_argument(
        "--alt-backend-dsn",
        type=str,
        default=None,
        help="Alt-backend database connection string (uses env vars if not provided)",
    )
    parser.add_argument(
        "--recap-db-dsn",
        type=str,
        default=None,
        help="Recap database connection string (uses RECAP_DB_DSN env var if not provided)",
    )
    args = parser.parse_args()

    if args.from_db:
        # Fetch from database
        if args.recap_db_dsn is None:
            recap_db_dsn = os.getenv("RECAP_DB_DSN")
            if not recap_db_dsn:
                parser.error("--recap-db-dsn or RECAP_DB_DSN environment variable is required when using --from-db")
        else:
            recap_db_dsn = args.recap_db_dsn

        # Fetch distinct genres from database
        print(f"Fetching distinct genres from database (past {args.days} days)...")
        try:
            genres = fetch_distinct_genres_from_db(
                recap_db_dsn,
                days=args.days,
                min_samples=10,
            )
        except RuntimeError as e:
            parser.error(f"Failed to fetch genres from database: {e}")

        # Default output path to genre_classifier_weights.json in recap-subworker resources
        if args.output is None:
            script_dir = Path(__file__).parent
            # Go up from recap-worker/scripts to project root, then to recap-subworker
            project_root = script_dir.parent.parent
            output_path = project_root / "recap-subworker" / "recap_subworker" / "resources" / "genre_classifier_weights.json"
        else:
            output_path = args.output

        samples = fetch_articles_from_db(
            args.alt_backend_dsn,
            recap_db_dsn,
            days=args.days,
            genres=genres,
        )
    else:
        # Load from JSON file
        if args.dataset is None or args.output is None:
            parser.error("dataset and output arguments are required when not using --from-db")

        samples = load_samples(args.dataset)
        output_path = args.output

    if not samples:
        print("Warning: No samples found. Cannot compute weights.")
        return

    # For JSON file mode, use GENRES constant
    if not args.from_db:
        genres = GENRES

    print(f"Computing weights from {len(samples)} samples for {len(genres)} genres...")
    weights = compute_weights(samples, genres)

    # Write output
    output_path.write_text(json.dumps(weights, indent=2, ensure_ascii=False) + "\n", encoding="utf-8")
    print(f"Successfully wrote weights to {output_path}")


if __name__ == "__main__":
    main()

