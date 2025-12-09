"""Simple in-memory admin job queue with configurable concurrency."""

from __future__ import annotations

import asyncio
from dataclasses import dataclass, field, asdict
from datetime import datetime, timezone
from typing import Awaitable, Callable, Dict, Optional
from uuid import UUID, uuid4

import structlog

AdminJobFn = Callable[[], Awaitable[dict]]


@dataclass(slots=True)
class AdminJob:
    job_id: UUID
    kind: str
    status: str = "queued"  # queued|running|succeeded|failed
    created_at: datetime = field(default_factory=lambda: datetime.now(timezone.utc))
    started_at: Optional[datetime] = None
    finished_at: Optional[datetime] = None
    error: Optional[str] = None
    result: Optional[dict] = None
    _fn: AdminJobFn | None = None

    def to_dict(self) -> dict:
        data = asdict(self)
        data.pop("_fn", None)
        return data


class AdminJobManager:
    """Manage admin jobs with a bounded worker pool."""

    def __init__(self, concurrency: int = 10) -> None:
        self._log = structlog.get_logger(__name__)
        self._queue: asyncio.Queue[AdminJob] = asyncio.Queue()
        self._jobs: Dict[UUID, AdminJob] = {}
        self._concurrency = max(1, concurrency)
        self._workers_started = False
        self._worker_tasks: list[asyncio.Task] = []

    def _ensure_workers(self) -> None:
        if self._workers_started:
            return
        self._workers_started = True
        for _ in range(self._concurrency):
            task = asyncio.create_task(self._worker_loop())
            self._worker_tasks.append(task)

    async def _worker_loop(self) -> None:
        while True:
            job = await self._queue.get()
            job.started_at = datetime.now(timezone.utc)
            job.status = "running"
            try:
                if job._fn is None:
                    raise RuntimeError("job function missing")
                job.result = await job._fn()
                job.status = "succeeded"
            except Exception as exc:  # pragma: no cover - runtime safety
                self._log.error("admin_job.failed", job_id=str(job.job_id), kind=job.kind, error=str(exc))
                job.status = "failed"
                job.error = str(exc)
            finally:
                job.finished_at = datetime.now(timezone.utc)
                self._queue.task_done()

    def enqueue(self, kind: str, fn: AdminJobFn) -> UUID:
        job_id = uuid4()
        job = AdminJob(job_id=job_id, kind=kind, _fn=fn)
        self._jobs[job_id] = job
        self._queue.put_nowait(job)
        self._ensure_workers()
        return job_id

    def get(self, job_id: UUID) -> Optional[dict]:
        job = self._jobs.get(job_id)
        if not job:
            return None
        return job.to_dict()

    def list(self, limit: int = 50) -> list[dict]:
        # Return recent jobs sorted by created_at desc
        return [j.to_dict() for j in sorted(self._jobs.values(), key=lambda j: j.created_at, reverse=True)[:limit]]

