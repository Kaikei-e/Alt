"""Supertonic v3 (Supertone/supertonic-3) engine driver.

CPU-only ONNX Runtime synthesis (`pip install supertonic`). The upstream
README states "GPU mode is not supported yet", so this driver always reports
`device == "cpu"` and the GPU keepalive tick is a no-op. Adding a ROCm /
MIGraphX EP path is a separate change once upstream surfaces provider
control.
"""

from __future__ import annotations

import asyncio
import logging
from dataclasses import dataclass
from typing import TYPE_CHECKING, Any

import numpy as np

if TYPE_CHECKING:
    from ..infra.config import Settings  # noqa: TC004

logger = logging.getLogger(__name__)

MODEL_NAME = "supertonic-3"
SAMPLE_RATE = 44100  # Supertonic v3: 44.1 kHz 16-bit WAV (README)


@dataclass(frozen=True, slots=True)
class SupVoiceConfig:
    """Maps an Alt-facing voice ID to a Supertonic preset voice name."""

    id: str
    name: str
    gender: str
    sup_voice: str


# Single-voice exposure decided in Phase 0 listening test.
# Extend by adding entries here when more voices are validated.
VOICES_CONFIG: tuple[SupVoiceConfig, ...] = (
    SupVoiceConfig(
        id="sup-F4",
        name="JA Voice (Supertonic F4)",
        gender="female",
        sup_voice="F4",
    ),
)

VOICES: list[dict[str, str]] = [
    {"id": v.id, "name": v.name, "gender": v.gender} for v in VOICES_CONFIG
]
VOICE_IDS: set[str] = {v.id for v in VOICES_CONFIG}
_VOICE_BY_ID: dict[str, SupVoiceConfig] = {v.id: v for v in VOICES_CONFIG}


class SupertonicEngine:
    """Supertonic v3 driver. CPU-only via ONNX Runtime CPUExecutionProvider."""

    def __init__(self, settings: Settings) -> None:
        self._settings = settings
        self._tts: Any = None
        self._styles: dict[str, Any] = {}
        self._ready = False

    @property
    def name(self) -> str:
        return MODEL_NAME

    @property
    def is_ready(self) -> bool:
        return self._ready

    @property
    def voices(self) -> list[dict[str, str]]:
        return VOICES

    @property
    def voice_ids(self) -> set[str]:
        return VOICE_IDS

    @property
    def device(self) -> str:
        return "cpu"

    @property
    def gpu_name(self) -> str | None:
        return None

    async def load(self) -> None:
        loop = asyncio.get_event_loop()
        await loop.run_in_executor(None, self._load_sync)

    def _load_sync(self) -> None:
        from supertonic import TTS  # type: ignore[import-not-found]

        logger.info("Loading Supertonic v3 (auto_download=True)...")
        self._tts = TTS(auto_download=True)
        # Pre-resolve every exposed voice style so synth_one is allocation-free.
        for cfg in VOICES_CONFIG:
            self._styles[cfg.id] = self._tts.get_voice_style(voice_name=cfg.sup_voice)
        self._ready = True
        logger.info("Supertonic v3 loaded successfully")
        self._warmup()

    def _warmup(self) -> None:
        """One short synthesis so the first user request does not pay the
        ONNX graph-init cost."""
        if self._tts is None:
            return
        try:
            cfg = VOICES_CONFIG[0]
            style = self._styles[cfg.id]
            logger.info("Warming up Supertonic v3 on %s ...", cfg.id)
            self._tts.synthesize("は", voice_style=style)
            logger.info("Supertonic v3 warmup complete")
        except Exception:
            logger.exception("Supertonic v3 warmup failed (continuing)")

    def unload(self) -> None:
        self._tts = None
        self._styles = {}
        self._ready = False
        logger.info("Supertonic v3 engine unloaded")

    def synth_one(self, *, sentence: str, voice: str, speed: float) -> tuple[np.ndarray, int]:
        if self._tts is None:
            raise RuntimeError("Engine not loaded")
        cfg = self._resolve_voice(voice)
        style = self._styles[cfg.id]
        wav, _duration = self._tts.synthesize(
            sentence,
            voice_style=style,
            total_steps=self._settings.sup_total_steps,
            speed=speed,
        )
        # Supertonic returns shape (1, num_samples); flatten to mono float32.
        audio = np.asarray(wav, dtype=np.float32)
        if audio.ndim > 1:
            audio = audio.reshape(-1)
        return audio, SAMPLE_RATE

    async def keepalive_tick(self) -> None:
        # CPU-only — no DPM downclock concern.
        return

    @staticmethod
    def _resolve_voice(voice: str) -> SupVoiceConfig:
        cfg = _VOICE_BY_ID.get(voice)
        if cfg is None:
            raise ValueError(f"unknown voice: {voice}")
        return cfg
