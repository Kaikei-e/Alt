#!/usr/bin/env python3
"""CORPUS-01 failure corpus validator, check runner, and capture-rate reporter.

Validates every failure case under `scripts/harness/corpus/<ID>/` (`C-01`..`C-NN`),
executes active implemented checks (`pyrefly`, `REPO-02`, `REPO-03`, `ARCH-02`,
`ARCH-03`, `ARCH-06`) against each case's `before` (must pass) and `after` (must
fail with `expected_signal`), counts unimplemented checks as `expected-miss`, and
reports the canonical capture rate.

Run:
    python3 scripts/harness/corpus.py
"""

from __future__ import annotations

import argparse
import os
import pathlib
import re
import shutil
import subprocess
import sys
import tempfile
import tomllib
from collections.abc import Callable, Mapping
from dataclasses import dataclass, field

EXIT_OK = 0
EXIT_FAIL = 1
EXIT_USAGE = 2

DEFAULT_MIN_CASES = 22
MAX_SNIPPET_BYTES = 8192
MAX_SNIPPET_LINES = 120
MIN_NOTES_NONSPACE_LEN = 10
OUTPUT_SNIPPET_LIMIT = 400
SUBPROCESS_TIMEOUT_SECONDS = 60

CASE_DIR_RE = re.compile(r"^C-(\d{2,})$")
COMMIT_SHA_RE = re.compile(r"^[0-9a-f]{7,40}$")
WORKTREE_UNCOMMITTED = "worktree-uncommitted"

VALID_CATEGORIES = frozenset(
    {
        "autofix-logic-corruption",
        "unrequested-behavior-change",
        "control-flow-drop",
        "type-check-evasion",
        "test-isolation-hazard",
        "signature-override-drift",
        "test-assertion-weakening",
        "generated-code-tampering",
        "silent-error-swallow",
        "worktree-hygiene",
        "scanner-silent-pass",
        "blanket-suppression",
    }
)

VALID_LANGUAGES = frozenset({"go", "python", "rust", "typescript", "shell", "binary"})
VALID_EVALUATION_MODES = frozenset({"after", "diff"})

IMPLEMENTED_CHECKS = frozenset(
    {
        "pyrefly",
        "REPO-02",
        "REPO-03",
        "ARCH-01",
        "ARCH-02",
        "ARCH-03",
        "ARCH-04",
        "ARCH-05",
        "ARCH-06",
    }
)
DIFF_CHECKS: frozenset[str] = frozenset()
UNIMPLEMENTED_CORPUS_CHECKS = frozenset(
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
PROCESS_CHECKS = frozenset(
    {
        "VERIFY-01",
        "VERIFY-02",
        "VERIFY-03",
        "VERIFY-04",
        "VERIFY-05",
        "VERIFY-06",
        "LOOP-01",
        "LOOP-02",
        "LOOP-03",
        "LOOP-04",
        "LOOP-05",
        "LOOP-06",
        "LOOP-07",
        "EXEC-01",
        "EXEC-02",
        "EXEC-03",
        "EXEC-04",
    }
)
UNIMPLEMENTED_CHECKS = UNIMPLEMENTED_CORPUS_CHECKS | PROCESS_CHECKS
ALL_KNOWN_CHECKS = IMPLEMENTED_CHECKS | UNIMPLEMENTED_CORPUS_CHECKS | PROCESS_CHECKS

PYREFLY_FALLBACK_SERVICES = (
    "recap-subworker",
    "news-creator/app",
    "tag-generator/app",
    "acolyte-orchestrator",
    "metrics",
    "dashboard",
    "rerank-server",
    "recap-evaluator",
    "tts-speaker",
)
REPO02_REQUIRED_SUPPORT = ("sample.go", "before.sarif", "after.sarif")
REPO03_CONFIG_PATTERNS: tuple[tuple[re.Pattern[str], str], ...] = (
    (re.compile(r"^(?:(?:before|after)[._-]?)?pyproject\.toml$"), "pyproject.toml"),
    (re.compile(r"^(?:(?:before|after)[._-]?)?Cargo\.toml$"), "Cargo.toml"),
    (re.compile(r"^(?:(?:before|after)[._-]?)?\.?golangci\.yml$"), ".golangci.yml"),
    (re.compile(r"^(?:(?:before|after)[._-]?)?\.?golangci\.yaml$"), ".golangci.yaml"),
)
IGNORED_CORPUS_ROOT_FILES = frozenset({"README.md", ".gitkeep"})


@dataclass(frozen=True)
class CorpusOrigin:
    branch: str
    base_commit: str
    files: tuple[str, ...]
    commits: tuple[str, ...]
    source_ref: str
    notes: str


@dataclass(frozen=True)
class CorpusDetection:
    planned_checks: tuple[str, ...]
    active_checks: tuple[str, ...]
    currently_caught: bool
    current_method: str
    expected_signal: str
    expected_signals: dict[str, str] = field(default_factory=dict)


@dataclass(frozen=True)
class CorpusCase:
    case_id: str
    case_dir: pathlib.Path
    title: str
    category: str
    language: str
    before_file: str
    after_file: str
    support_files: tuple[str, ...]
    evaluation_mode: str
    origin: CorpusOrigin
    detection: CorpusDetection


@dataclass(frozen=True)
class CheckOutcome:
    exit_code: int
    output: str
    runner_error: str = ""


@dataclass(frozen=True)
class RegisteredRunner:
    """Registered corpus check runner supporting single-state ('after') or pair ('diff') modes."""

    mode: str  # "after" | "diff"
    run_single: Callable[[CorpusCase, str, pathlib.Path, float], CheckOutcome] | None = None
    run_diff: (
        Callable[[CorpusCase, pathlib.Path, pathlib.Path, pathlib.Path], CheckOutcome] | None
    ) = None


@dataclass(frozen=True)
class CaseEvaluation:
    case_id: str
    title: str
    status: str  # "caught" | "expected-miss" | "fail"
    detail: str


def _snippet(text: str) -> str:
    compact = " ".join(text.split())
    if len(compact) <= OUTPUT_SNIPPET_LIMIT:
        return compact
    return f"{compact[:OUTPUT_SNIPPET_LIMIT]}..."


def _non_empty_str(table: dict[str, object], key: str, label: str, errors: list[str]) -> str:
    val = table.get(key)
    if not isinstance(val, str) or not val.strip():
        errors.append(f"{label}: '{key}' must be a non-empty string")
        return ""
    return val


def _str_list(
    table: dict[str, object],
    key: str,
    label: str,
    errors: list[str],
    *,
    allow_empty: bool,
) -> list[str]:
    raw = table.get(key)
    if not isinstance(raw, list) or not all(isinstance(item, str) for item in raw):
        errors.append(f"{label}: '{key}' must be a list of strings")
        return []
    if not allow_empty and not raw:
        errors.append(f"{label}: '{key}' must not be empty")
    return list(raw)


def _validate_plain_filename(
    case_dir: pathlib.Path,
    filename: str,
    field_name: str,
    label: str,
    errors: list[str],
) -> pathlib.Path | None:
    if (
        not filename
        or filename in (".", "..")
        or "/" in filename
        or "\\" in filename
        or ".." in filename
    ):
        errors.append(
            f"{label}: '{field_name}' must be a plain relative filename inside case dir, got {filename!r}"
        )
        return None

    candidate = case_dir / filename
    if candidate.is_symlink():
        errors.append(f"{label}: '{field_name}' must not be a symlink: {filename!r}")
        return None
    if not candidate.is_file():
        errors.append(f"{label}: '{field_name}' file not found: {filename!r}")
        return None

    case_root_resolved = case_dir.resolve()
    resolved = candidate.resolve()
    if not resolved.is_relative_to(case_root_resolved) or resolved.parent != case_root_resolved:
        errors.append(f"{label}: '{field_name}' escapes case directory: {filename!r}")
        return None
    return candidate


def _is_normalized_repo_rel_path(rel_path: str) -> bool:
    if not rel_path or rel_path.strip() != rel_path:
        return False
    if rel_path.startswith(("/", "./")) or "\\" in rel_path:
        return False
    parts = rel_path.split("/")
    return all(part not in ("", ".", "..") for part in parts)


def _validate_origin(
    data: dict[str, object],
    label: str,
    errors: list[str],
) -> CorpusOrigin | None:
    origin_raw = data.get("origin")
    if not isinstance(origin_raw, dict):
        errors.append(f"{label}: missing [origin] table")
        return None

    branch = _non_empty_str(origin_raw, "branch", f"{label}.origin", errors)
    base_commit = _non_empty_str(origin_raw, "base_commit", f"{label}.origin", errors)
    if base_commit and not COMMIT_SHA_RE.match(base_commit):
        errors.append(
            f"{label}.origin: 'base_commit' must be 7..40 lowercase hex chars, got {base_commit!r}"
        )

    files = _str_list(origin_raw, "files", f"{label}.origin", errors, allow_empty=False)
    errors.extend(
        f"{label}.origin: invalid repo-relative path in 'files': {rel_path!r}"
        for rel_path in files
        if not _is_normalized_repo_rel_path(rel_path)
    )

    commits = _str_list(origin_raw, "commits", f"{label}.origin", errors, allow_empty=False)
    errors.extend(
        f"{label}.origin: invalid commit {commit!r} "
        f"(expected 7..40 hex SHA or {WORKTREE_UNCOMMITTED!r})"
        for commit in commits
        if commit != WORKTREE_UNCOMMITTED and not COMMIT_SHA_RE.match(commit)
    )

    source_ref = _non_empty_str(origin_raw, "source_ref", f"{label}.origin", errors)
    notes = _non_empty_str(origin_raw, "notes", f"{label}.origin", errors)
    if notes and len(re.sub(r"\s+", "", notes)) < MIN_NOTES_NONSPACE_LEN:
        errors.append(
            f"{label}.origin: 'notes' must contain at least {MIN_NOTES_NONSPACE_LEN} non-space chars"
        )

    return CorpusOrigin(
        branch=branch,
        base_commit=base_commit,
        files=tuple(files),
        commits=tuple(commits),
        source_ref=source_ref,
        notes=notes,
    )


def _validate_check_lists(
    planned: list[str],
    active: list[str],
    det_label: str,
    errors: list[str],
) -> None:
    if len(set(planned)) != len(planned):
        errors.append(f"{det_label}: duplicate entries in 'planned_checks'")
    if len(set(active)) != len(active):
        errors.append(f"{det_label}: duplicate entries in 'active_checks'")

    unknown_planned = [c for c in planned if c not in ALL_KNOWN_CHECKS]
    if unknown_planned:
        errors.append(f"{det_label}: unknown 'planned_checks': {unknown_planned}")

    unimplemented_active = [c for c in active if c not in IMPLEMENTED_CHECKS]
    if unimplemented_active:
        errors.append(
            f"{det_label}: 'active_checks' contains non-implemented check(s): {unimplemented_active}"
        )

    active_not_planned = [c for c in active if c not in planned]
    if active_not_planned:
        errors.append(
            f"{det_label}: 'active_checks' must be a subset of 'planned_checks', "
            f"extra: {active_not_planned}"
        )

    unactivated_impl = [
        c
        for c in planned
        if c in IMPLEMENTED_CHECKS and c not in PROCESS_CHECKS and c not in active
    ]
    if unactivated_impl:
        errors.append(
            f"{det_label}: implemented check(s) in 'planned_checks' must be listed in "
            f"'active_checks': {unactivated_impl}"
        )


def _parse_expected_signals_table(
    raw_table: object,
    active: list[str],
    det_label: str,
    errors: list[str],
) -> dict[str, str]:
    if not isinstance(raw_table, dict) or not all(
        isinstance(k, str) and isinstance(v, str) for k, v in raw_table.items()
    ):
        errors.append(f"{det_label}: 'expected_signals' must be a table of string -> string")
        return {}
    parsed = {str(k): str(v) for k, v in raw_table.items()}
    if set(parsed.keys()) != set(active):
        errors.append(
            f"{det_label}: 'expected_signals' keys {sorted(parsed.keys())} must match "
            f"'active_checks' {sorted(active)}"
        )
    empty_keys = sorted(k for k, v in parsed.items() if not v.strip())
    if empty_keys:
        errors.append(
            f"{det_label}: 'expected_signals' has empty signal(s) for check(s): {empty_keys}"
        )
    return parsed


def _validate_expected_signals(
    det_raw: dict[str, object],
    active: list[str],
    caught_bool: bool,
    det_label: str,
    errors: list[str],
) -> tuple[str, dict[str, str]]:
    expected_signal_raw = det_raw.get("expected_signal", "")
    if not isinstance(expected_signal_raw, str):
        errors.append(f"{det_label}: 'expected_signal' must be a string")
        expected_signal = ""
    else:
        expected_signal = expected_signal_raw

    has_signals_table = "expected_signals" in det_raw
    if not caught_bool:
        if expected_signal != "":
            errors.append(f"{det_label}: 'expected_signal' must be '' when currently_caught=false")
        if has_signals_table and det_raw["expected_signals"] != {}:
            errors.append(
                f"{det_label}: 'expected_signals' must be empty when currently_caught=false"
            )
        return expected_signal, {}

    if has_signals_table:
        if expected_signal != "":
            errors.append(
                f"{det_label}: do not set both non-empty 'expected_signal' and 'expected_signals'"
            )
        parsed = _parse_expected_signals_table(
            det_raw["expected_signals"],
            active,
            det_label,
            errors,
        )
        return expected_signal, parsed

    if len(active) > 1:
        errors.append(
            f"{det_label}: multiple active_checks require [detection.expected_signals] "
            "mapping each active check to its expected signal"
        )
        return expected_signal, {}

    if not expected_signal.strip():
        errors.append(
            f"{det_label}: 'expected_signal' must be non-empty when currently_caught=true"
        )
        return expected_signal, {}

    return expected_signal, {active[0]: expected_signal} if active else {}


def _validate_detection(
    data: dict[str, object],
    label: str,
    errors: list[str],
) -> CorpusDetection | None:
    det_raw = data.get("detection")
    if not isinstance(det_raw, dict):
        errors.append(f"{label}: missing [detection] table")
        return None

    det_label = f"{label}.detection"
    planned = _str_list(det_raw, "planned_checks", det_label, errors, allow_empty=False)
    active = _str_list(det_raw, "active_checks", det_label, errors, allow_empty=True)
    _validate_check_lists(planned, active, det_label, errors)

    caught_val = det_raw.get("currently_caught")
    if not isinstance(caught_val, bool):
        errors.append(f"{det_label}: 'currently_caught' must be a boolean")
        caught_bool = False
    else:
        caught_bool = caught_val
        if caught_bool != bool(active):
            errors.append(
                f"{det_label}: 'currently_caught' ({caught_bool}) must equal "
                f"bool(active_checks) ({bool(active)})"
            )

    current_method = _non_empty_str(det_raw, "current_method", det_label, errors)
    expected_signal, expected_signals = _validate_expected_signals(
        det_raw,
        active,
        caught_bool,
        det_label,
        errors,
    )

    return CorpusDetection(
        planned_checks=tuple(planned),
        active_checks=tuple(active),
        currently_caught=caught_bool,
        current_method=current_method,
        expected_signal=expected_signal,
        expected_signals=expected_signals,
    )


def _validate_snippet_bytes(
    before_path: pathlib.Path,
    after_path: pathlib.Path,
    language: str,
    label: str,
    errors: list[str],
) -> None:
    before_bytes = before_path.read_bytes()
    after_bytes = after_path.read_bytes()

    if before_bytes == after_bytes:
        errors.append(f"{label}: before and after files are byte-identical")
    if len(before_bytes) > MAX_SNIPPET_BYTES:
        errors.append(
            f"{label}: before file exceeds {MAX_SNIPPET_BYTES} bytes ({len(before_bytes)} bytes)"
        )
    if len(after_bytes) > MAX_SNIPPET_BYTES:
        errors.append(
            f"{label}: after file exceeds {MAX_SNIPPET_BYTES} bytes ({len(after_bytes)} bytes)"
        )

    if language == "binary":
        return

    for role, raw in (("before", before_bytes), ("after", after_bytes)):
        try:
            text = raw.decode("utf-8")
        except UnicodeDecodeError as exc:
            errors.append(
                f"{label}: {role} file is not valid UTF-8 for language={language!r}: {exc}"
            )
            continue
        line_count = len(text.splitlines())
        if line_count > MAX_SNIPPET_LINES:
            errors.append(
                f"{label}: {role} file exceeds {MAX_SNIPPET_LINES} lines ({line_count} lines)"
            )


@dataclass(frozen=True)
class _CaseFileSpec:
    before_file: str
    after_file: str
    support_files: tuple[str, ...]
    language: str


def _validate_case_files(
    case_dir: pathlib.Path,
    spec: _CaseFileSpec,
    label: str,
    errors: list[str],
) -> None:
    if spec.before_file and spec.after_file and spec.before_file == spec.after_file:
        errors.append(f"{label}: 'before_file' and 'after_file' must be distinct")

    before_path = (
        _validate_plain_filename(case_dir, spec.before_file, "before_file", label, errors)
        if spec.before_file
        else None
    )
    after_path = (
        _validate_plain_filename(case_dir, spec.after_file, "after_file", label, errors)
        if spec.after_file
        else None
    )

    if len(set(spec.support_files)) != len(spec.support_files):
        errors.append(f"{label}: duplicate entries in 'support_files'")
    reserved = {spec.before_file, spec.after_file, "meta.toml"}
    for sf in spec.support_files:
        if sf in reserved:
            errors.append(f"{label}: 'support_files' must not duplicate {sf!r}")
            continue
        _validate_plain_filename(case_dir, sf, "support_files", label, errors)

    if before_path is not None and after_path is not None:
        _validate_snippet_bytes(before_path, after_path, spec.language, label, errors)


def load_case(case_dir: pathlib.Path) -> tuple[CorpusCase | None, list[str]]:
    """Parse and strictly validate a single corpus case directory."""
    expected_id = case_dir.name
    label = f"case {expected_id}"
    meta_path = case_dir / "meta.toml"
    if not meta_path.is_file() or meta_path.is_symlink():
        return None, [f"{label}: missing regular meta.toml"]

    try:
        data = tomllib.loads(meta_path.read_text(encoding="utf-8"))
    except (OSError, UnicodeDecodeError, tomllib.TOMLDecodeError) as exc:
        return None, [f"{label}: failed to parse meta.toml: {exc}"]

    errors: list[str] = []
    case_id = _non_empty_str(data, "id", label, errors)
    if case_id and case_id != expected_id:
        errors.append(f"{label}: meta.toml id {case_id!r} does not match directory {expected_id!r}")

    title = _non_empty_str(data, "title", label, errors)
    category = _non_empty_str(data, "category", label, errors)
    if category and category not in VALID_CATEGORIES:
        errors.append(f"{label}: invalid category {category!r}")

    language = _non_empty_str(data, "language", label, errors)
    if language and language not in VALID_LANGUAGES:
        errors.append(f"{label}: invalid language {language!r}")

    before_file = _non_empty_str(data, "before_file", label, errors)
    after_file = _non_empty_str(data, "after_file", label, errors)
    support_files: list[str] = []
    if "support_files" in data:
        support_files = _str_list(data, "support_files", label, errors, allow_empty=True)

    evaluation_mode = _non_empty_str(data, "evaluation_mode", label, errors)
    if evaluation_mode and evaluation_mode not in VALID_EVALUATION_MODES:
        errors.append(f"{label}: invalid evaluation_mode {evaluation_mode!r}")

    origin = _validate_origin(data, label, errors)
    detection = _validate_detection(data, label, errors)
    _validate_case_files(
        case_dir,
        _CaseFileSpec(
            before_file=before_file,
            after_file=after_file,
            support_files=tuple(support_files),
            language=language,
        ),
        label,
        errors,
    )

    if errors or origin is None or detection is None:
        return None, errors

    return (
        CorpusCase(
            case_id=case_id,
            case_dir=case_dir,
            title=title,
            category=category,
            language=language,
            before_file=before_file,
            after_file=after_file,
            support_files=tuple(support_files),
            evaluation_mode=evaluation_mode,
            origin=origin,
            detection=detection,
        ),
        [],
    )


def _collect_case_dirs(
    corpus_dir: pathlib.Path,
    errors: list[str],
) -> list[pathlib.Path]:
    case_dirs: list[pathlib.Path] = []
    for entry in sorted(corpus_dir.iterdir(), key=lambda p: p.name):
        if entry.is_symlink():
            errors.append(f"symlink entry not allowed in corpus: {entry.name!r}")
            continue
        if entry.is_dir():
            if not CASE_DIR_RE.match(entry.name):
                errors.append(f"unexpected non-case directory in corpus: {entry.name!r}")
                continue
            case_dirs.append(entry)
        elif entry.name not in IGNORED_CORPUS_ROOT_FILES:
            errors.append(f"unexpected file in corpus root: {entry.name!r}")
    return case_dirs


def discover_cases(
    corpus_dir: pathlib.Path,
    *,
    min_cases: int,
    allow_empty_corpus: bool,
) -> tuple[list[CorpusCase], list[str]]:
    """Validate contiguous C-01..C-NN directory structure and load all cases."""
    errors: list[str] = []
    if corpus_dir.is_symlink():
        return [], [f"corpus directory must not be a symlink: {corpus_dir}"]
    if not corpus_dir.is_dir():
        if allow_empty_corpus:
            return [], []
        return [], [f"corpus directory not found: {corpus_dir}"]

    case_dirs = _collect_case_dirs(corpus_dir, errors)

    if not case_dirs:
        if allow_empty_corpus and not errors:
            return [], []
        errors.append(f"corpus directory has 0 cases (expected >= {min_cases}): {corpus_dir}")
        return [], errors

    if len(case_dirs) < min_cases:
        errors.append(f"corpus has {len(case_dirs)} case(s), expected at least {min_cases}")

    expected_names = [f"C-{idx:02d}" for idx in range(1, len(case_dirs) + 1)]
    actual_names = [d.name for d in case_dirs]
    if actual_names != expected_names:
        errors.append(
            f"case directories must be contiguous C-01..C-{len(case_dirs):02d}: "
            f"expected {expected_names}, got {actual_names}"
        )

    cases: list[CorpusCase] = []
    for case_dir in case_dirs:
        loaded, case_errors = load_case(case_dir)
        errors.extend(case_errors)
        if loaded is not None:
            cases.append(loaded)
    return cases, errors


def _resolve_pyrefly_bin(repo_root: pathlib.Path) -> tuple[pathlib.Path | None, str]:
    env_bin = os.environ.get("PYREFLY_BIN", "").strip()
    if env_bin:
        candidate = pathlib.Path(env_bin)
        if candidate.is_file() and os.access(candidate, os.X_OK):
            return candidate, ""
        resolved = shutil.which(env_bin)
        if resolved is not None:
            return pathlib.Path(resolved), ""
        return None, f"PYREFLY_BIN={env_bin!r} does not point to an executable binary"

    on_path = shutil.which("pyrefly")
    if on_path is not None:
        return pathlib.Path(on_path), ""

    for svc in PYREFLY_FALLBACK_SERVICES:
        venv_bin = repo_root / svc / ".venv" / "bin" / "pyrefly"
        if venv_bin.is_file() and os.access(venv_bin, os.X_OK):
            return venv_bin, ""

    return None, "pyrefly executable not found (checked PYREFLY_BIN, PATH, and service .venv/bin)"


def _run_pyrefly_check(
    case: CorpusCase,
    phase: str,
    repo_root: pathlib.Path,
    timeout: float = SUBPROCESS_TIMEOUT_SECONDS,
) -> CheckOutcome:
    pyrefly_bin, err = _resolve_pyrefly_bin(repo_root)
    if pyrefly_bin is None:
        return CheckOutcome(exit_code=-1, output="", runner_error=err)

    target_name = case.before_file if phase == "before" else case.after_file
    with tempfile.TemporaryDirectory(prefix="alt-corpus-pyrefly-") as tmp_str:
        tmp_dir = pathlib.Path(tmp_str)
        sample_py = tmp_dir / "sample.py"
        shutil.copyfile(case.case_dir / target_name, sample_py)

        for sf in case.support_files:
            shutil.copyfile(case.case_dir / sf, tmp_dir / sf)

        if "pyrefly.toml" in case.support_files:
            cfg_path = tmp_dir / "pyrefly.toml"
        else:
            cfg_path = tmp_dir / "pyproject.toml"
            cfg_path.write_text('[tool.pyrefly]\npython-version = "3.14"\n', encoding="utf-8")

        cmd = [str(pyrefly_bin), "check", "--config", str(cfg_path), str(sample_py)]
        try:
            res = subprocess.run(  # noqa: S603 - resolved pyrefly binary with fixed argv
                cmd,
                cwd=str(tmp_dir),
                capture_output=True,
                text=True,
                timeout=timeout,
                check=False,
            )
        except subprocess.TimeoutExpired:
            return CheckOutcome(
                exit_code=-1,
                output="",
                runner_error=f"pyrefly timed out after {timeout}s",
            )
        except OSError as exc:
            return CheckOutcome(exit_code=-1, output="", runner_error=f"pyrefly exec error: {exc}")

    combined = f"{res.stdout}\n{res.stderr}".strip()
    return CheckOutcome(exit_code=res.returncode, output=combined)


def _run_repo02_check(
    case: CorpusCase,
    phase: str,
    repo_root: pathlib.Path,
    timeout: float = SUBPROCESS_TIMEOUT_SECONDS,
) -> CheckOutcome:
    missing = [sf for sf in REPO02_REQUIRED_SUPPORT if sf not in case.support_files]
    if missing:
        return CheckOutcome(
            exit_code=-1,
            output="",
            runner_error=f"REPO-02 case {case.case_id} missing required support_files: {missing}",
        )

    canary_script = repo_root / "scripts" / "lint" / "canary_audit.py"
    if not canary_script.is_file():
        return CheckOutcome(
            exit_code=-1,
            output="",
            runner_error=f"missing canary_audit.py at {canary_script}",
        )

    gosec_out_name = case.before_file if phase == "before" else case.after_file
    sarif_name = "before.sarif" if phase == "before" else "after.sarif"

    with tempfile.TemporaryDirectory(prefix="alt-corpus-repo02-") as tmp_str:
        proj_dir = pathlib.Path(tmp_str) / "proj"
        proj_dir.mkdir()
        shutil.copyfile(case.case_dir / "sample.go", proj_dir / "sample.go")

        cmd = [
            sys.executable,
            str(canary_script),
            "go-verify-scan",
            "--project-dir",
            str(proj_dir),
            "--gosec-out",
            str(case.case_dir / gosec_out_name),
            "--gosec-sarif",
            str(case.case_dir / sarif_name),
        ]
        try:
            res = subprocess.run(  # noqa: S603 - fixed argv running canary_audit.py
                cmd,
                capture_output=True,
                text=True,
                timeout=timeout,
                check=False,
            )
        except subprocess.TimeoutExpired:
            return CheckOutcome(
                exit_code=-1,
                output="",
                runner_error=f"REPO-02 timed out after {timeout}s",
            )
        except OSError as exc:
            return CheckOutcome(exit_code=-1, output="", runner_error=f"REPO-02 exec error: {exc}")

    combined = f"{res.stdout}\n{res.stderr}".strip()
    return CheckOutcome(exit_code=res.returncode, output=combined)


def _normalize_repo03_dest_name(target_name: str, suffix: str) -> str:
    for pattern, canonical in REPO03_CONFIG_PATTERNS:
        if pattern.match(target_name):
            return canonical
    return f"sample{suffix}"


def _run_repo03_check(
    case: CorpusCase,
    phase: str,
    repo_root: pathlib.Path,
    timeout: float = SUBPROCESS_TIMEOUT_SECONDS,
) -> CheckOutcome:
    checker_script = repo_root / "scripts" / "lint" / "check_suppressions.py"
    allowlist_src = repo_root / "scripts" / "lint" / "suppression_allowlist.toml"
    git_bin = shutil.which("git")
    if not checker_script.is_file() or not allowlist_src.is_file() or git_bin is None:
        return CheckOutcome(
            exit_code=-1,
            output="",
            runner_error="REPO-03 prerequisites missing (check_suppressions.py, allowlist, or git)",
        )

    target_name = case.before_file if phase == "before" else case.after_file
    target_src = case.case_dir / target_name

    with tempfile.TemporaryDirectory(prefix="alt-corpus-repo03-") as tmp_str:
        tmp_repo = pathlib.Path(tmp_str)
        try:
            init_res = subprocess.run(  # noqa: S603 - fixed git init invocation in temp dir
                [git_bin, "-C", str(tmp_repo), "init", "-q"],
                capture_output=True,
                text=True,
                timeout=timeout,
                check=False,
            )
        except subprocess.TimeoutExpired:
            return CheckOutcome(
                exit_code=-1,
                output="",
                runner_error=f"REPO-03 git init timed out after {timeout}s",
            )
        if init_res.returncode != 0:
            return CheckOutcome(
                exit_code=-1,
                output="",
                runner_error=f"git init failed: {_snippet(init_res.stderr)}",
            )

        real_adr_dir = repo_root / "docs" / "ADR"
        tmp_adr_parent = tmp_repo / "docs"
        tmp_adr_parent.mkdir(parents=True, exist_ok=True)
        if real_adr_dir.is_dir():
            (tmp_adr_parent / "ADR").symlink_to(real_adr_dir)
        else:
            fallback_adr = tmp_adr_parent / "ADR"
            fallback_adr.mkdir(parents=True, exist_ok=True)
            (fallback_adr / "000979.md").write_text("# ADR-000979\n", encoding="utf-8")

        pkg_dir = tmp_repo / "pkg"
        pkg_dir.mkdir(parents=True, exist_ok=True)
        dest_name = _normalize_repo03_dest_name(target_name, target_src.suffix)
        shutil.copyfile(target_src, pkg_dir / dest_name)
        for sf in case.support_files:
            shutil.copyfile(case.case_dir / sf, pkg_dir / sf)

        cmd = [
            sys.executable,
            str(checker_script),
            "--repo-root",
            str(tmp_repo),
            "--allowlist",
            str(allowlist_src),
        ]
        try:
            res = subprocess.run(  # noqa: S603 - fixed argv running check_suppressions.py
                cmd,
                capture_output=True,
                text=True,
                timeout=timeout,
                check=False,
            )
        except subprocess.TimeoutExpired:
            return CheckOutcome(
                exit_code=-1,
                output="",
                runner_error=f"REPO-03 timed out after {timeout}s",
            )
        except OSError as exc:
            return CheckOutcome(exit_code=-1, output="", runner_error=f"REPO-03 exec error: {exc}")

    combined = f"{res.stdout}\n{res.stderr}".strip()
    return CheckOutcome(exit_code=res.returncode, output=combined)


def _resolve_ast_grep_bin() -> tuple[pathlib.Path | None, str]:
    env_bin = os.environ.get("AST_GREP_BIN", "").strip()
    if env_bin:
        candidate = pathlib.Path(env_bin)
        if candidate.is_file() and os.access(candidate, os.X_OK):
            return candidate, ""
        resolved = shutil.which(env_bin)
        if resolved is not None:
            return pathlib.Path(resolved), ""
        return None, f"AST_GREP_BIN={env_bin!r} does not point to an executable binary"

    on_path = shutil.which("ast-grep")
    if on_path is not None:
        return pathlib.Path(on_path), ""

    local_bin = pathlib.Path.home() / ".local" / "bin" / "ast-grep"
    if local_bin.is_file() and os.access(local_bin, os.X_OK):
        return local_bin, ""

    return (
        None,
        "ast-grep executable not found (checked AST_GREP_BIN, PATH, and ~/.local/bin/ast-grep)",
    )


def _run_ast_grep_rules(
    case: CorpusCase,
    phase: str,
    rule_paths: list[pathlib.Path],
    *,
    prefix: str,
    timeout: float,
) -> CheckOutcome:
    ast_grep_bin, err = _resolve_ast_grep_bin()
    if ast_grep_bin is None:
        return CheckOutcome(exit_code=-1, output="", runner_error=err)

    missing_rules = [str(rp) for rp in rule_paths if not rp.is_file()]
    if not rule_paths or missing_rules:
        return CheckOutcome(
            exit_code=-1,
            output="",
            runner_error=f"missing ast-grep rule file(s): {missing_rules}",
        )

    target_name = case.before_file if phase == "before" else case.after_file
    with tempfile.TemporaryDirectory(prefix=prefix) as tmp_str:
        tmp_dir = pathlib.Path(tmp_str)
        src_dir = tmp_dir / "src"
        src_dir.mkdir()
        shutil.copyfile(case.case_dir / target_name, src_dir / target_name)
        for sf in case.support_files:
            shutil.copyfile(case.case_dir / sf, src_dir / sf)

        rules_dir = tmp_dir / "rules"
        rules_dir.mkdir()
        for rp in rule_paths:
            shutil.copyfile(rp, rules_dir / rp.name)

        sgconfig = tmp_dir / "sgconfig.yml"
        sgconfig.write_text("ruleDirs:\n  - rules\n", encoding="utf-8")

        cmd = [str(ast_grep_bin), "scan", "-c", str(sgconfig), "src"]
        try:
            res = subprocess.run(  # noqa: S603 - resolved ast-grep binary with fixed argv
                cmd,
                cwd=str(tmp_dir),
                capture_output=True,
                text=True,
                timeout=timeout,
                check=False,
            )
        except subprocess.TimeoutExpired:
            return CheckOutcome(
                exit_code=-1,
                output="",
                runner_error=f"ast-grep timed out after {timeout}s",
            )
        except OSError as exc:
            return CheckOutcome(exit_code=-1, output="", runner_error=f"ast-grep exec error: {exc}")

    combined = f"{res.stdout}\n{res.stderr}".strip()
    return CheckOutcome(exit_code=res.returncode, output=combined)


def _arch02_rule_paths(language: str, repo_root: pathlib.Path) -> tuple[list[pathlib.Path], str]:
    rules_dir = repo_root / "scripts" / "lint" / "ast-grep" / "rules"
    if language == "go":
        go_rules = sorted(rules_dir.glob("go-*.yml"))
        if not go_rules:
            return [], f"no go-*.yml ast-grep rules found in {rules_dir}"
        return go_rules, ""
    if language == "python":
        return [rules_dir / "python-no-manual-aclose-in-finally.yml"], ""
    if language == "rust":
        return [rules_dir / "rust-no-silent-option-regex.yml"], ""
    return [], f"ARCH-02 has no ast-grep rules configured for language {language!r}"


def _run_arch02_check(
    case: CorpusCase,
    phase: str,
    repo_root: pathlib.Path,
    timeout: float = SUBPROCESS_TIMEOUT_SECONDS,
) -> CheckOutcome:
    rule_paths, err = _arch02_rule_paths(case.language, repo_root)
    if err:
        return CheckOutcome(exit_code=-1, output="", runner_error=err)
    return _run_ast_grep_rules(
        case,
        phase,
        rule_paths,
        prefix="alt-corpus-arch02-",
        timeout=timeout,
    )


def _select_phase_pyproject(case: CorpusCase, phase: str) -> str | None:
    candidates = (
        f"{phase}.pyproject.toml",
        f"{phase}_pyproject.toml",
        f"{phase}-pyproject.toml",
        "pyproject.toml",
    )
    for name in candidates:
        if name in case.support_files:
            return name
    return None


def _stage_arch03_inputs(
    case: CorpusCase,
    phase: str,
    tmp_dir: pathlib.Path,
) -> tuple[pathlib.Path | None, list[pathlib.Path], str]:
    target_name = case.before_file if phase == "before" else case.after_file
    target_src = case.case_dir / target_name
    other_phase = "after" if phase == "before" else "before"
    pyproject_dest = tmp_dir / "pyproject.toml"
    source_paths: list[pathlib.Path] = []

    if target_src.suffix == ".toml":
        shutil.copyfile(target_src, pyproject_dest)
    else:
        pyproject_name = _select_phase_pyproject(case, phase)
        if pyproject_name is None:
            return (
                None,
                [],
                f"ARCH-03 case {case.case_id} missing phase pyproject.toml for {phase!r}",
            )
        shutil.copyfile(case.case_dir / pyproject_name, pyproject_dest)
        dest_py = tmp_dir / target_name
        shutil.copyfile(target_src, dest_py)
        source_paths.append(dest_py)

    for sf in case.support_files:
        if sf.endswith(".toml") or sf.startswith(
            (f"{other_phase}.", f"{other_phase}_", f"{other_phase}-")
        ):
            continue
        dest_sf = tmp_dir / sf
        shutil.copyfile(case.case_dir / sf, dest_sf)
        if sf.endswith(".py"):
            source_paths.append(dest_sf)

    return pyproject_dest, source_paths, ""


def _run_arch03_check(
    case: CorpusCase,
    phase: str,
    repo_root: pathlib.Path,
    timeout: float = SUBPROCESS_TIMEOUT_SECONDS,
) -> CheckOutcome:
    checker_script = repo_root / "scripts" / "lint" / "check_python_versions.py"
    if not checker_script.is_file():
        return CheckOutcome(
            exit_code=-1,
            output="",
            runner_error=f"missing check_python_versions.py at {checker_script}",
        )

    with tempfile.TemporaryDirectory(prefix="alt-corpus-arch03-") as tmp_str:
        tmp_dir = pathlib.Path(tmp_str)
        pyproject_dest, source_paths, err = _stage_arch03_inputs(case, phase, tmp_dir)
        if pyproject_dest is None or err:
            return CheckOutcome(exit_code=-1, output="", runner_error=err)

        cmd = [
            sys.executable,
            str(checker_script),
            "--repo-root",
            str(tmp_dir),
            "--pyproject",
            str(pyproject_dest),
        ]
        for src in source_paths:
            cmd.extend(["--source", str(src)])

        try:
            res = subprocess.run(  # noqa: S603 - fixed argv running check_python_versions.py
                cmd,
                capture_output=True,
                text=True,
                timeout=timeout,
                check=False,
            )
        except subprocess.TimeoutExpired:
            return CheckOutcome(
                exit_code=-1,
                output="",
                runner_error=f"ARCH-03 timed out after {timeout}s",
            )
        except OSError as exc:
            return CheckOutcome(exit_code=-1, output="", runner_error=f"ARCH-03 exec error: {exc}")

    combined = f"{res.stdout}\n{res.stderr}".strip()
    return CheckOutcome(exit_code=res.returncode, output=combined)


def _run_arch06_check(
    case: CorpusCase,
    phase: str,
    repo_root: pathlib.Path,
    timeout: float = SUBPROCESS_TIMEOUT_SECONDS,
) -> CheckOutcome:
    rule_path = (
        repo_root
        / "scripts"
        / "lint"
        / "ast-grep"
        / "rules"
        / "python-no-direct-sys-modules-assign.yml"
    )
    return _run_ast_grep_rules(
        case,
        phase,
        [rule_path],
        prefix="alt-corpus-arch06-",
        timeout=timeout,
    )


BUILTIN_RUNNERS: dict[str, RegisteredRunner] = {
    "pyrefly": RegisteredRunner(mode="after", run_single=_run_pyrefly_check),
    "REPO-02": RegisteredRunner(mode="after", run_single=_run_repo02_check),
    "REPO-03": RegisteredRunner(mode="after", run_single=_run_repo03_check),
    "ARCH-02": RegisteredRunner(mode="after", run_single=_run_arch02_check),
    "ARCH-03": RegisteredRunner(mode="after", run_single=_run_arch03_check),
    "ARCH-06": RegisteredRunner(mode="after", run_single=_run_arch06_check),
}


def run_check(
    check_id: str,
    case: CorpusCase,
    phase: str,
    repo_root: pathlib.Path,
    *,
    timeout: float = SUBPROCESS_TIMEOUT_SECONDS,
) -> CheckOutcome:
    """Dispatch an implemented single-state check runner on a case's before or after snippet."""
    spec = BUILTIN_RUNNERS.get(check_id)
    if spec is None or spec.run_single is None:
        return CheckOutcome(
            exit_code=-1,
            output="",
            runner_error=f"no single-state runner registered for check {check_id!r}",
        )
    return spec.run_single(case, phase, repo_root, timeout)


def _verify_pair_outcomes(
    check_id: str,
    baseline: CheckOutcome,
    regression: CheckOutcome,
    signal: str,
) -> str | None:
    if baseline.runner_error:
        return f"{check_id}:runner-error({baseline.runner_error})"
    if baseline.exit_code != 0:
        return (
            f"{check_id}:false-positive(before exit={baseline.exit_code}, "
            f"out={_snippet(baseline.output)})"
        )
    if regression.runner_error:
        return f"{check_id}:runner-error({regression.runner_error})"
    if regression.exit_code == 0:
        return f"{check_id}:unexpected-miss(after exit=0, out={_snippet(regression.output)})"
    if signal not in regression.output:
        return (
            f"{check_id}:wrong-signal(expected {signal!r} in output, "
            f"exit={regression.exit_code}, out={_snippet(regression.output)})"
        )
    return None


def _run_registered_check(
    check_id: str,
    spec: RegisteredRunner,
    case: CorpusCase,
    repo_root: pathlib.Path,
    timeout: float,
) -> tuple[CheckOutcome, CheckOutcome]:
    if spec.mode == "diff" or check_id in DIFF_CHECKS:
        if spec.run_diff is None:
            err = CheckOutcome(
                exit_code=-1,
                output="",
                runner_error=f"diff runner {check_id!r} missing run_diff callable",
            )
            return err, err
        before_path = case.case_dir / case.before_file
        after_path = case.case_dir / case.after_file
        baseline = spec.run_diff(case, before_path, before_path, repo_root)
        if baseline.runner_error or baseline.exit_code != 0:
            return baseline, CheckOutcome(exit_code=0, output="")
        regression = spec.run_diff(case, before_path, after_path, repo_root)
        return baseline, regression

    if spec.run_single is None:
        err = CheckOutcome(
            exit_code=-1,
            output="",
            runner_error=f"after runner {check_id!r} missing run_single callable",
        )
        return err, err
    baseline = spec.run_single(case, "before", repo_root, timeout)
    if baseline.runner_error or baseline.exit_code != 0:
        return baseline, CheckOutcome(exit_code=0, output="")
    regression = spec.run_single(case, "after", repo_root, timeout)
    return baseline, regression


def evaluate_case(
    case: CorpusCase,
    repo_root: pathlib.Path,
    *,
    extra_runners: Mapping[str, RegisteredRunner] | None = None,
    timeout: float = SUBPROCESS_TIMEOUT_SECONDS,
) -> CaseEvaluation:
    """Evaluate all active checks for a validated corpus case."""
    active = case.detection.active_checks
    if not active:
        planned_str = ",".join(case.detection.planned_checks)
        return CaseEvaluation(
            case_id=case.case_id,
            title=case.title,
            status="expected-miss",
            detail=f"planned={planned_str}",
        )

    failures: list[str] = []
    for check_id in active:
        spec = (
            extra_runners.get(check_id)
            if extra_runners is not None and check_id in extra_runners
            else BUILTIN_RUNNERS.get(check_id)
        )
        if spec is None:
            failures.append(f"{check_id}:runner-error(no runner registered)")
            continue
        signal = case.detection.expected_signals.get(check_id, case.detection.expected_signal)
        baseline, regression = _run_registered_check(check_id, spec, case, repo_root, timeout)
        failure = _verify_pair_outcomes(check_id, baseline, regression, signal)
        if failure is not None:
            failures.append(failure)

    if failures:
        return CaseEvaluation(
            case_id=case.case_id,
            title=case.title,
            status="fail",
            detail="; ".join(failures),
        )
    return CaseEvaluation(
        case_id=case.case_id,
        title=case.title,
        status="caught",
        detail=",".join(active),
    )


def format_capture_rate_line(
    evaluations: list[CaseEvaluation],
    validation_error_count: int,
) -> str:
    caught_ids = [ev.case_id for ev in evaluations if ev.status == "caught"]
    miss_count = sum(1 for ev in evaluations if ev.status == "expected-miss")
    eval_fail_count = sum(1 for ev in evaluations if ev.status == "fail")
    total_failures = eval_fail_count + validation_error_count
    total = len(evaluations)
    pct = (100.0 * len(caught_ids) / total) if total > 0 else 0.0
    caught_list = ",".join(caught_ids) if caught_ids else "none"
    return (
        f"CORPUS CAPTURE RATE: {len(caught_ids)}/{total} ({pct:.1f}%) "
        f"[caught={len(caught_ids)}: {caught_list} | "
        f"expected-miss={miss_count} | failures={total_failures}]"
    )


def parse_args(argv: list[str] | None = None) -> argparse.Namespace:
    default_root = pathlib.Path(__file__).resolve().parents[2]
    parser = argparse.ArgumentParser(
        description="Validate CORPUS-01 failure cases and report check capture rate.",
    )
    parser.add_argument(
        "--corpus-dir",
        type=pathlib.Path,
        default=None,
        help="Path to corpus directory (default: <repo-root>/scripts/harness/corpus).",
    )
    parser.add_argument(
        "--repo-root",
        type=pathlib.Path,
        default=default_root,
        help="Repository root path (default: detected repo root).",
    )
    parser.add_argument(
        "--min-cases",
        type=int,
        default=DEFAULT_MIN_CASES,
        help=f"Minimum required contiguous corpus cases (default: {DEFAULT_MIN_CASES}).",
    )
    parser.add_argument(
        "--allow-empty-corpus",
        action="store_true",
        default=False,
        help="Explicitly allow an empty or missing corpus directory.",
    )
    return parser.parse_args(argv)


def main(argv: list[str] | None = None) -> int:
    args = parse_args(argv)
    if args.min_cases < 0:
        sys.stderr.write("error: --min-cases must be >= 0\n")
        return EXIT_USAGE

    repo_root = args.repo_root.resolve()
    corpus_dir = (
        args.corpus_dir
        if args.corpus_dir is not None
        else repo_root / "scripts" / "harness" / "corpus"
    )

    cases, validation_errors = discover_cases(
        corpus_dir,
        min_cases=args.min_cases,
        allow_empty_corpus=args.allow_empty_corpus,
    )

    for err in validation_errors:
        sys.stderr.write(f"VALIDATION ERROR: {err}\n")

    evaluations: list[CaseEvaluation] = []
    for case in cases:
        ev = evaluate_case(case, repo_root)
        evaluations.append(ev)
        tag = ev.status.upper()
        sys.stdout.write(f"  {tag:<13}  {ev.case_id}  [{ev.detail}]  {ev.title}\n")

    summary_line = format_capture_rate_line(evaluations, len(validation_errors))
    sys.stdout.write(f"{summary_line}\n")

    has_eval_failures = any(ev.status == "fail" for ev in evaluations)
    if validation_errors or has_eval_failures:
        return EXIT_FAIL
    return EXIT_OK


if __name__ == "__main__":
    sys.exit(main())
