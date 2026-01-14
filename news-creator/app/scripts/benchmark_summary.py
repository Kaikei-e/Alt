#!/usr/bin/env python3
"""
Benchmark script for news-creator Summary performance.

Measures:
- TTFT (Time To First Token): load_duration + prompt_eval_duration
- Decode speed: eval_count / eval_duration
- Prefill speed: prompt_eval_count / prompt_eval_duration
- Total latency: total_duration
- OOM occurrence rate

Usage:
    cd news-creator/app
    uv run python scripts/benchmark_summary.py --iterations 10
    uv run python scripts/benchmark_summary.py --case small --iterations 5
"""

import argparse
import asyncio
import json
import statistics
import sys
import time
from dataclasses import asdict, dataclass, field
from datetime import datetime
from pathlib import Path
from typing import Any, Optional
from uuid import uuid4

import aiohttp


@dataclass
class BenchmarkResult:
    """Result of a single benchmark run."""

    case_name: str
    iteration: int
    input_chars: int
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
    error: Optional[str] = None
    model: str = ""
    timestamp: str = ""


@dataclass
class BenchmarkStats:
    """Statistical summary for a benchmark case."""

    case_name: str
    iterations: int
    success_count: int
    error_count: int

    # TTFT stats
    ttft_mean: float
    ttft_p50: float
    ttft_p95: float
    ttft_min: float
    ttft_max: float

    # Decode speed stats
    decode_mean: float
    decode_p50: float
    decode_p95: float
    decode_min: float
    decode_max: float

    # Prefill speed stats
    prefill_mean: float
    prefill_p50: float
    prefill_p95: float

    # Total latency stats
    latency_mean: float
    latency_p50: float
    latency_p95: float


@dataclass
class BenchmarkConfig:
    """Configuration for benchmark run."""

    ollama_url: str = "http://localhost:11434"
    model_16k: str = "gemma3-4b-16k"
    model_80k: str = "gemma3-4b-80k"
    timeout_seconds: int = 300
    warmup_iterations: int = 1
    output_dir: str = "benchmark_results"


# Test cases with varying input sizes
TEST_CASES = {
    "small": {
        "description": "Small input (~2,000 chars)",
        "clusters": 3,
        "sentences_per_cluster": 3,
        "expected_model": "16k",
    },
    "medium": {
        "description": "Medium input (~10,000 chars)",
        "clusters": 8,
        "sentences_per_cluster": 5,
        "expected_model": "16k",
    },
    "large": {
        "description": "Large input (~50,000 chars)",
        "clusters": 15,
        "sentences_per_cluster": 10,
        "expected_model": "16k",
    },
    "xl": {
        "description": "XL input (~150,000 chars, triggers 80K)",
        "clusters": 40,
        "sentences_per_cluster": 15,
        "expected_model": "80k",
    },
}

# Sample Japanese tech news sentences for realistic benchmarks
SAMPLE_SENTENCES = [
    "OpenAIは新しいGPT-5モデルを発表し、推論能力が大幅に向上したことを明らかにした。",
    "Googleは量子コンピュータの新しいマイルストーンを達成し、1000量子ビットの安定動作を実現した。",
    "Metaは次世代VRヘッドセットを発表し、解像度が従来比2倍に向上したと報告した。",
    "Amazonはクラウドサービスの新料金体系を発表し、中小企業向けの割引プランを導入した。",
    "Microsoftは新しいAI搭載のOffice機能を発表し、文書作成の効率が50%向上すると主張した。",
    "NVIDIAは最新のH200 GPUを発表し、AI推論性能が前世代比3倍に向上した。",
    "Appleは新しいM4チップを発表し、電力効率が40%向上したことを明らかにした。",
    "Teslaは完全自動運転のベータ版を一般公開し、対応車両が100万台を突破した。",
    "IBMは量子耐性暗号の新規格を提案し、2030年までの標準化を目指している。",
    "Intelは次世代プロセッサの製造プロセスを発表し、2nmプロセスへの移行を発表した。",
    "ソニーは新しいゲーミングヘッドセットを発表し、3Dオーディオ機能を強化した。",
    "任天堂は次世代ゲーム機の開発を示唆し、2025年の発売を計画していると報じられた。",
    "楽天は新しいモバイル決済サービスを開始し、手数料無料キャンペーンを実施している。",
    "LINEは新しいAIチャットボット機能を発表し、カスタマーサポートの自動化を支援する。",
    "サイバーエージェントは広告配信AIを刷新し、ターゲティング精度が25%向上した。",
]


def generate_test_input(case_config: dict) -> tuple[str, int]:
    """Generate test input for a benchmark case.

    Returns:
        Tuple of (cluster_section_text, character_count)
    """
    clusters = case_config["clusters"]
    sentences_per_cluster = case_config["sentences_per_cluster"]

    lines = []
    for cluster_id in range(clusters):
        lines.append(f"## クラスタ {cluster_id + 1}")
        for sent_idx in range(sentences_per_cluster):
            sentence = SAMPLE_SENTENCES[(cluster_id + sent_idx) % len(SAMPLE_SENTENCES)]
            # Add some variation
            if sent_idx == 0:
                lines.append(f"- [代表] {sentence}")
            else:
                lines.append(f"- {sentence}")
        lines.append("")

    text = "\n".join(lines)
    return text, len(text)


def build_prompt(genre: str, cluster_section: str) -> str:
    """Build the recap summary prompt (simplified version of jinja template)."""
    prompt = f"""あなたは熟練したニュース編集者です。
以下の入力だけを根拠に、ジャンル {genre} で直近に起きた
主な出来事・技術トレンドを要約してください。

### 出力仕様
- 出力は JSON オブジェクト 1 つのみ。
- 形式:
{{
  "title": "このジャンル要約の短く魅力的なタイトル (最大50文字、日本語)",
  "bullets": [
    "... [1]"
  ],
  "language": "ja",
  "references": [
    {{"id": 1, "url": "https://example.com", "domain": "example.com", "article_id": "..."}}
  ]
}}
- "bullets" は **3〜7 個**。
- 各 bullet の末尾に **出典リンク [n] を必須で付ける**（n は references の id に対応）。
- 各 bullet は:
  - 1〜2文の完結した日本語
  - 60〜160文字程度
  - 「誰/何が」「何をした・どう変えた」「狙い/影響」の少なくとも2要素を含む

### 共通制約
- 入力にない事実・数値・因果関係は作らない。
- 会話文・挨拶・説明文は書かない。出力はすべて日本語。

### 入力
(クラスタ化されたトピック。各クラスタは1つのトピックを表す):

\"\"\"
{cluster_section}
\"\"\"

### 出力
上記の仕様をすべて満たす JSON オブジェクトのみを出力してください。
"""
    return prompt


async def call_ollama(
    session: aiohttp.ClientSession,
    config: BenchmarkConfig,
    model: str,
    prompt: str,
) -> dict[str, Any]:
    """Call Ollama API and return full response with metrics."""
    url = f"{config.ollama_url}/api/generate"
    payload = {
        "model": model,
        "prompt": prompt,
        "stream": False,
        "options": {
            "num_ctx": 16384 if "16k" in model else 81920,
            "num_predict": 1200,
            "num_batch": 1024,
            "temperature": 0.15,
            "top_p": 0.85,
            "top_k": 40,
            "repeat_penalty": 1.15,
            "stop": ["<end_of_turn>"],
        },
        "keep_alive": "30m",
    }

    async with session.post(
        url, json=payload, timeout=aiohttp.ClientTimeout(total=config.timeout_seconds)
    ) as response:
        if response.status != 200:
            error_text = await response.text()
            raise RuntimeError(f"Ollama API error: {response.status} - {error_text}")
        return await response.json()


def extract_metrics(response: dict[str, Any], input_chars: int) -> BenchmarkResult:
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
        case_name="",  # Set by caller
        iteration=0,  # Set by caller
        input_chars=input_chars,
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


def calculate_stats(results: list[BenchmarkResult]) -> BenchmarkStats:
    """Calculate statistical summary from benchmark results."""
    successful = [r for r in results if r.success]
    errors = [r for r in results if not r.success]

    if not successful:
        return BenchmarkStats(
            case_name=results[0].case_name if results else "unknown",
            iterations=len(results),
            success_count=0,
            error_count=len(errors),
            ttft_mean=0, ttft_p50=0, ttft_p95=0, ttft_min=0, ttft_max=0,
            decode_mean=0, decode_p50=0, decode_p95=0, decode_min=0, decode_max=0,
            prefill_mean=0, prefill_p50=0, prefill_p95=0,
            latency_mean=0, latency_p50=0, latency_p95=0,
        )

    ttft_values = [r.ttft_s for r in successful]
    decode_values = [r.decode_tok_per_sec for r in successful]
    prefill_values = [r.prefill_tok_per_sec for r in successful]
    latency_values = [r.total_duration_s for r in successful]

    def percentile(data: list[float], p: float) -> float:
        if not data:
            return 0
        sorted_data = sorted(data)
        k = (len(sorted_data) - 1) * p / 100
        f = int(k)
        c = f + 1 if f + 1 < len(sorted_data) else f
        return sorted_data[f] + (sorted_data[c] - sorted_data[f]) * (k - f)

    return BenchmarkStats(
        case_name=results[0].case_name,
        iterations=len(results),
        success_count=len(successful),
        error_count=len(errors),
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
    case_name: str,
    iterations: int,
) -> tuple[list[BenchmarkResult], BenchmarkStats]:
    """Run benchmark for a single test case."""
    case_config = TEST_CASES[case_name]
    model = config.model_16k if case_config["expected_model"] == "16k" else config.model_80k

    print(f"\n{'=' * 60}")
    print(f"Case: {case_name} - {case_config['description']}")
    print(f"Model: {model}")
    print(f"Iterations: {iterations}")
    print("=" * 60)

    # Generate test input
    cluster_section, input_chars = generate_test_input(case_config)
    prompt = build_prompt("テクノロジー", cluster_section)

    print(f"Input size: {input_chars:,} chars")
    print(f"Prompt size: {len(prompt):,} chars (~{len(prompt) // 4:,} tokens)")

    results: list[BenchmarkResult] = []

    async with aiohttp.ClientSession() as session:
        # Warmup
        if config.warmup_iterations > 0:
            print(f"\nWarmup ({config.warmup_iterations} iteration(s))...")
            for i in range(config.warmup_iterations):
                try:
                    await call_ollama(session, config, model, prompt)
                    print(f"  Warmup {i + 1} complete")
                except Exception as e:
                    print(f"  Warmup {i + 1} failed: {e}")

        # Benchmark iterations
        print(f"\nRunning {iterations} iterations...")
        for i in range(iterations):
            try:
                response = await call_ollama(session, config, model, prompt)
                result = extract_metrics(response, input_chars)
                result.case_name = case_name
                result.iteration = i + 1
                results.append(result)

                print(
                    f"  [{i + 1}/{iterations}] "
                    f"TTFT: {result.ttft_s:.2f}s, "
                    f"Decode: {result.decode_tok_per_sec:.1f} tok/s, "
                    f"Total: {result.total_duration_s:.2f}s"
                )

            except Exception as e:
                error_result = BenchmarkResult(
                    case_name=case_name,
                    iteration=i + 1,
                    input_chars=input_chars,
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
                print(f"  [{i + 1}/{iterations}] ERROR: {e}")

    stats = calculate_stats(results)
    return results, stats


def print_stats(stats: BenchmarkStats) -> None:
    """Print formatted statistics."""
    print(f"\n{'─' * 60}")
    print(f"Stats for: {stats.case_name}")
    print(f"{'─' * 60}")
    print(f"Iterations: {stats.iterations} (Success: {stats.success_count}, Errors: {stats.error_count})")
    print()
    print("TTFT (Time To First Token):")
    print(f"  Mean: {stats.ttft_mean:.2f}s, P50: {stats.ttft_p50:.2f}s, P95: {stats.ttft_p95:.2f}s")
    print(f"  Min: {stats.ttft_min:.2f}s, Max: {stats.ttft_max:.2f}s")
    print()
    print("Decode Speed (tokens/sec):")
    print(f"  Mean: {stats.decode_mean:.1f}, P50: {stats.decode_p50:.1f}, P95: {stats.decode_p95:.1f}")
    print(f"  Min: {stats.decode_min:.1f}, Max: {stats.decode_max:.1f}")
    print()
    print("Prefill Speed (tokens/sec):")
    print(f"  Mean: {stats.prefill_mean:.1f}, P50: {stats.prefill_p50:.1f}, P95: {stats.prefill_p95:.1f}")
    print()
    print("Total Latency:")
    print(f"  Mean: {stats.latency_mean:.2f}s, P50: {stats.latency_p50:.2f}s, P95: {stats.latency_p95:.2f}s")


def save_results(
    all_results: list[BenchmarkResult],
    all_stats: list[BenchmarkStats],
    output_dir: str,
) -> None:
    """Save results to JSON files."""
    output_path = Path(output_dir)
    output_path.mkdir(parents=True, exist_ok=True)

    timestamp = datetime.now().strftime("%Y%m%d_%H%M%S")

    # Save detailed results
    results_file = output_path / f"benchmark_results_{timestamp}.json"
    with open(results_file, "w") as f:
        json.dump([asdict(r) for r in all_results], f, indent=2, ensure_ascii=False)
    print(f"\nResults saved to: {results_file}")

    # Save stats summary
    stats_file = output_path / f"benchmark_stats_{timestamp}.json"
    with open(stats_file, "w") as f:
        json.dump([asdict(s) for s in all_stats], f, indent=2, ensure_ascii=False)
    print(f"Stats saved to: {stats_file}")


async def main() -> None:
    """Main entry point."""
    parser = argparse.ArgumentParser(description="Benchmark news-creator Summary performance")
    parser.add_argument(
        "--case",
        choices=list(TEST_CASES.keys()) + ["all"],
        default="all",
        help="Test case to run (default: all)",
    )
    parser.add_argument(
        "--iterations",
        type=int,
        default=10,
        help="Number of iterations per case (default: 10)",
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
        "--model-16k",
        default="gemma3-4b-16k",
        help="Model name for 16K context (default: gemma3-4b-16k)",
    )
    parser.add_argument(
        "--model-80k",
        default="gemma3-4b-80k",
        help="Model name for 80K context (default: gemma3-4b-80k)",
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

    args = parser.parse_args()

    config = BenchmarkConfig(
        ollama_url=args.ollama_url,
        model_16k=args.model_16k,
        model_80k=args.model_80k,
        timeout_seconds=args.timeout,
        warmup_iterations=args.warmup,
        output_dir=args.output_dir,
    )

    cases_to_run = list(TEST_CASES.keys()) if args.case == "all" else [args.case]

    print("=" * 60)
    print("News-Creator Summary Benchmark")
    print("=" * 60)
    print(f"Ollama URL: {config.ollama_url}")
    print(f"Models: 16K={config.model_16k}, 80K={config.model_80k}")
    print(f"Cases: {', '.join(cases_to_run)}")
    print(f"Iterations per case: {args.iterations}")
    print(f"Warmup iterations: {config.warmup_iterations}")

    all_results: list[BenchmarkResult] = []
    all_stats: list[BenchmarkStats] = []

    for case_name in cases_to_run:
        results, stats = await run_benchmark(config, case_name, args.iterations)
        all_results.extend(results)
        all_stats.append(stats)
        print_stats(stats)

    # Summary
    print("\n" + "=" * 60)
    print("SUMMARY")
    print("=" * 60)
    print(f"{'Case':<12} {'TTFT P50':>10} {'Decode P50':>12} {'Latency P50':>12} {'Errors':>8}")
    print("-" * 60)
    for stats in all_stats:
        print(
            f"{stats.case_name:<12} "
            f"{stats.ttft_p50:>9.2f}s "
            f"{stats.decode_p50:>10.1f} tok/s "
            f"{stats.latency_p50:>10.2f}s "
            f"{stats.error_count:>8}"
        )

    # Evaluation against targets
    print("\n" + "=" * 60)
    print("EVALUATION vs TARGETS")
    print("=" * 60)
    targets = {
        "ttft": 2.0,  # < 2 seconds
        "decode": 40.0,  # > 40 tok/s
        "prefill": 500.0,  # > 500 tok/s
    }
    for stats in all_stats:
        ttft_ok = "PASS" if stats.ttft_p50 < targets["ttft"] else "FAIL"
        decode_ok = "PASS" if stats.decode_p50 > targets["decode"] else "FAIL"
        prefill_ok = "PASS" if stats.prefill_p50 > targets["prefill"] else "FAIL"
        print(f"{stats.case_name}:")
        print(f"  TTFT < 2s: {ttft_ok} ({stats.ttft_p50:.2f}s)")
        print(f"  Decode > 40 tok/s: {decode_ok} ({stats.decode_p50:.1f} tok/s)")
        print(f"  Prefill > 500 tok/s: {prefill_ok} ({stats.prefill_p50:.1f} tok/s)")

    # Save results
    save_results(all_results, all_stats, config.output_dir)


if __name__ == "__main__":
    asyncio.run(main())
