"""Configuration loading for tts-speaker."""

from __future__ import annotations

from functools import lru_cache
from typing import Literal

from pydantic import Field
from pydantic_settings import BaseSettings


class Settings(BaseSettings):
    """Runtime configuration derived from environment variables."""

    host: str = Field(default="0.0.0.0", validation_alias="HOST")
    port: int = Field(default=9700, validation_alias="PORT")
    engine: Literal["qwen", "supertonic"] = Field(default="qwen", validation_alias="TTS_ENGINE")
    default_voice: str = Field(default="qwen-ja-1", validation_alias="TTS_DEFAULT_VOICE")
    default_speed: float = Field(default=1.0, ge=0.5, le=2.0, validation_alias="TTS_DEFAULT_SPEED")
    log_level: str = Field(default="INFO", validation_alias="LOG_LEVEL")
    tts_max_stream_text_length: int = Field(
        default=30_000, validation_alias="TTS_MAX_STREAM_TEXT_LENGTH"
    )
    qwen_model_id: str = Field(
        default="Qwen/Qwen3-TTS-12Hz-0.6B-CustomVoice",
        validation_alias="TTS_QWEN_MODEL_ID",
    )
    qwen_dtype: str = Field(default="bfloat16", validation_alias="TTS_QWEN_DTYPE")
    qwen_attn_implementation: str = Field(default="sdpa", validation_alias="TTS_QWEN_ATTN")
    qwen_keepalive_interval_sec: float = Field(
        default=15.0, ge=0.0, validation_alias="TTS_QWEN_KEEPALIVE_INTERVAL_SEC"
    )
    sup_total_steps: int = Field(default=8, ge=1, le=12, validation_alias="TTS_SUP_TOTAL_STEPS")


@lru_cache(maxsize=1)
def get_settings() -> Settings:
    """Return cached settings instance."""
    return Settings()
