#!/usr/bin/env python3
"""Tests for scripts/check-no-service-token.sh.

Run:
    python3 scripts/tests/test-check-no-service-token.py
"""

from __future__ import annotations

import os
import shutil
import subprocess
import tempfile
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
SCRIPT = ROOT / "scripts" / "check-no-service-token.sh"
BASH_BIN = shutil.which("bash")
assert BASH_BIN is not None, "bash executable not found in PATH"


class CheckNoServiceTokenTests(unittest.TestCase):
    def _clean_env(self, extra_bin: Path | None = None) -> dict[str, str]:
        env = dict(os.environ)
        env.pop("RIPGREP_CONFIG_PATH", None)
        if extra_bin is not None:
            env["PATH"] = f"{extra_bin}:{env.get('PATH', '')}"
        return env

    def _setup_tree(self, tmp: str, files: dict[str, str]) -> Path:
        root = Path(tmp)
        script_dst = root / "scripts" / "check-no-service-token.sh"
        script_dst.parent.mkdir(parents=True, exist_ok=True)
        shutil.copy2(SCRIPT, script_dst)
        script_dst.chmod(0o755)
        for rel, content in files.items():
            path = root / rel
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_text(content, encoding="utf-8")
        return script_dst

    def test_clean_tree_exits_zero_and_prints_pass(self) -> None:
        assert BASH_BIN is not None
        with tempfile.TemporaryDirectory() as tmp:
            script = self._setup_tree(
                tmp,
                {"src/main.go": "package main\n\nfunc main() {}\n"},
            )
            env = self._clean_env()
            res = subprocess.run(  # noqa: S603 - fixed argv built in this test
                [BASH_BIN, str(script)],
                env=env,
                stdin=subprocess.DEVNULL,
                capture_output=True,
                text=True,
                check=False,
                timeout=10,
            )
            assert res.returncode == 0, (
                f"expected exit 0 on clean tree, got rc={res.returncode} stdout={res.stdout!r} stderr={res.stderr!r}"
            )
            assert "PASS" in res.stdout, res.stdout

            strict_res = subprocess.run(  # noqa: S603 - fixed argv built in this test
                [BASH_BIN, str(script), "--strict"],
                env=env,
                stdin=subprocess.DEVNULL,
                capture_output=True,
                text=True,
                check=False,
                timeout=10,
            )
            assert strict_res.returncode == 0, strict_res.stderr
            assert "PASS" in strict_res.stdout, strict_res.stdout

    def test_single_matching_file_report_only_exits_zero_and_reports_hit(self) -> None:
        assert BASH_BIN is not None
        with tempfile.TemporaryDirectory() as tmp:
            script = self._setup_tree(
                tmp,
                {"src/main.go": 'package main\n\nconst h = "X-Service-Token"\n'},
            )
            res = subprocess.run(  # noqa: S603 - fixed argv built in this test
                [BASH_BIN, str(script)],
                env=self._clean_env(),
                stdin=subprocess.DEVNULL,
                capture_output=True,
                text=True,
                check=False,
                timeout=10,
            )
            assert res.returncode == 0, (
                f"expected exit 0 in report-only mode, got rc={res.returncode} stdout={res.stdout!r} stderr={res.stderr!r}"
            )
            assert "[found 1] X-Service-Token" in res.stdout, res.stdout
            assert "FOUND 1 residue references" in res.stdout, res.stdout

    def test_single_matching_file_strict_exits_nonzero(self) -> None:
        assert BASH_BIN is not None
        with tempfile.TemporaryDirectory() as tmp:
            script = self._setup_tree(
                tmp,
                {"src/main.go": 'package main\n\nconst h = "X-Service-Token"\n'},
            )
            res = subprocess.run(  # noqa: S603 - fixed argv built in this test
                [BASH_BIN, str(script), "--strict"],
                env=self._clean_env(),
                stdin=subprocess.DEVNULL,
                capture_output=True,
                text=True,
                check=False,
                timeout=10,
            )
            assert res.returncode != 0, f"rc={res.returncode} stdout={res.stdout!r}"
            assert "[found 1] X-Service-Token" in res.stdout, res.stdout
            assert "FOUND 1 residue references" in res.stdout, res.stdout

    def test_piped_stdin_still_scans_worktree_in_strict_mode(self) -> None:
        assert BASH_BIN is not None
        with tempfile.TemporaryDirectory() as tmp:
            script = self._setup_tree(
                tmp,
                {"src/main.go": 'package main\n\nconst h = "X-Service-Token"\n'},
            )
            res = subprocess.run(  # noqa: S603 - fixed argv built in this test
                [BASH_BIN, str(script), "--strict"],
                env=self._clean_env(),
                input="clean piped stdin with no matches\n",
                capture_output=True,
                text=True,
                check=False,
                timeout=10,
            )
            assert res.returncode != 0, (
                f"expected non-zero exit in --strict mode with piped stdin, got rc={res.returncode} stdout={res.stdout!r}"
            )
            assert "[found 1] X-Service-Token" in res.stdout, res.stdout
            assert "FOUND 1 residue references" in res.stdout, res.stdout

    def test_rg_error_exit_2_fails_script(self) -> None:
        assert BASH_BIN is not None
        with tempfile.TemporaryDirectory() as tmp:
            script = self._setup_tree(
                tmp,
                {"src/main.go": "package main\n"},
            )
            bin_dir = Path(tmp) / "bin"
            bin_dir.mkdir()
            stub_rg = bin_dir / "rg"
            stub_rg.write_text("#!/usr/bin/env bash\nexit 2\n", encoding="utf-8")
            stub_rg.chmod(0o755)
            res = subprocess.run(  # noqa: S603 - fixed argv built in this test
                [BASH_BIN, str(script)],
                env=self._clean_env(extra_bin=bin_dir),
                stdin=subprocess.DEVNULL,
                capture_output=True,
                text=True,
                check=False,
                timeout=10,
            )
            assert res.returncode != 0, "expected non-zero exit when rg fails with exit code 2"
            assert "X-Service-Token" in res.stderr, res.stderr
            assert "2" in res.stderr, res.stderr


if __name__ == "__main__":
    unittest.main()
