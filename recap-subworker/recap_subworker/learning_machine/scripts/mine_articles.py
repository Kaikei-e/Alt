import asyncio
import logging
import sys
import json
import argparse
from pathlib import Path
from sqlalchemy.ext.asyncio import create_async_engine
from sqlalchemy import text
from typing import List, Dict, Any

# Ensure we can import from recap-subworker
# Add 3 levels up to path (scripts -> learning_machine -> recap-subworker -> root)
# But actually, recap-subworker is the root for python path usually if we run from there
# Let's handle sys.path robustly
current_dir = Path(__file__).resolve().parent
project_root = current_dir.parent.parent.parent
if str(project_root) not in sys.path:
    sys.path.insert(0, str(project_root))

from recap_subworker.infra.config import Settings

logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)

async def fetch_articles(db_url: str, limit: int, days: int) -> List[Dict[str, Any]]:
    """Fetch raw articles from recap-db."""
    engine = create_async_engine(db_url)

    async with engine.connect() as conn:
        logger.info(f"Querying articles from last {days} days (Limit: {limit})...")

        # We need to query recap_job_articles
        # Note: We want unique articles. The same article might be in multiple jobs maybe?
        # But recap_job_articles has (job_id, article_id) PK usually.
        # We should group by article_id or url to dedup.

        query = text("""
            SELECT DISTINCT ON (source_url)
                title,
                fulltext_html,
                published_at,
                source_url as url,
                article_id -- using article_id as ID
            FROM recap_job_articles
            WHERE published_at > NOW() - INTERVAL '1 day' * :days
              AND fulltext_html IS NOT NULL
            ORDER BY source_url, published_at DESC
            LIMIT :limit
        """)

        result = await conn.execute(query, {"days": days, "limit": limit})
        articles = []

        import trafilatura

        for row in result:
             text_content = ""
             if row.fulltext_html:
                 # Fast extraction from HTML string
                 text_content = trafilatura.extract(row.fulltext_html) or ""

             if not text_content:
                 # Fallback: maybe it's already text or failed
                 text_content = row.fulltext_html if row.fulltext_html and "<" not in row.fulltext_html else ""

             if not text_content.strip():
                 continue

             articles.append({
                 "id": row.article_id,
                 "title": row.title,
                 "content": text_content,
                 "published_at": str(row.published_at),
                 "url": row.url
             })

    await engine.dispose()
    return articles

def get_db_url_with_password(settings: Settings) -> str:
    """Resolve DB URL with password from secrets if needed."""
    db_url = settings.db_url

    # Local execution adjustment (recap-db -> localhost)
    # NOTE: This assumes we are running locally and using the port forwarded/exposed
    if "recap-db" in db_url:
        logger.info("Adjusting DB URL for local execution...")
        db_url = db_url.replace("recap-db", "localhost").replace("5432", "5435")

    from urllib.parse import urlparse, urlunparse

    # Try to find secrets directory
    # Assume we are in recap-subworker/learning_machine/scripts/mine_articles.py
    # Repo root is ../../../
    secret_path = project_root.parent / "secrets" / "recap_db_password.txt"

    if secret_path.exists():
        try:
            with open(secret_path, "r") as f:
                password = f.read().strip()

            u = urlparse(db_url)
            if '@' in u.netloc:
                user_pass, host_port = u.netloc.rsplit('@', 1)
                # Naive replacement, assuming standard format
                if ':' in user_pass:
                    user, _ = user_pass.split(':', 1)
                    new_user_pass = f"{user}:{password}"
                else:
                    new_user_pass = f"{user_pass}:{password}"

                new_netloc = f"{new_user_pass}@{host_port}"
                db_url = urlunparse((u.scheme, new_netloc, u.path, u.params, u.query, u.fragment))
                logger.info("Injected password from secrets.")
        except Exception as e:
            logger.warning(f"Failed to read password secret: {e}")

    return db_url

async def main():
    parser = argparse.ArgumentParser(description="Mine articles from recap-db")
    parser.add_argument("--days", type=int, default=180, help="Lookback days")
    parser.add_argument("--limit", type=int, default=20000, help="Max articles")
    parser.add_argument("--output", type=str, default="data/raw_articles.jsonl", help="Output path")
    args = parser.parse_args()

    settings = Settings()
    db_url = get_db_url_with_password(settings)

    articles = await fetch_articles(db_url, args.limit, args.days)
    logger.info(f"Fetched {len(articles)} articles.")

    output_path = Path(args.output)
    output_path.parent.mkdir(parents=True, exist_ok=True)

    with open(output_path, "w", encoding="utf-8") as f:
        for article in articles:
            f.write(json.dumps(article, ensure_ascii=False) + "\n")

    logger.info(f"Saved to {output_path}")

if __name__ == "__main__":
    asyncio.run(main())
