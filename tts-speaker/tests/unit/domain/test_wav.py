import io
import wave
from collections.abc import Callable

import pytest

from tts_speaker.domain.errors import AudioFormatError
from tts_speaker.domain.wav import concat_wav, wav_duration_seconds


def test_concat_wav_empty_list() -> None:
    with pytest.raises(AudioFormatError):
        concat_wav([], gap_ms=200)


def test_concat_wav_single_part(sample_wav_bytes: bytes) -> None:
    result = concat_wav([sample_wav_bytes], gap_ms=200)
    assert isinstance(result, bytes)
    assert len(result) > 0
    duration = wav_duration_seconds(result)
    assert pytest.approx(duration, abs=0.01) == 0.1


def test_concat_wav_multiple_parts_with_gap(sample_wav_bytes: bytes, sample_wav_bytes_2: bytes) -> None:
    # Part 1: 0.1s, Part 2: 0.2s, Gap: 200ms (0.2s)
    # Expected total duration: 0.1 + 0.2 + 0.2 = 0.5s
    result = concat_wav([sample_wav_bytes, sample_wav_bytes_2], gap_ms=200)
    duration = wav_duration_seconds(result)
    assert pytest.approx(duration, abs=0.01) == 0.5


def test_concat_wav_three_parts_with_gap(sample_wav_bytes: bytes) -> None:
    # 3 parts of 0.1s each, gap: 100ms (0.1s)
    # Expected total duration: 0.1 + 0.1 + 0.1 + (2 * 0.1) = 0.5s
    result = concat_wav([sample_wav_bytes, sample_wav_bytes, sample_wav_bytes], gap_ms=100)
    duration = wav_duration_seconds(result)
    assert pytest.approx(duration, abs=0.01) == 0.5


def test_concat_wav_mismatched_framerate(sample_wav_bytes: bytes, make_wav: Callable[..., bytes]) -> None:
    different_rate = make_wav(duration_seconds=0.1, sample_rate=24000, num_channels=1, sample_width=2)
    with pytest.raises(AudioFormatError):
        concat_wav([sample_wav_bytes, different_rate], gap_ms=100)


def test_concat_wav_mismatched_channels(sample_wav_bytes: bytes, make_wav: Callable[..., bytes]) -> None:
    stereo_wav = make_wav(duration_seconds=0.1, sample_rate=48000, num_channels=2, sample_width=2)
    with pytest.raises(AudioFormatError):
        concat_wav([sample_wav_bytes, stereo_wav], gap_ms=100)


def test_concat_wav_mismatched_sample_width(sample_wav_bytes: bytes, make_wav: Callable[..., bytes]) -> None:
    wav_8bit = make_wav(duration_seconds=0.1, sample_rate=48000, num_channels=1, sample_width=1)
    with pytest.raises(AudioFormatError):
        concat_wav([sample_wav_bytes, wav_8bit], gap_ms=100)


def test_concat_wav_corrupt_bytes() -> None:
    with pytest.raises(AudioFormatError):
        concat_wav([b"not-a-wav-file"], gap_ms=100)


def test_wav_duration_seconds_valid(sample_wav_bytes: bytes, sample_wav_bytes_2: bytes) -> None:
    assert pytest.approx(wav_duration_seconds(sample_wav_bytes), abs=0.01) == 0.1
    assert pytest.approx(wav_duration_seconds(sample_wav_bytes_2), abs=0.01) == 0.2


def test_wav_duration_seconds_corrupt() -> None:
    with pytest.raises(AudioFormatError):
        wav_duration_seconds(b"corrupt-data")


def test_concat_wav_rejects_non_2_sampwidth(make_wav: Callable[..., bytes]) -> None:
    wav_8bit = make_wav(duration_seconds=0.1, sample_rate=48000, num_channels=1, sample_width=1)
    with pytest.raises(AudioFormatError, match="sampwidth == 2"):
        concat_wav([wav_8bit], gap_ms=100)


def test_concat_wav_exact_bytes_with_distinct_samples() -> None:
    frames_a = b"\x01\x02" * 5
    frames_b = b"\x03\x04" * 7

    def _build_wav(frames: bytes, framerate: int = 1000) -> bytes:
        buf = io.BytesIO()
        with wave.open(buf, "wb") as wf:
            wf.setnchannels(1)
            wf.setsampwidth(2)
            wf.setframerate(framerate)
            wf.writeframes(frames)
        return buf.getvalue()

    part_a = _build_wav(frames_a, framerate=1000)
    part_b = _build_wav(frames_b, framerate=1000)
    gap_ms = 10
    concatenated = concat_wav([part_a, part_b], gap_ms=gap_ms)

    with wave.open(io.BytesIO(concatenated), "rb") as wf:
        out_frames = wf.readframes(wf.getnframes())

    gap_zeros = b"\x00\x00" * 10
    assert out_frames == frames_a + gap_zeros + frames_b
