#!/usr/bin/env python3
"""Validate single-source Python version configuration across the repository (ARCH-03).

Enforces:
1. Ruff must not set an explicit `target-version` in `[tool.ruff]`, `[tool.ruff.lint]`,
   or `[tool.ruff.format]` — Ruff derives target version exclusively from
   `[project].requires-python`.
2. Every `pyproject.toml` must declare `[project].requires-python` with an extractable
   minimum `major.minor` floor (e.g. `>=3.13`, `>=3.14,<3.15`).
3. `scripts/pyproject.toml` must declare a `3.13` floor matching host `python3`, and
   Python sources checked against a `<3.14` floor must parse cleanly under that grammar
   (explicitly rejecting PEP 758 unparenthesized `except A, B:` even when this script
   executes on Python 3.14+).
4. Service `Dockerfile*` `FROM python:<major.minor>...` stages must match the service's
   `[project].requires-python` floor.

Run:
    python3 scripts/lint/check_python_versions.py
    python3 scripts/lint/check_python_versions.py --project-dir scripts
    python3 scripts/lint/check_python_versions.py --pyproject path/to/pyproject.toml --source path/to/file.py
"""

from __future__ import annotations

import argparse
import ast
import io
import os
import pathlib
import re
import shutil
import subprocess
import sys
import tokenize
import tomllib
from dataclasses import dataclass

EXIT_CLEAN = 0
EXIT_VIOLATIONS = 1
EXIT_USAGE_ERROR = 2

SCRIPTS_EXPECTED_FLOOR = (3, 13)
PEP758_MIN_VERSION = (3, 14)

EXCLUDED_REPO_PREFIXES = ("stopped-using-k8s/", "scripts/harness/corpus/")
EXCLUDED_DIR_NAMES = frozenset(
    {
        ".git",
        ".venv",
        "__pycache__",
        "build",
        "dist",
        "node_modules",
        ".mypy_cache",
        ".pytest_cache",
        ".ruff_cache",
    }
)

REQUIRES_PYTHON_FLOOR_PATTERNS: tuple[re.Pattern[str], ...] = (
    re.compile(r">=\s*(\d+)\.(\d+)"),
    re.compile(r"~=\s*(\d+)\.(\d+)"),
    re.compile(r"==\s*(\d+)\.(\d+)"),
)
DOCKERFILE_FROM_PYTHON_RE = re.compile(
    r"^\s*FROM\s+(?:--platform=\S+\s+)?(?:docker\.io/(?:library/)?)?python:(\d+)\.(\d+)(?:[^\s]*)?",
    re.IGNORECASE,
)


@dataclass(frozen=True)
class Violation:
    path: str
    line: int
    rule: str
    message: str


def _rel_display(path: pathlib.Path, repo_root: pathlib.Path) -> str:
    try:
        return path.resolve().relative_to(repo_root.resolve()).as_posix()
    except ValueError:
        return path.as_posix()


def extract_requires_python_floor(spec: str) -> tuple[int, int] | None:
    """Extract the minimum (major, minor) floor from a PEP 440 requires-python string."""
    for pattern in REQUIRES_PYTHON_FLOOR_PATTERNS:
        match = pattern.search(spec)
        if match is not None:
            return (int(match.group(1)), int(match.group(2)))
    return None


def _is_unparenthesized_except(tokens: list[tokenize.TokenInfo], start_idx: int) -> bool:
    """Return True if the `except` starting at `start_idx` has a top-level comma before `:`."""
    depth = 0
    idx = start_idx + 1
    if idx < len(tokens) and tokens[idx].type == tokenize.OP and tokens[idx].string == "*":
        idx += 1

    while idx < len(tokens):
        tok = tokens[idx]
        tok_type = tok.type
        tok_str = tok.string
        if tok_type in (tokenize.NL, tokenize.COMMENT, tokenize.INDENT, tokenize.DEDENT):
            idx += 1
            continue
        if tok_type == tokenize.OP:
            if tok_str in ("(", "[", "{"):
                depth += 1
            elif tok_str in (")", "]", "}"):
                depth = max(0, depth - 1)
            elif depth == 0 and tok_str == ",":
                return True
            elif depth == 0 and tok_str == ":":
                return False
        elif depth == 0 and (
            (tok_type == tokenize.NAME and tok_str == "as") or tok_type == tokenize.NEWLINE
        ):
            return False
        idx += 1
    return False


def find_unparenthesized_except_lines(source: str) -> list[int]:
    """Detect PEP 758 unparenthesized multi-exception `except A, B:` lines via tokenization.

    Works identically on Python 3.13 and Python 3.14+ runtimes.
    """
    tokens: list[tokenize.TokenInfo] = []
    try:
        tokens.extend(tokenize.generate_tokens(io.StringIO(source).readline))
    except (tokenize.TokenError, IndentationError):
        return []

    offending_lines: list[int] = []
    seen_lines: set[int] = set()
    for idx, tok in enumerate(tokens):
        if tok.type != tokenize.NAME or tok.string != "except":
            continue
        if _is_unparenthesized_except(tokens, idx):
            line_no = tok.start[0]
            if line_no not in seen_lines:
                seen_lines.add(line_no)
                offending_lines.append(line_no)
    return offending_lines


def check_source_syntax(
    source_path: pathlib.Path,
    floor: tuple[int, int],
    *,
    display_path: str | None = None,
) -> list[Violation]:
    """Verify a Python source file parses cleanly under the declared floor version."""
    rel = display_path if display_path is not None else source_path.as_posix()
    try:
        text = source_path.read_text(encoding="utf-8")
    except (OSError, UnicodeDecodeError) as exc:
        return [
            Violation(
                path=rel,
                line=1,
                rule="source.python.read-error",
                message=f"failed to read Python source: {exc}",
            )
        ]

    violations: list[Violation] = []
    reported_lines: set[int] = set()

    if floor < PEP758_MIN_VERSION:
        for line_no in find_unparenthesized_except_lines(text):
            reported_lines.add(line_no)
            violations.append(
                Violation(
                    path=rel,
                    line=line_no,
                    rule="source.python.syntax-incompatible",
                    message=(
                        f"unparenthesized multi-exception `except` (PEP 758) requires Python 3.14+, "
                        f"incompatible with requires-python floor {floor[0]}.{floor[1]}"
                    ),
                )
            )

    runtime_mm = (sys.version_info.major, sys.version_info.minor)
    if floor > runtime_mm:
        violations.append(
            Violation(
                path=rel,
                line=1,
                rule="source.python.runtime-too-old",
                message=(
                    f"current Python runtime {runtime_mm[0]}.{runtime_mm[1]} is older than "
                    f"requires-python floor {floor[0]}.{floor[1]}; cannot verify AST syntax"
                ),
            )
        )
    else:
        try:
            ast.parse(text, filename=rel, feature_version=floor)
        except SyntaxError as exc:
            line_no = exc.lineno if exc.lineno is not None else 1
            if line_no not in reported_lines:
                violations.append(
                    Violation(
                        path=rel,
                        line=line_no,
                        rule="source.python.syntax-incompatible",
                        message=(
                            f"syntax incompatible with Python {floor[0]}.{floor[1]}: {exc.msg}"
                        ),
                    )
                )

    return violations


def _parse_pyproject(
    pyproject_path: pathlib.Path,
    *,
    repo_root: pathlib.Path,
) -> tuple[dict[str, object] | None, list[Violation]]:
    rel = _rel_display(pyproject_path, repo_root)
    try:
        data = tomllib.loads(pyproject_path.read_text(encoding="utf-8"))
    except (OSError, UnicodeDecodeError, tomllib.TOMLDecodeError) as exc:
        return None, [
            Violation(
                path=rel,
                line=1,
                rule="config.python.pyproject-parse-error",
                message=f"failed to parse pyproject.toml: {exc}",
            )
        ]
    return data, []


def _check_ruff_target_version(
    data: dict[str, object],
    rel: str,
    violations: list[Violation],
) -> None:
    tool_raw = data.get("tool")
    if not isinstance(tool_raw, dict):
        return
    ruff_raw = tool_raw.get("ruff")
    if not isinstance(ruff_raw, dict):
        return

    sections: tuple[tuple[str, object], ...] = (
        ("[tool.ruff]", ruff_raw),
        ("[tool.ruff.lint]", ruff_raw.get("lint")),
        ("[tool.ruff.format]", ruff_raw.get("format")),
    )
    for section_name, section_obj in sections:
        if isinstance(section_obj, dict) and "target-version" in section_obj:
            violations.append(
                Violation(
                    path=rel,
                    line=1,
                    rule="config.python.ruff-target-version-forbidden",
                    message=(
                        f"{section_name}.target-version is forbidden (ARCH-03); "
                        "remove it so Ruff derives target version exclusively from "
                        "[project].requires-python"
                    ),
                )
            )


def _extract_and_validate_floor(
    data: dict[str, object],
    rel: str,
    violations: list[Violation],
) -> tuple[int, int] | None:
    project_raw = data.get("project")
    req_raw = project_raw.get("requires-python") if isinstance(project_raw, dict) else None
    if not isinstance(req_raw, str) or not req_raw.strip():
        violations.append(
            Violation(
                path=rel,
                line=1,
                rule="config.python.requires-python-missing",
                message="[project].requires-python must be declared as the single source of truth",
            )
        )
        return None

    floor = extract_requires_python_floor(req_raw)
    if floor is None:
        violations.append(
            Violation(
                path=rel,
                line=1,
                rule="config.python.requires-python-invalid",
                message=(
                    f"could not extract minimum Python major.minor version from "
                    f"requires-python={req_raw!r}"
                ),
            )
        )
        return None

    if rel == "scripts/pyproject.toml" and floor != SCRIPTS_EXPECTED_FLOOR:
        violations.append(
            Violation(
                path=rel,
                line=1,
                rule="config.python.scripts-requires-python-mismatch",
                message=(
                    f"scripts/pyproject.toml must declare requires-python floor "
                    f"{SCRIPTS_EXPECTED_FLOOR[0]}.{SCRIPTS_EXPECTED_FLOOR[1]} "
                    f"to match host python3, got {req_raw!r}"
                ),
            )
        )
    return floor


def check_pyproject(
    pyproject_path: pathlib.Path,
    *,
    repo_root: pathlib.Path,
) -> list[Violation]:
    """Validate a single pyproject.toml for ARCH-03 compliance."""
    data, parse_violations = _parse_pyproject(pyproject_path, repo_root=repo_root)
    if data is None:
        return parse_violations

    rel = _rel_display(pyproject_path, repo_root)
    violations: list[Violation] = []
    _check_ruff_target_version(data, rel, violations)
    _extract_and_validate_floor(data, rel, violations)
    return violations


def discover_dockerfiles(
    project_dir: pathlib.Path,
    repo_root: pathlib.Path,
) -> list[pathlib.Path]:
    """Find Dockerfile* files belonging to a Python project (including parent dir for */app)."""
    search_dirs = [project_dir.resolve()]
    resolved_root = repo_root.resolve()
    parent_dir = search_dirs[0].parent
    if search_dirs[0].name == "app" and parent_dir != resolved_root and parent_dir.is_dir():
        search_dirs.append(parent_dir)

    found: dict[pathlib.Path, None] = {}
    for directory in search_dirs:
        if not directory.is_dir():
            continue
        for entry in sorted(directory.iterdir(), key=lambda p: p.name):
            if not entry.is_file() or entry.is_symlink():
                continue
            if entry.name == "Dockerfile" or entry.name.startswith("Dockerfile."):
                found[entry] = None
    return list(found.keys())


def check_dockerfiles(
    project_dir: pathlib.Path,
    floor: tuple[int, int],
    *,
    repo_root: pathlib.Path,
) -> list[Violation]:
    """Verify every `FROM python:<major.minor>...` stage matches the project floor."""
    violations: list[Violation] = []
    for dockerfile in discover_dockerfiles(project_dir, repo_root):
        rel = _rel_display(dockerfile, repo_root)
        try:
            lines = dockerfile.read_text(encoding="utf-8").splitlines()
        except (OSError, UnicodeDecodeError) as exc:
            violations.append(
                Violation(
                    path=rel,
                    line=1,
                    rule="config.python.dockerfile-read-error",
                    message=f"failed to read Dockerfile: {exc}",
                )
            )
            continue

        for line_no, raw_line in enumerate(lines, start=1):
            stripped = raw_line.strip()
            if not stripped or stripped.startswith("#"):
                continue
            match = DOCKERFILE_FROM_PYTHON_RE.match(stripped)
            if match is None:
                continue
            df_ver = (int(match.group(1)), int(match.group(2)))
            if df_ver != floor:
                violations.append(
                    Violation(
                        path=rel,
                        line=line_no,
                        rule="config.python.dockerfile-version-mismatch",
                        message=(
                            f"Dockerfile base image python:{df_ver[0]}.{df_ver[1]} does not "
                            f"match pyproject.toml requires-python floor {floor[0]}.{floor[1]}"
                        ),
                    )
                )
    return violations


def _iter_project_python_files(project_dir: pathlib.Path) -> list[pathlib.Path]:
    py_files: list[pathlib.Path] = []
    resolved = project_dir.resolve()
    for dirpath, dirnames, filenames in os.walk(resolved):
        cur = pathlib.Path(dirpath)
        rel_cur = cur.relative_to(resolved).as_posix()
        dirnames[:] = sorted(
            d
            for d in dirnames
            if d not in EXCLUDED_DIR_NAMES and not (rel_cur == "harness" and d == "corpus")
        )
        py_files.extend(cur / fname for fname in sorted(filenames) if fname.endswith(".py"))
    return py_files


def check_project_dir(
    project_dir: pathlib.Path,
    *,
    repo_root: pathlib.Path,
) -> list[Violation]:
    """Validate a Python project directory (pyproject, Dockerfiles, and <3.14 syntax)."""
    pyproject_path = project_dir / "pyproject.toml"
    data, parse_violations = _parse_pyproject(pyproject_path, repo_root=repo_root)
    if data is None:
        return parse_violations

    rel = _rel_display(pyproject_path, repo_root)
    violations: list[Violation] = []
    _check_ruff_target_version(data, rel, violations)
    floor = _extract_and_validate_floor(data, rel, violations)

    if floor is not None:
        violations.extend(check_dockerfiles(project_dir, floor, repo_root=repo_root))
        for py_file in _iter_project_python_files(project_dir):
            violations.extend(
                check_source_syntax(
                    py_file,
                    floor,
                    display_path=_rel_display(py_file, repo_root),
                )
            )
    return violations


def _is_excluded_repo_rel(rel_posix: str) -> bool:
    return any(rel_posix.startswith(prefix) for prefix in EXCLUDED_REPO_PREFIXES)


def discover_repo_pyprojects(repo_root: pathlib.Path) -> list[pathlib.Path]:
    """List tracked pyproject.toml files in repo_root (falling back to walk outside git)."""
    git_bin = shutil.which("git")
    if git_bin is not None and (repo_root / ".git").exists():
        env = os.environ.copy()
        env["GIT_OPTIONAL_LOCKS"] = "0"
        res = subprocess.run(  # noqa: S603 - fixed git executable and arguments
            [git_bin, "-C", str(repo_root), "ls-files", "*pyproject.toml"],
            env=env,
            capture_output=True,
            text=True,
            check=False,
        )
        if res.returncode == 0:
            results: list[pathlib.Path] = []
            for raw_rel in sorted(res.stdout.splitlines()):
                rel = raw_rel.strip()
                if not rel or _is_excluded_repo_rel(rel):
                    continue
                candidate = repo_root / rel
                if candidate.is_file():
                    results.append(candidate)
            return results

    found: list[pathlib.Path] = []
    resolved_root = repo_root.resolve()
    for dirpath, dirnames, filenames in os.walk(resolved_root):
        dirnames[:] = sorted(d for d in dirnames if d not in EXCLUDED_DIR_NAMES)
        for fname in filenames:
            if fname != "pyproject.toml":
                continue
            candidate = pathlib.Path(dirpath) / fname
            rel = candidate.relative_to(resolved_root).as_posix()
            if not _is_excluded_repo_rel(rel):
                found.append(candidate)
    return sorted(found)


def check_repo(repo_root: pathlib.Path) -> list[Violation]:
    """Validate every tracked Python project across the repository."""
    violations: list[Violation] = []
    for pyproject_path in discover_repo_pyprojects(repo_root):
        violations.extend(check_project_dir(pyproject_path.parent, repo_root=repo_root))
    return violations


def _parse_args(argv: list[str] | None) -> argparse.Namespace:
    default_root = pathlib.Path(__file__).resolve().parents[2]
    parser = argparse.ArgumentParser(
        description="Validate single-source Python target version config and Dockerfile parity."
    )
    parser.add_argument(
        "--repo-root",
        type=pathlib.Path,
        default=default_root,
        help="Repository root directory (default: auto-detected).",
    )
    parser.add_argument(
        "--project-dir",
        type=pathlib.Path,
        default=None,
        help="Validate a single project directory (relative to --repo-root or absolute).",
    )
    parser.add_argument(
        "--pyproject",
        type=pathlib.Path,
        default=None,
        help="Validate a specific pyproject.toml file (optionally paired with --source).",
    )
    parser.add_argument(
        "--source",
        type=pathlib.Path,
        action="append",
        default=[],
        help="Python source file to check against --pyproject's requires-python floor (repeatable).",
    )
    return parser.parse_args(argv)


def _format_violation(v: Violation) -> str:
    return f"{v.path}:{v.line}: [{v.rule}] {v.message}"


def _check_explicit_pyproject(
    pyproject_path: pathlib.Path,
    sources: list[pathlib.Path],
    repo_root: pathlib.Path,
) -> tuple[list[Violation], str]:
    if not pyproject_path.is_file():
        return [], f"error: pyproject file not found: {pyproject_path}"

    data, parse_violations = _parse_pyproject(pyproject_path, repo_root=repo_root)
    violations: list[Violation] = list(parse_violations)
    if data is None:
        return violations, ""

    rel = _rel_display(pyproject_path, repo_root)
    _check_ruff_target_version(data, rel, violations)
    floor = _extract_and_validate_floor(data, rel, violations)
    if floor is None:
        return violations, ""

    for src_path in sources:
        if not src_path.is_file():
            return [], f"error: source file not found: {src_path}"
        violations.extend(
            check_source_syntax(
                src_path,
                floor,
                display_path=_rel_display(src_path, repo_root),
            )
        )
    return violations, ""


def _collect_violations(
    args: argparse.Namespace,
    repo_root: pathlib.Path,
) -> tuple[list[Violation], str]:
    if args.pyproject is not None and args.project_dir is not None:
        return [], "error: --pyproject and --project-dir are mutually exclusive"
    if args.source and args.pyproject is None:
        return [], "error: --source requires --pyproject"

    if args.pyproject is not None:
        return _check_explicit_pyproject(args.pyproject, args.source, repo_root)

    if args.project_dir is not None:
        proj_dir = (
            args.project_dir if args.project_dir.is_absolute() else (repo_root / args.project_dir)
        )
        if not proj_dir.is_dir():
            return [], f"error: project directory not found: {proj_dir}"
        return check_project_dir(proj_dir, repo_root=repo_root), ""

    return check_repo(repo_root), ""


REEXEC_ENV_VAR = "CHECK_PYTHON_VERSIONS_REEXEC"


def _extract_floor_quiet(pyproject_path: pathlib.Path) -> tuple[int, int] | None:
    if not pyproject_path.is_file():
        return None
    try:
        data = tomllib.loads(pyproject_path.read_text(encoding="utf-8"))
    except (OSError, UnicodeDecodeError, tomllib.TOMLDecodeError):
        return None
    project_raw = data.get("project")
    req_raw = project_raw.get("requires-python") if isinstance(project_raw, dict) else None
    if not isinstance(req_raw, str):
        return None
    return extract_requires_python_floor(req_raw)


def _highest_required_floor(
    args: argparse.Namespace,
    repo_root: pathlib.Path,
) -> tuple[int, int] | None:
    if args.pyproject is not None:
        if not args.source:
            return None
        return _extract_floor_quiet(args.pyproject)
    if args.project_dir is not None:
        proj_dir = (
            args.project_dir if args.project_dir.is_absolute() else (repo_root / args.project_dir)
        )
        return _extract_floor_quiet(proj_dir / "pyproject.toml")

    floors = [
        floor
        for pp in discover_repo_pyprojects(repo_root)
        if (floor := _extract_floor_quiet(pp)) is not None
    ]
    return max(floors) if floors else None


def _find_python_for_floor(required: tuple[int, int]) -> pathlib.Path | None:
    name = f"python{required[0]}.{required[1]}"
    on_path = shutil.which(name)
    if on_path is not None:
        return pathlib.Path(on_path)
    local_bin = pathlib.Path.home() / ".local" / "bin" / name
    if local_bin.is_file() and os.access(local_bin, os.X_OK):
        return local_bin
    return None


def _maybe_reexec_for_floor(
    args: argparse.Namespace,
    argv: list[str] | None,
    repo_root: pathlib.Path,
) -> int | None:
    required = _highest_required_floor(args, repo_root)
    runtime_mm = (sys.version_info.major, sys.version_info.minor)
    if required is None or required <= runtime_mm:
        return None

    if os.environ.get(REEXEC_ENV_VAR) == "1":
        sys.stderr.write(
            f"error: Python {runtime_mm[0]}.{runtime_mm[1]} is older than required floor "
            f"{required[0]}.{required[1]} even after re-exec; cannot verify AST syntax.\n"
        )
        return EXIT_VIOLATIONS

    alt_py = _find_python_for_floor(required)
    if alt_py is None:
        sys.stderr.write(
            f"error: Python {runtime_mm[0]}.{runtime_mm[1]} is older than required floor "
            f"{required[0]}.{required[1]} and python{required[0]}.{required[1]} was not found "
            "in PATH or ~/.local/bin; refusing to skip AST syntax verification.\n"
        )
        return EXIT_VIOLATIONS

    child_env = os.environ.copy()
    child_env[REEXEC_ENV_VAR] = "1"
    forward_argv = list(argv) if argv is not None else sys.argv[1:]
    res = subprocess.run(  # noqa: S603 - re-execing this script under discovered newer Python
        [str(alt_py), str(pathlib.Path(__file__).resolve()), *forward_argv],
        env=child_env,
        check=False,
    )
    return res.returncode


def main(argv: list[str] | None = None) -> int:
    args = _parse_args(argv)
    repo_root = args.repo_root.resolve()
    reexec_rc = _maybe_reexec_for_floor(args, argv, repo_root)
    if reexec_rc is not None:
        return reexec_rc

    violations, usage_error = _collect_violations(args, repo_root)
    if usage_error:
        sys.stderr.write(f"{usage_error}\n")
        return EXIT_USAGE_ERROR

    if violations:
        for v in violations:
            sys.stderr.write(f"{_format_violation(v)}\n")
        sys.stderr.write(f"\nFound {len(violations)} Python version configuration violation(s).\n")
        return EXIT_VIOLATIONS

    sys.stdout.write("OK: Python version configuration check passed.\n")
    return EXIT_CLEAN


if __name__ == "__main__":
    raise SystemExit(main())
