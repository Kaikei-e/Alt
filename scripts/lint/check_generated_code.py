#!/usr/bin/env python3
"""Verify generated code provenance, generator version pins, and tamper resistance (CHECK-05).

Implements REPO-04 concretization:
1. Dynamic `Code generated ... DO NOT EDIT` header discovery (`--list-generated`)
   across tracked repository files (never hardcoded).
2. Generator pin & `@latest` audit (`--check-pins`):
   - Verifies `scripts/codegen/go.tool.mod` declares `tool (...)` directives with
     pinned `vX.Y.Z` versions in `require` and contains no `@latest`.
   - Verifies every `proto/buf.gen*.yaml` `remote:` plugin carries an explicit
     `:v<version>` tag.
   - Scans build/CI/Makefile/script/Dockerfile/runbook/best-practice files for
     unpinned `@latest` tool invocations.
3. Deterministic local regeneration drift checks (`--check-regen`, included in default):
   - `scripts/codegen/sync_go_canonical.py --check`
   - `scripts/codegen/pki_py/sync_python_pki.py --check`
   - `rag-orchestrator/spec/openapi.yaml` via `go tool -modfile=scripts/codegen/go.tool.mod oapi-codegen`
     compared byte-for-byte against committed `server.gen.go` and `client.gen.go`
   - `protovis` allowlist generation check (when `buf` is available on PATH)
4. Tamper detection for pair diffs (`--before <file> --after <file>`) and git diffs
   (`--base <rev> [--head <rev>]`), capturing CORPUS-01 `C-16` with signal
   `[codegen.do-not-edit-modified]`.

Usage:
    python3 scripts/lint/check_generated_code.py
    python3 scripts/lint/check_generated_code.py --list-generated
    python3 scripts/lint/check_generated_code.py --check-pins
    python3 scripts/lint/check_generated_code.py --check-regen
    python3 scripts/lint/check_generated_code.py --before before.go --after after.go
    python3 scripts/lint/check_generated_code.py --base HEAD~1
"""

from __future__ import annotations

import argparse
import pathlib
import re
import shutil
import subprocess
import sys
import tempfile
from dataclasses import dataclass

EXIT_CLEAN = 0
EXIT_VIOLATIONS = 1
EXIT_USAGE_ERROR = 2

HEADER_SCAN_LINES = 40
SUBPROCESS_TIMEOUT_SECONDS = 120

EXCLUDED_REPO_PREFIXES = ("stopped-using-k8s/", "scripts/harness/corpus/")
EXCLUDED_DIR_NAMES = frozenset(
    {
        ".git",
        ".venv",
        "__pycache__",
        "build",
        "dist",
        "node_modules",
        "target",
        "vendor",
    }
)

GENERATED_HEADER_RE = re.compile(
    r"^\s*(?://+|#+|/\*|\*|--|<!--|;)\s*Code generated .* DO NOT EDIT\.?\s*(?:\*/|-->)?\s*$",
    re.IGNORECASE,
)
MOCKGEN_SOURCE_RE = re.compile(r"^\s*//\s*Source:\s*(\S+)", re.IGNORECASE)
BUF_REMOTE_RE = re.compile(
    r"""(?:^\s*(?:-\s*)?|[{,]\s*)remote\s*:\s*(?:"([^"]+)"|'([^']+)'|([^\s,}\]]+))"""
)
BUF_PINNED_TAG_RE = re.compile(r":v\d+(?:\.\d+)*$")
SEMVER_RE = re.compile(r"^v\d+\.\d+\.\d+(?:[-+][^\s]+)?$")
AT_LATEST_RE = re.compile(r"(?:^|[\s\"'(=`])[^\s\"'(=`]+@latest(?=$|[\s\"'`);,|&\\<>])")
GO_GENERATE_LINE_RE = re.compile(r"^\s*//\s*go:generate\b")
PROTO_GEN_SUFFIXES: tuple[str, ...] = (
    ".pb.go",
    ".connect.go",
    "_pb2.py",
    "_pb2.pyi",
    "_connect.py",
    "_pb.ts",
    "_connect.ts",
)

OPENAPI_TARGETS: tuple[tuple[str, tuple[str, ...]], ...] = (
    (
        "rag-orchestrator/internal/adapter/rag_http/openapi/server.gen.go",
        ("-package", "openapi", "-generate", "types,echo5-server,spec"),
    ),
    (
        "alt-backend/app/orchestrator/gateway/rag_gateway/client.gen.go",
        (
            "-package",
            "rag_gateway",
            "-generate",
            "types,client",
            "-exclude-operation-ids",
            "CandidateSimilarity",
        ),
    ),
)

PROTOVIS_OUTPUTS: tuple[str, ...] = (
    "alt-butterfly-facade/internal/server/allowlist_gen.go",
    "alt-frontend-sv/src/lib/gen/allowlist.ts",
)


@dataclass(frozen=True)
class Violation:
    path: str
    line: int
    rule: str
    message: str


def default_repo_root() -> pathlib.Path:
    return pathlib.Path(__file__).resolve().parents[2]


def _is_excluded_rel(rel_posix: str) -> bool:
    if rel_posix.startswith(EXCLUDED_REPO_PREFIXES):
        return True
    parts = pathlib.PurePosixPath(rel_posix).parts
    return any(part in EXCLUDED_DIR_NAMES for part in parts)


def _is_safe_worktree_file(repo_root_resolved: pathlib.Path, full_path: pathlib.Path) -> bool:
    if full_path.is_symlink():
        return False
    if not full_path.is_file():
        return False
    try:
        return full_path.resolve().is_relative_to(repo_root_resolved)
    except OSError:
        return False


def _list_repo_files(repo_root: pathlib.Path) -> list[str]:
    git_bin = shutil.which("git")
    if git_bin is not None and (repo_root / ".git").exists():
        res = _run_subprocess([git_bin, "-C", str(repo_root), "ls-files"])
        if res.returncode == 0:
            files = [
                line.strip()
                for line in res.stdout.splitlines()
                if line.strip() and not _is_excluded_rel(line.strip())
            ]
            return sorted(files)

    found: list[str] = []
    for path in repo_root.rglob("*"):
        if path.is_symlink() or not path.is_file():
            continue
        rel = path.relative_to(repo_root).as_posix()
        if not _is_excluded_rel(rel):
            found.append(rel)
    return sorted(found)


def extract_generated_header(text: str) -> str | None:
    """Return the first matching `Code generated ... DO NOT EDIT` header in the first 40 lines."""
    for line in text.splitlines()[:HEADER_SCAN_LINES]:
        if GENERATED_HEADER_RE.match(line):
            return line.strip()
    return None


def _file_has_generated_header(path: pathlib.Path) -> bool:
    try:
        with path.open("r", encoding="utf-8", errors="ignore") as handle:
            for idx, line in enumerate(handle):
                if idx >= HEADER_SCAN_LINES:
                    break
                if GENERATED_HEADER_RE.match(line):
                    return True
    except OSError:
        return False
    return False


def discover_generated_files(repo_root: pathlib.Path) -> list[str]:
    """Dynamically discover all tracked files with a `DO NOT EDIT` header in their first 40 lines."""
    repo_root_resolved = repo_root.resolve()
    generated: list[str] = []
    for rel in _list_repo_files(repo_root):
        full_path = repo_root / rel
        if _is_safe_worktree_file(repo_root_resolved, full_path) and _file_has_generated_header(
            full_path
        ):
            generated.append(rel)
    return sorted(generated)


def _parse_block_entry(
    in_block: str,
    line: str,
    tools: list[str],
    requires: dict[str, str],
) -> None:
    parts = line.split()
    if in_block == "tool" and parts:
        tools.append(parts[0])
    elif in_block == "require" and len(parts) >= 2:  # noqa: PLR2004 - `<module> <version>`
        requires[parts[0]] = parts[1]


def _parse_go_tool_mod(text: str) -> tuple[list[str], dict[str, str]]:
    tools: list[str] = []
    requires: dict[str, str] = {}
    in_block: str | None = None

    for raw_line in text.splitlines():
        line = raw_line.split("//", 1)[0].strip()
        if not line:
            continue
        if in_block is not None:
            if line == ")":
                in_block = None
            else:
                _parse_block_entry(in_block, line, tools, requires)
            continue

        if line.startswith("tool ("):
            in_block = "tool"
        elif line.startswith("require ("):
            in_block = "require"
        elif line.startswith("tool "):
            parts = line.split()
            if len(parts) >= 2:  # noqa: PLR2004 - `tool <pkg>`
                tools.append(parts[1])
        elif line.startswith("require "):
            parts = line.split()
            if len(parts) >= 3:  # noqa: PLR2004 - `require <module> <version>`
                requires[parts[1]] = parts[2]

    return tools, requires


def _tool_has_pinned_require(tool_pkg: str, requires: dict[str, str]) -> bool:
    for mod_path, version in requires.items():
        if (tool_pkg == mod_path or tool_pkg.startswith(f"{mod_path}/")) and SEMVER_RE.match(
            version
        ):
            return True
    return False


def check_go_tool_mod(repo_root: pathlib.Path) -> list[Violation]:
    tool_mod_rel = "scripts/codegen/go.tool.mod"
    tool_mod = repo_root / tool_mod_rel
    if not tool_mod.is_file():
        return [
            Violation(
                path=tool_mod_rel,
                line=1,
                rule="codegen.missing-tool-mod",
                message="missing pinned Go tool manifest scripts/codegen/go.tool.mod",
            )
        ]

    text = tool_mod.read_text(encoding="utf-8")
    violations: list[Violation] = []
    for line_no, line in enumerate(text.splitlines(), start=1):
        if "@latest" in line:
            violations.append(
                Violation(
                    path=tool_mod_rel,
                    line=line_no,
                    rule="codegen.unpinned-latest",
                    message="go.tool.mod must not reference @latest",
                )
            )

    tools, requires = _parse_go_tool_mod(text)
    if not tools:
        violations.append(
            Violation(
                path=tool_mod_rel,
                line=1,
                rule="codegen.invalid-tool-mod",
                message="go.tool.mod must declare at least one `tool` directive",
            )
        )
        return violations

    violations.extend(
        Violation(
            path=tool_mod_rel,
            line=1,
            rule="codegen.invalid-tool-mod",
            message=f"tool `{tool_pkg}` lacks a pinned semver `require` entry in go.tool.mod",
        )
        for tool_pkg in tools
        if not _tool_has_pinned_require(tool_pkg, requires)
    )

    return violations


def _strip_yaml_comment(raw: str) -> str:
    in_quote: str | None = None
    escaped = False
    for idx, ch in enumerate(raw):
        if escaped:
            escaped = False
            continue
        if in_quote == '"' and ch == "\\":
            escaped = True
            continue
        if in_quote is not None:
            if ch == in_quote:
                in_quote = None
            continue
        if ch in ('"', "'"):
            in_quote = ch
        elif ch == "#" and (idx == 0 or raw[idx - 1].isspace()):
            return raw[:idx].rstrip()
    return raw.rstrip()


def check_buf_gen_pins(repo_root: pathlib.Path) -> list[Violation]:
    proto_dir = repo_root / "proto"
    if not proto_dir.is_dir():
        return []

    violations: list[Violation] = []
    for cfg_path in sorted(proto_dir.glob("buf.gen*.yaml")):
        rel = cfg_path.relative_to(repo_root).as_posix()
        for line_no, raw_line in enumerate(
            cfg_path.read_text(encoding="utf-8").splitlines(), start=1
        ):
            line = _strip_yaml_comment(raw_line)
            if not line:
                continue
            for match in BUF_REMOTE_RE.finditer(line):
                plugin_ref = (match.group(1) or match.group(2) or match.group(3) or "").strip()
                if not plugin_ref:
                    continue
                if not BUF_PINNED_TAG_RE.search(plugin_ref):
                    violations.append(
                        Violation(
                            path=rel,
                            line=line_no,
                            rule="codegen.unpinned-buf-plugin",
                            message=(
                                f"unpinned remote buf plugin `{plugin_ref}`; "
                                "specify an explicit `:v<version>` tag"
                            ),
                        )
                    )
    return violations


def _is_latest_scan_target(rel_posix: str) -> bool:
    if _is_excluded_rel(rel_posix) or rel_posix.startswith(("docs/ADR/", "docs/review/")):
        return False

    name = pathlib.PurePosixPath(rel_posix).name
    if rel_posix.startswith(".github/workflows/"):
        return not (rel_posix.startswith(".github/workflows/scripts/") and name.startswith("test_"))
    if rel_posix.startswith("scripts/"):
        return not rel_posix.startswith(("scripts/tests/", "scripts/harness/corpus/"))
    if (
        rel_posix.startswith(("docs/runbooks/", "docs/best_practices/"))
        or "Dockerfile" in name
        or name.startswith("Makefile")
    ):
        return True
    return (
        name.endswith(".go")
        and not name.endswith("_test.go")
        and "/testdata/" not in f"/{rel_posix}"
    )


def check_at_latest_references(repo_root: pathlib.Path) -> list[Violation]:
    repo_root_resolved = repo_root.resolve()
    violations: list[Violation] = []
    for rel in _list_repo_files(repo_root):
        if not _is_latest_scan_target(rel):
            continue
        full_path = repo_root / rel
        if not _is_safe_worktree_file(repo_root_resolved, full_path):
            continue
        try:
            text = full_path.read_text(encoding="utf-8", errors="ignore")
        except OSError:
            continue
        is_go_file = rel.endswith(".go")
        for line_no, line in enumerate(text.splitlines(), start=1):
            if is_go_file and GO_GENERATE_LINE_RE.match(line) is None:
                continue
            if AT_LATEST_RE.search(line):
                violations.append(
                    Violation(
                        path=rel,
                        line=line_no,
                        rule="codegen.unpinned-latest",
                        message=f"unpinned `@latest` reference found: {line.strip()}",
                    )
                )
    return violations


def check_pins(repo_root: pathlib.Path) -> list[Violation]:
    """Run all generator version pin and `@latest` checks."""
    violations: list[Violation] = []
    violations.extend(check_go_tool_mod(repo_root))
    violations.extend(check_buf_gen_pins(repo_root))
    violations.extend(check_at_latest_references(repo_root))
    return violations


def _run_subprocess(
    cmd: list[str],
    *,
    cwd: pathlib.Path | None = None,
) -> subprocess.CompletedProcess[str]:
    return subprocess.run(  # noqa: S603 - fixed local tool invocation
        cmd,
        cwd=str(cwd) if cwd is not None else None,
        capture_output=True,
        text=True,
        timeout=SUBPROCESS_TIMEOUT_SECONDS,
        check=False,
    )


def _check_sync_script(
    repo_root: pathlib.Path,
    script_rel: str,
    rule: str,
) -> list[Violation]:
    script_path = repo_root / script_rel
    if not script_path.is_file():
        return []
    res = _run_subprocess(
        [sys.executable, str(script_path), "--check", "--repo-root", str(repo_root)],
    )
    if res.returncode == 0:
        return []
    detail = (res.stderr or res.stdout).strip().splitlines()
    summary = detail[-1] if detail else f"{script_rel} --check failed (exit {res.returncode})"
    return [Violation(path=script_rel, line=1, rule=rule, message=summary)]


def _check_openapi_regen(repo_root: pathlib.Path) -> list[Violation]:
    rag_dir = repo_root / "rag-orchestrator"
    spec_path = rag_dir / "spec" / "openapi.yaml"
    tool_mod = repo_root / "scripts" / "codegen" / "go.tool.mod"
    if not spec_path.is_file() or not tool_mod.is_file():
        return []

    go_bin = shutil.which("go")
    if go_bin is None:
        return [
            Violation(
                path="rag-orchestrator/spec/openapi.yaml",
                line=1,
                rule="codegen.drift.openapi",
                message="go binary not found in PATH for oapi-codegen regeneration check",
            )
        ]

    violations: list[Violation] = []
    with tempfile.TemporaryDirectory(prefix="alt-oapi-check-") as tmp_str:
        tmp_dir = pathlib.Path(tmp_str)
        for idx, (target_rel, extra_flags) in enumerate(OPENAPI_TARGETS):
            target_path = repo_root / target_rel
            out_path = tmp_dir / f"gen_{idx}.go"
            cmd = [
                go_bin,
                "-C",
                str(rag_dir),
                "tool",
                f"-modfile={tool_mod}",
                "oapi-codegen",
                *extra_flags,
                "-o",
                str(out_path),
                "spec/openapi.yaml",
            ]
            res = _run_subprocess(cmd)
            if res.returncode != 0:
                err_msg = (res.stderr or res.stdout).strip()
                violations.append(
                    Violation(
                        path=target_rel,
                        line=1,
                        rule="codegen.drift.openapi",
                        message=f"oapi-codegen execution failed: {err_msg}",
                    )
                )
                continue
            if not target_path.is_file():
                violations.append(
                    Violation(
                        path=target_rel,
                        line=1,
                        rule="codegen.drift.openapi",
                        message=f"committed generated file missing: {target_rel}",
                    )
                )
                continue
            actual = target_path.read_bytes()
            expected = out_path.read_bytes()
            if actual != expected:
                violations.append(
                    Violation(
                        path=target_rel,
                        line=1,
                        rule="codegen.drift.openapi",
                        message=(
                            f"generated OpenAPI file drifted from rag-orchestrator/spec/openapi.yaml; "
                            f"run `make -C rag-orchestrator generate` ({target_rel})"
                        ),
                    )
                )
    return violations


def _check_protovis_regen(repo_root: pathlib.Path) -> list[Violation]:
    proto_dir = repo_root / "proto"
    protovis_dir = proto_dir / "tools" / "protovis"
    if not (proto_dir / "buf.yaml").is_file() or not (protovis_dir / "go.mod").is_file():
        return []

    buf_bin = shutil.which("buf")
    go_bin = shutil.which("go")
    missing_tools: list[Violation] = []
    if buf_bin is None:
        missing_tools.append(
            Violation(
                path="proto/buf.yaml",
                line=1,
                rule="codegen.regen-tool-missing",
                message="buf binary not found in PATH for protovis regeneration check",
            )
        )
    if go_bin is None:
        missing_tools.append(
            Violation(
                path="proto/tools/protovis",
                line=1,
                rule="codegen.regen-tool-missing",
                message="go binary not found in PATH for protovis regeneration check",
            )
        )
    if buf_bin is None or go_bin is None:
        return missing_tools

    violations: list[Violation] = []
    with tempfile.TemporaryDirectory(prefix="alt-protovis-check-") as tmp_str:
        tmp_dir = pathlib.Path(tmp_str)
        desc_path = tmp_dir / "descriptor.binpb"
        out_root = tmp_dir / "out"
        out_root.mkdir()

        buf_res = _run_subprocess(
            [buf_bin, "build", "--exclude-source-info", "-o", str(desc_path)],
            cwd=proto_dir,
        )
        if buf_res.returncode != 0:
            err_msg = (buf_res.stderr or buf_res.stdout).strip()
            return [
                Violation(
                    path="proto/buf.yaml",
                    line=1,
                    rule="codegen.drift.protovis",
                    message=f"buf build failed: {err_msg}",
                )
            ]

        pv_res = _run_subprocess(
            [go_bin, "run", ".", "-desc", str(desc_path), "-root", str(out_root)],
            cwd=protovis_dir,
        )
        if pv_res.returncode != 0:
            err_msg = (pv_res.stderr or pv_res.stdout).strip()
            return [
                Violation(
                    path="proto/tools/protovis",
                    line=1,
                    rule="codegen.drift.protovis",
                    message=f"protovis generator failed: {err_msg}",
                )
            ]

        for rel in PROTOVIS_OUTPUTS:
            committed = repo_root / rel
            regenerated = out_root / rel
            if not committed.is_file():
                violations.append(
                    Violation(
                        path=rel,
                        line=1,
                        rule="codegen.drift.protovis",
                        message=f"generated allowlist missing: {rel}",
                    )
                )
            elif not regenerated.is_file() or committed.read_bytes() != regenerated.read_bytes():
                violations.append(
                    Violation(
                        path=rel,
                        line=1,
                        rule="codegen.drift.protovis",
                        message=f"generated allowlist drifted from proto definitions: {rel}",
                    )
                )
    return violations


def check_regeneration(repo_root: pathlib.Path) -> list[Violation]:
    """Run all deterministic local code generators into temp outputs and verify zero drift."""
    violations: list[Violation] = []
    violations.extend(
        _check_sync_script(
            repo_root,
            "scripts/codegen/sync_go_canonical.py",
            "codegen.drift.go-canonical",
        )
    )
    violations.extend(
        _check_sync_script(
            repo_root,
            "scripts/codegen/pki_py/sync_python_pki.py",
            "codegen.drift.python-pki",
        )
    )
    violations.extend(_check_openapi_regen(repo_root))
    violations.extend(_check_protovis_regen(repo_root))
    return violations


def check_pair(
    before_path: pathlib.Path,
    after_path: pathlib.Path,
    *,
    logical_path: str | None = None,
) -> list[Violation]:
    """Check a `(before, after)` file pair for hand-editing of `DO NOT EDIT` generated code."""
    before_bytes = before_path.read_bytes()
    after_bytes = after_path.read_bytes()
    if before_bytes == after_bytes:
        return []

    before_text = before_bytes.decode("utf-8", errors="ignore")
    after_text = after_bytes.decode("utf-8", errors="ignore")
    before_hdr = extract_generated_header(before_text)
    after_hdr = extract_generated_header(after_text)

    if before_hdr is None and after_hdr is None:
        return []

    display_path = logical_path if logical_path else after_path.as_posix()
    if before_hdr is not None and after_hdr is None:
        reason = "DO NOT EDIT generated header was removed while modifying generated file"
    elif before_hdr == after_hdr:
        reason = (
            "generated file carrying `DO NOT EDIT` header was modified without regenerating "
            f"(header unchanged: `{after_hdr}`)"
        )
    else:
        reason = (
            "generated file carrying `DO NOT EDIT` header was modified without spec regeneration"
        )

    return [
        Violation(
            path=display_path,
            line=1,
            rule="codegen.do-not-edit-modified",
            message=reason,
        )
    ]


def _validate_git_rev(
    repo_root: pathlib.Path,
    git_bin: str,
    rev: str,
    *,
    label: str,
) -> tuple[str | None, Violation | None]:
    if not rev or not rev.strip() or rev.startswith("-"):
        return None, Violation(
            path=".",
            line=1,
            rule="codegen.invalid-git-rev",
            message=f"invalid git revision for {label}: {rev!r}",
        )
    res = _run_subprocess(
        [
            git_bin,
            "-C",
            str(repo_root),
            "rev-parse",
            "--verify",
            "--end-of-options",
            f"{rev}^{{commit}}",
        ]
    )
    if res.returncode != 0:
        err_msg = (res.stderr or res.stdout).strip()
        return None, Violation(
            path=".",
            line=1,
            rule="codegen.invalid-git-rev",
            message=f"invalid git revision for {label} ({rev!r}): {err_msg}",
        )
    return res.stdout.strip(), None


def _git_show_text(repo_root: pathlib.Path, rev: str, rel_path: str) -> str | None:
    git_bin = shutil.which("git")
    if git_bin is None:
        return None
    res = _run_subprocess(
        [git_bin, "-C", str(repo_root), "show", "--end-of-options", f"{rev}:{rel_path}"]
    )
    if res.returncode != 0:
        return None
    return res.stdout


def _extract_mockgen_source(text: str) -> str | None:
    for line in text.splitlines()[:HEADER_SCAN_LINES]:
        match = MOCKGEN_SOURCE_RE.match(line)
        if match is not None:
            return match.group(1)
    return None


def _extract_proto_stem(rel: str) -> str | None:
    name = pathlib.PurePosixPath(rel).name
    for suffix in PROTO_GEN_SUFFIXES:
        if name.endswith(suffix):
            stem = name[: -len(suffix)]
            return stem if stem else None
    return None


def _find_go_module_rel(repo_root: pathlib.Path, rel: str) -> str | None:
    cur = pathlib.PurePosixPath(rel).parent
    while True:
        candidate = repo_root / cur / "go.mod" if str(cur) != "." else repo_root / "go.mod"
        if candidate.is_file():
            return "" if str(cur) == "." else cur.as_posix()
        if str(cur) == ".":
            return None
        cur = cur.parent


def _has_same_module_go_source_change(
    repo_root: pathlib.Path,
    rel: str,
    changed_set: set[str],
) -> bool:
    mod_rel = _find_go_module_rel(repo_root, rel)
    prefix = f"{mod_rel}/" if mod_rel else ""
    for p in changed_set:
        if p == rel or not p.endswith(".go"):
            continue
        if mod_rel is not None and prefix and not p.startswith(prefix):
            continue
        parts = pathlib.PurePosixPath(p).parts
        if "mocks" in parts or parts[-1].startswith("mock_"):
            continue
        return True
    return False


def _has_matching_spec_change(
    repo_root: pathlib.Path,
    rel: str,
    text: str,
    changed_set: set[str],
) -> bool:
    proto_stem = _extract_proto_stem(rel)
    if proto_stem is not None:
        expected_proto = f"{proto_stem}.proto"
        return any(
            p.startswith("proto/buf.gen")
            or p == "proto/buf.yaml"
            or (p.endswith(".proto") and pathlib.PurePosixPath(p).name == expected_proto)
            for p in changed_set
        )
    if rel.endswith(("server.gen.go", "client.gen.go")):
        return "scripts/codegen/go.tool.mod" in changed_set or any(
            p.endswith("openapi.yaml") for p in changed_set
        )
    if rel in PROTOVIS_OUTPUTS:
        return any(
            p.endswith(".proto") or p.startswith("proto/tools/protovis/") or p == "proto/buf.yaml"
            for p in changed_set
        )

    mock_src = _extract_mockgen_source(text)
    if mock_src is not None:
        return "scripts/codegen/go.tool.mod" in changed_set or any(
            p == mock_src or p.endswith(f"/{mock_src}") for p in changed_set
        )

    name = pathlib.PurePosixPath(rel).name
    if name.startswith("mock_") and name.endswith(".go"):
        return "scripts/codegen/go.tool.mod" in changed_set or _has_same_module_go_source_change(
            repo_root, rel, changed_set
        )

    return False


@dataclass(frozen=True)
class _DiffContext:
    repo_root: pathlib.Path
    repo_root_resolved: pathlib.Path
    base_rev: str
    head_rev: str | None
    changed_set: set[str]


def _read_diff_after_text(
    ctx: _DiffContext,
    rel: str,
    before_text: str | None,
) -> tuple[str | None, Violation | None]:
    if ctx.head_rev is not None:
        return _git_show_text(ctx.repo_root, ctx.head_rev, rel), None

    full_path = ctx.repo_root / rel
    if full_path.is_symlink() or (
        full_path.exists() and not full_path.resolve().is_relative_to(ctx.repo_root_resolved)
    ):
        if before_text is not None and extract_generated_header(before_text) is not None:
            return None, Violation(
                path=rel,
                line=1,
                rule="codegen.do-not-edit-modified",
                message="tracked generated file was replaced with a symlink or out-of-repo path",
            )
        return None, None

    if not full_path.is_file():
        return None, None
    return full_path.read_text(encoding="utf-8", errors="ignore"), None


def _check_single_diff_file(ctx: _DiffContext, rel: str) -> Violation | None:
    before_text = _git_show_text(ctx.repo_root, ctx.base_rev, rel)
    after_text, unsafe_violation = _read_diff_after_text(ctx, rel, before_text)
    if unsafe_violation is not None:
        return unsafe_violation
    if before_text is None or after_text is None or before_text == after_text:
        return None

    before_hdr = extract_generated_header(before_text)
    after_hdr = extract_generated_header(after_text)
    if before_hdr is None and after_hdr is None:
        return None
    if before_hdr is not None and after_hdr is None:
        return Violation(
            path=rel,
            line=1,
            rule="codegen.do-not-edit-modified",
            message="DO NOT EDIT generated header was removed from tracked generated file",
        )
    if not _has_matching_spec_change(ctx.repo_root, rel, after_text, ctx.changed_set):
        return Violation(
            path=rel,
            line=1,
            rule="codegen.do-not-edit-modified",
            message=(
                "generated file carrying `DO NOT EDIT` header was modified without any "
                "matching spec/source/generator change"
            ),
        )
    return None


def check_git_diff(
    repo_root: pathlib.Path,
    *,
    base_rev: str,
    head_rev: str | None = None,
) -> list[Violation]:
    """Check git diff between `base_rev` and `head_rev` (or working tree) for generated tampering."""
    git_bin = shutil.which("git")
    if git_bin is None:
        return [
            Violation(
                path=".",
                line=1,
                rule="codegen.git-unavailable",
                message="git executable not found in PATH",
            )
        ]

    resolved_base, base_err = _validate_git_rev(repo_root, git_bin, base_rev, label="--base")
    if base_err is not None or resolved_base is None:
        return [base_err] if base_err is not None else []

    resolved_head: str | None = None
    if head_rev is not None:
        resolved_head, head_err = _validate_git_rev(repo_root, git_bin, head_rev, label="--head")
        if head_err is not None:
            return [head_err]

    diff_cmd = [git_bin, "-C", str(repo_root), "diff", "--name-only", resolved_base]
    if resolved_head is not None:
        diff_cmd.append(resolved_head)
    diff_cmd.append("--")
    res = _run_subprocess(diff_cmd)
    if res.returncode != 0:
        err_msg = (res.stderr or res.stdout).strip()
        return [
            Violation(
                path=".",
                line=1,
                rule="codegen.git-diff-error",
                message=f"git diff failed: {err_msg}",
            )
        ]

    changed_files = [
        line.strip()
        for line in res.stdout.splitlines()
        if line.strip() and not _is_excluded_rel(line.strip())
    ]
    ctx = _DiffContext(
        repo_root=repo_root,
        repo_root_resolved=repo_root.resolve(),
        base_rev=resolved_base,
        head_rev=resolved_head,
        changed_set=set(changed_files),
    )
    violations: list[Violation] = []
    for rel in sorted(changed_files):
        violation = _check_single_diff_file(ctx, rel)
        if violation is not None:
            violations.append(violation)
    return violations


def _format_violations(violations: list[Violation]) -> None:
    for v in violations:
        sys.stderr.write(f"{v.path}:{v.line}: [{v.rule}] {v.message}\n")


def parse_args(argv: list[str] | None = None) -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="Verify generated code pins, regeneration parity, and DO NOT EDIT tamper rules.",
    )
    parser.add_argument(
        "--repo-root",
        type=pathlib.Path,
        default=None,
        help="Override repository root directory.",
    )
    parser.add_argument(
        "--list-generated",
        action="store_true",
        help="List all tracked files matching `Code generated ... DO NOT EDIT` in their first 40 lines.",
    )
    parser.add_argument(
        "--check-pins",
        action="store_true",
        help="Run only generator pin and `@latest` audit checks.",
    )
    parser.add_argument(
        "--check-regen",
        action="store_true",
        help="Run only local deterministic generator drift checks.",
    )
    parser.add_argument(
        "--before",
        type=pathlib.Path,
        default=None,
        help="Before file path for pair tamper comparison (requires --after).",
    )
    parser.add_argument(
        "--after",
        type=pathlib.Path,
        default=None,
        help="After file path for pair tamper comparison (requires --before).",
    )
    parser.add_argument(
        "--logical-path",
        type=str,
        default=None,
        help="Optional repo-relative display path for --before/--after diagnostics.",
    )
    parser.add_argument(
        "--base",
        type=str,
        default=None,
        help="Git base revision to check diff against for DO NOT EDIT tampering.",
    )
    parser.add_argument(
        "--head",
        type=str,
        default=None,
        help="Optional Git head revision for --base diff comparison (default: working tree).",
    )
    parser.add_argument(
        "--only-diff",
        action="store_true",
        help="When used with --base, run only git diff tamper check (skipping pins and regen).",
    )
    return parser.parse_args(argv)


def _run_pair_mode(args: argparse.Namespace) -> int:
    if not args.before.is_file() or not args.after.is_file():
        sys.stderr.write("error: --before and --after paths must point to existing files\n")
        return EXIT_USAGE_ERROR
    violations = check_pair(args.before, args.after, logical_path=args.logical_path)
    if violations:
        _format_violations(violations)
        return EXIT_VIOLATIONS
    return EXIT_CLEAN


def _resolve_check_flags(args: argparse.Namespace) -> tuple[bool, bool, bool]:
    if args.only_diff:
        return False, False, True
    if args.base is not None:
        if args.check_pins or args.check_regen:
            return bool(args.check_pins), bool(args.check_regen), True
        return True, True, True
    run_pins = bool(args.check_pins or not args.check_regen)
    run_regen = bool(args.check_regen or not args.check_pins)
    return run_pins, run_regen, False


def _run_repo_checks(args: argparse.Namespace, repo_root: pathlib.Path) -> int:
    if args.list_generated:
        for rel in discover_generated_files(repo_root):
            sys.stdout.write(f"{rel}\n")
        return EXIT_CLEAN

    run_pins, run_regen, run_diff = _resolve_check_flags(args)
    violations: list[Violation] = []
    if run_pins:
        violations.extend(check_pins(repo_root))
    if run_regen:
        violations.extend(check_regeneration(repo_root))
    if run_diff and args.base is not None:
        violations.extend(check_git_diff(repo_root, base_rev=args.base, head_rev=args.head))

    if violations:
        _format_violations(violations)
        sys.stderr.write(f"FAIL: {len(violations)} generated-code violation(s) found.\n")
        return EXIT_VIOLATIONS

    ok_msg = (
        "OK: no tampered DO NOT EDIT generated files in git diff.\n"
        if args.only_diff
        else "OK: generated code pins and regeneration parity verified.\n"
    )
    sys.stdout.write(ok_msg)
    return EXIT_CLEAN


def _validate_cli_args(args: argparse.Namespace) -> str | None:
    if (args.before is None) != (args.after is None):
        return "error: --before and --after must be provided together\n"
    if args.head is not None and args.base is None:
        return "error: --head requires --base\n"
    if args.only_diff and args.base is None:
        return "error: --only-diff requires --base\n"
    if args.base is not None and (not args.base.strip() or args.base.startswith("-")):
        return f"error: invalid --base git revision: {args.base!r}\n"
    if args.head is not None and (not args.head.strip() or args.head.startswith("-")):
        return f"error: invalid --head git revision: {args.head!r}\n"
    return None


def main(argv: list[str] | None = None) -> int:
    args = parse_args(argv)
    arg_err = _validate_cli_args(args)
    if arg_err is not None:
        sys.stderr.write(arg_err)
        return EXIT_USAGE_ERROR

    if args.before is not None and args.after is not None:
        return _run_pair_mode(args)

    repo_root = args.repo_root.resolve() if args.repo_root is not None else default_repo_root()
    return _run_repo_checks(args, repo_root)


if __name__ == "__main__":
    raise SystemExit(main())
