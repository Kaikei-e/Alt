"""Gunicorn configuration tuned for recap-subworker."""

from __future__ import annotations

import asyncio
import multiprocessing
import threading

import structlog

from recap_subworker.infra.config import get_settings

_settings = get_settings()
logger = structlog.get_logger(__name__)

def _worker_count() -> int:
    if _settings.gunicorn_workers:
        return _settings.gunicorn_workers

    # If using CUDA, use 1 worker to avoid CUDA re-initialization errors in forked processes
    # Classification parallelism is handled by ClassificationRunner with spawn-based process pool
    if _settings.device.startswith("cuda"):
        return 1

    # If using process pool, we manage concurrency internally
    # So we should default to 1 gunicorn worker to avoid multiplicative process explosion
    if _settings.pipeline_mode == "processpool":
        return 1

    return max(2, multiprocessing.cpu_count() * 2 + 1)

# Global scheduler process for master process
_scheduler_process: multiprocessing.Process | None = None


def _run_scheduler_process() -> None:
    """Run the scheduler in a separate process to avoid polluting the master process with imports.

    This function (and the imported modules) will only be loaded in the spawned process.
    """
    import asyncio
    import structlog
    from recap_subworker.infra.config import get_settings
    from recap_subworker.services.learning_scheduler import LearningScheduler

    # Re-configure logging for the child process if necessary
    # (structlog configuration is usually preserved or re-run via side-effects of imports if configured at module level)

    logger = structlog.get_logger(__name__)
    settings = get_settings()

    logger.info("initializing learning scheduler in dedicated process")

    try:
        scheduler = LearningScheduler(
            settings,
            interval_hours=settings.learning_scheduler_interval_hours,
        )

        loop = asyncio.new_event_loop()
        asyncio.set_event_loop(loop)

        try:
            loop.run_until_complete(scheduler.start())

            # Monitor loop to keep process alive until scheduler stops
            async def monitor_scheduler():
                while scheduler._running:
                    await asyncio.sleep(1.0)
                loop.stop()

            loop.run_until_complete(monitor_scheduler())
        except KeyboardInterrupt:
            logger.info("scheduler process received interrupt")
        except Exception as exc:
            logger.error(
                "scheduler loop failed",
                error=str(exc),
                error_type=type(exc).__name__,
                exc_info=True,
            )
        finally:
            # Cleanup
            if scheduler._running:
                try:
                    loop.run_until_complete(scheduler.stop())
                except Exception as exc:
                    logger.warning("error stopping scheduler", error=str(exc))
            loop.close()

    except Exception as exc:
        logger.critical("failed to start scheduler process", error=str(exc), exc_info=True)
        import sys
        sys.exit(1)


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

    global _scheduler_process

    if not _settings.learning_scheduler_enabled:
        logger.info("learning scheduler disabled, skipping")
        return

    logger.info("spawning learning scheduler process")

    # Use 'spawn' context to ensure a clean process with no inherited state
    # This is critical to avoid "Cannot re-initialize CUDA" errors in workers
    ctx = multiprocessing.get_context("spawn")
    _scheduler_process = ctx.Process(
        target=_run_scheduler_process,
        daemon=True,
        name="learning-scheduler-proc",
    )
    _scheduler_process.start()
    logger.info("learning scheduler process started", pid=_scheduler_process.pid)


def on_exit(server) -> None:
    """Called just before exiting Gunicorn."""
    global _scheduler_process

    if _scheduler_process is not None and _scheduler_process.is_alive():
        logger.info("stopping learning scheduler process")
        _scheduler_process.terminate()
        _scheduler_process.join(timeout=5.0)
        if _scheduler_process.is_alive():
            logger.warning("scheduler process did not stop within timeout, killing")
            _scheduler_process.kill()


bind = f"{_settings.http_host}:{_settings.http_port}"
worker_class = "uvicorn.workers.UvicornWorker"
workers = _worker_count()
threads = 2  # Allow concurrent I/O within each worker
preload_app = False  # Disabled to avoid shared state issues with SQLAlchemy and multiprocessing
max_requests = _settings.gunicorn_max_requests
max_requests_jitter = _settings.gunicorn_max_requests_jitter
timeout = _settings.gunicorn_worker_timeout
graceful_timeout = _settings.gunicorn_graceful_timeout
accesslog = "-"
errorlog = "-"
