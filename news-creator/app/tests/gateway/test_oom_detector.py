"""Unit tests for OOMDetector and 2-model fallback routing."""

from unittest.mock import Mock

import pytest

from news_creator.gateway.model_router import ModelRouter
from news_creator.gateway.oom_detector import OOMDetector


class TestOOMDetectorEnabled:
    """Tests for OOMDetector when enabled=True."""

    @pytest.mark.parametrize(
        "message",
        [
            "CUDA out of memory: tried to allocate 2.00 GiB",
            "ggml_cuda_host_malloc: failed to allocate 512 MiB (out of memory)",
            "RuntimeError: OOM while loading runner",
            "VRAM exhausted on device 0",
            "gpu memory limit exceeded",
            "HTTP 500: insufficient memory to load model",
            "HTTP 500 - memory allocation failed in ggml backend",
        ],
    )
    def test_is_oom_error_detects_known_patterns(self, message: str):
        detector = OOMDetector(enabled=True)
        assert detector.is_oom_error(message) is True

    @pytest.mark.parametrize(
        "message",
        [
            "",
            "connection refused",
            "model not found",
            "HTTP 404: endpoint not found",
        ],
    )
    def test_is_oom_error_returns_false_for_non_oom_messages(self, message: str):
        detector = OOMDetector(enabled=True)
        assert detector.is_oom_error(message) is False

    def test_detect_oom_from_response_sets_oom_and_two_model_mode(self):
        detector = OOMDetector(enabled=True)
        assert detector.oom_detected is False
        assert detector.two_model_mode is False

        result = detector.detect_oom_from_response(
            {"error": "CUDA out of memory while allocating KV cache"}
        )

        assert result is True
        assert detector.oom_detected is True
        assert detector.two_model_mode is True

    def test_detect_oom_from_response_returns_false_on_normal_response(self):
        detector = OOMDetector(enabled=True)

        assert (
            detector.detect_oom_from_response({"response": "ok", "done": True}) is False
        )
        assert detector.detect_oom_from_response({"error": "invalid json"}) is False
        assert detector.oom_detected is False
        assert detector.two_model_mode is False

    def test_detect_oom_from_exception_sets_oom_and_two_model_mode(self):
        detector = OOMDetector(enabled=True)
        exc = RuntimeError(
            "Ollama API error: HTTP 500 - ggml_cuda_host_malloc: memory allocation failed"
        )

        result = detector.detect_oom_from_exception(exc)

        assert result is True
        assert detector.oom_detected is True
        assert detector.two_model_mode is True

    def test_detect_oom_from_exception_returns_false_on_unrelated_exception(self):
        detector = OOMDetector(enabled=True)
        exc = RuntimeError("Ollama API error: HTTP 503 - service unavailable")

        assert detector.detect_oom_from_exception(exc) is False
        assert detector.oom_detected is False
        assert detector.two_model_mode is False

    def test_reset_clears_oom_detected_and_two_model_mode(self):
        detector = OOMDetector(enabled=True)
        detector.detect_oom_from_response({"error": "CUDA out of memory"})
        assert detector.oom_detected is True
        assert detector.two_model_mode is True

        detector.reset()

        assert detector.oom_detected is False
        assert detector.two_model_mode is False


class TestOOMDetectorFallbackModelSelection:
    """Tests for fallback model selection from 60k to 12k after OOM detection."""

    @staticmethod
    def _make_config(model_60k_enabled: bool = False) -> Mock:
        config = Mock()
        config.model_routing_enabled = True
        config.model_name = "gemma4-e4b-q4km"
        config.llm_num_ctx = 8192
        config.llm_num_predict = 1200
        config.model_8k_name = "gemma4-e4b-12k"
        config.model_60k_name = "gemma4-e4b-60k"
        config.token_safety_margin_percent = 10
        config.token_safety_margin_fixed = 512
        config.oom_detection_enabled = True
        config.model_60k_enabled = model_60k_enabled
        return config

    def test_fallback_from_60k_to_12k_when_oom_detected(self):
        """After OOM on gemma4-e4b-60k, router in two_model_mode selects gemma4-e4b-12k for fallback."""
        detector = OOMDetector(enabled=True)
        config = self._make_config(model_60k_enabled=False)
        router = ModelRouter(config, oom_detector=detector)

        original_model = "gemma4-e4b-60k"
        assert detector.detect_oom_from_exception(RuntimeError("CUDA out of memory"))
        assert detector.two_model_mode is True

        selected_model, bucket = router.select_model("short prompt")
        assert selected_model != original_model
        assert selected_model == "gemma4-e4b-12k"
        assert bucket == 8192

    def test_already_on_12k_yields_no_different_fallback_model(self):
        """If already running on gemma4-e4b-12k, re-selecting returns the same model (no fallback)."""
        detector = OOMDetector(enabled=True)
        config = self._make_config(model_60k_enabled=False)
        router = ModelRouter(config, oom_detector=detector)

        original_model = "gemma4-e4b-12k"
        detector.detect_oom_from_response({"error": "out of memory"})

        selected_model, _ = router.select_model("short prompt")
        fallback_model = selected_model if selected_model != original_model else None
        assert fallback_model is None


class TestOOMDetectorDisabled:
    """Tests for OOMDetector when enabled=False."""

    def test_disabled_detector_always_returns_false(self):
        detector = OOMDetector(enabled=False)

        assert detector.is_oom_error("CUDA out of memory") is False
        assert (
            detector.detect_oom_from_response({"error": "CUDA out of memory"}) is False
        )
        assert (
            detector.detect_oom_from_exception(RuntimeError("CUDA out of memory"))
            is False
        )
        assert detector.oom_detected is False
        assert detector.two_model_mode is False
