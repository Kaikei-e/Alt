#!/usr/bin/env python3
"""Tests for scripts/agent-harness/adr_supersede_gate.py.

When an ADR adds a `supersedes` edge without naming the target in its own body,
the target silently drops out of `docdag query --binding` even though the
superseding document never argued for replacing it (see ADR-001011 / 001012).

Run:
    python3 scripts/tests/test-adr-supersede-gate.py
"""

from __future__ import annotations

import contextlib
import importlib.util
import io
import os
import pathlib
import shutil
import subprocess
import sys
import tempfile

ROOT = pathlib.Path(__file__).resolve().parents[2]
GATE_PATH = ROOT / "scripts" / "agent-harness" / "adr_supersede_gate.py"
GIT_BIN = shutil.which("git")
assert GIT_BIN is not None, "git executable not found in PATH"

WIDENED_FINDING_COUNT = 54
LIST_FORM_FINDING_COUNT = 2
USAGE_EXIT_CODE = 2

spec = importlib.util.spec_from_file_location("adr_supersede_gate", GATE_PATH)
assert spec is not None
assert spec.loader is not None
gate = importlib.util.module_from_spec(spec)
spec.loader.exec_module(gate)


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


def init_repo(tmp: pathlib.Path) -> pathlib.Path:
    repo = tmp / "repo"
    repo.mkdir()
    run_git(repo, "init")
    (repo / "docs" / "ADR").mkdir(parents=True)
    return repo


def write_adr(
    repo: pathlib.Path,
    adr_id: str,
    supersedes_block: str,
    body: str,
) -> pathlib.Path:
    path = repo / "docs" / "ADR" / f"{adr_id}.md"
    lines = [
        "---",
        f"title: ADR {adr_id}",
        "date: 2026-10-24",
        "status: accepted",
    ]
    if supersedes_block:
        lines.append(supersedes_block)
    lines.extend(["---", body])
    path.write_text("\n".join(lines) + "\n", encoding="utf-8")
    return path


def invoke_gate(repo: pathlib.Path, base: str) -> tuple[int, str, str]:
    stdout_buf = io.StringIO()
    stderr_buf = io.StringIO()
    with (
        contextlib.redirect_stdout(stdout_buf),
        contextlib.redirect_stderr(stderr_buf),
    ):
        rc = gate.main(["--base", base, "--dir", str(repo / "docs" / "ADR")])
    return rc, stdout_buf.getvalue(), stderr_buf.getvalue()


sys.stdout.write("1. new ADR superseding one named ADR passes\n")
with tempfile.TemporaryDirectory() as td:
    repo = init_repo(pathlib.Path(td))
    write_adr(repo, "000001", "", "# ADR-000001\n\nInitial.")
    run_git(repo, "add", ".")
    run_git(repo, "commit", "-m", "base")
    base_rev = run_git(repo, "rev-parse", "HEAD")

    write_adr(
        repo,
        "000002",
        'supersedes: ["000001"]',
        "# ADR-000002\n\nReplaces [[000001]] because it is obsolete.",
    )
    rc, out, err = invoke_gate(repo, base_rev)
    check("exit code is 0", rc == 0, f"rc={rc}, out={out!r}, err={err!r}")
    check(
        "summary reports 1 ADR and 000001",
        "1" in out and "000001" in out and "Summary:" in out,
        out,
    )

sys.stdout.write("2. new ADR superseding an unnamed ADR reports one finding\n")
with tempfile.TemporaryDirectory() as td:
    repo = init_repo(pathlib.Path(td))
    write_adr(repo, "000001", "", "# ADR-000001\n\nInitial.")
    run_git(repo, "add", ".")
    run_git(repo, "commit", "-m", "base")
    base_rev = run_git(repo, "rev-parse", "HEAD")

    write_adr(
        repo,
        "000002",
        'supersedes: ["000001"]',
        "# ADR-000002\n\nMentions 000001 without wikilink.",
    )
    rc, out, err = invoke_gate(repo, base_rev)
    expected_finding = (
        "ADR 000002 adds supersedes: 000001 but its body never references [[000001]]. "
        "A supersedes edge removes 000001 from the binding set (docdag query --binding). "
        "Name 000001 in the body with the reason it is replaced, or use depends-on, "
        "or drop the edge. See ADR-001011."
    )
    check("exit code is 1", rc == 1, f"rc={rc}")
    check("exact finding message present", expected_finding in out, out)
    check("summary reports 1 ADR", "1" in out and "000001" in out, out)

sys.stdout.write(
    "3. 2026-10-24 shape: 6 named -> widened to 60 without body change = 54 findings\n"
)
with tempfile.TemporaryDirectory() as td:
    repo = init_repo(pathlib.Path(td))
    base_ids = [f"{i:06d}" for i in range(1, 7)]
    extra_ids = [f"{i:06d}" for i in range(7, 61)]
    all_ids = base_ids + extra_ids
    for aid in all_ids:
        write_adr(repo, aid, "", f"# ADR-{aid}\n\nBody.")
    named_body = "# ADR-000940\n\nRetires " + " ".join(f"[[{i}]]" for i in base_ids)
    base_list = ", ".join(f'"{i}"' for i in base_ids)
    write_adr(repo, "000940", f"supersedes: [{base_list}]", named_body)
    run_git(repo, "add", ".")
    run_git(repo, "commit", "-m", "base with 6 supersedes")
    base_rev = run_git(repo, "rev-parse", "HEAD")

    wide_list = ", ".join(f'"{i}"' for i in all_ids)
    write_adr(repo, "000940", f"supersedes: [{wide_list}]", named_body)
    rc, out, err = invoke_gate(repo, base_rev)
    finding_lines = [
        line for line in out.splitlines() if line.startswith("ADR 000940 adds supersedes:")
    ]
    summary_lines = [line for line in out.splitlines() if line.startswith("Summary:")]
    check("exit code is 1", rc == 1, f"rc={rc}")
    check(
        "exactly 54 findings",
        len(finding_lines) == WIDENED_FINDING_COUNT,
        f"got {len(finding_lines)}",
    )
    check(
        "summary line reports 54",
        len(summary_lines) == 1 and "54" in summary_lines[0],
        f"summary={summary_lines!r}",
    )

sys.stdout.write("4. existing edge at base not named in body produces no finding\n")
with tempfile.TemporaryDirectory() as td:
    repo = init_repo(pathlib.Path(td))
    write_adr(repo, "000010", "", "# ADR-000010\n\nOld.")
    write_adr(
        repo,
        "000011",
        'supersedes: ["000010"]',
        "# ADR-000011\n\nNo wikilink here.",
    )
    run_git(repo, "add", ".")
    run_git(repo, "commit", "-m", "base")
    base_rev = run_git(repo, "rev-parse", "HEAD")

    # Modify body slightly without adding new supersedes edge.
    write_adr(
        repo,
        "000011",
        'supersedes: ["000010"]',
        "# ADR-000011\n\nStill no wikilink here, just edited text.",
    )
    rc, out, err = invoke_gate(repo, base_rev)
    check("exit code is 0", rc == 0, f"rc={rc}, out={out!r}")
    check("summary reports 0", "0" in out and "Summary:" in out, out)

sys.stdout.write("5. inline list and block list forms, quoted, bare, and wikilink entries\n")
with tempfile.TemporaryDirectory() as td:
    repo = init_repo(pathlib.Path(td))
    for i in range(1, 9):
        write_adr(repo, f"{i:06d}", "", f"# ADR-{i:06d}\n\nBase.")
    run_git(repo, "add", ".")
    run_git(repo, "commit", "-m", "base")
    base_rev = run_git(repo, "rev-parse", "HEAD")

    write_adr(
        repo,
        "000100",
        'supersedes: [000001, "000002", "[[000003]]", [[000004]]]',
        "# ADR-000100\n\nReplaces [[000001]], [[000002]], [[000003]]. Missing 4.",
    )
    write_adr(
        repo,
        "000101",
        'supersedes:\n  - 000005\n  - "000006"\n  - "[[000007]]"\n  - [[000008]]',
        "# ADR-000101\n\nReplaces [[000005]], [[000006]], [[000007]]. Missing 8.",
    )
    rc, out, err = invoke_gate(repo, base_rev)
    finding_lines = [line for line in out.splitlines() if " adds supersedes: " in line]
    check("exit code is 1", rc == 1, f"rc={rc}")
    check(
        "only unnamed 000004 and 000008 reported",
        len(finding_lines) == LIST_FORM_FINDING_COUNT
        and any("000100 adds supersedes: 000004" in ln for ln in finding_lines)
        and any("000101 adds supersedes: 000008" in ln for ln in finding_lines),
        f"findings={finding_lines!r}",
    )

sys.stdout.write("6. [[DST|label]] in body counts as a reference\n")
with tempfile.TemporaryDirectory() as td:
    repo = init_repo(pathlib.Path(td))
    write_adr(repo, "000224", "", "# ADR-000224\n\nOld.")
    run_git(repo, "add", ".")
    run_git(repo, "commit", "-m", "base")
    base_rev = run_git(repo, "rev-parse", "HEAD")

    write_adr(
        repo,
        "000226",
        'supersedes: ["000224"]',
        "# ADR-000226\n\n[[000224|ADR-224]] is replaced by this decision.",
    )
    rc, out, err = invoke_gate(repo, base_rev)
    check("exit code is 0", rc == 0, f"rc={rc}, out={out!r}")

sys.stdout.write("7. unknown base revision exits 2 (CLI subprocess check)\n")
with tempfile.TemporaryDirectory() as td:
    repo = init_repo(pathlib.Path(td))
    write_adr(repo, "000001", "", "# ADR-000001\n\nBase.")
    run_git(repo, "add", ".")
    run_git(repo, "commit", "-m", "base")

    res = subprocess.run(  # noqa: S603 - fixed argv built in this test
        [
            sys.executable,
            str(GATE_PATH),
            "--base",
            "nonexistent-revision-xyz",
            "--dir",
            str(repo / "docs" / "ADR"),
        ],
        capture_output=True,
        text=True,
        check=False,
    )
    check("exit code is 2", res.returncode == USAGE_EXIT_CODE, f"rc={res.returncode}")
    check(
        "clear message on unknown rev",
        bool((res.stderr + res.stdout).strip()),
        f"out={res.stdout!r}, err={res.stderr!r}",
    )

sys.stdout.write(f"\n{RESULTS.passed} passed, {RESULTS.failed} failed\n")
sys.exit(1 if RESULTS.failed else 0)
