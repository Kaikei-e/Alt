"""Test module to ensure ruff compliance across the codebase.

This module implements TDD for code quality by testing that all Python files
pass ruff linting and formatting checks.
"""

import subprocess
import sys
from pathlib import Path


class TestRuffCompliance:
    """Test class to verify ruff compliance."""

    def test_ruff_check_passes(self):
        """Test that all Python files pass ruff linting checks."""
        # Get the app directory path
        app_dir = Path(__file__).parent.parent.parent

        # Run ruff check
        result = subprocess.run(
            [sys.executable, "-m", "ruff", "check", "."],
            cwd=app_dir,
            capture_output=True,
            text=True,
        )

        # Assert that ruff check passes (exit code 0)
        assert result.returncode == 0, f"Ruff check failed:\n{result.stdout}\n{result.stderr}"

    def test_ruff_format_check_passes(self):
        """Test that all Python files pass ruff formatting checks."""
        # Get the app directory path
        app_dir = Path(__file__).parent.parent.parent

        # Run ruff format --check
        result = subprocess.run(
            [sys.executable, "-m", "ruff", "format", "--check", "."],
            cwd=app_dir,
            capture_output=True,
            text=True,
        )

        # Assert that ruff format check passes (exit code 0)
        assert result.returncode == 0, f"Ruff format check failed:\n{result.stdout}\n{result.stderr}"
