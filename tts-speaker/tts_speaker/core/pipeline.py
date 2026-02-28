"""TTSPipeline wrapper around Kokoro-82M."""

from __future__ import annotations

import asyncio
import logging
import os
from typing import TYPE_CHECKING

import numpy as np

if TYPE_CHECKING:
    pass

logger = logging.getLogger(__name__)

# Split pattern for Japanese sentence boundaries.
# Lookbehind keeps punctuation attached to the preceding segment (affects prosody).
# Kokoro's default '\n+' only splits on newlines, causing 400-char chunks that
# exceed the 510 phoneme limit for Japanese text (2-3 phonemes/char).
JAPANESE_SPLIT_PATTERN = r"(?<=[。！？\n])"

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
        self._device, self._gpu_name = self._detect_device()

    @staticmethod
    def _detect_device() -> tuple[str, str | None]:
        """Detect GPU (ROCm/HIP or CUDA), fall back to CPU.

        Returns:
            Tuple of (device, gpu_name). Raises RuntimeError if no GPU
            is detected unless TTS_ALLOW_CPU_FALLBACK=1.
        """
        import torch

        # Force CPU mode (useful when GPU is detected but MIOpen ops fail)
        if os.environ.get("TTS_FORCE_CPU") == "1":
            logger.info("TTS_FORCE_CPU=1, using CPU")
            return "cpu", None

        hsa_ver = os.environ.get("HSA_OVERRIDE_GFX_VERSION")
        hip_dev = os.environ.get("HIP_VISIBLE_DEVICES")
        logger.info("HSA_OVERRIDE_GFX_VERSION=%s, HIP_VISIBLE_DEVICES=%s", hsa_ver, hip_dev)

        if torch.cuda.is_available():
            gpu_name = torch.cuda.get_device_name(0)
            logger.info("GPU detected: %s", gpu_name)

            # Verify GPU compute actually works
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
        raise RuntimeError(
            "No GPU detected. Set TTS_ALLOW_CPU_FALLBACK=1 to allow CPU fallback."
        )

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
        # Note: .half() is NOT used because ROCm LSTM does not support FP16,
        # causing "parameter types mismatch" at inference time.
        if self._device == "cuda":
            logger.info("GPU inference enabled (FP32 — ROCm LSTM requires FP32)")
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

    async def synthesize_stream(
        self,
        *,
        text: str,
        voice: str = "jf_alpha",
        speed: float = 1.0,
    ):  # -> AsyncGenerator[np.ndarray, None]
        """Synthesize text to audio stream (yielding 24kHz float32 ndarray chunks).

        Runs the blocking TTS inference in a thread executor, yielding chunks as they are generated.
        """
        loop = asyncio.get_event_loop()
        # We need a queue to pass chunks from the sync thread to the async generator
        queue = asyncio.Queue()

        def producer():
            try:
                if not self._pipeline:
                    raise RuntimeError("Pipeline not loaded")

                # The pipeline call is a generator
                for _gs, _ps, audio in self._pipeline(
                    text, voice=voice, speed=speed, split_pattern=JAPANESE_SPLIT_PATTERN
                ):
                    if audio is not None:
                        if hasattr(audio, "cpu"):
                            audio = audio.cpu().numpy()
                        loop.call_soon_threadsafe(queue.put_nowait, audio)

                # Signal end of stream
                loop.call_soon_threadsafe(queue.put_nowait, None)
            except Exception as e:
                loop.call_soon_threadsafe(queue.put_nowait, e)

        # Run producer in executor
        loop.run_in_executor(None, producer)

        while True:
            chunk = await queue.get()
            if chunk is None:
                break
            if isinstance(chunk, Exception):
                raise chunk
            yield chunk.astype(np.float32)

    def _synthesize_sync(self, text: str, voice: str, speed: float) -> np.ndarray:
        """Synchronous synthesis."""
        if not self._pipeline:
            raise RuntimeError("Pipeline not loaded")

        samples = []
        for _gs, _ps, audio in self._pipeline(
            text, voice=voice, speed=speed, split_pattern=JAPANESE_SPLIT_PATTERN
        ):
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
