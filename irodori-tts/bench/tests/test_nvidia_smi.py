"""Tests for nvidia-smi CSV parsing, sampler telemetry, and docker inspect resolution."""

from __future__ import annotations

import subprocess
import sys
import unittest
from pathlib import Path

BENCH_DIR = Path(__file__).resolve().parent.parent
if str(BENCH_DIR) not in sys.path:
    sys.path.insert(0, str(BENCH_DIR))

from bench_vram_rtf import (
    SetupError,
    check_nvidia_smi,
    parse_compute_apps_csv,
    parse_gpu_used_csv,
    resolve_container_pid,
    start_vram_sampler,
)


class DummyCompletedProcess:
    def __init__(self, returncode: int = 0, stdout: str = "", stderr: str = ""):
        self.returncode = returncode
        self.stdout = stdout
        self.stderr = stderr


class TestNvidiaSmi(unittest.TestCase):
    def test_parse_compute_apps_empty_and_whitespace(self):
        self.assertEqual(parse_compute_apps_csv(""), {})
        self.assertEqual(parse_compute_apps_csv("   \n\n  "), {})

    def test_parse_compute_apps_na_values(self):
        output = "[N/A], [N/A]\n"
        self.assertEqual(parse_compute_apps_csv(output), {})

        mixed = "1234, [N/A]\n[N/A], 512\n5678, 1024\n"
        expected = {5678: 1024.0}
        self.assertEqual(parse_compute_apps_csv(mixed), expected)

    def test_parse_compute_apps_standard(self):
        output = "1001, 256\n1002, 512\n"
        self.assertEqual(parse_compute_apps_csv(output), {1001: 256.0, 1002: 512.0})

    def test_parse_compute_apps_with_whitespace_and_comments(self):
        output = "# comment line\n  1001 ,  512.5 \n 1002, 1024.0 \n"
        self.assertEqual(parse_compute_apps_csv(output), {1001: 512.5, 1002: 1024.0})

    def test_parse_compute_apps_duplicate_pid_sum(self):
        # Process running on multiple GPUs sums VRAM
        output = "1001, 500\n1001, 300\n"
        self.assertEqual(parse_compute_apps_csv(output), {1001: 800.0})

    def test_parse_gpu_used_empty_and_na(self):
        self.assertEqual(parse_gpu_used_csv(""), 0.0)
        self.assertEqual(parse_gpu_used_csv("[N/A]\n"), 0.0)

    def test_parse_gpu_used_single_and_multi_gpu(self):
        self.assertEqual(parse_gpu_used_csv("3200\n"), 3200.0)
        self.assertEqual(parse_gpu_used_csv("3200\n1800\n"), 5000.0)

    def test_sampler_with_target_pid_present(self):
        def fake_apps():
            return "1002, 1024.0\n9999, 512.0\n"

        def fake_gpu():
            return "3000.0\n"

        stop = start_vram_sampler(
            pid=1002,
            interval_ms=10,
            query_apps_fn=fake_apps,
            query_gpu_fn=fake_gpu,
        )
        peak_proc, peak_gpu, sample_count, pid_hits = stop()
        self.assertGreater(sample_count, 0)
        self.assertGreater(pid_hits, 0)
        self.assertEqual(peak_proc, 1024.0)
        self.assertEqual(peak_gpu, 3000.0)

    def test_sampler_with_target_pid_absent(self):
        def fake_apps():
            return "2001, 512.0\n2002, 256.0\n"

        def fake_gpu():
            return "2500.0\n"

        stop = start_vram_sampler(
            pid=1002,
            interval_ms=10,
            query_apps_fn=fake_apps,
            query_gpu_fn=fake_gpu,
        )
        peak_proc, peak_gpu, sample_count, pid_hits = stop()
        self.assertGreater(sample_count, 0)
        self.assertEqual(pid_hits, 0)
        self.assertEqual(peak_proc, 0.0)
        self.assertEqual(peak_gpu, 2500.0)

    def test_check_nvidia_smi_success(self):
        def fake_runner(cmd, **kwargs):
            return DummyCompletedProcess(0, "GPU 0: Example GPU", "")

        self.assertTrue(check_nvidia_smi(runner=fake_runner))

    def test_check_nvidia_smi_failure(self):
        def fake_runner_fail(cmd, **kwargs):
            return DummyCompletedProcess(127, "", "command not found")

        self.assertFalse(check_nvidia_smi(runner=fake_runner_fail))

        def fake_runner_exc(cmd, **kwargs):
            raise FileNotFoundError("nvidia-smi not found")

        self.assertFalse(check_nvidia_smi(runner=fake_runner_exc))

    def test_resolve_container_pid_explicit(self):
        self.assertEqual(resolve_container_pid("irodori-tts", pid_arg=1234), 1234)

    def test_resolve_container_pid_explicit_invalid(self):
        with self.assertRaises(SetupError):
            resolve_container_pid("irodori-tts", pid_arg=0)
        with self.assertRaises(SetupError):
            resolve_container_pid("irodori-tts", pid_arg=-5)

    def test_resolve_container_pid_docker_success(self):
        def fake_runner(cmd, **kwargs):
            self.assertIn("docker", cmd)
            self.assertIn("irodori-tts", cmd)
            return DummyCompletedProcess(0, "4567\n", "")

        pid = resolve_container_pid("irodori-tts", runner=fake_runner)
        self.assertEqual(pid, 4567)

    def test_resolve_container_pid_docker_failures(self):
        # Exit code != 0
        def fake_fail(cmd, **kwargs):
            return DummyCompletedProcess(1, "", "No such container")

        with self.assertRaises(SetupError):
            resolve_container_pid("irodori-tts", runner=fake_fail)

        # PID 0 (stopped container)
        def fake_zero(cmd, **kwargs):
            return DummyCompletedProcess(0, "0\n", "")

        with self.assertRaises(SetupError):
            resolve_container_pid("irodori-tts", runner=fake_zero)

        # Non-numeric output
        def fake_non_int(cmd, **kwargs):
            return DummyCompletedProcess(0, "not-a-number\n", "")

        with self.assertRaises(SetupError):
            resolve_container_pid("irodori-tts", runner=fake_non_int)

        # Subprocess exception
        def fake_exc(cmd, **kwargs):
            raise subprocess.SubprocessError("daemon unavailable")

        with self.assertRaises(SetupError):
            resolve_container_pid("irodori-tts", runner=fake_exc)


if __name__ == "__main__":
    unittest.main()
