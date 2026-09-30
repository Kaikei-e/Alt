"""Configuration settings for tts-speaker service."""

from pathlib import Path
from typing import Literal, Self

from pydantic import Field, HttpUrl, PrivateAttr, SecretStr, field_validator, model_validator
from pydantic_settings import BaseSettings, SettingsConfigDict


class Settings(BaseSettings):
    """Application settings loaded from environment variables."""

    model_config = SettingsConfigDict(env_prefix="")

    irodori_base_url: HttpUrl
    irodori_api_key_file: Path
    irodori_model_name: str = "irodori-tts"
    irodori_request_timeout_seconds: float = Field(default=330.0, gt=0.0)
    irodori_max_attempts: int = Field(default=3, ge=1, le=5)
    irodori_retry_backoff_seconds: float = Field(default=1.0, ge=0.0)
    tts_voice_id: str
    peer_identity_strict: bool
    tts_max_chunk_chars: int = Field(default=100, ge=20, le=200)
    tts_max_text_chars: int = Field(default=5000, ge=1, le=30000)
    tts_chunk_gap_ms: int = Field(default=200, ge=0, le=2000)
    tts_queue_timeout_seconds: float = Field(default=600.0, gt=0.0)
    log_level: Literal["DEBUG", "INFO", "WARNING", "ERROR", "CRITICAL"] = "INFO"

    _irodori_api_key: SecretStr = PrivateAttr(default=SecretStr(""))

    @property
    def irodori_api_key(self) -> SecretStr:
        """Read-only access to loaded API key."""
        return self._irodori_api_key

    @field_validator("tts_voice_id")
    @classmethod
    def _validate_voice(cls, v: str) -> str:
        stripped = v.strip()
        if stripped.lower() in ("", "none", "no-ref", "text-only"):
            raise ValueError(f"Reference voice ID is required and cannot be empty or disallowed; got '{v}'")
        return stripped

    @model_validator(mode="after")
    def validate_and_load_api_key(self) -> Self:
        file_path = self.irodori_api_key_file
        if not file_path.is_file():
            raise ValueError(f"API key file does not exist or is not a file: {file_path}")
        try:
            content = file_path.read_text(encoding="utf-8").strip()
        except OSError as e:
            raise ValueError(f"Failed to read API key file: {e}") from e
        if not content:
            raise ValueError("API key file is empty")
        self._irodori_api_key = SecretStr(content)
        return self
