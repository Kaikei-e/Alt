#!/usr/bin/env python3
"""
Analyze article size distribution in alt-db.

Measures character count and estimated token count for all articles,
calculates percentile distributions, and evaluates LLM context size fitness.

Usage:
    cd news-creator/app

    # Local development (with port forwarding)
    DB_HOST=localhost DB_PORT=5432 DB_USER=alt_db_user DB_PASSWORD=xxx DB_NAME=alt \
      uv run python scripts/analyze_article_sizes.py

    # With JSON output
    uv run python scripts/analyze_article_sizes.py --output-json results.json

    # Docker environment
    docker compose -f ../../compose/compose.yaml exec news-creator-backend \
      python scripts/analyze_article_sizes.py
"""

import argparse
import json
import os
import sys
from dataclasses import dataclass
from datetime import datetime
from pathlib import Path

import numpy as np
import psycopg2


@dataclass
class ArticleStats:
    """Statistics for article size distribution."""

    total_articles: int
    min_chars: int
    max_chars: int
    mean_chars: float
    min_tokens: int
    max_tokens: int
    mean_tokens: float
    char_percentiles: dict[str, int]
    token_percentiles: dict[str, int]
    context_fit_rates: dict[str, float]


@dataclass
class AnalysisReport:
    """Full analysis report."""

    generated_at: str
    total_articles: int
    char_stats: dict
    token_stats: dict
    context_fit_rates: dict[str, float]


# Context sizes with prompt overhead (~500 tokens)
CONTEXT_SIZES = {
    "8K": 8000 - 500,
    "16K": 16000 - 500,
    "60K": 60000 - 500,
}

PERCENTILES = [10, 25, 50, 75, 90, 95, 99, 99.9]


def estimate_tokens(text: str) -> int:
    """
    Estimate token count using character-based estimation.

    Same method as news_creator/utils/token_counter.py:
    1 character ~ 0.75 tokens (conservative for mixed Japanese/English)
    """
    if not text:
        return 1
    return max(1, (len(text) * 3) // 4)


def get_db_connection() -> psycopg2.extensions.connection:
    """Create database connection from environment variables."""
    host = os.getenv("DB_HOST", "localhost")
    port = os.getenv("DB_PORT", "5432")
    user = os.getenv("DB_USER", "alt_db_user")
    password = os.getenv("DB_PASSWORD", "")
    dbname = os.getenv("DB_NAME", "alt")

    return psycopg2.connect(
        host=host,
        port=port,
        user=user,
        password=password,
        dbname=dbname,
        connect_timeout=90,
    )


def fetch_article_contents(conn: psycopg2.extensions.connection) -> list[str]:
    """Fetch all article contents from the database."""
    with conn.cursor() as cur:
        cur.execute(
            "SELECT content FROM articles WHERE deleted_at IS NULL AND content IS NOT NULL"
        )
        rows = cur.fetchall()
    return [row[0] for row in rows]


def analyze_sizes(contents: list[str]) -> ArticleStats:
    """Analyze character and token distributions."""
    if not contents:
        return ArticleStats(
            total_articles=0,
            min_chars=0,
            max_chars=0,
            mean_chars=0.0,
            min_tokens=0,
            max_tokens=0,
            mean_tokens=0.0,
            char_percentiles={},
            token_percentiles={},
            context_fit_rates={},
        )

    char_counts = np.array([len(c) for c in contents])
    token_counts = np.array([estimate_tokens(c) for c in contents])

    # Calculate percentiles
    char_percentile_values = np.percentile(char_counts, PERCENTILES)
    token_percentile_values = np.percentile(token_counts, PERCENTILES)

    char_percentiles = {
        f"P{p}": int(v) for p, v in zip(PERCENTILES, char_percentile_values)
    }
    token_percentiles = {
        f"P{p}": int(v) for p, v in zip(PERCENTILES, token_percentile_values)
    }

    # Calculate context fit rates
    total = len(token_counts)
    context_fit_rates: dict[str, float] = {}
    for name, limit in CONTEXT_SIZES.items():
        fit_count = int(np.sum(token_counts <= limit))
        rate = round(fit_count / total * 100, 2)
        context_fit_rates[name] = rate

    return ArticleStats(
        total_articles=len(contents),
        min_chars=int(np.min(char_counts)),
        max_chars=int(np.max(char_counts)),
        mean_chars=float(np.mean(char_counts)),
        min_tokens=int(np.min(token_counts)),
        max_tokens=int(np.max(token_counts)),
        mean_tokens=float(np.mean(token_counts)),
        char_percentiles=char_percentiles,
        token_percentiles=token_percentiles,
        context_fit_rates=context_fit_rates,
    )


def format_number(n: int | float) -> str:
    """Format number with thousands separator."""
    if isinstance(n, float):
        return f"{n:,.1f}"
    return f"{n:,}"


def generate_markdown_report(stats: ArticleStats) -> str:
    """Generate Markdown report."""
    now = datetime.now().strftime("%Y-%m-%d %H:%M:%S")

    lines = [
        "# 記事サイズ分布レポート",
        "",
        "## 概要",
        f"- 総記事数: {format_number(stats.total_articles)}",
        f"- 分析日時: {now}",
        "",
        "## 文字数分布",
        "",
        "| Percentile | 文字数 |",
        "|------------|--------|",
    ]

    for key, value in stats.char_percentiles.items():
        lines.append(f"| {key} | {format_number(value)} |")

    lines.extend(
        [
            "",
            f"- 最小: {format_number(stats.min_chars)}",
            f"- 最大: {format_number(stats.max_chars)}",
            f"- 平均: {format_number(stats.mean_chars)}",
            "",
            "## トークン数分布",
            "",
            "| Percentile | トークン数 |",
            "|------------|------------|",
        ]
    )

    for key, value in stats.token_percentiles.items():
        lines.append(f"| {key} | {format_number(value)} |")

    lines.extend(
        [
            "",
            f"- 最小: {format_number(stats.min_tokens)}",
            f"- 最大: {format_number(stats.max_tokens)}",
            f"- 平均: {format_number(stats.mean_tokens)}",
            "",
            "## コンテキストサイズ適合率",
            "",
            "| Context Size | 適合率 | 推奨 |",
            "|--------------|--------|------|",
        ]
    )

    # Determine recommendation (first context that covers >= 95%)
    recommended = None
    for name, rate in stats.context_fit_rates.items():
        if rate >= 95.0 and recommended is None:
            recommended = name

    for name, rate in stats.context_fit_rates.items():
        rec_mark = "✅" if name == recommended else ""
        lines.append(f"| {name} | {rate:.1f}% | {rec_mark} |")

    # Recommendations
    lines.extend(
        [
            "",
            "## 推奨事項",
            "",
        ]
    )

    p95_tokens = stats.token_percentiles.get("P95", 0)
    p99_tokens = stats.token_percentiles.get("P99", 0)

    if recommended:
        lines.append(
            f"- {recommended} コンテキストで P95 ({format_number(p95_tokens)} トークン) までカバー可能"
        )

    fit_8k = stats.context_fit_rates.get("8K", 0)
    if fit_8k < 100:
        need_mapreduce = 100 - fit_8k
        lines.append(
            f"- 8K では約 {need_mapreduce:.1f}% の記事が Map-Reduce または切り詰めが必要"
        )

    fit_16k = stats.context_fit_rates.get("16K", 0)
    if fit_16k >= 95:
        lines.append(f"- 16K コンテキストで {fit_16k:.1f}% の記事を処理可能 (推奨)")

    lines.append(
        f"- P99 ({format_number(p99_tokens)} トークン) を超える長大記事は特別処理を検討"
    )

    return "\n".join(lines)


def generate_json_report(stats: ArticleStats) -> dict:
    """Generate JSON report."""
    return {
        "generated_at": datetime.now().isoformat(),
        "total_articles": stats.total_articles,
        "char_stats": {
            "min": stats.min_chars,
            "max": stats.max_chars,
            "mean": round(stats.mean_chars, 1),
            "percentiles": stats.char_percentiles,
        },
        "token_stats": {
            "min": stats.min_tokens,
            "max": stats.max_tokens,
            "mean": round(stats.mean_tokens, 1),
            "percentiles": stats.token_percentiles,
        },
        "context_fit_rates": stats.context_fit_rates,
    }


def main() -> None:
    """Main entry point."""
    parser = argparse.ArgumentParser(
        description="Analyze article size distribution in alt-db"
    )
    parser.add_argument(
        "--output-json",
        type=str,
        help="Output JSON file path (optional)",
    )
    parser.add_argument(
        "--quiet",
        action="store_true",
        help="Suppress Markdown output to stdout",
    )

    args = parser.parse_args()

    # Connect and fetch data
    print("Connecting to database...", file=sys.stderr)
    try:
        conn = get_db_connection()
    except Exception as e:
        print(f"Error: Failed to connect to database: {e}", file=sys.stderr)
        sys.exit(1)

    print("Fetching article contents...", file=sys.stderr)
    try:
        contents = fetch_article_contents(conn)
    except Exception as e:
        print(f"Error: Failed to fetch articles: {e}", file=sys.stderr)
        conn.close()
        sys.exit(1)
    finally:
        conn.close()

    print(f"Analyzing {len(contents)} articles...", file=sys.stderr)

    if not contents:
        print("Warning: No articles found in database", file=sys.stderr)
        sys.exit(0)

    # Analyze
    stats = analyze_sizes(contents)

    # Output Markdown report
    if not args.quiet:
        markdown_report = generate_markdown_report(stats)
        print(markdown_report)

    # Output JSON if requested
    if args.output_json:
        json_report = generate_json_report(stats)
        output_path = Path(args.output_json)
        output_path.parent.mkdir(parents=True, exist_ok=True)
        with open(output_path, "w", encoding="utf-8") as f:
            json.dump(json_report, f, indent=2, ensure_ascii=False)
        print(f"\nJSON report saved to: {args.output_json}", file=sys.stderr)


if __name__ == "__main__":
    main()
