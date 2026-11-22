#!/usr/bin/env python3
"""Unit tests for scripts/verify/run_pytest_randomized.py (ARCH-06).

Run:
    python3 scripts/tests/test-pytest-randomized.py
"""

from __future__ import annotations

import json
import os
import pathlib
import subprocess
import sys
import tempfile

REPO_ROOT = pathlib.Path(__file__).resolve().parents[2]
RUNNER = REPO_ROOT / "scripts" / "verify" / "run_pytest_randomized.py"

TEST_CASE_COUNT = 12
SEED_A = 12345
SEED_B = 67890

RESULTS = {"passed": 0, "failed": 0}


def check(name: str, condition: bool, detail: str = "") -> None:
    if condition:
        sys.stdout.write(f"  PASS  {name}\n")
        RESULTS["passed"] += 1
        return
    sys.stdout.write(f"  FAIL  {name}\n")
    if detail:
        sys.stdout.write(f"        {detail}\n")
    RESULTS["failed"] += 1


def _write_order_recording_suite(root: pathlib.Path) -> None:
    (root / "pyproject.toml").write_text(
        '[project]\nname = "rand-fixture"\nversion = "0.1.0"\nrequires-python = ">=3.11"\n',
        encoding="utf-8",
    )
    tests_dir = root / "tests"
    tests_dir.mkdir(parents=True, exist_ok=True)
    names = [f"test_case_{idx:02d}" for idx in range(TEST_CASE_COUNT)]
    body_lines = [
        "import os",
        "import pathlib",
        "",
    ]
    for test_name in names:
        body_lines.extend(
            [
                f"def {test_name}() -> None:",
                "    order_file = pathlib.Path(os.environ['ORDER_FILE'])",
                "    with order_file.open('a', encoding='utf-8') as fh:",
                f"        fh.write({test_name!r} + '\\n')",
                "",
            ]
        )
    (tests_dir / "test_order.py").write_text("\n".join(body_lines), encoding="utf-8")


def _run_runner(
    target_dir: pathlib.Path,
    *extra_args: str,
    order_file: pathlib.Path | None = None,
) -> subprocess.CompletedProcess[str]:
    cmd = [
        sys.executable,
        str(RUNNER),
        "--pytest-cmd",
        f"{sys.executable} -m pytest",
        str(target_dir),
        *extra_args,
    ]
    env = os.environ.copy()
    if order_file is not None:
        env["ORDER_FILE"] = str(order_file)
    return subprocess.run(  # noqa: S603 - fixed argv built in this test
        cmd,
        capture_output=True,
        text=True,
        env=env,
        check=False,
    )


def test_deterministic_seed_and_shuffling() -> None:
    with tempfile.TemporaryDirectory() as tmp:
        root = pathlib.Path(tmp)
        order_a = root / "order_a.txt"
        order_b = root / "order_b.txt"
        order_c = root / "order_c.txt"
        seed_log = root / "seeds.jsonl"

        _write_order_recording_suite(root)
        res_a = _run_runner(
            root,
            "--seed",
            str(SEED_A),
            "--seed-log",
            str(seed_log),
            "--",
            "-q",
            order_file=order_a,
        )
        lines_a = order_a.read_text(encoding="utf-8").splitlines() if order_a.exists() else []

        res_b = _run_runner(root, "--seed", str(SEED_A), "--", "-q", order_file=order_b)
        lines_b = order_b.read_text(encoding="utf-8").splitlines() if order_b.exists() else []

        res_c = _run_runner(root, "--seed", str(SEED_B), "--", "-q", order_file=order_c)
        lines_c = order_c.read_text(encoding="utf-8").splitlines() if order_c.exists() else []

        check(
            f"explicit seed succeeds and prints PYTEST_RANDOM_SEED={SEED_A}",
            res_a.returncode == 0 and f"PYTEST_RANDOM_SEED={SEED_A}" in res_a.stdout,
            f"rc={res_a.returncode} stdout={res_a.stdout!r} stderr={res_a.stderr!r}",
        )
        check(
            "same seed reproduces exact execution order",
            res_b.returncode == 0 and lines_a == lines_b and len(lines_a) == TEST_CASE_COUNT,
            f"a={lines_a} b={lines_b}",
        )
        check(
            "different seed produces a different shuffled order",
            res_c.returncode == 0 and lines_a != lines_c and sorted(lines_a) == sorted(lines_c),
            f"a={lines_a} c={lines_c}",
        )
        check(
            "shuffled order differs from sorted default order",
            lines_a != sorted(lines_a),
            f"a={lines_a}",
        )

        records = [
            json.loads(line)
            for line in seed_log.read_text(encoding="utf-8").splitlines()
            if line.strip()
        ]
        check(
            "seed log appends JSON record with seed and exit_code",
            len(records) == 1
            and records[0]["seed"] == SEED_A
            and records[0]["exit_code"] == 0
            and records[0]["target"] == str(root),
            f"records={records!r}",
        )


def test_generated_seed_and_failure_exit_code() -> None:
    with tempfile.TemporaryDirectory() as tmp:
        root = pathlib.Path(tmp)
        tests_dir = root / "tests"
        tests_dir.mkdir(parents=True, exist_ok=True)
        (tests_dir / "test_fail.py").write_text(
            "def test_always_fails() -> None:\n    assert False\n",
            encoding="utf-8",
        )
        seed_log = root / "seeds.jsonl"
        res = _run_runner(root, "--seed-log", str(seed_log), "--", "-q")

        check(
            "failing test propagates pytest non-zero exit code (1)",
            res.returncode == 1,
            f"rc={res.returncode} stdout={res.stdout!r} stderr={res.stderr!r}",
        )
        check(
            "omitted --seed auto-generates and prints PYTEST_RANDOM_SEED=<int>",
            "PYTEST_RANDOM_SEED=" in res.stdout,
            f"stdout={res.stdout!r}",
        )
        records = [
            json.loads(line)
            for line in seed_log.read_text(encoding="utf-8").splitlines()
            if line.strip()
        ]
        check(
            "seed log records non-zero exit code on failure",
            len(records) == 1
            and isinstance(records[0]["seed"], int)
            and records[0]["exit_code"] == 1,
            f"records={records!r}",
        )


def main() -> int:
    if not RUNNER.is_file():
        sys.stderr.write(f"FAIL: runner script missing: {RUNNER}\n")
        return 1
    test_deterministic_seed_and_shuffling()
    test_generated_seed_and_failure_exit_code()
    sys.stdout.write(f"\nSummary: {RESULTS['passed']} passed, {RESULTS['failed']} failed\n")
    return 0 if RESULTS["failed"] == 0 else 1


if __name__ == "__main__":
    raise SystemExit(main())
