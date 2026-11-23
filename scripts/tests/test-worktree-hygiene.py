#!/usr/bin/env python3
"""Tests for scripts/lint/check_worktree_hygiene.py (CHECK-06).

Run:
    python3 scripts/tests/test-worktree-hygiene.py
"""

from __future__ import annotations

import pathlib
import shutil
import struct
import subprocess
import sys
import tempfile

ROOT = pathlib.Path(__file__).resolve().parents[2]
HYGIENE_SCRIPT = ROOT / "scripts" / "lint" / "check_worktree_hygiene.py"
ALLOWLIST_PATH = ROOT / "scripts" / "lint" / "empty_file_allowlist.toml"
CORPUS_DIR = ROOT / "scripts" / "harness" / "corpus"

GIT_BIN = shutil.which("git")
assert GIT_BIN is not None, "git executable not found in PATH"

USAGE_RC = 2


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


def git(repo: pathlib.Path, *args: str) -> subprocess.CompletedProcess[str]:
    assert GIT_BIN is not None
    return subprocess.run(  # noqa: S603 - fixed argv built in this test
        [GIT_BIN, "-C", str(repo), *args],
        capture_output=True,
        text=True,
        check=True,
    )


def init_git_repo(repo: pathlib.Path) -> pathlib.Path:
    git(repo, "init", "-b", "main")
    git(repo, "config", "user.email", "test@example.com")
    git(repo, "config", "user.name", "Hygiene Test")
    return repo


def write_bytes(repo: pathlib.Path, rel_path: str, content: bytes) -> pathlib.Path:
    target = repo / rel_path
    target.parent.mkdir(parents=True, exist_ok=True)
    target.write_bytes(content)
    return target


def run_hygiene(
    repo: pathlib.Path,
    extra_args: list[str] | None = None,
) -> subprocess.CompletedProcess[str]:
    cmd = [sys.executable, str(HYGIENE_SCRIPT), "--repo-root", str(repo), *(extra_args or [])]
    return subprocess.run(  # noqa: S603 - fixed argv built in this test
        cmd,
        capture_output=True,
        text=True,
        check=False,
    )


def make_pe_bytes(e_lfanew: int = 0x80) -> bytes:
    buf = bytearray(e_lfanew + 16)
    buf[0:2] = b"MZ"
    struct.pack_into("<I", buf, 0x3C, e_lfanew)
    buf[e_lfanew : e_lfanew + 4] = b"PE\x00\x00"
    return bytes(buf)


def make_macho_fat_bytes(nfat_arch: int) -> bytes:
    return b"\xca\xfe\xba\xbe" + struct.pack(">I", nfat_arch) + (b"\x00" * 32)


def test_zero_byte_checks() -> None:
    sys.stdout.write("check_worktree_hygiene: tracked & untracked 0-byte checks\n")

    with tempfile.TemporaryDirectory() as tmp:
        repo = init_git_repo(pathlib.Path(tmp))
        write_bytes(repo, "app/main.py", b"print('ok')\n")
        write_bytes(repo, "app/__init__.py", b"")
        write_bytes(repo, "data/.gitkeep", b"")
        write_bytes(repo, ".c2quay/locks/production.lock", b"")
        git(repo, "add", "-A")
        git(repo, "commit", "-m", "init")

        res = run_hygiene(repo)
        check(
            "clean repo with 0-byte tracked files in HEAD passes",
            res.returncode == 0,
            res.stdout + res.stderr,
        )

    with tempfile.TemporaryDirectory() as tmp:
        repo = init_git_repo(pathlib.Path(tmp))
        write_bytes(repo, "recap-subworker/runs.py", b"def run():\n    return 1\n")
        git(repo, "add", "-A")
        git(repo, "commit", "-m", "init")

        write_bytes(repo, "recap-subworker/runs.py", b"")
        res_unstaged = run_hygiene(repo)
        check(
            "tracked file truncated to 0 bytes (unstaged) fails with [hygiene.zero-byte-tracked]",
            res_unstaged.returncode == 1
            and "[hygiene.zero-byte-tracked]" in (res_unstaged.stdout + res_unstaged.stderr)
            and "recap-subworker/runs.py" in (res_unstaged.stdout + res_unstaged.stderr),
            res_unstaged.stdout + res_unstaged.stderr,
        )

        git(repo, "add", "recap-subworker/runs.py")
        res_staged = run_hygiene(repo)
        check(
            "tracked file truncated to 0 bytes (staged) still fails with [hygiene.zero-byte-tracked]",
            res_staged.returncode == 1
            and "[hygiene.zero-byte-tracked]" in (res_staged.stdout + res_staged.stderr)
            and "recap-subworker/runs.py" in (res_staged.stdout + res_staged.stderr),
            res_staged.stdout + res_staged.stderr,
        )

    with tempfile.TemporaryDirectory() as tmp:
        repo = init_git_repo(pathlib.Path(tmp))
        write_bytes(repo, "README.md", b"# test\n")
        git(repo, "add", "-A")
        git(repo, "commit", "-m", "init")

        write_bytes(repo, "__init__.py", b"")
        write_bytes(repo, "pkg/sub/__init__.py", b"")
        write_bytes(repo, "empty_dir/.gitkeep", b"")
        res_allowed = run_hygiene(repo)
        check(
            "allowlisted untracked 0-byte files (__init__.py, .gitkeep) pass",
            res_allowed.returncode == 0,
            res_allowed.stdout + res_allowed.stderr,
        )

        write_bytes(repo, "pkg/orphan.py", b"")
        res_bad = run_hygiene(repo)
        check(
            "non-allowlisted untracked 0-byte file fails with [hygiene.zero-byte-untracked]",
            res_bad.returncode == 1
            and "[hygiene.zero-byte-untracked]" in (res_bad.stdout + res_bad.stderr)
            and "pkg/orphan.py" in (res_bad.stdout + res_bad.stderr),
            res_bad.stdout + res_bad.stderr,
        )

        git(repo, "add", "pkg/orphan.py")
        res_staged_empty = run_hygiene(repo)
        check(
            "newly staged non-allowlisted 0-byte file fails with [hygiene.zero-byte-staged]",
            res_staged_empty.returncode == 1
            and "[hygiene.zero-byte-staged]" in (res_staged_empty.stdout + res_staged_empty.stderr)
            and "pkg/orphan.py" in (res_staged_empty.stdout + res_staged_empty.stderr),
            res_staged_empty.stdout + res_staged_empty.stderr,
        )

        git(repo, "reset", "HEAD", "--", "pkg/orphan.py")
        (repo / "pkg/orphan.py").unlink()
        git(repo, "add", "pkg/sub/__init__.py")
        res_staged_allowed = run_hygiene(repo)
        check(
            "newly staged allowlisted 0-byte file (__init__.py) passes",
            res_staged_allowed.returncode == 0,
            res_staged_allowed.stdout + res_staged_allowed.stderr,
        )


def test_binary_artifact_checks() -> None:
    sys.stdout.write("check_worktree_hygiene: binary artifact detection (ELF, Mach-O, PE)\n")

    with tempfile.TemporaryDirectory() as tmp:
        repo = init_git_repo(pathlib.Path(tmp))
        write_bytes(repo, "main.go", b"package main\n")
        git(repo, "add", "-A")
        git(repo, "commit", "-m", "init")

        write_bytes(repo, "alt-backend/app/alt-backend", b"\x7fELF\x02\x01\x01" + (b"\x00" * 57))
        res_elf = run_hygiene(repo)
        check(
            "untracked ELF binary fails with [hygiene.binary-artifact]",
            res_elf.returncode == 1
            and "[hygiene.binary-artifact]" in (res_elf.stdout + res_elf.stderr)
            and "alt-backend/app/alt-backend" in (res_elf.stdout + res_elf.stderr),
            res_elf.stdout + res_elf.stderr,
        )

        git(repo, "add", "alt-backend/app/alt-backend")
        res_elf_staged = run_hygiene(repo)
        check(
            "staged ELF binary fails with [hygiene.binary-artifact]",
            res_elf_staged.returncode == 1
            and "[hygiene.binary-artifact]" in (res_elf_staged.stdout + res_elf_staged.stderr),
            res_elf_staged.stdout + res_elf_staged.stderr,
        )

    macho_magics = [
        ("macho-32-be", b"\xfe\xed\xfa\xce" + (b"\x00" * 28)),
        ("macho-32-le", b"\xce\xfa\xed\xfe" + (b"\x00" * 28)),
        ("macho-64-be", b"\xfe\xed\xfa\xcf" + (b"\x00" * 28)),
        ("macho-64-le", b"\xcf\xfa\xed\xfe" + (b"\x00" * 28)),
        ("macho-fat-2arch", make_macho_fat_bytes(2)),
    ]
    for label, payload in macho_magics:
        with tempfile.TemporaryDirectory() as tmp:
            repo = init_git_repo(pathlib.Path(tmp))
            write_bytes(repo, "README.md", b"hi\n")
            git(repo, "add", "-A")
            git(repo, "commit", "-m", "init")

            write_bytes(repo, f"bin/{label}", payload)
            res = run_hygiene(repo)
            check(
                f"untracked {label} fails with [hygiene.binary-artifact]",
                res.returncode == 1 and "[hygiene.binary-artifact]" in (res.stdout + res.stderr),
                res.stdout + res.stderr,
            )

    with tempfile.TemporaryDirectory() as tmp:
        repo = init_git_repo(pathlib.Path(tmp))
        write_bytes(repo, "README.md", b"hi\n")
        git(repo, "add", "-A")
        git(repo, "commit", "-m", "init")

        write_bytes(repo, "Sample.class", make_macho_fat_bytes(52))
        res_java = run_hygiene(repo)
        check(
            "Java .class (0xCAFEBABE with nfat_arch=52) does not false-positive",
            res_java.returncode == 0,
            res_java.stdout + res_java.stderr,
        )

    with tempfile.TemporaryDirectory() as tmp:
        repo = init_git_repo(pathlib.Path(tmp))
        write_bytes(repo, "README.md", b"hi\n")
        git(repo, "add", "-A")
        git(repo, "commit", "-m", "init")

        write_bytes(repo, "short_mz.txt", b"MZ short text\n")
        write_bytes(repo, "long_mz.txt", b"MZ" + (b" plain text line\n" * 10))
        res_mz_text = run_hygiene(repo)
        check(
            "plain text starting with MZ does not false-positive",
            res_mz_text.returncode == 0,
            res_mz_text.stdout + res_mz_text.stderr,
        )

        write_bytes(repo, "tool.exe", make_pe_bytes(0x80))
        res_pe = run_hygiene(repo)
        check(
            "valid Windows PE binary fails with [hygiene.binary-artifact]",
            res_pe.returncode == 1
            and "[hygiene.binary-artifact]" in (res_pe.stdout + res_pe.stderr)
            and "tool.exe" in (res_pe.stdout + res_pe.stderr),
            res_pe.stdout + res_pe.stderr,
        )


def test_exclusions_and_corpus() -> None:
    with tempfile.TemporaryDirectory() as tmp:
        repo = init_git_repo(pathlib.Path(tmp))
        write_bytes(repo, ".gitignore", b"ignored-bin\nignored-empty\n")
        write_bytes(repo, "stopped-using-k8s/old.yaml", b"kind: Pod\n")
        write_bytes(repo, "scripts/harness/corpus/C-99/before.py", b"x = 1\n")
        git(repo, "add", "-A")
        git(repo, "commit", "-m", "init")

        write_bytes(repo, "stopped-using-k8s/old.yaml", b"")
        write_bytes(repo, "stopped-using-k8s/legacy.elf", b"\x7fELF" + (b"\x00" * 60))
        write_bytes(repo, "scripts/harness/corpus/C-99/before.py", b"")
        write_bytes(repo, "scripts/harness/corpus/C-99/after.elf", b"\x7fELF" + (b"\x00" * 60))
        write_bytes(repo, "ignored-bin", b"\x7fELF" + (b"\x00" * 60))
        write_bytes(repo, "ignored-empty", b"")
        res_excl = run_hygiene(repo)
        check(
            "stopped-using-k8s/, scripts/harness/corpus/, and gitignored files are excluded",
            res_excl.returncode == 0,
            res_excl.stdout + res_excl.stderr,
        )

    sys.stdout.write("check_worktree_hygiene: corpus C-19 & C-20 simulation + real repo\n")
    with tempfile.TemporaryDirectory() as tmp:
        repo = init_git_repo(pathlib.Path(tmp))
        c19_before = (CORPUS_DIR / "C-19" / "before.py").read_bytes()
        c19_after = (CORPUS_DIR / "C-19" / "after.py").read_bytes()
        write_bytes(repo, "recap-subworker/recap_subworker/app/routers/runs.py", c19_before)
        git(repo, "add", "-A")
        git(repo, "commit", "-m", "c19 before")
        res_b = run_hygiene(repo)
        write_bytes(repo, "recap-subworker/recap_subworker/app/routers/runs.py", c19_after)
        res_a = run_hygiene(repo)
        check(
            "corpus C-19 before passes and after fails with [hygiene.zero-byte-tracked]",
            res_b.returncode == 0
            and res_a.returncode == 1
            and "[hygiene.zero-byte-tracked]" in (res_a.stdout + res_a.stderr),
            f"before={res_b.returncode} after={res_a.returncode} out={res_a.stdout + res_a.stderr}",
        )

    with tempfile.TemporaryDirectory() as tmp:
        repo = init_git_repo(pathlib.Path(tmp))
        c20_before = (CORPUS_DIR / "C-20" / "before.go").read_bytes()
        c20_after = (CORPUS_DIR / "C-20" / "after.elf").read_bytes()
        write_bytes(repo, "alt-backend/app/main.go", c20_before)
        git(repo, "add", "-A")
        git(repo, "commit", "-m", "c20 before")
        res_b = run_hygiene(repo)
        write_bytes(repo, "alt-backend/app/alt-backend", c20_after)
        res_a = run_hygiene(repo)
        check(
            "corpus C-20 before passes and after fails with [hygiene.binary-artifact]",
            res_b.returncode == 0
            and res_a.returncode == 1
            and "[hygiene.binary-artifact]" in (res_a.stdout + res_a.stderr),
            f"before={res_b.returncode} after={res_a.returncode} out={res_a.stdout + res_a.stderr}",
        )


def test_cli_errors_and_real_repo() -> None:
    with tempfile.TemporaryDirectory() as tmp:
        res_nongit = run_hygiene(pathlib.Path(tmp))
        check(
            "non-git --repo-root exits 2",
            res_nongit.returncode == USAGE_RC,
            f"rc={res_nongit.returncode} out={res_nongit.stdout + res_nongit.stderr}",
        )

    with tempfile.TemporaryDirectory() as tmp:
        repo = init_git_repo(pathlib.Path(tmp))
        write_bytes(repo, "a.txt", b"ok\n")
        git(repo, "add", "-A")
        git(repo, "commit", "-m", "init")
        res_bad_al = run_hygiene(repo, ["--allowlist", str(repo / "missing.toml")])
        check(
            "missing explicit --allowlist exits 2",
            res_bad_al.returncode == USAGE_RC,
            f"rc={res_bad_al.returncode} out={res_bad_al.stdout + res_bad_al.stderr}",
        )

    check("empty_file_allowlist.toml exists", ALLOWLIST_PATH.is_file(), str(ALLOWLIST_PATH))
    res_real = run_hygiene(ROOT)
    check(
        "real repository passes worktree hygiene check",
        res_real.returncode == 0,
        res_real.stdout + res_real.stderr,
    )


def main() -> int:
    test_zero_byte_checks()
    test_binary_artifact_checks()
    test_exclusions_and_corpus()
    test_cli_errors_and_real_repo()
    sys.stdout.write(f"\nResults: {RESULTS.passed} passed, {RESULTS.failed} failed\n")
    return 0 if RESULTS.failed == 0 else 1


if __name__ == "__main__":
    sys.exit(main())
