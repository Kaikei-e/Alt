"""Test module to ensure pyrefly type checking compliance.

This module implements TDD for type safety by testing that all Python files
pass pyrefly type checking.
"""

import subprocess
import sys
from pathlib import Path


class TestPyreflyCompliance:
    """Test class to verify pyrefly type checking compliance."""

    def test_pyrefly_check_passes(self):
        """Test that all Python files pass pyrefly type checking."""
        # Get the app directory path
        app_dir = Path(__file__).parent.parent.parent

        # Run pyrefly check
        result = subprocess.run(
            [sys.executable, "-m", "pyrefly", "check", "."],
            cwd=app_dir,
            capture_output=True,
            text=True,
        )

        # Assert that pyrefly check passes (exit code 0)
        assert result.returncode == 0, f"Pyrefly check failed:\n{result.stdout}\n{result.stderr}"
