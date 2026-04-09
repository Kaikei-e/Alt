#!/usr/bin/env python3
"""
Benchmark script for news-creator Summary performance using real articles.

Fetches real articles from alt-db and measures TTFT, decode speed, and latency.

Usage:
    cd news-creator/app

    # Set environment variables (when running outside Docker)
    export DB_HOST=localhost
    export DB_PORT=5432
    export DB_USER=alt_appuser
    export DB_PASSWORD=<from .env>
    export DB_NAME=alt

    # Run benchmark
    uv run python scripts/benchmark_real_articles.py --iterations 10

    # With custom Ollama URL
    uv run python scripts/benchmark_real_articles.py \
      --iterations 10 \
      --ollama-url http://localhost:11435 \
      --model gemma4-e4b-12k
"""

import argparse
import asyncio
import json
import os
import statistics
import sys
from dataclasses import asdict, dataclass
from datetime import datetime
from pathlib import Path
from typing import Any

import aiohttp
import psycopg2


@dataclass
class Article:
    """Article fetched from database."""

    id: str
    title: str
    content: str
    content_length: int
    size_category: str  # small, medium, large


@dataclass
class BenchmarkResult:
    """Result of a single benchmark run."""

    article_id: str
    title: str
    content_chars: int
    size_category: str
    iteration: int
    prompt_tokens: int
    completion_tokens: int

    # Timing metrics (in seconds)
    total_duration_s: float
    load_duration_s: float
    prompt_eval_duration_s: float
    eval_duration_s: float

    # Calculated metrics
    ttft_s: float  # load_duration + prompt_eval_duration
    prefill_tok_per_sec: float
    decode_tok_per_sec: float

    # Status
    success: bool
    error: str | None = None
    model: str = ""
    timestamp: str = ""


@dataclass
class BenchmarkStats:
    """Statistical summary for benchmark results."""

    total_iterations: int
    success_count: int
    error_count: int

    # By size category
    by_category: dict[str, dict[str, float]]

    # Overall stats
    ttft_mean: float
    ttft_p50: float
    ttft_p95: float
    ttft_min: float
    ttft_max: float

    decode_mean: float
    decode_p50: float
    decode_p95: float
    decode_min: float
    decode_max: float

    prefill_mean: float
    prefill_p50: float
    prefill_p95: float

    latency_mean: float
    latency_p50: float
    latency_p95: float


@dataclass
class BenchmarkConfig:
    """Configuration for benchmark run."""

    ollama_url: str = "http://localhost:11434"
    model: str = "gemma4-e4b-12k"
    num_ctx: int = 12288
    timeout_seconds: int = 300
    warmup_iterations: int = 1
    output_dir: str = "benchmark_results"

    # Article selection
    small_count: int = 3  # < 3,000 chars
    medium_count: int = 4  # 3,000-10,000 chars
    large_count: int = 3  # > 10,000 chars


# Size thresholds (characters)
SIZE_SMALL_MAX = 3000
SIZE_MEDIUM_MAX = 10000


def get_db_connection() -> psycopg2.extensions.connection:
    """Create database connection from environment variables."""
    host = os.getenv("DB_HOST", "localhost")
    port = os.getenv("DB_PORT", "5432")
    user = os.getenv("DB_USER", "alt_appuser")
    password = os.getenv("DB_PASSWORD", "")
    dbname = os.getenv("DB_NAME", "alt")

    return psycopg2.connect(
        host=host,
        port=port,
        user=user,
        password=password,
        dbname=dbname,
        connect_timeout=30,
    )


def fetch_articles_by_size(
    conn: psycopg2.extensions.connection,
    config: BenchmarkConfig,
) -> list[Article]:
    """
    Fetch articles with size distribution.

    Returns articles in three categories:
    - small: < 3,000 chars
    - medium: 3,000-10,000 chars
    - large: > 10,000 chars
    """
    articles: list[Article] = []

    with conn.cursor() as cur:
        # Fetch small articles
        cur.execute(
            """
            SELECT id, title, content, LENGTH(content) as content_length
            FROM articles
            WHERE deleted_at IS NULL
              AND LENGTH(content) >= 100
              AND LENGTH(content) < %s
            ORDER BY created_at DESC
            LIMIT %s
            """,
            (SIZE_SMALL_MAX, config.small_count),
        )
        for row in cur.fetchall():
            articles.append(
                Article(
                    id=str(row[0]),
                    title=row[1],
                    content=row[2],
                    content_length=row[3],
                    size_category="small",
                )
            )

        # Fetch medium articles
        cur.execute(
            """
            SELECT id, title, content, LENGTH(content) as content_length
            FROM articles
            WHERE deleted_at IS NULL
              AND LENGTH(content) >= %s
              AND LENGTH(content) < %s
            ORDER BY created_at DESC
            LIMIT %s
            """,
            (SIZE_SMALL_MAX, SIZE_MEDIUM_MAX, config.medium_count),
        )
        for row in cur.fetchall():
            articles.append(
                Article(
                    id=str(row[0]),
                    title=row[1],
                    content=row[2],
                    content_length=row[3],
                    size_category="medium",
                )
            )

        # Fetch large articles
        cur.execute(
            """
            SELECT id, title, content, LENGTH(content) as content_length
            FROM articles
            WHERE deleted_at IS NULL
              AND LENGTH(content) >= %s
            ORDER BY created_at DESC
            LIMIT %s
            """,
            (SIZE_MEDIUM_MAX, config.large_count),
        )
        for row in cur.fetchall():
            articles.append(
                Article(
                    id=str(row[0]),
                    title=row[1],
                    content=row[2],
                    content_length=row[3],
                    size_category="large",
                )
            )

    return articles


def build_summary_prompt(content: str) -> str:
    """Build summary prompt for article content."""
    return f"""あなたは熟練したニュース編集者です。
以下の記事を読み、重要なポイントを日本語で要約してください。

### 出力仕様
- 3〜5個の箇条書きで要約
- 各箇条書きは60〜120文字程度
- 事実のみを記載し、推測や意見は含めない
- 出力は日本語のみ

### 記事内容
\"\"\"
{content}
\"\"\"

### 要約
"""


async def call_ollama(
    session: aiohttp.ClientSession,
    config: BenchmarkConfig,
    prompt: str,
) -> dict[str, Any]:
    """Call Ollama API and return full response with metrics."""
    url = f"{config.ollama_url}/api/generate"
    payload = {
        "model": config.model,
        "prompt": prompt,
        "stream": False,
        "options": {
            "num_ctx": config.num_ctx,
            "num_predict": 1200,
            "num_batch": 1024,
            "temperature": 0.15,
            "top_p": 0.85,
            "top_k": 40,
            "repeat_penalty": 1.15,
            "stop": ["<turn|>"],
        },
        "keep_alive": "30m",
    }

    try:
        async with session.post(
            url,
            json=payload,
            timeout=aiohttp.ClientTimeout(total=config.timeout_seconds),
        ) as response:
            if response.status != 200:
                error_text = await response.text()
                raise RuntimeError(
                    f"Ollama API error: {response.status} - {error_text}"
                )
            return await response.json()
    except aiohttp.ClientError as e:
        raise RuntimeError(f"Connection error: {type(e).__name__}: {e}") from e
    except asyncio.TimeoutError as e:
        raise RuntimeError(f"Request timeout after {config.timeout_seconds}s") from e


def extract_metrics(
    response: dict[str, Any],
    article: Article,
    iteration: int,
) -> BenchmarkResult:
    """Extract and calculate metrics from Ollama response."""
    # Extract raw durations (nanoseconds to seconds)
    total_ns = response.get("total_duration", 0)
    load_ns = response.get("load_duration", 0)
    prompt_eval_ns = response.get("prompt_eval_duration", 0)
    eval_ns = response.get("eval_duration", 0)

    total_s = total_ns / 1e9
    load_s = load_ns / 1e9
    prompt_eval_s = prompt_eval_ns / 1e9
    eval_s = eval_ns / 1e9

    # Extract token counts
    prompt_tokens = response.get("prompt_eval_count", 0)
    completion_tokens = response.get("eval_count", 0)

    # Calculate derived metrics
    ttft_s = load_s + prompt_eval_s
    prefill_tok_per_sec = prompt_tokens / prompt_eval_s if prompt_eval_s > 0 else 0
    decode_tok_per_sec = completion_tokens / eval_s if eval_s > 0 else 0

    return BenchmarkResult(
        article_id=article.id,
        title=article.title[:50] + "..." if len(article.title) > 50 else article.title,
        content_chars=article.content_length,
        size_category=article.size_category,
        iteration=iteration,
        prompt_tokens=prompt_tokens,
        completion_tokens=completion_tokens,
        total_duration_s=total_s,
        load_duration_s=load_s,
        prompt_eval_duration_s=prompt_eval_s,
        eval_duration_s=eval_s,
        ttft_s=ttft_s,
        prefill_tok_per_sec=prefill_tok_per_sec,
        decode_tok_per_sec=decode_tok_per_sec,
        success=True,
        model=response.get("model", ""),
        timestamp=datetime.now().isoformat(),
    )


def percentile(data: list[float], p: float) -> float:
    """Calculate percentile value."""
    if not data:
        return 0
    sorted_data = sorted(data)
    k = (len(sorted_data) - 1) * p / 100
    f = int(k)
    c = f + 1 if f + 1 < len(sorted_data) else f
    return sorted_data[f] + (sorted_data[c] - sorted_data[f]) * (k - f)


def calculate_stats(results: list[BenchmarkResult]) -> BenchmarkStats:
    """Calculate statistical summary from benchmark results."""
    successful = [r for r in results if r.success]
    errors = [r for r in results if not r.success]

    if not successful:
        return BenchmarkStats(
            total_iterations=len(results),
            success_count=0,
            error_count=len(errors),
            by_category={},
            ttft_mean=0,
            ttft_p50=0,
            ttft_p95=0,
            ttft_min=0,
            ttft_max=0,
            decode_mean=0,
            decode_p50=0,
            decode_p95=0,
            decode_min=0,
            decode_max=0,
            prefill_mean=0,
            prefill_p50=0,
            prefill_p95=0,
            latency_mean=0,
            latency_p50=0,
            latency_p95=0,
        )

    # Overall stats
    ttft_values = [r.ttft_s for r in successful]
    decode_values = [r.decode_tok_per_sec for r in successful]
    prefill_values = [r.prefill_tok_per_sec for r in successful]
    latency_values = [r.total_duration_s for r in successful]

    # Stats by category
    by_category: dict[str, dict[str, float]] = {}
    for category in ["small", "medium", "large"]:
        cat_results = [r for r in successful if r.size_category == category]
        if cat_results:
            cat_ttft = [r.ttft_s for r in cat_results]
            cat_decode = [r.decode_tok_per_sec for r in cat_results]
            cat_latency = [r.total_duration_s for r in cat_results]
            by_category[category] = {
                "count": len(cat_results),
                "ttft_p50": percentile(cat_ttft, 50),
                "decode_p50": percentile(cat_decode, 50),
                "latency_p50": percentile(cat_latency, 50),
            }

    return BenchmarkStats(
        total_iterations=len(results),
        success_count=len(successful),
        error_count=len(errors),
        by_category=by_category,
        ttft_mean=statistics.mean(ttft_values),
        ttft_p50=percentile(ttft_values, 50),
        ttft_p95=percentile(ttft_values, 95),
        ttft_min=min(ttft_values),
        ttft_max=max(ttft_values),
        decode_mean=statistics.mean(decode_values),
        decode_p50=percentile(decode_values, 50),
        decode_p95=percentile(decode_values, 95),
        decode_min=min(decode_values),
        decode_max=max(decode_values),
        prefill_mean=statistics.mean(prefill_values),
        prefill_p50=percentile(prefill_values, 50),
        prefill_p95=percentile(prefill_values, 95),
        latency_mean=statistics.mean(latency_values),
        latency_p50=percentile(latency_values, 50),
        latency_p95=percentile(latency_values, 95),
    )


async def run_benchmark(
    config: BenchmarkConfig,
    articles: list[Article],
    iterations: int,
) -> tuple[list[BenchmarkResult], BenchmarkStats]:
    """Run benchmark for all articles."""
    print(f"\n{'=' * 60}")
    print("Benchmark Configuration")
    print("=" * 60)
    print(f"Model: {config.model}")
    print(f"Context: {config.num_ctx}")
    print(f"Ollama URL: {config.ollama_url}")
    print(f"Articles: {len(articles)}")
    print(f"Iterations per article: {iterations}")
    print(f"Total runs: {len(articles) * iterations}")
    print("=" * 60)

    # Show article distribution
    print("\nArticle Distribution:")
    for category in ["small", "medium", "large"]:
        cat_articles = [a for a in articles if a.size_category == category]
        if cat_articles:
            avg_len = sum(a.content_length for a in cat_articles) // len(cat_articles)
            print(f"  {category}: {len(cat_articles)} articles (avg {avg_len:,} chars)")

    results: list[BenchmarkResult] = []

    async with aiohttp.ClientSession() as session:
        # Warmup
        if config.warmup_iterations > 0 and articles:
            print(f"\nWarmup ({config.warmup_iterations} iteration(s))...")
            warmup_article = articles[0]
            prompt = build_summary_prompt(warmup_article.content)
            for i in range(config.warmup_iterations):
                try:
                    await call_ollama(session, config, prompt)
                    print(f"  Warmup {i + 1} complete")
                except Exception as e:
                    print(f"  Warmup {i + 1} failed: {e}")

        # Benchmark iterations
        print("\nRunning benchmark...")
        total_runs = len(articles) * iterations
        run_count = 0

        for iteration in range(1, iterations + 1):
            print(f"\n--- Iteration {iteration}/{iterations} ---")
            for article in articles:
                run_count += 1
                prompt = build_summary_prompt(article.content)

                try:
                    response = await call_ollama(session, config, prompt)
                    result = extract_metrics(response, article, iteration)
                    results.append(result)

                    print(
                        f"  [{run_count}/{total_runs}] "
                        f"[{article.size_category}] "
                        f"TTFT: {result.ttft_s:.2f}s, "
                        f"Decode: {result.decode_tok_per_sec:.1f} tok/s, "
                        f"Total: {result.total_duration_s:.2f}s"
                    )

                except Exception as e:
                    error_result = BenchmarkResult(
                        article_id=article.id,
                        title=article.title[:50],
                        content_chars=article.content_length,
                        size_category=article.size_category,
                        iteration=iteration,
                        prompt_tokens=0,
                        completion_tokens=0,
                        total_duration_s=0,
                        load_duration_s=0,
                        prompt_eval_duration_s=0,
                        eval_duration_s=0,
                        ttft_s=0,
                        prefill_tok_per_sec=0,
                        decode_tok_per_sec=0,
                        success=False,
                        error=str(e),
                        timestamp=datetime.now().isoformat(),
                    )
                    results.append(error_result)
                    print(f"  [{run_count}/{total_runs}] ERROR: {e}")

    stats = calculate_stats(results)
    return results, stats


def print_stats(stats: BenchmarkStats) -> None:
    """Print formatted statistics."""
    print(f"\n{'=' * 60}")
    print("RESULTS SUMMARY")
    print("=" * 60)
    print(
        f"Total: {stats.total_iterations} "
        f"(Success: {stats.success_count}, Errors: {stats.error_count})"
    )

    print("\n--- By Size Category ---")
    print(
        f"{'Category':<10} {'Count':>6} {'TTFT P50':>10} {'Decode P50':>12} {'Latency P50':>12}"
    )
    print("-" * 52)
    for category, cat_stats in stats.by_category.items():
        print(
            f"{category:<10} "
            f"{int(cat_stats['count']):>6} "
            f"{cat_stats['ttft_p50']:>9.2f}s "
            f"{cat_stats['decode_p50']:>10.1f} tok/s "
            f"{cat_stats['latency_p50']:>10.2f}s"
        )

    print("\n--- Overall Statistics ---")
    print("\nTTFT (Time To First Token):")
    print(
        f"  Mean: {stats.ttft_mean:.2f}s, P50: {stats.ttft_p50:.2f}s, P95: {stats.ttft_p95:.2f}s"
    )
    print(f"  Min: {stats.ttft_min:.2f}s, Max: {stats.ttft_max:.2f}s")

    print("\nDecode Speed (tokens/sec):")
    print(
        f"  Mean: {stats.decode_mean:.1f}, P50: {stats.decode_p50:.1f}, P95: {stats.decode_p95:.1f}"
    )
    print(f"  Min: {stats.decode_min:.1f}, Max: {stats.decode_max:.1f}")

    print("\nPrefill Speed (tokens/sec):")
    print(
        f"  Mean: {stats.prefill_mean:.1f}, P50: {stats.prefill_p50:.1f}, P95: {stats.prefill_p95:.1f}"
    )

    print("\nTotal Latency:")
    print(
        f"  Mean: {stats.latency_mean:.2f}s, P50: {stats.latency_p50:.2f}s, P95: {stats.latency_p95:.2f}s"
    )

    # Evaluation against targets
    print(f"\n{'=' * 60}")
    print("EVALUATION vs TARGETS")
    print("=" * 60)
    targets = {
        "ttft": 2.0,  # < 2 seconds
        "decode": 40.0,  # > 40 tok/s
    }
    ttft_ok = "✅ PASS" if stats.ttft_p50 < targets["ttft"] else "❌ FAIL"
    decode_ok = "✅ PASS" if stats.decode_p50 > targets["decode"] else "❌ FAIL"
    print(f"  TTFT < 2s: {ttft_ok} ({stats.ttft_p50:.2f}s)")
    print(f"  Decode > 40 tok/s: {decode_ok} ({stats.decode_p50:.1f} tok/s)")


def save_results(
    config: BenchmarkConfig,
    articles: list[Article],
    results: list[BenchmarkResult],
    stats: BenchmarkStats,
    output_dir: str,
) -> str:
    """Save results to JSON file."""
    output_path = Path(output_dir)
    output_path.mkdir(parents=True, exist_ok=True)

    timestamp = datetime.now().strftime("%Y%m%d_%H%M%S")
    output_file = output_path / f"benchmark_real_articles_{timestamp}.json"

    output_data = {
        "timestamp": datetime.now().isoformat(),
        "config": {
            "model": config.model,
            "num_ctx": config.num_ctx,
            "ollama_url": config.ollama_url,
            "iterations": len(results) // len(articles) if articles else 0,
            "article_count": len(articles),
        },
        "articles": [
            {
                "id": a.id,
                "title": a.title,
                "content_chars": a.content_length,
                "size_category": a.size_category,
            }
            for a in articles
        ],
        "results": [asdict(r) for r in results],
        "summary": {
            "total_iterations": stats.total_iterations,
            "success_count": stats.success_count,
            "error_count": stats.error_count,
            "by_category": stats.by_category,
            "ttft_p50": round(stats.ttft_p50, 3),
            "ttft_p95": round(stats.ttft_p95, 3),
            "decode_p50": round(stats.decode_p50, 1),
            "decode_p95": round(stats.decode_p95, 1),
            "latency_p50": round(stats.latency_p50, 2),
            "latency_p95": round(stats.latency_p95, 2),
        },
    }

    with open(output_file, "w", encoding="utf-8") as f:
        json.dump(output_data, f, indent=2, ensure_ascii=False)

    print(f"\nResults saved to: {output_file}")
    return str(output_file)


async def main() -> None:
    """Main entry point."""
    parser = argparse.ArgumentParser(
        description="Benchmark news-creator Summary performance with real articles"
    )
    parser.add_argument(
        "--iterations",
        type=int,
        default=10,
        help="Number of iterations per article (default: 10)",
    )
    parser.add_argument(
        "--warmup",
        type=int,
        default=1,
        help="Number of warmup iterations (default: 1)",
    )
    parser.add_argument(
        "--ollama-url",
        default="http://localhost:11434",
        help="Ollama API URL (default: http://localhost:11434)",
    )
    parser.add_argument(
        "--model",
        default="gemma4-e4b-12k",
        help="Model name (default: gemma4-e4b-12k)",
    )
    parser.add_argument(
        "--num-ctx",
        type=int,
        default=12288,
        help="Context window size (default: 12288)",
    )
    parser.add_argument(
        "--output-dir",
        default="benchmark_results",
        help="Output directory for results (default: benchmark_results)",
    )
    parser.add_argument(
        "--timeout",
        type=int,
        default=300,
        help="Timeout in seconds (default: 300)",
    )
    parser.add_argument(
        "--small-count",
        type=int,
        default=3,
        help="Number of small articles (<3K chars) (default: 3)",
    )
    parser.add_argument(
        "--medium-count",
        type=int,
        default=4,
        help="Number of medium articles (3K-10K chars) (default: 4)",
    )
    parser.add_argument(
        "--large-count",
        type=int,
        default=3,
        help="Number of large articles (>10K chars) (default: 3)",
    )

    args = parser.parse_args()

    config = BenchmarkConfig(
        ollama_url=args.ollama_url,
        model=args.model,
        num_ctx=args.num_ctx,
        timeout_seconds=args.timeout,
        warmup_iterations=args.warmup,
        output_dir=args.output_dir,
        small_count=args.small_count,
        medium_count=args.medium_count,
        large_count=args.large_count,
    )

    # Connect to database
    print("Connecting to database...")
    try:
        conn = get_db_connection()
    except Exception as e:
        print(f"Error: Failed to connect to database: {e}", file=sys.stderr)
        print("\nMake sure to set environment variables:", file=sys.stderr)
        print("  DB_HOST, DB_PORT, DB_USER, DB_PASSWORD, DB_NAME", file=sys.stderr)
        sys.exit(1)

    # Fetch articles
    print("Fetching articles...")
    try:
        articles = fetch_articles_by_size(conn, config)
    except Exception as e:
        print(f"Error: Failed to fetch articles: {e}", file=sys.stderr)
        conn.close()
        sys.exit(1)
    finally:
        conn.close()

    if not articles:
        print("Error: No articles found in database", file=sys.stderr)
        sys.exit(1)

    print(f"Fetched {len(articles)} articles")

    # Run benchmark
    results, stats = await run_benchmark(config, articles, args.iterations)

    # Print results
    print_stats(stats)

    # Save results
    save_results(config, articles, results, stats, config.output_dir)


if __name__ == "__main__":
    asyncio.run(main())
