from __future__ import annotations

import asyncio
from datetime import UTC, datetime
from uuid import UUID, uuid4

import pytest

from recap_subworker.services.async_jobs import (
    AdminJobService,
    ConcurrentAdminJobError,
)


class FakeSession:
    def __init__(self):
        self.commits = 0
        self.rollbacks = 0

    async def commit(self):
        self.commits += 1

    async def rollback(self):
        self.rollbacks += 1

    async def __aenter__(self):
        return self

    async def __aexit__(self, *args, **kwargs):
        return False


class FakeDAO:
    def __init__(self, session):
        self.session = session
        self.jobs: dict[str, dict] = {}
        self.running_flag = False
        self.orphan_sweeps: list[str] = []

    async def has_running_admin_job(self, kind: str) -> bool:
        if self.running_flag:
            return True
        return any(job["kind"] == kind and job["status"] == "running" for job in self.jobs.values())

    async def insert_admin_job(self, job_id: UUID, kind: str, status: str, payload, started_at):
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
        finished_at=None,
        result=None,
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
        finished_at=None,
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


@pytest.fixture
def fake_settings():
    class S:
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

    return S()


@pytest.mark.asyncio
async def test_enqueue_graph_job_creates_job(monkeypatch, fake_settings):
    fake_session = FakeSession()
    fake_dao = FakeDAO(fake_session)

    async def fake_session_factory():
        return fake_session

    class _Wrapper:
        async def __aenter__(self):
            return fake_session

        async def __aexit__(self, *args, **kwargs):
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
        settings=fake_settings,
        session_factory=lambda: _Wrapper(),
        learning_client=None,  # not used in this test
    )

    job_id = await service.enqueue_graph_job()

    assert str(job_id) in fake_dao.jobs
    assert fake_dao.jobs[str(job_id)]["status"] == "running"
    assert fake_session.commits == 1


@pytest.mark.asyncio
async def test_enqueue_graph_job_rejects_when_running(monkeypatch, fake_settings):
    fake_session = FakeSession()
    fake_dao = FakeDAO(fake_session)
    fake_dao.running_flag = True

    class _Wrapper:
        async def __aenter__(self):
            return fake_session

        async def __aexit__(self, *args, **kwargs):
            return False

    monkeypatch.setattr(
        "recap_subworker.services.async_jobs.SubworkerDAO", lambda session: fake_dao
    )
    monkeypatch.setattr(
        "recap_subworker.services.async_jobs.AdminJobService._schedule", lambda *a, **k: None
    )

    service = AdminJobService(
        settings=fake_settings,
        session_factory=lambda: _Wrapper(),
        learning_client=None,
    )

    with pytest.raises(ConcurrentAdminJobError):
        await service.enqueue_graph_job()


@pytest.mark.asyncio
async def test_shutdown_marks_cancelled_graph_job_failed_and_allows_new_enqueue(
    monkeypatch, fake_settings
):
    fake_session = FakeSession()
    fake_dao = FakeDAO(fake_session)

    monkeypatch.setattr(
        "recap_subworker.services.async_jobs.SubworkerDAO", lambda session: fake_dao
    )

    service = AdminJobService(
        settings=fake_settings,
        session_factory=lambda: fake_session,
        learning_client=None,
    )

    started = asyncio.Event()
    never_set = asyncio.Event()

    async def _blocking_graph_build():
        started.set()
        await never_set.wait()
        return {}

    monkeypatch.setattr(service, "_run_graph_build", _blocking_graph_build)

    first_job_id = await service.enqueue_graph_job()
    await asyncio.wait_for(started.wait(), timeout=5.0)
    await service.shutdown()

    assert fake_dao.jobs[str(first_job_id)]["status"] == "failed"

    async def _fast_graph_build():
        return {"edge_counts": {}, "total_edges": 0}

    monkeypatch.setattr(service, "_run_graph_build", _fast_graph_build)
    second_job_id = await service.enqueue_graph_job()
    await service.shutdown()
    assert second_job_id != first_job_id


@pytest.mark.asyncio
async def test_shutdown_marks_cancelled_learning_job_failed_and_allows_new_enqueue(
    monkeypatch, fake_settings
):
    fake_session = FakeSession()
    fake_dao = FakeDAO(fake_session)

    monkeypatch.setattr(
        "recap_subworker.services.async_jobs.SubworkerDAO", lambda session: fake_dao
    )

    service = AdminJobService(
        settings=fake_settings,
        session_factory=lambda: fake_session,
        learning_client=None,
    )

    started = asyncio.Event()
    never_set = asyncio.Event()

    async def _blocking_learning():
        started.set()
        await never_set.wait()
        return {}

    monkeypatch.setattr(service, "_run_learning", _blocking_learning)

    first_job_id = await service.enqueue_learning_job()
    await asyncio.wait_for(started.wait(), timeout=5.0)
    await service.shutdown()

    assert fake_dao.jobs[str(first_job_id)]["status"] == "failed"

    async def _fast_learning():
        return {"recap_worker_status": 200}

    monkeypatch.setattr(service, "_run_learning", _fast_learning)
    second_job_id = await service.enqueue_learning_job()
    await service.shutdown()
    assert second_job_id != first_job_id


@pytest.mark.asyncio
async def test_enqueue_sweeps_orphaned_running_admin_jobs(monkeypatch, fake_settings):
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
        settings=fake_settings,
        session_factory=lambda: fake_session,
        learning_client=None,
    )

    new_job_id = await service.enqueue_graph_job()

    assert fake_dao.orphan_sweeps
    assert fake_dao.jobs[str(orphan_id)]["status"] == "failed"
    assert fake_dao.jobs[str(new_job_id)]["status"] == "running"


@pytest.mark.asyncio
async def test_startup_sweep_fails_orphaned_admin_jobs(monkeypatch):
    from recap_subworker.app.main import sweep_orphaned_runs

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
async def test_empty_exception_message_is_not_mislabeled_as_cancelled(monkeypatch, fake_settings):
    from recap_subworker.services.async_jobs import CANCELLED_ADMIN_JOB_MESSAGE

    fake_session = FakeSession()
    fake_dao = FakeDAO(fake_session)
    monkeypatch.setattr(
        "recap_subworker.services.async_jobs.SubworkerDAO", lambda session: fake_dao
    )

    service = AdminJobService(
        settings=fake_settings,
        session_factory=lambda: fake_session,
        learning_client=None,
    )

    async def _timeout_graph():
        raise TimeoutError()

    monkeypatch.setattr(service, "_run_graph_build", _timeout_graph)

    job_id = await service.enqueue_graph_job()
    await asyncio.gather(*list(service._tasks), return_exceptions=True)

    assert fake_dao.jobs[str(job_id)]["status"] == "failed"
    assert fake_dao.jobs[str(job_id)]["error"] != CANCELLED_ADMIN_JOB_MESSAGE

