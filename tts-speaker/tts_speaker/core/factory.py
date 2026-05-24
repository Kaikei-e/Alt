"""TTS engine selection by `Settings.engine`."""

from __future__ import annotations

from typing import TYPE_CHECKING

from .engine import TTSEngine
from .engines.qwen import QwenEngine

if TYPE_CHECKING:
    from ..infra.config import Settings


def build_engine(settings: Settings) -> TTSEngine:
    """Construct the configured TTS engine."""
    if settings.engine == "qwen":
        return QwenEngine(settings)
    raise ValueError(f"unknown TTS engine: {settings.engine}")
