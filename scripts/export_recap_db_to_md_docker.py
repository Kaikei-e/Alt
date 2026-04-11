#!/usr/bin/env python3
"""
recap-dbのデータをMarkdownファイルに書き出すスクリプト（Dockerコンテナ内で実行）
"""

import os
import sys
import json
from datetime import datetime
from pathlib import Path
from typing import Any, Dict, List, Optional

try:
    import psycopg2
    from psycopg2.extras import RealDictCursor
except ImportError:
    print("psycopg2が必要です。インストールしてください: pip install psycopg2-binary")
    sys.exit(1)


def get_db_connection() -> psycopg2.extensions.connection:
    """データベース接続を取得（環境変数から）"""
    host = os.environ.get("RECAP_DB_HOST", "recap-db")
    port = os.environ.get("RECAP_DB_PORT", "5432")
    user = os.environ.get("RECAP_DB_USER", os.environ.get("RECAP_DB_USER", "recap_user"))
    password = os.environ.get("RECAP_DB_PASSWORD", "")
    dbname = os.environ.get("RECAP_DB_NAME", "recap")

    try:
        conn = psycopg2.connect(
            host=host,
            port=port,
            user=user,
            password=password,
            dbname=dbname,
            connect_timeout=10
        )
        return conn
    except psycopg2.Error as e:
        print(f"データベース接続エラー: {e}", file=sys.stderr)
        sys.exit(1)


def format_value(value: Any) -> str:
    """値をMarkdown形式にフォーマット"""
    if value is None:
        return "*NULL*"
    if isinstance(value, datetime):
        return value.strftime("%Y-%m-%d %H:%M:%S")
    if isinstance(value, (dict, list)):
        json_str = json.dumps(value, ensure_ascii=False, indent=2)
        return f"```json\n{json_str}\n```"
    value_str = str(value)
    if len(value_str) > 500:
        return f"```\n{value_str[:500]}...\n```"
    return value_str


def export_table_to_md(
    cursor: RealDictCursor,
    table_name: str,
    md_file,
    order_by: Optional[str] = None,
    limit: Optional[int] = None
) -> int:
    """テーブルのデータをMarkdown形式でエクスポート"""
    query = f"SELECT * FROM {table_name}"
    if order_by:
        query += f" ORDER BY {order_by}"
    if limit:
        query += f" LIMIT {limit}"

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


def export_recap_db_to_md(output_file: str = "/tmp/recap_db_export.md"):
    """recap-dbの全データをMarkdownファイルにエクスポート"""
    conn = get_db_connection()
    cursor = conn.cursor(cursor_factory=RealDictCursor)

    output_path = Path(output_file)

    try:
        with open(output_path, "w", encoding="utf-8") as md_file:
            md_file.write("# Recap Database Export\n\n")
            md_file.write(f"**エクスポート日時**: {datetime.now().strftime('%Y-%m-%d %H:%M:%S')}\n\n")
            md_file.write("---\n\n")

            # テーブル一覧を取得
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

            # 各テーブルのデータをエクスポート
            total_records = 0

            # recap_jobs
            md_file.write("## 1. recap_jobs\n\n")
            total_records += export_table_to_md(cursor, "recap_jobs", md_file, "kicked_at DESC")

            # recap_job_articles
            md_file.write("## 2. recap_job_articles\n\n")
            total_records += export_table_to_md(cursor, "recap_job_articles", md_file, "id DESC", limit=100)

            # recap_preprocess_metrics
            md_file.write("## 3. recap_preprocess_metrics\n\n")
            total_records += export_table_to_md(cursor, "recap_preprocess_metrics", md_file)

            # recap_subworker_runs
            md_file.write("## 4. recap_subworker_runs\n\n")
            total_records += export_table_to_md(cursor, "recap_subworker_runs", md_file, "started_at DESC")

            # recap_subworker_clusters
            md_file.write("## 5. recap_subworker_clusters\n\n")
            total_records += export_table_to_md(cursor, "recap_subworker_clusters", md_file, "run_id, cluster_id")

            # recap_subworker_sentences
            md_file.write("## 6. recap_subworker_sentences\n\n")
            total_records += export_table_to_md(cursor, "recap_subworker_sentences", md_file, "cluster_row_id, id", limit=500)

            # recap_subworker_diagnostics
            md_file.write("## 7. recap_subworker_diagnostics\n\n")
            total_records += export_table_to_md(cursor, "recap_subworker_diagnostics", md_file)

            # recap_sections
            md_file.write("## 8. recap_sections\n\n")
            total_records += export_table_to_md(cursor, "recap_sections", md_file)

            # recap_final_sections (存在する場合)
            cursor.execute("""
                SELECT EXISTS (
                    SELECT FROM information_schema.tables
                    WHERE table_schema = 'public'
                    AND table_name = 'recap_final_sections'
                )
            """)
            if cursor.fetchone()["exists"]:
                md_file.write("## 9. recap_final_sections\n\n")
                total_records += export_table_to_md(cursor, "recap_final_sections", md_file, "created_at DESC")

            # recap_outputs (存在する場合)
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

            md_file.write("---\n\n")
            md_file.write(f"**合計レコード数**: {total_records}\n\n")
            md_file.write(f"**エクスポート完了**: {datetime.now().strftime('%Y-%m-%d %H:%M:%S')}\n")

        print(f"✓ エクスポート完了: {output_path}")
        print(f"  合計レコード数: {total_records}")

    except Exception as e:
        print(f"エラーが発生しました: {e}", file=sys.stderr)
        import traceback
        traceback.print_exc()
        sys.exit(1)
    finally:
        cursor.close()
        conn.close()


if __name__ == "__main__":
    output_file = sys.argv[1] if len(sys.argv) > 1 else "/tmp/recap_db_export.md"
    export_recap_db_to_md(output_file)

