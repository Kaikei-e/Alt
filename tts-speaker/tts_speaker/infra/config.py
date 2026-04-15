"""Configuration loading for tts-speaker."""

from __future__ import annotations

from functools import lru_cache

from pydantic import Field
from pydantic_settings import BaseSettings


class Settings(BaseSettings):
    """Runtime configuration derived from environment variables."""

    host: str = Field(default="0.0.0.0", validation_alias="HOST")
    port: int = Field(default=9700, validation_alias="PORT")
    default_voice: str = Field(default="jf_alpha", validation_alias="TTS_DEFAULT_VOICE")
    default_speed: float = Field(default=1.0, ge=0.5, le=2.0, validation_alias="TTS_DEFAULT_SPEED")
    log_level: str = Field(default="INFO", validation_alias="LOG_LEVEL")
    tts_max_stream_text_length: int = Field(
        default=30_000, validation_alias="TTS_MAX_STREAM_TEXT_LENGTH"
    )


@lru_cache(maxsize=1)
def get_settings() -> Settings:
    """Return cached settings instance."""
    return Settings()
