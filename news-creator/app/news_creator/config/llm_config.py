"""LLM Configuration dataclass (Phase 1 refactoring).

Following Python 3.14 best practices:
- Frozen dataclass for immutable configuration
- Factory method for environment loading
"""

from __future__ import annotations

import os
import logging
from dataclasses import dataclass
from typing import Union

logger = logging.getLogger(__name__)


@dataclass(frozen=True)
class LLMConfig:
    """Immutable configuration for LLM service settings."""

    service_url: str
    model_name: str
    timeout_seconds: int = 300
    keep_alive: str = "24h"
    keep_alive_8k: str = "24h"
    keep_alive_60k: str = "15m"
    num_ctx: int = 8192
    num_batch: int = 1024
    num_predict: int = 1200
    temperature: float = 0.7
    top_p: float = 0.85
    top_k: int = 40
    repeat_penalty: float = 1.15
    num_keep: int = -1
    stop_tokens: tuple[str, ...] = ("<turn|>",)
    # Summary-specific settings
    summary_num_predict: int = 1000
    summary_temperature: float = 0.5
    # Repetition detection settings
    max_repetition_retries: int = 2
    repetition_threshold: float = 0.3
    # Recap settings
    recap_min_source_articles_for_llm: int = 1
    recap_min_representative_sentences_for_llm: int = 2
    recap_summary_temperature: float = 0.0
    recap_ja_ratio_threshold: float = 0.6
    recap_summary_repair_attempts: int = 1

    def get_options(self) -> dict:
        """Get LLM options as a dictionary for Ollama 'options' field."""
        return {
            "num_ctx": self.num_ctx,
            "num_predict": self.num_predict,
            "num_batch": self.num_batch,
            "temperature": self.temperature,
            "top_p": self.top_p,
            "top_k": self.top_k,
            "repeat_penalty": self.repeat_penalty,
            "num_keep": self.num_keep,
            "stop": list(self.stop_tokens),
        }

    def get_keep_alive_for_model(
        self, model_name: str, model_8k_name: str, model_60k_name: str
    ) -> Union[int, str]:
        """Get keep_alive value for a specific model."""
        if model_name == model_8k_name:
            return self.keep_alive_8k
        elif model_name == model_60k_name:
            return self.keep_alive_60k
        else:
            return self.keep_alive

    @classmethod
    def from_env(cls) -> LLMConfig:
        """Create LLMConfig from environment variables."""
        stop_tokens_str = os.getenv("LLM_STOP_TOKENS", "<turn|>")
        stop_tokens = tuple(
            token.strip() for token in stop_tokens_str.split(",") if token.strip()
        )
        if not stop_tokens:
            stop_tokens = ("<turn|>",)

        return cls(
            service_url=os.getenv("LLM_SERVICE_URL", "http://localhost:11435"),
            model_name=os.getenv("LLM_MODEL", "gemma4-e4b-q4km"),
            timeout_seconds=_get_int("LLM_TIMEOUT_SECONDS", 300),
            keep_alive=os.getenv("LLM_KEEP_ALIVE_SECONDS", "24h"),
            keep_alive_8k=os.getenv("LLM_KEEP_ALIVE_8K", "24h"),
            keep_alive_60k=os.getenv("LLM_KEEP_ALIVE_60K", "15m"),
            num_ctx=_get_int("LLM_NUM_CTX", 8192),
            num_batch=_get_int("LLM_NUM_BATCH", 1024),
            num_predict=_get_int("LLM_NUM_PREDICT", 1200),
            temperature=_get_float("LLM_TEMPERATURE", 0.7),
            top_p=_get_float("LLM_TOP_P", 0.85),
            top_k=_get_int("LLM_TOP_K", 40),
            repeat_penalty=_get_float("LLM_REPEAT_PENALTY", 1.15),
            num_keep=_get_int("LLM_NUM_KEEP", -1),
            stop_tokens=stop_tokens,
            summary_num_predict=_get_int("SUMMARY_NUM_PREDICT", 1000),
            summary_temperature=_get_float("SUMMARY_TEMPERATURE", 0.5),
            max_repetition_retries=_get_int("MAX_REPETITION_RETRIES", 2),
            repetition_threshold=_get_float("REPETITION_THRESHOLD", 0.3),
            recap_min_source_articles_for_llm=_get_int(
                "RECAP_MIN_SOURCE_ARTICLES_FOR_LLM", 1
            ),
            recap_min_representative_sentences_for_llm=_get_int(
                "RECAP_MIN_REPRESENTATIVE_SENTENCES_FOR_LLM", 2
            ),
            recap_summary_temperature=_get_float("RECAP_SUMMARY_TEMPERATURE", 0.0),
            recap_ja_ratio_threshold=_get_float("RECAP_JA_RATIO_THRESHOLD", 0.6),
            recap_summary_repair_attempts=_get_int("RECAP_SUMMARY_REPAIR_ATTEMPTS", 1),
        )


def _get_int(name: str, default: int) -> int:
    """Get integer value from environment variable with fallback."""
    try:
        return int(os.getenv(name, default))
    except ValueError:
        logger.warning("Invalid int for %s. Using default %s", name, default)
        return default


def _get_float(name: str, default: float) -> float:
    """Get float value from environment variable with fallback."""
    try:
        return float(os.getenv(name, default))
    except ValueError:
        logger.warning("Invalid float for %s. Using default %s", name, default)
        return default
