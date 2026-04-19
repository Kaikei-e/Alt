"""CLI entry point for offline evaluation runs.

Invocation::

    uv run python -m evaluation.run_eval --dataset evaluation/datasets/baseline.jsonl

The runner expects a generation function that takes an EvalCase and returns
``(body, source_map, articles_by_id, evidence_by_short_id)``. In CI, pass a
recorded fixture rather than calling the LLM live.
"""

from __future__ import annotations

import argparse
import json
import sys
from collections.abc import Callable
from pathlib import Path
from typing import Any

from evaluation.dataset import EvalCase, load_cases
from evaluation.metrics import citation_precision, faithfulness, lang_mix_ratio

GenerateFn = Callable[[EvalCase], tuple[str, dict[str, dict], dict[str, dict], dict[str, str]]]
JudgeFn = Callable[[str], float]


def run(cases: list[EvalCase], generate: GenerateFn, judge: JudgeFn | None = None) -> list[dict[str, Any]]:
    """Apply ``generate`` and score each case. Returns one result dict per case."""
    results: list[dict[str, Any]] = []
    for case in cases:
        body, source_map, articles_by_id, evidence = generate(case)
        precision = citation_precision(body, source_map, set(case.gold_source_ids))
        mix = lang_mix_ratio(body, source_map, articles_by_id)
        faith = faithfulness(body, evidence, judge) if judge is not None else None
        results.append(
            {
                "topic": case.topic,
                "query_lang": case.query_lang,
                "expected_lang_mix": case.expected_lang_mix,
                "citation_precision": precision,
                "lang_mix_ratio": mix,
                "faithfulness": faith,
            }
        )
    return results


def _aggregate(results: list[dict[str, Any]]) -> dict[str, Any]:
    """Collapse per-case results into simple aggregates for gating."""
    precisions = [r["citation_precision"] for r in results if r["citation_precision"] is not None]
    faiths = [r["faithfulness"] for r in results if r["faithfulness"] is not None]
    en_share = [r["lang_mix_ratio"].get("en", 0.0) for r in results if r["lang_mix_ratio"]]
    return {
        "cases": len(results),
        "citation_precision_mean": sum(precisions) / len(precisions) if precisions else None,
        "faithfulness_mean": sum(faiths) / len(faiths) if faiths else None,
        "lang_en_share_mean": sum(en_share) / len(en_share) if en_share else None,
    }


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description="Run Acolyte evaluation harness")
    parser.add_argument("--dataset", required=True, help="Path to JSONL dataset")
    parser.add_argument(
        "--output",
        default="-",
        help="Write JSON output to this path (default: stdout)",
    )
    args = parser.parse_args(argv)

    cases = load_cases(args.dataset)

    def _not_wired(_case: EvalCase) -> tuple[str, dict[str, dict], dict[str, dict], dict[str, str]]:
        raise RuntimeError(
            "run_eval CLI is a scaffold; wire a real generator (Acolyte pipeline or recorded fixture) before using."
        )

    results = run(cases, _not_wired)
    payload = {"results": results, "summary": _aggregate(results)}
    text = json.dumps(payload, ensure_ascii=False, indent=2)
    if args.output == "-":
        sys.stdout.write(text + "\n")
    else:
        Path(args.output).write_text(text + "\n", encoding="utf-8")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
