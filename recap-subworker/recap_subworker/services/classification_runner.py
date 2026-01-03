"""Async interface that drives the classification worker pool."""

from __future__ import annotations

import asyncio
import multiprocessing
import threading
import time
from typing import Any

from ..infra.config import Settings
# classification_worker is lazily imported in _ensure_pool() to avoid CUDA fork issues
# See: https://docs.pytorch.org/docs/stable/notes/multiprocessing.html


class ClassificationRunner:
    """Dispatcher that executes classification runs inside a dedicated process pool.

    The pool is initialized on-demand (lazy initialization) and automatically
    shuts down after a configurable idle timeout to save memory.
    """

    def __init__(self, settings: Settings) -> None:
        import structlog

        logger = structlog.get_logger(__name__)
        self._settings = settings
        self._pool: multiprocessing.Pool | None = None
        self._lock = threading.Lock()
        self._active_tasks = 0
        self._last_task_time: float | None = None
        self._idle_timer: threading.Timer | None = None
        self._shutting_down = False

        logger.info(
            "initializing ClassificationRunner (lazy pool initialization)",
            idle_timeout_seconds=settings.classification_pool_idle_timeout_seconds,
        )

    def _ensure_pool(self) -> multiprocessing.Pool:
        """Ensure the worker pool is initialized, creating it if necessary."""
        import structlog

        # Lazy import to avoid importing torch in master process before fork
        # This prevents "Cannot re-initialize CUDA in forked subprocess" errors
        from . import classification_worker

        logger = structlog.get_logger(__name__)

        with self._lock:
            if self._pool is not None:
                return self._pool

            if self._shutting_down:
                raise RuntimeError("ClassificationRunner is shutting down")

            logger.info(
                "initializing classification worker pool",
                processes=self._settings.classification_worker_processes,
                max_tasks_per_child=self._settings.classification_worker_max_tasks_per_child,
            )

            ctx = multiprocessing.get_context("spawn")
            self._pool = ctx.Pool(
                processes=self._settings.classification_worker_processes,
                initializer=classification_worker.initialize,
                initargs=(self._settings.model_dump(mode="json"),),
                maxtasksperchild=self._settings.classification_worker_max_tasks_per_child,
            )

            # Verify worker initialization with timeout
            self._verify_worker_initialization()

            logger.info("classification worker pool initialized successfully")
            return self._pool

    def _verify_worker_initialization(self) -> None:
        """Verify that worker processes are initialized correctly with a timeout."""
        import structlog

        from . import classification_worker  # Lazy import

        logger = structlog.get_logger(__name__)
        timeout = self._settings.classification_worker_init_timeout_seconds

        if self._pool is None:
            raise RuntimeError("Pool is None during verification")

        # Try a simple warmup to verify workers are ready
        try:
            future = self._pool.apply_async(classification_worker.predict_batch, ([],))
            result = future.get(timeout=timeout)
            logger.info(
                "classification worker pool verified successfully",
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
                if self._pool is not None:
                    self._pool.terminate()
                    self._pool.join()
            except Exception:
                pass
            self._pool = None
            raise RuntimeError(
                f"Failed to initialize classification worker pool within {timeout}s"
            ) from exc

    def _cancel_idle_timer(self) -> None:
        """Cancel the idle shutdown timer if it's running."""
        with self._lock:
            if self._idle_timer is not None:
                self._idle_timer.cancel()
                self._idle_timer = None

    def _schedule_idle_shutdown(self) -> None:
        """Schedule shutdown of the pool after idle timeout."""
        import structlog

        logger = structlog.get_logger(__name__)

        with self._lock:
            # Cancel existing timer if any
            if self._idle_timer is not None:
                self._idle_timer.cancel()

            # Only schedule shutdown if pool exists and no active tasks
            if self._pool is None or self._active_tasks > 0:
                return

            timeout = self._settings.classification_pool_idle_timeout_seconds
            logger.info(
                "scheduling idle shutdown",
                timeout_seconds=timeout,
            )

            def shutdown_after_timeout():
                with self._lock:
                    # Double-check conditions before shutting down
                    if self._pool is None or self._active_tasks > 0:
                        return

                    logger.info("idle timeout reached, shutting down classification worker pool")
                    self._shutdown_pool_internal()

            self._idle_timer = threading.Timer(timeout, shutdown_after_timeout)
            self._idle_timer.daemon = True
            self._idle_timer.start()

    def _shutdown_pool_internal(self) -> None:
        """Internal method to shutdown the pool (must be called with lock held)."""
        import structlog

        logger = structlog.get_logger(__name__)

        if self._pool is None:
            return

        # Cancel idle timer
        if self._idle_timer is not None:
            self._idle_timer.cancel()
            self._idle_timer = None

        try:
            # Close the pool to prevent new tasks
            self._pool.close()

            # Wait for running tasks to complete with timeout
            shutdown_timeout = 30.0
            start_time = time.time()

            def wait_for_completion():
                if self._pool is not None:
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
                if self._pool is not None:
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
                if self._pool is not None:
                    self._pool.terminate()
                    self._pool.join()
            except Exception:
                pass
        finally:
            self._pool = None

    async def predict_batch(self, texts: list[str]) -> list[dict[str, Any]]:
        """Execute classification in a worker process.

        The pool is initialized on-demand if not already active.
        """
        from . import classification_worker  # Lazy import

        # Ensure pool is initialized
        pool = self._ensure_pool()

        # Cancel idle shutdown timer since we have active work
        self._cancel_idle_timer()

        # Increment active task counter
        with self._lock:
            self._active_tasks += 1
            self._last_task_time = time.time()

        try:
            # Use apply_async for non-blocking execution, then wrap the AsyncResult in asyncio
            async_result = pool.apply_async(classification_worker.predict_batch, (texts,))
            # Wait for the result in an executor to avoid blocking the event loop
            loop = asyncio.get_event_loop()
            result = await loop.run_in_executor(None, async_result.get)
            return result
        finally:
            # Decrement active task counter
            with self._lock:
                self._active_tasks -= 1
                self._last_task_time = time.time()

            # Schedule idle shutdown if no more active tasks
            self._schedule_idle_shutdown()

    def shutdown(self) -> None:
        """Shutdown the process pool, waiting for running tasks to complete.

        This ensures worker processes are properly cleaned up to prevent memory leaks.
        """
        import structlog

        logger = structlog.get_logger(__name__)
        logger.info("shutting down ClassificationRunner")

        with self._lock:
            self._shutting_down = True
            self._cancel_idle_timer()
            self._shutdown_pool_internal()

        logger.info("ClassificationRunner shutdown complete")

