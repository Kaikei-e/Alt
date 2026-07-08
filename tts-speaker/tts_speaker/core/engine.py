"""TTS engine Port.

Orchestration (text chunking, preprocess, async executor, GPU keepalive) lives
in `core.pipeline.TTSPipeline`. Each engine driver implements this Protocol —
single-sentence synthesis + lifecycle — and stays free of orchestration
concerns.
"""

from __future__ import annotations

from dataclasses import dataclass
from typing import TYPE_CHECKING, Protocol

if TYPE_CHECKING:
    import numpy as np


@dataclass(frozen=True, slots=True)
class Voice:
    """Alt-facing voice identity. Engine-specific mapping (e.g. Qwen speaker
    preset) stays in the driver's own config type — this is only the shape
    exposed across the engine/pipeline/service boundary."""

    id: str
    name: str
    gender: str


class TTSEngine(Protocol):
    """Single-sentence synthesis driver."""

    @property
    def name(self) -> str: ...

    @property
    def is_ready(self) -> bool: ...

    @property
    def voices(self) -> tuple[Voice, ...]: ...

    @property
    def voice_ids(self) -> set[str]: ...

    @property
    def device(self) -> str:
        """Logical device tag — `cuda` triggers the GPU keepalive loop."""
        ...

    @property
    def gpu_name(self) -> str | None: ...

    async def load(self) -> None: ...

    def unload(self) -> None: ...

    def synth_one(self, *, sentence: str, voice: str, speed: float) -> tuple["np.ndarray", int]:
        """Synthesize one sentence. Returns `(audio_float32, sample_rate)`.

        Implementations validate `voice` against `voice_ids` and raise
        `ValueError` on unknown ids.
        """
        ...

    async def keepalive_tick(self) -> None: ...
