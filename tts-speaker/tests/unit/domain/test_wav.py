import io
import wave
from collections.abc import Callable

import pytest

from tts_speaker.domain.errors import AudioFormatError
from tts_speaker.domain.wav import append_silence, wav_info


def test_wav_info_valid(sample_wav_bytes: bytes, sample_wav_bytes_2: bytes) -> None:
    sr, duration = wav_info(sample_wav_bytes)
    assert sr == 48000
    assert pytest.approx(duration, abs=0.01) == 0.1

    sr2, duration2 = wav_info(sample_wav_bytes_2)
    assert sr2 == 48000
    assert pytest.approx(duration2, abs=0.01) == 0.2


def test_wav_info_corrupt() -> None:
    with pytest.raises(AudioFormatError):
        wav_info(b"corrupt-data")


def test_wav_info_rejects_stereo(make_wav: Callable[..., bytes]) -> None:
    stereo_wav = make_wav(duration_seconds=0.1, sample_rate=48000, num_channels=2, sample_width=2)
    with pytest.raises(AudioFormatError, match="mono"):
        wav_info(stereo_wav)


def test_wav_info_rejects_non_16bit(make_wav: Callable[..., bytes]) -> None:
    wav_8bit = make_wav(duration_seconds=0.1, sample_rate=48000, num_channels=1, sample_width=1)
    with pytest.raises(AudioFormatError, match="16-bit"):
        wav_info(wav_8bit)


def test_wav_info_rejects_invalid_framerate() -> None:
    raw = (
        b"RIFF"
        + (36).to_bytes(4, "little")
        + b"WAVE"
        + b"fmt "
        + (16).to_bytes(4, "little")
        + (1).to_bytes(2, "little")
        + (1).to_bytes(2, "little")
        + (0).to_bytes(4, "little")
        + (0).to_bytes(4, "little")
        + (2).to_bytes(2, "little")
        + (16).to_bytes(2, "little")
        + b"data"
        + (0).to_bytes(4, "little")
    )
    with pytest.raises(AudioFormatError, match="framerate"):
        wav_info(raw)


def test_append_silence_success(sample_wav_bytes: bytes) -> None:
    _, original_duration = wav_info(sample_wav_bytes)
    result = append_silence(sample_wav_bytes, ms=200)
    assert isinstance(result, bytes)
    _, new_duration = wav_info(result)
    assert pytest.approx(new_duration, abs=0.01) == original_duration + 0.2

    with wave.open(io.BytesIO(sample_wav_bytes), "rb") as wf_orig, wave.open(io.BytesIO(result), "rb") as wf_new:
        assert wf_orig.getnchannels() == wf_new.getnchannels()
        assert wf_orig.getsampwidth() == wf_new.getsampwidth()
        assert wf_orig.getframerate() == wf_new.getframerate()
        orig_frames = wf_orig.readframes(wf_orig.getnframes())
        new_frames = wf_new.readframes(wf_new.getnframes())

    expected_silence_frames = int(wf_orig.getframerate() * 0.2)
    expected_silence_bytes = b"\x00" * (expected_silence_frames * wf_orig.getnchannels() * wf_orig.getsampwidth())
    assert new_frames == orig_frames + expected_silence_bytes


def test_append_silence_zero_or_negative_ms(sample_wav_bytes: bytes) -> None:
    result_zero = append_silence(sample_wav_bytes, ms=0)
    assert wav_info(result_zero)[1] == pytest.approx(wav_info(sample_wav_bytes)[1], abs=0.001)

    result_neg = append_silence(sample_wav_bytes, ms=-50)
    assert wav_info(result_neg)[1] == pytest.approx(wav_info(sample_wav_bytes)[1], abs=0.001)


def test_append_silence_corrupt_bytes() -> None:
    with pytest.raises(AudioFormatError):
        append_silence(b"not-a-wav-file", ms=200)


def test_append_silence_rejects_non_2_sampwidth(make_wav: Callable[..., bytes]) -> None:
    wav_8bit = make_wav(duration_seconds=0.1, sample_rate=48000, num_channels=1, sample_width=1)
    with pytest.raises(AudioFormatError, match="sampwidth == 2"):
        append_silence(wav_8bit, ms=200)
