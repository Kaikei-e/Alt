"""Async interface that drives the pipeline worker pool."""

from __future__ import annotations

import asyncio
import multiprocessing
import signal
import threading
import time
from typing import Sequence

from ..domain.models import EvidenceRequest, EvidenceResponse, WarmupResponse
from ..infra.config import Settings
from . import pipeline_worker


class PipelineTaskRunner:
    """Dispatcher that executes evidence runs inside a dedicated process pool."""

    def __init__(self, settings: Settings) -> None:
        import structlog

        logger = structlog.get_logger(__name__)
        self._settings = settings
        ctx = multiprocessing.get_context("spawn")

        # Use multiprocessing.Pool instead of ProcessPoolExecutor to support maxtasksperchild
        # This prevents memory leaks by periodically replacing worker processes
        logger.info(
            "initializing pipeline worker pool",
            processes=settings.pipeline_worker_processes,
            max_tasks_per_child=settings.pipeline_worker_max_tasks_per_child,
        )

        self._pool = ctx.Pool(
            processes=settings.pipeline_worker_processes,
            initializer=pipeline_worker.initialize,
            initargs=(settings.model_dump(mode="json"),),
            maxtasksperchild=settings.pipeline_worker_max_tasks_per_child,
        )

        # Verify worker initialization with timeout
        self._verify_worker_initialization()

    def _verify_worker_initialization(self) -> None:
        """Verify that worker processes are initialized correctly with a timeout."""
        import structlog

        logger = structlog.get_logger(__name__)
        timeout = self._settings.pipeline_worker_init_timeout_seconds

        # Try a simple warmup to verify workers are ready
        try:
            future = self._pool.apply_async(pipeline_worker.warmup, ([]))
            result = future.get(timeout=timeout)
            logger.info(
                "pipeline worker pool initialized successfully",
                timeout_seconds=timeout,
            )
        except Exception as exc:
            logger.error(
                "pipeline worker pool initialization failed",
                error=str(exc),
                error_type=type(exc).__name__,
                timeout_seconds=timeout,
                exc_info=True,
            )
            # Clean up failed pool
            try:
                self._pool.terminate()
                self._pool.join(timeout=5.0)
            except Exception:
                pass
            raise RuntimeError(
                f"Failed to initialize pipeline worker pool within {timeout}s"
            ) from exc

    async def run(self, request: EvidenceRequest) -> EvidenceResponse:
        payload = request.model_dump(mode="json")
        loop = asyncio.get_event_loop()
        result = await loop.run_in_executor(
            None, self._pool.apply, pipeline_worker.run_pipeline, (payload,)
        )
        return EvidenceResponse.model_validate(result)

    async def warmup(self, samples: Sequence[str] | None = None) -> WarmupResponse:
        loop = asyncio.get_event_loop()
        result = await loop.run_in_executor(
            None, self._pool.apply, pipeline_worker.warmup, (list(samples or []),)
        )
        return WarmupResponse.model_validate(result)

    def shutdown(self) -> None:
        """Shutdown the process pool, waiting for running tasks to complete.

        This ensures worker processes are properly cleaned up to prevent memory leaks.
        """
        import structlog

        logger = structlog.get_logger(__name__)
        logger.info("shutting down PipelineTaskRunner")

        if self._pool is None:
            return

        try:
            # Close the pool to prevent new tasks
            self._pool.close()

            # Wait for running tasks to complete with timeout
            shutdown_timeout = 30.0
            start_time = time.time()

            def wait_for_completion():
                self._pool.join(timeout=shutdown_timeout)

            wait_thread = threading.Thread(target=wait_for_completion, daemon=True)
            wait_thread.start()
            wait_thread.join(timeout=shutdown_timeout + 5.0)

            elapsed = time.time() - start_time
            if elapsed >= shutdown_timeout:
                logger.warning(
                    "process pool shutdown timed out, terminating workers",
                    timeout_seconds=shutdown_timeout,
                )
                # Force termination if timeout
                self._pool.terminate()
                self._pool.join(timeout=5.0)
            else:
                logger.info(
                    "process pool shutdown complete",
                    elapsed_seconds=elapsed,
                )
        except Exception as exc:
            logger.warning(
                "error during process pool shutdown",
                error=str(exc),
                error_type=type(exc).__name__,
                exc_info=True,
            )
            # Force termination on error
            try:
                self._pool.terminate()
                self._pool.join(timeout=5.0)
            except Exception:
                pass
        finally:
            self._pool = None

        logger.info("PipelineTaskRunner shutdown complete")
