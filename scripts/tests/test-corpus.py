#!/usr/bin/env python3
"""TDD unit and integration tests for scripts/harness/corpus.py (CORPUS-01).

Verifies:
1. Valid caught + expected-miss corpora compute exact capture rate and exit 0.
2. Unexpected misses, false positives, and wrong diagnostic signals exit 1.
3. Stub `pyrefly` via PYREFLY_BIN verifies `--config` is passed (generated and
   support_files pyrefly.toml) and missing binary fails closed.
4. Real REPO-02 (canary_audit.py go-verify-scan) and REPO-03 (check_suppressions.py)
   check runners pass on clean before and fail with expected_signal on broken after.
5. Schema validation and path security reject non-contiguous IDs, identical
   before/after, path traversal/symlinks, oversized snippets (>120 lines / >8192B),
   inconsistent detection flags, and unactivated implemented checks.
6. Special valid cases: binary ELF snippets (`language = "binary"`), 0-byte after
   truncation, `"worktree-uncommitted"` commits, and full 22-case corpus.
7. Real repository corpus under `scripts/harness/corpus` (min 22 cases) unless
   `--allow-missing-corpus-dir` is explicitly passed before corpus files land.

Run:
    python3 scripts/tests/test-corpus.py
"""

from __future__ import annotations

import argparse
import importlib.util
import os
import pathlib
import stat
import subprocess
import sys
import tempfile
import types
from dataclasses import dataclass
from unittest import mock

ROOT = pathlib.Path(__file__).resolve().parents[2]
CORPUS_SCRIPT = ROOT / "scripts" / "harness" / "corpus.py"
REAL_CORPUS_DIR = ROOT / "scripts" / "harness" / "corpus"

DEFAULT_MIN_CASES = 22
MIN_CAUGHT_CASES = 22
MAX_SNIPPET_LINES = 120
MAX_SNIPPET_BYTES = 8192
EXIT_OK = 0
EXIT_FAIL = 1


class _Results:
    def __init__(self) -> None:
        self.passed = 0
        self.failed = 0


RESULTS = _Results()


def check(name: str, condition: bool, detail: str = "") -> None:
    if condition:
        sys.stdout.write(f"  PASS  {name}\n")
        RESULTS.passed += 1
    else:
        suffix = f" ({detail})" if detail else ""
        sys.stdout.write(f"  FAIL  {name}{suffix}\n")
        RESULTS.failed += 1


def _toml_str(value: str) -> str:
    escaped = value.replace("\\", "\\\\").replace('"', '\\"')
    return f'"{escaped}"'


def _toml_list(values: list[str]) -> str:
    return "[" + ", ".join(_toml_str(v) for v in values) + "]"


@dataclass(frozen=True)
class MetaSpec:
    case_id: str
    title: str = "Sample failure case"
    category: str = "autofix-logic-corruption"
    language: str = "python"
    before_file: str = "before.py"
    after_file: str = "after.py"
    support_files: tuple[str, ...] | None = None
    evaluation_mode: str = "after"
    branch: str = "chore/lint-rollout"
    base_commit: str = "6cae3ef6f"
    origin_files: tuple[str, ...] = ("scripts/sample.py",)
    commits: tuple[str, ...] = ("d8ce79dec",)
    source_ref: str = "docs/review/lint-rollout-harness-learnings-2026-11-21.md#2.1"
    notes: str = "Minimal deterministic failure reproduction snippet."
    planned_checks: tuple[str, ...] = ("VERIFY-01",)
    active_checks: tuple[str, ...] = ()
    currently_caught: bool = False
    current_method: str = "diff-review-only"
    expected_signal: str = ""
    expected_signals: dict[str, str] | None = None


def build_meta_toml(spec: MetaSpec) -> str:
    lines = [
        f"id = {_toml_str(spec.case_id)}",
        f"title = {_toml_str(spec.title)}",
        f"category = {_toml_str(spec.category)}",
        f"language = {_toml_str(spec.language)}",
        f"before_file = {_toml_str(spec.before_file)}",
        f"after_file = {_toml_str(spec.after_file)}",
    ]
    if spec.support_files is not None:
        lines.append(f"support_files = {_toml_list(list(spec.support_files))}")
    lines.extend(
        [
            f"evaluation_mode = {_toml_str(spec.evaluation_mode)}",
            "",
            "[origin]",
            f"branch = {_toml_str(spec.branch)}",
            f"base_commit = {_toml_str(spec.base_commit)}",
            f"files = {_toml_list(list(spec.origin_files))}",
            f"commits = {_toml_list(list(spec.commits))}",
            f"source_ref = {_toml_str(spec.source_ref)}",
            f"notes = {_toml_str(spec.notes)}",
            "",
            "[detection]",
            f"planned_checks = {_toml_list(list(spec.planned_checks))}",
            f"active_checks = {_toml_list(list(spec.active_checks))}",
            f"currently_caught = {'true' if spec.currently_caught else 'false'}",
            f"current_method = {_toml_str(spec.current_method)}",
            f"expected_signal = {_toml_str(spec.expected_signal)}",
        ]
    )
    if spec.expected_signals is not None:
        lines.append("")
        lines.append("[detection.expected_signals]")
        for check_id, sig in spec.expected_signals.items():
            lines.append(f"{_toml_str(check_id)} = {_toml_str(sig)}")
    lines.append("")
    return "\n".join(lines)


def write_case(
    corpus_dir: pathlib.Path,
    case_id: str,
    *,
    meta_toml: str,
    files: dict[str, bytes | str],
) -> pathlib.Path:
    case_dir = corpus_dir / case_id
    case_dir.mkdir(parents=True, exist_ok=True)
    (case_dir / "meta.toml").write_text(meta_toml, encoding="utf-8")
    for fname, content in files.items():
        target = case_dir / fname
        if isinstance(content, bytes):
            target.write_bytes(content)
        else:
            target.write_text(content, encoding="utf-8")
    return case_dir


def make_executable(path: pathlib.Path, content: str) -> pathlib.Path:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(content, encoding="utf-8")
    path.chmod(path.stat().st_mode | stat.S_IXUSR | stat.S_IXGRP | stat.S_IXOTH)
    return path


def make_stub_pyrefly(bin_dir: pathlib.Path, mode: str = "c09") -> pathlib.Path:
    script = f"""#!{sys.executable}
import pathlib, sys

mode = {mode!r}
args = sys.argv[1:]
if not args or args[0] != "check":
    sys.stderr.write(f"stub-pyrefly: expected 'check', got {{args!r}}\\n")
    sys.exit(2)

cfg_path = None
target_path = None
i = 1
while i < len(args):
    if args[i] == "--config" and i + 1 < len(args):
        cfg_path = pathlib.Path(args[i + 1])
        i += 2
    else:
        target_path = pathlib.Path(args[i])
        i += 1

if cfg_path is None or not cfg_path.is_file():
    sys.stderr.write("stub-pyrefly: missing --config file\\n")
    sys.exit(2)

cfg_text = cfg_path.read_text(encoding="utf-8")
if "pyrefly" not in cfg_text and "python-version" not in cfg_text:
    sys.stderr.write(f"stub-pyrefly: unexpected config content: {{cfg_text!r}}\\n")
    sys.exit(2)

if target_path is None or not target_path.is_file():
    sys.stderr.write("stub-pyrefly: missing target file\\n")
    sys.exit(2)

src = target_path.read_text(encoding="utf-8")
if mode == "wrong-signal":
    if "def format(self, fmt:" in src:
        sys.stderr.write("internal crash: unrelated diagnostic\\n")
        sys.exit(2)
    sys.exit(0)

if mode == "require-pyrefly-toml":
    if cfg_path.name != "pyrefly.toml":
        sys.stderr.write(f"stub-pyrefly: expected pyrefly.toml, got {{cfg_path.name}}\\n")
        sys.exit(2)

if "def format(self, fmt:" in src:
    sys.stdout.write("ERROR [bad-override-param-name] parameter renamed from format to fmt\\n")
    sys.exit(1)

sys.stdout.write("INFO 0 errors\\n")
sys.exit(0)
"""
    return make_executable(bin_dir / f"pyrefly-{mode}", script)


def run_corpus_cli(
    corpus_dir: pathlib.Path,
    *,
    min_cases: int | None = None,
    allow_empty_corpus: bool = False,
    extra_env: dict[str, str] | None = None,
) -> subprocess.CompletedProcess[str]:
    cmd = [
        sys.executable,
        str(CORPUS_SCRIPT),
        "--corpus-dir",
        str(corpus_dir),
        "--repo-root",
        str(ROOT),
    ]
    if min_cases is not None:
        cmd.extend(["--min-cases", str(min_cases)])
    if allow_empty_corpus:
        cmd.append("--allow-empty-corpus")
    env = os.environ.copy()
    if extra_env:
        env.update(extra_env)
    return subprocess.run(  # noqa: S603 - fixed argv invoking corpus.py with sys.executable
        cmd,
        capture_output=True,
        text=True,
        env=env,
        check=False,
    )


def add_repo02_case(corpus_dir: pathlib.Path, case_id: str) -> None:
    write_case(
        corpus_dir,
        case_id,
        meta_toml=build_meta_toml(
            MetaSpec(
                case_id=case_id,
                title="gosec zero-file silent pass",
                category="scanner-silent-pass",
                language="go",
                before_file="before.txt",
                after_file="after.txt",
                support_files=("sample.go", "before.sarif", "after.sarif"),
                evaluation_mode="after",
                origin_files=("scripts/lint/go.sh",),
                planned_checks=("REPO-02",),
                active_checks=("REPO-02",),
                currently_caught=True,
                current_method="REPO-02",
                expected_signal="ZERO-FILE SCAN DETECTED",
            )
        ),
        files={
            "sample.go": "package main\n\nfunc main() {}\n",
            "before.txt": "Summary:\n  Files  : 1\n  Issues : 0\n",
            "after.txt": "Summary:\n  Files  : 0\n  Issues : 0\n",
            "before.sarif": '{"version":"2.1.0","runs":[{"tool":{"driver":{"name":"gosec","rules":[]}},"results":[]}]}\n',
            "after.sarif": '{"version":"2.1.0","runs":[{"tool":{"driver":{"name":"gosec","rules":[]}},"results":[]}]}\n',
        },
    )


def add_repo03_case(
    corpus_dir: pathlib.Path,
    case_id: str,
    *,
    before_text: str = "def ok() -> int:\n    return 1\n",
    after_text: str = "# ruff: noqa: TRY003\ndef bad() -> int:\n    return 1\n",
    expected_signal: str = "source.broad.missing-adr",
) -> None:
    write_case(
        corpus_dir,
        case_id,
        meta_toml=build_meta_toml(
            MetaSpec(
                case_id=case_id,
                title="file-level ruff noqa without ADR",
                category="blanket-suppression",
                language="python",
                before_file="before.py",
                after_file="after.py",
                evaluation_mode="after",
                origin_files=("tag-generator/app/sample.py",),
                planned_checks=("REPO-03",),
                active_checks=("REPO-03",),
                currently_caught=True,
                current_method="REPO-03",
                expected_signal=expected_signal,
            )
        ),
        files={"before.py": before_text, "after.py": after_text},
    )


def add_pyrefly_case(
    corpus_dir: pathlib.Path,
    case_id: str,
    *,
    with_pyrefly_toml: bool = False,
) -> None:
    files: dict[str, bytes | str] = {
        "before.py": (
            "class Base:\n"
            "    def format(self, format: str) -> str:\n"
            "        return format\n\n"
            "class Child(Base):\n"
            "    def format(self, format: str) -> str:\n"
            "        return format.upper()\n"
        ),
        "after.py": (
            "class Base:\n"
            "    def format(self, format: str) -> str:\n"
            "        return format\n\n"
            "class Child(Base):\n"
            "    def format(self, fmt: str) -> str:\n"
            "        return fmt.upper()\n"
        ),
    }
    support: tuple[str, ...] = ()
    if with_pyrefly_toml:
        files["pyrefly.toml"] = 'python-version = "3.14"\n'
        support = ("pyrefly.toml",)
    write_case(
        corpus_dir,
        case_id,
        meta_toml=build_meta_toml(
            MetaSpec(
                case_id=case_id,
                title="override parameter renamed from format to fmt",
                category="signature-override-drift",
                language="python",
                before_file="before.py",
                after_file="after.py",
                support_files=support,
                evaluation_mode="after",
                origin_files=("recap-subworker/sample.py",),
                planned_checks=("pyrefly",),
                active_checks=("pyrefly",),
                currently_caught=True,
                current_method="pyrefly",
                expected_signal="bad-override-param-name",
            )
        ),
        files=files,
    )


def test_capture_rate_and_outcomes() -> None:
    sys.stdout.write("capture rate and detection outcomes\n")
    with tempfile.TemporaryDirectory() as tmp_str:
        tmp = pathlib.Path(tmp_str)

        # 1. Valid caught + expected-miss -> exit 0, 1/2 (50.0%)
        c_mixed = tmp / "mixed"
        write_case(
            c_mixed,
            "C-01",
            meta_toml=build_meta_toml(
                MetaSpec(
                    case_id="C-01",
                    planned_checks=("VERIFY-01", "EXEC-01"),
                    active_checks=(),
                    currently_caught=False,
                )
            ),
            files={"before.py": "x = 1\n", "after.py": "x = 2\n"},
        )
        add_repo03_case(c_mixed, "C-02")
        res = run_corpus_cli(c_mixed, min_cases=2)
        expected_line = (
            "CORPUS CAPTURE RATE: 1/2 (50.0%) [caught=1: C-02 | expected-miss=1 | failures=0]"
        )
        check(
            "caught + expected-miss exits 0 with 50.0% capture rate",
            res.returncode == EXIT_OK and expected_line in res.stdout,
            f"rc={res.returncode}\nstdout={res.stdout}\nstderr={res.stderr}",
        )

        # 2. Unexpected miss (after exits 0) -> exit 1
        c_miss = tmp / "unexpected-miss"
        add_repo03_case(
            c_miss,
            "C-01",
            before_text="x = 1\n",
            after_text="x = 2\n",
        )
        res = run_corpus_cli(c_miss, min_cases=1)
        out = res.stdout + res.stderr
        check(
            "unexpected miss (after exits 0) fails with exit 1",
            res.returncode == EXIT_FAIL and "unexpected-miss" in out and "failures=1" in res.stdout,
            f"rc={res.returncode}\nout={out}",
        )

        # 3. False positive (before exits 1) -> exit 1
        c_fp = tmp / "false-positive"
        add_repo03_case(
            c_fp,
            "C-01",
            before_text="# ruff: noqa: TRY003\nx = 1\n",
            after_text="# ruff: noqa: TRY003, F401\nx = 1\n",
        )
        res = run_corpus_cli(c_fp, min_cases=1)
        out = res.stdout + res.stderr
        check(
            "false positive (before exits non-zero) fails with exit 1",
            res.returncode == EXIT_FAIL and "false-positive" in out and "failures=1" in res.stdout,
            f"rc={res.returncode}\nout={out}",
        )

        # 4. Wrong diagnostic signal (after exits 2 without expected_signal) -> exit 1
        stub_wrong = make_stub_pyrefly(tmp / "bin", mode="wrong-signal")
        c_wrong = tmp / "wrong-signal"
        add_pyrefly_case(c_wrong, "C-01")
        res = run_corpus_cli(
            c_wrong,
            min_cases=1,
            extra_env={"PYREFLY_BIN": str(stub_wrong)},
        )
        out = res.stdout + res.stderr
        check(
            "wrong diagnostic signal fails with exit 1",
            res.returncode == EXIT_FAIL and "wrong-signal" in out and "failures=1" in res.stdout,
            f"rc={res.returncode}\nout={out}",
        )


def test_pyrefly_and_repo02_runners() -> None:
    sys.stdout.write("pyrefly and REPO-02 check runners\n")
    with tempfile.TemporaryDirectory() as tmp_str:
        tmp = pathlib.Path(tmp_str)
        stub_ok = make_stub_pyrefly(tmp / "bin", mode="c09")
        stub_toml = make_stub_pyrefly(tmp / "bin", mode="require-pyrefly-toml")

        # pyrefly with auto-generated pyproject.toml config
        c_auto = tmp / "pyrefly-auto"
        add_pyrefly_case(c_auto, "C-01", with_pyrefly_toml=False)
        res = run_corpus_cli(c_auto, min_cases=1, extra_env={"PYREFLY_BIN": str(stub_ok)})
        check(
            "pyrefly runner passes --config pyproject.toml and catches C-09 pattern",
            res.returncode == EXIT_OK and "CORPUS CAPTURE RATE: 1/1 (100.0%)" in res.stdout,
            f"rc={res.returncode}\nstdout={res.stdout}\nstderr={res.stderr}",
        )

        # pyrefly with explicit pyrefly.toml in support_files
        c_cfg = tmp / "pyrefly-toml"
        add_pyrefly_case(c_cfg, "C-01", with_pyrefly_toml=True)
        res = run_corpus_cli(c_cfg, min_cases=1, extra_env={"PYREFLY_BIN": str(stub_toml)})
        check(
            "pyrefly runner passes support_files pyrefly.toml to --config",
            res.returncode == EXIT_OK and "CORPUS CAPTURE RATE: 1/1 (100.0%)" in res.stdout,
            f"rc={res.returncode}\nstdout={res.stdout}\nstderr={res.stderr}",
        )

        # Missing PYREFLY_BIN fails closed (never silently passes)
        res = run_corpus_cli(
            c_auto,
            min_cases=1,
            extra_env={"PYREFLY_BIN": str(tmp / "bin" / "does-not-exist")},
        )
        check(
            "missing PYREFLY_BIN fails closed with exit 1",
            res.returncode == EXIT_FAIL,
            f"rc={res.returncode}\nstdout={res.stdout}\nstderr={res.stderr}",
        )

        # REPO-02 runner against real canary_audit.py
        c_r02 = tmp / "repo02"
        add_repo02_case(c_r02, "C-01")
        res = run_corpus_cli(c_r02, min_cases=1)
        check(
            "REPO-02 runner catches C-21 zero-file scan via canary_audit.py",
            res.returncode == EXIT_OK
            and "CORPUS CAPTURE RATE: 1/1 (100.0%) [caught=1: C-01 | expected-miss=0 | failures=0]"
            in res.stdout,
            f"rc={res.returncode}\nstdout={res.stdout}\nstderr={res.stderr}",
        )


def test_schema_and_security_rejections() -> None:
    sys.stdout.write("schema validation and path security\n")
    with tempfile.TemporaryDirectory() as tmp_str:
        tmp = pathlib.Path(tmp_str)

        # 1. Non-contiguous IDs (C-01, C-03)
        c_gap = tmp / "gap"
        write_case(
            c_gap,
            "C-01",
            meta_toml=build_meta_toml(MetaSpec(case_id="C-01")),
            files={"before.py": "a = 1\n", "after.py": "a = 2\n"},
        )
        write_case(
            c_gap,
            "C-03",
            meta_toml=build_meta_toml(MetaSpec(case_id="C-03")),
            files={"before.py": "a = 1\n", "after.py": "a = 2\n"},
        )
        res = run_corpus_cli(c_gap, min_cases=2)
        check(
            "non-contiguous IDs (C-01, C-03) rejected",
            res.returncode == EXIT_FAIL,
            res.stdout + res.stderr,
        )

        # 2. before == after
        c_same = tmp / "same"
        write_case(
            c_same,
            "C-01",
            meta_toml=build_meta_toml(MetaSpec(case_id="C-01")),
            files={"before.py": "same = 1\n", "after.py": "same = 1\n"},
        )
        res = run_corpus_cli(c_same, min_cases=1)
        check(
            "identical before and after bytes rejected",
            res.returncode == EXIT_FAIL,
            res.stdout + res.stderr,
        )

        # 3. Path traversal in before_file
        c_trav = tmp / "traversal"
        (tmp / "escape.py").write_text("escaped = 1\n", encoding="utf-8")
        write_case(
            c_trav,
            "C-01",
            meta_toml=build_meta_toml(MetaSpec(case_id="C-01", before_file="../escape.py")),
            files={"after.py": "a = 2\n"},
        )
        res = run_corpus_cli(c_trav, min_cases=1)
        check(
            "path traversal '..' in before_file rejected",
            res.returncode == EXIT_FAIL,
            res.stdout + res.stderr,
        )

        # 4. Symlink escape inside case dir
        c_sym = tmp / "symlink"
        case_sym = write_case(
            c_sym,
            "C-01",
            meta_toml=build_meta_toml(MetaSpec(case_id="C-01")),
            files={"after.py": "a = 2\n"},
        )
        (case_sym / "before.py").symlink_to(tmp / "escape.py")
        res = run_corpus_cli(c_sym, min_cases=1)
        check(
            "symlink escape in before_file rejected",
            res.returncode == EXIT_FAIL,
            res.stdout + res.stderr,
        )

        # 5. >120 lines text snippet rejected
        c_lines = tmp / "too-many-lines"
        long_text = "".join(f"x_{i} = {i}\n" for i in range(MAX_SNIPPET_LINES + 1))
        write_case(
            c_lines,
            "C-01",
            meta_toml=build_meta_toml(MetaSpec(case_id="C-01")),
            files={"before.py": "x = 1\n", "after.py": long_text},
        )
        res = run_corpus_cli(c_lines, min_cases=1)
        check(
            "text snippet > 120 lines rejected",
            res.returncode == EXIT_FAIL,
            res.stdout + res.stderr,
        )

        # 6. >8192 bytes snippet rejected
        c_bytes = tmp / "too-many-bytes"
        write_case(
            c_bytes,
            "C-01",
            meta_toml=build_meta_toml(
                MetaSpec(
                    case_id="C-01",
                    language="binary",
                    before_file="before.bin",
                    after_file="after.bin",
                )
            ),
            files={"before.bin": b"a", "after.bin": b"\x7fELF" + (b"x" * MAX_SNIPPET_BYTES)},
        )
        res = run_corpus_cli(c_bytes, min_cases=1)
        check(
            "snippet > 8192 bytes rejected",
            res.returncode == EXIT_FAIL,
            res.stdout + res.stderr,
        )

        # 7. currently_caught = true with active_checks = [] rejected
        c_inconsistent = tmp / "caught-no-active"
        write_case(
            c_inconsistent,
            "C-01",
            meta_toml=build_meta_toml(
                MetaSpec(
                    case_id="C-01",
                    planned_checks=("VERIFY-01",),
                    active_checks=(),
                    currently_caught=True,
                    expected_signal="some-signal",
                )
            ),
            files={"before.py": "a = 1\n", "after.py": "a = 2\n"},
        )
        res = run_corpus_cli(c_inconsistent, min_cases=1)
        check(
            "currently_caught=true with active_checks=[] rejected",
            res.returncode == EXIT_FAIL,
            res.stdout + res.stderr,
        )

        # 8. planned_checks includes implemented REPO-03 while active_checks = []
        c_unactivated = tmp / "unactivated-implemented"
        write_case(
            c_unactivated,
            "C-01",
            meta_toml=build_meta_toml(
                MetaSpec(
                    case_id="C-01",
                    planned_checks=("REPO-03",),
                    active_checks=(),
                    currently_caught=False,
                    expected_signal="",
                )
            ),
            files={"before.py": "a = 1\n", "after.py": "a = 2\n"},
        )
        res = run_corpus_cli(c_unactivated, min_cases=1)
        check(
            "implemented check in planned_checks but omitted from active_checks rejected",
            res.returncode == EXIT_FAIL,
            res.stdout + res.stderr,
        )

        # 9. currently_caught = false with non-empty expected_signal rejected
        c_signal_mismatch = tmp / "signal-mismatch"
        write_case(
            c_signal_mismatch,
            "C-01",
            meta_toml=build_meta_toml(
                MetaSpec(
                    case_id="C-01",
                    planned_checks=("VERIFY-01",),
                    active_checks=(),
                    currently_caught=False,
                    expected_signal="should-be-empty",
                )
            ),
            files={"before.py": "a = 1\n", "after.py": "a = 2\n"},
        )
        res = run_corpus_cli(c_signal_mismatch, min_cases=1)
        check(
            "currently_caught=false with non-empty expected_signal rejected",
            res.returncode == EXIT_FAIL,
            res.stdout + res.stderr,
        )

        # 10. Empty corpus fails by default, passes only with --allow-empty-corpus
        c_empty = tmp / "empty"
        c_empty.mkdir()
        res_default = run_corpus_cli(c_empty, min_cases=1)
        res_allowed = run_corpus_cli(c_empty, min_cases=1, allow_empty_corpus=True)
        check(
            "empty corpus fails by default and passes only with --allow-empty-corpus",
            res_default.returncode == EXIT_FAIL
            and res_allowed.returncode == EXIT_OK
            and "CORPUS CAPTURE RATE: 0/0 (0.0%)" in res_allowed.stdout,
            f"default={res_default.returncode}, allowed={res_allowed.returncode}",
        )


def test_special_valid_cases_and_full_22_fixture() -> None:
    sys.stdout.write("special valid cases and 22-case synthetic fixture\n")
    with tempfile.TemporaryDirectory() as tmp_str:
        tmp = pathlib.Path(tmp_str)
        stub_ok = make_stub_pyrefly(tmp / "bin", mode="c09")

        # Binary ELF + 0-byte after truncation + worktree-uncommitted + 22-case corpus
        c_full = tmp / "full-22"
        for idx in range(1, DEFAULT_MIN_CASES + 1):
            cid = f"C-{idx:02d}"
            if cid == "C-09":
                add_pyrefly_case(c_full, cid)
            elif cid == "C-19":
                write_case(
                    c_full,
                    cid,
                    meta_toml=build_meta_toml(
                        MetaSpec(
                            case_id=cid,
                            title="0-byte tracked file truncation under ENOSPC",
                            category="worktree-hygiene",
                            language="python",
                            before_file="before.py",
                            after_file="after.py",
                            commits=("worktree-uncommitted",),
                            planned_checks=("VERIFY-01",),
                            active_checks=(),
                            currently_caught=False,
                        )
                    ),
                    files={"before.py": "def router() -> str:\n    return 'ok'\n", "after.py": b""},
                )
            elif cid == "C-20":
                write_case(
                    c_full,
                    cid,
                    meta_toml=build_meta_toml(
                        MetaSpec(
                            case_id=cid,
                            title="untracked ELF binary left by go build",
                            category="worktree-hygiene",
                            language="binary",
                            before_file="before.bin",
                            after_file="after.elf",
                            commits=("worktree-uncommitted",),
                            planned_checks=("VERIFY-01",),
                            active_checks=(),
                            currently_caught=False,
                        )
                    ),
                    files={
                        "before.bin": b"",
                        "after.elf": b"\x7fELF\x02\x01\x01\x00\x00\x00\x00\x00",
                    },
                )
            elif cid == "C-21":
                add_repo02_case(c_full, cid)
            elif cid == "C-22":
                add_repo03_case(c_full, cid)
            else:
                write_case(
                    c_full,
                    cid,
                    meta_toml=build_meta_toml(
                        MetaSpec(
                            case_id=cid,
                            title=f"Synthetic expected-miss case {cid}",
                            planned_checks=("VERIFY-01",),
                            active_checks=(),
                            currently_caught=False,
                        )
                    ),
                    files={"before.py": f"val = {idx}\n", "after.py": f"val = {idx + 1}\n"},
                )

        res = run_corpus_cli(c_full, extra_env={"PYREFLY_BIN": str(stub_ok)})
        expected_summary = (
            "CORPUS CAPTURE RATE: 3/22 (13.6%) "
            "[caught=3: C-09,C-21,C-22 | expected-miss=19 | failures=0]"
        )
        check(
            "22-case corpus (with C-19 0-byte, C-20 ELF, C-09/C-21/C-22 caught) exits 0",
            res.returncode == EXIT_OK and expected_summary in res.stdout,
            f"rc={res.returncode}\nstdout={res.stdout}\nstderr={res.stderr}",
        )


def load_corpus_module() -> types.ModuleType:
    spec = importlib.util.spec_from_file_location("corpus_engine_under_test", CORPUS_SCRIPT)
    assert spec is not None
    assert spec.loader is not None
    mod = importlib.util.module_from_spec(spec)
    with mock.patch.dict(sys.modules, {"corpus_engine_under_test": mod}):
        spec.loader.exec_module(mod)
    return mod


def test_repo03_config_and_multi_check() -> None:
    sys.stdout.write("REPO-03 config normalization, expected_signals, and process checks\n")
    corpus_mod = load_corpus_module()

    expected_services = {
        "recap-subworker",
        "news-creator/app",
        "tag-generator/app",
        "acolyte-orchestrator",
        "metrics",
        "dashboard",
        "rerank-server",
        "recap-evaluator",
        "tts-speaker",
    }
    check(
        "PYREFLY_FALLBACK_SERVICES covers all 9 pyrefly services",
        set(getattr(corpus_mod, "PYREFLY_FALLBACK_SERVICES", ())) == expected_services,
        repr(getattr(corpus_mod, "PYREFLY_FALLBACK_SERVICES", ())),
    )
    check(
        "UNIMPLEMENTED_CORPUS_CHECKS and PROCESS_CHECKS are partitioned cleanly",
        hasattr(corpus_mod, "UNIMPLEMENTED_CORPUS_CHECKS")
        and hasattr(corpus_mod, "PROCESS_CHECKS")
        and "VERIFY-01" in getattr(corpus_mod, "PROCESS_CHECKS", frozenset())
        and "EXEC-01" in getattr(corpus_mod, "PROCESS_CHECKS", frozenset())
        and getattr(corpus_mod, "UNIMPLEMENTED_CORPUS_CHECKS", None) == frozenset()
        and getattr(corpus_mod, "UNIMPLEMENTED_CORPUS_CHECKS", frozenset()).isdisjoint(
            getattr(corpus_mod, "PROCESS_CHECKS", frozenset())
        ),
    )

    with tempfile.TemporaryDirectory() as tmp_str:
        tmp = pathlib.Path(tmp_str)
        stub_ok = make_stub_pyrefly(tmp / "bin", mode="c09")

        # Finding #1: REPO-03 config filename normalization (before.pyproject.toml / after.pyproject.toml)
        c_cfg = tmp / "repo03-pyproject"
        write_case(
            c_cfg,
            "C-01",
            meta_toml=build_meta_toml(
                MetaSpec(
                    case_id="C-01",
                    title="disallowed ruff ignore in pyproject.toml",
                    category="blanket-suppression",
                    language="python",
                    before_file="before.pyproject.toml",
                    after_file="after.pyproject.toml",
                    evaluation_mode="after",
                    origin_files=("scripts/pyproject.toml",),
                    planned_checks=("REPO-03",),
                    active_checks=("REPO-03",),
                    currently_caught=True,
                    current_method="REPO-03",
                    expected_signal="config.pyproject.ruff-ignore-disallowed",
                )
            ),
            files={
                "before.pyproject.toml": '[tool.ruff.lint]\nignore = ["E501"]\n',
                "after.pyproject.toml": '[tool.ruff.lint]\nignore = ["TRY003"]\n',
            },
        )
        res = run_corpus_cli(c_cfg, min_cases=1)
        check(
            "REPO-03 normalizes before.pyproject.toml / after.pyproject.toml and catches violation",
            res.returncode == EXIT_OK and "CORPUS CAPTURE RATE: 1/1 (100.0%)" in res.stdout,
            f"rc={res.returncode}\nstdout={res.stdout}\nstderr={res.stderr}",
        )

        # Finding #4: multi-check active_checks with expected_signals dict + PROCESS_CHECKS in planned_checks
        c_multi = tmp / "multi-active"
        write_case(
            c_multi,
            "C-01",
            meta_toml=build_meta_toml(
                MetaSpec(
                    case_id="C-01",
                    title="override drift + file-level noqa caught by two active checks",
                    category="signature-override-drift",
                    language="python",
                    before_file="before.py",
                    after_file="after.py",
                    evaluation_mode="after",
                    origin_files=("recap-subworker/sample.py",),
                    planned_checks=("pyrefly", "REPO-03", "VERIFY-01", "EXEC-01"),
                    active_checks=("pyrefly", "REPO-03"),
                    currently_caught=True,
                    current_method="pyrefly+REPO-03",
                    expected_signal="",
                    expected_signals={
                        "pyrefly": "bad-override-param-name",
                        "REPO-03": "source.broad.missing-adr",
                    },
                )
            ),
            files={
                "before.py": (
                    "class Base:\n"
                    "    def format(self, format: str) -> str:\n"
                    "        return format\n\n"
                    "class Child(Base):\n"
                    "    def format(self, format: str) -> str:\n"
                    "        return format.upper()\n"
                ),
                "after.py": (
                    "# ruff: noqa: TRY003\n"
                    "class Base:\n"
                    "    def format(self, format: str) -> str:\n"
                    "        return format\n\n"
                    "class Child(Base):\n"
                    "    def format(self, fmt: str) -> str:\n"
                    "        return fmt.upper()\n"
                ),
            },
        )
        res = run_corpus_cli(c_multi, min_cases=1, extra_env={"PYREFLY_BIN": str(stub_ok)})
        check(
            "multi-check active_checks with expected_signals and PROCESS_CHECKS exits 0",
            res.returncode == EXIT_OK and "CORPUS CAPTURE RATE: 1/1 (100.0%)" in res.stdout,
            f"rc={res.returncode}\nstdout={res.stdout}\nstderr={res.stderr}",
        )

        # Finding #4: multi-check active_checks without expected_signals dict must be rejected
        c_multi_bad = tmp / "multi-active-no-dict"
        write_case(
            c_multi_bad,
            "C-01",
            meta_toml=build_meta_toml(
                MetaSpec(
                    case_id="C-01",
                    planned_checks=("pyrefly", "REPO-03"),
                    active_checks=("pyrefly", "REPO-03"),
                    currently_caught=True,
                    expected_signal="bad-override-param-name",
                )
            ),
            files={"before.py": "x = 1\n", "after.py": "x = 2\n"},
        )
        res = run_corpus_cli(c_multi_bad, min_cases=1, extra_env={"PYREFLY_BIN": str(stub_ok)})
        check(
            "multi-check active_checks with only scalar expected_signal is rejected",
            res.returncode == EXIT_FAIL and "expected_signals" in res.stderr,
            res.stdout + res.stderr,
        )


def test_diff_mode_runner() -> None:
    sys.stdout.write("diff mode check runner\n")
    corpus_mod = load_corpus_module()

    with tempfile.TemporaryDirectory() as tmp_str:
        tmp = pathlib.Path(tmp_str)

        # Finding #4: diff-mode check runner via evaluate_case(..., extra_runners=...)
        c_diff = tmp / "diff-mode"
        case_dir = write_case(
            c_diff,
            "C-01",
            meta_toml=build_meta_toml(
                MetaSpec(
                    case_id="C-01",
                    title="diff-mode check catches regression while (before, before) passes",
                    evaluation_mode="diff",
                    planned_checks=("REPO-03",),
                    active_checks=("REPO-03",),
                    currently_caught=True,
                    expected_signal="DIFF-DROPPED-GUARD",
                )
            ),
            files={"before.py": "x = 1\n", "after.py": "x = 2\n"},
        )
        loaded_case, load_errs = corpus_mod.load_case(case_dir)
        assert loaded_case is not None, repr(load_errs)
        assert not load_errs, repr(load_errs)

        registered_cls = getattr(corpus_mod, "RegisteredRunner", None)
        if registered_cls is not None:

            def _diff_catch(
                _case: object,
                left_path: pathlib.Path,
                right_path: pathlib.Path,
                _repo_root: pathlib.Path,
            ) -> object:
                if left_path.read_bytes() == right_path.read_bytes():
                    return corpus_mod.CheckOutcome(exit_code=0, output="clean")
                return corpus_mod.CheckOutcome(exit_code=1, output="DIFF-DROPPED-GUARD")

            def _diff_fp(
                _case: object,
                _left_path: pathlib.Path,
                _right_path: pathlib.Path,
                _repo_root: pathlib.Path,
            ) -> object:
                return corpus_mod.CheckOutcome(exit_code=1, output="spurious diff error")

            ev_ok = corpus_mod.evaluate_case(
                loaded_case,
                ROOT,
                extra_runners={"REPO-03": registered_cls(mode="diff", run_diff=_diff_catch)},
            )
            ev_fp = corpus_mod.evaluate_case(
                loaded_case,
                ROOT,
                extra_runners={"REPO-03": registered_cls(mode="diff", run_diff=_diff_fp)},
            )
            check(
                "diff-mode runner passes (before, before) and catches (before, after)",
                ev_ok.status == "caught",
                repr(ev_ok),
            )
            check(
                "diff-mode runner flags (before, before) non-zero as false-positive",
                ev_fp.status == "fail" and "false-positive" in ev_fp.detail,
                repr(ev_fp),
            )
        else:
            check("RegisteredRunner available for diff-mode evaluation", False, "missing class")


def test_directory_symlinks_and_timeout() -> None:
    sys.stdout.write("directory symlink rejection and subprocess timeout\n")
    corpus_mod = load_corpus_module()

    with tempfile.TemporaryDirectory() as tmp_str:
        tmp = pathlib.Path(tmp_str)

        # Finding #7: symlinked case directory and symlinked corpus_dir are rejected
        c_real_case = tmp / "outside-case"
        write_case(
            c_real_case,
            "C-01",
            meta_toml=build_meta_toml(MetaSpec(case_id="C-01")),
            files={"before.py": "a = 1\n", "after.py": "a = 2\n"},
        )
        c_dir_sym = tmp / "dir-symlink"
        c_dir_sym.mkdir()
        (c_dir_sym / "C-01").symlink_to(c_real_case / "C-01")
        res = run_corpus_cli(c_dir_sym, min_cases=1)
        check(
            "symlinked case directory C-01 inside corpus_dir is rejected",
            res.returncode == EXIT_FAIL,
            res.stdout + res.stderr,
        )

        c_root_sym = tmp / "corpus-root-symlink"
        c_root_sym.symlink_to(c_real_case)
        res = run_corpus_cli(c_root_sym, min_cases=1)
        check(
            "symlinked corpus_dir root is rejected",
            res.returncode == EXIT_FAIL,
            res.stdout + res.stderr,
        )

        # Finding #7: subprocess timeout fails closed
        slow_pyrefly = make_executable(
            tmp / "bin" / "pyrefly-slow",
            f"#!{sys.executable}\nimport time\ntime.sleep(5)\n",
        )
        c_timeout = tmp / "timeout-case"
        add_pyrefly_case(c_timeout, "C-01")
        timeout_case, timeout_errs = corpus_mod.load_case(c_timeout / "C-01")
        assert timeout_case is not None, repr(timeout_errs)
        assert not timeout_errs, repr(timeout_errs)
        prev_env = os.environ.get("PYREFLY_BIN")
        os.environ["PYREFLY_BIN"] = str(slow_pyrefly)
        try:
            if hasattr(corpus_mod, "SUBPROCESS_TIMEOUT_SECONDS"):
                ev_timeout = corpus_mod.evaluate_case(timeout_case, ROOT, timeout=0.2)
                check(
                    "subprocess timeout fails closed with runner-error",
                    ev_timeout.status == "fail" and "timed out" in ev_timeout.detail,
                    repr(ev_timeout),
                )
            else:
                check("SUBPROCESS_TIMEOUT_SECONDS defined on corpus module", False, "missing")
        finally:
            if prev_env is None:
                os.environ.pop("PYREFLY_BIN", None)
            else:
                os.environ["PYREFLY_BIN"] = prev_env


def test_arch02_arch03_arch06_runners() -> None:
    sys.stdout.write("ARCH-02, ARCH-03, and ARCH-06 check runners\n")
    corpus_mod = load_corpus_module()

    expected_arch = frozenset({"ARCH-01", "ARCH-02", "ARCH-03", "ARCH-04", "ARCH-05", "ARCH-06"})
    check(
        "ARCH-01..ARCH-06 are in IMPLEMENTED_CHECKS and removed from UNIMPLEMENTED_CORPUS_CHECKS",
        expected_arch <= getattr(corpus_mod, "IMPLEMENTED_CHECKS", frozenset())
        and expected_arch.isdisjoint(
            getattr(corpus_mod, "UNIMPLEMENTED_CORPUS_CHECKS", frozenset())
        ),
        f"impl={getattr(corpus_mod, 'IMPLEMENTED_CHECKS', None)!r}",
    )

    with tempfile.TemporaryDirectory() as tmp_str:
        tmp = pathlib.Path(tmp_str)

        # 1. ARCH-02 Go (raw http.Server)
        c_go = tmp / "arch02-go"
        write_case(
            c_go,
            "C-01",
            meta_toml=build_meta_toml(
                MetaSpec(
                    case_id="C-01",
                    title="raw http.Server constructor violates ARCH-02",
                    category="unrequested-behavior-change",
                    language="go",
                    before_file="before.go",
                    after_file="after.go",
                    evaluation_mode="after",
                    origin_files=("auth-hub/cmd/auth-hub/main.go",),
                    planned_checks=("ARCH-02",),
                    active_checks=("ARCH-02",),
                    currently_caught=True,
                    current_method="ARCH-02",
                    expected_signal="go-no-raw-http-server",
                )
            ),
            files={
                "before.go": (
                    "package main\n\n"
                    'import "alt/httpserver"\n\n'
                    "func mk() any { return httpserver.New(httpserver.Config{}) }\n"
                ),
                "after.go": (
                    "package main\n\n"
                    'import "net/http"\n\n'
                    'func mk() *http.Server { return &http.Server{Addr: ":8080"} }\n'
                ),
            },
        )
        res = run_corpus_cli(c_go, min_cases=1)
        check(
            "ARCH-02 Go runner catches raw http.Server via ast-grep",
            res.returncode == EXIT_OK and "CORPUS CAPTURE RATE: 1/1 (100.0%)" in res.stdout,
            f"rc={res.returncode}\nstdout={res.stdout}\nstderr={res.stderr}",
        )

        # 2. ARCH-02 fail-closed when AST_GREP_BIN points to a nonexistent path
        res_missing = run_corpus_cli(
            c_go,
            min_cases=1,
            extra_env={"AST_GREP_BIN": str(tmp / "no-such-ast-grep")},
        )
        check(
            "ARCH-02 fails closed when AST_GREP_BIN does not exist",
            res_missing.returncode == EXIT_FAIL and "runner-error" in res_missing.stdout,
            f"rc={res_missing.returncode}\nstdout={res_missing.stdout}\nstderr={res_missing.stderr}",
        )

        # 3. ARCH-02 Python (manual aclose in finally)
        c_py = tmp / "arch02-py"
        write_case(
            c_py,
            "C-01",
            meta_toml=build_meta_toml(
                MetaSpec(
                    case_id="C-01",
                    title="manual aclose inside finally violates ARCH-02",
                    category="type-check-evasion",
                    language="python",
                    before_file="before.py",
                    after_file="after.py",
                    evaluation_mode="after",
                    origin_files=("news-creator/app/handler.py",),
                    planned_checks=("ARCH-02",),
                    active_checks=("ARCH-02",),
                    currently_caught=True,
                    current_method="ARCH-02",
                    expected_signal="python-no-manual-aclose-in-finally",
                )
            ),
            files={
                "before.py": (
                    "from contextlib import aclosing\n\n"
                    "async def f(s):\n"
                    "    async with aclosing(s):\n"
                    "        pass\n"
                ),
                "after.py": (
                    "async def f(s):\n"
                    "    try:\n"
                    "        pass\n"
                    "    finally:\n"
                    "        await s.aclose()\n"
                ),
            },
        )
        res = run_corpus_cli(c_py, min_cases=1)
        check(
            "ARCH-02 Python runner catches manual aclose in finally",
            res.returncode == EXIT_OK and "CORPUS CAPTURE RATE: 1/1 (100.0%)" in res.stdout,
            f"rc={res.returncode}\nstdout={res.stdout}\nstderr={res.stderr}",
        )

        # 4. ARCH-02 Rust (Option<Regex> / Regex::new().ok())
        c_rs = tmp / "arch02-rs"
        write_case(
            c_rs,
            "C-01",
            meta_toml=build_meta_toml(
                MetaSpec(
                    case_id="C-01",
                    title="Regex::new().ok() violates ARCH-02",
                    category="silent-error-swallow",
                    language="rust",
                    before_file="before.rs",
                    after_file="after.rs",
                    evaluation_mode="after",
                    origin_files=("rask-log-forwarder/app/src/domain/redact.rs",),
                    planned_checks=("ARCH-02",),
                    active_checks=("ARCH-02",),
                    currently_caught=True,
                    current_method="ARCH-02",
                    expected_signal="rust-no-silent-option-regex",
                )
            ),
            files={
                "before.rs": 'pub fn ok() -> Result<Regex, regex::Error> { Regex::new("a") }\n',
                "after.rs": 'pub fn bad() -> Option<Regex> { Regex::new("a").ok() }\n',
            },
        )
        res = run_corpus_cli(c_rs, min_cases=1)
        check(
            "ARCH-02 Rust runner catches Option<Regex> / Regex::new().ok()",
            res.returncode == EXIT_OK and "CORPUS CAPTURE RATE: 1/1 (100.0%)" in res.stdout,
            f"rc={res.returncode}\nstdout={res.stdout}\nstderr={res.stderr}",
        )

        # 5. ARCH-03 (check_python_versions.py with phase-matched pyproject.toml + source)
        c_arch03 = tmp / "arch03-py"
        write_case(
            c_arch03,
            "C-01",
            meta_toml=build_meta_toml(
                MetaSpec(
                    case_id="C-01",
                    title="ruff target-version + PEP 758 syntax violates ARCH-03",
                    category="autofix-logic-corruption",
                    language="python",
                    before_file="before.py",
                    after_file="after.py",
                    support_files=("before.pyproject.toml", "after.pyproject.toml"),
                    evaluation_mode="after",
                    origin_files=("scripts/pyproject.toml",),
                    planned_checks=("ARCH-03",),
                    active_checks=("ARCH-03",),
                    currently_caught=True,
                    current_method="ARCH-03",
                    expected_signal="config.python.ruff-target-version-forbidden",
                )
            ),
            files={
                "before.py": (
                    "def f(x: str) -> int:\n"
                    "    try:\n"
                    "        return int(x)\n"
                    "    except (ValueError, KeyError):\n"
                    "        return 0\n"
                ),
                "after.py": (
                    "def f(x: str) -> int:\n"
                    "    try:\n"
                    "        return int(x)\n"
                    "    except ValueError, KeyError:\n"
                    "        return 0\n"
                ),
                "before.pyproject.toml": (
                    '[project]\nname = "s"\nversion = "0.1.0"\nrequires-python = ">=3.13"\n'
                ),
                "after.pyproject.toml": (
                    '[project]\nname = "s"\nversion = "0.1.0"\nrequires-python = ">=3.13"\n'
                    '[tool.ruff]\ntarget-version = "py314"\n'
                ),
            },
        )
        res = run_corpus_cli(c_arch03, min_cases=1)
        check(
            "ARCH-03 runner catches target-version and PEP 758 syntax via check_python_versions.py",
            res.returncode == EXIT_OK and "CORPUS CAPTURE RATE: 1/1 (100.0%)" in res.stdout,
            f"rc={res.returncode}\nstdout={res.stdout}\nstderr={res.stderr}",
        )

        # 6. ARCH-06 (direct sys.modules assignment)
        c_arch06 = tmp / "arch06-py"
        write_case(
            c_arch06,
            "C-01",
            meta_toml=build_meta_toml(
                MetaSpec(
                    case_id="C-01",
                    title="direct sys.modules assignment violates ARCH-06",
                    category="test-isolation-hazard",
                    language="python",
                    before_file="before.py",
                    after_file="after.py",
                    evaluation_mode="after",
                    origin_files=("tag-generator/app/tests/test_a.py",),
                    planned_checks=("ARCH-06",),
                    active_checks=("ARCH-06",),
                    currently_caught=True,
                    current_method="ARCH-06",
                    expected_signal="python-no-direct-sys-modules-assign",
                )
            ),
            files={
                "before.py": (
                    "import sys\n"
                    "def test_ok(monkeypatch) -> None:\n"
                    '    monkeypatch.setitem(sys.modules, "foo", object())\n'
                ),
                "after.py": (
                    'import sys\ndef test_bad() -> None:\n    sys.modules["foo"] = object()\n'
                ),
            },
        )
        res = run_corpus_cli(c_arch06, min_cases=1)
        check(
            "ARCH-06 runner catches direct sys.modules mutation via ast-grep",
            res.returncode == EXIT_OK and "CORPUS CAPTURE RATE: 1/1 (100.0%)" in res.stdout,
            f"rc={res.returncode}\nstdout={res.stdout}\nstderr={res.stderr}",
        )

        # 7. ARCH-06 fail-closed when AST_GREP_BIN points to a nonexistent path
        res_missing_06 = run_corpus_cli(
            c_arch06,
            min_cases=1,
            extra_env={"AST_GREP_BIN": str(tmp / "no-such-ast-grep")},
        )
        check(
            "ARCH-06 fails closed when AST_GREP_BIN does not exist",
            res_missing_06.returncode == EXIT_FAIL and "runner-error" in res_missing_06.stdout,
            f"rc={res_missing_06.returncode}\nstdout={res_missing_06.stdout}\nstderr={res_missing_06.stderr}",
        )


def test_check01_check05_check06_runners() -> None:
    sys.stdout.write("CHECK-01, CHECK-05, and CHECK-06 diff runners\n")
    corpus_mod = load_corpus_module()

    expected_checks = frozenset(
        {
            "CHECK-01",
            "CHECK-02",
            "CHECK-03",
            "CHECK-04",
            "CHECK-05",
            "CHECK-06",
            "CHECK-07",
        }
    )
    check(
        "CHECK-01..CHECK-07 are in IMPLEMENTED_CHECKS and CHECK-01/05/06 in DIFF_CHECKS",
        expected_checks <= getattr(corpus_mod, "IMPLEMENTED_CHECKS", frozenset())
        and frozenset({"CHECK-01", "CHECK-05", "CHECK-06"})
        <= getattr(corpus_mod, "DIFF_CHECKS", frozenset())
        and expected_checks.isdisjoint(
            getattr(corpus_mod, "UNIMPLEMENTED_CORPUS_CHECKS", frozenset())
        ),
        f"impl={getattr(corpus_mod, 'IMPLEMENTED_CHECKS', None)!r}",
    )

    with tempfile.TemporaryDirectory() as tmp_str:
        tmp = pathlib.Path(tmp_str)

        # 1. CHECK-01 (Python control-flow return drop)
        c_01 = tmp / "check01-py"
        write_case(
            c_01,
            "C-01",
            meta_toml=build_meta_toml(
                MetaSpec(
                    case_id="C-01",
                    title="dropping else return caught by CHECK-01",
                    category="control-flow-drop",
                    language="python",
                    before_file="before.py",
                    after_file="after.py",
                    evaluation_mode="diff",
                    origin_files=("news-creator/app/sample.py",),
                    planned_checks=("CHECK-01",),
                    active_checks=("CHECK-01",),
                    currently_caught=True,
                    current_method="CHECK-01",
                    expected_signal="python.control_flow_return_change",
                )
            ),
            files={
                "before.py": (
                    "def acquire(flag: bool) -> int:\n"
                    "    if flag:\n"
                    "        return 1\n"
                    "    else:\n"
                    "        return 2\n"
                ),
                "after.py": ("def acquire(flag: bool) -> int:\n    if flag:\n        return 1\n"),
            },
        )
        res = run_corpus_cli(c_01, min_cases=1)
        check(
            "CHECK-01 runner catches Python control-flow return drop via behavior_screener.py",
            res.returncode == EXIT_OK and "CORPUS CAPTURE RATE: 1/1 (100.0%)" in res.stdout,
            f"rc={res.returncode}\nstdout={res.stdout}\nstderr={res.stderr}",
        )

        # 2. CHECK-05 (hand-edited DO NOT EDIT generated file)
        c_05 = tmp / "check05-codegen"
        write_case(
            c_05,
            "C-01",
            meta_toml=build_meta_toml(
                MetaSpec(
                    case_id="C-01",
                    title="hand-edited DO NOT EDIT generated Go file caught by CHECK-05",
                    category="generated-code-tampering",
                    language="go",
                    before_file="before.go",
                    after_file="after.go",
                    evaluation_mode="diff",
                    origin_files=(
                        "rag-orchestrator/internal/adapter/rag_http/openapi/server.gen.go",
                    ),
                    planned_checks=("CHECK-05",),
                    active_checks=("CHECK-05",),
                    currently_caught=True,
                    current_method="CHECK-05",
                    expected_signal="codegen.do-not-edit-modified",
                )
            ),
            files={
                "before.go": "// Code generated by oapi-codegen DO NOT EDIT.\npackage openapi\n\nconst V = 1\n",
                "after.go": "// Code generated by oapi-codegen DO NOT EDIT.\npackage openapi\n\nconst V = 2\n",
            },
        )
        res = run_corpus_cli(c_05, min_cases=1)
        check(
            "CHECK-05 runner catches DO NOT EDIT tampering via check_generated_code.py",
            res.returncode == EXIT_OK and "CORPUS CAPTURE RATE: 1/1 (100.0%)" in res.stdout,
            f"rc={res.returncode}\nstdout={res.stdout}\nstderr={res.stderr}",
        )

        # 3. CHECK-06 (0-byte tracked truncation + untracked ELF binary)
        c_06 = tmp / "check06-hygiene"
        write_case(
            c_06,
            "C-01",
            meta_toml=build_meta_toml(
                MetaSpec(
                    case_id="C-01",
                    title="0-byte tracked file truncation caught by CHECK-06",
                    category="worktree-hygiene",
                    language="python",
                    before_file="before.py",
                    after_file="after.py",
                    evaluation_mode="diff",
                    commits=("worktree-uncommitted",),
                    origin_files=("recap-subworker/recap_subworker/app/routers/runs.py",),
                    planned_checks=("CHECK-06",),
                    active_checks=("CHECK-06",),
                    currently_caught=True,
                    current_method="CHECK-06",
                    expected_signal="hygiene.zero-byte-tracked",
                )
            ),
            files={"before.py": "def router() -> str:\n    return 'ok'\n", "after.py": b""},
        )
        write_case(
            c_06,
            "C-02",
            meta_toml=build_meta_toml(
                MetaSpec(
                    case_id="C-02",
                    title="untracked ELF binary caught by CHECK-06",
                    category="worktree-hygiene",
                    language="binary",
                    before_file="before.go",
                    after_file="after.elf",
                    evaluation_mode="after",
                    commits=("worktree-uncommitted",),
                    origin_files=("alt-backend/app/alt-backend",),
                    planned_checks=("CHECK-06",),
                    active_checks=("CHECK-06",),
                    currently_caught=True,
                    current_method="CHECK-06",
                    expected_signal="hygiene.binary-artifact",
                )
            ),
            files={
                "before.go": "package main\n\nfunc main() {}\n",
                "after.elf": b"\x7fELF\x02\x01\x01\x00" + (b"\x00" * 56),
            },
        )
        res = run_corpus_cli(c_06, min_cases=2)
        check(
            "CHECK-06 runner catches 0-byte truncation and untracked ELF via check_worktree_hygiene.py",
            res.returncode == EXIT_OK and "CORPUS CAPTURE RATE: 2/2 (100.0%)" in res.stdout,
            f"rc={res.returncode}\nstdout={res.stdout}\nstderr={res.stderr}",
        )


def test_verify01_runner() -> None:
    sys.stdout.write("VERIFY-01 sequence characterization diff runner\n")
    corpus_mod = load_corpus_module()

    expected_process_impl = frozenset(
        {"VERIFY-01", "LOOP-01", "LOOP-02", "LOOP-03", "EXEC-01", "EXEC-02", "EXEC-03"}
    )
    check(
        "VERIFY-01/LOOP-01..03/EXEC-01..03 are in IMPLEMENTED_CHECKS and VERIFY-01 in DIFF_CHECKS",
        expected_process_impl <= getattr(corpus_mod, "IMPLEMENTED_CHECKS", frozenset())
        and "VERIFY-01" in getattr(corpus_mod, "DIFF_CHECKS", frozenset())
        and "VERIFY-01" in getattr(corpus_mod, "PROCESS_CHECKS", frozenset())
        and expected_process_impl.isdisjoint(
            getattr(corpus_mod, "UNIMPLEMENTED_CHECKS", frozenset())
        ),
        f"impl={getattr(corpus_mod, 'IMPLEMENTED_CHECKS', None)!r}, "
        f"unimpl={getattr(corpus_mod, 'UNIMPLEMENTED_CHECKS', None)!r}",
    )

    with tempfile.TemporaryDirectory() as tmp_str:
        tmp = pathlib.Path(tmp_str)

        char_test = (
            "#!/usr/bin/env bash\n"
            "set -euo pipefail\n"
            'dir="$(cd "$(dirname "$0")" && pwd)"\n'
            'out="$("$dir/check-compose-variables.sh")"\n'
            '[[ "$out" == "ok-no-env-file" ]]\n'
        )
        before_sh = "#!/usr/bin/env bash\nset -euo pipefail\necho ok-no-env-file\n"
        after_sh = (
            "#!/usr/bin/env bash\n"
            "set -euo pipefail\n"
            'echo "ERROR: --env-file is required" >&2\n'
            "exit 2\n"
        )

        # 1. Clean pass on before, SEQUENCE_FINAL_TEST_FAILED on after
        c_v01 = tmp / "verify01-ok"
        case_dir = write_case(
            c_v01,
            "C-01",
            meta_toml=build_meta_toml(
                MetaSpec(
                    case_id="C-01",
                    title="unrequested CLI breaking change caught by VERIFY-01",
                    category="unrequested-behavior-change",
                    language="shell",
                    before_file="before.sh",
                    after_file="after.sh",
                    support_files=("test_characterization.sh",),
                    evaluation_mode="diff",
                    origin_files=("scripts/check-compose-variables.sh",),
                    planned_checks=("VERIFY-01",),
                    active_checks=("VERIFY-01",),
                    currently_caught=True,
                    current_method="VERIFY-01",
                    expected_signal="SEQUENCE_FINAL_TEST_FAILED",
                )
            ),
            files={
                "before.sh": before_sh,
                "after.sh": after_sh,
                "test_characterization.sh": char_test,
            },
        )
        res = run_corpus_cli(c_v01, min_cases=1)
        check(
            "VERIFY-01 runner passes on before and catches regression with SEQUENCE_FINAL_TEST_FAILED",
            res.returncode == EXIT_OK and "CORPUS CAPTURE RATE: 1/1 (100.0%)" in res.stdout,
            f"rc={res.returncode}\nstdout={res.stdout}\nstderr={res.stderr}",
        )

        # 2. Direct _run_verify01_diff call checks (baseline pass + regression signal)
        run_v01 = getattr(corpus_mod, "_run_verify01_diff", None)
        if callable(run_v01):
            loaded, errs = corpus_mod.load_case(case_dir)
            assert loaded is not None, repr(errs)
            base_out = run_v01(loaded, case_dir / "before.sh", case_dir / "before.sh", ROOT)
            reg_out = run_v01(loaded, case_dir / "before.sh", case_dir / "after.sh", ROOT)
            check(
                "_run_verify01_diff returns exit 0 on (before, before) and SEQUENCE_FINAL_TEST_FAILED on (before, after)",
                base_out.exit_code == 0
                and not base_out.runner_error
                and reg_out.exit_code != 0
                and "SEQUENCE_FINAL_TEST_FAILED" in reg_out.output,
                f"base={base_out!r}, reg={reg_out!r}",
            )
        else:
            check("_run_verify01_diff helper defined on corpus module", False, "missing")

        # 3. False-positive detection when characterization test fails on baseline (before)
        c_fp = tmp / "verify01-fp"
        write_case(
            c_fp,
            "C-01",
            meta_toml=build_meta_toml(
                MetaSpec(
                    case_id="C-01",
                    title="broken characterization test fails on baseline",
                    category="unrequested-behavior-change",
                    language="shell",
                    before_file="before.sh",
                    after_file="after.sh",
                    support_files=("test_characterization.sh",),
                    evaluation_mode="diff",
                    origin_files=("scripts/check-compose-variables.sh",),
                    planned_checks=("VERIFY-01",),
                    active_checks=("VERIFY-01",),
                    currently_caught=True,
                    current_method="VERIFY-01",
                    expected_signal="SEQUENCE_FINAL_TEST_FAILED",
                )
            ),
            files={
                "before.sh": before_sh,
                "after.sh": after_sh,
                "test_characterization.sh": "#!/usr/bin/env bash\nexit 1\n",
            },
        )
        res_fp = run_corpus_cli(c_fp, min_cases=1)
        out_fp = res_fp.stdout + res_fp.stderr
        check(
            "VERIFY-01 flags baseline test failure as false-positive with SEQUENCE_BASELINE_TEST_FAILED",
            res_fp.returncode == EXIT_FAIL
            and "false-positive" in out_fp
            and "SEQUENCE_BASELINE_TEST_FAILED" in out_fp,
            f"rc={res_fp.returncode}\nout={out_fp}",
        )

        # 4. Missing test script in support_files fails closed with runner-error
        c_missing = tmp / "verify01-missing-test"
        write_case(
            c_missing,
            "C-01",
            meta_toml=build_meta_toml(
                MetaSpec(
                    case_id="C-01",
                    title="missing characterization test fails closed",
                    category="unrequested-behavior-change",
                    language="shell",
                    before_file="before.sh",
                    after_file="after.sh",
                    support_files=(),
                    evaluation_mode="diff",
                    origin_files=("scripts/check-compose-variables.sh",),
                    planned_checks=("VERIFY-01",),
                    active_checks=("VERIFY-01",),
                    currently_caught=True,
                    current_method="VERIFY-01",
                    expected_signal="SEQUENCE_FINAL_TEST_FAILED",
                )
            ),
            files={"before.sh": before_sh, "after.sh": after_sh},
        )
        res_missing = run_corpus_cli(c_missing, min_cases=1)
        out_missing = res_missing.stdout + res_missing.stderr
        check(
            "VERIFY-01 fails closed with runner-error when support_files has no test script",
            res_missing.returncode == EXIT_FAIL and "runner-error" in out_missing,
            f"rc={res_missing.returncode}\nout={out_missing}",
        )

        # 5. Delegation to task_packet.py verify-sequence when supported
        if callable(run_v01):
            fake_root = tmp / "fake-root"
            stub_tp = fake_root / "scripts" / "agent-harness" / "task_packet.py"
            stub_tp.parent.mkdir(parents=True, exist_ok=True)
            stub_tp.write_text(
                "import sys\n"
                "args = sys.argv[1:]\n"
                "if '--help' in args:\n"
                "    print('subcommands: verify-sequence')\n"
                "    sys.exit(0)\n"
                "assert args[0] == 'verify-sequence', args\n"
                "assert '--repo-root' in args and '--base-commit' in args and '--test-command' in args, args\n"
                "print('DELEGATED_OK ' + ' '.join(args))\n"
                "sys.exit(0)\n",
                encoding="utf-8",
            )
            loaded, _ = corpus_mod.load_case(case_dir)
            assert loaded is not None
            delegated = run_v01(loaded, case_dir / "before.sh", case_dir / "before.sh", fake_root)
            check(
                "_run_verify01_diff delegates to task_packet.py verify-sequence when available",
                delegated.exit_code == 0
                and "DELEGATED_OK verify-sequence" in delegated.output
                and "bash ./test_characterization.sh" in delegated.output,
                repr(delegated),
            )


def test_real_corpus(*, allow_missing_corpus_dir: bool) -> None:
    sys.stdout.write("real repository corpus verification\n")
    has_cases = REAL_CORPUS_DIR.is_dir() and any(REAL_CORPUS_DIR.iterdir())
    if not has_cases:
        if allow_missing_corpus_dir:
            sys.stdout.write(
                "  NOTE  scripts/harness/corpus is missing or empty; "
                "skipping real corpus check (--allow-missing-corpus-dir)\n"
            )
            return
        check(
            "real corpus directory scripts/harness/corpus exists and is non-empty",
            False,
            f"missing or empty: {REAL_CORPUS_DIR}",
        )
        return

    res = run_corpus_cli(REAL_CORPUS_DIR, min_cases=DEFAULT_MIN_CASES)
    if res.stdout:
        sys.stdout.write(res.stdout)
    if res.stderr:
        sys.stderr.write(res.stderr)
    caught_count = 0
    for line in res.stdout.splitlines():
        if line.startswith("CORPUS CAPTURE RATE: "):
            ratio = line.removeprefix("CORPUS CAPTURE RATE: ").split(" ", 1)[0]
            numerator, _, _denominator = ratio.partition("/")
            if numerator.isdigit():
                caught_count = int(numerator)
    check(
        f"real repository corpus passes (>= {DEFAULT_MIN_CASES} cases, >= {MIN_CAUGHT_CASES} caught)",
        res.returncode == EXIT_OK
        and "CORPUS CAPTURE RATE:" in res.stdout
        and "failures=0]" in res.stdout
        and caught_count >= MIN_CAUGHT_CASES,
        f"rc={res.returncode}, caught={caught_count}",
    )


def parse_args(argv: list[str] | None = None) -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="Run CORPUS-01 unit tests and real repository corpus verification.",
    )
    parser.add_argument(
        "--allow-missing-corpus-dir",
        action="store_true",
        default=False,
        help="Allow missing/empty scripts/harness/corpus during isolated engine development.",
    )
    return parser.parse_args(argv)


def main(argv: list[str] | None = None) -> int:
    args = parse_args(argv)
    if not CORPUS_SCRIPT.is_file():
        sys.stderr.write(f"FAIL: corpus engine missing: {CORPUS_SCRIPT}\n")
        return EXIT_FAIL

    test_capture_rate_and_outcomes()
    test_pyrefly_and_repo02_runners()
    test_schema_and_security_rejections()
    test_special_valid_cases_and_full_22_fixture()
    test_repo03_config_and_multi_check()
    test_diff_mode_runner()
    test_directory_symlinks_and_timeout()
    test_arch02_arch03_arch06_runners()
    test_check01_check05_check06_runners()
    test_verify01_runner()
    test_real_corpus(allow_missing_corpus_dir=args.allow_missing_corpus_dir)

    sys.stdout.write(f"\n{RESULTS.passed} passed, {RESULTS.failed} failed\n")
    return EXIT_FAIL if RESULTS.failed else EXIT_OK


if __name__ == "__main__":
    sys.exit(main())
