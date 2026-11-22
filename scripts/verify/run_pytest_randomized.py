#!/usr/bin/env python3
"""Run a Python service's pytest suite in deterministic randomized order (ARCH-06).

Injects a zero-dependency pytest plugin via ``PYTHONPATH`` + ``-p`` that shuffles
collected test items with ``random.Random(seed)``, prints ``PYTEST_RANDOM_SEED=<seed>``
prominently, optionally appends a JSONL record to ``--seed-log``, and returns
pytest's exact exit code.

Usage:
    python3 scripts/verify/run_pytest_randomized.py tag-generator/app
    python3 scripts/verify/run_pytest_randomized.py --seed 42 news-creator/app -- -q
"""

from __future__ import annotations

import argparse
import datetime
import json
import os
import pathlib
import secrets
import shlex
import subprocess
import sys
import tempfile

_PLUGIN_MODULE_NAME = "_alt_pytest_random_order_plugin"
_PLUGIN_SOURCE = '''"""Temporary pytest plugin injected by run_pytest_randomized.py."""

from __future__ import annotations

import os
import random


def pytest_collection_modifyitems(items: list[object]) -> None:
    raw_seed = os.environ.get("ALT_PYTEST_RANDOM_SEED")
    if raw_seed is None:
        return
    rng = random.Random(int(raw_seed))
    rng.shuffle(items)
'''


def _parse_args(argv: list[str]) -> tuple[argparse.Namespace, list[str]]:
    if "--" in argv:
        split_idx = argv.index("--")
        runner_argv = argv[:split_idx]
        pytest_args = argv[split_idx + 1 :]
    else:
        runner_argv = argv
        pytest_args = []

    parser = argparse.ArgumentParser(
        description="Run pytest with deterministic randomized test execution order."
    )
    parser.add_argument(
        "target_dir",
        type=pathlib.Path,
        help="Target Python service directory (working directory for pytest).",
    )
    parser.add_argument(
        "--seed",
        type=int,
        default=None,
        help="Deterministic integer seed (random 32-bit seed generated when omitted).",
    )
    parser.add_argument(
        "--seed-log",
        type=pathlib.Path,
        default=None,
        help="Optional path to a JSONL file recording {timestamp, target, seed, exit_code}.",
    )
    parser.add_argument(
        "--pytest-cmd",
        default="uv run pytest",
        help="Command prefix used to invoke pytest (default: 'uv run pytest').",
    )
    return parser.parse_args(runner_argv), pytest_args


def _append_seed_log(
    seed_log: pathlib.Path,
    target_dir: pathlib.Path,
    seed: int,
    exit_code: int,
) -> None:
    seed_log.parent.mkdir(parents=True, exist_ok=True)
    record = {
        "timestamp": datetime.datetime.now(datetime.UTC).isoformat(),
        "target": str(target_dir),
        "seed": seed,
        "exit_code": exit_code,
    }
    with seed_log.open("a", encoding="utf-8") as fh:
        fh.write(json.dumps(record, sort_keys=True) + "\n")


def main(argv: list[str] | None = None) -> int:
    args, extra_args = _parse_args(sys.argv[1:] if argv is None else argv)
    target_dir: pathlib.Path = args.target_dir
    if not target_dir.is_dir():
        sys.stderr.write(f"ERROR: target directory does not exist: {target_dir}\n")
        return 2

    seed = args.seed if args.seed is not None else secrets.randbits(32)

    sys.stdout.write(f"PYTEST_RANDOM_SEED={seed}\n")
    sys.stdout.flush()

    base_cmd = shlex.split(args.pytest_cmd)
    if not base_cmd:
        sys.stderr.write("ERROR: --pytest-cmd must not be empty\n")
        return 2

    with tempfile.TemporaryDirectory(prefix="alt-pytest-rand-") as plugin_dir:
        plugin_path = pathlib.Path(plugin_dir) / f"{_PLUGIN_MODULE_NAME}.py"
        plugin_path.write_text(_PLUGIN_SOURCE, encoding="utf-8")

        env = os.environ.copy()
        env["ALT_PYTEST_RANDOM_SEED"] = str(seed)
        existing_pythonpath = env.get("PYTHONPATH", "")
        env["PYTHONPATH"] = (
            f"{plugin_dir}{os.pathsep}{existing_pythonpath}" if existing_pythonpath else plugin_dir
        )

        cmd = [*base_cmd, "-p", _PLUGIN_MODULE_NAME, *extra_args]
        proc = subprocess.run(  # noqa: S603 - explicit argv invoking configured pytest runner
            cmd,
            cwd=str(target_dir),
            env=env,
            check=False,
        )

    if args.seed_log is not None:
        _append_seed_log(args.seed_log, target_dir, seed, proc.returncode)

    return proc.returncode


if __name__ == "__main__":
    raise SystemExit(main())
