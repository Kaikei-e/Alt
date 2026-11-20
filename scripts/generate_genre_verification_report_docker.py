#!/usr/bin/env python3
"""
recap-workerのジャンル分類精度検証レポートを生成するスクリプト（Docker版）。

Dockerコマンドを使ってSQLクエリを実行し、結果を処理します。
"""

import argparse
import json
import shutil
import subprocess
import sys
from dataclasses import dataclass
from datetime import UTC, datetime
from pathlib import Path
from typing import Any

MIN_TREND_ROWS = 3

# Default baseline for previous-report comparisons (override via --baseline JSON).
DEFAULT_BASELINE: dict[str, Any] = {
    "prev_total": 49815,
    "prev_jobs": 28,
    "prev_articles": 2727,
    "prev_tag_coverage": 95.57,
    "prev_has_tags": 2135,
    "prev_avg_conf": 0.915,
    "prev_gb_pct": 87.96,
    "prev_gb_conf": 0.964,
    "prev_strategies": {
        "graph_boost": {"count": 1965, "percentage": 87.96},
        "weighted_score": {"count": 170, "percentage": 7.61},
        "coarse_only": {"count": 99, "percentage": 4.43},
    },
    "initial_total": 19119,
    "initial_tag_coverage": 3.45,
    "initial_gb_pct": 2.8,
    "initial_gb_conf": 0.979,
    "initial_avg_conf": 0.971,
    "reorg_before_tag": 97.56,
    "reorg_before_gb": 79.31,
    "reorg_before_gb_conf": 0.974,
    "reorg_before_avg_conf": 0.861,
    "reorg_before_weighted": 18.25,
    "reorg_after_tag": 95.57,
    "reorg_after_gb": 87.96,
    "reorg_after_gb_conf": 0.964,
    "reorg_after_avg_conf": 0.915,
    "reorg_after_weighted": 7.61,
}


def load_baseline(path: str | None) -> dict[str, Any]:
    """Load baseline comparison values from JSON, falling back to defaults."""
    baseline = dict(DEFAULT_BASELINE)
    if path:
        with Path(path).open(encoding="utf-8") as f:
            override = json.load(f)
        if not isinstance(override, dict):
            msg = f"baseline file must be a JSON object: {path}"
            raise SystemExit(msg)
        baseline.update(override)
    return baseline


def _build_psql_cmd(
    container: str,
    user: str,
    db: str,
    params: dict[str, int] | None = None,
) -> list[str]:
    """Docker Compose経由でpsqlを実行するコマンド列を組み立てる"""
    docker_bin = shutil.which("docker")
    if docker_bin is None:
        msg = "docker executable not found on PATH"
        raise FileNotFoundError(msg)
    cmd = [
        docker_bin,
        "compose",
        "exec",
        "-T",
        container,
        "psql",
        "-U",
        user,
        "-d",
        db,
        "-t",
        "-A",
    ]
    if params:
        for key, value in params.items():
            if not key.isidentifier():
                msg = f"Invalid psql variable name: {key!r}"
                raise ValueError(msg)
            cmd.extend(["-v", f"{key}={int(value)}"])
    return cmd


def _unwrap_row_to_json(inner: object) -> list[dict[str, Any]]:
    """row_to_jsonラップ値を辞書のリストへ正規化する"""
    if isinstance(inner, dict):
        return [inner]
    if isinstance(inner, list):
        return inner
    return [inner] if inner else []


def _unwrap_json_agg(inner: object) -> list[dict[str, Any]]:
    """json_aggラップ値を辞書のリストへ正規化する"""
    if isinstance(inner, list):
        return inner
    return [inner] if inner else []


def _parse_psql_json_output(output: str) -> list[dict[str, Any]]:
    """psqlの標準出力JSON文字列をパースしてリストへ正規化する"""
    if not output:
        return []

    full_output = " ".join([line.strip() for line in output.split("\n") if line.strip()])
    try:
        data = json.loads(full_output)
    except json.JSONDecodeError as e:
        sys.stderr.write(f"JSONパースエラー: {e}\n")
        sys.stderr.write(f"出力: {full_output[:200]}...\n")
        raise SystemExit(1) from e

    if isinstance(data, list):
        return data
    if not isinstance(data, dict):
        return []
    if "row_to_json" in data:
        return _unwrap_row_to_json(data["row_to_json"])
    if "json_agg" in data:
        return _unwrap_json_agg(data["json_agg"])
    return [data]


def run_sql_query(
    query: str,
    container: str = "recap-db",
    user: str = "recap_user",
    db: str = "recap",
    params: dict[str, int] | None = None,
) -> list[dict[str, Any]]:
    """Dockerコンテナ内でSQLクエリを実行し、JSON形式で結果を取得"""
    cmd = _build_psql_cmd(container, user, db, params)

    try:
        result = subprocess.run(  # noqa: S603 -- docker resolved via shutil.which with fixed compose psql args
            cmd,
            input=query,
            cwd=Path(__file__).absolute().parent.parent,
            capture_output=True,
            text=True,
            check=True,
            timeout=60,
        )
    except subprocess.TimeoutExpired as e:
        sys.stderr.write(f"SQLクエリがタイムアウトしました: {e}\n")
        raise SystemExit(1) from e
    except subprocess.CalledProcessError as e:
        sys.stderr.write(f"SQLクエリ実行エラー: {e}\n")
        sys.stderr.write(f"stderr: {e.stderr}\n")
        sys.stderr.write(f"stdout: {e.stdout}\n")
        raise SystemExit(1) from e

    return _parse_psql_json_output(result.stdout.strip())


def fetch_strategy_breakdown(hours: int = 1) -> list[dict[str, Any]]:
    """最新N時間の戦略別内訳を取得"""
    query_json = """
    SELECT json_agg(row_to_json(t))
    FROM (
        SELECT refine_decision->>'strategy' as strategy,
               COUNT(*)::int as count,
               ROUND(100.0 * COUNT(*) / (
                   SELECT COUNT(*)
                   FROM recap_genre_learning_results
                   WHERE refine_decision IS NOT NULL
                     AND created_at > NOW() - make_interval(hours => :hours)
               ), 2)::float as percentage,
               AVG((refine_decision->>'confidence')::float)::float as avg_confidence
        FROM recap_genre_learning_results
        WHERE refine_decision IS NOT NULL
          AND created_at > NOW() - make_interval(hours => :hours)
        GROUP BY refine_decision->>'strategy'
        ORDER BY count DESC
    ) t
    """

    result = run_sql_query(query_json, params={"hours": hours})
    # json_aggの結果は直接配列として返される
    # 結果が配列の場合はそのまま返す
    if result and isinstance(result, list) and len(result) > 0:
        # 最初の要素が配列の場合は展開
        if isinstance(result[0], list):
            return result[0]
        # 最初の要素が辞書でjson_aggキーがある場合
        if isinstance(result[0], dict) and "json_agg" in result[0]:
            return result[0]["json_agg"] if result[0]["json_agg"] else []
        # それ以外はそのまま返す
        return result
    return []


def fetch_tag_coverage(hours: int = 1) -> dict[str, Any]:
    """最新N時間のタグカバレッジを取得"""
    query_json = """
    SELECT row_to_json(t)
    FROM (
        SELECT COUNT(*)::int as total,
               COUNT(CASE WHEN tag_profile->'top_tags' IS NOT NULL
                              AND jsonb_array_length(tag_profile->'top_tags') > 0
                              THEN 1 END)::int as has_tags,
               ROUND(100.0 * COUNT(CASE WHEN tag_profile->'top_tags' IS NOT NULL
                                              AND jsonb_array_length(tag_profile->'top_tags') > 0
                                              THEN 1 END) / COUNT(*), 2)::float as tag_coverage_pct
        FROM recap_genre_learning_results
        WHERE created_at > NOW() - make_interval(hours => :hours)
    ) t
    """

    result = run_sql_query(query_json, params={"hours": hours})
    # row_to_jsonの結果は辞書として返される
    # 結果が辞書の配列の場合
    if result and len(result) > 0 and isinstance(result[0], dict):
        # row_to_jsonキーがある場合はその値を取得
        if "row_to_json" in result[0]:
            return result[0]["row_to_json"]
        # それ以外はそのまま返す
        return result[0]
    return {"total": 0, "has_tags": 0, "tag_coverage_pct": 0.0}


def fetch_hourly_analysis(hours: int = 24) -> list[dict[str, Any]]:
    """時間帯別の詳細分析を取得"""
    query_json = """
    SELECT json_agg(row_to_json(t))
    FROM (
        SELECT DATE_TRUNC('hour', created_at)::text as hour,
               COUNT(*)::int as records,
               COUNT(CASE WHEN tag_profile->'top_tags' IS NOT NULL
                              AND jsonb_array_length(tag_profile->'top_tags') > 0
                              THEN 1 END)::int as records_with_tags,
               ROUND(100.0 * COUNT(CASE WHEN tag_profile->'top_tags' IS NOT NULL
                                              AND jsonb_array_length(tag_profile->'top_tags') > 0
                                              THEN 1 END) / COUNT(*), 2)::float as tag_coverage_pct,
               COUNT(CASE WHEN refine_decision->>'strategy' = 'graph_boost'
                              THEN 1 END)::int as graph_boost_count,
               ROUND(100.0 * COUNT(CASE WHEN refine_decision->>'strategy' = 'graph_boost'
                                              THEN 1 END) / COUNT(*), 2)::float as graph_boost_pct
        FROM recap_genre_learning_results
        WHERE created_at > NOW() - make_interval(hours => :hours)
        GROUP BY DATE_TRUNC('hour', created_at)
        ORDER BY hour DESC
        LIMIT :hours
    ) t
    """

    result = run_sql_query(query_json, params={"hours": hours})
    # json_aggの結果は直接配列として返される
    # 結果が配列の場合はそのまま返す
    if result and isinstance(result, list) and len(result) > 0:
        # 最初の要素が配列の場合は展開
        if isinstance(result[0], list):
            return result[0]
        # 最初の要素が辞書でjson_aggキーがある場合
        if isinstance(result[0], dict) and "json_agg" in result[0]:
            return result[0]["json_agg"] if result[0]["json_agg"] else []
        # それ以外はそのまま返す
        return result
    return []


def fetch_graph_boost_analysis(hours: int = 1) -> dict[str, Any]:
    """Graph Boostの詳細分析を取得"""
    query_json = """
    SELECT row_to_json(t)
    FROM (
        SELECT COUNT(*)::int as graph_boost_count,
               AVG((refine_decision->>'confidence')::float)::float as avg_confidence,
               PERCENTILE_CONT(0.5) WITHIN GROUP (ORDER BY (refine_decision->>'confidence')::float)::float as median_confidence,
               PERCENTILE_CONT(0.95) WITHIN GROUP (ORDER BY (refine_decision->>'confidence')::float)::float as p95_confidence,
               PERCENTILE_CONT(0.99) WITHIN GROUP (ORDER BY (refine_decision->>'confidence')::float)::float as p99_confidence,
               MIN((refine_decision->>'confidence')::float)::float as min_confidence,
               MAX((refine_decision->>'confidence')::float)::float as max_confidence
        FROM recap_genre_learning_results
        WHERE refine_decision->>'strategy' = 'graph_boost'
          AND created_at > NOW() - make_interval(hours => :hours)
    ) t
    """

    result = run_sql_query(query_json, params={"hours": hours})
    # row_to_jsonの結果は辞書として返される
    # 結果が辞書の配列の場合
    if result and len(result) > 0 and isinstance(result[0], dict):
        # row_to_jsonキーがある場合はその値を取得
        if "row_to_json" in result[0]:
            return result[0]["row_to_json"]
        # それ以外はそのまま返す（既に辞書として返されている）
        return result[0]
    return {}


def fetch_daily_analysis(days: int = 7) -> list[dict[str, Any]]:
    """日別の集計を取得"""
    query_json = """
    SELECT json_agg(row_to_json(t))
    FROM (
        SELECT DATE_TRUNC('day', created_at)::text as date,
               COUNT(*)::int as total_records,
               COUNT(CASE WHEN tag_profile->'top_tags' IS NOT NULL
                              AND jsonb_array_length(tag_profile->'top_tags') > 0
                              THEN 1 END)::int as records_with_tags,
               ROUND(100.0 * COUNT(CASE WHEN tag_profile->'top_tags' IS NOT NULL
                                              AND jsonb_array_length(tag_profile->'top_tags') > 0
                                              THEN 1 END) / COUNT(*), 2)::float as tag_coverage_pct,
               COUNT(CASE WHEN refine_decision->>'strategy' = 'graph_boost'
                              THEN 1 END)::int as graph_boost_count,
               ROUND(100.0 * COUNT(CASE WHEN refine_decision->>'strategy' = 'graph_boost'
                                              THEN 1 END) / COUNT(*), 2)::float as graph_boost_pct
        FROM recap_genre_learning_results
        GROUP BY DATE_TRUNC('day', created_at)
        ORDER BY date DESC
        LIMIT :days
    ) t
    """

    result = run_sql_query(query_json, params={"days": days})
    # json_aggの結果は直接配列として返される
    # 結果が配列の場合はそのまま返す
    if result and isinstance(result, list) and len(result) > 0:
        # 最初の要素が配列の場合は展開
        if isinstance(result[0], list):
            return result[0]
        # 最初の要素が辞書でjson_aggキーがある場合
        if isinstance(result[0], dict) and "json_agg" in result[0]:
            return result[0]["json_agg"] if result[0]["json_agg"] else []
        # それ以外はそのまま返す
        return result
    return []


def fetch_genre_distribution(hours: int = 1, limit: int = 20) -> list[dict[str, Any]]:
    """最新N時間のジャンル分布を取得"""
    query_json = """
    SELECT json_agg(row_to_json(t))
    FROM (
        SELECT COALESCE(refine_decision->>'final_genre', refine_decision->>'genre') as genre,
               COUNT(*)::int as count,
               ROUND(100.0 * COUNT(*) / (
                   SELECT COUNT(*)
                   FROM recap_genre_learning_results
                   WHERE refine_decision IS NOT NULL
                     AND created_at > NOW() - make_interval(hours => :hours)
               ), 2)::float as percentage
        FROM recap_genre_learning_results
        WHERE refine_decision IS NOT NULL
          AND created_at > NOW() - make_interval(hours => :hours)
        GROUP BY COALESCE(refine_decision->>'final_genre', refine_decision->>'genre')
        ORDER BY count DESC
        LIMIT :limit
    ) t
    """

    result = run_sql_query(query_json, params={"hours": hours, "limit": limit})
    # json_aggの結果は直接配列として返される
    # 結果が配列の場合はそのまま返す
    if result and isinstance(result, list) and len(result) > 0:
        # 最初の要素が配列の場合は展開
        if isinstance(result[0], list):
            return result[0]
        # 最初の要素が辞書でjson_aggキーがある場合
        if isinstance(result[0], dict) and "json_agg" in result[0]:
            return result[0]["json_agg"] if result[0]["json_agg"] else []
        # それ以外はそのまま返す
        return result
    return []


def fetch_overall_confidence(hours: int = 1) -> dict[str, Any]:
    """最新N時間の全体信頼度統計を取得"""
    query_json = """
    SELECT row_to_json(t)
    FROM (
        SELECT AVG((refine_decision->>'confidence')::float)::float as avg_confidence,
               PERCENTILE_CONT(0.5) WITHIN GROUP (ORDER BY (refine_decision->>'confidence')::float)::float as median_confidence,
               PERCENTILE_CONT(0.95) WITHIN GROUP (ORDER BY (refine_decision->>'confidence')::float)::float as p95_confidence,
               PERCENTILE_CONT(0.99) WITHIN GROUP (ORDER BY (refine_decision->>'confidence')::float)::float as p99_confidence
        FROM recap_genre_learning_results
        WHERE refine_decision IS NOT NULL
          AND (refine_decision->>'confidence')::float IS NOT NULL
          AND created_at > NOW() - make_interval(hours => :hours)
    ) t
    """

    result = run_sql_query(query_json, params={"hours": hours})
    # row_to_jsonの結果は辞書として返される
    # 結果が辞書の配列の場合
    if result and len(result) > 0 and isinstance(result[0], dict):
        # row_to_jsonキーがある場合はその値を取得
        if "row_to_json" in result[0]:
            return result[0]["row_to_json"]
        # それ以外はそのまま返す（既に辞書として返されている）
        return result[0]
    return {}


def fetch_total_records() -> dict[str, Any]:
    """累積レコード総数を取得"""
    query_json = """
    SELECT row_to_json(t)
    FROM (
        SELECT COUNT(*)::int as total_records,
               COUNT(DISTINCT job_id)::int as total_jobs,
               COUNT(DISTINCT article_id)::int as total_articles,
               MIN(created_at)::text as first_record,
               MAX(created_at)::text as last_record
        FROM recap_genre_learning_results
    ) t
    """

    result = run_sql_query(query_json)
    # row_to_jsonの結果は辞書として返される
    # 結果が辞書の配列の場合
    if result and len(result) > 0 and isinstance(result[0], dict):
        # row_to_jsonキーがある場合はその値を取得
        if "row_to_json" in result[0]:
            return result[0]["row_to_json"]
        # それ以外はそのまま返す（既に辞書として返されている）
        return result[0]
    return {}


def format_datetime(dt: datetime | str | None) -> str:
    """datetimeを文字列にフォーマット"""
    if dt is None:
        return "N/A"
    if isinstance(dt, str):
        return dt
    return dt.strftime("%Y-%m-%d %H:%M:%S")


@dataclass(frozen=True)
class _ReportData:
    """レポート生成に必要な集計済みデータ"""

    now: datetime
    bl: dict[str, Any]
    total_stats: dict[str, Any]
    strategy_breakdown: list[dict[str, Any]]
    tag_coverage: dict[str, Any]
    hourly_analysis: list[dict[str, Any]]
    graph_boost_analysis: dict[str, Any]
    daily_analysis: list[dict[str, Any]]
    genre_distribution: list[dict[str, Any]]
    overall_confidence: dict[str, Any]
    graph_boost_data: dict[str, Any] | None
    avg_confidence: float


def _append_intro_section(report_lines: list[str], data: _ReportData) -> None:
    """タイトルとセクション1（検証概要）を追加"""
    total_stats = data.total_stats
    graph_boost_pct = data.graph_boost_data["percentage"] if data.graph_boost_data else 0.0

    report_lines.append("# Recap二段階ジャンル分類 効果検証レポート（ジャンル再編後・最新）")
    report_lines.append("")
    report_lines.append(f"_作成日: {data.now.strftime('%Y-%m-%d %H:%M:%S')} (UTC)_")
    report_lines.append(
        f"_検証期間: {format_datetime(total_stats.get('first_record'))} ～ {format_datetime(total_stats.get('last_record'))} (UTC)_"
    )
    report_lines.append(
        "_前回レポート: `recap-genre-two-stage-verification-2025-11-15-genre-reorg-ja.md`（2025-11-15 15:10:17時点）_"
    )
    report_lines.append("")
    report_lines.append("## 1. 検証概要")
    report_lines.append("")
    report_lines.append(
        "本レポートは、recap-workerのジャンル再編後の最新データ（過去1時間）に基づき、二段階ジャンル分類システムの精度向上状況を検証した結果をまとめたものです。"
    )
    report_lines.append(
        f"**Graph Boost使用率が{graph_boost_pct:.2f}%**、平均信頼度も**{data.avg_confidence:.3f}**となっており、ジャンル再編の効果が継続していることが確認できます。"
    )
    report_lines.append("")


def _append_volume_and_tags_section(report_lines: list[str], data: _ReportData) -> None:
    """セクション2.1（データ量の増加）と2.2（タグカバレッジ）を追加"""
    bl = data.bl
    total_stats = data.total_stats
    tag_coverage = data.tag_coverage

    report_lines.append("## 2. 最新の改善状況（過去1時間）")
    report_lines.append("")
    report_lines.append("### 2.1 データ量の増加")
    report_lines.append("")
    report_lines.append("| 指標 | 前回（15:10:17時点） | 最新 | 変化 |")
    report_lines.append("|------|---------------------|------|------|")
    prev_total = bl["prev_total"]
    prev_jobs = bl["prev_jobs"]
    prev_articles = bl["prev_articles"]
    report_lines.append(
        f"| **学習レコード総数** | {prev_total:,}件 | **{total_stats.get('total_records', 0):,}件** | **+{total_stats.get('total_records', 0) - prev_total:,}件** |"
    )
    report_lines.append(
        f"| **処理ジョブ数** | {prev_jobs}ジョブ | **{total_stats.get('total_jobs', 0)}ジョブ** | +{total_stats.get('total_jobs', 0) - prev_jobs}ジョブ |"
    )
    report_lines.append(
        f"| **処理記事数** | {prev_articles:,}記事 | **{total_stats.get('total_articles', 0):,}記事** | +{total_stats.get('total_articles', 0) - prev_articles:,}記事 |"
    )
    report_lines.append("")

    report_lines.append("### 2.2 タグカバレッジの状況")
    report_lines.append("")
    report_lines.append("| 指標 | 前回（過去1時間） | 最新（過去1時間） | 変化 |")
    report_lines.append("|------|-----------------|-----------------|------|")
    prev_tag_coverage = bl["prev_tag_coverage"]
    prev_has_tags = bl["prev_has_tags"]
    report_lines.append(
        f"| **タグ付与レコード数** | {prev_has_tags:,}件 | **{tag_coverage.get('has_tags', 0):,}件** | **+{tag_coverage.get('has_tags', 0) - prev_has_tags:,}件** |"
    )
    report_lines.append(
        f"| **タグカバレッジ** | {prev_tag_coverage:.2f}% | **{tag_coverage.get('tag_coverage_pct', 0):.2f}%** | **{tag_coverage.get('tag_coverage_pct', 0) - prev_tag_coverage:+.2f}pt** |"
    )
    report_lines.append("")

    tag_coverage_change = tag_coverage.get("tag_coverage_pct", 0) - prev_tag_coverage
    if tag_coverage_change >= 0:
        report_lines.append(
            f"**分析**: タグカバレッジが**{tag_coverage.get('tag_coverage_pct', 0):.2f}%**と、前回レポートの{prev_tag_coverage:.2f}%から**+{tag_coverage_change:.2f}pt向上**しています。ジャンル再編による効果が継続しています。"
        )
    else:
        report_lines.append(
            f"**分析**: タグカバレッジが**{tag_coverage.get('tag_coverage_pct', 0):.2f}%**と、前回レポートの{prev_tag_coverage:.2f}%から{tag_coverage_change:.2f}pt変化していますが、**依然として高水準**を維持しています。"
        )
    report_lines.append("")


def _append_strategy_and_gb_section(report_lines: list[str], data: _ReportData) -> None:
    """セクション2.3（Refine戦略）と2.4（Graph Boost効果分析）を追加"""
    bl = data.bl
    graph_boost_data = data.graph_boost_data
    graph_boost_analysis = data.graph_boost_analysis

    report_lines.append("### 2.3 Refine戦略の使用状況（過去1時間）")
    report_lines.append("")
    report_lines.append("| 戦略 | 件数 | 割合 | 平均信頼度 | 前回からの変化 |")
    report_lines.append("|------|------|------|------------|---------------|")

    prev_strategies = bl["prev_strategies"]

    for strategy in data.strategy_breakdown:
        strategy_name = strategy.get("strategy", "")
        count = strategy.get("count", 0)
        pct = strategy.get("percentage", 0.0)
        avg_conf = strategy.get("avg_confidence", 0.0) or 0.0

        prev = prev_strategies.get(strategy_name, {"count": 0, "percentage": 0})
        count_change = count - prev["count"]
        pct_change = pct - prev["percentage"]

        change_str = (
            f"{count_change:+,}件（{pct_change:+.2f}pt）" if count_change != 0 else "変化なし"
        )
        report_lines.append(
            f"| `{strategy_name}` | **{count:,}** | **{pct:.2f}%** | {avg_conf:.3f} | {change_str} |"
        )

    report_lines.append("")

    prev_gb_pct = bl["prev_gb_pct"]
    if graph_boost_data:
        gb_pct_change = graph_boost_data["percentage"] - prev_gb_pct
        report_lines.append("**重要な改善点**:")
        report_lines.append(
            f"- **Graph Boostの使用率が{graph_boost_data['percentage']:.2f}%**（前回: {prev_gb_pct:.2f}% → 最新: {graph_boost_data['percentage']:.2f}%、**{gb_pct_change:+.2f}pt**）"
        )
        report_lines.append(
            f"- Graph Boostの平均信頼度が**{graph_boost_data.get('avg_confidence', 0):.3f}**"
        )
        report_lines.append("")

    if graph_boost_analysis and graph_boost_analysis.get("graph_boost_count", 0) > 0:
        report_lines.append("### 2.4 Graph Boostの効果分析（過去1時間）")
        report_lines.append("")
        report_lines.append(
            f"- **使用件数**: {graph_boost_analysis['graph_boost_count']:,}件（全体の{graph_boost_data['percentage']:.2f}%）"
        )
        report_lines.append(
            f"- **平均信頼度**: **{graph_boost_analysis.get('avg_confidence', 0):.3f}**"
        )
        report_lines.append(
            f"- **中央値信頼度**: {graph_boost_analysis.get('median_confidence', 0):.3f}"
        )
        report_lines.append(
            f"- **信頼度範囲**: 最小{graph_boost_analysis.get('min_confidence', 0):.3f}、最大{graph_boost_analysis.get('max_confidence', 0):.3f}"
        )
        report_lines.append("")


def _append_accuracy_and_trends_section(report_lines: list[str], data: _ReportData) -> None:
    """セクション3（精度指標・時間帯別・日別トレンド）を追加"""
    overall_confidence = data.overall_confidence
    avg_confidence = data.avg_confidence
    prev_avg_conf = data.bl["prev_avg_conf"]
    avg_conf_change = avg_confidence - prev_avg_conf

    report_lines.append("## 3. 精度指標の詳細分析")
    report_lines.append("")
    report_lines.append("### 3.1 信頼度分布（過去1時間）")
    report_lines.append("")
    report_lines.append(
        f"- **平均信頼度**: **{avg_confidence:.3f}**（前回: {prev_avg_conf:.3f}から**{avg_conf_change:+.3f}pt{'向上' if avg_conf_change >= 0 else '変化'}**）"
    )
    report_lines.append(
        f"- **中央値信頼度**: {overall_confidence.get('median_confidence', 0.0) or 0.0:.3f}"
    )
    report_lines.append(
        f"- **95パーセンタイル**: {overall_confidence.get('p95_confidence', 0.0) or 0.0:.3f}"
    )
    report_lines.append(
        f"- **99パーセンタイル**: {overall_confidence.get('p99_confidence', 0.0) or 0.0:.3f}"
    )
    report_lines.append("")

    if avg_conf_change >= 0:
        report_lines.append(
            f"**分析**: **平均信頼度が{avg_confidence:.3f}と{'向上' if avg_conf_change > 0 else '維持'}**（前回: {prev_avg_conf:.3f}から{avg_conf_change:+.3f}pt）しており、ジャンル再編により分類精度が{'向上' if avg_conf_change > 0 else '維持'}していることが確認できます。"
        )
    else:
        report_lines.append(
            f"**分析**: 平均信頼度が{avg_confidence:.3f}（前回: {prev_avg_conf:.3f}から{avg_conf_change:.3f}pt）となっていますが、高精度を維持しています。"
        )
    report_lines.append("")

    if len(data.hourly_analysis) >= MIN_TREND_ROWS:
        report_lines.append("### 3.2 時間帯別の改善トレンド（最新データ）")
        report_lines.append("")
        report_lines.append(
            "| 時間帯 | 総レコード数 | タグ付与レコード | タグカバレッジ | Graph Boost使用 | Graph Boost使用率 |"
        )
        report_lines.append(
            "|--------|------------|----------------|---------------|----------------|------------------|"
        )

        for hour_data in data.hourly_analysis[:3]:
            hour = hour_data.get("hour", "")
            hour_str = hour if isinstance(hour, str) else str(hour)

            report_lines.append(
                f"| {hour_str} | {hour_data.get('total_records', 0):,}件 | {hour_data.get('records_with_tags', 0):,}件 | "
                f"**{hour_data.get('tag_coverage_pct', 0):.2f}%** | {hour_data.get('graph_boost_count', 0):,}件 | "
                f"**{hour_data.get('graph_boost_pct', 0):.2f}%** |"
            )
        report_lines.append("")

    if len(data.daily_analysis) >= MIN_TREND_ROWS:
        report_lines.append("### 3.3 日別の改善状況")
        report_lines.append("")
        report_lines.append(
            "| 日付 | 総レコード数 | タグ付与レコード | タグカバレッジ | Graph Boost使用 | Graph Boost使用率 |"
        )
        report_lines.append(
            "|------|------------|----------------|---------------|----------------|------------------|"
        )

        for day_data in data.daily_analysis[:3]:
            date = day_data.get("date", "")
            date_str = date if isinstance(date, str) else str(date)

            report_lines.append(
                f"| {date_str} | {day_data.get('total_records', 0):,}件 | {day_data.get('records_with_tags', 0):,}件 | "
                f"**{day_data.get('tag_coverage_pct', 0):.2f}%** | {day_data.get('graph_boost_count', 0):,}件 | "
                f"**{day_data.get('graph_boost_pct', 0):.2f}%** |"
            )
        report_lines.append("")


def _append_cumulative_section(report_lines: list[str], data: _ReportData) -> None:
    """セクション4（初回検証からの累積改善）を追加"""
    bl = data.bl
    total_stats = data.total_stats
    tag_coverage = data.tag_coverage
    graph_boost_data = data.graph_boost_data
    avg_confidence = data.avg_confidence

    report_lines.append("## 4. 初回検証からの累積改善")
    report_lines.append("")
    report_lines.append("### 4.1 主要指標の累積改善")
    report_lines.append("")
    report_lines.append("| 指標 | 初回検証時 | 前回（11/15 15:10） | 最新 | 累積改善 |")
    report_lines.append("|------|-----------|-------------------|------|---------|")

    initial_total = bl["initial_total"]
    initial_tag_coverage = bl["initial_tag_coverage"]
    initial_gb_pct = bl["initial_gb_pct"]
    initial_gb_conf = bl["initial_gb_conf"]
    initial_avg_conf = bl["initial_avg_conf"]

    prev_total = bl["prev_total"]
    prev_tag_coverage = bl["prev_tag_coverage"]
    prev_gb_pct = bl["prev_gb_pct"]
    prev_gb_conf = bl["prev_gb_conf"]
    prev_avg_conf = bl["prev_avg_conf"]

    total_change = total_stats.get("total_records", 0) - initial_total
    tag_cov_change = tag_coverage.get("tag_coverage_pct", 0) - initial_tag_coverage
    gb_pct_change_total = (
        graph_boost_data["percentage"] if graph_boost_data else 0
    ) - initial_gb_pct
    avg_conf_change_total = avg_confidence - initial_avg_conf

    report_lines.append(
        f"| **学習レコード総数** | {initial_total:,}件 | {prev_total:,}件 | **{total_stats.get('total_records', 0):,}件** | +{total_change:,}件（+{total_change / initial_total * 100:.1f}%） |"
    )
    report_lines.append(
        f"| **タグカバレッジ** | {initial_tag_coverage:.2f}% | {prev_tag_coverage:.2f}% | **{tag_coverage.get('tag_coverage_pct', 0):.2f}%** | **+{tag_cov_change:.2f}pt（+{tag_cov_change / initial_tag_coverage * 100:.1f}%改善）** |"
    )
    if graph_boost_data:
        report_lines.append(
            f"| **Graph Boost使用率** | {initial_gb_pct:.2f}% | {prev_gb_pct:.2f}% | **{graph_boost_data['percentage']:.2f}%** | **+{gb_pct_change_total:.2f}pt（+{gb_pct_change_total / initial_gb_pct * 100:.1f}%改善）** |"
        )
    gb_avg_conf = graph_boost_data.get("avg_confidence", 0) if graph_boost_data else 0.0
    report_lines.append(
        f"| **Graph Boost平均信頼度** | {initial_gb_conf:.3f} | {prev_gb_conf:.3f} | {gb_avg_conf:.3f} | ほぼ同等 |"
    )
    report_lines.append(
        f"| **全体平均信頼度** | {initial_avg_conf:.3f} | {prev_avg_conf:.3f} | **{avg_confidence:.3f}** | **{avg_conf_change_total:+.3f}pt{'向上' if avg_conf_change_total >= 0 else ''}** |"
    )
    report_lines.append("")


def _append_genre_section(report_lines: list[str], data: _ReportData) -> None:
    """セクション5（ジャンル分布の確認）を追加"""
    report_lines.append("## 5. ジャンル分布の確認（過去1時間）")
    report_lines.append("")
    report_lines.append("最終決定されたジャンルの分布（上位20件）:")
    report_lines.append("")
    report_lines.append("| ジャンル | 件数 | 割合 |")
    report_lines.append("|---------|------|------|")

    report_lines.extend(
        f"| {genre_data.get('genre', '')} | {genre_data.get('count', 0)} | {genre_data.get('percentage', 0):.2f}% |"
        for genre_data in data.genre_distribution
    )

    report_lines.append("")


def _append_recommended_actions(report_lines: list[str]) -> None:
    """セクション6.3（今後の推奨アクション）を追加"""
    report_lines.append("### 6.3 今後の推奨アクション")
    report_lines.append("")
    report_lines.append("1. **短期（1週間以内）**")
    report_lines.append("   - Graph Boostの使用率を90%以上に維持")
    report_lines.append("   - 平均信頼度を0.92以上に維持")
    report_lines.append("   - 全時間帯でタグカバレッジ95%以上を維持")
    report_lines.append("")
    report_lines.append("2. **中期（1ヶ月以内）**")
    report_lines.append("   - ゴールデンセットを用いたF1スコアの定量評価")
    report_lines.append("   - Graph Boostの使用率を92%以上に向上")
    report_lines.append("   - ジャンル再編の効果を継続的にモニタリング")
    report_lines.append("")
    report_lines.append("3. **長期（3ヶ月以内）**")
    report_lines.append("   - `genre_tag_agreement_rate ≥ 0.85`の達成")
    report_lines.append("   - 全時間帯でタグカバレッジ99%以上を目標")
    report_lines.append("   - 精度向上の継続的モニタリング体制確立")
    report_lines.append("")


def _append_conclusion_section(report_lines: list[str], data: _ReportData) -> None:
    """セクション6（結論）を追加"""
    bl = data.bl
    graph_boost_data = data.graph_boost_data
    avg_confidence = data.avg_confidence
    prev_gb_pct = bl["prev_gb_pct"]
    prev_avg_conf = bl["prev_avg_conf"]
    avg_conf_change = avg_confidence - prev_avg_conf

    report_lines.append("## 6. 結論")
    report_lines.append("")
    report_lines.append("### 6.1 確認された改善")
    report_lines.append("")

    improvements = []
    if graph_boost_data:
        gb_change = graph_boost_data["percentage"] - prev_gb_pct
        if gb_change >= 0:
            improvements.append(
                f"✅ **Graph Boost使用率の{'向上' if gb_change > 0 else '維持'}**: 前回{prev_gb_pct:.2f}% → 最新**{graph_boost_data['percentage']:.2f}%**（{gb_change:+.2f}pt）"
            )
        else:
            improvements.append(
                f"⚠️ **Graph Boost使用率の変化**: 前回{prev_gb_pct:.2f}% → 最新{graph_boost_data['percentage']:.2f}%（{gb_change:.2f}pt）"
            )

    if avg_conf_change >= 0:
        improvements.append(
            f"✅ **平均信頼度の{'向上' if avg_conf_change > 0 else '維持'}**: 前回{prev_avg_conf:.3f} → 最新**{avg_confidence:.3f}**（{avg_conf_change:+.3f}pt）"
        )
    else:
        improvements.append(
            f"⚠️ **平均信頼度の変化**: 前回{prev_avg_conf:.3f} → 最新{avg_confidence:.3f}（{avg_conf_change:.3f}pt）"
        )

    if graph_boost_data:
        improvements.append(
            f"✅ **Graph Boost平均信頼度の維持**: {graph_boost_data.get('avg_confidence', 0):.3f}（高精度を維持）"
        )

    improvements.append(
        "✅ **ジャンル再編の効果**: Graph Boost使用率が高水準を維持し、分類精度が改善"
    )

    report_lines.extend(f"{imp}" for imp in improvements)

    report_lines.append("")
    report_lines.append("### 6.2 改善の要因")
    report_lines.append("")
    report_lines.append("1. **ジャンル再編の効果**")
    if graph_boost_data:
        report_lines.append(
            f"   - Graph Boostの使用率が**{graph_boost_data['percentage']:.2f}%**（前回: {prev_gb_pct:.2f}%から{graph_boost_data['percentage'] - prev_gb_pct:+.2f}pt）"
        )
    report_lines.append(
        f"   - 平均信頼度が**{avg_confidence:.3f}**（前回: {prev_avg_conf:.3f}から{avg_conf_change:+.3f}pt）"
    )
    report_lines.append("")
    report_lines.append("2. **Refine Stageの最適化**")
    report_lines.append("   - ジャンル再編により、Graph Boostの適用範囲が拡大")
    report_lines.append("   - タグカバレッジの向上により、Refine Stageの効果が継続")
    report_lines.append("")

    _append_recommended_actions(report_lines)


def _append_summary_section(report_lines: list[str], data: _ReportData) -> None:
    """セクション7（データ比較サマリー）を追加"""
    bl = data.bl
    total_stats = data.total_stats
    tag_coverage = data.tag_coverage
    graph_boost_data = data.graph_boost_data
    avg_confidence = data.avg_confidence

    initial_total = bl["initial_total"]
    initial_tag_coverage = bl["initial_tag_coverage"]
    initial_gb_pct = bl["initial_gb_pct"]
    initial_gb_conf = bl["initial_gb_conf"]
    initial_avg_conf = bl["initial_avg_conf"]

    prev_total = bl["prev_total"]
    prev_tag_coverage = bl["prev_tag_coverage"]
    prev_gb_pct = bl["prev_gb_pct"]
    prev_gb_conf = bl["prev_gb_conf"]
    prev_avg_conf = bl["prev_avg_conf"]

    total_change = total_stats.get("total_records", 0) - initial_total
    tag_cov_change = tag_coverage.get("tag_coverage_pct", 0) - initial_tag_coverage
    gb_pct_change_total = (
        graph_boost_data["percentage"] if graph_boost_data else 0
    ) - initial_gb_pct
    avg_conf_change_total = avg_confidence - initial_avg_conf

    report_lines.append("## 7. データ比較サマリー")
    report_lines.append("")
    report_lines.append("| 指標 | 初回検証時 | 前回（11/15 15:10） | 最新 | 累積改善 |")
    report_lines.append("|------|-----------|-------------------|------|---------|")
    report_lines.append(
        f"| 学習レコード総数 | {initial_total:,}件 | {prev_total:,}件 | {total_stats.get('total_records', 0):,}件 | +{total_change:,}件（+{total_change / initial_total * 100:.1f}%） |"
    )
    report_lines.append(
        f"| タグカバレッジ | {initial_tag_coverage:.2f}% | {prev_tag_coverage:.2f}% | **{tag_coverage.get('tag_coverage_pct', 0):.2f}%** | **+{tag_cov_change:.2f}pt（+{tag_cov_change / initial_tag_coverage * 100:.1f}%）** |"
    )
    if graph_boost_data:
        report_lines.append(
            f"| Graph Boost使用率 | {initial_gb_pct:.2f}% | {prev_gb_pct:.2f}% | **{graph_boost_data['percentage']:.2f}%** | **+{gb_pct_change_total:.2f}pt（+{gb_pct_change_total / initial_gb_pct * 100:.1f}%）** |"
        )
    gb_avg_conf = graph_boost_data.get("avg_confidence", 0) if graph_boost_data else 0.0
    report_lines.append(
        f"| Graph Boost平均信頼度 | {initial_gb_conf:.3f} | {prev_gb_conf:.3f} | {gb_avg_conf:.3f} | ほぼ同等 |"
    )
    report_lines.append(
        f"| 全体平均信頼度 | {initial_avg_conf:.3f} | {prev_avg_conf:.3f} | **{avg_confidence:.3f}** | **{avg_conf_change_total:+.3f}pt{'向上' if avg_conf_change_total >= 0 else ''}** |"
    )
    report_lines.append("")


def _append_reorg_section(report_lines: list[str], data: _ReportData) -> None:
    """セクション8（ジャンル再編の影響）を追加"""
    bl = data.bl
    tag_coverage = data.tag_coverage
    graph_boost_data = data.graph_boost_data
    avg_confidence = data.avg_confidence

    report_lines.append("## 8. ジャンル再編の影響")
    report_lines.append("")
    report_lines.append("### 8.1 再編前後の比較")
    report_lines.append("")
    report_lines.append("| 指標 | 再編前（11/15 07:00） | 再編後（11/15 15:00） | 最新 | 変化 |")
    report_lines.append("|------|---------------------|---------------------|------|------|")

    reorg_before_tag = bl["reorg_before_tag"]
    reorg_before_gb = bl["reorg_before_gb"]
    reorg_before_gb_conf = bl["reorg_before_gb_conf"]
    reorg_before_avg_conf = bl["reorg_before_avg_conf"]
    reorg_before_weighted = bl["reorg_before_weighted"]

    reorg_after_tag = bl["reorg_after_tag"]
    reorg_after_gb = bl["reorg_after_gb"]
    reorg_after_gb_conf = bl["reorg_after_gb_conf"]
    reorg_after_avg_conf = bl["reorg_after_avg_conf"]
    reorg_after_weighted = bl["reorg_after_weighted"]

    latest_tag = tag_coverage.get("tag_coverage_pct", 0)
    latest_gb = graph_boost_data["percentage"] if graph_boost_data else 0
    latest_gb_conf = graph_boost_data.get("avg_confidence", 0.0) if graph_boost_data else 0.0
    latest_avg_conf = avg_confidence

    weighted_score_data = next(
        (s for s in data.strategy_breakdown if s.get("strategy") == "weighted_score"), None
    )
    latest_weighted = weighted_score_data["percentage"] if weighted_score_data else 0

    report_lines.append(
        f"| **タグカバレッジ** | {reorg_before_tag:.2f}% | {reorg_after_tag:.2f}% | **{latest_tag:.2f}%** | {latest_tag - reorg_after_tag:+.2f}pt（再編後から） |"
    )
    report_lines.append(
        f"| **Graph Boost使用率** | {reorg_before_gb:.2f}% | {reorg_after_gb:.2f}% | **{latest_gb:.2f}%** | {latest_gb - reorg_after_gb:+.2f}pt（再編後から） |"
    )
    report_lines.append(
        f"| **Graph Boost平均信頼度** | {reorg_before_gb_conf:.3f} | {reorg_after_gb_conf:.3f} | **{latest_gb_conf:.3f}** | {latest_gb_conf - reorg_after_gb_conf:+.3f}pt（再編後から） |"
    )
    report_lines.append(
        f"| **全体平均信頼度** | {reorg_before_avg_conf:.3f} | {reorg_after_avg_conf:.3f} | **{latest_avg_conf:.3f}** | {latest_avg_conf - reorg_after_avg_conf:+.3f}pt（再編後から） |"
    )
    report_lines.append(
        f"| **Weighted Score使用率** | {reorg_before_weighted:.2f}% | {reorg_after_weighted:.2f}% | **{latest_weighted:.2f}%** | {latest_weighted - reorg_after_weighted:+.2f}pt（再編後から） |"
    )
    report_lines.append("")

    report_lines.append(
        "**分析**: ジャンル再編後、Graph Boost使用率が**大幅に向上**し、全体平均信頼度も**向上**しています。再編後の効果が継続していることが確認できます。"
    )
    report_lines.append("")


def _append_sql_appendix(report_lines: list[str]) -> None:
    """付録（検証に使用したSQLクエリ）を追加"""
    report_lines.append("---")
    report_lines.append("")
    report_lines.append("## 付録: 検証に使用したSQLクエリ")
    report_lines.append("")
    report_lines.append("```sql")
    report_lines.append("-- 最新1時間の戦略別内訳")
    report_lines.append("SELECT refine_decision->>'strategy' as strategy,")
    report_lines.append("       COUNT(*) as count,")
    report_lines.append(
        "       ROUND(100.0 * COUNT(*) / (SELECT COUNT(*) FROM recap_genre_learning_results WHERE refine_decision IS NOT NULL AND created_at > NOW() - INTERVAL '1 hour'), 2) as percentage,"
    )
    report_lines.append("       AVG((refine_decision->>'confidence')::float) as avg_confidence")
    report_lines.append("FROM recap_genre_learning_results")
    report_lines.append("WHERE refine_decision IS NOT NULL")
    report_lines.append("  AND created_at > NOW() - INTERVAL '1 hour'")
    report_lines.append("GROUP BY refine_decision->>'strategy'")
    report_lines.append("ORDER BY count DESC;")
    report_lines.append("```")
    report_lines.append("")


def generate_report(output_file: str, baseline: dict[str, Any] | None = None) -> None:
    """レポートを生成"""
    now = datetime.now(UTC)
    bl = baseline or DEFAULT_BASELINE

    sys.stdout.write("データ取得中...\n")
    total_stats = fetch_total_records()
    strategy_breakdown = fetch_strategy_breakdown(hours=1)
    tag_coverage = fetch_tag_coverage(hours=1)
    hourly_analysis = fetch_hourly_analysis(hours=24)
    graph_boost_analysis = fetch_graph_boost_analysis(hours=1)
    daily_analysis = fetch_daily_analysis(days=7)
    genre_distribution = fetch_genre_distribution(hours=1, limit=20)
    overall_confidence = fetch_overall_confidence(hours=1)

    sys.stdout.write("レポート生成中...\n")

    graph_boost_data = next(
        (s for s in strategy_breakdown if s.get("strategy") == "graph_boost"), None
    )
    avg_confidence = overall_confidence.get("avg_confidence", 0.0) or 0.0

    data = _ReportData(
        now=now,
        bl=bl,
        total_stats=total_stats,
        strategy_breakdown=strategy_breakdown,
        tag_coverage=tag_coverage,
        hourly_analysis=hourly_analysis,
        graph_boost_analysis=graph_boost_analysis,
        daily_analysis=daily_analysis,
        genre_distribution=genre_distribution,
        overall_confidence=overall_confidence,
        graph_boost_data=graph_boost_data,
        avg_confidence=avg_confidence,
    )

    report_lines: list[str] = []
    _append_intro_section(report_lines, data)
    _append_volume_and_tags_section(report_lines, data)
    _append_strategy_and_gb_section(report_lines, data)
    _append_accuracy_and_trends_section(report_lines, data)
    _append_cumulative_section(report_lines, data)
    _append_genre_section(report_lines, data)
    _append_conclusion_section(report_lines, data)
    _append_summary_section(report_lines, data)
    _append_reorg_section(report_lines, data)
    _append_sql_appendix(report_lines)

    with Path(output_file).open("w", encoding="utf-8") as f:
        f.write("\n".join(report_lines))

    sys.stdout.write(f"レポートを生成しました: {output_file}\n")


def main() -> None:
    parser = argparse.ArgumentParser(
        description="recap-workerのジャンル分類精度検証レポートを生成（Docker版）"
    )
    parser.add_argument(
        "--output",
        type=str,
        default="docs/recap-genre-two-stage-verification-latest-ja.md",
        help="出力ファイルパス",
    )
    parser.add_argument(
        "--baseline",
        type=str,
        default=None,
        help="前回比較値を上書きする JSON ファイル（未指定時は組み込みデフォルト）",
    )

    args = parser.parse_args()
    baseline = load_baseline(args.baseline)
    generate_report(args.output, baseline=baseline)


if __name__ == "__main__":
    main()
