import os
from datetime import datetime, UTC

import psycopg2
from psycopg2.extras import DictCursor
from article_fetcher.fetch import fetch_articles
from tag_extractor.extract import extract_tags
from tag_inserter.upsert_tags import upsert_tags


def main():
    print(f"Hello from tag-generator!")

    db_DSN = f"postgresql://{os.getenv('DB_TAG_GENERATOR_USER')}:{os.getenv('DB_TAG_GENERATOR_PASSWORD')}@{os.getenv('DB_HOST')}:{os.getenv('DB_PORT')}/{os.getenv('DB_NAME')}"

    conn = psycopg2.connect(
        db_DSN,
    )
    # 初期カーソル位置：未来から遡るので最大値を設定
    last_created_at = datetime.now(UTC).isoformat()
    last_id = "ffffffff-ffff-ffff-ffff-ffffffffffff"
    try:
        while True:
            rows = fetch_articles(conn, last_created_at, last_id)
            if not rows:
                break
            for row in rows:
                aid, title, content, c_at = row["id"], row["title"], row["content"], row["created_at"]
                tags = extract_tags(title, content)
                upsert_tags(conn, aid, tags)
                last_created_at, last_id = c_at, aid
    finally:
        conn.close()

if __name__ == "__main__":
    main()