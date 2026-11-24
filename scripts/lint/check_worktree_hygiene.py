#!/usr/bin/env python3
"""CHECK-06 worktree hygiene checker.

Detects two classes of accidental worktree corruption before commit/CI:
1. 0-byte files:
   - Tracked files that were non-empty in HEAD (`git cat-file -s HEAD:<path> > 0`)
     but are now 0 bytes in the working tree (`[hygiene.zero-byte-tracked]`).
   - Untracked non-ignored 0-byte files not matched by
     `scripts/lint/empty_file_allowlist.toml` (`[hygiene.zero-byte-untracked]`).
2. Binary build artifacts in untracked or staged files (`[hygiene.binary-artifact]`):
   - ELF (`\\x7fELF`)
   - Mach-O (32/64-bit big/little endian, and universal fat binaries with
     `1 <= nfat_arch <= 32` to avoid colliding with Java `.class` files)
   - Windows PE (`MZ` header >= 64 bytes with `PE\\x00\\x00` at `e_lfanew`)

Paths under `stopped-using-k8s/` and `scripts/harness/corpus/` are excluded.

Run:
    python3 scripts/lint/check_worktree_hygiene.py [--repo-root <dir>]
"""

from __future__ import annotations

import argparse
import os
import pathlib
import re
import shutil
import stat
import struct
import subprocess
import sys
import tomllib
from dataclasses import dataclass
from pathlib import Path

EXIT_OK = 0
EXIT_VIOLATIONS = 1
EXIT_USAGE = 2

EXCLUDED_PREFIXES: tuple[str, ...] = (
    "stopped-using-k8s/",
    "scripts/harness/corpus/",
)
DEFAULT_UNTRACKED_EMPTY_GLOBS: tuple[str, ...] = (
    "**/__init__.py",
    "**/.gitkeep",
    "**/py.typed",
)
ALLOWLIST_REL = Path("scripts") / "lint" / "empty_file_allowlist.toml"

REGULAR_FILE_MODES: frozenset[str] = frozenset({"100644", "100755"})
ELF_MAGIC = b"\x7fELF"
MACHO_SINGLE_MAGICS: frozenset[bytes] = frozenset(
    {
        b"\xfe\xed\xfa\xce",
        b"\xce\xfa\xed\xfe",
        b"\xfe\xed\xfa\xcf",
        b"\xcf\xfa\xed\xfe",
    }
)
MACHO_FAT_MAGIC = b"\xca\xfe\xba\xbe"
MACHO_FAT_MIN_HEADER = 8
MACHO_FAT_MIN_ARCH = 1
MACHO_FAT_MAX_ARCH = 32
PE_MAGIC_MZ = b"MZ"
PE_SIGNATURE = b"PE\x00\x00"
PE_MIN_SIZE = 64
PE_OFFSET_START = 0x3C
PE_OFFSET_END = 0x40
PE_SIG_LEN = 4
HEADER_READ_BYTES = 64
MIN_MAGIC_LEN = 4


@dataclass(frozen=True)
class HygieneFinding:
    code: str
    path: str
    detail: str


class HygieneUsageError(RuntimeError):
    """Raised on invalid CLI arguments, non-git repository, or unreadable allowlist."""


def _git_executable() -> str:
    git_bin = shutil.which("git")
    if git_bin is None:
        msg = "git executable not found on PATH"
        raise HygieneUsageError(msg)
    return git_bin


def _run_git(
    repo_root: Path,
    args: list[str],
    *,
    input_bytes: bytes | None = None,
) -> subprocess.CompletedProcess[bytes]:
    return subprocess.run(  # noqa: S603 -- git resolved via shutil.which with explicit args
        [_git_executable(), "-C", str(repo_root), *args],
        input=input_bytes,
        capture_output=True,
        check=False,
    )


def _resolve_default_repo_root() -> Path:
    res = subprocess.run(  # noqa: S603 -- git resolved via shutil.which with explicit args
        [_git_executable(), "rev-parse", "--show-toplevel"],
        capture_output=True,
        text=True,
        check=False,
    )
    if res.returncode == 0 and res.stdout.strip():
        return Path(res.stdout.strip()).resolve()
    return Path(__file__).resolve().parents[2]


def _is_excluded(rel_path: str) -> bool:
    return any(rel_path.startswith(prefix) for prefix in EXCLUDED_PREFIXES)


def _glob_to_regex(pattern: str) -> re.Pattern[str]:
    pat = pattern.strip().removeprefix("./")
    out: list[str] = ["^"]
    i = 0
    n = len(pat)
    while i < n:
        if pat[i : i + 3] == "**/":
            out.append("(?:.+/)?")
            i += 3
        elif pat[i : i + 3] == "/**" and i + 3 == n:
            out.append("/.+")
            i += 3
        elif pat[i : i + 2] == "**":
            out.append(".*")
            i += 2
        elif pat[i] == "*":
            out.append("[^/]*")
            i += 1
        elif pat[i] == "?":
            out.append("[^/]")
            i += 1
        else:
            out.append(re.escape(pat[i]))
            i += 1
    out.append("$")
    return re.compile("".join(out))


def _load_allowlist_regexes(
    repo_root: Path,
    explicit_allowlist: Path | None,
) -> list[re.Pattern[str]]:
    if explicit_allowlist is not None:
        if not explicit_allowlist.is_file():
            msg = f"allowlist file not found: {explicit_allowlist}"
            raise HygieneUsageError(msg)
        allowlist_path: Path | None = explicit_allowlist
    else:
        repo_candidate = repo_root / ALLOWLIST_REL
        script_candidate = Path(__file__).resolve().parent / "empty_file_allowlist.toml"
        if repo_candidate.is_file():
            allowlist_path = repo_candidate
        elif script_candidate.is_file():
            allowlist_path = script_candidate
        else:
            allowlist_path = None

    if allowlist_path is None:
        globs = list(DEFAULT_UNTRACKED_EMPTY_GLOBS)
    else:
        try:
            data = tomllib.loads(allowlist_path.read_text(encoding="utf-8"))
        except (OSError, UnicodeDecodeError, tomllib.TOMLDecodeError) as exc:
            msg = f"failed to parse allowlist {allowlist_path}: {exc}"
            raise HygieneUsageError(msg) from exc

        section = data.get("untracked_empty")
        raw_globs = section.get("globs") if isinstance(section, dict) else None
        if not isinstance(raw_globs, list) or not all(
            isinstance(item, str) and item.strip() for item in raw_globs
        ):
            msg = f"allowlist {allowlist_path} must define [untracked_empty].globs as a list of strings"
            raise HygieneUsageError(msg)
        globs = [str(item) for item in raw_globs]

    return [_glob_to_regex(pat) for pat in globs]


def _verify_git_repo(repo_root: Path) -> None:
    if not repo_root.is_dir():
        msg = f"repository root is not a directory: {repo_root}"
        raise HygieneUsageError(msg)
    res = _run_git(repo_root, ["rev-parse", "--is-inside-work-tree"])
    if res.returncode != 0 or res.stdout.strip() != b"true":
        msg = f"not inside a git worktree: {repo_root}"
        raise HygieneUsageError(msg)


def _has_head_commit(repo_root: Path) -> bool:
    res = _run_git(repo_root, ["rev-parse", "--verify", "--quiet", "HEAD^{commit}"])
    return res.returncode == 0


def _list_tracked_regular_files(repo_root: Path) -> list[str]:
    res = _run_git(repo_root, ["ls-files", "-s", "-z"])
    if res.returncode != 0:
        msg = f"git ls-files failed: {res.stderr.decode('utf-8', errors='replace').strip()}"
        raise HygieneUsageError(msg)

    seen: set[str] = set()
    files: list[str] = []
    for entry in res.stdout.split(b"\x00"):
        if not entry:
            continue
        meta_bytes, sep, path_bytes = entry.partition(b"\t")
        if not sep:
            continue
        mode = meta_bytes.split(b" ", 1)[0].decode("ascii", errors="replace")
        if mode not in REGULAR_FILE_MODES:
            continue
        rel_path = path_bytes.decode("utf-8", errors="surrogateescape")
        if _is_excluded(rel_path) or rel_path in seen:
            continue
        seen.add(rel_path)
        files.append(rel_path)
    return files


def _list_z_paths(repo_root: Path, git_args: list[str]) -> list[str]:
    res = _run_git(repo_root, git_args)
    if res.returncode != 0:
        msg = f"git {' '.join(git_args)} failed: {res.stderr.decode('utf-8', errors='replace').strip()}"
        raise HygieneUsageError(msg)

    paths: list[str] = []
    for entry in res.stdout.split(b"\x00"):
        if not entry:
            continue
        rel_path = entry.decode("utf-8", errors="surrogateescape")
        if not _is_excluded(rel_path):
            paths.append(rel_path)
    return paths


def _head_blob_sizes(repo_root: Path, rel_paths: list[str]) -> dict[str, int]:
    """Return HEAD blob sizes for paths that exist as blobs in HEAD."""
    if not rel_paths or not _has_head_commit(repo_root):
        return {}

    sizes: dict[str, int] = {}
    batch_paths: list[str] = []
    for rel_path in rel_paths:
        if "\n" in rel_path or "\r" in rel_path:
            res = _run_git(repo_root, ["cat-file", "-s", f"HEAD:{rel_path}"])
            if res.returncode == 0:
                raw_size = res.stdout.decode("ascii", errors="replace").strip()
                if raw_size.isdigit():
                    sizes[rel_path] = int(raw_size)
        else:
            batch_paths.append(rel_path)

    if not batch_paths:
        return sizes

    stdin_payload = "".join(f"HEAD:{p}\n" for p in batch_paths).encode("utf-8")
    res = _run_git(
        repo_root,
        ["cat-file", "--batch-check=%(objecttype) %(objectsize)"],
        input_bytes=stdin_payload,
    )
    if res.returncode != 0:
        return sizes

    lines = res.stdout.decode("utf-8", errors="replace").splitlines()
    for rel_path, line in zip(batch_paths, lines, strict=False):
        parts = line.split()
        if len(parts) == 2 and parts[0] == "blob" and parts[1].isdigit():  # noqa: PLR2004 -- objecttype + size
            sizes[rel_path] = int(parts[1])
    return sizes


def _classify_binary_header(header: bytes, file_size: int, file_path: Path) -> str | None:
    fmt: str | None = None
    if len(header) < MIN_MAGIC_LEN:
        return None
    if header.startswith(ELF_MAGIC):
        fmt = "ELF"
    elif header[:MIN_MAGIC_LEN] in MACHO_SINGLE_MAGICS:
        fmt = "Mach-O"
    elif header.startswith(MACHO_FAT_MAGIC) and len(header) >= MACHO_FAT_MIN_HEADER:
        nfat_arch = struct.unpack(">I", header[4:8])[0]
        if MACHO_FAT_MIN_ARCH <= nfat_arch <= MACHO_FAT_MAX_ARCH:
            fmt = "Mach-O universal"
    elif header.startswith(PE_MAGIC_MZ) and file_size >= PE_MIN_SIZE:
        e_lfanew = struct.unpack("<I", header[PE_OFFSET_START:PE_OFFSET_END])[0]
        if e_lfanew <= file_size - PE_SIG_LEN:
            with file_path.open("rb") as fh:
                fh.seek(e_lfanew)
                if fh.read(PE_SIG_LEN) == PE_SIGNATURE:
                    fmt = "PE"
    return fmt


def detect_binary_format(file_path: Path) -> str | None:
    """Return binary format label ('ELF', 'Mach-O', 'Mach-O universal', 'PE') or None."""
    try:
        st = file_path.lstat()
        if not stat.S_ISREG(st.st_mode) or st.st_size < MIN_MAGIC_LEN:
            return None
        with file_path.open("rb") as fh:
            header = fh.read(HEADER_READ_BYTES)
        return _classify_binary_header(header, st.st_size, file_path)
    except OSError:
        return None


def _check_tracked_zero_byte(
    repo_root: Path,
    tracked_files: list[str],
    findings: list[HygieneFinding],
) -> None:
    zero_candidates: list[str] = []
    for rel_path in tracked_files:
        full_path = repo_root / rel_path
        try:
            st = full_path.lstat()
        except OSError:
            continue
        if stat.S_ISREG(st.st_mode) and st.st_size == 0:
            zero_candidates.append(rel_path)

    head_sizes = _head_blob_sizes(repo_root, zero_candidates)
    for rel_path in zero_candidates:
        head_size = head_sizes.get(rel_path, 0)
        if head_size > 0:
            findings.append(
                HygieneFinding(
                    code="hygiene.zero-byte-tracked",
                    path=rel_path,
                    detail=(
                        f"tracked file was {head_size} bytes in HEAD "
                        "but is now 0 bytes in the working tree"
                    ),
                )
            )


def _check_untracked_zero_byte(
    repo_root: Path,
    untracked_files: list[str],
    allowlist_regexes: list[re.Pattern[str]],
    findings: list[HygieneFinding],
) -> None:
    for rel_path in untracked_files:
        full_path = repo_root / rel_path
        try:
            st = full_path.lstat()
        except OSError:
            continue
        if not stat.S_ISREG(st.st_mode) or st.st_size != 0:
            continue
        if any(rx.match(rel_path) for rx in allowlist_regexes):
            continue
        findings.append(
            HygieneFinding(
                code="hygiene.zero-byte-untracked",
                path=rel_path,
                detail="untracked 0-byte file is not matched by scripts/lint/empty_file_allowlist.toml",
            )
        )


def _check_staged_added_zero_byte(
    repo_root: Path,
    staged_added_files: list[str],
    allowlist_regexes: list[re.Pattern[str]],
    findings: list[HygieneFinding],
) -> None:
    for rel_path in staged_added_files:
        if any(rx.match(rel_path) for rx in allowlist_regexes):
            continue
        full_path = repo_root / rel_path
        try:
            st = full_path.lstat()
        except OSError:
            continue
        if stat.S_ISREG(st.st_mode) and st.st_size == 0:
            findings.append(
                HygieneFinding(
                    code="hygiene.zero-byte-staged",
                    path=rel_path,
                    detail="newly staged 0-byte file is not matched by scripts/lint/empty_file_allowlist.toml",
                )
            )


def _check_binary_artifacts(
    repo_root: Path,
    untracked_files: list[str],
    staged_files: list[str],
    findings: list[HygieneFinding],
) -> int:
    sources: dict[str, list[str]] = {}
    for rel_path in untracked_files:
        sources.setdefault(rel_path, []).append("untracked")
    for rel_path in staged_files:
        sources.setdefault(rel_path, []).append("staged")

    for rel_path in sorted(sources):
        fmt = detect_binary_format(repo_root / rel_path)
        if fmt is not None:
            origin = "+".join(sources[rel_path])
            findings.append(
                HygieneFinding(
                    code="hygiene.binary-artifact",
                    path=rel_path,
                    detail=f"detected {fmt} binary artifact ({origin})",
                )
            )
    return len(sources)


def check_worktree_hygiene(
    repo_root: Path,
    *,
    explicit_allowlist: Path | None = None,
) -> tuple[list[HygieneFinding], dict[str, int]]:
    _verify_git_repo(repo_root)
    allowlist_regexes = _load_allowlist_regexes(repo_root, explicit_allowlist)

    tracked_files = _list_tracked_regular_files(repo_root)
    untracked_files = _list_z_paths(repo_root, ["ls-files", "--others", "--exclude-standard", "-z"])
    staged_files = _list_z_paths(
        repo_root, ["diff", "--cached", "--name-only", "--diff-filter=AM", "-z"]
    )
    staged_added_files = _list_z_paths(
        repo_root, ["diff", "--cached", "--name-only", "--diff-filter=A", "-z"]
    )

    findings: list[HygieneFinding] = []
    _check_tracked_zero_byte(repo_root, tracked_files, findings)
    _check_untracked_zero_byte(repo_root, untracked_files, allowlist_regexes, findings)
    _check_staged_added_zero_byte(repo_root, staged_added_files, allowlist_regexes, findings)
    checked_binary_candidates = _check_binary_artifacts(
        repo_root, untracked_files, staged_files, findings
    )

    stats = {
        "tracked": len(tracked_files),
        "untracked": len(untracked_files),
        "binary_candidates": checked_binary_candidates,
    }
    return findings, stats


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(
        description="Verify git worktree hygiene (0-byte truncations and binary build artifacts)."
    )
    parser.add_argument(
        "--repo-root",
        type=Path,
        default=None,
        help="Git repository root to inspect (defaults to current git toplevel).",
    )
    parser.add_argument(
        "--allowlist",
        type=Path,
        default=None,
        help="Optional override path for empty_file_allowlist.toml.",
    )
    args = parser.parse_args(argv)

    try:
        repo_root = (
            args.repo_root.resolve() if args.repo_root is not None else _resolve_default_repo_root()
        )
        findings, stats = check_worktree_hygiene(
            repo_root,
            explicit_allowlist=args.allowlist.resolve() if args.allowlist is not None else None,
        )
    except HygieneUsageError as exc:
        sys.stderr.write(f"ERROR: {exc}\n")
        return EXIT_USAGE

    if findings:
        for finding in findings:
            sys.stdout.write(f"[{finding.code}] {finding.path}: {finding.detail}\n")
        return EXIT_VIOLATIONS

    sys.stdout.write(
        "OK: worktree hygiene clean "
        f"(tracked={stats['tracked']}, untracked={stats['untracked']}, "
        f"binary_candidates={stats['binary_candidates']})\n"
    )
    return EXIT_OK


if __name__ == "__main__":
    _launcher = (
        pathlib.Path(__file__).resolve().parents[2] / "scripts" / "altharness" / "alt-harness"
    )
    if _launcher.is_file():
        os.execv(_launcher, [str(_launcher), "hygiene", *sys.argv[1:]])  # noqa: S606 -- delegates CLI execution to the Go harness
    sys.exit(main())
