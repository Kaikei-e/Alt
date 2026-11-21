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
GOSEC_VERSION = "v2.30.0"
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
    gosec_mode: str = "healthy",
) -> tuple[pathlib.Path, pathlib.Path, dict[str, str]]:
    """Build an isolated mini-repo and fake XDG_CACHE_HOME Go linter binaries."""
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
import pathlib, sys

mode = {golangci_mode!r}
if mode == "silent_pass":
    sys.stdout.write("0 issues.\\n")
    sys.exit(0)

cwd = pathlib.Path.cwd()
for go_file in sorted(cwd.rglob("*.go")):
    text = go_file.read_text(encoding="utf-8")
    if "InsecureSkipVerify: true" in text:
        sys.stdout.write(
            f"{{go_file.name}}:10:1: G402: TLS InsecureSkipVerify set to true. (gosec)\\n"
        )
        sys.exit(1)

sys.stdout.write("0 issues.\\n")
sys.exit(0)
"""

    gosec_script = f"""#!{sys.executable}
import json, pathlib, sys

mode = {gosec_mode!r}
args = sys.argv[1:]
out_path = None
for idx, arg in enumerate(args):
    if arg == "-out" and idx + 1 < len(args):
        out_path = pathlib.Path(args[idx + 1])

cwd = pathlib.Path.cwd()
go_files = [
    p for p in sorted(cwd.rglob("*.go"))
    if "gen" not in p.relative_to(cwd).parts
]
is_canary = any(
    "InsecureSkipVerify: true" in p.read_text(encoding="utf-8")
    for p in go_files
)

def write_sarif(results: list[dict[str, object]]) -> None:
    if out_path is None:
        return
    payload = {{
        "version": "2.1.0",
        "runs": [{{"tool": {{"driver": {{"name": "gosec"}}}}, "results": results}}],
    }}
    out_path.write_text(json.dumps(payload), encoding="utf-8")

if mode == "silent_pass":
    write_sarif([])
    sys.stdout.write("Summary:\\n  Files  : 1\\n  Issues : 0\\n")
    sys.exit(0)

if mode == "canary_crash_empty_sarif" and is_canary:
    write_sarif([])
    sys.stdout.write(
        "Golang errors in file: [.]:\\n  > loading files from package\\n"
        "Summary:\\n  Files  : 0\\n  Issues : 0\\n"
    )
    sys.exit(1)

if is_canary:
    write_sarif([{{
        "ruleId": "G402",
        "level": "error",
        "message": {{"text": "TLS InsecureSkipVerify set to true."}},
        "locations": [{{
            "physicalLocation": {{
                "artifactLocation": {{"uri": "canary.go"}},
                "region": {{"startLine": 10}},
            }}
        }}],
    }}])
    sys.stdout.write("Summary:\\n  Files  : 1\\n  Issues : 1\\n")
    sys.exit(1)

if mode == "zero_files":
    write_sarif([])
    sys.stdout.write("Summary:\\n  Files  : 0\\n  Lines  : 0\\n  Issues : 0\\n")
    sys.exit(0)

if mode == "zero_files_load_error":
    write_sarif([])
    sys.stdout.write(
        "Golang errors in file: [.]:\\n  > go.mod requires go >= 1.99.0\\n"
        "Summary:\\n  Files  : 0\\n  Lines  : 0\\n  Issues : 0\\n"
    )
    sys.exit(0)

write_sarif([])
sys.stdout.write(f"Summary:\\n  Files  : {{len(go_files)}}\\n  Issues : 0\\n")
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
        cache_base / f"gosec-{GOSEC_VERSION}" / "gosec",
        gosec_script,
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
sys.stdout.write("A1. Go normal run passes canary + scan check with zero contamination\n")
with tempfile.TemporaryDirectory() as td:
    repo, svc, env = build_go_test_repo(pathlib.Path(td))
    res = run_go_sh(repo, "svc-go", env)
    sarif_path = svc / "_reports" / "gosec.sarif"
    sarif_data = json.loads(sarif_path.read_text(encoding="utf-8")) if sarif_path.is_file() else {}
    sarif_results = [item for run in sarif_data.get("runs", []) for item in run.get("results", [])]
    files_after = snapshot_tree(svc)
    check("go.sh exits 0", res.returncode == 0, f"rc={res.returncode}, stderr={res.stderr!r}")
    check(
        "summary includes canary PASS", "canary" in res.stdout and "PASS" in res.stdout, res.stdout
    )
    check(
        "no canary files left in project dir",
        not any("canary" in p.lower() for p in files_after),
        f"files={sorted(files_after)}",
    )
    check("gosec.sarif contains 0 canary findings", len(sarif_results) == 0, repr(sarif_results))

sys.stdout.write("A2. Go broken gosec (silent exit 0 + empty SARIF) fails canary audit\n")
with tempfile.TemporaryDirectory() as td:
    repo, svc, env = build_go_test_repo(pathlib.Path(td), gosec_mode="silent_pass")
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

sys.stdout.write("A3. Go broken golangci-lint (silent exit 0) fails canary audit\n")
with tempfile.TemporaryDirectory() as td:
    repo, svc, env = build_go_test_repo(pathlib.Path(td), golangci_mode="silent_pass")
    res = run_go_sh(repo, "svc-go", env)
    files_after = snapshot_tree(svc)
    check("go.sh exits 1", res.returncode == 1, f"rc={res.returncode}, stdout={res.stdout!r}")
    check("stderr reports CANARY AUDIT FAILED", "CANARY AUDIT FAILED" in res.stderr, res.stderr)
    check(
        "no canary files left",
        not any("canary" in p.lower() for p in files_after),
        f"files={sorted(files_after)}",
    )

sys.stdout.write("A4. Go gosec crashing on canary with empty SARIF fails canary audit\n")
with tempfile.TemporaryDirectory() as td:
    repo, svc, env = build_go_test_repo(pathlib.Path(td), gosec_mode="canary_crash_empty_sarif")
    res = run_go_sh(repo, "svc-go", env)
    check("go.sh exits 1", res.returncode == 1, f"rc={res.returncode}")
    check("stderr reports CANARY AUDIT FAILED", "CANARY AUDIT FAILED" in res.stderr, res.stderr)

sys.stdout.write("A5. Go gosec scanning 0 files on real module fails with exit 1\n")
with tempfile.TemporaryDirectory() as td:
    repo, svc, env = build_go_test_repo(pathlib.Path(td), gosec_mode="zero_files")
    res = run_go_sh(repo, "svc-go", env)
    check("go.sh exits 1", res.returncode == 1, f"rc={res.returncode}, stdout={res.stdout!r}")
    check(
        "stderr reports ZERO-FILE SCAN DETECTED",
        "ZERO-FILE SCAN DETECTED" in res.stderr,
        res.stderr,
    )

sys.stdout.write("A6. Go gosec 0 files with package load errors fails with exit 1\n")
with tempfile.TemporaryDirectory() as td:
    repo, svc, env = build_go_test_repo(pathlib.Path(td), gosec_mode="zero_files_load_error")
    res = run_go_sh(repo, "svc-go", env)
    check("go.sh exits 1", res.returncode == 1, f"rc={res.returncode}")
    check(
        "stderr reports ZERO-FILE SCAN DETECTED",
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
    gosec_sarif.write_text('{"version":"2.1.0","runs":[{"results":[]}]}', encoding="utf-8")
    res = subprocess.run(  # noqa: S603 - fixed argv running canary_audit.py under test
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
    check("go-verify-scan exits 0 for generated-only module", res.returncode == 0, res.stderr)

sys.stdout.write("A8. Real Go scanners pass isolated canary audit against pki-agent\n")
go_ver = detect_go_toolchain_version()
cache_root = pathlib.Path(os.environ.get("XDG_CACHE_HOME", pathlib.Path.home() / ".cache"))
real_cache_base = cache_root / "alt-lint" / go_ver
real_golangci = real_cache_base / f"golangci-lint-{GOLANGCI_LINT_VERSION}" / "golangci-lint"
real_gosec = real_cache_base / f"gosec-{GOSEC_VERSION}" / "gosec"
if real_golangci.is_file() and real_gosec.is_file():
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
            "--gosec-bin",
            str(real_gosec),
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
    check("real Go scanner binaries cached (skipped if absent)", True)

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
