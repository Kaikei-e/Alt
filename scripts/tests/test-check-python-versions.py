#!/usr/bin/env python3
"""Unit and integration tests for scripts/lint/check_python_versions.py (ARCH-03).

Run:
    python3 scripts/tests/test-check-python-versions.py
"""

from __future__ import annotations

import os
import pathlib
import subprocess
import sys
import tempfile
import types
from collections.abc import Sequence
from typing import Protocol

ROOT = pathlib.Path(__file__).resolve().parents[2]
SCRIPT = ROOT / "scripts" / "lint" / "check_python_versions.py"
CORPUS_C05 = ROOT / "scripts" / "harness" / "corpus" / "C-05"


class _HasRule(Protocol):
    @property
    def rule(self) -> str: ...


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


def rules_of(violations: Sequence[_HasRule]) -> set[str]:
    return {v.rule for v in violations}


def run_cli(*args: str) -> subprocess.CompletedProcess[str]:
    env = os.environ.copy()
    env["GIT_OPTIONAL_LOCKS"] = "0"
    return subprocess.run(  # noqa: S603 - invoking local validator script with fixed args
        [sys.executable, str(SCRIPT), *args],
        env=env,
        capture_output=True,
        text=True,
        check=False,
    )


def _test_ruff_target_version(checker: types.ModuleType, tmp: pathlib.Path) -> None:
    clean_toml = tmp / "clean.pyproject.toml"
    clean_toml.write_text(
        '[project]\nname = "demo"\nrequires-python = ">=3.14"\n\n[tool.ruff]\nline-length = 100\n',
        encoding="utf-8",
    )
    v = checker.check_pyproject(clean_toml, repo_root=tmp)
    check("clean pyproject has 0 violations", len(v) == 0, f"got {v!r}")

    for label, section in (
        ("[tool.ruff].target-version", "[tool.ruff]"),
        ("[tool.ruff.lint].target-version", "[tool.ruff.lint]"),
        ("[tool.ruff.format].target-version", "[tool.ruff.format]"),
    ):
        toml_path = tmp / f"{section.strip('[]').replace('.', '_')}.pyproject.toml"
        toml_path.write_text(
            f'[project]\nname = "demo"\nrequires-python = ">=3.14"\n\n'
            f'{section}\ntarget-version = "py314"\n',
            encoding="utf-8",
        )
        v = checker.check_pyproject(toml_path, repo_root=tmp)
        check(
            f"rejects {label}",
            rules_of(v) == {"config.python.ruff-target-version-forbidden"},
            f"got {v!r}",
        )


def _test_requires_python_and_scripts(checker: types.ModuleType, tmp: pathlib.Path) -> None:
    missing_req = tmp / "missing.pyproject.toml"
    missing_req.write_text('[project]\nname = "demo"\n', encoding="utf-8")
    v = checker.check_pyproject(missing_req, repo_root=tmp)
    check(
        "rejects missing [project].requires-python",
        "config.python.requires-python-missing" in rules_of(v),
        f"got {v!r}",
    )

    bad_req = tmp / "bad.pyproject.toml"
    bad_req.write_text(
        '[project]\nname = "demo"\nrequires-python = "not-a-version"\n',
        encoding="utf-8",
    )
    v = checker.check_pyproject(bad_req, repo_root=tmp)
    check(
        "rejects unextractable requires-python",
        "config.python.requires-python-invalid" in rules_of(v),
        f"got {v!r}",
    )

    scripts_dir = tmp / "scripts"
    scripts_dir.mkdir()
    scripts_toml = scripts_dir / "pyproject.toml"
    scripts_toml.write_text(
        '[project]\nname = "alt-scripts"\nrequires-python = ">=3.14"\n',
        encoding="utf-8",
    )
    v = checker.check_pyproject(scripts_toml, repo_root=tmp)
    check(
        "rejects scripts/pyproject.toml with >=3.14 floor",
        "config.python.scripts-requires-python-mismatch" in rules_of(v),
        f"got {v!r}",
    )

    scripts_toml.write_text(
        '[project]\nname = "alt-scripts"\nrequires-python = ">=3.13"\n',
        encoding="utf-8",
    )
    v = checker.check_pyproject(scripts_toml, repo_root=tmp)
    check("accepts scripts/pyproject.toml with >=3.13 floor", len(v) == 0, f"got {v!r}")


def _test_syntax_and_pep758(checker: types.ModuleType, tmp: pathlib.Path) -> None:
    pep758_bad = tmp / "pep758_bad.py"
    pep758_bad.write_text(
        "def f():\n"
        "    try:\n"
        "        return 1\n"
        "    except ValueError, KeyError:\n"
        "        return 0\n",
        encoding="utf-8",
    )
    v = checker.check_source_syntax(pep758_bad, (3, 13))
    check(
        "rejects PEP 758 except A, B: under 3.13 floor",
        "source.python.syntax-incompatible" in rules_of(v),
        f"got {v!r}",
    )

    pep758_star_bad = tmp / "pep758_star_bad.py"
    pep758_star_bad.write_text(
        "def f():\n"
        "    try:\n"
        "        return 1\n"
        "    except* ValueError, KeyError as exc:\n"
        "        return exc\n",
        encoding="utf-8",
    )
    v = checker.check_source_syntax(pep758_star_bad, (3, 13))
    check(
        "rejects PEP 758 except* A, B as exc: under 3.13 floor",
        "source.python.syntax-incompatible" in rules_of(v),
        f"got {v!r}",
    )

    token_hits = checker.find_unparenthesized_except_lines(
        "try:\n    pass\nexcept ValueError, KeyError:\n    pass\n"
    )
    check(
        "token detector catches unparenthesized except on line 3",
        token_hits == [3],
        f"got {token_hits!r}",
    )

    pep758_ok = tmp / "pep758_ok.py"
    pep758_ok.write_text(
        'MSG = "except ValueError, KeyError:"\n'
        "# except ValueError, KeyError:\n"
        "def f():\n"
        "    try:\n"
        "        return 1\n"
        "    except (ValueError, KeyError) as exc:\n"
        "        return exc\n",
        encoding="utf-8",
    )
    v = checker.check_source_syntax(pep758_ok, (3, 13))
    check(
        "accepts parenthesized except and ignores strings/comments",
        len(v) == 0,
        f"got {v!r}",
    )

    v_future = checker.check_source_syntax(pep758_ok, (3, 99))
    check(
        "never silently skips ast.parse when floor exceeds current runtime",
        "source.python.runtime-too-old" in rules_of(v_future),
        f"got {v_future!r}",
    )

    future_toml = tmp / "future.pyproject.toml"
    future_toml.write_text(
        '[project]\nname = "future"\nrequires-python = ">=3.99"\n',
        encoding="utf-8",
    )
    future_res = run_cli("--pyproject", str(future_toml), "--source", str(pep758_ok))
    check(
        "CLI fails closed when no Python >= floor interpreter is available",
        future_res.returncode == 1,
        f"rc={future_res.returncode} out={future_res.stdout!r} err={future_res.stderr!r}",
    )


def _test_dockerfiles(checker: types.ModuleType, tmp: pathlib.Path) -> None:
    svc_dir = tmp / "svc" / "app"
    svc_dir.mkdir(parents=True)
    svc_toml = svc_dir / "pyproject.toml"
    svc_toml.write_text(
        '[project]\nname = "svc"\nrequires-python = ">=3.14,<3.15"\n',
        encoding="utf-8",
    )
    svc_df = tmp / "svc" / "Dockerfile"
    svc_df.write_text(
        "FROM python:3.14-slim@sha256:deadbeef AS builder\n"
        "FROM python:3.14-slim@sha256:deadbeef AS runtime\n",
        encoding="utf-8",
    )
    (tmp / "svc" / "Dockerfile.backend").write_text(
        "FROM ollama/ollama:0.40.12\n",
        encoding="utf-8",
    )
    v = checker.check_project_dir(svc_dir, repo_root=tmp)
    check("matching parent Dockerfile passes for */app service", len(v) == 0, f"got {v!r}")

    svc_df.write_text(
        "FROM python:3.14-slim AS builder\nFROM python:3.13-slim AS runtime\n",
        encoding="utf-8",
    )
    v = checker.check_project_dir(svc_dir, repo_root=tmp)
    check(
        "mismatched Dockerfile stage fails with dockerfile-version-mismatch",
        rules_of(v) == {"config.python.dockerfile-version-mismatch"},
        f"got {v!r}",
    )


def _test_corpus_and_repo() -> None:
    before_res = run_cli(
        "--pyproject",
        str(CORPUS_C05 / "before.pyproject.toml"),
        "--source",
        str(CORPUS_C05 / "before.py"),
    )
    check(
        "CLI passes on C-05 before.pyproject.toml + before.py",
        before_res.returncode == 0,
        f"rc={before_res.returncode} out={before_res.stdout!r} err={before_res.stderr!r}",
    )

    after_res = run_cli(
        "--pyproject",
        str(CORPUS_C05 / "after.pyproject.toml"),
        "--source",
        str(CORPUS_C05 / "after.py"),
    )
    after_combined = f"{after_res.stdout}\n{after_res.stderr}"
    check(
        "CLI fails on C-05 after.pyproject.toml + after.py with both config & syntax violations",
        after_res.returncode == 1
        and "config.python.ruff-target-version-forbidden" in after_combined
        and "source.python.syntax-incompatible" in after_combined,
        f"rc={after_res.returncode} out={after_combined!r}",
    )

    after_src_only = run_cli(
        "--pyproject",
        str(CORPUS_C05 / "before.pyproject.toml"),
        "--source",
        str(CORPUS_C05 / "after.py"),
    )
    after_src_combined = f"{after_src_only.stdout}\n{after_src_only.stderr}"
    check(
        "CLI catches C-05 after.py PEP 758 syntax even when pyproject is clean (>=3.13)",
        after_src_only.returncode == 1
        and "source.python.syntax-incompatible" in after_src_combined,
        f"rc={after_src_only.returncode} out={after_src_combined!r}",
    )

    repo_res = run_cli("--repo-root", str(ROOT))
    repo_combined = f"{repo_res.stdout}\n{repo_res.stderr}"
    check(
        "real repository passes check_python_versions.py",
        repo_res.returncode == 0,
        f"rc={repo_res.returncode} out={repo_combined[:400]!r}",
    )


def main() -> int:
    if not SCRIPT.is_file():
        sys.stderr.write(f"FAIL: validator script missing: {SCRIPT}\n")
        return 1

    lint_dir = str(SCRIPT.parent)
    if lint_dir not in sys.path:
        sys.path.insert(0, lint_dir)
    import check_python_versions as checker  # noqa: PLC0415 - loaded after existence check

    with tempfile.TemporaryDirectory(prefix="alt-pyver-test-") as tmp_str:
        tmp = pathlib.Path(tmp_str)
        _test_ruff_target_version(checker, tmp)
        _test_requires_python_and_scripts(checker, tmp)
        _test_syntax_and_pep758(checker, tmp)
        _test_dockerfiles(checker, tmp)

    _test_corpus_and_repo()

    sys.stdout.write(
        f"\ncheck_python_versions tests: {RESULTS.passed} passed, {RESULTS.failed} failed\n"
    )
    return 0 if RESULTS.failed == 0 else 1


if __name__ == "__main__":
    raise SystemExit(main())
