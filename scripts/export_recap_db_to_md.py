#!/usr/bin/env python3
"""
recap-dbのデータをMarkdownファイルに書き出すスクリプト
"""

from __future__ import annotations

import os
import sys
import json
from datetime import datetime, timezone
from pathlib import Path
from typing import Any, Dict, List, Optional, Tuple

try:
    import psycopg2
    from psycopg2.extras import RealDictCursor
except ImportError:
    print("psycopg2が必要です。インストールしてください: pip install psycopg2-binary")
    sys.exit(1)


def load_env_vars() -> Dict[str, str]:
    """環境変数または.envファイルからデータベース接続情報を読み込む"""
    env_file = Path(__file__).parent.parent / ".env"
    env_vars = {}

    if env_file.exists():
        with open(env_file, "r", encoding="utf-8") as f:
            for line in f:
                line = line.strip()
                if line and not line.startswith("#") and "=" in line:
                    key, value = line.split("=", 1)
                    env_vars[key.strip()] = value.strip().strip('"').strip("'")

    # 環境変数で上書き
    for key in ["RECAP_DB_HOST", "RECAP_DB_PORT", "RECAP_DB_USER", "RECAP_DB_PASSWORD", "RECAP_DB_NAME"]:
        if key in os.environ:
            env_vars[key] = os.environ[key]

    return env_vars


def get_db_connection() -> psycopg2.extensions.connection:
    """データベース接続を取得"""
    env_vars = load_env_vars()

    host = env_vars.get("RECAP_DB_HOST", "localhost")
    port = env_vars.get("RECAP_DB_PORT", "5435")
    user = env_vars.get("RECAP_DB_USER", "recap_user")
    password = env_vars.get("RECAP_DB_PASSWORD", "")
    dbname = env_vars.get("RECAP_DB_NAME", "recap")

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
        print(f"データベース接続エラー: {e}")
        sys.exit(1)


def format_value(value: Any) -> str:
    """値をMarkdown形式にフォーマット"""
    if value is None:
        return "*NULL*"
    if isinstance(value, datetime):
        return value.strftime("%Y-%m-%d %H:%M:%S")
    if isinstance(value, dict):
        return f"```json\n{json.dumps(value, ensure_ascii=False, indent=2)}\n```"
    if isinstance(value, list):
        return f"```json\n{json.dumps(value, ensure_ascii=False, indent=2)}\n```"
    if isinstance(value, str) and len(value) > 200:
        return f"```\n{value[:200]}...\n```"
    return str(value)


def export_table_to_md(
    cursor: RealDictCursor,
    table_name: str,
    md_file,
    order_by: Optional[str] = None
) -> int:
    """テーブルのデータをMarkdown形式でエクスポート"""
    query = f"SELECT * FROM {table_name}"
    if order_by:
        query += f" ORDER BY {order_by}"

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


def score_summary_quality(
    *,
    status: str,
    cluster_count: int,
    diagnostics: Dict[str, Any],
    summary_text: Optional[str],
    bullets: List[Dict[str, Any]],
) -> float:
    """ヒューリスティックにRecap品質スコアを算出する。"""

    components: List[float] = []
    noise_ratio = diagnostics.get("noise_ratio")
    if isinstance(noise_ratio, (int, float)):
        components.append(max(0.0, min(1.0, 1.0 - float(noise_ratio))))

    components.append(min(1.0, cluster_count / 6.0))

    total_sentences = diagnostics.get("total_sentences")
    if isinstance(total_sentences, (int, float)) and total_sentences > 0:
        components.append(min(1.0, float(total_sentences) / 800.0))

    bullet_count = len(bullets)
    if bullet_count == 0:
        components.append(0.0)
    else:
        components.append(min(1.0, bullet_count / 4.0))

    if summary_text:
        stripped = summary_text.strip()
        length = len(stripped)
        if length == 0:
            components.append(0.0)
        elif length < 220:
            components.append(min(1.0, length / 220.0))
        elif length > 1200:
            components.append(max(0.0, 1.0 - (length - 1200) / 1200.0))
        else:
            components.append(1.0)

        # JSONゴミが混入している場合は減点
        if stripped.startswith("{") or stripped.count("\"") > length * 0.15:
            components.append(0.0)
    else:
        components.append(0.0)

    if not components:
        base_score = 0.0
    else:
        base_score = sum(components) / len(components)

    if status != "succeeded":
        base_score *= 0.4

    return max(0.0, min(1.0, base_score))


def build_reference_summary(clusters: List[Dict[str, Any]], max_sentences: int = 6) -> str:
    """代表文から参照要約を構築する。"""

    references: List[str] = []
    for cluster in clusters:
        reps = cluster.get("representatives") or []
        for rep in reps:
            text = rep.get("text")
            if text and text not in references:
                references.append(text.strip())
            if len(references) >= max_sentences:
                break
        if len(references) >= max_sentences:
            break
    return "\n".join(references)


def normalize_bullets(raw: Optional[str | List[Dict[str, Any]]]) -> List[Dict[str, Any]]:
    if raw is None:
        return []
    if isinstance(raw, list):
        return [
            bullet for bullet in raw if isinstance(bullet, dict) and bullet.get("text")
        ]
    try:
        parsed = json.loads(raw)
    except (TypeError, json.JSONDecodeError):
        return []
    if isinstance(parsed, list):
        return [
            bullet for bullet in parsed if isinstance(bullet, dict) and bullet.get("text")
        ]
    return []


def collect_golden_runs(cursor: RealDictCursor, limit_per_bucket: int = 20) -> Dict[str, Any]:
    """ゴールデンセットとなる良例・悪例を抽出する。"""

    cursor.execute(
        """
        SELECT
            r.id,
            r.job_id,
            r.genre,
            r.status,
            r.cluster_count,
            r.started_at,
            r.finished_at,
            r.response_payload,
            r.request_payload,
            o.title_ja,
            o.summary_ja,
            o.bullets_ja
        FROM recap_subworker_runs r
        LEFT JOIN recap_outputs o
          ON o.job_id = r.job_id AND o.genre = r.genre
        WHERE r.response_payload IS NOT NULL
        ORDER BY r.finished_at DESC NULLS LAST, r.id DESC
        """
    )
    rows = cursor.fetchall()

    candidates: List[Tuple[float, Dict[str, Any]]] = []
    for row in rows:
        response = row["response_payload"] or {}
        diagnostics = response.get("diagnostics") or {}
        clusters = response.get("clusters") or []

        summary_text = row.get("summary_ja")
        if not summary_text:
            summary = response.get("summary") or {}
            summary_text = summary.get("text") or summary.get("summary_ja")

        bullets = normalize_bullets(row.get("bullets_ja"))
        quality = score_summary_quality(
            status=row.get("status", ""),
            cluster_count=row.get("cluster_count") or 0,
            diagnostics=diagnostics,
            summary_text=summary_text,
            bullets=bullets,
        )

        reference_summary = build_reference_summary(clusters)
        entry = {
            "id": row.get("id"),
            "job_id": row.get("job_id"),
            "genre": row.get("genre"),
            "status": row.get("status"),
            "cluster_count": row.get("cluster_count"),
            "summary_text": summary_text,
            "title_ja": row.get("title_ja"),
            "bullets": bullets,
            "diagnostics": diagnostics,
            "quality_score": round(quality, 4),
            "reference_summary": reference_summary,
            "representative_count": len(reference_summary.splitlines()) if reference_summary else 0,
            "started_at": row.get("started_at"),
            "finished_at": row.get("finished_at"),
        }
        candidates.append((quality, entry))

    if not candidates:
        return {"good": [], "bad": []}

    sorted_by_quality = sorted(candidates, key=lambda item: item[0])
    bad_examples = [
        item[1]
        for item in sorted_by_quality[:limit_per_bucket]
        if item[0] <= 0.45
    ]
    good_examples = [
        item[1]
        for item in reversed(sorted_by_quality[-limit_per_bucket:])
        if item[0] >= 0.7
    ]

    return {
        "good": good_examples,
        "bad": bad_examples,
        "total_candidates": len(candidates),
    }


def export_recap_db_to_md(output_file: str = "recap_db_export.md"):
    """recap-dbの全データをMarkdownファイルにエクスポート"""
    conn = get_db_connection()
    cursor = conn.cursor(cursor_factory=RealDictCursor)

    output_path = Path(__file__).parent.parent / output_file

    resources_dir = Path(__file__).parent.parent / "recap-worker" / "resources"
    resources_dir.mkdir(parents=True, exist_ok=True)
    golden_output_path = resources_dir / "golden_runs.json"

    total_records = 0

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
            # recap_jobs
            md_file.write("## 1. recap_jobs\n\n")
            total_records += export_table_to_md(cursor, "recap_jobs", md_file, "kicked_at DESC")

            # recap_job_articles
            md_file.write("## 2. recap_job_articles\n\n")
            total_records += export_table_to_md(cursor, "recap_job_articles", md_file, "id DESC")

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
            total_records += export_table_to_md(cursor, "recap_subworker_sentences", md_file, "cluster_row_id, id")

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

        golden_payload = collect_golden_runs(cursor)
        golden_payload["generated_at"] = datetime.now(timezone.utc).isoformat()
        golden_payload["criteria"] = {
            "good_threshold": 0.7,
            "bad_threshold": 0.45,
            "quality_components": [
                "1 - noise_ratio",
                "cluster_count/6",
                "total_sentences/800",
                "bullet_count/4",
                "summary_length",
            ],
        }

        with open(golden_output_path, "w", encoding="utf-8") as golden_file:
            json.dump(golden_payload, golden_file, ensure_ascii=False, indent=2, default=str)

        print(f"✓ エクスポート完了: {output_path}")
        print(f"  合計レコード数: {total_records}")
        print(f"✓ ゴールデンセット出力: {golden_output_path}")
        print(f"  良例: {len(golden_payload.get('good', []))} / 悪例: {len(golden_payload.get('bad', []))}")

    except Exception as e:
        print(f"エラーが発生しました: {e}")
        sys.exit(1)
    finally:
        cursor.close()
        conn.close()


if __name__ == "__main__":
    output_file = sys.argv[1] if len(sys.argv) > 1 else "recap_db_export.md"
    export_recap_db_to_md(output_file)

