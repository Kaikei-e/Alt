#!/usr/bin/env python3
"""
Benchmark script for gpt-oss:20b (CPU) model performance.

Measures:
- TTFT (Time To First Token)
- Decode speed (tokens/sec)
- Prefill speed (tokens/sec)
- Load duration (cold start indicator)
- Total duration (E2E latency)

Usage:
    python benchmark_augur.py --ollama-url http://localhost:11435 --iterations 10
"""

import argparse
import json
import statistics
import sys
import time
from dataclasses import dataclass, field
from datetime import datetime, timezone
from typing import Any

import httpx

# Synthetic queries for benchmarking
QUERIES = {
    "short": {
        "prompt": "What is RSS?",
        "description": "Short query (~50 tokens)",
        "expected_context_tokens": 50,
    },
    "medium": {
        "prompt": """Summarize the key points of the following article about machine learning:

Machine learning is a subset of artificial intelligence that focuses on building systems that learn from data. Unlike traditional programming where rules are explicitly coded, machine learning algorithms improve through experience. The three main types are supervised learning, unsupervised learning, and reinforcement learning. Each has distinct applications and methodologies.""",
        "description": "Medium query (~200 tokens)",
        "expected_context_tokens": 200,
    },
    "long": {
        "prompt": """Analyze the following comprehensive text about climate change and provide a detailed summary:

Climate change represents one of the most significant challenges facing humanity in the 21st century. The Earth's climate has changed throughout history, but the current warming trend is particularly significant because it is clearly the result of human activities since the mid-20th century and is proceeding at an unprecedented rate.

The primary cause of current climate change is the emission of greenhouse gases, particularly carbon dioxide from burning fossil fuels. Since the Industrial Revolution, atmospheric CO2 levels have increased by more than 50%, from about 280 parts per million to over 420 ppm today. This increase traps more heat in the atmosphere, leading to a cascade of effects.

Global average temperatures have risen by approximately 1.1 degrees Celsius since the pre-industrial era. While this may seem small, the impacts are already visible: more frequent and intense heat waves, changing precipitation patterns, rising sea levels, and shifting ecosystems. Arctic sea ice is declining at a rate of about 13% per decade, and many glaciers worldwide are retreating.

The consequences extend beyond environmental changes. Climate change affects agriculture, water resources, human health, and economic stability. Extreme weather events cause billions of dollars in damage annually. Low-lying coastal areas face existential threats from rising seas, potentially displacing millions of people by the end of the century.""",
        "description": "Long query (~500 tokens)",
        "expected_context_tokens": 500,
    },
}


@dataclass
class BenchmarkResult:
    """Single benchmark iteration result."""

    query_type: str
    ttft_seconds: float
    decode_speed: float  # tokens/sec
    prefill_speed: float | None  # tokens/sec
    load_duration_seconds: float | None
    total_duration_seconds: float
    prompt_eval_count: int | None
    eval_count: int | None
    response_length: int
    success: bool
    error: str | None = None


@dataclass
class BenchmarkStatistics:
    """Statistics for a query type."""

    mean: float
    median: float
    p50: float
    p95: float
    min: float
    max: float
    std: float


@dataclass
class QueryStatistics:
    """All statistics for a query type."""

    ttft: BenchmarkStatistics | None = None
    decode_speed: BenchmarkStatistics | None = None
    prefill_speed: BenchmarkStatistics | None = None
    load_duration: BenchmarkStatistics | None = None
    total_duration: BenchmarkStatistics | None = None


@dataclass
class BenchmarkReport:
    """Full benchmark report."""

    metadata: dict = field(default_factory=dict)
    statistics: dict[str, QueryStatistics] = field(default_factory=dict)
    raw_results: list[dict] = field(default_factory=list)


def calculate_statistics(values: list[float]) -> BenchmarkStatistics | None:
    """Calculate statistics from a list of values."""
    if not values:
        return None

    sorted_vals = sorted(values)
    n = len(sorted_vals)

    return BenchmarkStatistics(
        mean=statistics.mean(values),
        median=statistics.median(values),
        p50=sorted_vals[int(n * 0.50)] if n > 0 else 0,
        p95=sorted_vals[min(int(n * 0.95), n - 1)] if n > 0 else 0,
        min=min(values),
        max=max(values),
        std=statistics.stdev(values) if len(values) > 1 else 0,
    )


def run_benchmark_iteration(
    client: httpx.Client,
    ollama_url: str,
    model: str,
    prompt: str,
    query_type: str,
    timeout: float = 300.0,
) -> BenchmarkResult:
    """Run a single benchmark iteration using streaming mode."""
    url = f"{ollama_url.rstrip('/')}/api/generate"
    payload = {
        "model": model,
        "prompt": prompt,
        "stream": True,
        "keep_alive": -1,  # Keep model loaded
        "think": False,  # Disable thinking mode for Qwen3
    }

    try:
        request_start = time.perf_counter()
        first_token_time: float | None = None
        response_text = ""
        final_metrics: dict[str, Any] = {}

        with client.stream("POST", url, json=payload, timeout=timeout) as response:
            response.raise_for_status()

            for line in response.iter_lines():
                if not line:
                    continue

                chunk = json.loads(line)

                # Record TTFT on first token
                if first_token_time is None and chunk.get("response"):
                    first_token_time = time.perf_counter()

                # Accumulate response
                response_text += chunk.get("response", "")

                # Capture final metrics when done
                if chunk.get("done"):
                    final_metrics = chunk
                    break

        request_end = time.perf_counter()

        # Extract metrics from final chunk
        eval_count = final_metrics.get("eval_count")
        eval_duration = final_metrics.get("eval_duration")  # nanoseconds
        prompt_eval_count = final_metrics.get("prompt_eval_count")
        prompt_eval_duration = final_metrics.get("prompt_eval_duration")  # nanoseconds
        load_duration = final_metrics.get("load_duration")  # nanoseconds
        total_duration = final_metrics.get("total_duration")  # nanoseconds

        # Calculate TTFT
        ttft = (first_token_time - request_start) if first_token_time else (request_end - request_start)

        # Calculate decode speed (tokens/sec)
        decode_speed = 0.0
        if eval_count and eval_duration and eval_duration > 0:
            decode_speed = eval_count / (eval_duration / 1e9)

        # Calculate prefill speed (tokens/sec)
        prefill_speed = None
        if prompt_eval_count and prompt_eval_duration and prompt_eval_duration > 0:
            prefill_speed = prompt_eval_count / (prompt_eval_duration / 1e9)

        # Load duration in seconds
        load_duration_seconds = load_duration / 1e9 if load_duration else None

        # Total duration in seconds
        total_duration_seconds = total_duration / 1e9 if total_duration else (request_end - request_start)

        return BenchmarkResult(
            query_type=query_type,
            ttft_seconds=ttft,
            decode_speed=decode_speed,
            prefill_speed=prefill_speed,
            load_duration_seconds=load_duration_seconds,
            total_duration_seconds=total_duration_seconds,
            prompt_eval_count=prompt_eval_count,
            eval_count=eval_count,
            response_length=len(response_text),
            success=True,
        )

    except Exception as e:
        return BenchmarkResult(
            query_type=query_type,
            ttft_seconds=0,
            decode_speed=0,
            prefill_speed=None,
            load_duration_seconds=None,
            total_duration_seconds=0,
            prompt_eval_count=None,
            eval_count=None,
            response_length=0,
            success=False,
            error=str(e),
        )


def run_warmup(
    client: httpx.Client,
    ollama_url: str,
    model: str,
    warmup_iterations: int,
) -> None:
    """Run warmup iterations to load the model into memory."""
    print(f"Running {warmup_iterations} warmup iterations...", flush=True)
    for i in range(warmup_iterations):
        result = run_benchmark_iteration(
            client=client,
            ollama_url=ollama_url,
            model=model,
            prompt="Hello",
            query_type="warmup",
            timeout=300.0,
        )
        status = "OK" if result.success else f"FAILED: {result.error}"
        print(f"  Warmup {i + 1}/{warmup_iterations}: {status}", flush=True)
    print(flush=True)


def run_benchmarks(
    ollama_url: str,
    model: str,
    iterations: int,
    warmup_iterations: int,
    timeout: float = 300.0,
) -> BenchmarkReport:
    """Run all benchmark iterations and collect results."""
    report = BenchmarkReport()
    all_results: list[BenchmarkResult] = []

    with httpx.Client() as client:
        # Warmup
        run_warmup(client, ollama_url, model, warmup_iterations)

        # Run benchmarks for each query type
        for query_type, query_info in QUERIES.items():
            print(f"Benchmarking {query_type} queries ({query_info['description']})...", flush=True)
            query_results: list[BenchmarkResult] = []

            for i in range(iterations):
                result = run_benchmark_iteration(
                    client=client,
                    ollama_url=ollama_url,
                    model=model,
                    prompt=query_info["prompt"],
                    query_type=query_type,
                    timeout=timeout,
                )
                query_results.append(result)
                all_results.append(result)

                if result.success:
                    print(
                        f"  Iteration {i + 1}/{iterations}: "
                        f"TTFT={result.ttft_seconds:.3f}s, "
                        f"decode={result.decode_speed:.1f} tok/s, "
                        f"total={result.total_duration_seconds:.2f}s",
                        flush=True,
                    )
                else:
                    print(f"  Iteration {i + 1}/{iterations}: FAILED - {result.error}", flush=True)

            print(flush=True)

    # Calculate statistics
    for query_type in QUERIES:
        type_results = [r for r in all_results if r.query_type == query_type and r.success]

        if not type_results:
            report.statistics[query_type] = QueryStatistics()
            continue

        ttft_values = [r.ttft_seconds for r in type_results]
        decode_values = [r.decode_speed for r in type_results if r.decode_speed > 0]
        prefill_values = [r.prefill_speed for r in type_results if r.prefill_speed is not None]
        load_values = [r.load_duration_seconds for r in type_results if r.load_duration_seconds is not None]
        total_values = [r.total_duration_seconds for r in type_results]

        report.statistics[query_type] = QueryStatistics(
            ttft=calculate_statistics(ttft_values),
            decode_speed=calculate_statistics(decode_values),
            prefill_speed=calculate_statistics(prefill_values) if prefill_values else None,
            load_duration=calculate_statistics(load_values) if load_values else None,
            total_duration=calculate_statistics(total_values),
        )

    # Store raw results
    report.raw_results = [
        {
            "query_type": r.query_type,
            "ttft_seconds": r.ttft_seconds,
            "decode_speed": r.decode_speed,
            "prefill_speed": r.prefill_speed,
            "load_duration_seconds": r.load_duration_seconds,
            "total_duration_seconds": r.total_duration_seconds,
            "prompt_eval_count": r.prompt_eval_count,
            "eval_count": r.eval_count,
            "response_length": r.response_length,
            "success": r.success,
            "error": r.error,
        }
        for r in all_results
    ]

    return report


def fetch_model_config(ollama_url: str, model: str) -> dict[str, Any]:
    """Fetch model configuration from Ollama."""
    try:
        with httpx.Client() as client:
            response = client.post(
                f"{ollama_url.rstrip('/')}/api/show",
                json={"name": model},
                timeout=30.0,
            )
            if response.status_code == 200:
                data = response.json()
                # Extract parameters from modelfile
                params = {}
                modelfile = data.get("modelfile", "")
                for line in modelfile.split("\n"):
                    if line.startswith("PARAMETER"):
                        parts = line.split()
                        if len(parts) >= 3:
                            key = parts[1]
                            value = parts[2]
                            try:
                                params[key] = int(value)
                            except ValueError:
                                try:
                                    params[key] = float(value)
                                except ValueError:
                                    params[key] = value
                return params
    except Exception:
        pass
    return {}


def serialize_statistics(stats: BenchmarkStatistics | None) -> dict[str, float] | None:
    """Serialize BenchmarkStatistics to dict."""
    if stats is None:
        return None
    return {
        "mean": round(stats.mean, 4),
        "median": round(stats.median, 4),
        "p50": round(stats.p50, 4),
        "p95": round(stats.p95, 4),
        "min": round(stats.min, 4),
        "max": round(stats.max, 4),
        "std": round(stats.std, 4),
    }


def serialize_query_statistics(qs: QueryStatistics) -> dict[str, Any]:
    """Serialize QueryStatistics to dict."""
    return {
        "ttft": serialize_statistics(qs.ttft),
        "decode_speed": serialize_statistics(qs.decode_speed),
        "prefill_speed": serialize_statistics(qs.prefill_speed),
        "load_duration": serialize_statistics(qs.load_duration),
        "total_duration": serialize_statistics(qs.total_duration),
    }


def print_summary(report: BenchmarkReport) -> None:
    """Print benchmark summary to console."""
    print("=" * 60)
    print("BENCHMARK SUMMARY")
    print("=" * 60)
    print(f"Model: {report.metadata.get('model', 'unknown')}")
    print(f"Timestamp: {report.metadata.get('timestamp', 'unknown')}")
    print()

    for query_type, stats in report.statistics.items():
        print(f"--- {query_type.upper()} QUERIES ---")
        if stats.ttft:
            print(f"  TTFT (Time To First Token):")
            print(f"    Mean: {stats.ttft.mean:.3f}s, P50: {stats.ttft.p50:.3f}s, P95: {stats.ttft.p95:.3f}s")
        if stats.decode_speed:
            print(f"  Decode Speed:")
            print(f"    Mean: {stats.decode_speed.mean:.1f} tok/s, P50: {stats.decode_speed.p50:.1f} tok/s")
        if stats.prefill_speed:
            print(f"  Prefill Speed:")
            print(f"    Mean: {stats.prefill_speed.mean:.1f} tok/s, P50: {stats.prefill_speed.p50:.1f} tok/s")
        if stats.total_duration:
            print(f"  Total Duration:")
            print(f"    Mean: {stats.total_duration.mean:.2f}s, P50: {stats.total_duration.p50:.2f}s")
        print()


def main() -> int:
    """Main entry point."""
    parser = argparse.ArgumentParser(
        description="Benchmark gpt-oss:20b (CPU) model performance",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
Example:
    python benchmark_augur.py --ollama-url http://localhost:11435 --iterations 10

Metrics collected:
    TTFT: Time from request start to first token received
    Decode Speed: eval_count / (eval_duration / 1e9) tokens/sec
    Prefill Speed: prompt_eval_count / (prompt_eval_duration / 1e9) tokens/sec
    Load Duration: Model cold start indicator
    Total Duration: End-to-end latency
        """,
    )
    parser.add_argument(
        "--ollama-url",
        type=str,
        default="http://localhost:11435",
        help="Ollama API URL (default: http://localhost:11435)",
    )
    parser.add_argument(
        "--model",
        type=str,
        default="gpt-oss20b-igpu",
        help="Model name (default: gpt-oss20b-igpu)",
    )
    parser.add_argument(
        "--iterations",
        type=int,
        default=10,
        help="Number of benchmark iterations per query type (default: 10)",
    )
    parser.add_argument(
        "--warmup",
        type=int,
        default=2,
        help="Number of warmup iterations (default: 2)",
    )
    parser.add_argument(
        "--timeout",
        type=float,
        default=300.0,
        help="Request timeout in seconds (default: 300)",
    )
    parser.add_argument(
        "--output",
        type=str,
        default="benchmark_results.json",
        help="Output JSON file path (default: benchmark_results.json)",
    )

    args = parser.parse_args()

    print(f"Augur Performance Benchmark", flush=True)
    print(f"Model: {args.model}", flush=True)
    print(f"Ollama URL: {args.ollama_url}", flush=True)
    print(f"Iterations: {args.iterations}", flush=True)
    print(f"Warmup: {args.warmup}", flush=True)
    print(f"Timeout: {args.timeout}s", flush=True)
    print(flush=True)

    # Fetch model config
    model_config = fetch_model_config(args.ollama_url, args.model)
    print(f"Model config: {model_config}", flush=True)
    print(flush=True)

    # Run benchmarks
    report = run_benchmarks(
        ollama_url=args.ollama_url,
        model=args.model,
        iterations=args.iterations,
        warmup_iterations=args.warmup,
        timeout=args.timeout,
    )

    # Add metadata
    report.metadata = {
        "timestamp": datetime.now(timezone.utc).isoformat(),
        "model": args.model,
        "ollama_url": args.ollama_url,
        "iterations": args.iterations,
        "warmup_iterations": args.warmup,
        "timeout_seconds": args.timeout,
        "config": model_config,
    }

    # Print summary
    print_summary(report)

    # Serialize and save report
    output_data = {
        "metadata": report.metadata,
        "statistics": {k: serialize_query_statistics(v) for k, v in report.statistics.items()},
        "raw_results": report.raw_results,
    }

    with open(args.output, "w") as f:
        json.dump(output_data, f, indent=2)

    print(f"Results saved to: {args.output}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
