#!/usr/bin/env python3
"""
Low-quality tag regeneration script.

This script identifies articles with low-confidence tags and regenerates them
using the current tag extraction logic. Tags are only updated when the new
confidence is higher than the existing one.

Usage:
    uv run python scripts/regenerate_low_quality_tags.py --help
    uv run python scripts/regenerate_low_quality_tags.py --dry-run
    uv run python scripts/regenerate_low_quality_tags.py --confidence-threshold 0.5 --batch-size 100
    uv run python scripts/regenerate_low_quality_tags.py --limit 1000
    uv run python scripts/regenerate_low_quality_tags.py --all --batch-size 10
    uv run python scripts/regenerate_low_quality_tags.py --all --language ja --batch-size 50

Examples:
    # Preview what would be regenerated (no changes)
    uv run python scripts/regenerate_low_quality_tags.py --dry-run --limit 50

    # Regenerate tags for articles with confidence < 0.4
    uv run python scripts/regenerate_low_quality_tags.py --confidence-threshold 0.4 --batch-size 50

    # Run a full regeneration with default settings
    uv run python scripts/regenerate_low_quality_tags.py

    # Regenerate tags for ALL articles (ignore confidence threshold)
    uv run python scripts/regenerate_low_quality_tags.py --all --batch-size 10

    # Preview all articles regeneration
    uv run python scripts/regenerate_low_quality_tags.py --all --dry-run --limit 50

    # Regenerate tags for Japanese articles only
    uv run python scripts/regenerate_low_quality_tags.py --all --language ja --batch-size 50

    # Resume an interrupted run using offset
    uv run python scripts/regenerate_low_quality_tags.py --all --language ja --offset 1000 --batch-size 50
"""

from __future__ import annotations

import argparse
import os
import sys
from dataclasses import dataclass
from typing import TYPE_CHECKING, Any

import psycopg2
import structlog

# Add parent directory to path for imports
sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))

from article_fetcher.fetch import ArticleFetcher, ArticleFetcherConfig
from tag_extractor.extract import TagExtractionConfig, TagExtractor
from tag_generator.cascade import CascadeConfig, CascadeController
from tag_inserter.upsert_tags import TagInserter, TagInserterConfig

if TYPE_CHECKING:
    from psycopg2.extensions import connection as Connection

logger = structlog.get_logger(__name__)


@dataclass
class RegenerationConfig:
    """Configuration for tag regeneration."""

    confidence_threshold: float = 0.5
    batch_size: int = 100
    limit: int | None = None
    dry_run: bool = False
    verbose: bool = False
    all_articles: bool = False
    language_filter: str | None = None  # 'ja', 'en', etc.
    offset: int = 0  # Skip first N articles (for resuming)


def get_database_connection() -> Connection:
    """Create a database connection from environment variables."""
    db_host = os.getenv("DB_HOST", "localhost")
    db_port = os.getenv("DB_PORT", "5432")
    db_name = os.getenv("DB_NAME", "alt")
    db_user = os.getenv("DB_USER", "postgres")
    db_password = os.getenv("DB_PASSWORD", "")

    conn = psycopg2.connect(
        host=db_host,
        port=db_port,
        dbname=db_name,
        user=db_user,
        password=db_password,
    )
    return conn


def ensure_connection(conn: Connection | None) -> Connection:
    """Ensure we have a valid database connection, reconnecting if needed."""
    if conn is None:
        return get_database_connection()

    try:
        # Test the connection with a simple query
        with conn.cursor() as cursor:
            cursor.execute("SELECT 1")
            cursor.fetchone()
        return conn
    except (psycopg2.Error, psycopg2.OperationalError):
        # Connection is dead, create a new one
        logger.warning("Database connection lost, reconnecting...")
        try:
            conn.close()
        except Exception:
            pass
        return get_database_connection()


def count_low_confidence_articles(conn: Connection, threshold: float) -> int:
    """Count articles with average confidence below threshold."""
    query = """
        SELECT COUNT(DISTINCT a.id)
        FROM articles a
        INNER JOIN article_tags at ON a.id = at.article_id
        INNER JOIN feed_tags ft ON at.feed_tag_id = ft.id
        GROUP BY a.id
        HAVING AVG(ft.confidence) < %s
    """
    with conn.cursor() as cursor:
        cursor.execute(
            f"SELECT COUNT(*) FROM ({query}) AS subquery",
            (threshold,),
        )
        result = cursor.fetchone()
        return result[0] if result else 0


def count_all_articles(conn: Connection) -> int:
    """Count all articles with content."""
    query = """
        SELECT COUNT(*)
        FROM articles a
        WHERE a.content IS NOT NULL AND a.content != ''
    """
    with conn.cursor() as cursor:
        cursor.execute(query)
        result = cursor.fetchone()
        return result[0] if result else 0


def preview_low_confidence_articles(
    conn: Connection,
    threshold: float,
    limit: int = 10,
) -> list[dict[str, Any]]:
    """Preview articles that would be regenerated."""
    query = """
        SELECT
            a.id::text AS id,
            a.title,
            LEFT(a.content, 100) AS content_preview,
            AVG(ft.confidence) AS avg_confidence,
            COUNT(at.feed_tag_id) AS tag_count,
            ARRAY_AGG(ft.tag_name) AS tags
        FROM articles a
        INNER JOIN article_tags at ON a.id = at.article_id
        INNER JOIN feed_tags ft ON at.feed_tag_id = ft.id
        GROUP BY a.id, a.title, a.content
        HAVING AVG(ft.confidence) < %s
        ORDER BY AVG(ft.confidence) ASC
        LIMIT %s
    """
    with conn.cursor() as cursor:
        cursor.execute(query, (threshold, limit))
        if cursor.description is None:
            return []
        columns = [desc[0] for desc in cursor.description]
        return [dict(zip(columns, row, strict=False)) for row in cursor.fetchall()]


def run_dry_run(conn: Connection, config: RegenerationConfig) -> None:
    """Run in dry-run mode - preview changes without making them."""
    print(f"\n{'=' * 60}")
    print("DRY RUN MODE - No changes will be made")
    print(f"{'=' * 60}\n")

    # Count total articles
    if config.all_articles:
        total = count_all_articles(conn)
        print(f"Total articles with content: {total}")
    else:
        total = count_low_confidence_articles(conn, config.confidence_threshold)
        print(f"Total articles with avg confidence < {config.confidence_threshold}: {total}")

    if total == 0:
        print("No articles found for regeneration.")
        return

    # Preview articles (only for low-confidence mode)
    if not config.all_articles:
        preview_limit = min(config.limit or 10, 20)
        articles = preview_low_confidence_articles(conn, config.confidence_threshold, preview_limit)

        print(f"\nPreviewing first {len(articles)} articles:\n")
        print("-" * 60)

        for article in articles:
            print(f"ID: {article['id']}")
            print(f"Title: {article['title'][:80]}...")
            print(f"Avg Confidence: {article['avg_confidence']:.3f}")
            print(f"Tag Count: {article['tag_count']}")
            print(f"Current Tags: {', '.join(article['tags'][:5])}{'...' if len(article['tags']) > 5 else ''}")
            print("-" * 60)

    # Estimate batches
    batch_count = (total + config.batch_size - 1) // config.batch_size
    if config.limit:
        batch_count = min(batch_count, (config.limit + config.batch_size - 1) // config.batch_size)
        total_to_process = min(total, config.limit)
    else:
        total_to_process = total

    print("\nRegeneration plan:")
    print(f"  - Mode: {'ALL articles' if config.all_articles else 'Low-confidence only'}")
    print(f"  - Articles to process: {total_to_process}")
    print(f"  - Batch size: {config.batch_size}")
    print(f"  - Estimated batches: {batch_count}")
    if not config.all_articles:
        print(f"  - Confidence threshold: {config.confidence_threshold}")
    if config.language_filter:
        print(f"  - Language filter: {config.language_filter}")
        print("  - Note: Language detection happens during processing, estimated batches may vary")
    if config.offset > 0:
        print(f"  - Starting offset: {config.offset}")


def filter_articles_by_language(
    articles: list[dict[str, Any]],
    language: str,
    tag_extractor: TagExtractor,
) -> list[dict[str, Any]]:
    """Filter articles by detected language.

    Args:
        articles: List of article dicts with 'title' and 'content' keys.
        language: Target language code (e.g., 'ja', 'en').
        tag_extractor: TagExtractor instance for language detection.

    Returns:
        Filtered list of articles matching the specified language.
    """
    filtered = []
    for article in articles:
        text = f"{article.get('title', '')}\n{article.get('content', '')}"
        detected_lang = tag_extractor._detect_language(text)
        if detected_lang == language:
            filtered.append(article)
    return filtered


def run_regeneration(conn: Connection, config: RegenerationConfig) -> dict[str, Any]:
    """Run the tag regeneration process."""
    # Set up components
    article_fetcher = ArticleFetcher(ArticleFetcherConfig(batch_size=config.batch_size))
    device = os.getenv("TAG_DEVICE", "cpu")
    tag_extractor = TagExtractor(TagExtractionConfig(device=device))
    tag_inserter = TagInserter(TagInserterConfig())
    cascade_controller = CascadeController(CascadeConfig())

    # Track overall stats
    total_stats: dict[str, Any] = {
        "total_processed": 0,
        "successful": 0,
        "failed": 0,
        "updated_higher_confidence": 0,
        "skipped_lower_confidence": 0,
        "batches_completed": 0,
    }

    # Count total for progress tracking
    if config.all_articles:
        total_articles = count_all_articles(conn)
    else:
        total_articles = count_low_confidence_articles(conn, config.confidence_threshold)

    # When using language filter, we process all articles and filter by language
    # The limit applies to the number of matching-language articles processed
    if config.language_filter:
        print(f"\nLanguage filter enabled: {config.language_filter}")
        print("Note: Language filtering happens after fetching, so progress may be slower.")
        articles_to_process = config.limit or total_articles
    else:
        articles_to_process = config.limit or total_articles

    print("\nStarting regeneration...")
    if config.all_articles:
        print("Mode: ALL articles")
    else:
        print(f"Confidence threshold: {config.confidence_threshold}")
    if config.language_filter:
        print(f"Language filter: {config.language_filter}")
    print(f"Batch size: {config.batch_size}")
    if config.offset > 0:
        print(f"Starting offset: {config.offset}")
    print()

    # Track language-filtered article counts
    total_stats["language_filtered_count"] = 0
    total_stats["language_skipped_count"] = 0

    processed = 0  # Count of language-matched articles processed
    db_offset = config.offset  # DB-level offset for fetching
    batch_num = 0
    consecutive_failures = 0
    max_consecutive_failures = 3
    # Fetch more articles when using language filter to compensate for filtering
    fetch_multiplier = 3 if config.language_filter else 1

    while processed < articles_to_process:
        batch_num += 1
        remaining = articles_to_process - processed
        # Fetch more when filtering by language
        fetch_size = min(config.batch_size * fetch_multiplier, remaining * fetch_multiplier)

        try:
            # Ensure connection is valid before fetching
            conn = ensure_connection(conn)

            # Fetch articles based on mode
            if config.all_articles:
                articles = article_fetcher.fetch_all_articles_for_regeneration(
                    conn,
                    offset=db_offset,
                    limit=fetch_size,
                )
            else:
                articles = article_fetcher.fetch_low_confidence_articles(
                    conn,
                    confidence_threshold=config.confidence_threshold,
                    limit=fetch_size,
                )

            if not articles:
                print("No more articles to process.")
                break

            # Update db_offset for next batch
            db_offset += len(articles)

            # Apply language filter if specified
            if config.language_filter:
                original_count = len(articles)
                articles = filter_articles_by_language(articles, config.language_filter, tag_extractor)
                skipped = original_count - len(articles)
                total_stats["language_filtered_count"] += len(articles)
                total_stats["language_skipped_count"] += skipped
                if config.verbose:
                    logger.info(
                        "Language filter applied",
                        language=config.language_filter,
                        original_count=original_count,
                        filtered_count=len(articles),
                        skipped=skipped,
                    )

                if not articles:
                    # No matching articles in this batch, continue to next
                    if config.verbose:
                        print(f"Batch {batch_num}: No {config.language_filter} articles found, continuing...")
                    continue

            # Limit to remaining needed articles
            if len(articles) > remaining:
                articles = articles[:remaining]

            # Extract tags for all articles
            article_tags_batch = []
            for article in articles:
                try:
                    outcome = tag_extractor.extract_tags_with_metrics(article["title"], article["content"])

                    if outcome.tags:
                        decision = cascade_controller.evaluate(outcome)
                        article_tags_batch.append(
                            {
                                "article_id": article["id"],
                                "tags": outcome.tags,
                                "tag_confidences": outcome.tag_confidences,
                                "cascade": decision.as_dict(),
                            }
                        )
                except Exception as e:
                    logger.error(f"Error extracting tags for article {article.get('id')}: {e}")
                    total_stats["failed"] += 1

            # Upsert tags with confidence comparison
            if article_tags_batch:
                # Ensure connection is valid after potentially long tag extraction
                conn = ensure_connection(conn)

                if conn.autocommit:
                    conn.autocommit = False

                try:
                    result = tag_inserter.batch_upsert_tags_with_comparison(conn, article_tags_batch)

                    if result.get("success"):
                        conn.commit()
                        total_stats["successful"] += result.get("processed_articles", 0)
                        total_stats["updated_higher_confidence"] += result.get("updated_higher_confidence", 0)
                        total_stats["skipped_lower_confidence"] += result.get("skipped_lower_confidence", 0)
                    else:
                        conn.rollback()
                        total_stats["failed"] += result.get("failed_articles", 0)
                except Exception as e:
                    try:
                        conn.rollback()
                    except Exception:
                        pass
                    logger.error(f"Batch upsert failed: {e}")
                    total_stats["failed"] += len(article_tags_batch)
                finally:
                    try:
                        conn.autocommit = True
                    except Exception:
                        pass

            batch_processed = len(articles)
            total_stats["total_processed"] += batch_processed
            total_stats["batches_completed"] += 1
            processed += batch_processed

            # Progress logging
            progress_pct = (processed / articles_to_process) * 100
            if config.language_filter:
                print(
                    f"Batch {batch_num}: Processed {batch_processed} {config.language_filter} articles "
                    f"({processed}/{articles_to_process} total, {progress_pct:.1f}%) [db_offset: {db_offset}]"
                )
            else:
                print(
                    f"Batch {batch_num}: Processed {batch_processed} articles "
                    f"({processed}/{articles_to_process} total, {progress_pct:.1f}%)"
                )

            if config.verbose:
                print(f"  - Updated (higher confidence): {total_stats['updated_higher_confidence']}")
                print(f"  - Skipped (lower confidence): {total_stats['skipped_lower_confidence']}")
                print(f"  - Failed: {total_stats['failed']}")
                if config.language_filter:
                    print(f"  - Language matched: {total_stats['language_filtered_count']}")
                    print(f"  - Language skipped: {total_stats['language_skipped_count']}")

            # Reset consecutive failure count on success
            consecutive_failures = 0

        except Exception as e:
            logger.error(f"Batch {batch_num} failed: {e}")
            total_stats["failed"] += config.batch_size
            consecutive_failures += 1

            # Break on connection errors to avoid infinite loop
            error_msg = str(e).lower()
            if "connection" in error_msg or "closed" in error_msg:
                logger.error("Connection error detected, stopping regeneration")
                break

            # Break on too many consecutive failures
            if consecutive_failures >= max_consecutive_failures:
                logger.error(f"Too many consecutive failures ({consecutive_failures}), stopping")
                break

    return total_stats


def main() -> None:
    """Main entry point."""
    parser = argparse.ArgumentParser(
        description="Regenerate low-quality tags for articles",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog=__doc__,
    )

    parser.add_argument(
        "--confidence-threshold",
        type=float,
        default=0.5,
        help="Maximum average confidence to include (default: 0.5)",
    )
    parser.add_argument(
        "--batch-size",
        type=int,
        default=100,
        help="Number of articles per batch (default: 100)",
    )
    parser.add_argument(
        "--limit",
        type=int,
        default=None,
        help="Maximum number of articles to process (default: all)",
    )
    parser.add_argument(
        "--dry-run",
        action="store_true",
        help="Preview changes without making them",
    )
    parser.add_argument(
        "--verbose",
        "-v",
        action="store_true",
        help="Enable verbose output",
    )
    parser.add_argument(
        "--all",
        action="store_true",
        help="Regenerate tags for all articles (ignore confidence threshold)",
    )
    parser.add_argument(
        "--language",
        type=str,
        default=None,
        help="Filter by detected language (e.g., 'ja' for Japanese, 'en' for English)",
    )
    parser.add_argument(
        "--offset",
        type=int,
        default=0,
        help="Skip first N articles (for resuming interrupted runs)",
    )

    args = parser.parse_args()

    config = RegenerationConfig(
        confidence_threshold=args.confidence_threshold,
        batch_size=args.batch_size,
        limit=args.limit,
        dry_run=args.dry_run,
        verbose=args.verbose,
        all_articles=args.all,
        language_filter=args.language,
        offset=args.offset,
    )

    # Configure logging
    import logging

    log_level = logging.DEBUG if config.verbose else logging.INFO
    structlog.configure(
        wrapper_class=structlog.make_filtering_bound_logger(log_level),
    )

    try:
        conn = get_database_connection()
        print("Connected to database successfully.")

        if config.dry_run:
            run_dry_run(conn, config)
        else:
            stats = run_regeneration(conn, config)

            print(f"\n{'=' * 60}")
            print("Regeneration Complete")
            print(f"{'=' * 60}")
            print(f"Total processed: {stats['total_processed']}")
            print(f"Successful: {stats['successful']}")
            print(f"Failed: {stats['failed']}")
            print(f"Updated (higher confidence): {stats['updated_higher_confidence']}")
            print(f"Skipped (lower confidence): {stats['skipped_lower_confidence']}")
            print(f"Batches completed: {stats['batches_completed']}")
            if config.language_filter:
                print(f"Language matched ({config.language_filter}): {stats.get('language_filtered_count', 0)}")
                print(f"Language skipped: {stats.get('language_skipped_count', 0)}")

        conn.close()

    except psycopg2.Error as e:
        print(f"Database error: {e}", file=sys.stderr)
        sys.exit(1)
    except Exception as e:
        print(f"Error: {e}", file=sys.stderr)
        sys.exit(1)


if __name__ == "__main__":
    main()
