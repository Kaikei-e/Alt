#!/usr/bin/env python3
"""Emit the GitHub Actions job matrices for an E2E Playwright workflow.

    python3 e2e/playwright/_lib/ci-matrix.py                                     # human-readable (default: e2e-playwright.yml)
    python3 e2e/playwright/_lib/ci-matrix.py --workflow alt-frontend-sv.yml      # matrices for a dedicated workflow
    python3 e2e/playwright/_lib/ci-matrix.py --github                            # writes $GITHUB_OUTPUT

Three matrices come out of `e2e/playwright/suites.yaml`:

    single        suites that run as one job     -> ["mq-hub", "auth-hub", ...]
    shardedBuild  suites that fan out            -> ["alt-backend"]
    shardedRun    one entry per (suite, shard)   -> [{"suite": …, "shard": "1/4"}, …]

Each suite in `suites.yaml` may declare an optional `workflow:` field naming the
bare filename of the workflow under `.github/workflows/` that owns it in PR CI
(default: `e2e-playwright.yml`). Passing `--workflow <file>` emits the matrices
for the suites owned by `<file>` and logs skipped suites (with their owning
workflow) to stderr; any `workflow` value that is not a bare filename of an
existing file under `.github/workflows/` fails with exit code 1.

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
WORKFLOWS_DIR = ROOT / ".github" / "workflows"
DEFAULT_WORKFLOW = "e2e-playwright.yml"


def _validate_workflow_name(value: object, *, label: str) -> str | None:
    if (
        not isinstance(value, str)
        or not value
        or Path(value).name != value
        or "/" in value
        or "\\" in value
        or value in {".", ".."}
    ):
        print(
            f"{label}: workflow {value!r} must be a bare filename under {WORKFLOWS_DIR}",
            file=sys.stderr,
        )
        return None
    if not (WORKFLOWS_DIR / value).is_file():
        print(
            f"{label}: workflow {value!r} does not exist under {WORKFLOWS_DIR}",
            file=sys.stderr,
        )
        return None
    return value


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "--github",
        action="store_true",
        help="append the matrices to $GITHUB_OUTPUT as JSON",
    )
    parser.add_argument(
        "--workflow",
        default=DEFAULT_WORKFLOW,
        help=f"emit matrices for suites owned by this workflow under .github/workflows/ (default: {DEFAULT_WORKFLOW})",
    )
    args = parser.parse_args()

    target_workflow = _validate_workflow_name(args.workflow, label="--workflow")
    if target_workflow is None:
        return 1

    with SUITES_YAML.open() as handle:
        suites = (yaml.safe_load(handle) or {}).get("suites", {})

    if not suites:
        print(f"{SUITES_YAML} declares no suites", file=sys.stderr)
        return 1

    single: list[str] = []
    sharded_build: list[str] = []
    sharded_run: list[dict[str, str]] = []

    for name in sorted(suites):
        entry = suites[name] or {}
        raw_workflow = entry.get("workflow", DEFAULT_WORKFLOW)
        suite_workflow = _validate_workflow_name(
            raw_workflow, label=f"suite {name!r}"
        )
        if suite_workflow is None:
            return 1
        if suite_workflow != target_workflow:
            print(
                f"skipping suite {name} (owned by {suite_workflow})",
                file=sys.stderr,
            )
            continue
        shards = entry.get("shards")
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
