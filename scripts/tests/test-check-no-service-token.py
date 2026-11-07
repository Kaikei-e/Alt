#!/usr/bin/env python3
"""Tests for scripts/check-no-service-token.sh.

Run:
    python3 scripts/tests/test-check-no-service-token.py
"""

from __future__ import annotations

import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[2]
SCRIPT = ROOT / "scripts" / "check-no-service-token.sh"


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
        with tempfile.TemporaryDirectory() as tmp:
            script = self._setup_tree(
                tmp,
                {"src/main.go": "package main\n\nfunc main() {}\n"},
            )
            env = self._clean_env()
            res = subprocess.run(
                ["bash", str(script)],
                env=env,
                stdin=subprocess.DEVNULL,
                capture_output=True,
                text=True,
                check=False,
                timeout=10,
            )
            self.assertEqual(
                res.returncode,
                0,
                f"expected exit 0 on clean tree, got rc={res.returncode} stdout={res.stdout!r} stderr={res.stderr!r}",
            )
            self.assertIn("PASS", res.stdout)

            strict_res = subprocess.run(
                ["bash", str(script), "--strict"],
                env=env,
                stdin=subprocess.DEVNULL,
                capture_output=True,
                text=True,
                check=False,
                timeout=10,
            )
            self.assertEqual(strict_res.returncode, 0)
            self.assertIn("PASS", strict_res.stdout)

    def test_single_matching_file_report_only_exits_zero_and_reports_hit(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            script = self._setup_tree(
                tmp,
                {"src/main.go": 'package main\n\nconst h = "X-Service-Token"\n'},
            )
            res = subprocess.run(
                ["bash", str(script)],
                env=self._clean_env(),
                stdin=subprocess.DEVNULL,
                capture_output=True,
                text=True,
                check=False,
                timeout=10,
            )
            self.assertEqual(
                res.returncode,
                0,
                f"expected exit 0 in report-only mode, got rc={res.returncode} stdout={res.stdout!r} stderr={res.stderr!r}",
            )
            self.assertIn("[found 1] X-Service-Token", res.stdout)
            self.assertIn("FOUND 1 residue references", res.stdout)

    def test_single_matching_file_strict_exits_nonzero(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            script = self._setup_tree(
                tmp,
                {"src/main.go": 'package main\n\nconst h = "X-Service-Token"\n'},
            )
            res = subprocess.run(
                ["bash", str(script), "--strict"],
                env=self._clean_env(),
                stdin=subprocess.DEVNULL,
                capture_output=True,
                text=True,
                check=False,
                timeout=10,
            )
            self.assertNotEqual(res.returncode, 0)
            self.assertIn("[found 1] X-Service-Token", res.stdout)
            self.assertIn("FOUND 1 residue references", res.stdout)

    def test_piped_stdin_still_scans_worktree_in_strict_mode(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            script = self._setup_tree(
                tmp,
                {"src/main.go": 'package main\n\nconst h = "X-Service-Token"\n'},
            )
            res = subprocess.run(
                ["bash", str(script), "--strict"],
                env=self._clean_env(),
                input="clean piped stdin with no matches\n",
                capture_output=True,
                text=True,
                check=False,
                timeout=10,
            )
            self.assertNotEqual(
                res.returncode,
                0,
                f"expected non-zero exit in --strict mode with piped stdin, got rc={res.returncode} stdout={res.stdout!r}",
            )
            self.assertIn("[found 1] X-Service-Token", res.stdout)
            self.assertIn("FOUND 1 residue references", res.stdout)

    def test_rg_error_exit_2_fails_script(self) -> None:
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
            res = subprocess.run(
                ["bash", str(script)],
                env=self._clean_env(extra_bin=bin_dir),
                stdin=subprocess.DEVNULL,
                capture_output=True,
                text=True,
                check=False,
                timeout=10,
            )
            self.assertNotEqual(
                res.returncode,
                0,
                "expected non-zero exit when rg fails with exit code 2",
            )
            self.assertIn("X-Service-Token", res.stderr)
            self.assertIn("2", res.stderr)


if __name__ == "__main__":
    unittest.main()
