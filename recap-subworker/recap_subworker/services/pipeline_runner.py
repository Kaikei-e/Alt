"""Async interface that drives the pipeline worker pool."""

from __future__ import annotations

import asyncio
import multiprocessing
from concurrent.futures import ProcessPoolExecutor
from typing import Sequence

from ..domain.models import EvidenceRequest, EvidenceResponse, WarmupResponse
from ..infra.config import Settings
from . import pipeline_worker


class PipelineTaskRunner:
    """Dispatcher that executes evidence runs inside a dedicated process pool."""

    def __init__(self, settings: Settings) -> None:
        self._settings = settings
        ctx = multiprocessing.get_context("spawn")
        self._executor = ProcessPoolExecutor(
            max_workers=settings.pipeline_worker_processes,
            mp_context=ctx,
            initializer=pipeline_worker.initialize,
            initargs=(settings.model_dump(mode="json"),),
        )

    async def run(self, request: EvidenceRequest) -> EvidenceResponse:
        payload = request.model_dump(mode="json")
        future = self._executor.submit(pipeline_worker.run_pipeline, payload)
        result = await asyncio.wrap_future(future)
        return EvidenceResponse.model_validate(result)

    async def warmup(self, samples: Sequence[str] | None = None) -> WarmupResponse:
        future = self._executor.submit(pipeline_worker.warmup, list(samples or []))
        result = await asyncio.wrap_future(future)
        return WarmupResponse.model_validate(result)

    def shutdown(self) -> None:
        self._executor.shutdown(wait=False)
