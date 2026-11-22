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
RUST_SH = LINT_DIR / "rust.sh"
FRONTEND_SH = LINT_DIR / "frontend.sh"
DENO_SH = LINT_DIR / "deno.sh"
PROTO_SH = LINT_DIR / "proto.sh"
RUN_SH = LINT_DIR / "run.sh"
CANARY_AUDIT_PY = LINT_DIR / "canary_audit.py"
CANARY_LEDGER_TOML = LINT_DIR / "canary_ledger.toml"
GOLANGCI_YML = ROOT / ".golangci.yml"

GOLANGCI_LINT_VERSION = "v2.14.0"
GOVULNCHECK_VERSION = "v1.6.0"

BASH_BIN = shutil.which("bash")
GO_BIN = shutil.which("go")
UV_BIN = shutil.which("uv")
CARGO_BIN = shutil.which("cargo")
BUF_BIN = shutil.which("buf")
BUN_BIN = shutil.which("bun")

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
    govulncheck_mode: str = "healthy",
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

mode = {govulncheck_mode!r}
args = sys.argv[1:]
fmt = "text"
for i, tok in enumerate(args):
    if tok.startswith("-format="):
        fmt = tok.split("=", 1)[1]
    elif tok == "-format" and i + 1 < len(args):
        fmt = args[i + 1]

if fmt == "json":
    if mode == "missing_db":
        sys.stdout.write(json.dumps({{"config": {{"scanner_name": "govulncheck"}}}}) + "\\n")
        sys.stdout.write(json.dumps({{"SBOM": {{"modules": [{{"path": "svc-go"}}], "roots": ["svc-go"]}}}}) + "\\n")
    elif mode == "empty_sbom":
        sys.stdout.write(json.dumps({{
            "config": {{
                "protocol_version": "v1.0.0",
                "scanner_name": "govulncheck",
                "scanner_version": "v1.6.0",
                "db": "https://vuln.go.dev",
                "go_version": "go1.27.4",
            }}
        }}) + "\\n")
        sys.stdout.write(json.dumps({{"SBOM": {{"modules": [], "roots": []}}}}) + "\\n")
    else:
        sys.stdout.write(json.dumps({{
            "config": {{
                "protocol_version": "v1.0.0",
                "scanner_name": "govulncheck",
                "scanner_version": "v1.6.0",
                "db": "https://vuln.go.dev",
                "go_version": "go1.27.4",
            }}
        }}) + "\\n")
        sys.stdout.write(json.dumps({{
            "SBOM": {{
                "go_version": "go1.27.4",
                "modules": [{{"path": "svc-go"}}, {{"path": "stdlib", "version": "v1.27.4"}}],
                "roots": ["svc-go"],
            }}
        }}) + "\\n")
    sys.exit(0)

driver_props = {{
    "protocol_version": "v1.0.0",
    "scanner_name": "govulncheck",
    "scanner_version": "v1.6.0",
    "db": "" if mode == "missing_db" else "https://vuln.go.dev",
    "go_version": "go1.27.4",
}}
sys.stdout.write(json.dumps({{
    "version": "2.1.0",
    "runs": [{{
        "tool": {{
            "driver": {{
                "name": "govulncheck",
                "properties": driver_props,
                "rules": [],
            }}
        }},
        "results": [],
    }}],
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


def _run_py_tool(
    args: list[str],
    *,
    env: dict[str, str] | None = None,
) -> subprocess.CompletedProcess[str]:
    return subprocess.run(  # noqa: S603 - fixed argv running repo lint script in test
        [sys.executable, *args],
        env=env,
        capture_output=True,
        text=True,
        check=False,
    )


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
    with_pyrefly: bool = False
    ruff_mode: str = "healthy"
    bandit_mode: str = "healthy"
    pyrefly_mode: str = "healthy"
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
    if cfg.with_pyrefly:
        dev_deps.append('  "pyrefly>=0.42.0",')
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
    if cfg.with_pyrefly:
        pyproject_lines.extend(
            [
                "",
                "[tool.pyrefly]",
                'project-includes = ["."]',
            ]
        )
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

if tool == "pyrefly":
    mode = os.environ.get("MOCK_PYREFLY_MODE", "healthy")
    if mode == "silent_pass":
        sys.stdout.write("INFO 0 errors\\n")
        sys.exit(0)
    targets: list[pathlib.Path] = []
    i = 1
    while i < len(rest):
        tok = rest[i]
        if tok in ("--config", "-c"):
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
            if py_file.is_file() and 'not_an_int' in py_file.read_text(encoding="utf-8"):
                sys.stderr.write(
                    "ERROR `Literal['not_an_int']` is not assignable to `int` [bad-assignment]\\n"
                )
                sys.exit(1)
    sys.stdout.write("INFO 0 errors\\n")
    sys.exit(0)

sys.stderr.write(f"stub uv: unexpected tool {{tool}}\\n")
sys.exit(2)
"""
    make_executable(bin_dir / "uv", uv_script)

    env = os.environ.copy()
    env["PATH"] = f"{bin_dir}:{env.get('PATH', '')}"
    env["MOCK_RUFF_MODE"] = cfg.ruff_mode
    env["MOCK_BANDIT_MODE"] = cfg.bandit_mode
    env["MOCK_PYREFLY_MODE"] = cfg.pyrefly_mode
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

# ---------------------------------------------------------------------------
# Group A9 & B9: govulncheck verification and pyrefly canary
# ---------------------------------------------------------------------------
sys.stdout.write("A9. Go govulncheck coverage and DB metadata verification\n")
for vuln_mode, expected_substr in (
    ("missing_db", "govulncheck"),
    ("empty_sbom", "ZERO-FILE SCAN DETECTED"),
):
    with tempfile.TemporaryDirectory() as td:
        repo, svc, env = build_go_test_repo(pathlib.Path(td), govulncheck_mode=vuln_mode)
        res = run_go_sh(repo, "svc-go", env)
        check(
            f"govulncheck {vuln_mode}: go.sh exits 1",
            res.returncode == 1,
            f"rc={res.returncode}, stdout={res.stdout!r}, stderr={res.stderr!r}",
        )
        check(
            f"govulncheck {vuln_mode}: stderr reports {expected_substr}",
            expected_substr in res.stderr,
            res.stderr,
        )

sys.stdout.write("B9. Python pyrefly canary (healthy, broken silent-pass, not declared)\n")
with tempfile.TemporaryDirectory() as td:
    repo, svc, env = build_python_test_repo(
        pathlib.Path(td),
        PythonRepoSpec(with_bandit=False, with_pyrefly=True, pyrefly_mode="healthy"),
    )
    res = run_python_sh(repo, "svc-py", env)
    check(
        "pyrefly healthy: python.sh exits 0",
        res.returncode == 0,
        f"rc={res.returncode}, stderr={res.stderr!r}",
    )
    check(
        "pyrefly healthy: summary includes pyrefly: pass", "pyrefly: pass" in res.stdout, res.stdout
    )

with tempfile.TemporaryDirectory() as td:
    repo, svc, env = build_python_test_repo(
        pathlib.Path(td),
        PythonRepoSpec(with_bandit=False, with_pyrefly=True, pyrefly_mode="silent_pass"),
    )
    res = run_python_sh(repo, "svc-py", env)
    check(
        "pyrefly silent_pass: python.sh exits 1",
        res.returncode == 1,
        f"rc={res.returncode}, stdout={res.stdout!r}",
    )
    check(
        "pyrefly silent_pass: stderr reports CANARY AUDIT FAILED",
        "CANARY AUDIT FAILED" in res.stderr,
        res.stderr,
    )

if UV_BIN is not None:
    rerank_dir = ROOT / "rerank-server"
    if (rerank_dir / "pyproject.toml").is_file() and (rerank_dir / "uv.lock").is_file():
        probe = subprocess.run(  # noqa: S603 - fixed argv probing cached pyrefly dev group
            [UV_BIN, "run", "--frozen", "--only-group", "dev", "pyrefly", "--version"],
            cwd=str(rerank_dir),
            capture_output=True,
            text=True,
            check=False,
        )
        if probe.returncode == 0:
            res = subprocess.run(  # noqa: S603 - fixed argv running real pyrefly canary
                [
                    sys.executable,
                    str(CANARY_AUDIT_PY),
                    "python-canary",
                    "--project-dir",
                    str(rerank_dir),
                    "--check-bandit",
                    "false",
                    "--check-pyrefly",
                    "true",
                ],
                capture_output=True,
                text=True,
                check=False,
            )
            check(
                "real pyrefly canary exits 0 on rerank-server",
                res.returncode == 0,
                f"rc={res.returncode}, stderr={res.stderr!r}",
            )

# ---------------------------------------------------------------------------
# Group C: Rust canary tests (rust-canary + rust.sh)
# ---------------------------------------------------------------------------
sys.stdout.write("C1. Rust rust-canary wired in rust.sh (healthy vs silent_pass)\n")
for rust_mode, expected_rc in (("healthy", 0), ("silent_pass", 1)):
    with tempfile.TemporaryDirectory() as td:
        td_path = pathlib.Path(td)
        repo = td_path / "repo"
        lint_dir = repo / "scripts" / "lint"
        lint_dir.mkdir(parents=True)
        shutil.copy2(RUST_SH, lint_dir / "rust.sh")
        shutil.copy2(CANARY_AUDIT_PY, lint_dir / "canary_audit.py")
        shutil.copy2(RUN_SH, lint_dir / "run.sh")
        crate = repo / "svc-rust"
        (crate / "src").mkdir(parents=True)
        (crate / "Cargo.toml").write_text(
            '[package]\nname = "svc-rust"\nversion = "0.1.0"\nedition = "2021"\n',
            encoding="utf-8",
        )
        (crate / "rust-toolchain.toml").write_text(
            '[toolchain]\nchannel = "1.99.1"\ncomponents = ["rustfmt", "clippy"]\n',
            encoding="utf-8",
        )
        (crate / "src" / "lib.rs").write_text("pub fn ok() -> i32 { 1 }\n", encoding="utf-8")

        bin_dir = td_path / "stub-bin"
        cargo_stub = f"""#!{sys.executable}
import pathlib, sys

mode = {rust_mode!r}
args = sys.argv[1:]
if args == ["--version"]:
    sys.stdout.write("cargo 1.99.1 (stub)\\n")
    sys.exit(0)
if args and args[0] == "fmt":
    sys.exit(0)
if args and args[0] == "clippy":
    lib_rs = pathlib.Path.cwd() / "src" / "lib.rs"
    is_canary = lib_rs.is_file() and "x == x" in lib_rs.read_text(encoding="utf-8")
    if is_canary and mode == "healthy":
        sys.stderr.write("error: equal expressions as operands to `==` (clippy::eq_op)\\n")
        sys.exit(101)
    sys.exit(0)
sys.exit(0)
"""
        make_executable(bin_dir / "cargo", cargo_stub)
        env = os.environ.copy()
        env["PATH"] = f"{bin_dir}:{env.get('PATH', '')}"
        env["XDG_CACHE_HOME"] = str(td_path / "cache")
        res = subprocess.run(  # noqa: S603 - fixed argv running rust.sh under test
            [BASH_BIN, str(lint_dir / "rust.sh"), "svc-rust"],
            cwd=str(repo),
            env=env,
            capture_output=True,
            text=True,
            check=False,
        )
        check(
            f"rust.sh ({rust_mode}) exits {expected_rc}",
            res.returncode == expected_rc,
            f"rc={res.returncode}, stdout={res.stdout!r}, stderr={res.stderr!r}",
        )
        if expected_rc == 1:
            check(
                "rust.sh (silent_pass) reports CANARY AUDIT FAILED",
                "CANARY AUDIT FAILED" in res.stderr,
                res.stderr,
            )

if CARGO_BIN is not None and (ROOT / "plecto" / "Cargo.toml").is_file():
    before_plecto = snapshot_tree(ROOT / "plecto")
    res = subprocess.run(  # noqa: S603 - fixed argv running real rust-canary
        [
            sys.executable,
            str(CANARY_AUDIT_PY),
            "rust-canary",
            "--project-dir",
            str(ROOT / "plecto"),
        ],
        capture_output=True,
        text=True,
        check=False,
    )
    after_plecto = snapshot_tree(ROOT / "plecto")
    check(
        "real rust-canary exits 0 on plecto",
        res.returncode == 0,
        f"rc={res.returncode}, stderr={res.stderr!r}",
    )
    check("plecto tree unchanged after rust-canary", before_plecto == after_plecto)

# ---------------------------------------------------------------------------
# Group D: Frontend canary tests (frontend-canary + frontend.sh)
# ---------------------------------------------------------------------------
sys.stdout.write("D1. Frontend frontend-canary wired in frontend.sh (healthy vs silent_pass)\n")
for biome_mode, expected_rc in (("healthy", 0), ("silent_pass", 1)):
    with tempfile.TemporaryDirectory() as td:
        td_path = pathlib.Path(td)
        repo = td_path / "repo"
        lint_dir = repo / "scripts" / "lint"
        lint_dir.mkdir(parents=True)
        shutil.copy2(FRONTEND_SH, lint_dir / "frontend.sh")
        shutil.copy2(CANARY_AUDIT_PY, lint_dir / "canary_audit.py")
        shutil.copy2(RUN_SH, lint_dir / "run.sh")
        fe = repo / "svc-fe"
        (fe / "node_modules" / "@biomejs" / "biome").mkdir(parents=True)
        (fe / "node_modules" / "svelte-check").mkdir(parents=True)
        (fe / "node_modules" / ".bin").mkdir(parents=True)
        (fe / "package.json").write_text('{"name":"svc-fe"}\n', encoding="utf-8")
        (fe / "Dockerfile").write_text("FROM oven/bun:1.3.12-alpine AS base\n", encoding="utf-8")
        (fe / "bun.lock").write_text(
            '"@biomejs/biome": ["@biomejs/biome@2.5.2"]\n"svelte-check": ["svelte-check@4.7.6"]\n',
            encoding="utf-8",
        )
        (fe / "node_modules" / "@biomejs" / "biome" / "package.json").write_text(
            '{"version":"2.5.2"}\n', encoding="utf-8"
        )
        (fe / "node_modules" / "svelte-check" / "package.json").write_text(
            '{"version":"4.7.6"}\n', encoding="utf-8"
        )
        biome_stub = f"""#!{sys.executable}
import pathlib, sys

mode = {biome_mode!r}
args = sys.argv[1:]
if mode == "silent_pass":
    sys.stdout.write("Checked 1 file. No fixes applied.\\n")
    sys.exit(0)
for tok in args:
    p = pathlib.Path(tok)
    if p.is_file() and "debugger;" in p.read_text(encoding="utf-8"):
        sys.stderr.write("lint/suspicious/noDebugger: This is an unexpected use of the debugger statement.\\n")
        sys.exit(1)
sys.exit(0)
"""
        make_executable(fe / "node_modules" / ".bin" / "biome", biome_stub)
        bin_dir = td_path / "stub-bin"
        bun_stub = f"""#!{sys.executable}
import sys
if sys.argv[1:] == ["--version"]:
    sys.stdout.write("1.3.12\\n")
sys.exit(0)
"""
        make_executable(bin_dir / "bun", bun_stub)
        env = os.environ.copy()
        env["PATH"] = f"{bin_dir}:{env.get('PATH', '')}"
        res = subprocess.run(  # noqa: S603 - fixed argv running frontend.sh under test
            [BASH_BIN, str(lint_dir / "frontend.sh"), "svc-fe"],
            cwd=str(repo),
            env=env,
            capture_output=True,
            text=True,
            check=False,
        )
        check(
            f"frontend.sh ({biome_mode}) exits {expected_rc}",
            res.returncode == expected_rc,
            f"rc={res.returncode}, stdout={res.stdout!r}, stderr={res.stderr!r}",
        )
        if expected_rc == 1:
            check(
                "frontend.sh (silent_pass) reports CANARY AUDIT FAILED",
                "CANARY AUDIT FAILED" in res.stderr,
                res.stderr,
            )

# ---------------------------------------------------------------------------
# Group E: Deno canary tests (deno-canary + deno.sh)
# ---------------------------------------------------------------------------
sys.stdout.write("E1. Deno deno-canary wired in deno.sh (healthy vs silent_pass)\n")
for deno_mode, expected_rc in (("healthy", 0), ("silent_pass", 1)):
    with tempfile.TemporaryDirectory() as td:
        td_path = pathlib.Path(td)
        repo = td_path / "repo"
        lint_dir = repo / "scripts" / "lint"
        lint_dir.mkdir(parents=True)
        shutil.copy2(DENO_SH, lint_dir / "deno.sh")
        shutil.copy2(CANARY_AUDIT_PY, lint_dir / "canary_audit.py")
        shutil.copy2(RUN_SH, lint_dir / "run.sh")
        svc = repo / "svc-deno"
        svc.mkdir(parents=True)
        (svc / ".dvmrc").write_text("2.6.3\n", encoding="utf-8")
        (svc / "deno.json").write_text("{}\n", encoding="utf-8")
        (svc / "main.ts").write_text("export const ok = 1;\n", encoding="utf-8")
        bin_dir = td_path / "stub-bin"
        deno_stub = f"""#!{sys.executable}
import pathlib, sys

mode = {deno_mode!r}
args = sys.argv[1:]
if args == ["--version"]:
    sys.stdout.write("deno 2.6.3 (stub)\\n")
    sys.exit(0)
if args and args[0] == "lint":
    if mode == "silent_pass":
        sys.stdout.write("Checked 1 file\\n")
        sys.exit(0)
    for tok in args[1:]:
        p = pathlib.Path(tok)
        if p.is_file() and "debugger;" in p.read_text(encoding="utf-8"):
            sys.stderr.write("error[no-debugger]: `debugger` statement is not allowed\\n")
            sys.exit(1)
sys.exit(0)
"""
        make_executable(bin_dir / "deno", deno_stub)
        env = os.environ.copy()
        env["PATH"] = f"{bin_dir}:{env.get('PATH', '')}"
        res = subprocess.run(  # noqa: S603 - fixed argv running deno.sh under test
            [BASH_BIN, str(lint_dir / "deno.sh"), "svc-deno"],
            cwd=str(repo),
            env=env,
            capture_output=True,
            text=True,
            check=False,
        )
        check(
            f"deno.sh ({deno_mode}) exits {expected_rc}",
            res.returncode == expected_rc,
            f"rc={res.returncode}, stdout={res.stdout!r}, stderr={res.stderr!r}",
        )
        if expected_rc == 1:
            check(
                "deno.sh (silent_pass) reports CANARY AUDIT FAILED",
                "CANARY AUDIT FAILED" in res.stderr,
                res.stderr,
            )

cached_deno = cache_root / "alt-lint" / "deno" / "2.6.3" / "deno"
if cached_deno.is_file():
    res = subprocess.run(  # noqa: S603 - fixed argv running real deno-canary
        [
            sys.executable,
            str(CANARY_AUDIT_PY),
            "deno-canary",
            "--project-dir",
            str(ROOT / "auth-token-manager"),
            "--deno-bin",
            str(cached_deno),
        ],
        capture_output=True,
        text=True,
        check=False,
    )
    check(
        "real deno-canary exits 0 with cached deno 2.6.3",
        res.returncode == 0,
        f"rc={res.returncode}, stderr={res.stderr!r}",
    )

# ---------------------------------------------------------------------------
# Group F: Proto canary tests (proto-canary + proto.sh)
# ---------------------------------------------------------------------------
sys.stdout.write("F1. Proto proto-canary wired in proto.sh (healthy vs silent_pass)\n")
for buf_mode, expected_rc in (("healthy", 0), ("silent_pass", 1)):
    with tempfile.TemporaryDirectory() as td:
        td_path = pathlib.Path(td)
        repo = td_path / "repo"
        lint_dir = repo / "scripts" / "lint"
        lint_dir.mkdir(parents=True)
        shutil.copy2(PROTO_SH, lint_dir / "proto.sh")
        shutil.copy2(CANARY_AUDIT_PY, lint_dir / "canary_audit.py")
        shutil.copy2(RUN_SH, lint_dir / "run.sh")
        proto_dir = repo / "proto"
        (proto_dir / "tools" / "protovis").mkdir(parents=True)
        (proto_dir / "buf.yaml").write_text("version: v2\n", encoding="utf-8")
        # Stub allowlist files and minimal protovis that writes identical files
        (repo / "alt-butterfly-facade" / "internal" / "server").mkdir(parents=True)
        (repo / "alt-frontend-sv" / "src" / "lib" / "gen").mkdir(parents=True)
        (repo / "alt-butterfly-facade" / "internal" / "server" / "allowlist_gen.go").write_text(
            "package server\n", encoding="utf-8"
        )
        (repo / "alt-frontend-sv" / "src" / "lib" / "gen" / "allowlist.ts").write_text(
            "export const ALLOW = [];\n", encoding="utf-8"
        )
        bin_dir = td_path / "stub-bin"
        buf_stub = f"""#!{sys.executable}
import pathlib, sys

mode = {buf_mode!r}
args = sys.argv[1:]
if args == ["--version"]:
    sys.stdout.write("1.48.0\\n")
    sys.exit(0)
if args and args[0] == "lint":
    canary_proto = pathlib.Path.cwd() / "canary.proto"
    if canary_proto.is_file() and mode == "healthy":
        sys.stdout.write("canary.proto:3:6:Enum name \\"bad_enum\\" should be PascalCase.\\n")
        sys.exit(100)
    sys.exit(0)
if args and args[0] == "build":
    for i, tok in enumerate(args):
        if tok == "-o" and i + 1 < len(args):
            pathlib.Path(args[i + 1]).write_bytes(b"stub")
    sys.exit(0)
sys.exit(0)
"""
        go_stub = f"""#!{sys.executable}
import pathlib, sys

args = sys.argv[1:]
root_out = None
for i, tok in enumerate(args):
    if tok == "-root" and i + 1 < len(args):
        root_out = pathlib.Path(args[i + 1])
if root_out is not None:
    p1 = root_out / "alt-butterfly-facade" / "internal" / "server" / "allowlist_gen.go"
    p2 = root_out / "alt-frontend-sv" / "src" / "lib" / "gen" / "allowlist.ts"
    p1.parent.mkdir(parents=True, exist_ok=True)
    p2.parent.mkdir(parents=True, exist_ok=True)
    p1.write_text("package server\\n", encoding="utf-8")
    p2.write_text("export const ALLOW = [];\\n", encoding="utf-8")
sys.exit(0)
"""
        make_executable(bin_dir / "buf", buf_stub)
        make_executable(bin_dir / "go", go_stub)
        env = os.environ.copy()
        env["PATH"] = f"{bin_dir}:{env.get('PATH', '')}"
        res = subprocess.run(  # noqa: S603 - fixed argv running proto.sh under test
            [BASH_BIN, str(lint_dir / "proto.sh"), "proto"],
            cwd=str(repo),
            env=env,
            capture_output=True,
            text=True,
            check=False,
        )
        check(
            f"proto.sh ({buf_mode}) exits {expected_rc}",
            res.returncode == expected_rc,
            f"rc={res.returncode}, stdout={res.stdout!r}, stderr={res.stderr!r}",
        )
        if expected_rc == 1:
            check(
                "proto.sh (silent_pass) reports CANARY AUDIT FAILED",
                "CANARY AUDIT FAILED" in res.stderr,
                res.stderr,
            )

if BUF_BIN is not None:
    res = subprocess.run(  # noqa: S603 - fixed argv running real proto-canary
        [sys.executable, str(CANARY_AUDIT_PY), "proto-canary", "--buf-bin", BUF_BIN],
        capture_output=True,
        text=True,
        check=False,
    )
    check(
        "real proto-canary exits 0 with buf",
        res.returncode == 0,
        f"rc={res.returncode}, stderr={res.stderr!r}",
    )

# ---------------------------------------------------------------------------
# Group G: Actionlint canary tests (actionlint-canary)
# ---------------------------------------------------------------------------
sys.stdout.write("G1. Actionlint actionlint-canary (stub healthy/silent_pass + real)\n")
for al_mode, expected_rc in (("healthy", 0), ("silent_pass", 1)):
    with tempfile.TemporaryDirectory() as td:
        td_path = pathlib.Path(td)
        al_stub = td_path / "actionlint"
        make_executable(
            al_stub,
            f"""#!{sys.executable}
import pathlib, sys

mode = {al_mode!r}
if mode == "silent_pass":
    sys.exit(0)
for tok in sys.argv[1:]:
    p = pathlib.Path(tok)
    if p.is_file() and "invalid_context" in p.read_text(encoding="utf-8"):
        sys.stdout.write(f"{{p}}:7:23: undefined variable \\"invalid_context\\" [expression]\\n")
        sys.exit(1)
sys.exit(0)
""",
        )
        res = subprocess.run(  # noqa: S603 - fixed argv running actionlint-canary under test
            [
                sys.executable,
                str(CANARY_AUDIT_PY),
                "actionlint-canary",
                "--actionlint-bin",
                str(al_stub),
            ],
            capture_output=True,
            text=True,
            check=False,
        )
        check(
            f"actionlint-canary ({al_mode}) exits {expected_rc}",
            res.returncode == expected_rc,
            f"rc={res.returncode}, stdout={res.stdout!r}, stderr={res.stderr!r}",
        )
        if expected_rc == 1:
            check(
                "actionlint-canary (silent_pass) reports CANARY AUDIT FAILED",
                "CANARY AUDIT FAILED" in res.stderr,
                res.stderr,
            )

res_real_al = subprocess.run(  # noqa: S603 - fixed argv running real actionlint-canary
    [sys.executable, str(CANARY_AUDIT_PY), "actionlint-canary"],
    cwd=str(ROOT),
    capture_output=True,
    text=True,
    check=False,
)
check(
    "real actionlint-canary exits 0",
    res_real_al.returncode == 0,
    f"rc={res_real_al.returncode}, stderr={res_real_al.stderr!r}",
)

# ---------------------------------------------------------------------------
# Group H: 45-target canary coverage ledger verification
# ---------------------------------------------------------------------------
sys.stdout.write("H1. 45-target canary coverage ledger (real + negative drift tests)\n")
res_ledger = _run_py_tool([str(CANARY_AUDIT_PY), "verify-ledger", "--repo-root", str(ROOT)])
check(
    "real repo verify-ledger exits 0",
    res_ledger.returncode == 0,
    f"rc={res_ledger.returncode}, stdout={res_ledger.stdout!r}, stderr={res_ledger.stderr!r}",
)
check(
    "real repo verify-ledger accounts for all 45 targets",
    "45" in res_ledger.stdout,
    res_ledger.stdout,
)

with tempfile.TemporaryDirectory() as td:
    td_path = pathlib.Path(td)
    mini_repo = td_path / "mini-repo"
    (mini_repo / "scripts" / "lint").mkdir(parents=True)
    shutil.copy2(RUN_SH, mini_repo / "scripts" / "lint" / "run.sh")
    (mini_repo / "svc-a").mkdir()
    (mini_repo / "svc-a" / "go.mod").write_text("module svc-a\n\ngo 1.27.4\n", encoding="utf-8")
    (mini_repo / "svc-a" / "main.go").write_text("package main\nfunc main() {}\n", encoding="utf-8")
    (mini_repo / "svc-py").mkdir()
    (mini_repo / "svc-py" / "pyproject.toml").write_text(
        '[project]\nname = "svc-py"\n[dependency-groups]\ndev = ["ruff>=0.15.0", "bandit>=1.8.0", "pyrefly>=0.42.0"]\n',
        encoding="utf-8",
    )

    # 1) Missing target in ledger fails
    missing_ledger = td_path / "missing.toml"
    missing_ledger.write_text(
        'version = 1\n[targets."svc-a"]\nkind = "go"\n'
        '[targets."svc-a".scanners.golangci-lint]\nstatus = "canary"\ncanary = "go-canary"\n'
        '[targets."svc-a".scanners.gosec]\nstatus = "canary"\ncanary = "go-canary"\n'
        '[targets."svc-a".scanners.govulncheck]\nstatus = "canary"\ncanary = "go-verify-govulncheck"\n',
        encoding="utf-8",
    )
    res_missing = subprocess.run(  # noqa: S603 - fixed argv running verify-ledger negative test
        [
            sys.executable,
            str(CANARY_AUDIT_PY),
            "verify-ledger",
            "--repo-root",
            str(mini_repo),
            "--ledger",
            str(missing_ledger),
        ],
        capture_output=True,
        text=True,
        check=False,
    )
    check(
        "verify-ledger fails when a target from run.sh --list is missing",
        res_missing.returncode == 1 and "svc-py" in res_missing.stderr,
        f"rc={res_missing.returncode}, stderr={res_missing.stderr!r}",
    )

    # 2) Empty reason on exempt/not-applicable fails
    empty_reason_ledger = td_path / "empty-reason.toml"
    empty_reason_ledger.write_text(
        "version = 1\n"
        '[targets."svc-a"]\nkind = "go"\n'
        '[targets."svc-a".scanners.golangci-lint]\nstatus = "canary"\ncanary = "go-canary"\n'
        '[targets."svc-a".scanners.gosec]\nstatus = "canary"\ncanary = "go-canary"\n'
        '[targets."svc-a".scanners.govulncheck]\nstatus = "canary"\ncanary = "go-verify-govulncheck"\n'
        '[targets."svc-a".scanners.gofmt]\nstatus = "exempt"\nreason = ""\n'
        '[targets."svc-py"]\nkind = "python"\n'
        '[targets."svc-py".scanners.ruff]\nstatus = "canary"\ncanary = "python-canary"\n'
        '[targets."svc-py".scanners.bandit]\nstatus = "canary"\ncanary = "python-canary"\n'
        '[targets."svc-py".scanners.pyrefly]\nstatus = "canary"\ncanary = "python-canary"\n',
        encoding="utf-8",
    )
    res_empty_reason = subprocess.run(  # noqa: S603 - fixed argv running verify-ledger negative test
        [
            sys.executable,
            str(CANARY_AUDIT_PY),
            "verify-ledger",
            "--repo-root",
            str(mini_repo),
            "--ledger",
            str(empty_reason_ledger),
        ],
        capture_output=True,
        text=True,
        check=False,
    )
    check(
        "verify-ledger fails when exempt scanner has empty reason",
        res_empty_reason.returncode == 1 and "reason" in res_empty_reason.stderr.lower(),
        f"rc={res_empty_reason.returncode}, stderr={res_empty_reason.stderr!r}",
    )

    # 3) Python target declares bandit/pyrefly but ledger says not-applicable -> fails
    drift_ledger = td_path / "drift.toml"
    drift_ledger.write_text(
        "version = 1\n"
        '[targets."svc-a"]\nkind = "go"\n'
        '[targets."svc-a".scanners.golangci-lint]\nstatus = "canary"\ncanary = "go-canary"\n'
        '[targets."svc-a".scanners.gosec]\nstatus = "canary"\ncanary = "go-canary"\n'
        '[targets."svc-a".scanners.govulncheck]\nstatus = "canary"\ncanary = "go-verify-govulncheck"\n'
        '[targets."svc-py"]\nkind = "python"\n'
        '[targets."svc-py".scanners.ruff]\nstatus = "canary"\ncanary = "python-canary"\n'
        '[targets."svc-py".scanners.bandit]\nstatus = "not-applicable"\nreason = "falsely marked not applicable"\n'
        '[targets."svc-py".scanners.pyrefly]\nstatus = "canary"\ncanary = "python-canary"\n',
        encoding="utf-8",
    )
    res_drift = subprocess.run(  # noqa: S603 - fixed argv running verify-ledger negative test
        [
            sys.executable,
            str(CANARY_AUDIT_PY),
            "verify-ledger",
            "--repo-root",
            str(mini_repo),
            "--ledger",
            str(drift_ledger),
        ],
        capture_output=True,
        text=True,
        check=False,
    )
    check(
        "verify-ledger fails when Python pyproject declares bandit but ledger marks not-applicable",
        res_drift.returncode == 1 and "bandit" in res_drift.stderr,
        f"rc={res_drift.returncode}, stderr={res_drift.stderr!r}",
    )

# ---------------------------------------------------------------------------
# Group I: Entrypoint canary (entrypoint_canary.py) Go gofmt + CLI tests
# ---------------------------------------------------------------------------
sys.stdout.write(
    "I1. entrypoint_canary.py verifies Go gofmt + G402 and supports --available-only\n"
)
ENTRYPOINT_CANARY_PY = LINT_DIR / "entrypoint_canary.py"
with tempfile.TemporaryDirectory() as td:
    td_path = pathlib.Path(td)
    ep_repo, ep_svc, ep_env = build_go_test_repo(td_path)
    shutil.copy2(RUN_SH, ep_repo / "scripts" / "lint" / "run.sh")
    shutil.copy2(ENTRYPOINT_CANARY_PY, ep_repo / "scripts" / "lint" / "entrypoint_canary.py")
    # entrypoint_canary.py targets docker/otel-relay for Go; stage a minimal copy there
    otel_dir = ep_repo / "docker" / "otel-relay"
    otel_dir.mkdir(parents=True)
    shutil.copy2(ep_svc / "go.mod", otel_dir / "go.mod")
    shutil.copy2(ep_svc / "main.go", otel_dir / "main.go")

    # Stub golangci-lint in build_go_test_repo catches InsecureSkipVerify; real gofmt catches unformatted code
    res_ep_go = _run_py_tool(
        [
            str(ENTRYPOINT_CANARY_PY),
            "--repo-root",
            str(ep_repo),
            "--languages",
            "go",
        ],
        env=ep_env,
    )
    check(
        "entrypoint_canary.py Go case passes and verifies both gofmt and G402",
        res_ep_go.returncode == 0 and "gofmt" in (res_ep_go.stdout + res_ep_go.stderr).lower(),
        f"rc={res_ep_go.returncode}, out={res_ep_go.stdout!r}, err={res_ep_go.stderr!r}",
    )

if BUF_BIN is not None:
    res_ep_proto = _run_py_tool(
        [
            str(ENTRYPOINT_CANARY_PY),
            "--repo-root",
            str(ROOT),
            "--languages",
            "proto",
            "--skip-baseline",
        ]
    )
    check(
        "real entrypoint_canary.py --languages proto --skip-baseline exits 0",
        res_ep_proto.returncode == 0,
        f"rc={res_ep_proto.returncode}, out={res_ep_proto.stdout!r}, err={res_ep_proto.stderr!r}",
    )

res_ep_help = _run_py_tool([str(ENTRYPOINT_CANARY_PY), "--help"])
check(
    "entrypoint_canary.py exposes --available-only flag",
    res_ep_help.returncode == 0 and "--available-only" in res_ep_help.stdout,
    res_ep_help.stdout + res_ep_help.stderr,
)

sys.stdout.write(f"\n{RESULTS.passed} passed, {RESULTS.failed} failed\n")
sys.exit(1 if RESULTS.failed else 0)
