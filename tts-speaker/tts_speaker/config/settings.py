"""Configuration settings for tts-speaker service."""

from pathlib import Path
from typing import Any

from pydantic import HttpUrl, SecretStr
from pydantic_settings import BaseSettings, SettingsConfigDict


class Settings(BaseSettings):
    """Application settings loaded from environment variables."""

    model_config = SettingsConfigDict(env_prefix="")

    irodori_base_url: HttpUrl
    irodori_api_key_file: Path
    irodori_model_name: str = "irodori-tts"
    irodori_request_timeout_seconds: float = 330.0
    irodori_max_attempts: int = 3
    irodori_retry_backoff_seconds: float = 1.0
    tts_voice_id: str
    tts_max_chunk_chars: int = 100
    tts_max_text_chars: int = 5000
    tts_chunk_gap_ms: int = 200
    host: str = "0.0.0.0"
    port: int = 9700
    log_level: str = "INFO"
    irodori_api_key: SecretStr = SecretStr("")

    def __init__(self, **values: Any) -> None:
        raise NotImplementedError
