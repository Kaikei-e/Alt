#!/usr/bin/env python3
"""Isolated canary self-audit, scan coverage guard, and coverage ledger for Alt linters.

Prevents silent-pass scanner failures (e.g., Go toolchain/loader mismatches where
gosec exits 0 with an empty SARIF report) by:
1. Running the resolved scanner binaries against isolated temporary canary projects
   containing deterministic, high-confidence violations outside the target repo tree.
2. Verifying that production scans analyzed > 0 files/packages/modules whenever
   non-generated source files exist in the target project.
3. Enforcing a 45-target canary coverage ledger (`scripts/lint/canary_ledger.toml`)
   against `./scripts/lint/run.sh --list`.

Subcommands:
    go-canary                 Run isolated golangci-lint (incl. gosec SARIF) canary check.
    go-verify-scan            Verify Go scan coverage and SARIF integrity on non-generated sources.
    go-verify-govulncheck     Verify govulncheck DB metadata and SBOM module/package coverage.
    python-canary             Run isolated ruff (and optional bandit/pyrefly) canary checks.
    python-verify-files       Verify target directory contains > 0 non-generated .py files.
    python-verify-bandit-scan Verify bandit scanned > 0 lines on non-empty Python sources.
    rust-canary               Run isolated cargo clippy canary check.
    frontend-canary           Run isolated Biome lint canary check.
    deno-canary               Run isolated deno lint canary check.
    proto-canary              Run isolated buf lint canary check.
    actionlint-canary         Run isolated actionlint canary check.
    verify-ledger             Verify canary_ledger.toml accounts for every target in run.sh --list.
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
import tomllib
from collections.abc import Iterator
from dataclasses import dataclass

EXIT_OK = 0
EXIT_FAIL = 1
EXIT_USAGE = 2

GOLANGCI_FINDINGS_RC = 1
RUFF_FINDINGS_RC = 1
OUTPUT_SNIPPET_LIMIT = 400
MIN_EXEMPT_REASON_LEN = 10

GO_CANARY_EXPECTED_RULE = "G402"
GO_CANARY_ALT_LINT_MARKER = "ineffassign"
RUFF_CANARY_EXPECTED_RULE = "F401"
BANDIT_CANARY_EXPECTED_RULES = ("B602", "B307")
PYREFLY_CANARY_MARKERS = ("bad-assignment", "not assignable", "error")
RUST_CANARY_MARKERS = ("eq_op", "equal expressions")
FRONTEND_CANARY_MARKERS = ("nodebugger", "debugger")
DENO_CANARY_EXPECTED_RULE = "no-debugger"
PROTO_CANARY_MARKERS = ("bad_enum", "badpkg", "enum_", "package_")
ACTIONLINT_CANARY_MARKERS = ("invalid_context", "expression")
ACTIONLINT_GO_MODULE = "github.com/rhysd/actionlint/cmd/actionlint@v1.7.12"

VALID_LEDGER_KINDS = frozenset({"go", "python", "rust", "frontend", "deno", "proto", "named"})
VALID_SCANNER_STATUSES = frozenset({"canary", "exempt", "not-applicable"})
VALID_CANARY_COMMANDS = frozenset(
    {
        "go-canary",
        "go-verify-govulncheck",
        "python-canary",
        "rust-canary",
        "frontend-canary",
        "deno-canary",
        "proto-canary",
        "actionlint-canary",
    }
)
REQUIRED_CANARY_SCANNERS_BY_KIND: dict[str, tuple[str, ...]] = {
    "go": ("golangci-lint", "gosec", "govulncheck"),
    "python": ("ruff",),
    "rust": ("clippy",),
    "frontend": ("biome",),
    "deno": ("deno-lint",),
    "proto": ("buf-lint",),
    "named": (),
}

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
RUST_CHANNEL_RE = re.compile(r'^\s*channel\s*=\s*"([^"]+)"')
DEP_NAME_SPLIT_RE = re.compile(r"[\s\[<>=!~;@]")
DEP_NORM_RE = re.compile(r"[-_.]+")

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

PY_PYREFLY_CANARY_SOURCE = '''"""Isolated pyrefly type-error canary."""


def canary_type_error() -> int:
    value: int = "not_an_int"
    return value
'''

RUST_CANARY_CARGO_TOML = """[package]
name = "alt-rust-canary"
version = "0.1.0"
edition = "2021"
"""

RUST_CANARY_LIB_RS = """pub fn canary_check(x: i32) -> bool {
    x == x
}
"""

FRONTEND_CANARY_BIOME_JSON = """{
  "linter": {
    "enabled": true,
    "rules": {
      "recommended": true
    }
  }
}
"""

TS_DEBUGGER_CANARY_SOURCE = """export function canaryCheck(value: number): number {
  debugger;
  return value;
}
"""

PROTO_CANARY_BUF_YAML = """version: v2
lint:
  use:
    - STANDARD
"""

PROTO_CANARY_SOURCE = """syntax = "proto3";

package badpkg;

enum bad_enum {
  bad_value = 0;
}
"""

ACTIONLINT_CANARY_WORKFLOW = (
    "name: Canary Broken Workflow\n"
    "on: push\n"
    "jobs:\n"
    "  broken:\n"
    "    runs-on: ubuntu-latest\n"
    "    steps:\n"
    "      - run: echo $" + "{{ invalid_context.nonexistent }}\n"
)


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
    check_pyrefly: bool = False
    work_dir: pathlib.Path | None = None


@dataclass(frozen=True)
class RustCanaryConfig:
    """Parameters for running isolated Rust cargo clippy canary checks."""

    project_dir: pathlib.Path
    toolchain_file: pathlib.Path | None = None
    cargo_bin: pathlib.Path | None = None
    target_dir: pathlib.Path | None = None
    work_dir: pathlib.Path | None = None


@dataclass(frozen=True)
class FrontendCanaryConfig:
    """Parameters for running isolated Biome lint canary checks."""

    project_dir: pathlib.Path
    biome_bin: pathlib.Path | None = None
    work_dir: pathlib.Path | None = None


@dataclass(frozen=True)
class DenoCanaryConfig:
    """Parameters for running isolated deno lint canary checks."""

    project_dir: pathlib.Path
    deno_bin: pathlib.Path | None = None
    work_dir: pathlib.Path | None = None


@dataclass(frozen=True)
class ProtoCanaryConfig:
    """Parameters for running isolated buf lint canary checks."""

    buf_bin: pathlib.Path | None = None
    work_dir: pathlib.Path | None = None


@dataclass(frozen=True)
class ActionlintCanaryConfig:
    """Parameters for running isolated actionlint canary checks."""

    actionlint_bin: pathlib.Path | None = None
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


def _check_pyrefly_canary(
    uv_bin: str,
    project_dir: pathlib.Path,
    canary_dir: pathlib.Path,
) -> bool:
    pyrefly_dir = canary_dir / "pyrefly"
    pyrefly_dir.mkdir(parents=True, exist_ok=True)
    canary_type_py = pyrefly_dir / "canary_type.py"
    canary_type_py.write_text(PY_PYREFLY_CANARY_SOURCE, encoding="utf-8")

    pyproject_path = project_dir / "pyproject.toml"
    config_path = pyrefly_dir / "pyrefly.toml"
    use_project_config = False
    try:
        pyproject_data = tomllib.loads(pyproject_path.read_text(encoding="utf-8"))
        tool_cfg = pyproject_data.get("tool")
        if isinstance(tool_cfg, dict) and isinstance(tool_cfg.get("pyrefly"), dict):
            use_project_config = True
    except (OSError, tomllib.TOMLDecodeError):
        use_project_config = False

    if use_project_config:
        config_path = pyproject_path
    else:
        config_path.write_text('project-includes = ["canary_type.py"]\n', encoding="utf-8")

    cmd = [
        uv_bin,
        "run",
        "--frozen",
        "--only-group",
        "dev",
        "pyrefly",
        "check",
        "--config",
        str(config_path),
        str(canary_type_py),
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
        _error(f"ERROR: CANARY AUDIT FAILED (python/pyrefly): execution error: {exc}")
        return False

    combined = f"{res.stdout}\n{res.stderr}"
    lowered = combined.lower()
    detected = any(marker in lowered for marker in PYREFLY_CANARY_MARKERS)
    if res.returncode == 0 or not detected:
        _error(
            "ERROR: CANARY AUDIT FAILED (python/pyrefly): "
            "expected non-zero exit with type-error diagnostic, "
            f"got exit {res.returncode} (output: {_snippet(combined)})"
        )
        return False
    return True


def run_python_canary(cfg: PythonCanaryConfig) -> int:
    """Execute isolated ruff (and optional bandit/pyrefly) canary self-audit."""
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

        if cfg.check_pyrefly and not _check_pyrefly_canary(uv_bin, cfg.project_dir, canary_dir):
            return EXIT_FAIL

    extras: list[str] = []
    if cfg.check_bandit:
        extras.append("bandit (B602/B307)")
    if cfg.check_pyrefly:
        extras.append("pyrefly (bad-assignment)")
    extra_note = f", {', '.join(extras)}" if extras else ""
    _info(f"canary audit passed (python): ruff ({RUFF_CANARY_EXPECTED_RULE}){extra_note}")
    return EXIT_OK


def _parse_json_stream(raw_text: str) -> list[dict[str, object]] | None:
    """Parse concatenated JSON objects emitted by `govulncheck -format=json`."""
    decoder = json.JSONDecoder()
    objs: list[dict[str, object]] = []
    idx = 0
    length = len(raw_text)
    while idx < length:
        while idx < length and raw_text[idx].isspace():
            idx += 1
        if idx >= length:
            break
        try:
            obj, end = decoder.raw_decode(raw_text, idx)
        except json.JSONDecodeError:
            return None
        if isinstance(obj, dict):
            objs.append(obj)
        idx = end
    return objs


def _govulncheck_sarif_error(
    sarif_path: pathlib.Path,
    *,
    require_properties: bool,
) -> str | None:
    if not sarif_path.is_file():
        return f"missing SARIF at {sarif_path}"
    try:
        sarif_data = json.loads(sarif_path.read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError) as exc:
        return f"invalid SARIF JSON: {exc}"

    runs = sarif_data.get("runs") if isinstance(sarif_data, dict) else None
    if not isinstance(runs, list) or not runs:
        return f"SARIF {sarif_path} has missing or empty runs"

    first_run = runs[0]
    tool = first_run.get("tool") if isinstance(first_run, dict) else None
    driver = tool.get("driver") if isinstance(tool, dict) else None
    if not isinstance(driver, dict) or driver.get("name") != "govulncheck":
        return f"SARIF {sarif_path} missing tool.driver.name == 'govulncheck'"
    if not isinstance(driver.get("rules"), list):
        return f"SARIF {sarif_path} missing tool.driver.rules list"

    return _govulncheck_sarif_props_error(
        driver.get("properties"),
        sarif_path,
        require_properties=require_properties,
    )


def _govulncheck_sarif_props_error(
    props: object,
    sarif_path: pathlib.Path,
    *,
    require_properties: bool,
) -> str | None:
    if not isinstance(props, dict):
        if require_properties:
            return f"SARIF {sarif_path} missing tool.driver.properties metadata"
        return None
    for key in ("db", "go_version"):
        if key in props and not str(props.get(key, "")).strip():
            return f"SARIF {sarif_path} has empty tool.driver.properties.{key}"
    if require_properties and (
        not str(props.get("db", "")).strip() or not str(props.get("go_version", "")).strip()
    ):
        return f"SARIF {sarif_path} missing db or go_version in tool.driver.properties"
    return None


def _validate_govulncheck_sarif(
    sarif_path: pathlib.Path,
    *,
    require_properties: bool,
) -> bool:
    err = _govulncheck_sarif_error(sarif_path, require_properties=require_properties)
    if err is not None:
        _error(f"ERROR: CANARY AUDIT FAILED (go/govulncheck): {err}")
        return False
    return True


def _extract_govulncheck_json_sections(
    govulncheck_json_path: pathlib.Path,
) -> tuple[dict[str, object] | None, dict[str, object] | None, str | None]:
    if not govulncheck_json_path.is_file():
        return None, None, f"missing JSON stream output at {govulncheck_json_path}"
    objs = _parse_json_stream(govulncheck_json_path.read_text(encoding="utf-8", errors="replace"))
    if not objs:
        return None, None, f"invalid or empty JSON stream at {govulncheck_json_path}"

    config_obj: dict[str, object] | None = None
    sbom_obj: dict[str, object] | None = None
    for obj in objs:
        cfg_val = obj.get("config")
        if isinstance(cfg_val, dict):
            config_obj = cfg_val
        sbom_val = obj.get("SBOM")
        if isinstance(sbom_val, dict):
            sbom_obj = sbom_val
    return config_obj, sbom_obj, None


def _verify_govulncheck_sbom(
    project_dir: pathlib.Path,
    sbom_obj: dict[str, object] | None,
    non_gen_count: int,
) -> tuple[int, int] | None:
    if sbom_obj is None:
        _error(
            "ERROR: ZERO-FILE SCAN DETECTED (go/govulncheck): "
            f"missing SBOM object despite {non_gen_count} non-generated .go files in {project_dir}"
        )
        return None
    modules = sbom_obj.get("modules")
    roots = sbom_obj.get("roots")
    packages = sbom_obj.get("packages")
    mod_count = len(modules) if isinstance(modules, list) else 0
    pkg_count = (
        len(roots)
        if isinstance(roots, list) and roots
        else (len(packages) if isinstance(packages, list) else 0)
    )
    if mod_count == 0 or pkg_count == 0:
        _error(
            "ERROR: ZERO-FILE SCAN DETECTED (go/govulncheck): "
            f"govulncheck SBOM scanned {mod_count} module(s) and {pkg_count} package(s) "
            f"despite {non_gen_count} non-generated .go files in {project_dir}"
        )
        return None
    return mod_count, pkg_count


def _verify_govulncheck_sarif_only(project_dir: pathlib.Path, non_gen_count: int) -> int:
    if non_gen_count == 0:
        _info(
            f"govulncheck scan check passed: valid SARIF metadata "
            f"(0 non-generated .go files in {project_dir})"
        )
        return EXIT_OK
    pkg_count = _verify_go_packages_loaded(project_dir, non_gen_count)
    if pkg_count is None:
        return EXIT_FAIL
    _info(
        f"govulncheck scan check passed: valid SARIF DB metadata, {pkg_count} package(s) resolved"
    )
    return EXIT_OK


def _verify_govulncheck_json_stream(
    project_dir: pathlib.Path,
    govulncheck_json_path: pathlib.Path,
    non_gen_count: int,
) -> int:
    config_obj, sbom_obj, stream_err = _extract_govulncheck_json_sections(govulncheck_json_path)
    if stream_err is not None or config_obj is None:
        detail = stream_err or "JSON output missing 'config' vulnerability DB metadata"
        _error(f"ERROR: CANARY AUDIT FAILED (go/govulncheck): {detail}")
        return EXIT_FAIL

    db_val = str(config_obj.get("db", "")).strip()
    go_ver_val = str(config_obj.get("go_version", "")).strip()
    if not db_val or not go_ver_val:
        _error(
            "ERROR: CANARY AUDIT FAILED (go/govulncheck): "
            f"config missing required db/go_version metadata (db={db_val!r}, go_version={go_ver_val!r})"
        )
        return EXIT_FAIL

    if non_gen_count == 0:
        _info(
            f"govulncheck scan check passed: db={db_val} ({go_ver_val}), "
            f"0 non-generated .go files in {project_dir}"
        )
        return EXIT_OK

    counts = _verify_govulncheck_sbom(project_dir, sbom_obj, non_gen_count)
    if counts is None:
        return EXIT_FAIL
    mod_count, pkg_count = counts
    _info(
        f"govulncheck scan check passed: db={db_val} ({go_ver_val}), "
        f"{mod_count} module(s), {pkg_count} root package(s)"
    )
    return EXIT_OK


def verify_go_govulncheck(
    project_dir: pathlib.Path,
    govulncheck_sarif_path: pathlib.Path,
    govulncheck_json_path: pathlib.Path | None = None,
) -> int:
    """Verify govulncheck vulnerability DB metadata and SBOM package/module coverage."""
    if not _validate_govulncheck_sarif(
        govulncheck_sarif_path,
        require_properties=govulncheck_json_path is None,
    ):
        return EXIT_FAIL

    non_gen_count = count_non_generated_go_files(project_dir)
    if govulncheck_json_path is None:
        return _verify_govulncheck_sarif_only(project_dir, non_gen_count)
    return _verify_govulncheck_json_stream(project_dir, govulncheck_json_path, non_gen_count)


def _find_rust_toolchain_file(project_dir: pathlib.Path) -> pathlib.Path | None:
    current = project_dir.resolve()
    while True:
        candidate = current / "rust-toolchain.toml"
        if candidate.is_file():
            return candidate
        if current.parent == current:
            return None
        current = current.parent


def _extract_rust_channel(toolchain_file: pathlib.Path) -> str | None:
    for raw_line in toolchain_file.read_text(encoding="utf-8").splitlines():
        match = RUST_CHANNEL_RE.match(raw_line)
        if match is not None:
            return match.group(1)
    return None


def run_rust_canary(cfg: RustCanaryConfig) -> int:
    """Execute isolated cargo clippy canary self-audit with zero external deps."""
    toolchain_file = (
        cfg.toolchain_file
        if cfg.toolchain_file is not None
        else _find_rust_toolchain_file(cfg.project_dir)
    )
    if toolchain_file is None or not toolchain_file.is_file():
        _error(
            "ERROR: CANARY AUDIT FAILED (rust/clippy): "
            f"rust-toolchain.toml not found for {cfg.project_dir}"
        )
        return EXIT_FAIL

    channel = _extract_rust_channel(toolchain_file)
    if not channel:
        _error(f"ERROR: CANARY AUDIT FAILED (rust/clippy): missing channel in {toolchain_file}")
        return EXIT_FAIL

    cargo_bin_str = str(cfg.cargo_bin) if cfg.cargo_bin is not None else shutil.which("cargo")
    if cargo_bin_str is None:
        _error("ERROR: CANARY AUDIT FAILED (rust/clippy): cargo executable not found in PATH")
        return EXIT_FAIL

    cache_root = pathlib.Path(os.environ.get("XDG_CACHE_HOME", pathlib.Path.home() / ".cache"))
    target_dir = (
        cfg.target_dir
        if cfg.target_dir is not None
        else cache_root / "alt-lint" / "rust-canary-target"
    )
    target_dir.mkdir(parents=True, exist_ok=True)

    env = os.environ.copy()
    env["CARGO_TARGET_DIR"] = str(target_dir)

    with _work_dir_context(cfg.work_dir, prefix="alt-rust-canary-") as canary_dir:
        (canary_dir / "src").mkdir(parents=True, exist_ok=True)
        (canary_dir / "rust-toolchain.toml").write_text(
            f'[toolchain]\nchannel = "{channel}"\ncomponents = ["clippy"]\n',
            encoding="utf-8",
        )
        (canary_dir / "Cargo.toml").write_text(RUST_CANARY_CARGO_TOML, encoding="utf-8")
        (canary_dir / "src" / "lib.rs").write_text(RUST_CANARY_LIB_RS, encoding="utf-8")

        cmd = [cargo_bin_str, "clippy", "--offline", "--", "-D", "warnings"]
        try:
            res = subprocess.run(  # noqa: S603 - resolved cargo binary with fixed argv
                cmd,
                cwd=str(canary_dir),
                env=env,
                capture_output=True,
                text=True,
                check=False,
            )
        except OSError as exc:
            _error(f"ERROR: CANARY AUDIT FAILED (rust/clippy): execution error: {exc}")
            return EXIT_FAIL

    combined = f"{res.stdout}\n{res.stderr}"
    lowered = combined.lower()
    if res.returncode == 0 or not any(marker in lowered for marker in RUST_CANARY_MARKERS):
        _error(
            "ERROR: CANARY AUDIT FAILED (rust/clippy): "
            "expected non-zero exit with clippy::eq_op diagnostic, "
            f"got exit {res.returncode} (output: {_snippet(combined)})"
        )
        return EXIT_FAIL

    _info(f"canary audit passed (rust): cargo clippy ({channel}, clippy::eq_op)")
    return EXIT_OK


def run_frontend_canary(cfg: FrontendCanaryConfig) -> int:
    """Execute isolated Biome lint canary self-audit."""
    biome_bin = (
        cfg.biome_bin
        if cfg.biome_bin is not None
        else cfg.project_dir / "node_modules" / ".bin" / "biome"
    )
    if not biome_bin.is_file():
        _error(
            f"ERROR: CANARY AUDIT FAILED (frontend/biome): biome binary not found at {biome_bin}"
        )
        return EXIT_FAIL

    with _work_dir_context(cfg.work_dir, prefix="alt-frontend-canary-") as canary_dir:
        (canary_dir / "biome.json").write_text(FRONTEND_CANARY_BIOME_JSON, encoding="utf-8")
        canary_ts = canary_dir / "canary.ts"
        canary_ts.write_text(TS_DEBUGGER_CANARY_SOURCE, encoding="utf-8")

        cmd = [str(biome_bin), "lint", "--config-path", str(canary_dir), str(canary_ts)]
        try:
            res = subprocess.run(  # noqa: S603 - resolved project biome binary with fixed argv
                cmd,
                cwd=str(canary_dir),
                capture_output=True,
                text=True,
                check=False,
            )
        except OSError as exc:
            _error(f"ERROR: CANARY AUDIT FAILED (frontend/biome): execution error: {exc}")
            return EXIT_FAIL

    combined = f"{res.stdout}\n{res.stderr}"
    lowered = combined.lower()
    if res.returncode == 0 or not any(marker in lowered for marker in FRONTEND_CANARY_MARKERS):
        _error(
            "ERROR: CANARY AUDIT FAILED (frontend/biome): "
            "expected non-zero exit with noDebugger diagnostic, "
            f"got exit {res.returncode} (output: {_snippet(combined)})"
        )
        return EXIT_FAIL

    _info("canary audit passed (frontend): biome lint (noDebugger)")
    return EXIT_OK


def run_deno_canary(cfg: DenoCanaryConfig) -> int:
    """Execute isolated deno lint canary self-audit."""
    deno_bin_str = str(cfg.deno_bin) if cfg.deno_bin is not None else shutil.which("deno")
    if deno_bin_str is None:
        _error("ERROR: CANARY AUDIT FAILED (deno/lint): deno executable not found in PATH")
        return EXIT_FAIL

    with _work_dir_context(cfg.work_dir, prefix="alt-deno-canary-") as canary_dir:
        canary_ts = canary_dir / "canary.ts"
        canary_ts.write_text(TS_DEBUGGER_CANARY_SOURCE, encoding="utf-8")

        cmd = [deno_bin_str, "lint", str(canary_ts)]
        try:
            res = subprocess.run(  # noqa: S603 - resolved deno binary with fixed argv
                cmd,
                cwd=str(canary_dir),
                capture_output=True,
                text=True,
                check=False,
            )
        except OSError as exc:
            _error(f"ERROR: CANARY AUDIT FAILED (deno/lint): execution error: {exc}")
            return EXIT_FAIL

    combined = f"{res.stdout}\n{res.stderr}"
    if res.returncode == 0 or DENO_CANARY_EXPECTED_RULE not in combined:
        _error(
            "ERROR: CANARY AUDIT FAILED (deno/lint): "
            f"expected non-zero exit with {DENO_CANARY_EXPECTED_RULE} diagnostic, "
            f"got exit {res.returncode} (output: {_snippet(combined)})"
        )
        return EXIT_FAIL

    _info(f"canary audit passed (deno): deno lint ({DENO_CANARY_EXPECTED_RULE})")
    return EXIT_OK


def run_proto_canary(cfg: ProtoCanaryConfig) -> int:
    """Execute isolated buf lint canary self-audit."""
    buf_bin_str = str(cfg.buf_bin) if cfg.buf_bin is not None else shutil.which("buf")
    if buf_bin_str is None:
        _error("ERROR: CANARY AUDIT FAILED (proto/buf): buf executable not found in PATH")
        return EXIT_FAIL

    with _work_dir_context(cfg.work_dir, prefix="alt-proto-canary-") as canary_dir:
        (canary_dir / "buf.yaml").write_text(PROTO_CANARY_BUF_YAML, encoding="utf-8")
        (canary_dir / "canary.proto").write_text(PROTO_CANARY_SOURCE, encoding="utf-8")

        cmd = [buf_bin_str, "lint"]
        try:
            res = subprocess.run(  # noqa: S603 - resolved buf binary with fixed argv
                cmd,
                cwd=str(canary_dir),
                capture_output=True,
                text=True,
                check=False,
            )
        except OSError as exc:
            _error(f"ERROR: CANARY AUDIT FAILED (proto/buf): execution error: {exc}")
            return EXIT_FAIL

    combined = f"{res.stdout}\n{res.stderr}"
    lowered = combined.lower()
    if res.returncode == 0 or not any(marker in lowered for marker in PROTO_CANARY_MARKERS):
        _error(
            "ERROR: CANARY AUDIT FAILED (proto/buf): "
            "expected non-zero exit with buf STANDARD lint diagnostic, "
            f"got exit {res.returncode} (output: {_snippet(combined)})"
        )
        return EXIT_FAIL

    _info("canary audit passed (proto): buf lint (STANDARD)")
    return EXIT_OK


def run_actionlint_canary(cfg: ActionlintCanaryConfig) -> int:
    """Execute isolated actionlint canary self-audit."""
    cmd_prefix: list[str]
    if cfg.actionlint_bin is not None:
        cmd_prefix = [str(cfg.actionlint_bin)]
    else:
        found = shutil.which("actionlint")
        if found is not None:
            cmd_prefix = [found]
        else:
            go_bin = shutil.which("go")
            if go_bin is None:
                _error(
                    "ERROR: CANARY AUDIT FAILED (repo/actionlint): "
                    "neither actionlint nor go found in PATH"
                )
                return EXIT_FAIL
            cmd_prefix = [go_bin, "run", ACTIONLINT_GO_MODULE]

    with _work_dir_context(cfg.work_dir, prefix="alt-actionlint-canary-") as canary_dir:
        canary_yml = canary_dir / "canary.yml"
        canary_yml.write_text(ACTIONLINT_CANARY_WORKFLOW, encoding="utf-8")

        cmd = [*cmd_prefix, "-shellcheck=", str(canary_yml)]
        try:
            res = subprocess.run(  # noqa: S603 - resolved actionlint/go binary with fixed argv
                cmd,
                capture_output=True,
                text=True,
                check=False,
            )
        except OSError as exc:
            _error(f"ERROR: CANARY AUDIT FAILED (repo/actionlint): execution error: {exc}")
            return EXIT_FAIL

    combined = f"{res.stdout}\n{res.stderr}"
    lowered = combined.lower()
    if res.returncode == 0 or not any(marker in lowered for marker in ACTIONLINT_CANARY_MARKERS):
        _error(
            "ERROR: CANARY AUDIT FAILED (repo/actionlint): "
            "expected non-zero exit with expression diagnostic, "
            f"got exit {res.returncode} (output: {_snippet(combined)})"
        )
        return EXIT_FAIL

    _info("canary audit passed (repo): actionlint (expression)")
    return EXIT_OK


def _read_python_dev_deps(pyproject_path: pathlib.Path) -> set[str]:
    data = tomllib.loads(pyproject_path.read_text(encoding="utf-8"))
    dep_groups = data.get("dependency-groups")
    dev_entries = dep_groups.get("dev", []) if isinstance(dep_groups, dict) else []
    names: set[str] = set()
    if isinstance(dev_entries, list):
        for entry in dev_entries:
            if not isinstance(entry, str):
                continue
            raw_name = DEP_NAME_SPLIT_RE.split(entry.strip(), maxsplit=1)[0]
            if raw_name:
                names.add(DEP_NORM_RE.sub("-", raw_name).lower())
    return names


def _validate_single_scanner_entry(
    target: str,
    scanner_name: str,
    cfg: object,
) -> str | None:
    if not isinstance(cfg, dict):
        return f"target {target!r} scanner {scanner_name!r}: entry must be a table"
    status = cfg.get("status")
    if status not in VALID_SCANNER_STATUSES:
        return (
            f"target {target!r} scanner {scanner_name!r}: "
            f"invalid status {status!r} (expected one of {sorted(VALID_SCANNER_STATUSES)})"
        )
    if status == "canary":
        canary_cmd = cfg.get("canary")
        if not isinstance(canary_cmd, str) or canary_cmd not in VALID_CANARY_COMMANDS:
            return (
                f"target {target!r} scanner {scanner_name!r}: "
                f"status='canary' requires valid canary command (got {canary_cmd!r})"
            )
        return None
    reason = cfg.get("reason")
    if not isinstance(reason, str) or len(reason.strip()) < MIN_EXEMPT_REASON_LEN:
        return (
            f"target {target!r} scanner {scanner_name!r}: "
            f"status={status!r} requires non-empty reason (>={MIN_EXEMPT_REASON_LEN} chars)"
        )
    return None


_GIT_SECTION_RE = re.compile(r"^\s*\[([^\]]+)\]\s*(?:[#;].*)?$")
_GIT_KV_RE = re.compile(r"^\s*([A-Za-z0-9-]+)(?:\s*=\s*(.*?))?\s*(?:[#;].*)?$")


def _git_config_has_sparse_checkout(cfg_path: pathlib.Path) -> bool:
    try:
        lines = cfg_path.read_text(encoding="utf-8", errors="replace").splitlines()
    except OSError:
        return False
    in_core = False
    for raw_line in lines:
        stripped = raw_line.strip()
        if not stripped or stripped.startswith(("#", ";")):
            continue
        sec_match = _GIT_SECTION_RE.match(raw_line)
        if sec_match is not None:
            in_core = sec_match.group(1).strip().lower() == "core"
            continue
        if not in_core:
            continue
        kv_match = _GIT_KV_RE.match(raw_line)
        if kv_match is None or kv_match.group(1).lower() != "sparsecheckout":
            continue
        val = kv_match.group(2)
        if val is None or val.strip().strip('"').lower() in {"true", "1", "yes", "on"}:
            return True
    return False


def is_git_sparse_checkout(repo_root: pathlib.Path) -> bool:
    """Return True only when git config explicitly enables core.sparseCheckout."""
    dot_git = repo_root / ".git"
    git_dir: pathlib.Path | None = None
    common_dir: pathlib.Path | None = None
    if dot_git.is_dir():
        git_dir = dot_git
        common_dir = dot_git
    elif dot_git.is_file():
        try:
            first = dot_git.read_text(encoding="utf-8", errors="replace").splitlines()[0].strip()
        except (OSError, IndexError):
            return False
        if not first.startswith("gitdir:"):
            return False
        git_dir = (repo_root / first.split(":", 1)[1].strip()).resolve()
        common_dir = git_dir
        commondir_file = git_dir / "commondir"
        if commondir_file.is_file():
            try:
                rel = commondir_file.read_text(encoding="utf-8", errors="replace").strip()
                if rel:
                    common_dir = (git_dir / rel).resolve()
            except OSError:
                pass
    if git_dir is None or common_dir is None:
        return False
    candidates = (git_dir / "config.worktree", git_dir / "config", common_dir / "config")
    return any(_git_config_has_sparse_checkout(p) for p in candidates if p.is_file())


def _validate_python_ledger_scanners(
    target: str,
    scanners: dict[str, object],
    repo_root: pathlib.Path,
    *,
    require_pyproject: bool = True,
) -> list[str]:
    errors = [
        f"target {target!r} (python): missing explicit scanner entry for {optional_tool!r}"
        for optional_tool in ("bandit", "pyrefly")
        if not isinstance(scanners.get(optional_tool), dict)
    ]
    if errors:
        return errors

    pyproject_path = repo_root / target / "pyproject.toml"
    if not pyproject_path.is_file():
        if require_pyproject:
            return [f"target {target!r}: missing pyproject.toml at {pyproject_path}"]
        return []

    dev_deps = _read_python_dev_deps(pyproject_path)
    for optional_tool in ("bandit", "pyrefly"):
        entry = scanners.get(optional_tool)
        if not isinstance(entry, dict):
            continue
        declared = optional_tool in dev_deps
        expected_status = "canary" if declared else "not-applicable"
        actual_status = entry.get("status")
        if actual_status != expected_status:
            errors.append(
                f"target {target!r} (python): pyproject.toml "
                f"{'declares' if declared else 'does not declare'} {optional_tool!r}, "
                f"so ledger status must be {expected_status!r} (got {actual_status!r})"
            )
    return errors


def _validate_ledger_scanners(
    target: str,
    kind: str,
    scanners: object,
    repo_root: pathlib.Path,
    *,
    require_pyproject: bool = True,
) -> list[str]:
    if not isinstance(scanners, dict) or not scanners:
        return [f"target {target!r}: missing or empty 'scanners' table"]

    errors: list[str] = []
    for scanner_name, cfg in scanners.items():
        err = _validate_single_scanner_entry(target, str(scanner_name), cfg)
        if err is not None:
            errors.append(err)

    for required_scanner in REQUIRED_CANARY_SCANNERS_BY_KIND.get(kind, ()):
        entry = scanners.get(required_scanner)
        if not isinstance(entry, dict) or entry.get("status") != "canary":
            errors.append(
                f"target {target!r} ({kind}): required scanner {required_scanner!r} "
                "must have status = 'canary'"
            )

    if kind == "python":
        errors.extend(
            _validate_python_ledger_scanners(
                target,
                scanners,
                repo_root,
                require_pyproject=require_pyproject,
            )
        )
    return errors


def _list_repo_lint_targets(repo_root: pathlib.Path) -> tuple[list[str] | None, str | None]:
    run_sh = repo_root / "scripts" / "lint" / "run.sh"
    if not run_sh.is_file():
        return None, f"missing {run_sh}"
    bash_bin = shutil.which("bash")
    if bash_bin is None:
        return None, "bash executable not found in PATH"
    res = subprocess.run(  # noqa: S603 - fixed argv querying repo lint targets
        [bash_bin, str(run_sh), "--list"],
        cwd=str(repo_root),
        capture_output=True,
        text=True,
        check=False,
    )
    if res.returncode != 0:
        return None, f"run.sh --list failed (exit {res.returncode}): {_snippet(res.stderr)}"
    return sorted({line.strip() for line in res.stdout.splitlines() if line.strip()}), None


def _load_ledger_targets(ledger_path: pathlib.Path) -> tuple[dict[str, object] | None, str | None]:
    if not ledger_path.is_file():
        return None, f"missing ledger file {ledger_path}"
    try:
        ledger_data = tomllib.loads(ledger_path.read_text(encoding="utf-8"))
    except (OSError, tomllib.TOMLDecodeError) as exc:
        return None, f"invalid TOML in {ledger_path}: {exc}"
    targets_table = ledger_data.get("targets")
    if not isinstance(targets_table, dict):
        return None, f"missing [targets] table in {ledger_path}"
    return targets_table, None


def _tally_scanner_statuses(scanners: object) -> tuple[int, int, int]:
    if not isinstance(scanners, dict):
        return (0, 0, 0)
    canary = exempt = na = 0
    for cfg in scanners.values():
        if not isinstance(cfg, dict):
            continue
        status = cfg.get("status")
        if status == "canary":
            canary += 1
        elif status == "exempt":
            exempt += 1
        elif status == "not-applicable":
            na += 1
    return canary, exempt, na


def _validate_ledger_entries(
    targets_to_check: list[str],
    listed_set: set[str],
    targets_table: dict[str, object],
    repo_root: pathlib.Path,
    *,
    sparse_active: bool,
) -> tuple[list[str], int, int, int]:
    errors: list[str] = []
    canary_count = exempt_count = na_count = 0
    for target in targets_to_check:
        entry = targets_table.get(target)
        if not isinstance(entry, dict):
            continue
        kind = entry.get("kind")
        if not isinstance(kind, str) or kind not in VALID_LEDGER_KINDS:
            errors.append(
                f"target {target!r}: invalid kind {kind!r} "
                f"(expected one of {sorted(VALID_LEDGER_KINDS)})"
            )
            continue
        scanners = entry.get("scanners")
        require_pyproject = (target in listed_set) or not sparse_active
        errors.extend(
            _validate_ledger_scanners(
                target,
                kind,
                scanners,
                repo_root,
                require_pyproject=require_pyproject,
            )
        )
        c_cnt, e_cnt, n_cnt = _tally_scanner_statuses(scanners)
        canary_count += c_cnt
        exempt_count += e_cnt
        na_count += n_cnt
    return errors, canary_count, exempt_count, na_count


def verify_canary_ledger(
    repo_root: pathlib.Path,
    ledger_path: pathlib.Path | None = None,
    *,
    allow_sparse: bool = False,
) -> int:
    """Verify canary_ledger.toml covers every target from ./scripts/lint/run.sh --list."""
    resolved_ledger = (
        ledger_path
        if ledger_path is not None
        else repo_root / "scripts" / "lint" / "canary_ledger.toml"
    )
    listed_targets, list_err = _list_repo_lint_targets(repo_root)
    if list_err is not None or listed_targets is None:
        _error(f"ERROR: LEDGER CHECK FAILED: {list_err}")
        return EXIT_FAIL

    targets_table, load_err = _load_ledger_targets(resolved_ledger)
    if load_err is not None or targets_table is None:
        _error(f"ERROR: LEDGER CHECK FAILED: {load_err}")
        return EXIT_FAIL

    is_sparse = is_git_sparse_checkout(repo_root)
    sparse_active = allow_sparse and is_sparse
    listed_set = set(listed_targets)
    ledger_set = set(targets_table.keys())

    errors: list[str] = []
    missing = sorted(listed_set - ledger_set)
    extra = sorted(ledger_set - listed_set)
    if missing:
        errors.append(f"targets in run.sh --list missing from ledger: {missing}")
    if extra and not sparse_active:
        hint = (
            " (git core.sparseCheckout=true; run verify-ledger on a full checkout "
            "or pass --allow-sparse for checked-out subset validation)"
            if is_sparse
            else ""
        )
        errors.append(f"stale targets in ledger absent from run.sh --list: {extra}{hint}")

    targets_to_check = sorted(ledger_set) if sparse_active else listed_targets
    entry_errors, canary_count, exempt_count, na_count = _validate_ledger_entries(
        targets_to_check,
        listed_set,
        targets_table,
        repo_root,
        sparse_active=sparse_active,
    )
    errors.extend(entry_errors)

    if errors:
        for err in errors:
            _error(f"ERROR: LEDGER CHECK FAILED: {err}")
        return EXIT_FAIL

    sparse_note = f"sparse materialized={len(listed_targets)}, " if sparse_active else ""
    _info(
        f"canary ledger verified: {len(targets_to_check)} targets accounted for "
        f"({sparse_note}canary={canary_count}, exempt={exempt_count}, not-applicable={na_count})"
    )
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
        description="Isolated canary self-audit, scan coverage guard, and ledger verifier.",
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

    go_vuln_p = subparsers.add_parser("go-verify-govulncheck")
    go_vuln_p.add_argument("--project-dir", type=pathlib.Path, required=True)
    go_vuln_p.add_argument("--govulncheck-sarif", type=pathlib.Path, required=True)
    go_vuln_p.add_argument("--govulncheck-json", type=pathlib.Path, default=None)

    py_canary_p = subparsers.add_parser("python-canary")
    py_canary_p.add_argument("--project-dir", type=pathlib.Path, required=True)
    py_canary_p.add_argument("--check-bandit", type=_parse_bool_flag, default=False)
    py_canary_p.add_argument("--check-pyrefly", type=_parse_bool_flag, default=False)
    py_canary_p.add_argument("--work-dir", type=pathlib.Path, default=None)

    py_files_p = subparsers.add_parser("python-verify-files")
    py_files_p.add_argument("--project-dir", type=pathlib.Path, required=True)

    py_bandit_p = subparsers.add_parser("python-verify-bandit-scan")
    py_bandit_p.add_argument("--project-dir", type=pathlib.Path, required=True)
    py_bandit_p.add_argument("--bandit-out", type=pathlib.Path, required=True)

    rust_canary_p = subparsers.add_parser("rust-canary")
    rust_canary_p.add_argument("--project-dir", type=pathlib.Path, required=True)
    rust_canary_p.add_argument("--toolchain-file", type=pathlib.Path, default=None)
    rust_canary_p.add_argument("--cargo-bin", type=pathlib.Path, default=None)
    rust_canary_p.add_argument("--target-dir", type=pathlib.Path, default=None)
    rust_canary_p.add_argument("--work-dir", type=pathlib.Path, default=None)

    fe_canary_p = subparsers.add_parser("frontend-canary")
    fe_canary_p.add_argument("--project-dir", type=pathlib.Path, required=True)
    fe_canary_p.add_argument("--biome-bin", type=pathlib.Path, default=None)
    fe_canary_p.add_argument("--work-dir", type=pathlib.Path, default=None)

    deno_canary_p = subparsers.add_parser("deno-canary")
    deno_canary_p.add_argument("--project-dir", type=pathlib.Path, required=True)
    deno_canary_p.add_argument("--deno-bin", type=pathlib.Path, default=None)
    deno_canary_p.add_argument("--work-dir", type=pathlib.Path, default=None)

    proto_canary_p = subparsers.add_parser("proto-canary")
    proto_canary_p.add_argument("--buf-bin", type=pathlib.Path, default=None)
    proto_canary_p.add_argument("--work-dir", type=pathlib.Path, default=None)

    al_canary_p = subparsers.add_parser("actionlint-canary")
    al_canary_p.add_argument("--actionlint-bin", type=pathlib.Path, default=None)
    al_canary_p.add_argument("--work-dir", type=pathlib.Path, default=None)

    ledger_p = subparsers.add_parser("verify-ledger")
    ledger_p.add_argument("--repo-root", type=pathlib.Path, required=True)
    ledger_p.add_argument("--ledger", type=pathlib.Path, default=None)
    ledger_p.add_argument("--allow-sparse", action="store_true", default=False)

    return parser


def _resolve_opt(path: pathlib.Path | None) -> pathlib.Path | None:
    return path.resolve() if path is not None else None


def _dispatch_command(args: argparse.Namespace) -> int:
    handlers = {
        "go-canary": lambda a: run_go_canary(
            GoCanaryConfig(
                project_dir=a.project_dir.resolve(),
                config_file=a.config.resolve(),
                golangci_lint_bin=a.golangci_lint_bin.resolve(),
                gosec_bin=_resolve_opt(a.gosec_bin),
                work_dir=_resolve_opt(a.work_dir),
            )
        ),
        "go-verify-scan": lambda a: verify_go_scan(
            project_dir=a.project_dir.resolve(),
            gosec_out_path=_resolve_opt(a.gosec_out),
            gosec_sarif_path=a.gosec_sarif.resolve(),
        ),
        "go-verify-govulncheck": lambda a: verify_go_govulncheck(
            project_dir=a.project_dir.resolve(),
            govulncheck_sarif_path=a.govulncheck_sarif.resolve(),
            govulncheck_json_path=_resolve_opt(a.govulncheck_json),
        ),
        "python-canary": lambda a: run_python_canary(
            PythonCanaryConfig(
                project_dir=a.project_dir.resolve(),
                check_bandit=a.check_bandit,
                check_pyrefly=a.check_pyrefly,
                work_dir=_resolve_opt(a.work_dir),
            )
        ),
        "python-verify-files": lambda a: verify_python_files(a.project_dir.resolve()),
        "python-verify-bandit-scan": lambda a: verify_python_bandit_scan(
            project_dir=a.project_dir.resolve(),
            bandit_out_path=a.bandit_out.resolve(),
        ),
        "rust-canary": lambda a: run_rust_canary(
            RustCanaryConfig(
                project_dir=a.project_dir.resolve(),
                toolchain_file=_resolve_opt(a.toolchain_file),
                cargo_bin=_resolve_opt(a.cargo_bin),
                target_dir=_resolve_opt(a.target_dir),
                work_dir=_resolve_opt(a.work_dir),
            )
        ),
        "frontend-canary": lambda a: run_frontend_canary(
            FrontendCanaryConfig(
                project_dir=a.project_dir.resolve(),
                biome_bin=_resolve_opt(a.biome_bin),
                work_dir=_resolve_opt(a.work_dir),
            )
        ),
        "deno-canary": lambda a: run_deno_canary(
            DenoCanaryConfig(
                project_dir=a.project_dir.resolve(),
                deno_bin=_resolve_opt(a.deno_bin),
                work_dir=_resolve_opt(a.work_dir),
            )
        ),
        "proto-canary": lambda a: run_proto_canary(
            ProtoCanaryConfig(
                buf_bin=_resolve_opt(a.buf_bin),
                work_dir=_resolve_opt(a.work_dir),
            )
        ),
        "actionlint-canary": lambda a: run_actionlint_canary(
            ActionlintCanaryConfig(
                actionlint_bin=_resolve_opt(a.actionlint_bin),
                work_dir=_resolve_opt(a.work_dir),
            )
        ),
        "verify-ledger": lambda a: verify_canary_ledger(
            repo_root=a.repo_root.resolve(),
            ledger_path=_resolve_opt(a.ledger),
            allow_sparse=bool(a.allow_sparse),
        ),
    }
    handler = handlers.get(args.command)
    if handler is None:
        _error(f"ERROR: unknown command: {args.command}")
        return EXIT_USAGE
    return handler(args)


def main(argv: list[str] | None = None) -> int:
    parser = build_parser()
    args = parser.parse_args(argv)
    return _dispatch_command(args)


if __name__ == "__main__":
    _launcher = (
        pathlib.Path(__file__).resolve().parents[2] / "scripts" / "altharness" / "alt-harness"
    )
    if _launcher.is_file():
        os.execv(_launcher, [str(_launcher), "canary", *sys.argv[1:]])  # noqa: S606
    sys.exit(main())
