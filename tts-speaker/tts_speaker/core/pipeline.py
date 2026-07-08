"""TTS orchestration — sentence splitting + engine-injected synthesis.

The pipeline owns chunking, async dispatch, and stream coordination. Engine
specifics (model load, single-sentence synthesis, GPU keepalive) live behind
the `TTSEngine` Protocol.
"""

from __future__ import annotations

import asyncio
import contextlib
import logging
import re
import threading
from typing import TYPE_CHECKING, Any

import numpy as np

if TYPE_CHECKING:
    from collections.abc import AsyncGenerator

    from .engine import TTSEngine, Voice

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
        # `synth_one` is not guaranteed thread-safe across concurrent calls
        # into the same loaded model (torch/ONNX session state) — serialize
        # every engine invocation through this lock regardless of which
        # executor thread is calling in.
        self._synth_lock = threading.Lock()

    @property
    def engine(self) -> TTSEngine:
        return self._engine

    @property
    def is_ready(self) -> bool:
        return self._engine.is_ready

    @property
    def voices(self) -> tuple[Voice, ...]:
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
        loop = asyncio.get_running_loop()
        return await loop.run_in_executor(None, self._synthesize_sync, text, voice, speed)

    def _synthesize_sync(self, text: str, voice: str, speed: float) -> tuple[np.ndarray, int]:
        samples: list[np.ndarray] = []
        sr: int | None = None
        with self._synth_lock:
            for sentence in self._split_sentences(text):
                chunk, this_sr = self._engine.synth_one(
                    sentence=sentence, voice=voice, speed=speed
                )
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
    ) -> AsyncGenerator[tuple[np.ndarray, int], None]:
        """Synthesize and yield one (audio_float32, sample_rate) per sentence."""
        loop = asyncio.get_running_loop()
        # Bounded so a slow consumer applies backpressure to the producer
        # thread instead of letting every sentence's audio pile up in memory.
        queue: asyncio.Queue[Any] = asyncio.Queue(maxsize=2)
        cancel_event = threading.Event()

        def producer() -> None:
            try:
                for sentence in self._split_sentences(text):
                    if cancel_event.is_set():
                        return
                    with self._synth_lock:
                        chunk, sr = self._engine.synth_one(
                            sentence=sentence, voice=voice, speed=speed
                        )
                    if cancel_event.is_set():
                        return
                    asyncio.run_coroutine_threadsafe(queue.put((chunk, sr)), loop).result()
                asyncio.run_coroutine_threadsafe(queue.put(None), loop).result()
            except Exception as exc:
                asyncio.run_coroutine_threadsafe(queue.put(exc), loop).result()

        future = loop.run_in_executor(None, producer)
        get_task: asyncio.Task[Any] | None = None

        try:
            while True:
                get_task = asyncio.ensure_future(queue.get())
                done, _ = await asyncio.wait(
                    {get_task, future}, return_when=asyncio.FIRST_COMPLETED
                )
                if get_task not in done:
                    # The producer future finished (or died) without ever
                    # reaching the queue again — stop waiting on a queue
                    # nobody feeds instead of hanging forever.
                    break
                item = get_task.result()
                get_task = None
                if item is None:
                    break
                if isinstance(item, Exception):
                    raise item
                yield item
        finally:
            # Consumer disconnected (GeneratorExit) or the loop raised early —
            # tell the producer thread to stop between sentences instead of
            # letting it synthesize the rest of the text into a dead queue,
            # and wait for it to actually finish before we return.
            cancel_event.set()
            if get_task is not None and not get_task.done():
                get_task.cancel()
                with contextlib.suppress(asyncio.CancelledError):
                    await get_task
            # Drain any item the producer is currently blocked trying to put
            # so a bounded, unconsumed queue can't deadlock the producer
            # thread against `future` below.
            with contextlib.suppress(asyncio.QueueEmpty):
                while True:
                    queue.get_nowait()
            future.cancel()
            with contextlib.suppress(asyncio.CancelledError):
                await future
