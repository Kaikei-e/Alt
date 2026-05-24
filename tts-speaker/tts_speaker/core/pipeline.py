"""TTS orchestration — sentence splitting + engine-injected synthesis.

The pipeline owns chunking, async dispatch, and stream coordination. Engine
specifics (model load, single-sentence synthesis, GPU keepalive) live behind
the `TTSEngine` Protocol.
"""

from __future__ import annotations

import asyncio
import logging
import re
from typing import TYPE_CHECKING, Any

import numpy as np

if TYPE_CHECKING:
    from collections.abc import AsyncIterator

    from .engine import TTSEngine

logger = logging.getLogger(__name__)

# Sentence boundary regex. Inherited from Kokoro-era ADR-000304: lookbehind
# keeps punctuation attached so prosody isn't broken by re.split.
JAPANESE_SPLIT_PATTERN = r"(?<=[。！？\n])"

# Comma-level split fallback. `、` is used when a single sentence (between
# full stops) has accumulated COMMA_SPLIT_THRESHOLD_CHARS or more characters
# — without this, a single 200-char sentence keeps the FE waiting for the
# whole synthesis before the first chunk arrives.
COMMA_SPLIT_THRESHOLD_CHARS = 30


class TTSPipeline:
    """Engine-agnostic synthesis orchestration."""

    def __init__(self, engine: TTSEngine) -> None:
        self._engine = engine

    @property
    def engine(self) -> TTSEngine:
        return self._engine

    @property
    def is_ready(self) -> bool:
        return self._engine.is_ready

    @property
    def voices(self) -> list[dict[str, str]]:
        return self._engine.voices

    @property
    def voice_ids(self) -> set[str]:
        return self._engine.voice_ids

    @property
    def device(self) -> str:
        return self._engine.device

    @property
    def gpu_name(self) -> str | None:
        return self._engine.gpu_name

    async def load(self) -> None:
        await self._engine.load()

    def unload(self) -> None:
        self._engine.unload()

    async def keepalive_tick(self) -> None:
        await self._engine.keepalive_tick()

    @staticmethod
    def _split_sentences(text: str) -> list[str]:
        """Split on sentence terminators, with a comma-level fallback for long runs."""
        primary = [s for s in re.split(JAPANESE_SPLIT_PATTERN, text) if s.strip()]
        if not primary:
            return [text]
        result: list[str] = []
        for segment in primary:
            if len(segment) < COMMA_SPLIT_THRESHOLD_CHARS or "、" not in segment:
                result.append(segment)
                continue
            sub = [s for s in re.split(r"(?<=、)", segment) if s.strip()]
            result.extend(sub if sub else [segment])
        return result

    async def synthesize(
        self,
        *,
        text: str,
        voice: str,
        speed: float = 1.0,
    ) -> tuple[np.ndarray, int]:
        """Synthesize text to audio. Returns (audio_float32, sample_rate)."""
        loop = asyncio.get_event_loop()
        return await loop.run_in_executor(None, self._synthesize_sync, text, voice, speed)

    def _synthesize_sync(self, text: str, voice: str, speed: float) -> tuple[np.ndarray, int]:
        samples: list[np.ndarray] = []
        sr: int | None = None
        for sentence in self._split_sentences(text):
            chunk, this_sr = self._engine.synth_one(sentence=sentence, voice=voice, speed=speed)
            samples.append(chunk)
            sr = this_sr
        if sr is None or not samples:
            raise RuntimeError("No audio generated")
        return np.concatenate(samples).astype(np.float32), sr

    async def synthesize_stream(
        self,
        *,
        text: str,
        voice: str,
        speed: float = 1.0,
    ) -> AsyncIterator[tuple[np.ndarray, int]]:
        """Synthesize and yield one (audio_float32, sample_rate) per sentence."""
        loop = asyncio.get_event_loop()
        queue: asyncio.Queue[Any] = asyncio.Queue()

        def producer() -> None:
            try:
                for sentence in self._split_sentences(text):
                    chunk, sr = self._engine.synth_one(sentence=sentence, voice=voice, speed=speed)
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
