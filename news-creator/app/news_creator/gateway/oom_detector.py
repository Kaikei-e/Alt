"""OOM (Out of Memory) detection and fallback to 2-model mode."""

import logging
from typing import Optional

logger = logging.getLogger(__name__)


class OOMDetector:
    """Detects OOM errors from Ollama API responses and manages fallback mode."""

    # OOM error patterns to detect
    OOM_PATTERNS = [
        "out of memory",
        "oom",
        "vram",
        "cuda out of memory",
        "gpu memory",
        "insufficient memory",
        "memory allocation failed",
    ]

    def __init__(self, enabled: bool = True):
        """
        Initialize OOM detector.

        Args:
            enabled: Whether OOM detection is enabled
        """
        self.enabled = enabled
        self._oom_detected = False
        self._two_model_mode = False

    def is_oom_error(self, error_message: str) -> bool:
        """
        Check if error message indicates OOM.

        Args:
            error_message: Error message from Ollama API

        Returns:
            True if OOM error detected
        """
        if not self.enabled or not error_message:
            return False

        error_lower = error_message.lower()
        for pattern in self.OOM_PATTERNS:
            if pattern in error_lower:
                logger.warning(
                    f"OOM pattern detected: '{pattern}' in error message",
                    extra={"error_message": error_message[:200]},
                )
                return True
        return False

    def detect_oom_from_response(self, response_data: dict) -> bool:
        """
        Detect OOM from Ollama API response.

        Args:
            response_data: Response dictionary from Ollama API

        Returns:
            True if OOM detected
        """
        if not self.enabled:
            return False

        # Check error field
        error_msg = response_data.get("error", "")
        if error_msg and self.is_oom_error(error_msg):
            self._oom_detected = True
            self._two_model_mode = True
            logger.error(
                "OOM detected from Ollama response. Switching to 2-model mode (16K, 80K).",
                extra={"error": error_msg},
            )
            return True

        return False

    def detect_oom_from_exception(self, exception: Exception) -> bool:
        """
        Detect OOM from exception message.

        Args:
            exception: Exception raised by Ollama API call

        Returns:
            True if OOM detected
        """
        if not self.enabled:
            return False

        error_msg = str(exception)
        if self.is_oom_error(error_msg):
            self._oom_detected = True
            self._two_model_mode = True
            logger.error(
                "OOM detected from exception. Switching to 2-model mode (16K, 80K).",
                extra={"exception": error_msg},
            )
            return True

        return False

    @property
    def oom_detected(self) -> bool:
        """Check if OOM has been detected."""
        return self._oom_detected

    @property
    def two_model_mode(self) -> bool:
        """Check if 2-model mode is active (16K, 80K only)."""
        return self._two_model_mode

    def reset(self):
        """Reset OOM detection state (for testing or manual recovery)."""
        self._oom_detected = False
        self._two_model_mode = False
        logger.info("OOM detector state reset")

