"""Tests for Numba threading layer configuration."""

import os
import subprocess
import sys


class TestNumbaThreadingLayer:
    """Numba threading layer must be TBB to avoid concurrent access errors."""

    def test_numba_threading_layer_set_to_tbb(self):
        """NUMBA_THREADING_LAYER should be set to 'tbb' before numba is imported."""
        from recap_subworker.app import main  # noqa: F401

        assert os.environ.get("NUMBA_THREADING_LAYER") == "tbb"

    def test_tbb_shared_library_exists(self):
        """libtbb.so must exist in the venv for Numba to use TBB threading."""
        venv_lib = os.path.join(sys.prefix, "lib")
        tbb_files = [f for f in os.listdir(venv_lib) if f.startswith("libtbb.so")]
        assert len(tbb_files) > 0, f"libtbb.so not found in {venv_lib}"

    def test_numba_can_use_tbb(self):
        """Numba must actually load TBB threading layer in a subprocess."""
        venv_lib = os.path.join(sys.prefix, "lib")
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
        )
        assert result.returncode == 0, f"Numba TBB test failed: {result.stderr}"
        assert "tbb" in result.stdout.strip(), f"Expected 'tbb', got: {result.stdout}"
