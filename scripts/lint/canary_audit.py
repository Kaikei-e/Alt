#!/usr/bin/env python3
"""Isolated canary self-audit and 0-file scan guard for Go and Python linters (REPO-02).

Prevents silent-pass scanner failures (e.g., Go toolchain/loader mismatches where
gosec exits 0 with an empty SARIF report) by:
1. Running the resolved scanner binaries against an isolated temporary canary project
   containing deterministic, high-confidence violations outside the target repo tree.
2. Verifying that production scans analyzed > 0 files whenever non-generated source
   files exist in the target project.

Subcommands:
    go-canary                 Run isolated golangci-lint (incl. gosec SARIF) canary check.
    go-verify-scan            Verify Go scan coverage and SARIF integrity on non-generated sources.
    python-canary             Run isolated ruff (and optional bandit) canary checks.
    python-verify-files       Verify target directory contains > 0 non-generated .py files.
    python-verify-bandit-scan Verify bandit scanned > 0 lines on non-empty Python sources.
"""

from __future__ import annotations

import argparse
import contextlib
import json
import os
import pathlib
import re
import shutil
import subprocess
import sys
import tempfile
from collections.abc import Iterator
from dataclasses import dataclass

EXIT_OK = 0
EXIT_FAIL = 1
EXIT_USAGE = 2

GOLANGCI_FINDINGS_RC = 1
RUFF_FINDINGS_RC = 1
OUTPUT_SNIPPET_LIMIT = 400

GO_CANARY_EXPECTED_RULE = "G402"
GO_CANARY_ALT_LINT_MARKER = "ineffassign"
RUFF_CANARY_EXPECTED_RULE = "F401"
BANDIT_CANARY_EXPECTED_RULES = ("B602", "B307")

GO_EXCLUDED_DIRS = frozenset({".git", "vendor", "gen", "_reports", "node_modules"})
GO_GENERATED_SUFFIXES = (".pb.go", ".connect.go")
GO_GENERATED_EXACT = frozenset({"server.gen.go"})

PY_EXCLUDED_DIRS = frozenset(
    {
        ".git",
        ".venv",
        "venv",
        "build",
        "dist",
        "__pycache__",
        ".mypy_cache",
        ".pytest_cache",
        ".ruff_cache",
        "node_modules",
        "gen",
        "generated",
    }
)
PY_GENERATED_SUFFIXES = ("_pb2.py", "_pb2_grpc.py", "_connect.py", "_gen.py", ".gen.py")

GO_DIRECTIVE_RE = re.compile(r"^go\s+(\S+)")
TOOLCHAIN_DIRECTIVE_RE = re.compile(r"^toolchain\s+(\S+)")
GOSEC_FILES_RE = re.compile(r"^\s*Files\s*:\s*(\d+)\s*$", re.MULTILINE)
GOSEC_RULE_PREFIX_RE = re.compile(r"^(G\d{3}):")
BANDIT_LOC_RE = re.compile(r"Total lines of code:\s*(\d+)")

GO_CANARY_SOURCE = """package canary

import "crypto/tls"

// CanaryCheck intentionally contains known lint and security violations
// to verify static analyzers do not silently pass with zero findings.
func CanaryCheck() *tls.Config {
\tx := 1
\tx = 2
\t_ = x
\treturn &tls.Config{InsecureSkipVerify: true}
}
"""

PY_CANARY_SOURCE = """import os
import subprocess


def canary_check(cmd: str) -> None:
    subprocess.Popen(cmd, shell=True)
    eval(cmd)
"""


@dataclass(frozen=True)
class GoCanaryConfig:
    """Parameters for running isolated Go scanner canary checks."""

    project_dir: pathlib.Path
    config_file: pathlib.Path
    golangci_lint_bin: pathlib.Path
    gosec_bin: pathlib.Path | None = None
    work_dir: pathlib.Path | None = None


@dataclass(frozen=True)
class PythonCanaryConfig:
    """Parameters for running isolated Python scanner canary checks."""

    project_dir: pathlib.Path
    check_bandit: bool
    work_dir: pathlib.Path | None = None


def _info(message: str) -> None:
    sys.stdout.write(f"{message}\n")


def _error(message: str) -> None:
    sys.stderr.write(f"{message}\n")


def _snippet(text: str) -> str:
    compact = " ".join(text.split())
    if len(compact) <= OUTPUT_SNIPPET_LIMIT:
        return compact
    return f"{compact[:OUTPUT_SNIPPET_LIMIT]}..."


def _work_dir_context(
    work_dir: pathlib.Path | None,
    prefix: str,
) -> contextlib.AbstractContextManager[pathlib.Path]:
    if work_dir is not None:
        work_dir.mkdir(parents=True, exist_ok=True)
        return contextlib.nullcontext(work_dir)

    @contextlib.contextmanager
    def _managed_temp() -> Iterator[pathlib.Path]:
        with tempfile.TemporaryDirectory(prefix=prefix) as tmp:
            yield pathlib.Path(tmp)

    return _managed_temp()


def extract_go_mod_directives(go_mod_path: pathlib.Path) -> tuple[str | None, str | None]:
    """Read `go` and optional `toolchain` directives from a go.mod file."""
    go_version: str | None = None
    toolchain_version: str | None = None
    for raw_line in go_mod_path.read_text(encoding="utf-8").splitlines():
        line = raw_line.strip()
        if go_version is None:
            go_match = GO_DIRECTIVE_RE.match(line)
            if go_match is not None:
                go_version = go_match.group(1)
        if toolchain_version is None:
            tc_match = TOOLCHAIN_DIRECTIVE_RE.match(line)
            if tc_match is not None:
                toolchain_version = tc_match.group(1)
    return go_version, toolchain_version


def extract_sarif_rule_ids(sarif_data: object) -> set[str]:
    """Collect ruleId strings (plus concrete G<NNN> gosec IDs from message.text) from SARIF."""
    if not isinstance(sarif_data, dict):
        return set()
    runs = sarif_data.get("runs")
    if not isinstance(runs, list):
        return set()
    rule_ids: set[str] = set()
    for run in runs:
        if not isinstance(run, dict):
            continue
        results = run.get("results")
        if not isinstance(results, list):
            continue
        for item in results:
            if not isinstance(item, dict):
                continue
            rule_id = item.get("ruleId")
            if isinstance(rule_id, str) and rule_id:
                rule_ids.add(rule_id)
                if rule_id == "gosec":
                    message = item.get("message")
                    msg_text = message.get("text") if isinstance(message, dict) else None
                    if isinstance(msg_text, str):
                        gosec_match = GOSEC_RULE_PREFIX_RE.match(msg_text)
                        if gosec_match is not None:
                            rule_ids.add(gosec_match.group(1))
    return rule_ids


def _prepare_go_canary_dir(
    canary_dir: pathlib.Path,
    go_version: str,
    toolchain_version: str | None,
) -> None:
    go_mod_lines = ["module canary", "", f"go {go_version}"]
    if toolchain_version is not None:
        go_mod_lines.append(f"toolchain {toolchain_version}")
    (canary_dir / "go.mod").write_text("\n".join(go_mod_lines) + "\n", encoding="utf-8")
    (canary_dir / "canary.go").write_text(GO_CANARY_SOURCE, encoding="utf-8")


def _check_golangci_canary(
    cfg: GoCanaryConfig,
    canary_dir: pathlib.Path,
    env: dict[str, str],
) -> bool:
    sarif_path = canary_dir / "canary.sarif"
    cmd = [
        str(cfg.golangci_lint_bin),
        "run",
        "--config",
        str(cfg.config_file),
        "--output.text.path",
        "stdout",
        "--output.sarif.path",
        str(sarif_path),
    ]
    try:
        res = subprocess.run(  # noqa: S603 - resolved linter binary with fixed argv
            cmd,
            cwd=str(canary_dir),
            env=env,
            capture_output=True,
            text=True,
            check=False,
        )
    except OSError as exc:
        _error(f"ERROR: CANARY AUDIT FAILED (go/golangci-lint): execution error: {exc}")
        return False

    combined = f"{res.stdout}\n{res.stderr}"
    if res.returncode != GOLANGCI_FINDINGS_RC:
        _error(
            "ERROR: CANARY AUDIT FAILED (go/golangci-lint): "
            f"expected exit {GOLANGCI_FINDINGS_RC} with SARIF rules "
            f"{GO_CANARY_EXPECTED_RULE} and {GO_CANARY_ALT_LINT_MARKER}, "
            f"got exit {res.returncode} (output: {_snippet(combined)})"
        )
        return False

    if not sarif_path.is_file():
        _error(
            "ERROR: CANARY AUDIT FAILED (go/golangci-lint): "
            f"missing canary SARIF output at {sarif_path} (output: {_snippet(combined)})"
        )
        return False

    try:
        sarif_data = json.loads(sarif_path.read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError) as exc:
        _error(f"ERROR: CANARY AUDIT FAILED (go/golangci-lint): invalid canary SARIF JSON: {exc}")
        return False

    rule_ids = extract_sarif_rule_ids(sarif_data)
    missing_rules = [
        r for r in (GO_CANARY_EXPECTED_RULE, GO_CANARY_ALT_LINT_MARKER) if r not in rule_ids
    ]
    if missing_rules:
        _error(
            "ERROR: CANARY AUDIT FAILED (go/golangci-lint): "
            f"canary SARIF missing expected rule(s) {missing_rules}, "
            f"found rules={sorted(rule_ids)} (output: {_snippet(combined)})"
        )
        return False
    return True


def run_go_canary(cfg: GoCanaryConfig) -> int:
    """Execute isolated golangci-lint (incl. gosec SARIF) canary self-audit."""
    go_mod_path = cfg.project_dir / "go.mod"
    if not go_mod_path.is_file():
        _error(f"ERROR: CANARY AUDIT FAILED (go): go.mod not found at {go_mod_path}")
        return EXIT_FAIL

    go_version, toolchain_version = extract_go_mod_directives(go_mod_path)
    if go_version is None:
        _error(f"ERROR: CANARY AUDIT FAILED (go): missing 'go' directive in {go_mod_path}")
        return EXIT_FAIL

    env = os.environ.copy()
    env["GOFLAGS"] = "-buildvcs=false"

    with _work_dir_context(cfg.work_dir, prefix="alt-go-canary-") as canary_dir:
        _prepare_go_canary_dir(canary_dir, go_version, toolchain_version)
        if not _check_golangci_canary(cfg, canary_dir, env):
            return EXIT_FAIL

    _info(
        f"canary audit passed (go): golangci-lint (exit {GOLANGCI_FINDINGS_RC}, "
        f"SARIF rules {GO_CANARY_EXPECTED_RULE}, {GO_CANARY_ALT_LINT_MARKER})"
    )
    return EXIT_OK


def count_non_generated_go_files(project_dir: pathlib.Path) -> int:
    """Count non-test .go files excluding gen/, *.pb.go, *.connect.go, and server.gen.go."""
    count = 0
    for _root, dirnames, filenames in project_dir.walk():
        dirnames[:] = [d for d in dirnames if d not in GO_EXCLUDED_DIRS]
        for fname in filenames:
            if not fname.endswith(".go") or fname.endswith("_test.go"):
                continue
            if fname in GO_GENERATED_EXACT or fname.endswith(GO_GENERATED_SUFFIXES):
                continue
            count += 1
    return count


def _sarif_contains_canary_uri(sarif_data: object) -> bool:
    if not isinstance(sarif_data, dict):
        return False
    runs = sarif_data.get("runs")
    if not isinstance(runs, list):
        return False
    for run in runs:
        if not isinstance(run, dict):
            continue
        for item in run.get("results", []):
            if not isinstance(item, dict):
                continue
            for loc in item.get("locations", []):
                if not isinstance(loc, dict):
                    continue
                phys = loc.get("physicalLocation", {})
                uri = phys.get("artifactLocation", {}).get("uri", "")
                if isinstance(uri, str) and "alt-go-canary-" in uri:
                    return True
    return False


def _parse_gosec_files_scanned(
    gosec_out_path: pathlib.Path,
    project_dir: pathlib.Path,
    non_gen_count: int,
) -> int | None:
    if not gosec_out_path.is_file():
        _error(
            "ERROR: ZERO-FILE SCAN DETECTED (go): "
            f"missing gosec output file {gosec_out_path} despite "
            f"{non_gen_count} non-generated .go files"
        )
        return None

    out_text = gosec_out_path.read_text(encoding="utf-8", errors="replace")
    files_match = GOSEC_FILES_RE.search(out_text)
    if files_match is None:
        _error(
            "ERROR: ZERO-FILE SCAN DETECTED (go): "
            f"gosec output missing 'Files:' summary despite {non_gen_count} non-generated .go files"
        )
        return None

    files_scanned = int(files_match.group(1))
    if files_scanned == 0:
        has_load_errors = "Golang errors in file:" in out_text
        load_note = " (with Golang package load errors)" if has_load_errors else ""
        _error(
            "ERROR: ZERO-FILE SCAN DETECTED (go): "
            f"gosec scanned 0 files{load_note} despite {non_gen_count} "
            f"non-generated .go files in {project_dir}"
        )
        return None

    return files_scanned


def _validate_sarif_run(run: object, gosec_sarif_path: pathlib.Path) -> bool:
    if not isinstance(run, dict):
        _error(f"ERROR: invalid gosec SARIF at {gosec_sarif_path}: run entry must be an object")
        return False

    tool = run.get("tool")
    driver = tool.get("driver") if isinstance(tool, dict) else None
    rules = driver.get("rules") if isinstance(driver, dict) else None
    if not isinstance(rules, list):
        _error(
            f"ERROR: invalid gosec SARIF at {gosec_sarif_path}: "
            "missing tool.driver.rules list (SARIF 2.1.0 compliance)"
        )
        return False

    declared_rule_ids = {
        r["id"] for r in rules if isinstance(r, dict) and isinstance(r.get("id"), str) and r["id"]
    }
    has_artifacts = run.get("artifacts") is not None
    results = run.get("results", [])
    if not isinstance(results, list):
        _error(f"ERROR: invalid gosec SARIF at {gosec_sarif_path}: run.results must be a list")
        return False

    for item in results:
        if not isinstance(item, dict):
            continue
        rule_id = item.get("ruleId")
        if isinstance(rule_id, str) and rule_id not in declared_rule_ids:
            _error(
                f"ERROR: invalid gosec SARIF at {gosec_sarif_path}: "
                f"result ruleId {rule_id!r} missing from tool.driver.rules"
            )
            return False
        if not has_artifacts:
            for loc in item.get("locations", []):
                if not isinstance(loc, dict):
                    continue
                phys = loc.get("physicalLocation")
                art_loc = phys.get("artifactLocation") if isinstance(phys, dict) else None
                if isinstance(art_loc, dict) and "index" in art_loc:
                    _error(
                        f"ERROR: invalid gosec SARIF at {gosec_sarif_path}: "
                        "artifactLocation.index present without run.artifacts"
                    )
                    return False
    return True


def _validate_gosec_sarif(gosec_sarif_path: pathlib.Path) -> bool:
    if not gosec_sarif_path.is_file():
        _error(f"ERROR: missing gosec SARIF at {gosec_sarif_path}")
        return False

    try:
        sarif_data = json.loads(gosec_sarif_path.read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError) as exc:
        _error(f"ERROR: invalid gosec SARIF at {gosec_sarif_path}: {exc}")
        return False

    if not isinstance(sarif_data, dict) or not isinstance(sarif_data.get("runs"), list):
        _error(f"ERROR: invalid gosec SARIF at {gosec_sarif_path}: missing top-level 'runs' list")
        return False

    if _sarif_contains_canary_uri(sarif_data):
        _error(
            "ERROR: CANARY AUDIT FAILED (go): "
            f"production SARIF {gosec_sarif_path} contaminated by canary artifact"
        )
        return False

    return all(_validate_sarif_run(run, gosec_sarif_path) for run in sarif_data["runs"])


def _verify_go_packages_loaded(
    project_dir: pathlib.Path,
    non_gen_count: int,
) -> int | None:
    go_bin = shutil.which("go")
    if go_bin is None:
        _error(
            "ERROR: ZERO-FILE SCAN DETECTED (go): "
            f"go executable not found in PATH despite {non_gen_count} non-generated .go files"
        )
        return None

    env = os.environ.copy()
    env["GOFLAGS"] = "-buildvcs=false"
    try:
        res = subprocess.run(  # noqa: S603 - resolved go binary with fixed argv
            [go_bin, "list", "./..."],
            cwd=str(project_dir),
            env=env,
            capture_output=True,
            text=True,
            check=False,
        )
    except OSError as exc:
        _error(f"ERROR: ZERO-FILE SCAN DETECTED (go): failed to run 'go list ./...': {exc}")
        return None

    combined = f"{res.stdout}\n{res.stderr}"
    if res.returncode != 0:
        _error(
            "ERROR: ZERO-FILE SCAN DETECTED (go): "
            f"'go list ./...' failed (exit {res.returncode}) despite {non_gen_count} "
            f"non-generated .go files in {project_dir} (output: {_snippet(combined)})"
        )
        return None

    packages = [line.strip() for line in res.stdout.splitlines() if line.strip()]
    if not packages:
        _error(
            "ERROR: ZERO-FILE SCAN DETECTED (go): "
            f"'go list ./...' resolved 0 Go packages despite {non_gen_count} "
            f"non-generated .go files in {project_dir} (output: {_snippet(combined)})"
        )
        return None

    return len(packages)


def _verify_go_scan_legacy(
    project_dir: pathlib.Path,
    gosec_out_path: pathlib.Path,
    gosec_sarif_path: pathlib.Path,
    non_gen_count: int,
) -> int:
    if non_gen_count == 0:
        _info(f"go scan check skipped: 0 non-generated .go files in {project_dir}")
        return EXIT_OK

    files_scanned = _parse_gosec_files_scanned(gosec_out_path, project_dir, non_gen_count)
    if files_scanned is None or not _validate_gosec_sarif(gosec_sarif_path):
        return EXIT_FAIL

    _info(
        f"go scan check passed: gosec scanned {files_scanned} files "
        f"({non_gen_count} non-generated .go files)"
    )
    return EXIT_OK


def verify_go_scan(
    project_dir: pathlib.Path,
    gosec_out_path: pathlib.Path | None,
    gosec_sarif_path: pathlib.Path,
) -> int:
    """Verify Go scan coverage and SARIF integrity when non-generated .go files exist."""
    non_gen_count = count_non_generated_go_files(project_dir)
    if gosec_out_path is not None:
        return _verify_go_scan_legacy(project_dir, gosec_out_path, gosec_sarif_path, non_gen_count)

    if not _validate_gosec_sarif(gosec_sarif_path):
        return EXIT_FAIL

    if non_gen_count == 0:
        _info(f"go scan check skipped: 0 non-generated .go files in {project_dir}")
        return EXIT_OK

    package_count = _verify_go_packages_loaded(project_dir, non_gen_count)
    if package_count is None:
        return EXIT_FAIL

    _info(
        f"go scan check passed: resolved {package_count} Go package(s) "
        f"({non_gen_count} non-generated .go files)"
    )
    return EXIT_OK


def _is_excluded_py_dir(dirname: str) -> bool:
    return dirname in PY_EXCLUDED_DIRS or dirname.endswith(".egg-info")


def _is_generated_py_file(filename: str) -> bool:
    return filename.endswith(PY_GENERATED_SUFFIXES)


def list_non_generated_py_files(project_dir: pathlib.Path) -> list[pathlib.Path]:
    """List non-generated .py files excluding .venv, build, dist, gen/, and harness/corpus/."""
    files: list[pathlib.Path] = []
    for root, dirnames, filenames in project_dir.walk():
        dirnames[:] = [
            d
            for d in dirnames
            if not _is_excluded_py_dir(d) and not (root.name == "harness" and d == "corpus")
        ]
        for fname in filenames:
            if not fname.endswith(".py") or _is_generated_py_file(fname):
                continue
            files.append(root / fname)
    return files


def verify_python_files(project_dir: pathlib.Path) -> int:
    """Verify at least one non-generated .py file exists in project_dir."""
    py_files = list_non_generated_py_files(project_dir)
    if not py_files:
        _error(
            "ERROR: ZERO-FILE SCAN DETECTED (python): "
            f"no non-generated .py files found in {project_dir}"
        )
        return EXIT_FAIL
    _info(f"python source check passed: {len(py_files)} non-generated .py files found")
    return EXIT_OK


def _file_has_code_lines(path: pathlib.Path) -> bool:
    try:
        for raw_line in path.read_text(encoding="utf-8", errors="replace").splitlines():
            stripped = raw_line.strip()
            if stripped and not stripped.startswith("#"):
                return True
    except OSError:
        return False
    return False


def verify_python_bandit_scan(
    project_dir: pathlib.Path,
    bandit_out_path: pathlib.Path,
) -> int:
    """Verify bandit scanned > 0 lines of code when non-empty .py files exist."""
    code_files = [p for p in list_non_generated_py_files(project_dir) if _file_has_code_lines(p)]
    if not code_files:
        return EXIT_OK

    if not bandit_out_path.is_file():
        _error(
            "ERROR: ZERO-FILE SCAN DETECTED (python/bandit): "
            f"missing bandit output file {bandit_out_path}"
        )
        return EXIT_FAIL

    text = bandit_out_path.read_text(encoding="utf-8", errors="replace")
    match = BANDIT_LOC_RE.search(text)
    if match is None:
        _error(
            "ERROR: ZERO-FILE SCAN DETECTED (python/bandit): "
            "bandit output missing 'Total lines of code:' summary "
            f"despite {len(code_files)} non-empty .py files"
        )
        return EXIT_FAIL

    lines_scanned = int(match.group(1))
    if lines_scanned == 0:
        _error(
            "ERROR: ZERO-FILE SCAN DETECTED (python/bandit): "
            f"bandit scanned 0 lines of code despite {len(code_files)} "
            f"non-empty .py files in {project_dir}"
        )
        return EXIT_FAIL

    _info(f"bandit scan check passed: {lines_scanned} lines of code scanned")
    return EXIT_OK


def _check_ruff_canary(
    uv_bin: str,
    project_dir: pathlib.Path,
    canary_py: pathlib.Path,
) -> bool:
    pyproject_path = project_dir / "pyproject.toml"
    cmd = [
        uv_bin,
        "run",
        "--frozen",
        "--only-group",
        "dev",
        "ruff",
        "check",
        "--no-cache",
        "--config",
        str(pyproject_path),
        str(canary_py),
    ]
    try:
        res = subprocess.run(  # noqa: S603 - resolved uv binary with fixed argv
            cmd,
            cwd=str(project_dir),
            capture_output=True,
            text=True,
            check=False,
        )
    except OSError as exc:
        _error(f"ERROR: CANARY AUDIT FAILED (python/ruff): execution error: {exc}")
        return False

    combined = f"{res.stdout}\n{res.stderr}"
    if res.returncode != RUFF_FINDINGS_RC or RUFF_CANARY_EXPECTED_RULE not in combined:
        _error(
            "ERROR: CANARY AUDIT FAILED (python/ruff): "
            f"expected exit {RUFF_FINDINGS_RC} with rule {RUFF_CANARY_EXPECTED_RULE}, "
            f"got exit {res.returncode} (output: {_snippet(combined)})"
        )
        return False
    return True


def _check_bandit_canary(
    uv_bin: str,
    project_dir: pathlib.Path,
    canary_dir: pathlib.Path,
) -> bool:
    cmd = [
        uv_bin,
        "run",
        "--frozen",
        "--only-group",
        "dev",
        "bandit",
        "-r",
        str(canary_dir),
        "-ll",
        "-ii",
    ]
    try:
        res = subprocess.run(  # noqa: S603 - resolved uv binary with fixed argv
            cmd,
            cwd=str(project_dir),
            capture_output=True,
            text=True,
            check=False,
        )
    except OSError as exc:
        _error(f"ERROR: CANARY AUDIT FAILED (python/bandit): execution error: {exc}")
        return False

    combined = f"{res.stdout}\n{res.stderr}"
    detected = any(rule in combined for rule in BANDIT_CANARY_EXPECTED_RULES)
    if res.returncode == 0 or not detected:
        _error(
            "ERROR: CANARY AUDIT FAILED (python/bandit): "
            f"expected non-zero exit and one of {BANDIT_CANARY_EXPECTED_RULES}, "
            f"got exit {res.returncode} (output: {_snippet(combined)})"
        )
        return False
    return True


def run_python_canary(cfg: PythonCanaryConfig) -> int:
    """Execute isolated ruff (and optional bandit) canary self-audit."""
    pyproject_path = cfg.project_dir / "pyproject.toml"
    if not pyproject_path.is_file():
        _error(f"ERROR: CANARY AUDIT FAILED (python): pyproject.toml not found at {pyproject_path}")
        return EXIT_FAIL

    uv_bin = shutil.which("uv")
    if uv_bin is None:
        _error("ERROR: CANARY AUDIT FAILED (python): uv executable not found in PATH")
        return EXIT_FAIL

    with _work_dir_context(cfg.work_dir, prefix="alt-py-canary-") as canary_dir:
        canary_py = canary_dir / "canary.py"
        canary_py.write_text(PY_CANARY_SOURCE, encoding="utf-8")

        if not _check_ruff_canary(uv_bin, cfg.project_dir, canary_py):
            return EXIT_FAIL

        if cfg.check_bandit and not _check_bandit_canary(uv_bin, cfg.project_dir, canary_dir):
            return EXIT_FAIL

    bandit_note = ", bandit (B602/B307)" if cfg.check_bandit else ""
    _info(f"canary audit passed (python): ruff ({RUFF_CANARY_EXPECTED_RULE}){bandit_note}")
    return EXIT_OK


def _parse_bool_flag(value: str) -> bool:
    lowered = value.strip().lower()
    if lowered in {"true", "1", "yes"}:
        return True
    if lowered in {"false", "0", "no"}:
        return False
    msg = f"invalid boolean value: {value}"
    raise argparse.ArgumentTypeError(msg)


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(
        description="Isolated canary self-audit and 0-file scan guard for Go and Python linters.",
    )
    subparsers = parser.add_subparsers(dest="command", required=True)

    go_canary_p = subparsers.add_parser("go-canary")
    go_canary_p.add_argument("--project-dir", type=pathlib.Path, required=True)
    go_canary_p.add_argument("--config", type=pathlib.Path, required=True)
    go_canary_p.add_argument("--golangci-lint-bin", type=pathlib.Path, required=True)
    go_canary_p.add_argument("--gosec-bin", type=pathlib.Path, default=None)
    go_canary_p.add_argument("--work-dir", type=pathlib.Path, default=None)

    go_scan_p = subparsers.add_parser("go-verify-scan")
    go_scan_p.add_argument("--project-dir", type=pathlib.Path, required=True)
    go_scan_p.add_argument("--gosec-out", type=pathlib.Path, default=None)
    go_scan_p.add_argument("--gosec-sarif", type=pathlib.Path, required=True)

    py_canary_p = subparsers.add_parser("python-canary")
    py_canary_p.add_argument("--project-dir", type=pathlib.Path, required=True)
    py_canary_p.add_argument("--check-bandit", type=_parse_bool_flag, default=False)
    py_canary_p.add_argument("--work-dir", type=pathlib.Path, default=None)

    py_files_p = subparsers.add_parser("python-verify-files")
    py_files_p.add_argument("--project-dir", type=pathlib.Path, required=True)

    py_bandit_p = subparsers.add_parser("python-verify-bandit-scan")
    py_bandit_p.add_argument("--project-dir", type=pathlib.Path, required=True)
    py_bandit_p.add_argument("--bandit-out", type=pathlib.Path, required=True)

    return parser


def main(argv: list[str] | None = None) -> int:
    parser = build_parser()
    args = parser.parse_args(argv)

    if args.command == "go-canary":
        return run_go_canary(
            GoCanaryConfig(
                project_dir=args.project_dir.resolve(),
                config_file=args.config.resolve(),
                golangci_lint_bin=args.golangci_lint_bin.resolve(),
                gosec_bin=args.gosec_bin.resolve() if args.gosec_bin is not None else None,
                work_dir=args.work_dir.resolve() if args.work_dir is not None else None,
            )
        )
    if args.command == "go-verify-scan":
        return verify_go_scan(
            project_dir=args.project_dir.resolve(),
            gosec_out_path=args.gosec_out.resolve() if args.gosec_out is not None else None,
            gosec_sarif_path=args.gosec_sarif.resolve(),
        )
    if args.command == "python-canary":
        return run_python_canary(
            PythonCanaryConfig(
                project_dir=args.project_dir.resolve(),
                check_bandit=args.check_bandit,
                work_dir=args.work_dir.resolve() if args.work_dir is not None else None,
            )
        )
    if args.command == "python-verify-files":
        return verify_python_files(args.project_dir.resolve())
    if args.command == "python-verify-bandit-scan":
        return verify_python_bandit_scan(
            project_dir=args.project_dir.resolve(),
            bandit_out_path=args.bandit_out.resolve(),
        )

    _error(f"ERROR: unknown command: {args.command}")
    return EXIT_USAGE


if __name__ == "__main__":
    sys.exit(main())
