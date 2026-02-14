"""Configuration loading for tts-speaker."""

from __future__ import annotations

import os
from functools import lru_cache

from pydantic import Field, model_validator
from pydantic_settings import BaseSettings


class Settings(BaseSettings):
    """Runtime configuration derived from environment variables."""

    host: str = Field(default="0.0.0.0", validation_alias="HOST")
    port: int = Field(default=9700, validation_alias="PORT")
    default_voice: str = Field(default="jf_alpha", validation_alias="TTS_DEFAULT_VOICE")
    default_speed: float = Field(default=1.0, ge=0.5, le=2.0, validation_alias="TTS_DEFAULT_SPEED")
    log_level: str = Field(default="INFO", validation_alias="LOG_LEVEL")
    service_secret: str = Field(default="", validation_alias="SERVICE_SECRET")
    tts_max_stream_text_length: int = Field(
        default=30_000, validation_alias="TTS_MAX_STREAM_TEXT_LENGTH"
    )

    @model_validator(mode="after")
    def load_secret_file(self) -> "Settings":
        """Read SERVICE_SECRET_FILE if present (Docker secrets pattern)."""
        secret_file = os.getenv("SERVICE_SECRET_FILE")
        if secret_file:
            try:
                with open(secret_file) as f:
                    self.service_secret = f.read().strip()
            except OSError:
                pass
        return self


@lru_cache(maxsize=1)
def get_settings() -> Settings:
    """Return cached settings instance."""
    return Settings()
