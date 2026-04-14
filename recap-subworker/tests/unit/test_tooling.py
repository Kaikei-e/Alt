"""Tooling baseline tests.

Asserts that Ruff and Pyrefly pass cleanly over the production package.
Enforces the Phase 0 tooling hardening invariant: zero lint / type errors
at the source of truth (pyproject.toml).
"""

from __future__ import annotations

import shutil
import subprocess
from pathlib import Path

import pytest

SERVICE_ROOT = Path(__file__).resolve().parents[2]
PACKAGE_DIR = SERVICE_ROOT / "recap_subworker"


def _have_uv() -> bool:
    return shutil.which("uv") is not None


@pytest.mark.skipif(not _have_uv(), reason="uv not installed on this machine")
def test_ruff_passes_on_production_package() -> None:
    result = subprocess.run(  # noqa: S603
        ["uv", "run", "ruff", "check", str(PACKAGE_DIR)],
        check=False,
        capture_output=True,
        text=True,
        cwd=SERVICE_ROOT,
    )
    assert result.returncode == 0, (
        "ruff check failed:\n" + result.stdout + "\n" + result.stderr
    )


@pytest.mark.skipif(not _have_uv(), reason="uv not installed on this machine")
def test_pyrefly_passes_on_project() -> None:
    result = subprocess.run(  # noqa: S603
        ["uv", "run", "pyrefly", "check", "."],
        check=False,
        capture_output=True,
        text=True,
        cwd=SERVICE_ROOT,
    )
    assert result.returncode == 0, (
        "pyrefly check failed:\n" + result.stdout + "\n" + result.stderr
    )
