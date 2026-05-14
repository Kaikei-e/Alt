"""TTSPipeline wrapper around Qwen3-TTS-12Hz-0.6B-CustomVoice."""

from __future__ import annotations

import asyncio
import logging
import os
import re
from dataclasses import dataclass
from typing import TYPE_CHECKING, Any

import numpy as np

if TYPE_CHECKING:
    from collections.abc import AsyncIterator

logger = logging.getLogger(__name__)

# Sentence boundary regex. Inherited from Kokoro-era ADR-000304: lookbehind keeps
# punctuation attached so prosody isn't broken by re.split. Still required for
# Qwen3-TTS because we synthesize one sentence per call to keep SynthesizeStream
# chunks small (UX) and to bound peak VRAM during long-form synthesis.
JAPANESE_SPLIT_PATTERN = r"(?<=[。！？\n])"

MODEL_NAME = "qwen3-tts-12hz-0.6b-customvoice"


@dataclass(frozen=True, slots=True)
class VoiceConfig:
    """Maps an Alt-facing voice ID to a Qwen CustomVoice preset + style instruct."""

    id: str
    name: str
    gender: str
    qwen_speaker: str
    instruct: str


# Voice slots selected after subjective review of the 9 Qwen CustomVoice presets
# speaking Japanese (see voice-samples/ + manifest.json). Three female presets
# kept; the Alt-facing ID is stable so future re-ranking does not break clients.
# `qwen_speaker` is the actual preset name passed to generate_custom_voice.
VOICES_CONFIG: tuple[VoiceConfig, ...] = (
    VoiceConfig(
        id="qwen-ja-1",
        name="JA Voice 1",
        gender="female",
        qwen_speaker="sohee",
        instruct="自然なペースで日本語で読み上げてください",
    ),
    VoiceConfig(
        id="qwen-ja-2",
        name="JA Voice 2",
        gender="female",
        qwen_speaker="serena",
        instruct="自然なペースで日本語で読み上げてください",
    ),
    VoiceConfig(
        id="qwen-ja-3",
        name="JA Voice 3",
        gender="female",
        qwen_speaker="ono_anna",
        instruct="自然なペースで日本語で読み上げてください",
    ),
)

VOICES: list[dict[str, str]] = [
    {"id": v.id, "name": v.name, "gender": v.gender} for v in VOICES_CONFIG
]
VOICE_IDS: set[str] = {v.id for v in VOICES_CONFIG}
_VOICE_BY_ID: dict[str, VoiceConfig] = {v.id: v for v in VOICES_CONFIG}


class TTSPipeline:
    """Wrapper around Qwen3TTSModel for Japanese TTS via CustomVoice presets.

    Loads a single multi-speaker Qwen3-TTS-12Hz-0.6B-CustomVoice model and
    routes Alt's voice IDs to the corresponding preset speaker name via
    VOICES_CONFIG. Inference runs in a thread executor to avoid blocking the
    asyncio loop. SynthesizeStream splits text on Japanese sentence punctuation
    and yields one (chunk, sr) tuple per sentence.
    """

    def __init__(self) -> None:
        self._model: Any = None
        self._ready = False
        self._device, self._gpu_name = self._detect_device()

    @staticmethod
    def _detect_device() -> tuple[str, str | None]:
        """Detect GPU (ROCm/HIP or CUDA), fall back to CPU.

        Returns:
            Tuple of (device, gpu_name). Raises RuntimeError if no GPU is
            detected unless TTS_ALLOW_CPU_FALLBACK=1.
        """
        import torch

        if os.environ.get("TTS_FORCE_CPU") == "1":
            logger.info("TTS_FORCE_CPU=1, using CPU")
            return "cpu", None

        hsa_ver = os.environ.get("HSA_OVERRIDE_GFX_VERSION")
        hip_dev = os.environ.get("HIP_VISIBLE_DEVICES")
        logger.info("HSA_OVERRIDE_GFX_VERSION=%s, HIP_VISIBLE_DEVICES=%s", hsa_ver, hip_dev)

        if torch.cuda.is_available():
            gpu_name = torch.cuda.get_device_name(0)
            logger.info("GPU detected: %s", gpu_name)

            try:
                t = torch.tensor([1.0, 2.0], device="cuda")
                result = (t * t).sum().item()
                assert result == 5.0  # noqa: S101
                logger.info("GPU compute verification passed")
            except Exception:
                logger.exception("GPU compute verification failed")
                if os.environ.get("TTS_ALLOW_CPU_FALLBACK") == "1":
                    logger.warning("Falling back to CPU (TTS_ALLOW_CPU_FALLBACK=1)")
                    return "cpu", None
                raise RuntimeError("GPU detected but compute verification failed")

            return "cuda", gpu_name

        logger.warning("No GPU detected (torch.cuda.is_available() = False)")
        if os.environ.get("TTS_ALLOW_CPU_FALLBACK") == "1":
            logger.warning("Falling back to CPU (TTS_ALLOW_CPU_FALLBACK=1)")
            return "cpu", None
        raise RuntimeError("No GPU detected. Set TTS_ALLOW_CPU_FALLBACK=1 to allow CPU fallback.")

    @property
    def is_ready(self) -> bool:
        return self._ready

    @property
    def voices(self) -> list[dict[str, str]]:
        return VOICES

    @property
    def device_map(self) -> str:
        return "cuda:0" if self._device == "cuda" else "cpu"

    async def load(self) -> None:
        """Load the Qwen3-TTS model (blocking call run in executor)."""
        loop = asyncio.get_event_loop()
        await loop.run_in_executor(None, self._load_sync)

    def _load_sync(self) -> None:
        """Synchronous model loading."""
        import torch
        from qwen_tts import Qwen3TTSModel

        from ..infra.config import get_settings

        settings = get_settings()
        dtype = getattr(torch, settings.qwen_dtype)

        logger.info(
            "Loading Qwen3-TTS model %s (device=%s, dtype=%s, attn=%s)...",
            settings.qwen_model_id,
            self.device_map,
            settings.qwen_dtype,
            settings.qwen_attn_implementation,
        )
        self._model = Qwen3TTSModel.from_pretrained(
            settings.qwen_model_id,
            device_map=self.device_map,
            dtype=dtype,
            attn_implementation=settings.qwen_attn_implementation,
        )
        self._ready = True
        logger.info("Qwen3-TTS model loaded successfully")

    @staticmethod
    def _resolve_voice(voice: str) -> VoiceConfig:
        cfg = _VOICE_BY_ID.get(voice)
        if cfg is None:
            raise ValueError(f"unknown voice: {voice}")
        return cfg

    @staticmethod
    def _apply_speed_hint(base_instruct: str, speed: float) -> str:
        """Project the legacy `speed` parameter onto Qwen's `instruct` knob.

        Qwen3-TTS-12Hz has no scalar speed; pacing is controlled by the
        natural-language instruct string. Map the legacy 0.5–2.0 range to a
        small set of pacing hints to preserve API shape compatibility with
        Kokoro callers.
        """
        if speed <= 0.85:
            suffix = "、ゆっくりと"
        elif speed >= 1.15:
            suffix = "、少し速めに"
        else:
            suffix = ""
        return base_instruct + suffix

    @staticmethod
    def _split_sentences(text: str) -> list[str]:
        parts = [s for s in re.split(JAPANESE_SPLIT_PATTERN, text) if s.strip()]
        return parts if parts else [text]

    def _synth_one(self, sentence: str, cfg: VoiceConfig, instruct: str) -> tuple[np.ndarray, int]:
        if self._model is None:
            raise RuntimeError("Pipeline not loaded")
        wavs, sr = self._model.generate_custom_voice(
            text=sentence,
            language="Japanese",
            speaker=cfg.qwen_speaker,
            instruct=instruct,
        )
        return np.asarray(wavs[0], dtype=np.float32), int(sr)

    async def synthesize(
        self,
        *,
        text: str,
        voice: str = "qwen-ja-1",
        speed: float = 1.0,
    ) -> tuple[np.ndarray, int]:
        """Synthesize text to audio. Returns (audio_float32, sample_rate)."""
        loop = asyncio.get_event_loop()
        return await loop.run_in_executor(None, self._synthesize_sync, text, voice, speed)

    def _synthesize_sync(self, text: str, voice: str, speed: float) -> tuple[np.ndarray, int]:
        cfg = self._resolve_voice(voice)
        instruct = self._apply_speed_hint(cfg.instruct, speed)
        samples: list[np.ndarray] = []
        sr: int | None = None
        for sentence in self._split_sentences(text):
            chunk, this_sr = self._synth_one(sentence, cfg, instruct)
            samples.append(chunk)
            sr = this_sr
        if sr is None or not samples:
            raise RuntimeError("No audio generated")
        return np.concatenate(samples).astype(np.float32), sr

    async def synthesize_stream(
        self,
        *,
        text: str,
        voice: str = "qwen-ja-1",
        speed: float = 1.0,
    ) -> AsyncIterator[tuple[np.ndarray, int]]:
        """Synthesize and yield one (audio_float32, sample_rate) per sentence."""
        loop = asyncio.get_event_loop()
        queue: asyncio.Queue[Any] = asyncio.Queue()

        cfg = self._resolve_voice(voice)
        instruct = self._apply_speed_hint(cfg.instruct, speed)

        def producer() -> None:
            try:
                for sentence in self._split_sentences(text):
                    chunk, sr = self._synth_one(sentence, cfg, instruct)
                    loop.call_soon_threadsafe(queue.put_nowait, (chunk, sr))
                loop.call_soon_threadsafe(queue.put_nowait, None)
            except Exception as exc:
                loop.call_soon_threadsafe(queue.put_nowait, exc)

        loop.run_in_executor(None, producer)

        while True:
            item = await queue.get()
            if item is None:
                break
            if isinstance(item, Exception):
                raise item
            yield item

    def unload(self) -> None:
        """Unload the pipeline and free resources."""
        self._model = None
        self._ready = False
        logger.info("Qwen3-TTS pipeline unloaded")
