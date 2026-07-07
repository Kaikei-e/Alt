"""日本語レポート生成

Jinja2テンプレートを使用して日本語Markdownレポートを生成します。
"""

from __future__ import annotations

from pathlib import Path
from typing import Any

from jinja2 import Environment, FileSystemLoader, select_autoescape
from pydantic import BaseModel

from alt_metrics.analysis import get_health_status, get_health_status_emoji
from alt_metrics.models import AnalysisResult


def _get_template_env() -> Environment:
    """Jinja2環境を取得"""
    template_dir = Path(__file__).parent / "templates"
    return Environment(
        loader=FileSystemLoader(template_dir),
        autoescape=select_autoescape(["html", "xml"]),
        trim_blocks=True,
        lstrip_blocks=True,
    )


def _row_keys(row: dict[str, Any] | BaseModel) -> list[str]:
    """行データ（dictまたはPydanticモデル）からカラム名一覧を取得"""
    return list(row.model_fields.keys()) if isinstance(row, BaseModel) else list(row.keys())


def _row_value(row: dict[str, Any] | BaseModel, column: str) -> Any:
    """行データ（dictまたはPydanticモデル）からカラム値を取得"""
    return getattr(row, column, "") if isinstance(row, BaseModel) else row.get(column, "")


def format_table(data: list[dict[str, Any] | BaseModel], columns: list[str] | None = None) -> str:
    """データをMarkdownテーブルにフォーマット

    Args:
        data: テーブルデータ（dictまたはPydanticモデルのリスト）
        columns: 表示するカラム名のリスト

    Returns:
        Markdownテーブル文字列
    """
    if not data:
        return "_データがありません_\n"

    cols = columns or _row_keys(data[0])
    header = "| " + " | ".join(cols) + " |"
    separator = "|" + "|".join("---" for _ in cols) + "|"
    rows = []
    for row in data:
        values = [str(_row_value(row, c))[:60] for c in cols]
        rows.append("| " + " | ".join(values) + " |")

    return "\n".join([header, separator, *rows]) + "\n"


def generate_japanese_report(result: AnalysisResult) -> str:
    """分析結果から日本語Markdownレポートを生成

    Args:
        result: 分析結果

    Returns:
        日本語Markdownレポート文字列
    """
    env = _get_template_env()
    template = env.get_template("report_ja.md.j2")

    # テンプレートに渡すコンテキストを準備
    status = get_health_status(result.overall_health_score)
    emoji = get_health_status_emoji(status)

    # サマリー統計
    total_logs = sum(s.total_logs for s in result.service_health)
    total_errors = sum(s.error_count for s in result.service_health)
    healthy_count = len([s for s in result.service_health if s.health_score >= 90])
    degraded_count = len([s for s in result.service_health if s.health_score < 70])

    # サービス健全性データを準備
    service_health_data = [
        {
            "service": s.name,
            "score": s.health_score,
            "status": get_health_status(s.health_score),
            "error_rate": f"{s.error_rate}%",
            "p95_ms": s.p95_latency_ms,
            "logs": s.total_logs,
        }
        for s in sorted(result.service_health, key=lambda x: x.health_score)
    ]

    context = {
        "result": result,
        "status": status,
        "emoji": emoji,
        "total_logs": total_logs,
        "total_errors": total_errors,
        "healthy_count": healthy_count,
        "degraded_count": degraded_count,
        "service_health_data": service_health_data,
        "format_table": format_table,
        "get_health_status": get_health_status,
    }

    return template.render(**context)
