"""TTSPipeline wrapper around Kokoro-82M."""

from __future__ import annotations

import asyncio
import logging
from typing import TYPE_CHECKING

import numpy as np

if TYPE_CHECKING:
    pass

logger = logging.getLogger(__name__)

VOICES = [
    {"id": "jf_alpha", "name": "Alpha", "gender": "female"},
    {"id": "jf_gongitsune", "name": "Gongitsune", "gender": "female"},
    {"id": "jf_nezumi", "name": "Nezumi", "gender": "female"},
    {"id": "jf_tebukuro", "name": "Tebukuro", "gender": "female"},
    {"id": "jm_kumo", "name": "Kumo", "gender": "male"},
]

VOICE_IDS = {v["id"] for v in VOICES}


class TTSPipeline:
    """Wrapper around KPipeline for Kokoro-82M Japanese TTS."""

    def __init__(self) -> None:
        self._pipeline = None
        self._ready = False
        self._device = self._detect_device()

    @staticmethod
    def _detect_device() -> str:
        """Detect GPU (ROCm/HIP or CUDA), fall back to CPU."""
        import torch

        if torch.cuda.is_available():
            name = torch.cuda.get_device_name(0)
            logger.info("GPU detected: %s", name)
            return "cuda"
        logger.info("No GPU detected, using CPU")
        return "cpu"

    @property
    def is_ready(self) -> bool:
        return self._ready

    @property
    def voices(self) -> list[dict]:
        return VOICES

    async def load(self) -> None:
        """Load the KPipeline model (blocking call run in executor)."""
        loop = asyncio.get_event_loop()
        await loop.run_in_executor(None, self._load_sync)

    def _load_sync(self) -> None:
        """Synchronous model loading."""
        from kokoro import KPipeline

        logger.info("Loading Kokoro-82M pipeline (lang=ja, device=%s)...", self._device)
        self._pipeline = KPipeline(lang_code="j", device=self._device)
        # FP16 on GPU for ROCm 7.2 performance
        if self._device == "cuda":
            self._pipeline.model.half()
            logger.info("FP16 enabled for GPU inference")
        self._ready = True
        logger.info("Kokoro-82M pipeline loaded successfully")

    async def synthesize(
        self,
        *,
        text: str,
        voice: str = "jf_alpha",
        speed: float = 1.0,
    ) -> np.ndarray:
        """Synthesize text to audio (24kHz float32 ndarray).

        Runs the blocking TTS inference in a thread executor.
        """
        loop = asyncio.get_event_loop()
        return await loop.run_in_executor(
            None, self._synthesize_sync, text, voice, speed
        )

    def _synthesize_sync(self, text: str, voice: str, speed: float) -> np.ndarray:
        """Synchronous synthesis."""
        if not self._pipeline:
            raise RuntimeError("Pipeline not loaded")

        samples = []
        for _gs, _ps, audio in self._pipeline(text, voice=voice, speed=speed):
            if audio is not None:
                samples.append(audio)

        if not samples:
            raise RuntimeError("No audio generated")

        return np.concatenate(samples).astype(np.float32)

    def unload(self) -> None:
        """Unload the pipeline and free resources."""
        self._pipeline = None
        self._ready = False
        logger.info("Kokoro-82M pipeline unloaded")
