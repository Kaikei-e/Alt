"""Gunicorn configuration tuned for recap-subworker."""

from __future__ import annotations

import multiprocessing

from recap_subworker.infra.config import get_settings


_settings = get_settings()


def _worker_count() -> int:
    if _settings.gunicorn_workers:
        return _settings.gunicorn_workers
    return max(2, multiprocessing.cpu_count() * 2 + 1)


bind = f"{_settings.http_host}:{_settings.http_port}"
worker_class = "uvicorn.workers.UvicornWorker"
workers = _worker_count()
max_requests = _settings.gunicorn_max_requests
max_requests_jitter = _settings.gunicorn_max_requests_jitter
timeout = _settings.gunicorn_worker_timeout
graceful_timeout = _settings.gunicorn_graceful_timeout
accesslog = "-"
errorlog = "-"
