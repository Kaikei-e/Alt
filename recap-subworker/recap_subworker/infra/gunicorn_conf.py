"""Gunicorn configuration tuned for recap-subworker."""

from __future__ import annotations

import asyncio
import multiprocessing
import threading

import structlog

from recap_subworker.infra.config import get_settings
from recap_subworker.services.learning_scheduler import LearningScheduler
from recap_subworker.services.learning_scheduler import LearningScheduler

_settings = get_settings()
logger = structlog.get_logger(__name__)

# Global scheduler instance for master process
_scheduler: LearningScheduler | None = None
_scheduler_thread: threading.Thread | None = None


def _worker_count() -> int:
    if _settings.gunicorn_workers:
        return _settings.gunicorn_workers

    # If using process pool, we manage concurrency internally
    # So we should default to 1 gunicorn worker to avoid multiplicative process explosion
    if _settings.pipeline_mode == "processpool":
        return 1

    return max(2, multiprocessing.cpu_count() * 2 + 1)


def _run_scheduler_loop(scheduler: LearningScheduler, loop: asyncio.AbstractEventLoop) -> None:
    """Run the scheduler in a separate event loop (for master process)."""
    asyncio.set_event_loop(loop)
    try:
        loop.run_until_complete(scheduler.start())
        # Keep the loop running until scheduler stops
        # Monitor the scheduler's _running flag and stop the loop when it becomes False
        async def monitor_scheduler():
            while scheduler._running:
                await asyncio.sleep(0.5)
            loop.stop()

        loop.create_task(monitor_scheduler())
        try:
            loop.run_forever()
        except KeyboardInterrupt:
            pass
    except Exception as exc:
        logger.error(
            "scheduler loop failed",
            error=str(exc),
            error_type=type(exc).__name__,
            exc_info=True,
        )
    finally:
        try:
            if scheduler._running:
                loop.run_until_complete(scheduler.stop())
        except Exception as exc:
            logger.warning(
                "error stopping scheduler in cleanup",
                error=str(exc),
                error_type=type(exc).__name__,
            )
        finally:
            loop.close()


import logging

class NoisyPathFilter(logging.Filter):
    def filter(self, record):
        return "/v1/extract" not in record.getMessage()

def on_starting(server) -> None:
    """Called just before the master process is initialized."""
    # Add filter to Gunicorn access logger
    gunicorn_logger = logging.getLogger("gunicorn.access")
    gunicorn_logger.addFilter(NoisyPathFilter())
    # Add filter to Uvicorn access logger (used by UvicornWorker)
    uvicorn_logger = logging.getLogger("uvicorn.access")
    uvicorn_logger.addFilter(NoisyPathFilter())

    global _scheduler, _scheduler_thread

    if not _settings.learning_scheduler_enabled:
        logger.info("learning scheduler disabled, skipping")
        return

    logger.info("initializing learning scheduler in master process")
    _scheduler = LearningScheduler(
        _settings,
        interval_hours=_settings.learning_scheduler_interval_hours,
    )

    # Create event loop for the scheduler thread
    loop = asyncio.new_event_loop()

    # Start scheduler in a separate thread with its own event loop
    _scheduler_thread = threading.Thread(
        target=_run_scheduler_loop,
        args=(_scheduler, loop),
        daemon=True,
        name="learning-scheduler",
    )
    _scheduler_thread.start()
    logger.info("learning scheduler started in master process")


def on_exit(server) -> None:
    """Called just before exiting Gunicorn."""
    global _scheduler, _scheduler_thread

    if _scheduler is not None:
        logger.info("stopping learning scheduler in master process")
        # Stop the scheduler by setting _running to False
        # This will cause the monitor task to stop the event loop
        _scheduler._running = False

    if _scheduler_thread is not None:
        _scheduler_thread.join(timeout=5.0)
        if _scheduler_thread.is_alive():
            logger.warning("scheduler thread did not stop within timeout")


    if _scheduler_thread is not None:
        _scheduler_thread.join(timeout=5.0)
        if _scheduler_thread.is_alive():
            logger.warning("scheduler thread did not stop within timeout")


bind = f"{_settings.http_host}:{_settings.http_port}"
worker_class = "uvicorn.workers.UvicornWorker"
workers = _worker_count()
threads = 2  # Allow concurrent I/O within each worker
preload_app = True  # Share ML model memory across workers via Copy-on-Write
max_requests = _settings.gunicorn_max_requests
max_requests_jitter = _settings.gunicorn_max_requests_jitter
timeout = _settings.gunicorn_worker_timeout
graceful_timeout = _settings.gunicorn_graceful_timeout
accesslog = "-"
errorlog = "-"
