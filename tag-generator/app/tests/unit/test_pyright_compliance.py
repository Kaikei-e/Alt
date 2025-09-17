"""Test module to ensure pyright type checking compliance.

This module implements TDD for type safety by testing that all Python files
pass pyright type checking.
"""

import subprocess
import sys
from pathlib import Path


class TestPyrightCompliance:
    """Test class to verify pyright type checking compliance."""

    def test_pyright_check_passes(self):
        """Test that all Python files pass pyright type checking."""
        # Get the app directory path
        app_dir = Path(__file__).parent.parent.parent

        # Run pyright check
        result = subprocess.run(
            [sys.executable, "-m", "pyright", "."],
            cwd=app_dir,
            capture_output=True,
            text=True,
        )

        # Assert that pyright check passes (exit code 0)
        assert result.returncode == 0, f"Pyright check failed:\n{result.stdout}\n{result.stderr}"
