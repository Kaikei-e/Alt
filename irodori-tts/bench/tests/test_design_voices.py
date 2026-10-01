"""Tests for design_voices.py execution flow and manifest output."""

from __future__ import annotations

import io
import json
import sys
import tempfile
import unittest
import wave
from pathlib import Path

BENCH_DIR = Path(__file__).resolve().parent.parent
if str(BENCH_DIR) not in sys.path:
    sys.path.insert(0, str(BENCH_DIR))

from design_voices import run_design_voices


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


class TestDesignVoices(unittest.TestCase):
    def setUp(self):
        self.dummy_wav = make_dummy_wav(2.5)

    def test_run_design_voices_success(self):
        recorded_requests = []

        def fake_http(url, method="GET", headers=None, data=None, timeout=60.0):
            payload = json.loads(data.decode("utf-8"))
            recorded_requests.append(payload)
            return 200, self.dummy_wav, {}

        with tempfile.TemporaryDirectory() as tmp_dir:
            out_dir = Path(tmp_dir) / "voices"
            manifest = run_design_voices(
                base_url="http://127.0.0.1:8088",
                api_key="secret-token",
                caption="bright lively cheerful voice",
                seeds=[1, 5, 9],
                text="テスト音声です。",
                out_dir=out_dir,
                http_fn=fake_http,
            )
            self.assertEqual(len(manifest), 3)

            # Check requests sent
            self.assertEqual(len(recorded_requests), 3)
            for idx, seed in enumerate([1, 5, 9]):
                req = recorded_requests[idx]
                self.assertEqual(req["model"], "irodori-tts")
                self.assertEqual(req["voice"], "none")
                self.assertEqual(req["input"], "テスト音声です。")
                self.assertEqual(
                    req["irodori"]["caption"], "bright lively cheerful voice"
                )
                self.assertEqual(req["irodori"]["seed"], seed)
                self.assertFalse(req["irodori"]["chunking_enabled"])

            # Check generated WAV files
            self.assertTrue((out_dir / "seed_1.wav").is_file())
            self.assertTrue((out_dir / "seed_5.wav").is_file())
            self.assertTrue((out_dir / "seed_9.wav").is_file())

            # Check manifest.json
            manifest_path = out_dir / "manifest.json"
            self.assertTrue(manifest_path.is_file())
            saved_manifest = json.loads(manifest_path.read_text(encoding="utf-8"))
            self.assertEqual(len(saved_manifest), 3)

            for entry in saved_manifest:
                self.assertIn("seed", entry)
                self.assertIn("caption", entry)
                self.assertIn("text", entry)
                self.assertIn("file", entry)
                self.assertIn("audio_seconds", entry)
                self.assertAlmostEqual(entry["audio_seconds"], 2.5, places=1)

    def test_run_design_voices_server_error(self):
        def fake_http_err(url, method="GET", headers=None, data=None, timeout=60.0):
            return 500, b"Internal server error in VoiceDesign", {}

        with tempfile.TemporaryDirectory() as tmp_dir:
            out_dir = Path(tmp_dir) / "voices"
            with self.assertRaises(RuntimeError) as ctx:
                run_design_voices(
                    base_url="http://127.0.0.1:8088",
                    api_key="secret-token",
                    caption="caption",
                    seeds=[42],
                    text="テスト",
                    out_dir=out_dir,
                    http_fn=fake_http_err,
                )
            self.assertIn("500", str(ctx.exception))

    def test_run_design_voices_incremental_manifest_on_failure(self):
        # Mid-run failure keeps what was already produced
        count = 0

        def fake_http_mid_fail(
            url, method="GET", headers=None, data=None, timeout=60.0
        ):
            nonlocal count
            count += 1
            if count == 1:
                return 200, self.dummy_wav, {}
            return 500, b"Failed on second seed", {}

        with tempfile.TemporaryDirectory() as tmp_dir:
            out_dir = Path(tmp_dir) / "voices"
            with self.assertRaises(RuntimeError):
                run_design_voices(
                    base_url="http://127.0.0.1:8088",
                    api_key="secret-token",
                    caption="caption",
                    seeds=[1, 2],
                    text="テスト",
                    out_dir=out_dir,
                    http_fn=fake_http_mid_fail,
                )

            # Seed 1 file and manifest entry must exist despite seed 2 failure
            self.assertTrue((out_dir / "seed_1.wav").is_file())
            manifest_file = out_dir / "manifest.json"
            self.assertTrue(manifest_file.is_file())
            saved = json.loads(manifest_file.read_text(encoding="utf-8"))
            self.assertEqual(len(saved), 1)
            self.assertEqual(saved[0]["seed"], 1)


if __name__ == "__main__":
    unittest.main()
