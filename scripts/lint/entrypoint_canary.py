#!/usr/bin/env python3
"""End-to-end isolated entrypoint canary for `scripts/lint/run.sh` (CHECK-04).

Stages an isolated temporary workspace copy for 1 real repository target per
language family (Go, Python, Rust, Frontend, Deno, Proto), verifies the
unmutated staged baseline passes, injects 1 deterministic lint violation into
each staged target, invokes `bash <staged_root>/scripts/lint/run.sh <target>`,
and asserts BOTH:
1. `run.sh` exits non-zero (`returncode != 0`), AND
2. The expected scanner canary passed AND the exact injected violation
   diagnostic appears in the output (confirming failure for the right reason,
   not a broken staging environment).

The real repository worktree is never mutated.

Run:
    python3 scripts/lint/entrypoint_canary.py
    python3 scripts/lint/entrypoint_canary.py --languages go,python,deno
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
import time
from collections.abc import Callable
from dataclasses import dataclass

EXIT_OK = 0
EXIT_FAIL = 1

ANSI_ESCAPE_RE = re.compile(r"\x1b\[[0-9;]*[A-Za-z]")
DOCKERFILE_BUN_PIN_RE = re.compile(r"(^FROM\s+oven/bun:)([0-9][0-9.]*)(-.*)?$", re.MULTILINE)
OUTPUT_SNIPPET_LIMIT = 800
QUICK_SUBPROCESS_TIMEOUT_S = 15
BASELINE_SUBPROCESS_TIMEOUT_S = 180
LINT_SUBPROCESS_TIMEOUT_S = 300

ALL_LANGUAGES = ("go", "python", "rust", "frontend", "deno", "proto")

COPY_IGNORE = shutil.ignore_patterns(
    ".git",
    ".venv",
    "venv",
    "node_modules",
    "target",
    "_reports",
    ".svelte-kit",
    "build",
    "dist",
    "playwright-report",
    "test-results",
    "__pycache__",
    ".ruff_cache",
    ".mypy_cache",
    ".pytest_cache",
)


@dataclass(frozen=True)
class CaseResult:
    language: str
    target: str
    passed: bool
    duration_s: float
    detail: str


def _strip_ansi(text: str) -> str:
    return ANSI_ESCAPE_RE.sub("", text)


def _snippet(text: str) -> str:
    clean = _strip_ansi(text)
    if len(clean) <= OUTPUT_SNIPPET_LIMIT:
        return clean.strip()
    return f"...{clean[-OUTPUT_SNIPPET_LIMIT:].strip()}"


def _find_main_worktree(repo_root: pathlib.Path) -> pathlib.Path | None:
    """Return the primary git worktree root when invoked from a secondary worktree."""
    git_bin = shutil.which("git")
    if git_bin is None:
        return None
    res = subprocess.run(  # noqa: S603 - fixed argv querying local git worktrees
        [git_bin, "-C", str(repo_root), "worktree", "list", "--porcelain"],
        capture_output=True,
        text=True,
        check=False,
        timeout=QUICK_SUBPROCESS_TIMEOUT_S,
    )
    if res.returncode != 0:
        return None
    for line in res.stdout.splitlines():
        if line.startswith("worktree "):
            candidate = pathlib.Path(line.removeprefix("worktree ").strip())
            if candidate.is_dir():
                return candidate
    return None


def _stage_common(repo_root: pathlib.Path, staged_root: pathlib.Path) -> None:
    shutil.copytree(
        repo_root / "scripts" / "lint",
        staged_root / "scripts" / "lint",
        ignore=COPY_IGNORE,
    )
    golangci = repo_root / ".golangci.yml"
    if golangci.is_file():
        shutil.copy2(golangci, staged_root / ".golangci.yml")


def _stage_target_dir(
    repo_root: pathlib.Path, staged_root: pathlib.Path, target: str
) -> pathlib.Path:
    src = repo_root / target
    dst = staged_root / target
    shutil.copytree(src, dst, ignore=COPY_IGNORE)
    return dst


def _run_staged_lint(
    staged_root: pathlib.Path,
    target: str,
    *,
    extra_env: dict[str, str] | None = None,
) -> subprocess.CompletedProcess[str]:
    bash_bin = shutil.which("bash")
    if bash_bin is None:
        msg = "bash executable not found in PATH"
        raise RuntimeError(msg)
    env = os.environ.copy()
    if extra_env:
        env.update(extra_env)
    return subprocess.run(  # noqa: S603 - fixed argv invoking staged scripts/lint/run.sh
        [bash_bin, str(staged_root / "scripts" / "lint" / "run.sh"), target],
        cwd=str(staged_root),
        env=env,
        capture_output=True,
        text=True,
        check=False,
        timeout=LINT_SUBPROCESS_TIMEOUT_S,
    )


def _assert_markers(output: str, markers: tuple[str, ...]) -> list[str]:
    clean = _strip_ansi(output)
    return [marker for marker in markers if marker not in clean]


def _run_go_case(repo_root: pathlib.Path, *, verify_baseline: bool) -> CaseResult:
    target = "docker/otel-relay"
    start = time.monotonic()
    with tempfile.TemporaryDirectory(prefix="alt-ep-go-") as td:
        staged_root = pathlib.Path(td)
        _stage_common(repo_root, staged_root)
        target_dir = _stage_target_dir(repo_root, staged_root, target)

        if verify_baseline:
            base_res = _run_staged_lint(staged_root, target)
            if base_res.returncode != 0:
                return CaseResult(
                    "go",
                    target,
                    False,
                    time.monotonic() - start,
                    f"baseline failed (rc={base_res.returncode}): {_snippet(base_res.stdout + base_res.stderr)}",
                )

        violation_file = target_dir / "entrypoint_canary_violation.go"
        violation_file.write_text(
            'package main\n\nimport "crypto/tls"\n\n'
            "func entrypointCanaryViolation() *tls.Config {\n"
            "    x:=1\n"
            "\tx = 2\n"
            "\t_ = x\n"
            "\treturn &tls.Config{InsecureSkipVerify: true}\n"
            "}\n",
            encoding="utf-8",
        )

        mut_res = _run_staged_lint(staged_root, target)
        combined = f"{mut_res.stdout}\n{mut_res.stderr}"
        missing = _assert_markers(
            combined,
            (
                "canary audit passed (go)",
                "Run gofmt -w on these files:",
                "entrypoint_canary_violation.go",
                "G402",
            ),
        )
        if mut_res.returncode == 0 or missing:
            return CaseResult(
                "go",
                target,
                False,
                time.monotonic() - start,
                f"expected rc!=0 and markers (missing={missing}, rc={mut_res.returncode}): {_snippet(combined)}",
            )

    return CaseResult(
        "go",
        target,
        True,
        time.monotonic() - start,
        "baseline pass + gofmt and G402/ineffassign caught",
    )


def _run_python_case(repo_root: pathlib.Path, *, verify_baseline: bool) -> CaseResult:
    target = "rerank-server"
    start = time.monotonic()
    with tempfile.TemporaryDirectory(prefix="alt-ep-python-") as td:
        td_path = pathlib.Path(td)
        staged_root = td_path / "repo"
        staged_root.mkdir()
        _stage_common(repo_root, staged_root)
        target_dir = _stage_target_dir(repo_root, staged_root, target)
        extra_env = {
            "UV_PROJECT_ENVIRONMENT": str(td_path / "venv"),
            "UV_LINK_MODE": "copy",
        }

        if verify_baseline:
            base_res = _run_staged_lint(staged_root, target, extra_env=extra_env)
            if base_res.returncode != 0:
                return CaseResult(
                    "python",
                    target,
                    False,
                    time.monotonic() - start,
                    f"baseline failed (rc={base_res.returncode}): {_snippet(base_res.stdout + base_res.stderr)}",
                )

        violation_file = target_dir / "entrypoint_canary_violation.py"
        violation_file.write_text(
            '"""Entrypoint canary injected violation."""\n\n'
            "import os\n\n\n"
            "def entrypoint_canary_violation() -> int:\n"
            '    bad_value: int = "not_an_int"\n'
            "    return bad_value\n",
            encoding="utf-8",
        )

        mut_res = _run_staged_lint(staged_root, target, extra_env=extra_env)
        combined = f"{mut_res.stdout}\n{mut_res.stderr}"
        missing = _assert_markers(
            combined,
            (
                "canary audit passed (python)",
                "entrypoint_canary_violation.py",
                "F401",
                "bad-assignment",
            ),
        )
        if mut_res.returncode == 0 or missing:
            return CaseResult(
                "python",
                target,
                False,
                time.monotonic() - start,
                f"expected rc!=0 and markers (missing={missing}, rc={mut_res.returncode}): {_snippet(combined)}",
            )

    return CaseResult(
        "python",
        target,
        True,
        time.monotonic() - start,
        "baseline pass + F401/bad-assignment caught",
    )


def _run_rust_case(repo_root: pathlib.Path, *, verify_baseline: bool) -> CaseResult:
    target = "plecto"
    start = time.monotonic()
    cache_root = pathlib.Path(os.environ.get("XDG_CACHE_HOME", pathlib.Path.home() / ".cache"))
    shared_target_dir = cache_root / "alt-lint" / "entrypoint-canary-rust-target"
    shared_target_dir.mkdir(parents=True, exist_ok=True)
    extra_env = {"CARGO_TARGET_DIR": str(shared_target_dir)}

    with tempfile.TemporaryDirectory(prefix="alt-ep-rust-") as td:
        staged_root = pathlib.Path(td)
        _stage_common(repo_root, staged_root)
        target_dir = _stage_target_dir(repo_root, staged_root, target)

        if verify_baseline:
            base_res = _run_staged_lint(staged_root, target, extra_env=extra_env)
            if base_res.returncode != 0:
                return CaseResult(
                    "rust",
                    target,
                    False,
                    time.monotonic() - start,
                    f"baseline failed (rc={base_res.returncode}): {_snippet(base_res.stdout + base_res.stderr)}",
                )

        lib_rs = target_dir / "src" / "lib.rs"
        original = lib_rs.read_text(encoding="utf-8")
        lib_rs.write_text(
            original.rstrip()
            + "\n\npub fn entrypoint_canary_violation(x: i32) -> bool {\n    x == x\n}\n",
            encoding="utf-8",
        )

        mut_res = _run_staged_lint(staged_root, target, extra_env=extra_env)
        combined = f"{mut_res.stdout}\n{mut_res.stderr}"
        missing = _assert_markers(
            combined,
            (
                "canary audit passed (rust)",
                "eq_op",
                "clippy: fail",
            ),
        )
        if mut_res.returncode == 0 or missing:
            return CaseResult(
                "rust",
                target,
                False,
                time.monotonic() - start,
                f"expected rc!=0 and markers (missing={missing}, rc={mut_res.returncode}): {_snippet(combined)}",
            )

    return CaseResult(
        "rust", target, True, time.monotonic() - start, "baseline pass + clippy::eq_op caught"
    )


def _resolve_frontend_node_modules(repo_root: pathlib.Path, target: str) -> pathlib.Path | None:
    direct = repo_root / target / "node_modules"
    if direct.is_dir():
        return direct
    main_wt = _find_main_worktree(repo_root)
    if main_wt is not None:
        fallback = main_wt / target / "node_modules"
        if fallback.is_dir():
            return fallback
    return None


def _align_staged_bun_pin_if_needed(dockerfile: pathlib.Path) -> str | None:
    """Align only the ephemeral staged Dockerfile pin if host bun differs from Dockerfile."""
    bun_bin = shutil.which("bun")
    if bun_bin is None or not dockerfile.is_file():
        return None
    res = subprocess.run(  # noqa: S603 - fixed argv querying local bun version
        [bun_bin, "--version"],
        capture_output=True,
        text=True,
        check=False,
        timeout=QUICK_SUBPROCESS_TIMEOUT_S,
    )
    if res.returncode != 0:
        return None
    host_ver = res.stdout.strip()
    if not host_ver:
        return None
    content = dockerfile.read_text(encoding="utf-8")
    match = DOCKERFILE_BUN_PIN_RE.search(content)
    if match is None or match.group(2) == host_ver:
        return None
    pinned_ver = match.group(2)
    updated = DOCKERFILE_BUN_PIN_RE.sub(
        lambda m: f"{m.group(1)}{host_ver}{m.group(3) or ''}",
        content,
    )
    dockerfile.write_text(updated, encoding="utf-8")
    return f"staged Dockerfile bun pin aligned {pinned_ver}->{host_ver} for local host bun"


def _run_frontend_case(repo_root: pathlib.Path, *, verify_baseline: bool) -> CaseResult:
    target = "alt-frontend-sv"
    start = time.monotonic()
    node_modules = _resolve_frontend_node_modules(repo_root, target)
    if node_modules is None:
        return CaseResult(
            "frontend",
            target,
            False,
            time.monotonic() - start,
            f"missing {target}/node_modules (run `bun install --frozen-lockfile` in {target})",
        )

    bun_bin = shutil.which("bun")
    if bun_bin is None:
        return CaseResult(
            "frontend",
            target,
            False,
            time.monotonic() - start,
            "bun executable not found in PATH",
        )

    with tempfile.TemporaryDirectory(prefix="alt-ep-frontend-") as td:
        staged_root = pathlib.Path(td)
        _stage_common(repo_root, staged_root)
        target_dir = _stage_target_dir(repo_root, staged_root, target)
        (target_dir / "node_modules").symlink_to(node_modules)
        pin_note = _align_staged_bun_pin_if_needed(target_dir / "Dockerfile")

        if verify_baseline:
            base_res = subprocess.run(  # noqa: S603 - fixed argv running staged Biome baseline check
                [bun_bin, "run", "lint:check"],
                cwd=str(target_dir),
                capture_output=True,
                text=True,
                check=False,
                timeout=BASELINE_SUBPROCESS_TIMEOUT_S,
            )
            if base_res.returncode != 0:
                return CaseResult(
                    "frontend",
                    target,
                    False,
                    time.monotonic() - start,
                    f"baseline lint:check failed (rc={base_res.returncode}): {_snippet(base_res.stdout + base_res.stderr)}",
                )

        violation_file = target_dir / "src" / "lib" / "entrypoint_canary_violation.ts"
        violation_file.write_text(
            "export function entrypointCanaryViolation(value: number): number {\n"
            "\tdebugger;\n"
            "\treturn value;\n"
            "}\n",
            encoding="utf-8",
        )

        mut_res = _run_staged_lint(staged_root, target)
        combined = f"{mut_res.stdout}\n{mut_res.stderr}"
        missing = _assert_markers(
            combined,
            (
                "canary audit passed (frontend)",
                "entrypoint_canary_violation.ts",
                "noDebugger",
                "lint:check: fail",
            ),
        )
        if mut_res.returncode == 0 or missing:
            return CaseResult(
                "frontend",
                target,
                False,
                time.monotonic() - start,
                f"expected rc!=0 and markers (missing={missing}, rc={mut_res.returncode}): {_snippet(combined)}",
            )

    suffix = f" ({pin_note})" if pin_note else ""
    return CaseResult(
        "frontend",
        target,
        True,
        time.monotonic() - start,
        f"baseline lint:check pass + Biome noDebugger caught{suffix}",
    )


def _run_deno_case(repo_root: pathlib.Path, *, verify_baseline: bool) -> CaseResult:
    target = "auth-token-manager"
    start = time.monotonic()
    with tempfile.TemporaryDirectory(prefix="alt-ep-deno-") as td:
        staged_root = pathlib.Path(td)
        _stage_common(repo_root, staged_root)
        target_dir = _stage_target_dir(repo_root, staged_root, target)

        if verify_baseline:
            base_res = _run_staged_lint(staged_root, target)
            if base_res.returncode != 0:
                return CaseResult(
                    "deno",
                    target,
                    False,
                    time.monotonic() - start,
                    f"baseline failed (rc={base_res.returncode}): {_snippet(base_res.stdout + base_res.stderr)}",
                )

        violation_file = target_dir / "src" / "entrypoint_canary_violation.ts"
        violation_file.write_text(
            "export function entrypointCanaryViolation(value: number): number {\n"
            "  debugger;\n"
            "  return value;\n"
            "}\n",
            encoding="utf-8",
        )

        mut_res = _run_staged_lint(staged_root, target)
        combined = f"{mut_res.stdout}\n{mut_res.stderr}"
        missing = _assert_markers(
            combined,
            (
                "canary audit passed (deno)",
                "entrypoint_canary_violation.ts",
                "no-debugger",
                "deno lint: fail",
            ),
        )
        if mut_res.returncode == 0 or missing:
            return CaseResult(
                "deno",
                target,
                False,
                time.monotonic() - start,
                f"expected rc!=0 and markers (missing={missing}, rc={mut_res.returncode}): {_snippet(combined)}",
            )

    return CaseResult(
        "deno", target, True, time.monotonic() - start, "baseline pass + deno no-debugger caught"
    )


def _run_proto_case(repo_root: pathlib.Path, *, verify_baseline: bool) -> CaseResult:
    target = "proto"
    start = time.monotonic()
    with tempfile.TemporaryDirectory(prefix="alt-ep-proto-") as td:
        staged_root = pathlib.Path(td)
        _stage_common(repo_root, staged_root)
        target_dir = _stage_target_dir(repo_root, staged_root, target)
        for rel in (
            "alt-butterfly-facade/internal/server/allowlist_gen.go",
            "alt-frontend-sv/src/lib/gen/allowlist.ts",
        ):
            src_file = repo_root / rel
            if src_file.is_file():
                dst_file = staged_root / rel
                dst_file.parent.mkdir(parents=True, exist_ok=True)
                shutil.copy2(src_file, dst_file)

        if verify_baseline:
            base_res = _run_staged_lint(staged_root, target)
            if base_res.returncode != 0:
                return CaseResult(
                    "proto",
                    target,
                    False,
                    time.monotonic() - start,
                    f"baseline failed (rc={base_res.returncode}): {_snippet(base_res.stdout + base_res.stderr)}",
                )

        violation_file = target_dir / "entrypoint_canary_violation.proto"
        violation_file.write_text(
            'syntax = "proto3";\n\n'
            "package badpkg;\n\n"
            "enum bad_entrypoint_enum {\n"
            "  bad_entrypoint_value = 0;\n"
            "}\n",
            encoding="utf-8",
        )

        mut_res = _run_staged_lint(staged_root, target)
        combined = f"{mut_res.stdout}\n{mut_res.stderr}"
        missing = _assert_markers(
            combined,
            (
                "canary audit passed (proto)",
                "entrypoint_canary_violation.proto",
                "FAIL: buf-lint",
            ),
        )
        if mut_res.returncode == 0 or missing:
            return CaseResult(
                "proto",
                target,
                False,
                time.monotonic() - start,
                f"expected rc!=0 and markers (missing={missing}, rc={mut_res.returncode}): {_snippet(combined)}",
            )

    return CaseResult(
        "proto", target, True, time.monotonic() - start, "baseline pass + buf lint STANDARD caught"
    )


CASE_RUNNERS: dict[str, Callable[..., CaseResult]] = {
    "go": _run_go_case,
    "python": _run_python_case,
    "rust": _run_rust_case,
    "frontend": _run_frontend_case,
    "deno": _run_deno_case,
    "proto": _run_proto_case,
}


def _is_language_available(repo_root: pathlib.Path, lang: str) -> bool:
    checks: dict[str, Callable[[], bool]] = {
        "go": lambda: shutil.which("go") is not None and shutil.which("jq") is not None,
        "python": lambda: shutil.which("uv") is not None,
        "rust": lambda: shutil.which("cargo") is not None,
        "frontend": lambda: (
            shutil.which("bun") is not None
            and _resolve_frontend_node_modules(repo_root, "alt-frontend-sv") is not None
        ),
        "deno": lambda: shutil.which("deno") is not None,
        "proto": lambda: shutil.which("buf") is not None and shutil.which("go") is not None,
    }
    probe = checks.get(lang)
    return probe() if probe is not None else False


def parse_args(argv: list[str] | None = None) -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="End-to-end isolated entrypoint canary for scripts/lint/run.sh.",
    )
    parser.add_argument(
        "--repo-root",
        type=pathlib.Path,
        default=pathlib.Path(__file__).resolve().parents[2],
        help="Repository root directory (defaults to auto-detected repo root).",
    )
    parser.add_argument(
        "--languages",
        type=str,
        default=",".join(ALL_LANGUAGES),
        help="Comma-separated subset of language families to verify.",
    )
    parser.add_argument(
        "--skip-baseline",
        action="store_true",
        help="Skip unmutated baseline run before injecting violations.",
    )
    parser.add_argument(
        "--available-only",
        action="store_true",
        help="Run only requested language families whose toolchain binaries are present on PATH.",
    )
    return parser.parse_args(argv)


def main(argv: list[str] | None = None) -> int:
    args = parse_args(argv)
    repo_root = args.repo_root.resolve()
    languages = [tok.strip().lower() for tok in args.languages.split(",") if tok.strip()]
    unknown = [lang for lang in languages if lang not in CASE_RUNNERS]
    if unknown:
        sys.stderr.write(
            f"ERROR: unknown language(s): {unknown} (expected subset of {list(ALL_LANGUAGES)})\n"
        )
        return 2

    if args.available_only:
        selected = [lang for lang in languages if _is_language_available(repo_root, lang)]
        skipped = [lang for lang in languages if lang not in selected]
        if skipped:
            sys.stdout.write(f"  SKIP  unavailable language toolchain(s): {', '.join(skipped)}\n")
        if not selected:
            sys.stderr.write("ERROR: --available-only matched 0 available language toolchains\n")
            return EXIT_FAIL
        languages = selected

    results: list[CaseResult] = []
    for lang in languages:
        runner = CASE_RUNNERS[lang]
        res = runner(repo_root, verify_baseline=not args.skip_baseline)
        results.append(res)
        status = "PASS" if res.passed else "FAIL"
        sys.stdout.write(
            f"  {status}  [{res.language:8s}] {res.target:22s} ({res.duration_s:5.2f}s) {res.detail}\n"
        )
        sys.stdout.flush()

    failed = [r for r in results if not r.passed]
    sys.stdout.write(
        f"\nentrypoint canary summary: {len(results) - len(failed)}/{len(results)} passed\n"
    )
    return EXIT_OK if not failed else EXIT_FAIL


if __name__ == "__main__":
    sys.exit(main())
