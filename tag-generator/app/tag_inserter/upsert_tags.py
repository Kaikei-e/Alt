module_name = "tag_inserter"

import psycopg2
import psycopg2.extras

BATCH_SIZE = 1000


def upsert_tags(conn, article_id: str, tags: list[str]):
    """
    tags テーブルと article_tags 中間テーブルへまとめて Upsert。
    article_id は文字列 UUID。
    """
    with conn.cursor() as cur:
        # 1) tags テーブルに挿入（重複はスキップ）
        tag_rows = [(t,) for t in tags]
        psycopg2.extras.execute_batch(
            cur,
            """
            INSERT INTO tags (name)
            VALUES (%s)
            ON CONFLICT (name) DO NOTHING
            """,
            tag_rows,
            page_size=200
        )

        # 2) 登録済みタグの ID をまとめて取得
        cur.execute(
            "SELECT id, name FROM tags WHERE name = ANY(%s)",
            (tags,)
        )
        id_map = {name: tid for tid, name in cur.fetchall()}

        # 3) article_tags 中間テーブルに Upsert
        rel_rows = [(article_id, id_map[t]) for t in tags]
        psycopg2.extras.execute_batch(
            cur,
            """
            INSERT INTO article_tags (article_id, tag_id)
            VALUES (%s::uuid, %s)
            ON CONFLICT (article_id, tag_id) DO NOTHING
            """,
            rel_rows,
            page_size=200
        )

    # コミットはバッチ毎まとめて行う
    conn.commit()