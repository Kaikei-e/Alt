"""Tests for GPU detection and fallback in TTSPipeline."""

from __future__ import annotations

from unittest.mock import MagicMock, patch

from tts_speaker.core.pipeline import TTSPipeline


def test_detect_device_cpu_fallback():
    """Falls back to CPU when no GPU is available."""
    mock_torch = MagicMock()
    mock_torch.cuda.is_available.return_value = False

    with patch.dict("sys.modules", {"torch": mock_torch}):
        device = TTSPipeline._detect_device()

    assert device == "cpu"


def test_detect_device_gpu_available():
    """Uses CUDA/ROCm when GPU is available."""
    mock_torch = MagicMock()
    mock_torch.cuda.is_available.return_value = True
    mock_torch.cuda.get_device_name.return_value = "AMD Radeon 890M"

    with patch.dict("sys.modules", {"torch": mock_torch}):
        device = TTSPipeline._detect_device()

    assert device == "cuda"
