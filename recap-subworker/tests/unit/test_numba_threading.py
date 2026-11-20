"""Tests for Numba threading layer configuration."""

import os
import subprocess
import sys
from pathlib import Path

from recap_subworker.app import main  # noqa: F401 - triggers NUMBA_* env setup


class TestNumbaThreadingLayer:
    """Numba threading layer must be TBB to avoid concurrent access errors."""

    def test_numba_threading_layer_set_to_tbb(self) -> None:
        """NUMBA_THREADING_LAYER should be set to 'tbb' before numba is imported."""
        assert os.environ.get("NUMBA_THREADING_LAYER") == "tbb"

    def test_numba_num_threads_set_to_one(self) -> None:
        """NUMBA_NUM_THREADS should be set to '1' by default to avoid host core exhaustion."""
        assert os.environ.get("NUMBA_NUM_THREADS") == "1"

    def test_tbb_shared_library_exists(self) -> None:
        """libtbb.so must exist in the venv for Numba to use TBB threading."""
        venv_lib = Path(sys.prefix) / "lib"
        tbb_files = [f.name for f in venv_lib.iterdir() if f.name.startswith("libtbb.so")]
        assert len(tbb_files) > 0, f"libtbb.so not found in {venv_lib}"

    def test_numba_can_use_tbb(self) -> None:
        """Numba must actually load TBB threading layer in a subprocess."""
        venv_lib = str(Path(sys.prefix) / "lib")
        env = os.environ.copy()
        env["NUMBA_THREADING_LAYER"] = "tbb"
        env["LD_LIBRARY_PATH"] = venv_lib + ":" + env.get("LD_LIBRARY_PATH", "")

        result = subprocess.run(
            [
                sys.executable,
                "-c",
                "from numba import njit, prange\n"
                "@njit(parallel=True)\n"
                "def f():\n"
                "    s=0\n"
                "    for i in prange(10): s+=i\n"
                "    return s\n"
                "f()\n"
                "import numba; print(numba.threading_layer())",
            ],
            capture_output=True,
            text=True,
            env=env,
            timeout=60,
            check=False,
        )
        assert result.returncode == 0, f"Numba TBB test failed: {result.stderr}"
        assert "tbb" in result.stdout.strip(), f"Expected 'tbb', got: {result.stdout}"
