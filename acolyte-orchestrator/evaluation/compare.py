"""Diff two ``run_eval`` outputs and report per-metric delta.

Invocation::

    uv run python -m evaluation.compare \\
        --before evaluation/results/w1-baseline.json \\
        --after  evaluation/results/w2-phase2.json

Guards against silent dataset tampering (ASI-06 Evaluation Manipulation)
by requiring that both reports reference the *same dataset file* and carry
the same SHA-256 digest. If the digest differs the tool refuses to diff.
"""

from __future__ import annotations

import argparse
import json
import sys
from pathlib import Path
from typing import Any


def _diff_mean(
    before: dict[str, Any], after: dict[str, Any], key: str
) -> tuple[float | None, float | None, float | None]:
    b = before["summary"].get(key)
    a = after["summary"].get(key)
    if b is None or a is None:
        return b, a, None
    return b, a, a - b


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description="Diff two eval run outputs")
    parser.add_argument("--before", required=True)
    parser.add_argument("--after", required=True)
    parser.add_argument(
        "--allow-dataset-drift",
        action="store_true",
        help="permit comparing runs that used different datasets (normally blocked as tamper signal)",
    )
    args = parser.parse_args(argv)

    before = json.loads(Path(args.before).read_text(encoding="utf-8"))
    after = json.loads(Path(args.after).read_text(encoding="utf-8"))

    b_digest = before.get("metadata", {}).get("dataset_sha256")
    a_digest = after.get("metadata", {}).get("dataset_sha256")
    if not args.allow_dataset_drift and b_digest != a_digest:
        sys.stderr.write(
            f"dataset digest differs: before={b_digest} after={a_digest}. "
            "Re-run eval with the same dataset, or pass --allow-dataset-drift.\n",
        )
        return 2

    lines: list[str] = []
    for key in ("citation_precision_mean", "faithfulness_mean", "lang_en_share_mean"):
        b, a, delta = _diff_mean(before, after, key)
        lines.append(f"{key}: before={b} after={a} delta={delta}")
    print("\n".join(lines))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
