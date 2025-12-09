"""Unit tests for RunManager."""

from __future__ import annotations

import asyncio
from uuid import uuid4

import pytest
from recap_subworker.domain.models import (
    ClusterDocument,
    ClusterJobParams,
    ClusterJobPayload,
    ClusterLabel,
    ClusterStats,
    Diagnostics,
    EvidenceBudget,
    EvidenceCluster,
    EvidenceRequest,
    EvidenceResponse,
    RepresentativeSentence,
    RepresentativeSource,
)
from recap_subworker.infra.config import Settings
from recap_subworker.services.run_manager import (
    ConcurrentRunError,
    IdempotencyMismatchError,
    RunManager,
    RunSubmission,
)
from recap_subworker.db.dao import RunRecord


class DummySessionContext:
    def __init__(self, session):
        self.session = session

    async def __aenter__(self):
        return self.session

    async def __aexit__(self, exc_type, exc, tb):
        return False


class FakeSession:
    def __init__(self):
        self.commit_called = False
        self.rollback_called = False

    async def commit(self):
        self.commit_called = True

    async def rollback(self):
        self.rollback_called = True


class FakeDAO:
    def __init__(self, session):
        self.session = session
        self.inserted = []
        self.idempotent_record: RunRecord | None = None
        self.running = False
        self.fetched_record: RunRecord | None = None
        self.persisted_clusters = None
        self.diagnostic_entries = None
        self.success_status = None
        self.failure_status = None

    async def find_run_by_idempotency(self, *args, **kwargs):
        return self.idempotent_record

    async def has_running_run(self, *args, **kwargs):
        return self.running

    async def insert_run(self, run):
        self.inserted.append(run)
        return 7

    async def fetch_run(self, run_id):
        return self.fetched_record

    async def insert_clusters(self, run_id, clusters):
        self.persisted_clusters = (run_id, clusters)

    async def upsert_diagnostics(self, run_id, entries):
        self.diagnostic_entries = (run_id, list(entries))

    async def insert_system_metrics(self, metric_type, metrics, job_id=None):
        # No-op for tests; just store last call
        self.system_metrics = (metric_type, metrics, job_id)

    async def mark_run_success(self, run_id, cluster_count, payload, status="succeeded"):
        self.success_status = (run_id, cluster_count, status)

    async def mark_run_failure(self, run_id, status, error_message):
        self.failure_status = (run_id, status, error_message)


class PipelineRunnerStub:
    def __init__(self, response, delay: float = 0.0):
        self.response = response
        self.delay = delay
        self.called_with: list[EvidenceRequest] = []

    async def run(self, request):
        self.called_with.append(request)
        if self.delay:
            await asyncio.sleep(self.delay)
        return self.response

    async def warmup(self):  # pragma: no cover - not used in tests
        return None


@pytest.fixture
def payload() -> ClusterJobPayload:
    params = ClusterJobParams(max_sentences_total=2000, umap_n_components=25, hdbscan_min_cluster_size=5, mmr_lambda=0.35)
    docs = [
        ClusterDocument(article_id=f"art-{i}", title="T", paragraphs=["x" * 35])
        for i in range(10)
    ]
    return ClusterJobPayload(params=params, documents=docs)


def make_manager(
    dao: FakeDAO,
    session: FakeSession,
    *,
    settings: Settings | None = None,
    pipeline=None,
    pipeline_runner=None,
) -> RunManager:
    def session_factory():
        return DummySessionContext(session)

    def dao_factory(session):
        return dao

    return RunManager(settings or Settings(), session_factory, dao_factory, pipeline=pipeline, pipeline_runner=pipeline_runner)


@pytest.mark.asyncio
async def test_create_run_inserts_and_schedules(payload):
    session = FakeSession()
    dao = FakeDAO(session)
    manager = make_manager(dao, session)
    scheduled = {}

    def recorder(record):
        scheduled["run_id"] = record.run_id

    manager._schedule_background = recorder  # type: ignore[attr-defined]
    submission = RunSubmission(job_id=uuid4(), genre="ai", payload=payload, idempotency_key=None)

    record = await manager.create_run(submission)

    assert record.run_id == 7
    assert session.commit_called
    assert scheduled["run_id"] == 7
    assert dao.inserted


@pytest.mark.asyncio
async def test_create_run_reuses_existing_idempotent(payload):
    session = FakeSession()
    dao = FakeDAO(session)
    existing = RunRecord(
        run_id=3,
        job_id=uuid4(),
        genre="ai",
        status="running",
        cluster_count=0,
        request_payload={"request_hash": None},
        response_payload=None,
        error_message=None,
    )
    dao.idempotent_record = existing
    manager = make_manager(dao, session)
    manager._schedule_background = lambda record: (_ for _ in ()).throw(AssertionError())  # type: ignore[attr-defined]
    submission = RunSubmission(job_id=existing.job_id, genre="ai", payload=payload, idempotency_key="k")

    record = await manager.create_run(submission)

    assert record.run_id == existing.run_id
    assert session.rollback_called
    assert not dao.inserted


@pytest.mark.asyncio
async def test_create_run_detects_idempotency_mismatch(payload):
    session = FakeSession()
    dao = FakeDAO(session)
    dao.idempotent_record = RunRecord(
        run_id=1,
        job_id=uuid4(),
        genre="ai",
        status="running",
        cluster_count=0,
        request_payload={"request_hash": "different"},
        response_payload=None,
        error_message=None,
    )
    manager = make_manager(dao, session)
    submission = RunSubmission(job_id=dao.idempotent_record.job_id, genre="ai", payload=payload, idempotency_key="k")

    with pytest.raises(IdempotencyMismatchError):
        await manager.create_run(submission)


@pytest.mark.asyncio
async def test_create_run_blocks_when_running(payload):
    session = FakeSession()
    dao = FakeDAO(session)
    dao.running = True
    manager = make_manager(dao, session)
    submission = RunSubmission(job_id=uuid4(), genre="ai", payload=payload, idempotency_key=None)

    with pytest.raises(ConcurrentRunError):
        await manager.create_run(submission)


@pytest.mark.asyncio
async def test_get_run_uses_dao(payload):
    session = FakeSession()
    dao = FakeDAO(session)
    record = RunRecord(
        run_id=5,
        job_id=uuid4(),
        genre="ai",
        status="running",
        cluster_count=0,
        request_payload={},
        response_payload=None,
        error_message=None,
    )
    dao.fetched_record = record
    manager = make_manager(dao, session)

    fetched = await manager.get_run(5)

    assert fetched == record
    assert session.rollback_called


@pytest.mark.asyncio
async def test_process_run_persists_clusters(payload):
    session = FakeSession()
    dao = FakeDAO(session)
    job_id = uuid4()
    record = RunRecord(
        run_id=1,
        job_id=job_id,
        genre="ai",
        status="running",
        cluster_count=0,
        request_payload={"payload": payload.model_dump(mode="json")},
        response_payload=None,
        error_message=None,
    )
    dao.fetched_record = record

    pipeline_response = EvidenceResponse(
        job_id="job",
        genre="ai",
        clusters=[
            EvidenceCluster(
                cluster_id=0,
                size=1,
                label=ClusterLabel(top_terms=["ai"]),
                representatives=[
                    RepresentativeSentence(
                        text="Representative sentence long enough for testing.",
                        lang="ja",
                        embedding_ref="e/0/0",
                        reasons=["centrality"],
                        source=RepresentativeSource(source_id="art-0", url=None, paragraph_idx=0),
                    )
                ],
                supporting_ids=["art-0"],
                stats=ClusterStats(avg_sim=0.9, token_count=10),
            )
        ],
        evidence_budget=EvidenceBudget(sentences=1, tokens_estimated=10),
        diagnostics=Diagnostics(dedup_pairs=1),
    )

    class PipelineStub:
        def __init__(self, response):
            self.response = response

        def run(self, request):
            return self.response

    manager = RunManager(
        Settings(),
        lambda: DummySessionContext(session),
        lambda _session: dao,
        pipeline=PipelineStub(pipeline_response),
    )

    await manager._process_run(1)

    assert dao.persisted_clusters is not None
    assert dao.success_status == (1, 1, "succeeded")
    assert dao.diagnostic_entries is not None


@pytest.mark.asyncio
async def test_process_run_uses_pipeline_runner(payload):
    session = FakeSession()
    dao = FakeDAO(session)
    job_id = uuid4()
    record = RunRecord(
        run_id=9,
        job_id=job_id,
        genre="tech",
        status="running",
        cluster_count=0,
        request_payload={"payload": payload.model_dump(mode="json")},
        response_payload=None,
        error_message=None,
    )
    dao.fetched_record = record

    pipeline_response = EvidenceResponse(
        job_id="job",
        genre="tech",
        clusters=[],
        evidence_budget=EvidenceBudget(sentences=0, tokens_estimated=0),
        diagnostics=Diagnostics(),
    )
    runner = PipelineRunnerStub(pipeline_response)
    settings = Settings(max_background_runs=1, pipeline_mode="processpool")
    manager = make_manager(dao, session, settings=settings, pipeline_runner=runner)

    await manager._process_run(9)

    assert runner.called_with
    assert dao.success_status == (9, 0, "succeeded")


@pytest.mark.asyncio
async def test_process_run_times_out(payload):
    session = FakeSession()
    dao = FakeDAO(session)
    job_id = uuid4()
    record = RunRecord(
        run_id=11,
        job_id=job_id,
        genre="ai",
        status="running",
        cluster_count=0,
        request_payload={"payload": payload.model_dump(mode="json")},
        response_payload=None,
        error_message=None,
    )
    dao.fetched_record = record

    pipeline_response = EvidenceResponse(
        job_id="job",
        genre="ai",
        clusters=[],
        evidence_budget=EvidenceBudget(sentences=0, tokens_estimated=0),
        diagnostics=Diagnostics(),
    )
    runner = PipelineRunnerStub(pipeline_response, delay=0.05)
    settings = Settings(max_background_runs=1, pipeline_mode="processpool")
    manager = make_manager(dao, session, settings=settings, pipeline_runner=runner)
    manager._run_timeout = 0.01

    await manager._process_run(11)

    assert dao.failure_status is not None
    assert dao.failure_status[0] == 11
    assert "timed out" in dao.failure_status[2]
