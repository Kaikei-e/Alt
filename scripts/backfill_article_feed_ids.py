#!/usr/bin/env python3
"""
Backfill article feed_ids from matching feed links.

This script updates feed_id for existing articles where article.url matches feed.link.
This is a data migration script, not a schema migration.

Usage:
    # From host:
    docker compose run --rm tag-generator python3 /scripts/backfill_article_feed_ids.py

    # Or via Makefile:
    make backfill-feed-ids
"""

import os
import sys
import psycopg2
from psycopg2.extras import RealDictCursor
from typing import Optional

# Database connection parameters
# Use POSTGRES_USER for backfill operations (requires UPDATE on articles and SELECT on feeds)
DB_HOST = os.getenv("DB_HOST", "localhost")
DB_PORT = os.getenv("DB_PORT", "5432")
DB_NAME = os.getenv("POSTGRES_DB") or os.getenv("DB_NAME", "alt")
DB_USER = os.getenv("POSTGRES_USER", "alt_db_user")
DB_PASSWORD = os.getenv("POSTGRES_PASSWORD", "")

# Build connection string
DATABASE_URL = os.getenv(
    "DATABASE_URL",
    f"postgresql://{DB_USER}:{DB_PASSWORD}@{DB_HOST}:{DB_PORT}/{DB_NAME}",
)


def get_db_connection():
    """Create and return a database connection."""
    try:
        conn = psycopg2.connect(DATABASE_URL)
        return conn
    except psycopg2.Error as e:
        print(f"Error connecting to database: {e}", file=sys.stderr)
        sys.exit(1)


def backfill_feed_ids(conn, batch_size: int = 1000) -> dict:
    """
    Update feed_id for articles where feed_id is NULL and article.url matches feed.link.

    Args:
        conn: Database connection
        batch_size: Number of articles to process in each batch

    Returns:
        Dictionary with update statistics
    """
    stats = {
        "total_articles_with_null_feed_id": 0,
        "articles_with_matching_feed": 0,
        "articles_updated": 0,
        "articles_without_match": 0,
    }

    try:
        with conn.cursor(cursor_factory=RealDictCursor) as cursor:
            # Count total articles with NULL feed_id
            cursor.execute("SELECT COUNT(*) FROM articles WHERE feed_id IS NULL")
            stats["total_articles_with_null_feed_id"] = cursor.fetchone()["count"]

            if stats["total_articles_with_null_feed_id"] == 0:
                print("No articles with NULL feed_id found. Nothing to update.")
                return stats

            print(f"Found {stats['total_articles_with_null_feed_id']} articles with NULL feed_id")

            # Process articles in batches
            offset = 0
            while True:
                # Fetch batch of articles with NULL feed_id
                cursor.execute(
                    """
                    SELECT id, url
                    FROM articles
                    WHERE feed_id IS NULL
                    ORDER BY created_at DESC, id DESC
                    LIMIT %s OFFSET %s
                    """,
                    (batch_size, offset),
                )

                articles = cursor.fetchall()

                if not articles:
                    break

                print(f"Processing batch: {offset} to {offset + len(articles)} articles...")

                updated_count = 0
                for article in articles:
                    article_id = article["id"]
                    article_url = article["url"]

                    # Find matching feed
                    cursor.execute(
                        """
                        SELECT id
                        FROM feeds
                        WHERE link = %s
                        ORDER BY created_at DESC, id DESC
                        LIMIT 1
                        """,
                        (article_url,),
                    )

                    feed_result = cursor.fetchone()

                    if feed_result:
                        feed_id = feed_result["id"]
                        # Update article with feed_id
                        cursor.execute(
                            "UPDATE articles SET feed_id = %s WHERE id = %s",
                            (feed_id, article_id),
                        )
                        updated_count += 1
                        stats["articles_updated"] += 1
                    else:
                        stats["articles_without_match"] += 1

                # Commit after each batch
                conn.commit()
                print(f"  Updated {updated_count} articles in this batch")

                offset += len(articles)

                # Break if we got fewer articles than batch_size (last batch)
                if len(articles) < batch_size:
                    break

            # Final count
            cursor.execute(
                """
                SELECT COUNT(*) as matched
                FROM articles a
                INNER JOIN feeds f ON a.url = f.link
                WHERE a.feed_id IS NOT NULL
                """
            )
            stats["articles_with_matching_feed"] = cursor.fetchone()["matched"]

    except psycopg2.Error as e:
        print(f"Database error: {e}", file=sys.stderr)
        conn.rollback()
        sys.exit(1)

    return stats


def main():
    """Main execution function."""
    print("=" * 60)
    print("Backfilling article feed_ids from matching feed links")
    print("=" * 60)
    print()

    conn = get_db_connection()
    try:
        stats = backfill_feed_ids(conn)

        print()
        print("=" * 60)
        print("Update Summary:")
        print("=" * 60)
        print(f"Total articles with NULL feed_id: {stats['total_articles_with_null_feed_id']}")
        print(f"Articles updated: {stats['articles_updated']}")
        print(f"Articles without matching feed: {stats['articles_without_match']}")
        print(f"Total articles with matching feed: {stats['articles_with_matching_feed']}")
        print("=" * 60)

        if stats["articles_updated"] > 0:
            print(f"\n✓ Successfully updated {stats['articles_updated']} articles")
        else:
            print("\n⚠ No articles were updated")

    finally:
        conn.close()


if __name__ == "__main__":
    main()

