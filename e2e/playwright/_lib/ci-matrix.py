#!/usr/bin/env python3
"""Emit the GitHub Actions job matrices for the E2E Playwright workflow.

    python3 e2e/playwright/_lib/ci-matrix.py            # human-readable
    python3 e2e/playwright/_lib/ci-matrix.py --github   # writes $GITHUB_OUTPUT

Three matrices come out of `e2e/playwright/suites.yaml`:

    single        suites that run as one job     -> ["mq-hub", "auth-hub", ...]
    shardedBuild  suites that fan out            -> ["alt-backend"]
    shardedRun    one entry per (suite, shard)   -> [{"suite": …, "shard": "1/4"}, …]

Why generate rather than hand-write
-----------------------------------
The shard count is a property of a suite — how long it takes — and it lives in
suites.yaml next to that suite's other facts. Hand-written matrices drift from
it silently and in the direction that hurts: someone raises `shards: 4` to
`6` because the suite got slower, CI keeps running four, and the change reads
as having been made. Generating the matrix means suites.yaml is the only place
the number exists.

A sharded suite gets its images built once and handed to its shards as an
artifact; an unsharded one builds in the same job that runs it. Splitting the
matrices this way is what keeps a four-shard suite from repeating the same
three image builds four times — the dominant cost of the job, and one that
does not parallelise away.
"""

from __future__ import annotations

import argparse
import json
import os
import sys
from pathlib import Path

import yaml

ROOT = Path(__file__).resolve().parents[3]
SUITES_YAML = ROOT / "e2e" / "playwright" / "suites.yaml"


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "--github",
        action="store_true",
        help="append the matrices to $GITHUB_OUTPUT as JSON",
    )
    args = parser.parse_args()

    with SUITES_YAML.open() as handle:
        suites = (yaml.safe_load(handle) or {}).get("suites", {})

    if not suites:
        print(f"{SUITES_YAML} declares no suites", file=sys.stderr)
        return 1

    single: list[str] = []
    sharded_build: list[str] = []
    sharded_run: list[dict[str, str]] = []

    for name in sorted(suites):
        shards = (suites[name] or {}).get("shards")
        if shards is None or int(shards) <= 1:
            single.append(name)
            continue
        total = int(shards)
        sharded_build.append(name)
        sharded_run.extend(
            {"suite": name, "shard": f"{index}/{total}", "index": str(index)}
            for index in range(1, total + 1)
        )

    outputs = {
        "single": json.dumps(single),
        "shardedBuild": json.dumps(sharded_build),
        "shardedRun": json.dumps(sharded_run),
        # GitHub skips a matrix job whose list is empty, but `strategy.matrix`
        # rejects an empty array outright, so each consumer needs an `if:` as
        # well. These booleans are what those `if:`s read.
        "hasSingle": json.dumps(bool(single)),
        "hasSharded": json.dumps(bool(sharded_build)),
    }

    if args.github:
        target = os.environ.get("GITHUB_OUTPUT")
        if not target:
            print("--github given but GITHUB_OUTPUT is not set", file=sys.stderr)
            return 1
        with open(target, "a", encoding="utf-8") as handle:
            for key, value in outputs.items():
                handle.write(f"{key}={value}\n")

    for key, value in outputs.items():
        print(f"{key}={value}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
