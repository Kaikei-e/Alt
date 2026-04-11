#!/usr/bin/env python3
"""
eval.mdを検証結果で更新するスクリプト
"""

import json
from datetime import datetime
from pathlib import Path
from typing import Any, Dict, List, Optional


def load_verification_results() -> Optional[Dict[str, Any]]:
    """検証結果JSONを読み込む"""
    script_dir = Path(__file__).parent
    results_file = script_dir / 'reports' / 'recap_verification_results.json'

    if not results_file.exists():
        print(f"検証結果ファイルが見つかりません: {results_file}")
        print("検証スクリプトを実行します...")
        # 検証スクリプトを直接実行
        import subprocess
        import sys
        try:
            result = subprocess.run(
                [sys.executable, str(script_dir / 'compute_recap_coverage.py'), '--verify'],
                cwd=script_dir.parent,
                capture_output=True,
                text=True,
                timeout=300
            )
            print(result.stdout)
            if result.stderr:
                print("エラー:", result.stderr)

            # 再度ファイルを確認
            if results_file.exists():
                with open(results_file, 'r', encoding='utf-8') as f:
                    return json.load(f)
            else:
                print("検証スクリプトの実行後も結果ファイルが生成されませんでした")
                return None
        except Exception as e:
            print(f"検証スクリプトの実行に失敗しました: {e}")
            return None

    with open(results_file, 'r', encoding='utf-8') as f:
        return json.load(f)


def format_job_id(job_id: str) -> str:
    """Job IDを短縮形式で表示"""
    return job_id[:8] + '...' if len(job_id) > 8 else job_id


def format_datetime(dt: Any) -> str:
    """日時をフォーマット"""
    if isinstance(dt, str):
        return dt
    if hasattr(dt, 'isoformat'):
        return dt.isoformat()
    return str(dt)


def generate_eval_report(results: Dict[str, Any], template_path: Path, output_path: Path) -> None:
    """eval.mdを生成"""

    # レポートで言及されているJob ID
    report_job_ids = {
        '5cc12453-0c22-40f2-b100-0ad1945a73c9': 'Latest Job',
        '4e5be544-9abe-40d6-9bb9-42a5bd3e0774': 'Previous Baseline'
    }

    # テンプレートを読み込む
    with open(template_path, 'r', encoding='utf-8') as f:
        content = f.read()

    # 作成日時を更新
    content = content.replace(
        '**作成日時:** [実行日時を記載]',
        f'**作成日時:** {datetime.now().strftime("%Y-%m-%d %H:%M:%S")}'
    )

    # 1.1 Job IDの存在確認
    job_existence_table = []
    for job_id, label in report_job_ids.items():
        if job_id in results:
            job_data = results[job_id]
            kicked_at = job_data.get('metrics', {}).get('kicked_at', 'N/A')
            status = '✓'
        else:
            kicked_at = 'N/A'
            status = '✗'
        job_existence_table.append(
            f'| `{job_id[:8]}...` | {status} | {format_datetime(kicked_at)} | {label} |'
        )

    content = content.replace(
        '| `5cc12453-0c22-40f2-b100-0ad1945a73c9` | [✓/✗] | [日時] | Latest Job |',
        job_existence_table[0] if len(job_existence_table) > 0 else '| `5cc12453...` | ✗ | N/A | Latest Job |'
    )
    content = content.replace(
        '| `4e5be544-9abe-40d6-9bb9-42a5bd3e0774` | [✓/✗] | [日時] | Previous Baseline |',
        job_existence_table[1] if len(job_existence_table) > 1 else '| `4e5be544...` | ✗ | N/A | Previous Baseline |'
    )

    # 1.2 最新の完了済みJob
    latest_jobs = []
    for job_id, job_data in list(results.items())[:3]:
        metrics = job_data.get('metrics', {})
        status_summary = job_data.get('status_summary', {})
        kicked_at = metrics.get('kicked_at', 'N/A')
        succeeded = status_summary.get('status_counts', {}).get('succeeded', {}).get('genre_count', 0)
        partial = status_summary.get('status_counts', {}).get('partial', {}).get('genre_count', 0)
        total_success = succeeded + partial
        latest_jobs.append(
            f'| `{job_id[:8]}...` | {format_datetime(kicked_at)} | {total_success} | succeeded/partial |'
        )

    if latest_jobs:
        content = content.replace(
            '| [最新Job] | [日時] | [数] | [succeeded/partial] |',
            latest_jobs[0]
        )

    # 2.1 Latest Jobの再計算結果
    latest_job_id = '5cc12453-0c22-40f2-b100-0ad1945a73c9'
    if latest_job_id in results:
        job_data = results[latest_job_id]
        metrics = job_data.get('metrics', {})
        report_values = {
            'avg': 0.8175,
            'std': 0.0330,
            'min': 0.7519,
            'max': 0.8588,
            'count': 14
        }
        actual_values = {
            'avg': metrics.get('avg_coverage', 0.0),
            'std': metrics.get('std_coverage', 0.0),
            'min': metrics.get('min_coverage', 0.0),
            'max': metrics.get('max_coverage', 0.0),
            'count': metrics.get('total_genres', 0)
        }

        # 統計テーブルを更新
        stats_table = []
        for key, label in [('avg', '平均カバレッジ'), ('std', '標準偏差'), ('min', '最小値'), ('max', '最大値'), ('count', 'サンプル数（ジャンル数）')]:
            report_val = report_values[key]
            actual_val = actual_values[key]
            diff = actual_val - report_val
            match = '一致' if abs(diff) < 0.0001 else '不一致'
            stats_table.append(
                f'| {label} | {report_val:.4f} | {actual_val:.4f} | {diff:+.4f} | {match} |'
            )

        content = content.replace(
            '| 平均カバレッジ | 0.8175 | [計算値] | [差異] | [一致/不一致] |',
            stats_table[0]
        )
        content = content.replace(
            '| 標準偏差 | 0.0330 | [計算値] | [差異] | [一致/不一致] |',
            stats_table[1]
        )
        content = content.replace(
            '| 最小値 | 0.7519 | [計算値] | [差異] | [一致/不一致] |',
            stats_table[2]
        )
        content = content.replace(
            '| 最大値 | 0.8588 | [計算値] | [差異] | [一致/不一致] |',
            stats_table[3]
        )
        content = content.replace(
            '| サンプル数（ジャンル数） | 14 | [実際の数] | [差異] | [一致/不一致] |',
            stats_table[4]
        )

        # ジャンル別カバレッジ詳細
        genre_results = metrics.get('genre_results', [])
        report_genres = {
            'ai_data': 0.8409,
            'consumer_tech': 0.8555,
            'other': 0.8588,
            'other.0': 0.7649,
            'other.2': 0.8437,
            'other.6': 0.8372
        }

        genre_table = []
        for genre_name, report_coverage in report_genres.items():
            genre_data = next((g for g in genre_results if g['genre'] == genre_name), None)
            if genre_data:
                actual_coverage = genre_data['coverage']
                diff = actual_coverage - report_coverage
                bullets = genre_data.get('bullets', 0)
                centroids = genre_data.get('centroids', 0)
                genre_table.append(
                    f'| `{genre_name}` | {report_coverage:.4f} | {actual_coverage:.4f} | {diff:+.4f} | {bullets} | {centroids} |'
                )
            else:
                genre_table.append(
                    f'| `{genre_name}` | {report_coverage:.4f} | データなし | - | - | - |'
                )

        # 既存のジャンルテーブル行を置換
        for i, genre_row in enumerate(genre_table[:6]):
            old_pattern = f'| `{list(report_genres.keys())[i]}` | {list(report_genres.values())[i]:.4f} | [計算値] | [差異] | [数] | [数] |'
            content = content.replace(old_pattern, genre_row)

    # 3.1 Jobステータス分布
    if latest_job_id in results:
        job_data = results[latest_job_id]
        status_summary = job_data.get('status_summary', {})
        status_counts = status_summary.get('status_counts', {})
        total_genres = status_summary.get('total_genres', 0)
        output_count = status_summary.get('output_count', 0)

        succeeded = status_counts.get('succeeded', {}).get('genre_count', 0)
        partial = status_counts.get('partial', {}).get('genre_count', 0)
        failed = status_counts.get('failed', {}).get('genre_count', 0)
        running = status_counts.get('running', {}).get('genre_count', 0)

        content = content.replace(
            '**Latest Job (`5cc12453`):**\n- succeeded: [数] ジャンル\n- partial: [数] ジャンル\n- failed: [数] ジャンル\n- running: [数] ジャンル\n- 総ジャンル数: [数]\n- 出力ジャンル数 (`recap_outputs`): [数]',
            f'**Latest Job (`5cc12453`):**\n- succeeded: {succeeded} ジャンル\n- partial: {partial} ジャンル\n- failed: {failed} ジャンル\n- running: {running} ジャンル\n- 総ジャンル数: {total_genres}\n- 出力ジャンル数 (`recap_outputs`): {output_count}'
        )

    # 3.2 前処理メトリクス
    if latest_job_id in results:
        job_data = results[latest_job_id]
        preprocess = job_data.get('preprocess')
        if preprocess:
            content = content.replace(
                '**Latest Job (`5cc12453`):**\n- 取得記事数: [数]\n- 処理済み記事数: [数]\n- 空記事除外数: [数]\n- HTMLクリーニング済み: [数]\n- 総文字数: [数]\n- 平均文字数/記事: [数]',
                f'**Latest Job (`5cc12453`):**\n- 取得記事数: {preprocess.get("total_articles_fetched", 0)}\n- 処理済み記事数: {preprocess.get("articles_processed", 0)}\n- 空記事除外数: {preprocess.get("articles_dropped_empty", 0)}\n- HTMLクリーニング済み: {preprocess.get("articles_html_cleaned", 0)}\n- 総文字数: {preprocess.get("total_characters", 0)}\n- 平均文字数/記事: {preprocess.get("avg_chars_per_article", 0):.1f}'
            )

    # ファイルに書き込む
    with open(output_path, 'w', encoding='utf-8') as f:
        f.write(content)

    print(f"eval.mdを更新しました: {output_path}")


def main():
    repo_root = Path(__file__).parent.parent
    template_path = repo_root / 'eval.md'
    output_path = repo_root / 'eval.md'
    results = load_verification_results()

    if results is None:
        return

    generate_eval_report(results, template_path, output_path)


if __name__ == '__main__':
    main()
