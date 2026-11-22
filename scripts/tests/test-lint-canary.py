#!/usr/bin/env python3
"""TDD tests for REPO-02 static-analysis canary self-audit and 0-file scan detection.

Verifies:
1. Go (`scripts/lint/go.sh` + `scripts/lint/canary_audit.py`):
   - Healthy scanners pass canary audit and non-zero file scan verification.
   - Broken `gosec` or `golangci-lint` returning exit 0 with 0 findings fails with
     `CANARY AUDIT FAILED` and exit 1.
   - Broken `gosec` exiting non-zero without expected SARIF rule fails canary audit.
   - `gosec` reporting `Files : 0` (or package load errors) on non-generated `.go` files
     fails with `ZERO-FILE SCAN DETECTED` and exit 1.
   - Generated-only `.go` modules do not false-fail 0-file scan detection.
   - Project working tree and `./_reports/gosec.sarif` remain completely uncontaminated.
   - Real cached Go scanners pass canary audit against `pki-agent`.
2. Python (`scripts/lint/python.sh` + `scripts/lint/canary_audit.py`):
   - Healthy `ruff` (+ `bandit` when declared) pass canary audit and file check.
   - Broken `ruff` or `bandit` returning exit 0 with 0 findings fails with
     `CANARY AUDIT FAILED` and exit 1.
   - Project containing only `.venv`/`build`/`dist`/`gen` `.py` files fails with
     `ZERO-FILE SCAN DETECTED` and exit 1.
   - Broken `bandit` scanning 0 lines on a non-empty Python project fails with exit 1.
   - Projects without `bandit` in dev dependencies skip bandit canary cleanly.
   - Paths like `tag-generator/app/tag_generator/main.py` are not falsely excluded.
   - Real `uv`/`ruff`/`bandit` pass canary audit against `scripts` and `tag-generator/app`
     even though `tag-generator/app/pyproject.toml` skips `B602`.

Run:
    cd scripts && uv run --frozen --only-group dev python tests/test-lint-canary.py
"""

from __future__ import annotations

import json
import os
import pathlib
import shutil
import stat
import subprocess
import sys
import tempfile
from dataclasses import dataclass

ROOT = pathlib.Path(__file__).resolve().parents[2]
LINT_DIR = ROOT / "scripts" / "lint"
GO_SH = LINT_DIR / "go.sh"
PYTHON_SH = LINT_DIR / "python.sh"
CANARY_AUDIT_PY = LINT_DIR / "canary_audit.py"
GOLANGCI_YML = ROOT / ".golangci.yml"

GOLANGCI_LINT_VERSION = "v2.14.0"
GOVULNCHECK_VERSION = "v1.6.0"

BASH_BIN = shutil.which("bash")
GO_BIN = shutil.which("go")
UV_BIN = shutil.which("uv")

assert BASH_BIN is not None, "bash executable not found in PATH"
assert GO_BIN is not None, "go executable not found in PATH"


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


def make_executable(path: pathlib.Path, content: str) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(content, encoding="utf-8")
    current_mode = path.stat().st_mode
    path.chmod(current_mode | stat.S_IXUSR | stat.S_IXGRP | stat.S_IXOTH)


def detect_go_toolchain_version() -> str:
    assert GO_BIN is not None
    res = subprocess.run(  # noqa: S603 - fixed argv querying local go toolchain
        [GO_BIN, "env", "GOVERSION"],
        cwd=str(ROOT),
        capture_output=True,
        text=True,
        check=True,
    )
    version = res.stdout.strip()
    assert version, "go env GOVERSION returned empty string"
    return version


def snapshot_tree(directory: pathlib.Path) -> set[str]:
    entries: set[str] = set()
    for root_dir, dirnames, filenames in directory.walk():
        dirnames[:] = [d for d in dirnames if d != ".venv"]
        for fname in filenames:
            rel = (root_dir / fname).relative_to(directory)
            entries.add(rel.as_posix())
    return entries


def build_go_test_repo(
    tmp: pathlib.Path,
    *,
    golangci_mode: str = "healthy",
) -> tuple[pathlib.Path, pathlib.Path, dict[str, str]]:
    """Build an isolated mini-repo and fake XDG_CACHE_HOME Go linter binaries (no standalone gosec)."""
    repo = tmp / "repo"
    lint_dir = repo / "scripts" / "lint"
    lint_dir.mkdir(parents=True)
    shutil.copy2(GO_SH, lint_dir / "go.sh")
    if CANARY_AUDIT_PY.is_file():
        shutil.copy2(CANARY_AUDIT_PY, lint_dir / "canary_audit.py")
    shutil.copy2(GOLANGCI_YML, repo / ".golangci.yml")

    svc = repo / "svc-go"
    svc.mkdir()
    (svc / "go.mod").write_text("module svc-go\n\ngo 1.27.4\n", encoding="utf-8")
    (svc / "main.go").write_text("package main\n\nfunc main() {}\n", encoding="utf-8")

    go_ver = detect_go_toolchain_version()
    cache_home = tmp / "cache"
    cache_base = cache_home / "alt-lint" / go_ver

    golangci_script = f"""#!{sys.executable}
import json, pathlib, sys

mode = {golangci_mode!r}
args = sys.argv[1:]
sarif_path = None
for idx, arg in enumerate(args):
    if arg == "--output.sarif.path" and idx + 1 < len(args):
        sarif_path = pathlib.Path(args[idx + 1])
    elif arg.startswith("--output.sarif.path="):
        sarif_path = pathlib.Path(arg.split("=", 1)[1])

def write_sarif(results: list[dict[str, object]]) -> None:
    if sarif_path is None:
        return
    sarif_path.parent.mkdir(parents=True, exist_ok=True)
    payload = {{
        "version": "2.1.0",
        "runs": [{{"tool": {{"driver": {{"name": "golangci-lint"}}}}, "results": results}}],
    }}
    sarif_path.write_text(json.dumps(payload), encoding="utf-8")

if mode == "silent_pass":
    write_sarif([])
    sys.stdout.write("0 issues.\\n")
    sys.exit(0)

cwd = pathlib.Path.cwd()
go_files = sorted(cwd.rglob("*.go"))
is_canary = any(
    "InsecureSkipVerify: true" in p.read_text(encoding="utf-8")
    for p in go_files
)

gosec_result = {{
    "ruleId": "gosec",
    "level": "error",
    "message": {{"text": "G402: TLS InsecureSkipVerify set to true."}},
    "fixes": [{{"description": {{"text": "Set InsecureSkipVerify to false"}}}}],
    "locations": [{{
        "physicalLocation": {{
            "artifactLocation": {{"uri": "canary.go"}},
            "region": {{"startLine": 9}},
        }}
    }}],
}}
ineffassign_result = {{
    "ruleId": "ineffassign",
    "level": "error",
    "message": {{"text": "ineffectual assignment to x"}},
    "locations": [{{
        "physicalLocation": {{
            "artifactLocation": {{"uri": "canary.go"}},
            "region": {{"startLine": 6}},
        }}
    }}],
}}

if is_canary:
    if mode == "canary_crash_empty_sarif":
        write_sarif([])
        sys.stderr.write("error: failed to load packages\\n")
        sys.exit(1)
    if mode == "canary_missing_gosec":
        write_sarif([ineffassign_result])
        sys.stdout.write("canary.go:6:2: ineffectual assignment to x (ineffassign)\\n")
        sys.exit(1)
    if mode == "canary_missing_ineffassign":
        write_sarif([gosec_result])
        sys.stdout.write("canary.go:9:41: G402: TLS InsecureSkipVerify set to true. (gosec)\\n")
        sys.exit(1)
    write_sarif([gosec_result, ineffassign_result])
    sys.stdout.write(
        "canary.go:9:41: G402: TLS InsecureSkipVerify set to true. (gosec)\\n"
        "canary.go:6:2: ineffectual assignment to x (ineffassign)\\n"
    )
    sys.exit(1)

if mode == "mixed_findings":
    write_sarif([
        {{
            "ruleId": "gosec",
            "level": "error",
            "message": {{"text": "G402: TLS InsecureSkipVerify set to true."}},
            "fixes": [{{"description": {{"text": "remove"}}}}],
            "locations": [{{
                "physicalLocation": {{
                    "artifactLocation": {{"uri": "main.go", "index": 0}},
                    "region": {{"startLine": 3}},
                }}
            }}],
        }},
        {{
            "ruleId": "gosec",
            "level": "error",
            "message": {{"text": "G402: TLS InsecureSkipVerify set to true.\\nsecond line"}},
            "locations": [{{
                "physicalLocation": {{
                    "artifactLocation": {{"uri": "other.go", "index": 1}},
                    "region": {{"startLine": 9}},
                }}
            }}],
        }},
        {{
            "ruleId": "errcheck",
            "level": "error",
            "message": {{"text": "Error return value of `foo` is not checked"}},
            "locations": [{{
                "physicalLocation": {{
                    "artifactLocation": {{"uri": "main.go", "index": 0}},
                    "region": {{"startLine": 4}},
                }}
            }}],
        }},
    ])
    sys.stdout.write(
        "main.go:3:1: G402: TLS InsecureSkipVerify set to true. (gosec)\\n"
        "other.go:9:1: G402: TLS InsecureSkipVerify set to true. (gosec)\\n"
        "main.go:4:1: Error return value is not checked (errcheck)\\n"
    )
    sys.exit(1)

write_sarif([])
sys.stdout.write("0 issues.\\n")
sys.exit(0)
"""

    govulncheck_script = f"""#!{sys.executable}
import json, sys
sys.stdout.write(json.dumps({{
    "version": "2.1.0",
    "runs": [{{"tool": {{"driver": {{"name": "govulncheck", "rules": []}}}}, "results": []}}],
}}))
sys.exit(0)
"""

    make_executable(
        cache_base / f"golangci-lint-{GOLANGCI_LINT_VERSION}" / "golangci-lint",
        golangci_script,
    )
    make_executable(
        cache_base / f"govulncheck-{GOVULNCHECK_VERSION}" / "govulncheck",
        govulncheck_script,
    )

    env = os.environ.copy()
    env["XDG_CACHE_HOME"] = str(cache_home)
    return repo, svc, env


def run_go_sh(
    repo: pathlib.Path, target: str, env: dict[str, str]
) -> subprocess.CompletedProcess[str]:
    assert BASH_BIN is not None
    return subprocess.run(  # noqa: S603 - fixed argv running repo script under test
        [BASH_BIN, str(repo / "scripts" / "lint" / "go.sh"), target],
        cwd=str(repo),
        env=env,
        capture_output=True,
        text=True,
        check=False,
    )


@dataclass(frozen=True)
class PythonRepoSpec:
    """Configuration for constructing an isolated Python test repository."""

    with_bandit: bool = True
    ruff_mode: str = "healthy"
    bandit_mode: str = "healthy"
    create_app_py: bool = True
    project_rel: str = "svc-py"


def build_python_test_repo(
    tmp: pathlib.Path,
    spec: PythonRepoSpec | None = None,
) -> tuple[pathlib.Path, pathlib.Path, dict[str, str]]:
    """Build an isolated Python project and fake `uv` shim on PATH."""
    cfg = spec if spec is not None else PythonRepoSpec()
    repo = tmp / "repo"
    lint_dir = repo / "scripts" / "lint"
    lint_dir.mkdir(parents=True)
    shutil.copy2(PYTHON_SH, lint_dir / "python.sh")
    if CANARY_AUDIT_PY.is_file():
        shutil.copy2(CANARY_AUDIT_PY, lint_dir / "canary_audit.py")

    svc = repo / cfg.project_rel
    svc.mkdir(parents=True)

    dev_deps = ['  "ruff>=0.15.0",']
    if cfg.with_bandit:
        dev_deps.append('  "bandit>=1.8.0",')
    pyproject_lines = [
        "[project]",
        'name = "svc-py"',
        'version = "0.1.0"',
        'requires-python = ">=3.12"',
        "",
        "[dependency-groups]",
        "dev = [",
        *dev_deps,
        "]",
        "",
        "[tool.ruff.lint]",
        'select = ["E", "F", "S"]',
        "",
        "[tool.bandit]",
        'skips = ["B602", "B101"]',
    ]
    (svc / "pyproject.toml").write_text("\n".join(pyproject_lines) + "\n", encoding="utf-8")

    if cfg.create_app_py:
        (svc / "app.py").write_text(
            'def greet() -> str:\n    return "ok"\n',
            encoding="utf-8",
        )

    bin_dir = tmp / "stub-bin"
    uv_script = f"""#!{sys.executable}
import os, pathlib, subprocess, sys

args = sys.argv[1:]
# Strip leading uv run flags to locate the subtool invocation.
idx = 0
if idx < len(args) and args[idx] == "run":
    idx += 1
while idx < len(args) and args[idx].startswith("-"):
    if args[idx] in ("--only-group", "--group"):
        idx += 2
    else:
        idx += 1

if idx >= len(args):
    sys.stderr.write("stub uv: missing command\\n")
    sys.exit(2)

tool = args[idx]
rest = args[idx + 1:]

if tool == "python":
    res = subprocess.run([sys.executable, *rest], check=False)
    sys.exit(res.returncode)

if tool == "ruff":
    sub = rest[0] if rest else ""
    if sub == "format":
        sys.stdout.write("1 file already formatted\\n")
        sys.exit(0)
    mode = os.environ.get("MOCK_RUFF_MODE", "healthy")
    if mode == "silent_pass":
        sys.stdout.write("All checks passed!\\n")
        sys.exit(0)
    # Inspect positional path arguments for canary violation.
    targets: list[pathlib.Path] = []
    i = 1
    while i < len(rest):
        tok = rest[i]
        if tok in ("--config", "--cache-dir"):
            i += 2
            continue
        if tok.startswith("-"):
            i += 1
            continue
        targets.append(pathlib.Path(tok))
        i += 1
    for target in targets:
        py_files = [target] if target.is_file() else list(target.rglob("*.py"))
        for py_file in py_files:
            if not py_file.is_file():
                continue
            text = py_file.read_text(encoding="utf-8")
            if "import os" in text:
                sys.stdout.write(f"F401 `os` imported but unused\\n --> {{py_file}}:1:8\\n")
                sys.exit(1)
    sys.stdout.write("All checks passed!\\n")
    sys.exit(0)

if tool == "bandit":
    mode = os.environ.get("MOCK_BANDIT_MODE", "healthy")
    if mode == "silent_pass":
        sys.stdout.write("Test results:\\n\\tNo issues identified.\\nCode scanned:\\n\\tTotal lines of code: 5\\n")
        sys.exit(0)
    # Find target after -r
    target = pathlib.Path(".")
    for i, tok in enumerate(rest):
        if tok == "-r" and i + 1 < len(rest):
            target = pathlib.Path(rest[i + 1])
            break
    py_files = [target] if target.is_file() else list(target.rglob("*.py"))
    is_canary = any(
        "shell=True" in p.read_text(encoding="utf-8")
        for p in py_files
        if p.is_file()
    )
    if is_canary:
        sys.stdout.write(
            ">> Issue: [B602:subprocess_popen_with_shell_equals_true] shell=True\\n"
            "   Severity: High   Confidence: High\\n"
            "Code scanned:\\n\\tTotal lines of code: 5\\n"
        )
        sys.exit(1)
    if mode == "zero_scan":
        sys.stdout.write(
            "Test results:\\n\\tNo issues identified.\\n"
            "Code scanned:\\n\\tTotal lines of code: 0\\n"
        )
        sys.exit(0)
    sys.stdout.write(
        "Test results:\\n\\tNo issues identified.\\n"
        "Code scanned:\\n\\tTotal lines of code: 2\\n"
    )
    sys.exit(0)

sys.stderr.write(f"stub uv: unexpected tool {{tool}}\\n")
sys.exit(2)
"""
    make_executable(bin_dir / "uv", uv_script)

    env = os.environ.copy()
    env["PATH"] = f"{bin_dir}:{env.get('PATH', '')}"
    env["MOCK_RUFF_MODE"] = cfg.ruff_mode
    env["MOCK_BANDIT_MODE"] = cfg.bandit_mode
    return repo, svc, env


def run_python_sh(
    repo: pathlib.Path,
    target: str,
    env: dict[str, str],
) -> subprocess.CompletedProcess[str]:
    assert BASH_BIN is not None
    return subprocess.run(  # noqa: S603 - fixed argv running repo script under test
        [BASH_BIN, str(repo / "scripts" / "lint" / "python.sh"), target],
        cwd=str(repo),
        env=env,
        capture_output=True,
        text=True,
        check=False,
    )


# ---------------------------------------------------------------------------
# Group A: Go tests
# ---------------------------------------------------------------------------
sys.stdout.write("A1. Go normal run (single-path golangci-lint) passes with zero contamination\n")
with tempfile.TemporaryDirectory() as td:
    repo, svc, env = build_go_test_repo(pathlib.Path(td))
    res = run_go_sh(repo, "svc-go", env)
    sarif_path = svc / "_reports" / "gosec.sarif"
    sarif_data = json.loads(sarif_path.read_text(encoding="utf-8")) if sarif_path.is_file() else {}
    sarif_results = [item for run in sarif_data.get("runs", []) for item in run.get("results", [])]
    driver_names = [
        run.get("tool", {}).get("driver", {}).get("name") for run in sarif_data.get("runs", [])
    ]
    files_after = snapshot_tree(svc)
    check("go.sh exits 0", res.returncode == 0, f"rc={res.returncode}, stderr={res.stderr!r}")
    check(
        "summary includes canary PASS", "canary" in res.stdout and "PASS" in res.stdout, res.stdout
    )
    check(
        "no canary files or raw golangci-lint.sarif left in project dir",
        not any("canary" in p.lower() for p in files_after)
        and not (svc / "_reports" / "golangci-lint.sarif").exists(),
        f"files={sorted(files_after)}",
    )
    driver_rules = [
        run.get("tool", {}).get("driver", {}).get("rules") for run in sarif_data.get("runs", [])
    ]
    check("gosec.sarif driver name is gosec", driver_names == ["gosec"], repr(driver_names))
    check(
        "gosec.sarif driver rules is empty list when 0 findings",
        driver_rules == [[]],
        repr(driver_rules),
    )
    check("gosec.sarif contains 0 canary findings", len(sarif_results) == 0, repr(sarif_results))

sys.stdout.write("A2. Go mixed findings emits clean gosec-only SARIF with G<NNN> ruleId\n")
with tempfile.TemporaryDirectory() as td:
    repo, svc, env = build_go_test_repo(pathlib.Path(td), golangci_mode="mixed_findings")
    res = run_go_sh(repo, "svc-go", env)
    sarif_path = svc / "_reports" / "gosec.sarif"
    sarif_data = json.loads(sarif_path.read_text(encoding="utf-8")) if sarif_path.is_file() else {}
    sarif_results = [item for run in sarif_data.get("runs", []) for item in run.get("results", [])]
    driver_rules = sarif_data.get("runs", [{}])[0].get("tool", {}).get("driver", {}).get("rules")
    has_artifact_index = any(
        "index" in loc.get("physicalLocation", {}).get("artifactLocation", {})
        for item in sarif_results
        for loc in item.get("locations", [])
    )
    expected_gosec_results = 2
    check("go.sh exits 1 on findings", res.returncode == 1, f"rc={res.returncode}")
    check(
        "gosec.sarif keeps only gosec results with G402 ruleId and no fixes",
        len(sarif_results) == expected_gosec_results
        and all(r.get("ruleId") == "G402" and "fixes" not in r for r in sarif_results),
        repr(sarif_results),
    )
    check(
        "gosec.sarif deduplicates tool.driver.rules with first-line shortDescription",
        driver_rules
        == [
            {
                "id": "G402",
                "shortDescription": {"text": "G402: TLS InsecureSkipVerify set to true."},
            }
        ],
        repr(driver_rules),
    )
    check(
        "gosec.sarif strips artifactLocation.index when run.artifacts is absent",
        not has_artifact_index,
        repr(sarif_results),
    )
    check(
        "golangci-lint.sarif removed after conversion",
        not (svc / "_reports" / "golangci-lint.sarif").exists(),
    )

sys.stdout.write("A2b. go-verify-scan rejects SARIF 2.1.0 schema violations\n")
with tempfile.TemporaryDirectory() as td:
    td_path = pathlib.Path(td)
    proj = td_path / "proj"
    proj.mkdir()
    (proj / "go.mod").write_text("module proj\n\ngo 1.27.4\n", encoding="utf-8")
    (proj / "main.go").write_text("package main\n\nfunc main() {}\n", encoding="utf-8")

    missing_rules_sarif = td_path / "missing-rules.sarif"
    missing_rules_sarif.write_text(
        json.dumps(
            {
                "version": "2.1.0",
                "runs": [{"tool": {"driver": {"name": "gosec"}}, "results": []}],
            }
        ),
        encoding="utf-8",
    )
    res_missing_rules = subprocess.run(  # noqa: S603 - fixed argv running canary_audit.py under test
        [
            sys.executable,
            str(CANARY_AUDIT_PY),
            "go-verify-scan",
            "--project-dir",
            str(proj),
            "--gosec-sarif",
            str(missing_rules_sarif),
        ],
        capture_output=True,
        text=True,
        check=False,
    )
    check(
        "go-verify-scan rejects SARIF missing tool.driver.rules",
        res_missing_rules.returncode == 1,
        f"rc={res_missing_rules.returncode}, stderr={res_missing_rules.stderr!r}",
    )

    uncovered_rule_sarif = td_path / "uncovered-rule.sarif"
    uncovered_rule_sarif.write_text(
        json.dumps(
            {
                "version": "2.1.0",
                "runs": [
                    {
                        "tool": {"driver": {"name": "gosec", "rules": []}},
                        "results": [{"ruleId": "G402", "message": {"text": "G402: bad"}}],
                    }
                ],
            }
        ),
        encoding="utf-8",
    )
    res_uncovered = subprocess.run(  # noqa: S603 - fixed argv running canary_audit.py under test
        [
            sys.executable,
            str(CANARY_AUDIT_PY),
            "go-verify-scan",
            "--project-dir",
            str(proj),
            "--gosec-sarif",
            str(uncovered_rule_sarif),
        ],
        capture_output=True,
        text=True,
        check=False,
    )
    check(
        "go-verify-scan rejects SARIF with result ruleId absent from tool.driver.rules",
        res_uncovered.returncode == 1,
        f"rc={res_uncovered.returncode}, stderr={res_uncovered.stderr!r}",
    )

    dangling_index_sarif = td_path / "dangling-index.sarif"
    dangling_index_sarif.write_text(
        json.dumps(
            {
                "version": "2.1.0",
                "runs": [
                    {
                        "tool": {
                            "driver": {
                                "name": "gosec",
                                "rules": [{"id": "G402", "shortDescription": {"text": "G402"}}],
                            }
                        },
                        "results": [
                            {
                                "ruleId": "G402",
                                "message": {"text": "G402: bad"},
                                "locations": [
                                    {
                                        "physicalLocation": {
                                            "artifactLocation": {"uri": "main.go", "index": 0}
                                        }
                                    }
                                ],
                            }
                        ],
                    }
                ],
            }
        ),
        encoding="utf-8",
    )
    res_dangling = subprocess.run(  # noqa: S603 - fixed argv running canary_audit.py under test
        [
            sys.executable,
            str(CANARY_AUDIT_PY),
            "go-verify-scan",
            "--project-dir",
            str(proj),
            "--gosec-sarif",
            str(dangling_index_sarif),
        ],
        capture_output=True,
        text=True,
        check=False,
    )
    check(
        "go-verify-scan rejects SARIF with artifactLocation.index when run.artifacts is absent",
        res_dangling.returncode == 1,
        f"rc={res_dangling.returncode}, stderr={res_dangling.stderr!r}",
    )

sys.stdout.write("A3. Go broken golangci-lint (silent exit 0) fails canary audit\n")
with tempfile.TemporaryDirectory() as td:
    repo, svc, env = build_go_test_repo(pathlib.Path(td), golangci_mode="silent_pass")
    res = run_go_sh(repo, "svc-go", env)
    files_after = snapshot_tree(svc)
    check("go.sh exits 1", res.returncode == 1, f"rc={res.returncode}, stdout={res.stdout!r}")
    check("stderr reports CANARY AUDIT FAILED", "CANARY AUDIT FAILED" in res.stderr, res.stderr)
    check(
        "no canary files or contaminated sarif left",
        not any("canary" in p.lower() for p in files_after)
        and not (svc / "_reports" / "gosec.sarif").exists(),
        f"files={sorted(files_after)}",
    )

sys.stdout.write("A4. Go canary missing G402 or ineffassign in SARIF fails canary audit\n")
for mode_name in ("canary_crash_empty_sarif", "canary_missing_gosec", "canary_missing_ineffassign"):
    with tempfile.TemporaryDirectory() as td:
        repo, svc, env = build_go_test_repo(pathlib.Path(td), golangci_mode=mode_name)
        res = run_go_sh(repo, "svc-go", env)
        check(f"{mode_name}: go.sh exits 1", res.returncode == 1, f"rc={res.returncode}")
        check(
            f"{mode_name}: stderr reports CANARY AUDIT FAILED",
            "CANARY AUDIT FAILED" in res.stderr,
            res.stderr,
        )

sys.stdout.write("A5. Go single-path go-verify-scan fails when go list resolves 0 packages\n")
with tempfile.TemporaryDirectory() as td:
    td_path = pathlib.Path(td)
    zero_pkg_mod = td_path / "zero-pkg"
    zero_pkg_mod.mkdir()
    (zero_pkg_mod / "go.mod").write_text("module zero-pkg\n\ngo 1.27.4\n", encoding="utf-8")
    (zero_pkg_mod / "ignored.go").write_text(
        "//go:build ignore\n\npackage main\n\nfunc main() {}\n",
        encoding="utf-8",
    )
    gosec_sarif = td_path / "gosec.sarif"
    gosec_sarif.write_text(
        '{"version":"2.1.0","runs":[{"tool":{"driver":{"name":"gosec","rules":[]}},"results":[]}]}',
        encoding="utf-8",
    )
    res = subprocess.run(  # noqa: S603 - fixed argv running canary_audit.py under test
        [
            sys.executable,
            str(CANARY_AUDIT_PY),
            "go-verify-scan",
            "--project-dir",
            str(zero_pkg_mod),
            "--gosec-sarif",
            str(gosec_sarif),
        ],
        capture_output=True,
        text=True,
        check=False,
    )
    check(
        "single-path go-verify-scan exits 1 on 0 Go packages",
        res.returncode == 1,
        f"rc={res.returncode}, stdout={res.stdout!r}",
    )
    check(
        "stderr reports ZERO-FILE SCAN DETECTED",
        "ZERO-FILE SCAN DETECTED" in res.stderr,
        res.stderr,
    )

sys.stdout.write("A6. Backward-compatible go-verify-scan --gosec-out detects Files : 0 (C-21)\n")
with tempfile.TemporaryDirectory() as td:
    td_path = pathlib.Path(td)
    proj = td_path / "proj"
    proj.mkdir()
    (proj / "sample.go").write_text("package sample\n", encoding="utf-8")
    gosec_sarif = td_path / "gosec.sarif"
    gosec_sarif.write_text(
        '{"version":"2.1.0","runs":[{"tool":{"driver":{"name":"gosec","rules":[]}},"results":[]}]}',
        encoding="utf-8",
    )
    for label, content in (
        ("zero_files", "Summary:\n  Files  : 0\n  Lines  : 0\n  Issues : 0\n"),
        (
            "zero_files_load_error",
            "Golang errors in file: [.]:\n  > go.mod requires go >= 1.99.0\n"
            "Summary:\n  Files  : 0\n  Lines  : 0\n  Issues : 0\n",
        ),
    ):
        gosec_out = td_path / f"{label}.out"
        gosec_out.write_text(content, encoding="utf-8")
        res = subprocess.run(  # noqa: S603 - fixed argv running canary_audit.py under test
            [
                sys.executable,
                str(CANARY_AUDIT_PY),
                "go-verify-scan",
                "--project-dir",
                str(proj),
                "--gosec-out",
                str(gosec_out),
                "--gosec-sarif",
                str(gosec_sarif),
            ],
            capture_output=True,
            text=True,
            check=False,
        )
        check(f"{label}: exits 1", res.returncode == 1, f"rc={res.returncode}")
        check(
            f"{label}: stderr reports ZERO-FILE SCAN DETECTED",
            "ZERO-FILE SCAN DETECTED" in res.stderr,
            res.stderr,
        )

sys.stdout.write("A7. Go generated-only module does not false-fail 0-file scan check\n")
with tempfile.TemporaryDirectory() as td:
    gen_mod = pathlib.Path(td) / "gen-only"
    (gen_mod / "gen").mkdir(parents=True)
    (gen_mod / "go.mod").write_text("module gen-only\n\ngo 1.27.4\n", encoding="utf-8")
    (gen_mod / "gen" / "api.go").write_text("package gen\n", encoding="utf-8")
    (gen_mod / "types.pb.go").write_text("package genonly\n", encoding="utf-8")
    (gen_mod / "svc.connect.go").write_text("package genonly\n", encoding="utf-8")
    (gen_mod / "server.gen.go").write_text("package genonly\n", encoding="utf-8")
    gosec_out = pathlib.Path(td) / "gosec.out"
    gosec_out.write_text("Summary:\n  Files  : 0\n  Issues : 0\n", encoding="utf-8")
    gosec_sarif = pathlib.Path(td) / "gosec.sarif"
    gosec_sarif.write_text(
        '{"version":"2.1.0","runs":[{"tool":{"driver":{"name":"gosec","rules":[]}},"results":[]}]}',
        encoding="utf-8",
    )
    res_legacy = subprocess.run(  # noqa: S603 - fixed argv running canary_audit.py under test
        [
            sys.executable,
            str(CANARY_AUDIT_PY),
            "go-verify-scan",
            "--project-dir",
            str(gen_mod),
            "--gosec-out",
            str(gosec_out),
            "--gosec-sarif",
            str(gosec_sarif),
        ],
        capture_output=True,
        text=True,
        check=False,
    )
    check(
        "go-verify-scan (legacy --gosec-out) exits 0 for generated-only module",
        res_legacy.returncode == 0,
        res_legacy.stderr,
    )
    res_single = subprocess.run(  # noqa: S603 - fixed argv running canary_audit.py under test
        [
            sys.executable,
            str(CANARY_AUDIT_PY),
            "go-verify-scan",
            "--project-dir",
            str(gen_mod),
            "--gosec-sarif",
            str(gosec_sarif),
        ],
        capture_output=True,
        text=True,
        check=False,
    )
    check(
        "go-verify-scan (single-path) exits 0 for generated-only module",
        res_single.returncode == 0,
        res_single.stderr,
    )

sys.stdout.write(
    "A8. Real golangci-lint passes isolated go-canary (without --gosec-bin) on pki-agent\n"
)
go_ver = detect_go_toolchain_version()
cache_root = pathlib.Path(os.environ.get("XDG_CACHE_HOME", pathlib.Path.home() / ".cache"))
real_cache_base = cache_root / "alt-lint" / go_ver
real_golangci = real_cache_base / f"golangci-lint-{GOLANGCI_LINT_VERSION}" / "golangci-lint"
if real_golangci.is_file():
    pki_dir = ROOT / "pki-agent"
    before_pki = snapshot_tree(pki_dir)
    res = subprocess.run(  # noqa: S603 - fixed argv running real canary audit
        [
            sys.executable,
            str(CANARY_AUDIT_PY),
            "go-canary",
            "--project-dir",
            str(pki_dir),
            "--config",
            str(GOLANGCI_YML),
            "--golangci-lint-bin",
            str(real_golangci),
        ],
        capture_output=True,
        text=True,
        check=False,
    )
    after_pki = snapshot_tree(pki_dir)
    check(
        "real go-canary exits 0", res.returncode == 0, f"rc={res.returncode}, stderr={res.stderr!r}"
    )
    check("pki-agent tree unchanged after canary", before_pki == after_pki)
else:
    check("real golangci-lint binary cached (skipped if absent)", True)

# ---------------------------------------------------------------------------
# Group B: Python tests
# ---------------------------------------------------------------------------
sys.stdout.write("B1. Python normal run (with bandit) passes canary + scan check cleanly\n")
with tempfile.TemporaryDirectory() as td:
    repo, svc, env = build_python_test_repo(pathlib.Path(td), PythonRepoSpec(with_bandit=True))
    res = run_python_sh(repo, "svc-py", env)
    files_after = snapshot_tree(svc)
    check("python.sh exits 0", res.returncode == 0, f"rc={res.returncode}, stderr={res.stderr!r}")
    check("summary includes canary: pass", "canary: pass" in res.stdout, res.stdout)
    check(
        "no canary files left in python project dir",
        not any("canary" in p.lower() for p in files_after),
        f"files={sorted(files_after)}",
    )

sys.stdout.write("B2. Python broken ruff (silent exit 0) fails canary audit\n")
with tempfile.TemporaryDirectory() as td:
    repo, svc, env = build_python_test_repo(
        pathlib.Path(td),
        PythonRepoSpec(ruff_mode="silent_pass"),
    )
    res = run_python_sh(repo, "svc-py", env)
    files_after = snapshot_tree(svc)
    check("python.sh exits 1", res.returncode == 1, f"rc={res.returncode}, stdout={res.stdout!r}")
    check("stderr reports CANARY AUDIT FAILED", "CANARY AUDIT FAILED" in res.stderr, res.stderr)
    check(
        "no canary files left",
        not any("canary" in p.lower() for p in files_after),
        f"files={sorted(files_after)}",
    )

sys.stdout.write("B3. Python broken bandit (silent exit 0) fails canary audit\n")
with tempfile.TemporaryDirectory() as td:
    repo, svc, env = build_python_test_repo(
        pathlib.Path(td),
        PythonRepoSpec(with_bandit=True, bandit_mode="silent_pass"),
    )
    res = run_python_sh(repo, "svc-py", env)
    check("python.sh exits 1", res.returncode == 1, f"rc={res.returncode}, stdout={res.stdout!r}")
    check("stderr reports CANARY AUDIT FAILED", "CANARY AUDIT FAILED" in res.stderr, res.stderr)

sys.stdout.write("B4. Python target with 0 non-generated .py files fails with exit 1\n")
with tempfile.TemporaryDirectory() as td:
    repo, svc, env = build_python_test_repo(
        pathlib.Path(td),
        PythonRepoSpec(create_app_py=False),
    )
    for rel_path in (
        ".venv/site.py",
        "build/out.py",
        "dist/pkg.py",
        "gen/proto.py",
        "foo_pb2.py",
    ):
        target_file = svc / rel_path
        target_file.parent.mkdir(parents=True, exist_ok=True)
        target_file.write_text("x = 1\n", encoding="utf-8")
    res = run_python_sh(repo, "svc-py", env)
    check("python.sh exits 1", res.returncode == 1, f"rc={res.returncode}, stdout={res.stdout!r}")
    check(
        "stderr reports ZERO-FILE SCAN DETECTED",
        "ZERO-FILE SCAN DETECTED" in res.stderr,
        res.stderr,
    )

sys.stdout.write("B5. Python bandit scanning 0 lines on real module fails with exit 1\n")
with tempfile.TemporaryDirectory() as td:
    repo, svc, env = build_python_test_repo(
        pathlib.Path(td),
        PythonRepoSpec(with_bandit=True, bandit_mode="zero_scan"),
    )
    res = run_python_sh(repo, "svc-py", env)
    check("python.sh exits 1", res.returncode == 1, f"rc={res.returncode}, stdout={res.stdout!r}")
    check(
        "stderr reports ZERO-FILE SCAN DETECTED",
        "ZERO-FILE SCAN DETECTED" in res.stderr,
        res.stderr,
    )

sys.stdout.write("B6. Python project without bandit declared passes ruff-only canary\n")
with tempfile.TemporaryDirectory() as td:
    repo, svc, env = build_python_test_repo(
        pathlib.Path(td),
        PythonRepoSpec(with_bandit=False, bandit_mode="silent_pass"),
    )
    res = run_python_sh(repo, "svc-py", env)
    check("python.sh exits 0", res.returncode == 0, f"rc={res.returncode}, stderr={res.stderr!r}")
    check("bandit reported not declared", "bandit: not declared" in res.stdout, res.stdout)

sys.stdout.write("B7. Python project named tag-generator/app is not excluded by gen filter\n")
with tempfile.TemporaryDirectory() as td:
    repo, svc, env = build_python_test_repo(
        pathlib.Path(td),
        PythonRepoSpec(
            with_bandit=False,
            create_app_py=False,
            project_rel="tag-generator/app",
        ),
    )
    pkg_file = svc / "tag_generator" / "main.py"
    pkg_file.parent.mkdir(parents=True)
    pkg_file.write_text("VALUE = 1\n", encoding="utf-8")
    res = run_python_sh(repo, "tag-generator/app", env)
    check("python.sh exits 0", res.returncode == 0, f"rc={res.returncode}, stderr={res.stderr!r}")

sys.stdout.write("B8. Real uv/ruff/bandit pass canary audit on scripts and tag-generator/app\n")
if UV_BIN is not None:
    for proj_rel, check_bandit_flag in (("scripts", "false"), ("tag-generator/app", "true")):
        proj_dir = ROOT / proj_rel
        if not (proj_dir / "pyproject.toml").is_file() or not (proj_dir / "uv.lock").is_file():
            sys.stdout.write(f"  SKIP  {proj_rel} not checked out in sparse workspace\n")
            continue
        probe = subprocess.run(  # noqa: S603 - fixed argv probing frozen dev group availability
            [UV_BIN, "run", "--frozen", "--only-group", "dev", "ruff", "--version"],
            cwd=str(proj_dir),
            capture_output=True,
            text=True,
            check=False,
        )
        if probe.returncode != 0:
            sys.stdout.write(f"  SKIP  {proj_rel} uv dev environment not cached offline\n")
            continue
        before_tree = snapshot_tree(proj_dir)
        res = subprocess.run(  # noqa: S603 - fixed argv running real python canary
            [
                sys.executable,
                str(CANARY_AUDIT_PY),
                "python-canary",
                "--project-dir",
                str(proj_dir),
                "--check-bandit",
                check_bandit_flag,
            ],
            capture_output=True,
            text=True,
            check=False,
        )
        after_tree = snapshot_tree(proj_dir)
        check(
            f"real python-canary exits 0 on {proj_rel}",
            res.returncode == 0,
            f"rc={res.returncode}, stderr={res.stderr!r}",
        )
        check(f"{proj_rel} tree unchanged after canary", before_tree == after_tree)
else:
    sys.stdout.write("  SKIP  uv binary not installed on PATH (covered by stub B1-B7)\n")

sys.stdout.write(f"\n{RESULTS.passed} passed, {RESULTS.failed} failed\n")
sys.exit(1 if RESULTS.failed else 0)
