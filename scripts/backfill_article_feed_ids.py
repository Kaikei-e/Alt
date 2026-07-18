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


def get_db_connection() -> "psycopg2.extensions.connection":
    """Create and return a database connection."""
    try:
        conn = psycopg2.connect(DATABASE_URL, connect_timeout=10)
        return conn
    except psycopg2.Error as e:
        print(f"Error connecting to database: {e}", file=sys.stderr)
        sys.exit(1)


def backfill_feed_ids(conn: "psycopg2.extensions.connection", batch_size: int = 1000) -> dict[str, int]:
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

            # Process articles in batches using keyset pagination.
            #
            # OFFSET-based pagination is unsafe here: each batch's UPDATE moves
            # rows out of the `WHERE feed_id IS NULL` result set, so advancing
            # OFFSET by the batch size skips rows that shifted into the
            # already-scanned range. Tracking the last-seen id and querying
            # `id > last_seen_id` avoids that, since it never depends on the
            # position of rows within the (shrinking) matching set.
            last_seen_id: str | None = None
            while True:
                # Fetch batch of articles with NULL feed_id
                if last_seen_id is None:
                    cursor.execute(
                        """
                        SELECT id, url
                        FROM articles
                        WHERE feed_id IS NULL
                        ORDER BY id
                        LIMIT %s
                        """,
                        (batch_size,),
                    )
                else:
                    cursor.execute(
                        """
                        SELECT id, url
                        FROM articles
                        WHERE feed_id IS NULL AND id > %s
                        ORDER BY id
                        LIMIT %s
                        """,
                        (last_seen_id, batch_size),
                    )

                articles = cursor.fetchall()

                if not articles:
                    break

                print(f"Processing batch of {len(articles)} articles after id={last_seen_id}...")

                article_ids = [a["id"] for a in articles]
                cursor.execute(
                    """
                    UPDATE articles AS a
                    SET feed_id = f.id
                    FROM (
                        SELECT DISTINCT ON (link) id, link
                        FROM feeds
                        ORDER BY link, created_at DESC, id DESC
                    ) AS f
                    WHERE a.id = ANY(%s)
                      AND a.feed_id IS NULL
                      AND a.url = f.link
                    RETURNING a.id
                    """,
                    (article_ids,),
                )
                updated_ids = {row["id"] for row in cursor.fetchall()}
                updated_count = len(updated_ids)
                stats["articles_updated"] += updated_count
                stats["articles_without_match"] += len(articles) - updated_count

                # Commit after each batch
                conn.commit()
                print(f"  Updated {updated_count} articles in this batch")

                last_seen_id = articles[-1]["id"]

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


def main() -> None:
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

