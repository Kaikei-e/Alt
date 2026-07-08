"""Qwen3-TTS-12Hz-0.6B-CustomVoice engine driver.

Extracted from the engine-monolithic pipeline so other drivers (Supertonic
v3, etc.) can plug in via `core.engine.TTSEngine`.
"""

from __future__ import annotations

import asyncio
import logging
import os
from dataclasses import dataclass
from typing import TYPE_CHECKING, Any

import numpy as np

from ..engine import Voice

if TYPE_CHECKING:
    from ..infra.config import Settings  # noqa: TC004

logger = logging.getLogger(__name__)

MODEL_NAME = "qwen3-tts-12hz-0.6b-customvoice"


@dataclass(frozen=True, slots=True)
class VoiceConfig:
    """Maps an Alt-facing voice ID to a Qwen CustomVoice preset + style instruct."""

    id: str
    name: str
    gender: str
    qwen_speaker: str
    instruct: str


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

VOICES: tuple[Voice, ...] = tuple(
    Voice(id=v.id, name=v.name, gender=v.gender) for v in VOICES_CONFIG
)
VOICE_IDS: set[str] = {v.id for v in VOICES_CONFIG}
_VOICE_BY_ID: dict[str, VoiceConfig] = {v.id: v for v in VOICES_CONFIG}


class QwenEngine:
    """Qwen3-TTS-0.6B-CustomVoice driver."""

    def __init__(self, settings: Settings) -> None:
        self._settings = settings
        self._model: Any = None
        self._ready = False
        self._device, self._gpu_name = self._detect_device()

    @property
    def name(self) -> str:
        return MODEL_NAME

    @property
    def is_ready(self) -> bool:
        return self._ready

    @property
    def voices(self) -> tuple[Voice, ...]:
        return VOICES

    @property
    def voice_ids(self) -> set[str]:
        return VOICE_IDS

    @property
    def device(self) -> str:
        return self._device

    @property
    def gpu_name(self) -> str | None:
        return self._gpu_name

    @property
    def _device_map(self) -> str:
        return "cuda:0" if self._device == "cuda" else "cpu"

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
            except (RuntimeError, AssertionError) as err:
                logger.exception("GPU compute verification failed")
                if os.environ.get("TTS_ALLOW_CPU_FALLBACK") == "1":
                    logger.warning("Falling back to CPU (TTS_ALLOW_CPU_FALLBACK=1)")
                    return "cpu", None
                raise RuntimeError("GPU detected but compute verification failed") from err
            return "cuda", gpu_name

        logger.warning("No GPU detected (torch.cuda.is_available() = False)")
        if os.environ.get("TTS_ALLOW_CPU_FALLBACK") == "1":
            logger.warning("Falling back to CPU (TTS_ALLOW_CPU_FALLBACK=1)")
            return "cpu", None
        raise RuntimeError("No GPU detected. Set TTS_ALLOW_CPU_FALLBACK=1 to allow CPU fallback.")

    async def load(self) -> None:
        loop = asyncio.get_running_loop()
        await loop.run_in_executor(None, self._load_sync)

    def _load_sync(self) -> None:
        import torch
        from qwen_tts import Qwen3TTSModel

        dtype = getattr(torch, self._settings.qwen_dtype)

        # torch.backends.cudnn.benchmark defaults to TRUE on ROCm (opposite of
        # CUDA). With variable conv1d shapes from the codec's chunked_decode,
        # benchmark=True drives MIOpen into a workspace-search path that
        # PyTorch calls with workspace=0, forcing the naive solver fallback
        # (29s vs 251ms for a single conv on the same shape).
        torch.backends.cudnn.benchmark = False
        torch.backends.cudnn.deterministic = False

        logger.info(
            "Loading Qwen3-TTS model %s (device=%s, dtype=%s, attn=%s)...",
            self._settings.qwen_model_id,
            self._device_map,
            self._settings.qwen_dtype,
            self._settings.qwen_attn_implementation,
        )
        self._model = Qwen3TTSModel.from_pretrained(
            self._settings.qwen_model_id,
            device_map=self._device_map,
            dtype=dtype,
            attn_implementation=self._settings.qwen_attn_implementation,
        )
        self._ready = True
        logger.info("Qwen3-TTS model loaded successfully")
        self._warmup()

    def _warmup(self) -> None:
        """Pre-run a 1-character synthesis so the first user request does not
        pay the MIOpen / ROCm kernel JIT compile cost."""
        if self._model is None:
            return
        try:
            first_voice = VOICES_CONFIG[0]
            logger.info("Warming up Qwen3-TTS on %s ...", self._device_map)
            self._model.generate_custom_voice(
                text="は",
                language=self._language(),
                speaker=first_voice.qwen_speaker,
                instruct=first_voice.instruct,
            )
            logger.info("Qwen3-TTS warmup complete")
        except Exception:
            logger.exception("Qwen3-TTS warmup failed (continuing)")

    def unload(self) -> None:
        self._model = None
        self._ready = False
        logger.info("Qwen3-TTS engine unloaded")

    def synth_one(self, *, sentence: str, voice: str, speed: float) -> tuple[np.ndarray, int]:
        if self._model is None:
            raise RuntimeError("Engine not loaded")
        cfg = self._resolve_voice(voice)
        instruct = self._apply_speed_hint(cfg.instruct, speed)
        wavs, sr = self._model.generate_custom_voice(
            text=sentence,
            language=self._language(),
            speaker=cfg.qwen_speaker,
            instruct=instruct,
        )
        return np.asarray(wavs[0], dtype=np.float32), int(sr)

    async def keepalive_tick(self) -> None:
        """Run a no-op matmul on the model device to defeat AMD DPM idle downclock."""
        if self._device != "cuda" or self._model is None:
            return
        loop = asyncio.get_running_loop()
        await loop.run_in_executor(None, self._keepalive_sync)

    def _keepalive_sync(self) -> None:
        try:
            import torch

            t = torch.ones((128, 128), device="cuda")
            _ = (t @ t).sum().item()
        except Exception:
            logger.debug("keepalive matmul failed (continuing)", exc_info=True)

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
    def _language() -> str:
        # Qwen3-TTS supports lowercase language names.
        return "japanese"
