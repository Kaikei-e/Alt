#!/usr/bin/env python3
"""
recap-dbのデータをMarkdownファイルに書き出すスクリプト（Dockerコンテナ内で実行）
"""

import json
import os
import sys
import traceback
from datetime import datetime
from pathlib import Path
from typing import TextIO

ORDER_BY_TOKEN_MAX = 2
VALUE_PREVIEW_MAX_LEN = 500

try:
    import psycopg2
    from psycopg2 import sql
    from psycopg2.extras import RealDictCursor
except ImportError:
    sys.stdout.write("psycopg2が必要です。インストールしてください: pip install psycopg2-binary\n")
    sys.exit(1)


ALLOWED_EXPORT_TABLES = frozenset(
    {
        "recap_jobs",
        "recap_job_articles",
        "recap_preprocess_metrics",
        "recap_subworker_runs",
        "recap_subworker_clusters",
        "recap_subworker_sentences",
        "recap_subworker_diagnostics",
        "recap_sections",
        "recap_final_sections",
        "recap_outputs",
    }
)


def _build_order_by(order_by: str) -> sql.Composable:
    """ORDER BY句をカラム名検証つきで安全に組み立てる"""
    parts = []
    for raw in order_by.split(","):
        tokens = raw.strip().split()
        if not tokens or len(tokens) > ORDER_BY_TOKEN_MAX:
            msg = f"Invalid ORDER BY clause: {order_by!r}"
            raise ValueError(msg)
        column = sql.Identifier(tokens[0])
        if len(tokens) == ORDER_BY_TOKEN_MAX:
            direction = tokens[1].upper()
            if direction not in ("ASC", "DESC"):
                msg = f"Invalid ORDER BY direction: {tokens[1]!r}"
                raise ValueError(msg)
            parts.append(sql.SQL("{} {}").format(column, sql.SQL(direction)))
        else:
            parts.append(sql.SQL("{}").format(column))
    return sql.SQL(", ").join(parts)


def get_db_connection() -> psycopg2.extensions.connection:
    """データベース接続を取得（環境変数から）"""
    host = os.environ.get("RECAP_DB_HOST", "recap-db")
    port = os.environ.get("RECAP_DB_PORT", "5432")
    user = os.environ.get("RECAP_DB_USER", "recap_user")
    password = os.environ.get("RECAP_DB_PASSWORD", "")
    dbname = os.environ.get("RECAP_DB_NAME", "recap")

    try:
        conn = psycopg2.connect(
            host=host, port=port, user=user, password=password, dbname=dbname, connect_timeout=10
        )
    except psycopg2.Error as e:
        sys.stderr.write(f"データベース接続エラー: {e}\n")
        sys.exit(1)
    else:
        return conn


def format_value(value: object) -> str:
    """値をMarkdown形式にフォーマット"""
    if value is None:
        return "*NULL*"
    if isinstance(value, datetime):
        return value.strftime("%Y-%m-%d %H:%M:%S")
    if isinstance(value, (dict, list)):
        json_str = json.dumps(value, ensure_ascii=False, indent=2)
        return f"```json\n{json_str}\n```"
    value_str = str(value)
    if len(value_str) > VALUE_PREVIEW_MAX_LEN:
        return f"```\n{value_str[:VALUE_PREVIEW_MAX_LEN]}...\n```"
    return value_str


def export_table_to_md(
    cursor: RealDictCursor,
    table_name: str,
    md_file: TextIO,
    order_by: str | None = None,
    limit: int | None = None,
) -> int:
    """テーブルのデータをMarkdown形式でエクスポート"""
    if table_name not in ALLOWED_EXPORT_TABLES:
        msg = f"Table not in export allowlist: {table_name!r}"
        raise ValueError(msg)

    query = sql.SQL("SELECT * FROM {}").format(sql.Identifier(table_name))
    if order_by:
        query = sql.SQL("{} ORDER BY {}").format(query, _build_order_by(order_by))
    if limit:
        query = sql.SQL("{} LIMIT {}").format(query, sql.Literal(int(limit)))

    cursor.execute(query)
    rows = cursor.fetchall()

    if not rows:
        md_file.write(f"\n### {table_name}\n\n")
        md_file.write("*データなし*\n\n")
        return 0

    md_file.write(f"\n### {table_name}\n\n")
    md_file.write(f"**レコード数**: {len(rows)}\n\n")

    for idx, row in enumerate(rows, 1):
        md_file.write(f"#### レコード #{idx}\n\n")
        for key, value in row.items():
            md_file.write(f"- **{key}**: {format_value(value)}\n")
        md_file.write("\n")

    return len(rows)


def _write_table_list(cursor: RealDictCursor, md_file: TextIO) -> None:
    """公開テーブル一覧をMarkdownへ書き出す"""
    cursor.execute("""
        SELECT table_name
        FROM information_schema.tables
        WHERE table_schema = 'public'
        AND table_type = 'BASE TABLE'
        ORDER BY table_name
    """)
    tables = [row["table_name"] for row in cursor.fetchall()]

    md_file.write("## テーブル一覧\n\n")
    for table in tables:
        md_file.write(f"- `{table}`\n")
    md_file.write("\n---\n\n")


def _export_core_tables(cursor: RealDictCursor, md_file: TextIO) -> int:
    """常設8テーブルをMarkdownへ書き出し、合計レコード数を返す"""
    core_tables: tuple[tuple[int, str, str | None, int | None], ...] = (
        (1, "recap_jobs", "kicked_at DESC", None),
        (2, "recap_job_articles", "id DESC", 100),
        (3, "recap_preprocess_metrics", None, None),
        (4, "recap_subworker_runs", "started_at DESC", None),
        (5, "recap_subworker_clusters", "run_id, cluster_id", None),
        (6, "recap_subworker_sentences", "cluster_row_id, id", 500),
        (7, "recap_subworker_diagnostics", None, None),
        (8, "recap_sections", None, None),
    )
    total_records = 0
    for section_num, table_name, order_by, limit in core_tables:
        md_file.write(f"## {section_num}. {table_name}\n\n")
        total_records += export_table_to_md(cursor, table_name, md_file, order_by, limit=limit)
    return total_records


def _export_optional_tables(cursor: RealDictCursor, md_file: TextIO) -> int:
    """存在する場合のみ追加テーブルをMarkdownへ書き出し、合計レコード数を返す"""
    total_records = 0

    cursor.execute("""
        SELECT EXISTS (
            SELECT FROM information_schema.tables
            WHERE table_schema = 'public'
            AND table_name = 'recap_final_sections'
        )
    """)
    if cursor.fetchone()["exists"]:
        md_file.write("## 9. recap_final_sections\n\n")
        total_records += export_table_to_md(
            cursor, "recap_final_sections", md_file, "created_at DESC"
        )

    cursor.execute("""
        SELECT EXISTS (
            SELECT FROM information_schema.tables
            WHERE table_schema = 'public'
            AND table_name = 'recap_outputs'
        )
    """)
    if cursor.fetchone()["exists"]:
        md_file.write("## 10. recap_outputs\n\n")
        total_records += export_table_to_md(cursor, "recap_outputs", md_file, "created_at DESC")

    return total_records


def export_recap_db_to_md(output_file: str = "/tmp/recap_db_export.md") -> None:  # noqa: S108 — default container path read by scripts/export_recap_db_to_md.sh via docker compose cp
    """recap-dbの全データをMarkdownファイルにエクスポート"""
    conn = get_db_connection()
    cursor = conn.cursor(cursor_factory=RealDictCursor)

    output_path = Path(output_file)

    try:
        with output_path.open("w", encoding="utf-8") as md_file:
            md_file.write("# Recap Database Export\n\n")
            md_file.write(
                f"**エクスポート日時**: {datetime.now().astimezone().strftime('%Y-%m-%d %H:%M:%S')}\n\n"
            )
            md_file.write("---\n\n")

            _write_table_list(cursor, md_file)

            total_records = _export_core_tables(cursor, md_file)
            total_records += _export_optional_tables(cursor, md_file)

            md_file.write("---\n\n")
            md_file.write(f"**合計レコード数**: {total_records}\n\n")
            md_file.write(
                f"**エクスポート完了**: {datetime.now().astimezone().strftime('%Y-%m-%d %H:%M:%S')}\n"
            )

        sys.stdout.write(f"✓ エクスポート完了: {output_path}\n")
        sys.stdout.write(f"  合計レコード数: {total_records}\n")

    except (psycopg2.Error, OSError) as e:
        sys.stderr.write(f"エラーが発生しました: {e}\n")
        traceback.print_exc()
        sys.exit(1)
    finally:
        cursor.close()
        conn.close()


if __name__ == "__main__":
    output_file = sys.argv[1] if len(sys.argv) > 1 else "/tmp/recap_db_export.md"  # noqa: S108 — default container path read by scripts/export_recap_db_to_md.sh via docker compose cp
    export_recap_db_to_md(output_file)
