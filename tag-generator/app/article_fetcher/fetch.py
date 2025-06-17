module_name = "article_fetcher"

import psycopg2
import psycopg2.extras
from psycopg2.extras import DictCursor

BATCH_SIZE = 500

def fetch_articles(conn, last_created_at: str, last_id: str):
    sql = """
        SELECT id::text AS id, title, content, created_at
        FROM articles
        WHERE
            (created_at < %s)
            OR (created_at = %s AND id::text < %s)
        ORDER BY created_at DESC, id DESC
        LIMIT %s
    """
    with conn.cursor(cursor_factory=psycopg2.extras.DictCursor) as cur:
        cur.execute(sql, (last_created_at, last_created_at, last_id, BATCH_SIZE))
        return cur.fetchall()