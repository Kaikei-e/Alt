#!/usr/bin/env python3
"""
eval.mdを検証結果で更新するスクリプト
"""

import argparse
import json
import subprocess
import sys
from datetime import datetime
from pathlib import Path
from typing import Any

JOB_ID_SHORT_LEN = 8
METRIC_MATCH_EPSILON = 0.0001

DEFAULT_EVAL_CONFIG: dict[str, Any] = {
    "report_job_ids": {
        "5cc12453-0c22-40f2-b100-0ad1945a73c9": "Latest Job",
        "4e5be544-9abe-40d6-9bb9-42a5bd3e0774": "Previous Baseline",
    },
    "latest_job_id": "5cc12453-0c22-40f2-b100-0ad1945a73c9",
    "report_values": {
        "avg": 0.8175,
        "std": 0.0330,
        "min": 0.7519,
        "max": 0.8588,
        "count": 14,
    },
    "report_genres": {
        "ai_data": 0.8409,
        "consumer_tech": 0.8555,
        "other": 0.8588,
        "other.0": 0.7649,
        "other.2": 0.8437,
        "other.6": 0.8372,
    },
}


def load_verification_results() -> dict[str, Any] | None:
    """検証結果JSONを読み込む"""
    script_dir = Path(__file__).parent
    results_file = script_dir / "reports" / "recap_verification_results.json"

    if not results_file.exists():
        sys.stdout.write(f"検証結果ファイルが見つかりません: {results_file}\n")
        sys.stdout.write("検証スクリプトを実行します...\n")
        # 検証スクリプトを直接実行
        try:
            result = subprocess.run(  # noqa: S603 -- fixed script path invoked with sys.executable
                [sys.executable, str(script_dir / "compute_recap_coverage.py"), "--verify"],
                cwd=script_dir.parent,
                capture_output=True,
                text=True,
                timeout=300,
                check=False,
            )
            sys.stdout.write(f"{result.stdout}\n")
            if result.stderr:
                sys.stdout.write(f"エラー: {result.stderr}\n")

            # 再度ファイルを確認
            if results_file.exists():
                with results_file.open(encoding="utf-8") as f:
                    return json.load(f)
            else:
                sys.stdout.write("検証スクリプトの実行後も結果ファイルが生成されませんでした\n")
                return None
        except (subprocess.TimeoutExpired, OSError) as e:
            sys.stderr.write(f"検証スクリプトの実行に失敗しました: {e}\n")
            return None

    with results_file.open(encoding="utf-8") as f:
        return json.load(f)


def format_job_id(job_id: str) -> str:
    """Job IDを短縮形式で表示"""
    return job_id[:JOB_ID_SHORT_LEN] + "..." if len(job_id) > JOB_ID_SHORT_LEN else job_id


def format_datetime(dt: object) -> str:
    """日時をフォーマット"""
    if isinstance(dt, str):
        return dt
    if hasattr(dt, "isoformat"):
        return dt.isoformat()
    return str(dt)


def _apply_job_existence_section(
    content: str, results: dict[str, Any], report_job_ids: dict[str, str]
) -> str:
    """セクション1.1（Job ID存在確認テーブル）を置換"""
    job_existence_table = []
    for job_id, label in report_job_ids.items():
        if job_id in results:
            job_data = results[job_id]
            kicked_at = job_data.get("metrics", {}).get("kicked_at", "N/A")
            status = "✓"
        else:
            kicked_at = "N/A"
            status = "✗"
        job_existence_table.append(
            f"| `{job_id[:8]}...` | {status} | {format_datetime(kicked_at)} | {label} |"
        )

    content = content.replace(
        "| `5cc12453-0c22-40f2-b100-0ad1945a73c9` | [✓/✗] | [日時] | Latest Job |",
        job_existence_table[0]
        if len(job_existence_table) > 0
        else "| `5cc12453...` | ✗ | N/A | Latest Job |",
    )
    return content.replace(
        "| `4e5be544-9abe-40d6-9bb9-42a5bd3e0774` | [✓/✗] | [日時] | Previous Baseline |",
        job_existence_table[1]
        if len(job_existence_table) > 1
        else "| `4e5be544...` | ✗ | N/A | Previous Baseline |",
    )


def _apply_latest_jobs_section(content: str, results: dict[str, Any]) -> str:
    """セクション1.2（最新の完了済みJobテーブル）を置換"""
    latest_jobs = []
    for job_id, job_data in list(results.items())[:3]:
        metrics = job_data.get("metrics", {})
        status_summary = job_data.get("status_summary", {})
        kicked_at = metrics.get("kicked_at", "N/A")
        succeeded = (
            status_summary.get("status_counts", {}).get("succeeded", {}).get("genre_count", 0)
        )
        partial = status_summary.get("status_counts", {}).get("partial", {}).get("genre_count", 0)
        total_success = succeeded + partial
        latest_jobs.append(
            f"| `{job_id[:8]}...` | {format_datetime(kicked_at)} | {total_success} | succeeded/partial |"
        )

    if latest_jobs:
        content = content.replace(
            "| [最新Job] | [日時] | [数] | [succeeded/partial] |", latest_jobs[0]
        )
    return content


def _apply_stats_table(content: str, metrics: dict[str, Any], report_values: dict[str, Any]) -> str:
    """セクション2.1の統計テーブル行を置換"""
    actual_values = {
        "avg": metrics.get("avg_coverage", 0.0),
        "std": metrics.get("std_coverage", 0.0),
        "min": metrics.get("min_coverage", 0.0),
        "max": metrics.get("max_coverage", 0.0),
        "count": metrics.get("total_genres", 0),
    }

    stats_table = []
    for key, label in [
        ("avg", "平均カバレッジ"),
        ("std", "標準偏差"),
        ("min", "最小値"),
        ("max", "最大値"),
        ("count", "サンプル数（ジャンル数）"),
    ]:
        report_val = report_values[key]
        actual_val = actual_values[key]
        diff = actual_val - report_val
        match = "一致" if abs(diff) < METRIC_MATCH_EPSILON else "不一致"
        stats_table.append(
            f"| {label} | {report_val:.4f} | {actual_val:.4f} | {diff:+.4f} | {match} |"
        )

    content = content.replace(
        "| 平均カバレッジ | 0.8175 | [計算値] | [差異] | [一致/不一致] |", stats_table[0]
    )
    content = content.replace(
        "| 標準偏差 | 0.0330 | [計算値] | [差異] | [一致/不一致] |", stats_table[1]
    )
    content = content.replace(
        "| 最小値 | 0.7519 | [計算値] | [差異] | [一致/不一致] |", stats_table[2]
    )
    content = content.replace(
        "| 最大値 | 0.8588 | [計算値] | [差異] | [一致/不一致] |", stats_table[3]
    )
    return content.replace(
        "| サンプル数（ジャンル数） | 14 | [実際の数] | [差異] | [一致/不一致] |",
        stats_table[4],
    )


def _apply_genre_table(
    content: str, metrics: dict[str, Any], report_genres: dict[str, float]
) -> str:
    """セクション2.1のジャンル別カバレッジ詳細テーブル行を置換"""
    genre_results = metrics.get("genre_results", [])
    genre_table = []
    for genre_name, report_coverage in report_genres.items():
        genre_data = next((g for g in genre_results if g["genre"] == genre_name), None)
        if genre_data:
            actual_coverage = genre_data["coverage"]
            diff = actual_coverage - report_coverage
            bullets = genre_data.get("bullets", 0)
            centroids = genre_data.get("centroids", 0)
            genre_table.append(
                f"| `{genre_name}` | {report_coverage:.4f} | {actual_coverage:.4f} | {diff:+.4f} | {bullets} | {centroids} |"
            )
        else:
            genre_table.append(
                f"| `{genre_name}` | {report_coverage:.4f} | データなし | - | - | - |"
            )

    for i, genre_row in enumerate(genre_table[:6]):
        old_pattern = f"| `{list(report_genres.keys())[i]}` | {list(report_genres.values())[i]:.4f} | [計算値] | [差異] | [数] | [数] |"
        content = content.replace(old_pattern, genre_row)
    return content


def _apply_status_and_preprocess_sections(content: str, job_data: dict[str, Any]) -> str:
    """セクション3.1（ステータス分布）と3.2（前処理メトリクス）を置換"""
    status_summary = job_data.get("status_summary", {})
    status_counts = status_summary.get("status_counts", {})
    total_genres = status_summary.get("total_genres", 0)
    output_count = status_summary.get("output_count", 0)

    succeeded = status_counts.get("succeeded", {}).get("genre_count", 0)
    partial = status_counts.get("partial", {}).get("genre_count", 0)
    failed = status_counts.get("failed", {}).get("genre_count", 0)
    running = status_counts.get("running", {}).get("genre_count", 0)

    content = content.replace(
        "**Latest Job (`5cc12453`):**\n- succeeded: [数] ジャンル\n- partial: [数] ジャンル\n- failed: [数] ジャンル\n- running: [数] ジャンル\n- 総ジャンル数: [数]\n- 出力ジャンル数 (`recap_outputs`): [数]",
        f"**Latest Job (`5cc12453`):**\n- succeeded: {succeeded} ジャンル\n- partial: {partial} ジャンル\n- failed: {failed} ジャンル\n- running: {running} ジャンル\n- 総ジャンル数: {total_genres}\n- 出力ジャンル数 (`recap_outputs`): {output_count}",
    )

    preprocess = job_data.get("preprocess")
    if preprocess:
        content = content.replace(
            "**Latest Job (`5cc12453`):**\n- 取得記事数: [数]\n- 処理済み記事数: [数]\n- 空記事除外数: [数]\n- HTMLクリーニング済み: [数]\n- 総文字数: [数]\n- 平均文字数/記事: [数]",
            f"**Latest Job (`5cc12453`):**\n- 取得記事数: {preprocess.get('total_articles_fetched', 0)}\n- 処理済み記事数: {preprocess.get('articles_processed', 0)}\n- 空記事除外数: {preprocess.get('articles_dropped_empty', 0)}\n- HTMLクリーニング済み: {preprocess.get('articles_html_cleaned', 0)}\n- 総文字数: {preprocess.get('total_characters', 0)}\n- 平均文字数/記事: {preprocess.get('avg_chars_per_article', 0):.1f}",
        )
    return content


def generate_eval_report(
    results: dict[str, Any],
    template_path: Path,
    output_path: Path,
    config: dict[str, Any] | None = None,
) -> None:
    """eval.mdを生成"""
    cfg = {**DEFAULT_EVAL_CONFIG, **(config or {})}
    report_job_ids = cfg["report_job_ids"]

    with template_path.open(encoding="utf-8") as f:
        content = f.read()

    content = content.replace(
        "**作成日時:** [実行日時を記載]",
        f"**作成日時:** {datetime.now().astimezone().strftime('%Y-%m-%d %H:%M:%S')}",
    )

    content = _apply_job_existence_section(content, results, report_job_ids)
    content = _apply_latest_jobs_section(content, results)

    latest_job_id = cfg["latest_job_id"]
    if latest_job_id in results:
        job_data = results[latest_job_id]
        metrics = job_data.get("metrics", {})
        content = _apply_stats_table(content, metrics, cfg["report_values"])
        content = _apply_genre_table(content, metrics, cfg["report_genres"])
        content = _apply_status_and_preprocess_sections(content, job_data)

    with output_path.open("w", encoding="utf-8") as f:
        f.write(content)

    sys.stdout.write(f"eval.mdを更新しました: {output_path}\n")


def main() -> None:
    parser = argparse.ArgumentParser(description="Update eval.md from verification results")
    parser.add_argument(
        "--config",
        type=Path,
        default=None,
        help="JSON config overriding job UUIDs and expected report stats",
    )
    args = parser.parse_args()
    config = dict(DEFAULT_EVAL_CONFIG)
    if args.config:
        with args.config.open(encoding="utf-8") as f:
            override = json.load(f)
        if not isinstance(override, dict):
            msg = f"config must be a JSON object: {args.config}"
            raise SystemExit(msg)
        config.update(override)

    repo_root = Path(__file__).parent.parent
    template_path = repo_root / "eval.md"
    output_path = repo_root / "eval.md"
    results = load_verification_results()

    if results is None:
        return

    generate_eval_report(results, template_path, output_path, config=config)


if __name__ == "__main__":
    main()
