"""Tests for GPU detection and fallback in TTSPipeline."""

from __future__ import annotations

from unittest.mock import MagicMock, patch

import pytest

from tts_speaker.core.pipeline import TTSPipeline


def _make_mock_torch(*, cuda_available: bool, device_name: str = "AMD Radeon 890M"):
    """Create a mock torch module with configurable GPU state."""
    mock_torch = MagicMock()
    mock_torch.cuda.is_available.return_value = cuda_available
    mock_torch.cuda.get_device_name.return_value = device_name

    # Mock tensor operations for GPU compute verification
    mock_tensor = MagicMock()
    mock_result = MagicMock()
    mock_result.item.return_value = 5.0
    mock_tensor.__mul__ = lambda self, other: mock_tensor
    mock_tensor.sum.return_value = mock_result
    mock_torch.tensor.return_value = mock_tensor

    return mock_torch


def test_detect_device_gpu_available():
    """Uses CUDA/ROCm when GPU is available."""
    mock_torch = _make_mock_torch(cuda_available=True)

    with patch.dict("sys.modules", {"torch": mock_torch}):
        device, gpu_name = TTSPipeline._detect_device()

    assert device == "cuda"
    assert gpu_name == "AMD Radeon 890M"


def test_detect_device_no_gpu_raises_by_default():
    """Raises RuntimeError when no GPU is detected (default mode)."""
    mock_torch = _make_mock_torch(cuda_available=False)

    with (
        patch.dict("sys.modules", {"torch": mock_torch}),
        patch.dict("os.environ", {"TTS_ALLOW_CPU_FALLBACK": "0"}, clear=False),
        pytest.raises(RuntimeError, match="No GPU detected"),
    ):
        TTSPipeline._detect_device()


def test_detect_device_cpu_fallback_when_allowed():
    """Falls back to CPU when TTS_ALLOW_CPU_FALLBACK=1."""
    mock_torch = _make_mock_torch(cuda_available=False)

    with (
        patch.dict("sys.modules", {"torch": mock_torch}),
        patch.dict("os.environ", {"TTS_ALLOW_CPU_FALLBACK": "1"}, clear=False),
    ):
        device, gpu_name = TTSPipeline._detect_device()

    assert device == "cpu"
    assert gpu_name is None


def test_detect_device_gpu_compute_failure_raises():
    """Raises RuntimeError when GPU compute verification fails."""
    mock_torch = MagicMock()
    mock_torch.cuda.is_available.return_value = True
    mock_torch.cuda.get_device_name.return_value = "AMD Radeon 890M"
    mock_torch.tensor.side_effect = RuntimeError("HIP error")

    with (
        patch.dict("sys.modules", {"torch": mock_torch}),
        patch.dict("os.environ", {"TTS_ALLOW_CPU_FALLBACK": "0"}, clear=False),
        pytest.raises(RuntimeError, match="GPU detected but compute verification failed"),
    ):
        TTSPipeline._detect_device()


def test_detect_device_gpu_compute_failure_fallback():
    """Falls back to CPU on compute failure when TTS_ALLOW_CPU_FALLBACK=1."""
    mock_torch = MagicMock()
    mock_torch.cuda.is_available.return_value = True
    mock_torch.cuda.get_device_name.return_value = "AMD Radeon 890M"
    mock_torch.tensor.side_effect = RuntimeError("HIP error")

    with (
        patch.dict("sys.modules", {"torch": mock_torch}),
        patch.dict("os.environ", {"TTS_ALLOW_CPU_FALLBACK": "1"}, clear=False),
    ):
        device, gpu_name = TTSPipeline._detect_device()

    assert device == "cpu"
    assert gpu_name is None


def test_detect_device_logs_env_vars():
    """Logs HSA_OVERRIDE_GFX_VERSION and HIP_VISIBLE_DEVICES."""
    mock_torch = _make_mock_torch(cuda_available=True)

    with (
        patch.dict("sys.modules", {"torch": mock_torch}),
        patch.dict(
            "os.environ",
            {"HSA_OVERRIDE_GFX_VERSION": "11.0.0", "HIP_VISIBLE_DEVICES": "0"},
            clear=False,
        ),
        patch("tts_speaker.core.pipeline.logger") as mock_logger,
    ):
        TTSPipeline._detect_device()

    mock_logger.info.assert_any_call(
        "HSA_OVERRIDE_GFX_VERSION=%s, HIP_VISIBLE_DEVICES=%s", "11.0.0", "0"
    )
