"""Configuration module for News Creator service."""

import os
import logging
from typing import List

logger = logging.getLogger(__name__)


class NewsCreatorConfig:
    """Configuration for News Creator service from environment variables."""

    def __init__(self):
        """Initialize configuration from environment variables."""
        # Authentication settings
        self.auth_service_url = os.getenv(
            "AUTH_SERVICE_URL",
            "http://auth-service.alt-auth.svc.cluster.local:8080",
        )
        self.service_name = "news-creator"
        self.service_secret = os.getenv("SERVICE_SECRET", "")
        self.token_ttl = 3600

        if not self.service_secret:
            raise ValueError("SERVICE_SECRET environment variable is required")

        # LLM service settings
        self.llm_service_url = os.getenv("LLM_SERVICE_URL", "http://localhost:11435")
        self.model_name = os.getenv("LLM_MODEL", "gemma3:4b")
        self.llm_timeout_seconds = self._get_int("LLM_TIMEOUT_SECONDS", 180)  # 3分に増加
        self.llm_keep_alive = self._get_int("LLM_KEEP_ALIVE_SECONDS", -1)

        # ---- Generation parameters (Gemma3 + Ollama options) ----
        # 実効 16k 前提。必要なら環境変数で上書き
        self.llm_num_ctx = self._get_int("LLM_NUM_CTX", 16384)
        self.llm_num_predict = self._get_int("LLM_NUM_PREDICT", 1200)  # 復活
        self.llm_temperature = self._get_float("LLM_TEMPERATURE", 0.2)
        self.llm_top_p = self._get_float("LLM_TOP_P", 0.9)             # 復活
        self.llm_top_k = self._get_int("LLM_TOP_K", 50)
        self.llm_repeat_penalty = self._get_float("LLM_REPEAT_PENALTY", 1.07)
        self.llm_num_keep = self._get_int("LLM_NUM_KEEP", -1)          # system保持

        # Stop tokens（Gemma3 は <start_of_turn>/<end_of_turn>）
        # 既定は Gemma3 正式トークンのみ。空なら安全に補充。
        stop_tokens_str = os.getenv("LLM_STOP_TOKENS", "<end_of_turn>")
        self.llm_stop_tokens = [
            token.strip() for token in stop_tokens_str.split(",") if token.strip()
        ]
        if not self.llm_stop_tokens:
            self.llm_stop_tokens = ["<end_of_turn>"]

        # Summary-specific settings
        self.summary_num_predict = self._get_int("SUMMARY_NUM_PREDICT", 500)

        logger.info(
            "News creator configuration initialized",
            extra={
                "auth_service_url": self.auth_service_url,
                "service_name": self.service_name,
                "llm_service_url": self.llm_service_url,
                "model": self.model_name,
            },
        )

    def _get_int(self, name: str, default: int) -> int:
        """Get integer value from environment variable with fallback."""
        try:
            return int(os.getenv(name, default))
        except ValueError:
            logger.warning("Invalid int for %s. Using default %s", name, default)
            return default

    def _get_float(self, name: str, default: float) -> float:
        """Get float value from environment variable with fallback."""
        try:
            return float(os.getenv(name, default))
        except ValueError:
            logger.warning("Invalid float for %s. Using default %s", name, default)
            return default

    def get_llm_options(self) -> dict:
        """Get LLM options as a dictionary (for Ollama 'options')."""
        return {
            "num_ctx": self.llm_num_ctx,
            "num_predict": self.llm_num_predict,
            "temperature": self.llm_temperature,
            "top_p": self.llm_top_p,
            "top_k": self.llm_top_k,
            "repeat_penalty": self.llm_repeat_penalty,
            "num_keep": self.llm_num_keep,
            "stop": list(self.llm_stop_tokens),
        }
