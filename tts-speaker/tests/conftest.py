"""Shared pytest fixtures for tts-speaker tests."""

import io
import wave
from pathlib import Path
from unittest.mock import AsyncMock

import pytest

from tts_speaker.port.speech_synthesizer_port import SpeechSynthesizerPort


def create_synthetic_wav(
    duration_seconds: float = 0.1,
    sample_rate: int = 48000,
    num_channels: int = 1,
    sample_width: int = 2,
) -> bytes:
    """Create synthetic WAV audio bytes."""
    buf = io.BytesIO()
    with wave.open(buf, "wb") as wf:
        wf.setnchannels(num_channels)
        wf.setsampwidth(sample_width)
        wf.setframerate(sample_rate)
        num_frames = int(sample_rate * duration_seconds)
        wf.writeframes(b"\x00" * (num_frames * num_channels * sample_width))
    return buf.getvalue()


@pytest.fixture
def make_wav():
    """Factory fixture for creating synthetic WAV audio bytes."""
    return create_synthetic_wav


@pytest.fixture
def sample_wav_bytes() -> bytes:
    """Fixture providing 100ms 48000Hz mono 16-bit WAV bytes."""
    return create_synthetic_wav(duration_seconds=0.1, sample_rate=48000, num_channels=1, sample_width=2)


@pytest.fixture
def sample_wav_bytes_2() -> bytes:
    """Fixture providing 200ms 48000Hz mono 16-bit WAV bytes."""
    return create_synthetic_wav(duration_seconds=0.2, sample_rate=48000, num_channels=1, sample_width=2)


@pytest.fixture
def dummy_api_key_file(tmp_path: Path) -> Path:
    """Fixture creating a temporary file with a valid API key."""
    key_file = tmp_path / "irodori_api_key.txt"
    key_file.write_text("test-secret-key-12345\n")
    return key_file


@pytest.fixture
def empty_api_key_file(tmp_path: Path) -> Path:
    """Fixture creating an empty temporary file."""
    key_file = tmp_path / "empty_key.txt"
    key_file.write_text("   \n")
    return key_file


@pytest.fixture
def fake_sleep_recorder():
    """Fixture providing a mock sleep function that records sleep intervals."""
    delays: list[float] = []

    async def _fake_sleep(delay: float) -> None:
        delays.append(delay)

    return _fake_sleep, delays


@pytest.fixture
def mock_synthesizer(sample_wav_bytes: bytes) -> AsyncMock:
    """Fixture providing a mock SpeechSynthesizerPort."""
    mock = AsyncMock(spec=SpeechSynthesizerPort)
    mock.synthesize_chunk.return_value = sample_wav_bytes
    return mock
