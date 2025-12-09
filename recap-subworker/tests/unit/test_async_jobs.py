from __future__ import annotations

from uuid import UUID

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

    async def has_running_admin_job(self, kind: str) -> bool:
        return self.running_flag

    async def insert_admin_job(self, job_id: UUID, kind: str, status: str, payload, started_at):
        self.jobs[str(job_id)] = {
            "job_id": job_id,
            "kind": kind,
            "status": status,
            "payload": payload,
            "started_at": started_at,
        }
        return job_id


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
    monkeypatch.setattr("recap_subworker.services.async_jobs.SubworkerDAO", lambda session: fake_dao)
    monkeypatch.setattr("recap_subworker.services.async_jobs.TagLabelGraphBuilder", lambda *a, **k: None)
    monkeypatch.setattr("recap_subworker.services.async_jobs.AdminJobService._schedule", lambda *a, **k: None)

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

    monkeypatch.setattr("recap_subworker.services.async_jobs.SubworkerDAO", lambda session: fake_dao)
    monkeypatch.setattr("recap_subworker.services.async_jobs.AdminJobService._schedule", lambda *a, **k: None)

    service = AdminJobService(
        settings=fake_settings,
        session_factory=lambda: _Wrapper(),
        learning_client=None,
    )

    with pytest.raises(ConcurrentAdminJobError):
        await service.enqueue_graph_job()

