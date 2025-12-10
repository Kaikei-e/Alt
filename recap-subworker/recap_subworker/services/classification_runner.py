"""Async interface that drives the classification worker pool."""

from __future__ import annotations

import asyncio
import multiprocessing
import threading
import time
from typing import Any

from ..infra.config import Settings
from . import classification_worker


class ClassificationRunner:
    """Dispatcher that executes classification runs inside a dedicated process pool."""

    def __init__(self, settings: Settings) -> None:
        import structlog

        logger = structlog.get_logger(__name__)
        self._settings = settings
        ctx = multiprocessing.get_context("spawn")

        # Use multiprocessing.Pool instead of ProcessPoolExecutor to support maxtasksperchild
        # This prevents memory leaks by periodically replacing worker processes
        logger.info(
            "initializing classification worker pool",
            processes=settings.classification_worker_processes,
            max_tasks_per_child=settings.classification_worker_max_tasks_per_child,
        )

        self._pool = ctx.Pool(
            processes=settings.classification_worker_processes,
            initializer=classification_worker.initialize,
            initargs=(settings.model_dump(mode="json"),),
            maxtasksperchild=settings.classification_worker_max_tasks_per_child,
        )

        # Verify worker initialization with timeout
        self._verify_worker_initialization()

    def _verify_worker_initialization(self) -> None:
        """Verify that worker processes are initialized correctly with a timeout."""
        import structlog

        logger = structlog.get_logger(__name__)
        timeout = self._settings.classification_worker_init_timeout_seconds

        # Try a simple warmup to verify workers are ready
        try:
            future = self._pool.apply_async(classification_worker.predict_batch, ([],))
            result = future.get(timeout=timeout)
            logger.info(
                "classification worker pool initialized successfully",
                timeout_seconds=timeout,
            )
        except Exception as exc:
            logger.error(
                "classification worker pool initialization failed",
                error=str(exc),
                error_type=type(exc).__name__,
                timeout_seconds=timeout,
                exc_info=True,
            )
            # Clean up failed pool
            try:
                self._pool.terminate()
                self._pool.join()
            except Exception:
                pass
            raise RuntimeError(
                f"Failed to initialize classification worker pool within {timeout}s"
            ) from exc

    async def predict_batch(self, texts: list[str]) -> list[dict[str, Any]]:
        """Execute classification in a worker process."""
        # Use apply_async for non-blocking execution, then wrap the AsyncResult in asyncio
        async_result = self._pool.apply_async(classification_worker.predict_batch, (texts,))
        # Wait for the result in an executor to avoid blocking the event loop
        loop = asyncio.get_event_loop()
        result = await loop.run_in_executor(None, async_result.get)
        return result

    def shutdown(self) -> None:
        """Shutdown the process pool, waiting for running tasks to complete.

        This ensures worker processes are properly cleaned up to prevent memory leaks.
        """
        import structlog

        logger = structlog.get_logger(__name__)
        logger.info("shutting down ClassificationRunner")

        if self._pool is None:
            return

        try:
            # Close the pool to prevent new tasks
            self._pool.close()

            # Wait for running tasks to complete with timeout
            shutdown_timeout = 30.0
            start_time = time.time()

            def wait_for_completion():
                self._pool.join()

            wait_thread = threading.Thread(target=wait_for_completion, daemon=True)
            wait_thread.start()
            wait_thread.join(timeout=shutdown_timeout + 5.0)

            elapsed = time.time() - start_time
            if elapsed >= shutdown_timeout:
                logger.warning(
                    "classification process pool shutdown timed out, terminating workers",
                    timeout_seconds=shutdown_timeout,
                )
                # Force termination if timeout
                self._pool.terminate()
                self._pool.join()
            else:
                logger.info(
                    "classification process pool shutdown complete",
                    elapsed_seconds=elapsed,
                )
        except Exception as exc:
            logger.warning(
                "error during classification process pool shutdown",
                error=str(exc),
                error_type=type(exc).__name__,
                exc_info=True,
            )
            # Force termination on error
            try:
                self._pool.terminate()
                self._pool.join()
            except Exception:
                pass
        finally:
            self._pool = None

        logger.info("ClassificationRunner shutdown complete")

