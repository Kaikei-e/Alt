"""Tests for full bench_vram_rtf execution flow, HTTP sequence, and main() exit codes."""

from __future__ import annotations

import io
import json
import subprocess
import sys
import tempfile
import unittest
import wave
from pathlib import Path

BENCH_DIR = Path(__file__).resolve().parent.parent
if str(BENCH_DIR) not in sys.path:
    sys.path.insert(0, str(BENCH_DIR))

from bench_vram_rtf import SetupError, format_summary, main, run_benchmark


def make_dummy_wav(duration_s: float = 1.0, sample_rate: int = 48000) -> bytes:
    """Generate minimal valid PCM WAV bytes."""
    buf = io.BytesIO()
    total_frames = int(duration_s * sample_rate)
    with wave.open(buf, "wb") as wf:
        wf.setnchannels(1)
        wf.setsampwidth(2)
        wf.setframerate(sample_rate)
        wf.writeframes(b"\x00\x00" * total_frames)
    return buf.getvalue()


class TestBenchVramRtfFlow(unittest.TestCase):
    def setUp(self):
        self.dummy_wav = make_dummy_wav(1.0)
        self.inputs = [
            "これは１行目のテスト用合成テキストです。文字数を適切に設定してテストを実施します。",
            "これは２行目のテスト用合成テキストです。文字数を適切に設定してテストを実施します。",
        ]

    def test_run_benchmark_success_sequence_and_json_shape(self):
        http_sequence: list[tuple[str, str]] = []

        def fake_http(url, method="GET", headers=None, data=None, timeout=60.0):
            # Normalize path
            path = url.split("127.0.0.1:8088")[-1]
            http_sequence.append((method, path))
            if path == "/v1/audio/speech":
                return 200, self.dummy_wav, {"content-type": "audio/wav"}
            if path == "/internal/cuda-memory/reset-peak":
                return 200, b'{"status": "ok"}', {"content-type": "application/json"}
            if path == "/internal/cuda-memory":
                # First call is baseline after reset, second call is final probe
                probe = {
                    "device": "cuda:0",
                    "allocated_bytes": 1048576000,
                    "reserved_bytes": 1258291200,  # 1200 MiB
                    "max_allocated_bytes": 2097152000,  # 2000 MiB
                    "max_reserved_bytes": 2306867200,  # 2200 MiB
                }
                return (
                    200,
                    json.dumps(probe).encode("utf-8"),
                    {"content-type": "application/json"},
                )
            return 404, b"Not Found", {}

        def fake_apps_fn():
            return "1234, 2100.0\n"

        def fake_gpu_fn():
            return "3500.0\n"

        # Deterministic timestamps for exact RTF percentiles (duration is 1.0s)
        # Warmup (2 calls): 4 timestamps
        # Measured (5 calls): 10 timestamps, durations: 0.2, 0.4, 0.6, 0.8, 1.0
        # -> RTFs: [0.2, 0.4, 0.6, 0.8, 1.0] -> p50=0.6, p95=0.96
        timestamps = [
            0.0,
            0.5,
            0.5,
            1.0,
            1.0,
            1.2,
            1.2,
            1.6,
            1.6,
            2.2,
            2.2,
            3.0,
            3.0,
            4.0,
        ]
        ts_iter = iter(timestamps)

        import bench_vram_rtf

        orig_perf_counter = bench_vram_rtf.time.perf_counter
        try:
            bench_vram_rtf.time.perf_counter = lambda: next(ts_iter)
            result = run_benchmark(
                label="C1",
                base_url="http://127.0.0.1:8088",
                api_key="test-secret-key",
                voice="default",
                input_lines=self.inputs,
                count=5,
                warmup=2,
                pid=1234,
                sample_interval_ms=10,
                budget_mib=1000.0,
                budget_ratio=0.9,
                rtf_max=1.0,
                http_fn=fake_http,
                query_apps_fn=fake_apps_fn,
                query_gpu_fn=fake_gpu_fn,
            )
        finally:
            bench_vram_rtf.time.perf_counter = orig_perf_counter

        # Verify exact call sequence:
        # 1. 2 warmup speech calls
        # 2. 1 reset-peak probe
        # 3. 1 baseline probe query for context overhead
        # 4. 5 measured speech calls
        # 5. 1 final probe query
        expected_sequence = (
            [("POST", "/v1/audio/speech")] * 2
            + [("POST", "/internal/cuda-memory/reset-peak")]
            + [("GET", "/internal/cuda-memory")]
            + [("POST", "/v1/audio/speech")] * 5
            + [("GET", "/internal/cuda-memory")]
        )
        self.assertEqual(http_sequence, expected_sequence)

        # Verify nested shape only (no flat rtf_mean, etc.)
        self.assertNotIn("rtf_mean", result)
        self.assertNotIn("rtf_p50", result)
        self.assertNotIn("rtf_p95", result)
        self.assertNotIn("latency_p50", result)
        self.assertIn("rtf", result)
        self.assertIn("latency_seconds", result)

        # Exact end-to-end RTF assertions from deterministic fake clock
        self.assertEqual(result["rtf"]["p50"], 0.6)
        self.assertEqual(result["rtf"]["p95"], 0.96)
        self.assertEqual(result["rtf"]["mean"], 0.6)
        self.assertEqual(result["rtf"]["max"], 1.0)

        # Check telemetry and effective peak
        self.assertGreater(result["sample_count"], 0)
        self.assertGreater(result["pid_hits"], 0)
        self.assertEqual(result["peak_process_mib"], 2100.0)
        self.assertAlmostEqual(result["peak_reserved_mib"], 2200.0, places=1)
        # initial process_mib = 2100, initial reserved_mib = 1200 -> overhead = 900
        # effective_peak = max(2100, 2200 + 900) = 3100
        self.assertAlmostEqual(result["context_overhead_mib"], 900.0, places=1)
        self.assertAlmostEqual(result["effective_peak_mib"], 3100.0, places=1)
        # Limit is 1000 * 0.9 = 900.0, so 3100 exceeds limit -> budget_pass is False
        self.assertFalse(result["budget_pass"])

        # Summary formatting test
        summary = format_summary(result)
        self.assertIn("Irodori-TTS Benchmark Summary: C1", summary)
        self.assertIn("Effective Peak:", summary)

    def test_run_benchmark_unparseable_wav_treated_as_failure(self):
        def fake_http(url, method="GET", headers=None, data=None, timeout=60.0):
            if "/v1/audio/speech" in url:
                return 200, b"corrupted bytes", {}
            return 200, b"{}", {}

        result = run_benchmark(
            label="C2",
            base_url="http://127.0.0.1:8088",
            api_key="key",
            voice="default",
            input_lines=self.inputs,
            budget_mib=1000.0,
            count=5,
            warmup=1,
            pid=1234,
            http_fn=fake_http,
            query_apps_fn=lambda: "1234, 100.0",
            query_gpu_fn=lambda: "500.0",
        )
        self.assertEqual(result["failures"], 1)
        self.assertEqual(result["failure_detail"]["error"], "unparseable WAV")
        self.assertFalse(result["all_passed"])

    def test_run_benchmark_warmup_connection_refused_raises_setup_error(self):
        def fake_http_refused(url, method="GET", headers=None, data=None, timeout=60.0):
            return 0, b"Connection refused", {}

        with self.assertRaises(SetupError) as ctx:
            run_benchmark(
                label="C3",
                base_url="http://127.0.0.1:8088",
                api_key="key",
                voice="default",
                input_lines=self.inputs,
                budget_mib=1000.0,
                count=5,
                warmup=1,
                pid=1234,
                http_fn=fake_http_refused,
                query_apps_fn=lambda: "",
                query_gpu_fn=lambda: "",
            )
        self.assertIn("warmup", str(ctx.exception).lower())

    def test_run_benchmark_probe_failures_raise_setup_error(self):
        def fake_http_reset_fail(
            url, method="GET", headers=None, data=None, timeout=60.0
        ):
            if "/v1/audio/speech" in url:
                return 200, self.dummy_wav, {}
            if "/internal/cuda-memory/reset-peak" in url:
                return 500, b"Internal reset failure", {}
            return 200, b"{}", {}

        with self.assertRaises(SetupError) as ctx:
            run_benchmark(
                label="C4",
                base_url="http://127.0.0.1:8088",
                api_key="key",
                voice="default",
                input_lines=self.inputs,
                budget_mib=1000.0,
                count=5,
                warmup=1,
                pid=1234,
                http_fn=fake_http_reset_fail,
                query_apps_fn=lambda: "1234, 100.0",
                query_gpu_fn=lambda: "500.0",
            )
        self.assertIn("reset peak", str(ctx.exception).lower())

    def test_run_benchmark_context_overhead_q_apps_failure_raises_setup_error(self):
        def fake_q_apps_fail():
            raise TimeoutError("nvidia-smi timed out")

        with self.assertRaises(SetupError) as ctx:
            run_benchmark(
                label="C-fail",
                base_url="http://127.0.0.1:8088",
                api_key="key",
                voice="default",
                input_lines=self.inputs,
                budget_mib=1000.0,
                count=2,
                warmup=1,
                pid=1234,
                http_fn=lambda *a, **k: (200, self.dummy_wav, {}),
                query_apps_fn=fake_q_apps_fail,
                query_gpu_fn=lambda: "500.0\n",
            )
        self.assertIn("context overhead", str(ctx.exception).lower())

    def test_run_benchmark_mid_run_crash_with_probe_failure_sets_probe_null(self):
        def fake_http(url, method="GET", headers=None, data=None, timeout=60.0):
            if "/internal/cuda-memory/reset-peak" in url:
                return 200, b"{}", {}
            if "/internal/cuda-memory" in url:
                if getattr(fake_http, "baseline_done", False):
                    return 500, b"Internal probe error after crash", {}
                fake_http.baseline_done = True
                return (
                    200,
                    json.dumps({"reserved_bytes": 100 * 1024 * 1024}).encode("utf-8"),
                    {},
                )
            if "/v1/audio/speech" in url:
                if getattr(fake_http, "warmup_done", False):
                    return 500, b"Server crashed mid-run", {}
                fake_http.warmup_done = True
                return 200, self.dummy_wav, {}
            return 404, b"", {}

        result = run_benchmark(
            label="C-crash",
            base_url="http://127.0.0.1:8088",
            api_key="key",
            voice="default",
            input_lines=self.inputs,
            budget_mib=1000.0,
            count=5,
            warmup=1,
            pid=1234,
            http_fn=fake_http,
            query_apps_fn=lambda: "1234, 1500.0\n",
            query_gpu_fn=lambda: "2000.0\n",
        )
        self.assertEqual(result["failures"], 1)
        self.assertIsNone(result["peak_allocated_mib"])
        self.assertIsNone(result["peak_reserved_mib"])
        self.assertFalse(result["all_passed"])
        self.assertFalse(result["budget_pass"])


class TestMainCli(unittest.TestCase):
    """Test main() covering exit codes 0, 1, 2 and asserting API key is never leaked."""

    def setUp(self):
        self.dummy_wav = make_dummy_wav(1.0)
        self.secret_token = "SECRET_TOKEN_DO_NOT_LEAK_XYZ123"
        self.orig_subprocess_run = subprocess.run

        def guard_subprocess(*a, **k):
            raise AssertionError(f"Unpatched host subprocess call: {a} {k}")

        subprocess.run = guard_subprocess

    def tearDown(self):
        subprocess.run = self.orig_subprocess_run

    def test_main_exit_0_gate_pass_and_no_key_leak(self):
        with tempfile.TemporaryDirectory() as tmp_dir:
            tmp = Path(tmp_dir)
            key_file = tmp / "api_key.txt"
            key_file.write_text(self.secret_token, encoding="utf-8")

            inputs_file = tmp / "inputs.txt"
            inputs_file.write_text("テスト文章です。\n", encoding="utf-8")

            out_dir = tmp / "results"

            def fake_http(url, method="GET", headers=None, data=None, timeout=60.0):
                if "/v1/audio/speech" in url:
                    return 200, self.dummy_wav, {}
                if "/internal/cuda-memory/reset-peak" in url:
                    return 200, b'{"status": "ok"}', {}
                if "/internal/cuda-memory" in url:
                    # Low VRAM to pass budget
                    probe = {
                        "device": "cuda:0",
                        "allocated_bytes": 200 * 1024 * 1024,
                        "reserved_bytes": 300 * 1024 * 1024,
                        "max_allocated_bytes": 400 * 1024 * 1024,
                        "max_reserved_bytes": 500 * 1024 * 1024,
                    }
                    return 200, json.dumps(probe).encode("utf-8"), {}
                return 404, b"", {}

            import bench_vram_rtf

            orig_http = bench_vram_rtf.default_http_request
            orig_check = bench_vram_rtf.check_nvidia_smi
            orig_apps = bench_vram_rtf.run_query_compute_apps
            orig_gpu = bench_vram_rtf.run_query_gpu_used
            try:
                bench_vram_rtf.default_http_request = fake_http
                bench_vram_rtf.check_nvidia_smi = lambda: True
                bench_vram_rtf.run_query_compute_apps = lambda: "999, 600.0\n"
                bench_vram_rtf.run_query_gpu_used = lambda: "800.0\n"

                stdout_buf = io.StringIO()
                old_stdout = sys.stdout
                sys.stdout = stdout_buf
                try:
                    code = main(
                        [
                            "--label",
                            "C1",
                            "--api-key-file",
                            str(key_file),
                            "--inputs",
                            str(inputs_file),
                            "--count",
                            "2",
                            "--warmup",
                            "1",
                            "--pid",
                            "999",
                            "--budget-mib",
                            "1000",
                            "--out-dir",
                            str(out_dir),
                        ]
                    )
                finally:
                    sys.stdout = old_stdout

                stdout_text = stdout_buf.getvalue()
                self.assertEqual(code, 0)

                # Assert API key is neither in stdout nor in the saved JSON file
                self.assertNotIn(self.secret_token, stdout_text)

                json_files = list(out_dir.glob("*.json"))
                self.assertEqual(len(json_files), 1)
                json_content = json_files[0].read_text(encoding="utf-8")
                self.assertNotIn(self.secret_token, json_content)

            finally:
                bench_vram_rtf.default_http_request = orig_http
                bench_vram_rtf.check_nvidia_smi = orig_check
                bench_vram_rtf.run_query_compute_apps = orig_apps
                bench_vram_rtf.run_query_gpu_used = orig_gpu

    def test_main_exit_1_gate_fail(self):
        with tempfile.TemporaryDirectory() as tmp_dir:
            tmp = Path(tmp_dir)
            key_file = tmp / "api_key.txt"
            key_file.write_text(self.secret_token, encoding="utf-8")

            inputs_file = tmp / "inputs.txt"
            inputs_file.write_text("テスト文章です。\n", encoding="utf-8")

            out_dir = tmp / "results"

            def fake_http(url, method="GET", headers=None, data=None, timeout=60.0):
                if "/v1/audio/speech" in url:
                    return 200, self.dummy_wav, {}
                if "/internal/cuda-memory/reset-peak" in url:
                    return 200, b"{}", {}
                if "/internal/cuda-memory" in url:
                    # Very high VRAM to fail budget
                    probe = {
                        "device": "cuda:0",
                        "allocated_bytes": 4000 * 1024 * 1024,
                        "reserved_bytes": 4000 * 1024 * 1024,
                        "max_allocated_bytes": 4500 * 1024 * 1024,
                        "max_reserved_bytes": 4500 * 1024 * 1024,
                    }
                    return 200, json.dumps(probe).encode("utf-8"), {}
                return 404, b"", {}

            import bench_vram_rtf

            orig_http = bench_vram_rtf.default_http_request
            orig_check = bench_vram_rtf.check_nvidia_smi
            orig_apps = bench_vram_rtf.run_query_compute_apps
            orig_gpu = bench_vram_rtf.run_query_gpu_used
            try:
                bench_vram_rtf.default_http_request = fake_http
                bench_vram_rtf.check_nvidia_smi = lambda: True
                bench_vram_rtf.run_query_compute_apps = lambda: "999, 4500.0\n"
                bench_vram_rtf.run_query_gpu_used = lambda: "5000.0\n"

                stdout_buf = io.StringIO()
                old_stdout = sys.stdout
                sys.stdout = stdout_buf
                try:
                    code = main(
                        [
                            "--label",
                            "C2",
                            "--api-key-file",
                            str(key_file),
                            "--inputs",
                            str(inputs_file),
                            "--count",
                            "2",
                            "--warmup",
                            "1",
                            "--pid",
                            "999",
                            "--budget-mib",
                            "1000",
                            "--out-dir",
                            str(out_dir),
                        ]
                    )
                finally:
                    sys.stdout = old_stdout

                self.assertEqual(code, 1)
            finally:
                bench_vram_rtf.default_http_request = orig_http
                bench_vram_rtf.check_nvidia_smi = orig_check
                bench_vram_rtf.run_query_compute_apps = orig_apps
                bench_vram_rtf.run_query_gpu_used = orig_gpu

    def test_main_warmup_500_exits_1_with_warmup_failure_detail(self):
        with tempfile.TemporaryDirectory() as tmp_dir:
            tmp = Path(tmp_dir)
            key_file = tmp / "api_key.txt"
            key_file.write_text(self.secret_token, encoding="utf-8")
            inputs_file = tmp / "inputs.txt"
            inputs_file.write_text("テスト文章です。\n", encoding="utf-8")
            out_dir = tmp / "results"

            def fake_http(url, method="GET", headers=None, data=None, timeout=60.0):
                if "/v1/audio/speech" in url:
                    return 500, b"Warmup CUDA out of memory error", {}
                return 200, b"{}", {}

            import bench_vram_rtf

            orig_http = bench_vram_rtf.default_http_request
            orig_check = bench_vram_rtf.check_nvidia_smi
            try:
                bench_vram_rtf.default_http_request = fake_http
                bench_vram_rtf.check_nvidia_smi = lambda: True

                code = main(
                    [
                        "--label",
                        "C-warmup",
                        "--api-key-file",
                        str(key_file),
                        "--inputs",
                        str(inputs_file),
                        "--count",
                        "1",
                        "--warmup",
                        "1",
                        "--pid",
                        "999",
                        "--budget-mib",
                        "1000",
                        "--out-dir",
                        str(out_dir),
                    ]
                )
                self.assertEqual(code, 1)

                json_files = list(out_dir.glob("*.json"))
                self.assertEqual(len(json_files), 1)
                saved = json.loads(json_files[0].read_text(encoding="utf-8"))
                self.assertEqual(saved["failures"], 1)
                self.assertIsNotNone(saved["failure_detail"])
                self.assertEqual(saved["failure_detail"]["stage"], "warmup")
                self.assertEqual(saved["failure_detail"]["status"], 500)
                self.assertIn(
                    "Warmup CUDA out of memory", saved["failure_detail"]["error"]
                )
                self.assertFalse(saved["budget_pass"])
                self.assertFalse(saved["all_passed"])
            finally:
                bench_vram_rtf.default_http_request = orig_http
                bench_vram_rtf.check_nvidia_smi = orig_check

    def test_main_mid_run_crash_writes_json_and_exits_1(self):
        with tempfile.TemporaryDirectory() as tmp_dir:
            tmp = Path(tmp_dir)
            key_file = tmp / "api_key.txt"
            key_file.write_text(self.secret_token, encoding="utf-8")
            inputs_file = tmp / "inputs.txt"
            inputs_file.write_text("テスト文章です。\n", encoding="utf-8")
            out_dir = tmp / "results"

            call_count = 0

            def fake_http(url, method="GET", headers=None, data=None, timeout=60.0):
                nonlocal call_count
                if "/internal/cuda-memory/reset-peak" in url:
                    return 200, b"{}", {}
                if "/internal/cuda-memory" in url:
                    call_count += 1
                    if call_count > 1:
                        # Final probe GET after crash fails
                        return 500, b"Server crashed during execution", {}
                    return (
                        200,
                        json.dumps({"reserved_bytes": 100 * 1024 * 1024}).encode(
                            "utf-8"
                        ),
                        {},
                    )
                if "/v1/audio/speech" in url:
                    if call_count >= 1:
                        # Measured request fails
                        return 500, b"Speech generation failed mid-run", {}
                    # Warmup request succeeds
                    return 200, self.dummy_wav, {}
                return 404, b"", {}

            import bench_vram_rtf

            orig_http = bench_vram_rtf.default_http_request
            orig_check = bench_vram_rtf.check_nvidia_smi
            orig_apps = bench_vram_rtf.run_query_compute_apps
            orig_gpu = bench_vram_rtf.run_query_gpu_used
            try:
                bench_vram_rtf.default_http_request = fake_http
                bench_vram_rtf.check_nvidia_smi = lambda: True
                bench_vram_rtf.run_query_compute_apps = lambda: "999, 1500.0\n"
                bench_vram_rtf.run_query_gpu_used = lambda: "2000.0\n"

                code = main(
                    [
                        "--label",
                        "C-crash-main",
                        "--api-key-file",
                        str(key_file),
                        "--inputs",
                        str(inputs_file),
                        "--count",
                        "2",
                        "--warmup",
                        "1",
                        "--pid",
                        "999",
                        "--budget-mib",
                        "1000",
                        "--out-dir",
                        str(out_dir),
                    ]
                )
                self.assertEqual(code, 1)

                json_files = list(out_dir.glob("*.json"))
                self.assertEqual(len(json_files), 1)
                saved = json.loads(json_files[0].read_text(encoding="utf-8"))
                self.assertEqual(saved["failures"], 1)
                self.assertIsNone(saved["peak_allocated_mib"])
                self.assertIsNone(saved["peak_reserved_mib"])
                self.assertFalse(saved["all_passed"])
                self.assertFalse(saved["budget_pass"])
            finally:
                bench_vram_rtf.default_http_request = orig_http
                bench_vram_rtf.check_nvidia_smi = orig_check
                bench_vram_rtf.run_query_compute_apps = orig_apps
                bench_vram_rtf.run_query_gpu_used = orig_gpu

    def test_main_exit_2_setup_errors(self):
        # 1. Invalid label format
        code = main(
            [
                "--label",
                "Invalid Label with spaces!",
                "--api-key-file",
                "/nonexistent",
                "--budget-mib",
                "1000",
            ]
        )
        self.assertEqual(code, 2)

        # 2. Missing API key file
        code = main(
            [
                "--label",
                "C1",
                "--api-key-file",
                "/path/does/not/exist",
                "--budget-mib",
                "1000",
            ]
        )
        self.assertEqual(code, 2)

        with tempfile.TemporaryDirectory() as tmp_dir:
            tmp = Path(tmp_dir)
            key_file = tmp / "api_key.txt"
            key_file.write_text("secret", encoding="utf-8")
            inputs_file = tmp / "inputs.txt"
            inputs_file.write_text("テスト\n", encoding="utf-8")

            # 3. Missing inputs file
            code = main(
                [
                    "--label",
                    "C1",
                    "--api-key-file",
                    str(key_file),
                    "--inputs",
                    str(tmp / "nonexistent_inputs.txt"),
                    "--budget-mib",
                    "1000",
                ]
            )
            self.assertEqual(code, 2)

            import bench_vram_rtf

            orig_check = bench_vram_rtf.check_nvidia_smi
            orig_resolve = bench_vram_rtf.resolve_container_pid
            orig_apps = bench_vram_rtf.run_query_compute_apps
            orig_gpu = bench_vram_rtf.run_query_gpu_used
            orig_http = bench_vram_rtf.default_http_request
            try:
                # 4. nvidia-smi missing / execution failure
                bench_vram_rtf.check_nvidia_smi = lambda: False
                code = main(
                    [
                        "--label",
                        "C1",
                        "--api-key-file",
                        str(key_file),
                        "--inputs",
                        str(inputs_file),
                        "--budget-mib",
                        "1000",
                    ]
                )
                self.assertEqual(code, 2)

                # 5. docker inspect failure resolving PID
                bench_vram_rtf.check_nvidia_smi = lambda: True

                def fail_resolve(container_name, pid_arg=None):
                    raise bench_vram_rtf.SetupError(
                        f"docker inspect failed for '{container_name}'"
                    )

                bench_vram_rtf.resolve_container_pid = fail_resolve
                code = main(
                    [
                        "--label",
                        "C1",
                        "--api-key-file",
                        str(key_file),
                        "--inputs",
                        str(inputs_file),
                        "--container",
                        "bad-container",
                        "--budget-mib",
                        "1000",
                    ]
                )
                self.assertEqual(code, 2)

                # 6. PID hits == 0 at end
                bench_vram_rtf.resolve_container_pid = orig_resolve
                bench_vram_rtf.run_query_compute_apps = lambda: (
                    "1111, 500.0\n"
                )  # Target PID is 999
                bench_vram_rtf.run_query_gpu_used = lambda: "500.0\n"
                bench_vram_rtf.default_http_request = lambda *a, **k: (
                    200,
                    self.dummy_wav,
                    {},
                )
                out_results = tmp / "results"
                code = main(
                    [
                        "--label",
                        "C1",
                        "--api-key-file",
                        str(key_file),
                        "--inputs",
                        str(inputs_file),
                        "--count",
                        "1",
                        "--warmup",
                        "1",
                        "--pid",
                        "999",
                        "--budget-mib",
                        "1000",
                        "--out-dir",
                        str(out_results),
                    ]
                )
                self.assertEqual(code, 2)

                # Assert the saved JSON has budget_pass false and all_passed false when PID is absent
                json_files = list(out_results.glob("*.json"))
                self.assertEqual(len(json_files), 1)
                saved = json.loads(json_files[0].read_text(encoding="utf-8"))
                self.assertFalse(saved["budget_pass"])
                self.assertFalse(saved["all_passed"])
                self.assertEqual(saved["pid_hits"], 0)
            finally:
                bench_vram_rtf.check_nvidia_smi = orig_check
                bench_vram_rtf.resolve_container_pid = orig_resolve
                bench_vram_rtf.run_query_compute_apps = orig_apps
                bench_vram_rtf.run_query_gpu_used = orig_gpu
                bench_vram_rtf.default_http_request = orig_http

    def test_main_omitting_budget_mib_exits_2(self):
        stderr_buf = io.StringIO()
        old_stderr = sys.stderr
        sys.stderr = stderr_buf
        try:
            with self.assertRaises(SystemExit) as ctx:
                main(["--label", "C1", "--api-key-file", "/path/to/key"])
            self.assertEqual(ctx.exception.code, 2)
        finally:
            sys.stderr = old_stderr
        self.assertIn("--budget-mib", stderr_buf.getvalue())


if __name__ == "__main__":
    unittest.main()
