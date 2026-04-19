"""CLI entry point for offline evaluation runs.

Invocation::

    uv run python -m evaluation.run_eval \\
        --dataset evaluation/datasets/baseline.jsonl \\
        --generator fixture \\
        --fixtures evaluation/fixtures/2026-04-19 \\
        --output evaluation/results/ci.json

The runner expects a generation function that takes an EvalCase and returns
``(body, source_map, articles_by_id, evidence_by_short_id)``.

Supported modes:

- ``--generator fixture``: deterministic, loads JSON files from
  ``--fixtures``. Used by CI.
- ``--generator db-replay``: reconstructs past report output directly from
  the acolyte / alt databases. Used to measure the current baseline
  without regenerating reports. Requires ``ACOLYTE_DB_DSN`` and
  ``ARTICLES_DB_DSN`` env vars.
- ``--generator scaffold`` (default): raises a RuntimeError. Kept as the
  explicit "you have not wired a generator yet" signal.
"""

from __future__ import annotations

import argparse
import hashlib
import json
import os
import sys
from collections.abc import Callable
from datetime import UTC, datetime
from pathlib import Path
from typing import Any

from evaluation.dataset import EvalCase, load_cases
from evaluation.metrics import citation_precision, faithfulness, lang_mix_ratio

GenerateFn = Callable[[EvalCase], tuple[str, dict[str, dict], dict[str, dict], dict[str, str]]]
JudgeFn = Callable[[str], float]


def run(
    cases: list[EvalCase],
    generate: GenerateFn,
    judge: JudgeFn | None = None,
) -> list[dict[str, Any]]:
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


def _dataset_digest(path: Path) -> str:
    """SHA-256 of the dataset file, used as tamper-evidence in the results."""
    return hashlib.sha256(path.read_bytes()).hexdigest()


def _build_generator(args: argparse.Namespace) -> GenerateFn:
    if args.generator == "fixture":
        if not args.fixtures:
            raise SystemExit("--generator fixture requires --fixtures <dir>")
        from evaluation.generators.recorded_fixture import RecordedFixtureGenerator

        return RecordedFixtureGenerator(Path(args.fixtures))

    if args.generator == "db-replay":
        import psycopg

        acolyte_dsn = os.environ.get("ACOLYTE_DB_DSN", "")
        alt_dsn = os.environ.get("ARTICLES_DB_DSN", "")
        if not acolyte_dsn or not alt_dsn:
            raise SystemExit(
                "db-replay requires ACOLYTE_DB_DSN and ARTICLES_DB_DSN env vars",
            )
        from evaluation.generators.db_replay import DBReplayGenerator

        acolyte_conn = psycopg.connect(acolyte_dsn)
        alt_conn = psycopg.connect(alt_dsn)
        return DBReplayGenerator(acolyte_conn, alt_conn, section_key=args.section_key)

    def _scaffold(_case: EvalCase) -> tuple[str, dict, dict, dict]:
        raise RuntimeError(
            "run_eval CLI: no generator wired. Pass --generator fixture|db-replay or plug in a live generator.",
        )

    return _scaffold


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description="Run Acolyte evaluation harness")
    parser.add_argument("--dataset", required=True, help="Path to JSONL dataset")
    parser.add_argument(
        "--generator",
        choices=["fixture", "db-replay", "scaffold"],
        default="scaffold",
        help="How to obtain report outputs per case",
    )
    parser.add_argument("--fixtures", default="", help="Fixture directory for --generator fixture")
    parser.add_argument(
        "--section-key",
        default="analysis",
        help="report_section_versions.section_key for --generator db-replay",
    )
    parser.add_argument(
        "--output",
        default="-",
        help="Write JSON output to this path (default: stdout)",
    )
    args = parser.parse_args(argv)

    dataset_path = Path(args.dataset)
    cases = load_cases(dataset_path)
    generator = _build_generator(args)

    results = run(cases, generator)
    payload = {
        "metadata": {
            "dataset": str(dataset_path),
            "dataset_sha256": _dataset_digest(dataset_path),
            "generator": args.generator,
            "section_key": args.section_key,
            "generated_at": datetime.now(UTC).isoformat(),
        },
        "results": results,
        "summary": _aggregate(results),
    }
    text = json.dumps(payload, ensure_ascii=False, indent=2)
    if args.output == "-":
        sys.stdout.write(text + "\n")
    else:
        Path(args.output).write_text(text + "\n", encoding="utf-8")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
