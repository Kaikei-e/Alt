from __future__ import annotations

import asyncio
from datetime import UTC, datetime
from typing import TYPE_CHECKING, Never, cast
from uuid import UUID, uuid4

import pytest

if TYPE_CHECKING:
    from sqlalchemy.ext.asyncio import AsyncSession, async_sessionmaker

from recap_subworker.app.main import sweep_orphaned_runs
from recap_subworker.services.admin_jobs import AdminJobFn, AdminJobManager
from recap_subworker.services.async_jobs import (
    CANCELLED_ADMIN_JOB_MESSAGE,
    AdminJobService,
    ConcurrentAdminJobError,
)


class FakeSession:
    def __init__(self) -> None:
        self.commits = 0
        self.rollbacks = 0

    async def commit(self) -> None:
        self.commits += 1

    async def rollback(self) -> None:
        self.rollbacks += 1

    async def __aenter__(self) -> FakeSession:
        return self

    async def __aexit__(
        self,
        *args: object,
        **kwargs: object,
    ) -> bool:
        return False


class FakeDAO:
    def __init__(self, session: FakeSession) -> None:
        self.session = session
        self.jobs: dict[str, dict] = {}
        self.running_flag = False
        self.orphan_sweeps: list[str] = []

    async def has_running_admin_job(self, kind: str) -> bool:
        if self.running_flag:
            return True
        return any(job["kind"] == kind and job["status"] == "running" for job in self.jobs.values())

    async def insert_admin_job(
        self,
        job_id: UUID,
        kind: str,
        status: str,
        payload: object,
        started_at: datetime,
    ) -> UUID:
        self.jobs[str(job_id)] = {
            "job_id": job_id,
            "kind": kind,
            "status": status,
            "payload": payload,
            "started_at": started_at,
            "finished_at": None,
            "result": None,
            "error": None,
        }
        return job_id

    async def update_admin_job_status(
        self,
        job_id: UUID,
        status: str,
        finished_at: datetime | None = None,
        result: object = None,
        error: str | None = None,
    ) -> None:
        job = self.jobs.get(str(job_id))
        if job is not None:
            job["status"] = status
            job["finished_at"] = finished_at
            job["result"] = result
            job["error"] = error

    async def fail_orphaned_admin_jobs(
        self,
        error_message: str,
        finished_at: datetime | None = None,
    ) -> int:
        self.orphan_sweeps.append(error_message)
        swept = 0
        for job in self.jobs.values():
            if job["status"] == "running":
                job["status"] = "failed"
                job["error"] = error_message
                job["finished_at"] = finished_at
                swept += 1
        return swept


class _FakeSettings:
    graph_build_max_concurrency = 1
    graph_build_windows = "7"
    graph_build_max_tags = 6
    graph_build_min_confidence = 0.3
    graph_build_min_support = 3
    graph_build_enabled = True
    learning_snapshot_days = 7
    learning_auto_detect_genres = True
    learning_cluster_genres = ""
    learning_graph_margin = 0.15
    learning_bayes_enabled = False
    learning_bayes_iterations = 30
    learning_bayes_seed = 42
    learning_bayes_min_samples = 100
    recap_worker_learning_url = "http://localhost:9005/admin/genre-learning"
    learning_request_timeout_seconds = 5.0


@pytest.fixture
def fake_settings() -> _FakeSettings:
    return _FakeSettings()


@pytest.mark.asyncio
async def test_enqueue_graph_job_creates_job(
    monkeypatch: pytest.MonkeyPatch, fake_settings: _FakeSettings
) -> None:
    fake_session = FakeSession()
    fake_dao = FakeDAO(fake_session)

    class _Wrapper:
        async def __aenter__(self) -> FakeSession:
            return fake_session

        async def __aexit__(
            self,
            *args: object,
            **kwargs: object,
        ) -> bool:
            return False

    # Patch DAO and schedule to avoid background execution
    monkeypatch.setattr(
        "recap_subworker.services.async_jobs.SubworkerDAO", lambda session: fake_dao
    )
    monkeypatch.setattr(
        "recap_subworker.services.async_jobs.TagLabelGraphBuilder", lambda *a, **k: None
    )
    monkeypatch.setattr(
        "recap_subworker.services.async_jobs.AdminJobService._schedule", lambda *a, **k: None
    )

    service = AdminJobService(
        settings=fake_settings,  # type: ignore[arg-type]
        session_factory=cast("async_sessionmaker[AsyncSession]", _Wrapper),
        learning_client=None,  # not used in this test
    )

    job_id = await service.enqueue_graph_job()

    assert str(job_id) in fake_dao.jobs
    assert fake_dao.jobs[str(job_id)]["status"] == "running"
    assert fake_session.commits == 1


@pytest.mark.asyncio
async def test_enqueue_graph_job_rejects_when_running(
    monkeypatch: pytest.MonkeyPatch, fake_settings: _FakeSettings
) -> None:
    fake_session = FakeSession()
    fake_dao = FakeDAO(fake_session)
    fake_dao.running_flag = True

    class _Wrapper:
        async def __aenter__(self) -> FakeSession:
            return fake_session

        async def __aexit__(
            self,
            *args: object,
            **kwargs: object,
        ) -> bool:
            return False

    monkeypatch.setattr(
        "recap_subworker.services.async_jobs.SubworkerDAO", lambda session: fake_dao
    )
    monkeypatch.setattr(
        "recap_subworker.services.async_jobs.AdminJobService._schedule", lambda *a, **k: None
    )

    service = AdminJobService(
        settings=fake_settings,  # type: ignore[arg-type]
        session_factory=cast("async_sessionmaker[AsyncSession]", _Wrapper),
        learning_client=None,
    )

    with pytest.raises(ConcurrentAdminJobError):
        await service.enqueue_graph_job()


@pytest.mark.asyncio
async def test_shutdown_marks_cancelled_graph_job_failed_and_allows_new_enqueue(
    monkeypatch: pytest.MonkeyPatch, fake_settings: _FakeSettings
) -> None:
    fake_session = FakeSession()
    fake_dao = FakeDAO(fake_session)

    monkeypatch.setattr(
        "recap_subworker.services.async_jobs.SubworkerDAO", lambda session: fake_dao
    )

    service = AdminJobService(
        settings=fake_settings,  # type: ignore[arg-type]
        session_factory=cast("async_sessionmaker[AsyncSession]", lambda: fake_session),
        learning_client=None,
    )

    started = asyncio.Event()
    never_set = asyncio.Event()

    async def _blocking_graph_build() -> dict[str, object]:
        started.set()
        await never_set.wait()
        return {}

    monkeypatch.setattr(service, "_run_graph_build", _blocking_graph_build)

    first_job_id = await service.enqueue_graph_job()
    await asyncio.wait_for(started.wait(), timeout=5.0)
    await service.shutdown()

    assert fake_dao.jobs[str(first_job_id)]["status"] == "failed"

    async def _fast_graph_build() -> dict[str, object]:
        return {"edge_counts": {}, "total_edges": 0}

    monkeypatch.setattr(service, "_run_graph_build", _fast_graph_build)
    second_job_id = await service.enqueue_graph_job()
    await service.shutdown()
    assert second_job_id != first_job_id


@pytest.mark.asyncio
async def test_shutdown_marks_cancelled_learning_job_failed_and_allows_new_enqueue(
    monkeypatch: pytest.MonkeyPatch, fake_settings: _FakeSettings
) -> None:
    fake_session = FakeSession()
    fake_dao = FakeDAO(fake_session)

    monkeypatch.setattr(
        "recap_subworker.services.async_jobs.SubworkerDAO", lambda session: fake_dao
    )

    service = AdminJobService(
        settings=fake_settings,  # type: ignore[arg-type]
        session_factory=cast("async_sessionmaker[AsyncSession]", lambda: fake_session),
        learning_client=None,
    )

    started = asyncio.Event()
    never_set = asyncio.Event()

    async def _blocking_learning() -> dict[str, object]:
        started.set()
        await never_set.wait()
        return {}

    monkeypatch.setattr(service, "_run_learning", _blocking_learning)

    first_job_id = await service.enqueue_learning_job()
    await asyncio.wait_for(started.wait(), timeout=5.0)
    await service.shutdown()

    assert fake_dao.jobs[str(first_job_id)]["status"] == "failed"

    async def _fast_learning() -> dict[str, object]:
        return {"recap_worker_status": 200}

    monkeypatch.setattr(service, "_run_learning", _fast_learning)
    second_job_id = await service.enqueue_learning_job()
    await service.shutdown()
    assert second_job_id != first_job_id


@pytest.mark.asyncio
async def test_enqueue_sweeps_orphaned_running_admin_jobs(
    monkeypatch: pytest.MonkeyPatch, fake_settings: _FakeSettings
) -> None:
    fake_session = FakeSession()
    fake_dao = FakeDAO(fake_session)
    orphan_id = uuid4()
    await fake_dao.insert_admin_job(
        job_id=orphan_id,
        kind="graph",
        status="running",
        payload=None,
        started_at=datetime.now(UTC),
    )

    monkeypatch.setattr(
        "recap_subworker.services.async_jobs.SubworkerDAO", lambda session: fake_dao
    )
    monkeypatch.setattr(
        "recap_subworker.services.async_jobs.AdminJobService._schedule", lambda *a, **k: None
    )

    service = AdminJobService(
        settings=fake_settings,  # type: ignore[arg-type]
        session_factory=cast("async_sessionmaker[AsyncSession]", lambda: fake_session),
        learning_client=None,
    )

    new_job_id = await service.enqueue_graph_job()

    assert fake_dao.orphan_sweeps
    assert fake_dao.jobs[str(orphan_id)]["status"] == "failed"
    assert fake_dao.jobs[str(new_job_id)]["status"] == "running"


@pytest.mark.asyncio
async def test_startup_sweep_fails_orphaned_admin_jobs(monkeypatch: pytest.MonkeyPatch) -> None:
    fake_session = FakeSession()
    fake_dao = FakeDAO(fake_session)
    orphan_id = uuid4()
    await fake_dao.insert_admin_job(
        job_id=orphan_id,
        kind="learning",
        status="running",
        payload=None,
        started_at=datetime.now(UTC),
    )

    async def _fail_orphaned_runs(_msg: str) -> int:
        return 0

    fake_dao.fail_orphaned_runs = _fail_orphaned_runs  # type: ignore[attr-defined]
    monkeypatch.setattr("recap_subworker.app.main.SubworkerDAO", lambda session: fake_dao)

    await sweep_orphaned_runs(lambda: fake_session)

    assert fake_dao.orphan_sweeps
    assert fake_dao.jobs[str(orphan_id)]["status"] == "failed"
    assert fake_dao.jobs[str(orphan_id)]["finished_at"] is not None


@pytest.mark.asyncio
async def test_empty_exception_message_is_not_mislabeled_as_cancelled(
    monkeypatch: pytest.MonkeyPatch, fake_settings: _FakeSettings
) -> None:
    fake_session = FakeSession()
    fake_dao = FakeDAO(fake_session)
    monkeypatch.setattr(
        "recap_subworker.services.async_jobs.SubworkerDAO", lambda session: fake_dao
    )

    service = AdminJobService(
        settings=fake_settings,  # type: ignore[arg-type]
        session_factory=cast("async_sessionmaker[AsyncSession]", lambda: fake_session),
        learning_client=None,
    )

    async def _timeout_graph() -> Never:
        raise TimeoutError()

    monkeypatch.setattr(service, "_run_graph_build", _timeout_graph)

    job_id = await service.enqueue_graph_job()
    await asyncio.gather(*list(service._tasks), return_exceptions=True)

    assert fake_dao.jobs[str(job_id)]["status"] == "failed"
    assert fake_dao.jobs[str(job_id)]["error"] != CANCELLED_ADMIN_JOB_MESSAGE


@pytest.mark.asyncio
async def test_admin_job_manager_keeps_worker_alive_when_fn_missing() -> None:
    manager = AdminJobManager(concurrency=1)

    async def _succeed() -> dict[str, str]:
        return {"ok": "true"}

    missing_job_id = manager.enqueue("missing_fn", cast("AdminJobFn", None))
    next_job_id = manager.enqueue("valid_fn", _succeed)

    try:
        await asyncio.wait_for(manager._queue.join(), timeout=0.5)
    finally:
        for task in manager._worker_tasks:
            task.cancel()
        await asyncio.gather(*manager._worker_tasks, return_exceptions=True)

    missing_job = manager.get(missing_job_id)
    next_job = manager.get(next_job_id)
    assert missing_job is not None
    assert missing_job["status"] == "failed"
    assert missing_job["error"] == "job function missing"
    assert next_job is not None
    assert next_job["status"] == "succeeded"
    assert next_job["result"] == {"ok": "true"}
