"""Tests for percentile calculations, RTF calculation, and WAV duration."""

from __future__ import annotations

import io
import sys
import unittest
import wave
from pathlib import Path

# Ensure bench module is importable
BENCH_DIR = Path(__file__).resolve().parent.parent
if str(BENCH_DIR) not in sys.path:
    sys.path.insert(0, str(BENCH_DIR))

from bench_vram_rtf import calculate_percentile, calculate_rtf, get_wav_duration_seconds


def make_dummy_wav(duration_s: float, sample_rate: int = 48000) -> bytes:
    """Generate minimal valid PCM WAV bytes in memory."""
    buf = io.BytesIO()
    total_frames = int(duration_s * sample_rate)
    with wave.open(buf, "wb") as wf:
        wf.setnchannels(1)
        wf.setsampwidth(2)
        wf.setframerate(sample_rate)
        wf.writeframes(b"\x00\x00" * total_frames)
    return buf.getvalue()


class TestMetrics(unittest.TestCase):
    def test_percentile_empty(self):
        self.assertEqual(calculate_percentile([], 50), 0.0)

    def test_percentile_single_value(self):
        self.assertEqual(calculate_percentile([42.5], 50), 42.5)
        self.assertEqual(calculate_percentile([42.5], 95), 42.5)
        self.assertEqual(calculate_percentile([42.5], 0), 42.5)
        self.assertEqual(calculate_percentile([42.5], 100), 42.5)

    def test_percentile_odd_length(self):
        # 5 items: index = (p/100) * 4
        data = [10.0, 20.0, 30.0, 40.0, 50.0]
        self.assertEqual(calculate_percentile(data, 0), 10.0)
        self.assertEqual(calculate_percentile(data, 50), 30.0)
        self.assertEqual(calculate_percentile(data, 100), 50.0)
        # p=95 -> index = 0.95 * 4 = 3.8 -> 40.0 + 0.8 * (50.0 - 40.0) = 48.0
        self.assertAlmostEqual(calculate_percentile(data, 95), 48.0)

    def test_percentile_even_length(self):
        data = [1.0, 2.0, 3.0, 4.0]
        # p=50 -> index = 0.5 * 3 = 1.5 -> 2.0 + 0.5 * 1.0 = 2.5
        self.assertAlmostEqual(calculate_percentile(data, 50), 2.5)

    def test_percentile_unsorted_input(self):
        data = [50.0, 10.0, 40.0, 20.0, 30.0]
        self.assertEqual(calculate_percentile(data, 50), 30.0)
        self.assertAlmostEqual(calculate_percentile(data, 95), 48.0)

    def test_rtf_normal(self):
        # 0.5s wall time for 2.0s audio -> RTF = 0.25
        self.assertAlmostEqual(calculate_rtf(0.5, 2.0), 0.25)
        # 2.0s wall time for 1.0s audio -> RTF = 2.0
        self.assertAlmostEqual(calculate_rtf(2.0, 1.0), 2.0)

    def test_rtf_zero_or_negative_audio(self):
        self.assertEqual(calculate_rtf(1.0, 0.0), 0.0)
        self.assertEqual(calculate_rtf(1.0, -0.5), 0.0)

    def test_wav_duration_normal(self):
        wav_1s = make_dummy_wav(1.0, sample_rate=48000)
        self.assertAlmostEqual(get_wav_duration_seconds(wav_1s), 1.0, places=3)

        wav_half = make_dummy_wav(0.5, sample_rate=48000)
        self.assertAlmostEqual(get_wav_duration_seconds(wav_half), 0.5, places=3)

    def test_wav_duration_corrupt_data(self):
        self.assertEqual(get_wav_duration_seconds(b""), 0.0)
        self.assertEqual(get_wav_duration_seconds(b"not a valid wav header"), 0.0)


if __name__ == "__main__":
    unittest.main()
