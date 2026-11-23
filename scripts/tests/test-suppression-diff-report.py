#!/usr/bin/env python3
"""Tests for scripts/lint/suppression_diff_report.py (CHECK-03 PR Job Summary).

Verifies:
1. Diffing <base_rev> vs working tree and <base_rev> vs --head <rev>.
2. Extracting config allowlist, broad source, and inline source suppressions before and after.
3. Line-number shifts do not produce spurious added/removed entries.
4. GFM Markdown table includes totals, net delta, Added Suppressions (path:line, kind/rule,
   reason, expires), and Removed Suppressions.
5. --summary-file appends cleanly to an existing summary file ($GITHUB_STEP_SUMMARY).

Run:
    python3 scripts/tests/test-suppression-diff-report.py
"""

from __future__ import annotations

import os
import pathlib
import shutil
import subprocess
import sys
import tempfile

ROOT = pathlib.Path(__file__).resolve().parents[2]
SCRIPT = ROOT / "scripts" / "lint" / "suppression_diff_report.py"
GIT_BIN = shutil.which("git")
assert GIT_BIN is not None, "git executable not found in PATH"


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


def git_env() -> dict[str, str]:
    env = os.environ.copy()
    env.update(
        {
            "GIT_AUTHOR_NAME": "Test Author",
            "GIT_AUTHOR_EMAIL": "test@example.com",
            "GIT_COMMITTER_NAME": "Test Committer",
            "GIT_COMMITTER_EMAIL": "test@example.com",
        }
    )
    return env


def run_git(repo: pathlib.Path, *args: str) -> str:
    assert GIT_BIN is not None
    res = subprocess.run(  # noqa: S603 - fixed argv built in this test
        [GIT_BIN, "-C", str(repo), *args],
        env=git_env(),
        capture_output=True,
        text=True,
        check=True,
    )
    return res.stdout.strip()


def run_cli(*args: str) -> subprocess.CompletedProcess[str]:
    return subprocess.run(  # noqa: S603 - fixed argv built in this test
        [sys.executable, str(SCRIPT), *args],
        capture_output=True,
        text=True,
        check=False,
    )


ALLOWLIST_V1 = """\
version = 1

[ruff]
allowed_global_ignores = ["E501"]
allowed_test_per_file_ignores = ["S101"]

[[bandit]]
path = "svc/pyproject.toml"
status = "frozen-baseline"
reason = "Old bandit skip"
expires = "2027-05-22"
exclude_dirs = ["tests"]
skips = ["B101"]
"""

ALLOWLIST_V2 = """\
version = 1

[ruff]
allowed_global_ignores = ["E501"]
allowed_test_per_file_ignores = ["S101"]

[[bandit]]
path = "svc/pyproject.toml"
status = "frozen-baseline"
reason = "Old bandit skip"
expires = "2027-05-22"
exclude_dirs = ["tests"]
skips = ["B101"]

[[cargo_lint_allows]]
path = "crate/Cargo.toml"
section = "lints.clippy"
adr = "ADR-000979"
reason = "Approved pedantic allow"
expires = "2027-06-01"
allows = ["doc_markdown"]
"""


def test_working_tree_and_head_diff() -> None:
    sys.stdout.write("suppression diff report generation\n")
    if not SCRIPT.is_file():
        check("suppression_diff_report.py exists", False, str(SCRIPT))
        return

    with tempfile.TemporaryDirectory() as tmp_str:
        repo = pathlib.Path(tmp_str) / "repo"
        repo.mkdir()
        run_git(repo, "init")

        al = repo / "scripts" / "lint" / "suppression_allowlist.toml"
        al.parent.mkdir(parents=True)
        al.write_text(ALLOWLIST_V1, encoding="utf-8")

        py_file = repo / "app.py"
        py_file.write_text(
            "x = 1  # noqa: F401 -- removed later\ny = 2  # noqa: S603 -- shifted line stays\n",
            encoding="utf-8",
        )
        go_file = repo / "main.go"
        go_file.write_text(
            "package main\nfunc f() { _ = a() // #nosec G304 -- stays unchanged\n}\n",
            encoding="utf-8",
        )
        run_git(repo, "add", ".")
        run_git(repo, "commit", "-m", "base")
        base_sha = run_git(repo, "rev-parse", "HEAD")

        # Working tree changes:
        # - Add 1 config allowlist entry (cargo_lint_allows)
        # - Remove 1 inline python_noqa (F401), shift S603 down 2 lines (no diff)
        # - Add 1 inline python_noqa (S608) + 1 broad Rust mod allow (with expires)
        al.write_text(ALLOWLIST_V2, encoding="utf-8")
        py_file.write_text(
            "# comment\n# comment 2\n"
            "y = 2  # noqa: S603 -- shifted line stays\n"
            'q = f"DELETE FROM {t}"  # noqa: S608 -- identifier (trusted constant)\n',
            encoding="utf-8",
        )
        rs_file = repo / "lib.rs"
        rs_file.write_text(
            "// ADR-000902: generated bindings require allow (expires: 2027-05-22)\n"
            "#[allow(clippy::too_many_arguments)]\n"
            "mod bindings {}\n",
            encoding="utf-8",
        )

        summary_file = pathlib.Path(tmp_str) / "step_summary.md"
        summary_file.write_text("# Existing Header\n", encoding="utf-8")

        res = run_cli(
            "--repo-root",
            str(repo),
            "--base",
            base_sha,
            "--summary-file",
            str(summary_file),
        )
        check("working tree diff exits 0", res.returncode == 0, res.stdout + res.stderr)
        out = res.stdout
        check(
            "markdown table includes Total row and +2 net delta",
            "| **Total**" in out and "+2" in out,
            out,
        )
        check(
            "added inline python_noqa S608 reported with path:line",
            "`app.py:4`" in out and "S608" in out,
            out,
        )
        check(
            "added broad Rust allow reported with expires 2027-05-22",
            "`lib.rs:2`" in out and "2027-05-22" in out,
            out,
        )
        check(
            "added config allowlist entry reported with expires 2027-06-01",
            "crate/Cargo.toml" in out and "2027-06-01" in out,
            out,
        )
        check("removed inline F401 reported", "removed later" in out and "F401" in out, out)
        check(
            "shifted unchanged S603 is NOT reported as added or removed",
            "shifted line stays" not in out,
            out,
        )

        summary_text = summary_file.read_text(encoding="utf-8")
        check(
            "--summary-file appends while preserving existing content",
            summary_text.startswith("# Existing Header\n")
            and "Suppression Governance Diff Report" in summary_text,
            summary_text,
        )

        # Commit changes and test --head <rev> mode
        run_git(repo, "add", ".")
        run_git(repo, "commit", "-m", "head")
        head_sha = run_git(repo, "rev-parse", "HEAD")

        res_head = run_cli(
            "--repo-root",
            str(repo),
            "--base",
            base_sha,
            "--head",
            head_sha,
        )
        check(
            "--head mode matches working tree diff",
            res_head.returncode == 0
            and "`app.py:4`" in res_head.stdout
            and "`lib.rs:2`" in res_head.stdout,
            res_head.stdout + res_head.stderr,
        )
        _verify_security_and_fail_closed(pathlib.Path(tmp_str), repo, al, base_sha, head_sha)


def _verify_security_and_fail_closed(
    tmp: pathlib.Path,
    repo: pathlib.Path,
    al: pathlib.Path,
    base_sha: str,
    head_sha: str,
) -> None:
    clobber_target = tmp / "clobbered.txt"
    res_flag_base = run_cli("--repo-root", str(repo), f"--base=--output={clobber_target}")
    check(
        "rejects --base starting with '-' (CWE-88) without creating output file",
        res_flag_base.returncode != 0 and not clobber_target.exists(),
        f"rc={res_flag_base.returncode} stderr={res_flag_base.stderr}",
    )
    res_flag_head = run_cli(
        "--repo-root", str(repo), "--base", base_sha, f"--head=--output={clobber_target}"
    )
    check(
        "rejects --head starting with '-' (CWE-88) without creating output file",
        res_flag_head.returncode != 0 and not clobber_target.exists(),
        f"rc={res_flag_head.returncode} stderr={res_flag_head.stderr}",
    )

    outside_py = tmp / "outside_secret.py"
    outside_py.write_text("x = 1  # noqa: S603 -- leaked outside secret\n", encoding="utf-8")
    (repo / "escape_link.py").symlink_to(outside_py)
    res_sym = run_cli("--repo-root", str(repo), "--base", head_sha)
    check(
        "symlink escaping repo_root is not read in working tree diff",
        res_sym.returncode == 0 and "leaked outside secret" not in res_sym.stdout,
        res_sym.stdout + res_sym.stderr,
    )
    (repo / "escape_link.py").unlink()

    al.write_text("version = 1\n[[bandit\nbroken toml\n", encoding="utf-8")
    res_bad_toml = run_cli("--repo-root", str(repo), "--base", head_sha)
    check(
        "corrupted working-tree suppression_allowlist.toml fails closed (non-zero)",
        res_bad_toml.returncode != 0,
        f"rc={res_bad_toml.returncode} out={res_bad_toml.stdout + res_bad_toml.stderr}",
    )


def main() -> int:
    test_working_tree_and_head_diff()
    sys.stdout.write(f"\n{RESULTS.passed} passed, {RESULTS.failed} failed\n")
    return 1 if RESULTS.failed else 0


if __name__ == "__main__":
    sys.exit(main())
