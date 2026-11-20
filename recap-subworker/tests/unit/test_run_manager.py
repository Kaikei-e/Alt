"""Unit tests for RunManager."""

from __future__ import annotations

import asyncio
import contextlib
from collections.abc import Iterable, Sequence
from dataclasses import replace
from types import TracebackType
from typing import TYPE_CHECKING, Never, cast
from unittest.mock import MagicMock
from uuid import UUID, uuid4

import pytest
from sqlalchemy.exc import InterfaceError
from sqlalchemy.sql import ClauseElement, Executable
from structlog.testing import capture_logs

if TYPE_CHECKING:
    from recap_subworker.services.pipeline import EvidencePipeline
    from recap_subworker.services.pipeline_runner import PipelineTaskRunner

from recap_subworker.db.dao import RunRecord, SubworkerDAO
from recap_subworker.domain.models import (
    ClassificationJobPayload,
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
    ClassificationRunSubmission,
    ConcurrentRunError,
    IdempotencyMismatchError,
    RunManager,
    RunSubmission,
    _hash_payload,
)

INSERTED_RUN_ID = 7
TIMED_OUT_RUN_ID = 11
MIN_REPRESENTATIVE_CHARS = 20
EXPECTED_SESSION_COUNT = 3


class _UnexpectedCallError(AssertionError):
    """Raised when a test stub is unexpectedly invoked or not cancelled."""

    @classmethod
    def predict_batch_unexpected(cls) -> _UnexpectedCallError:
        return cls("predict_batch should not be called in this test")

    @classmethod
    def predict_batch_not_cancelled(cls) -> _UnexpectedCallError:
        return cls("predict_batch must be cancelled, not completed")

    @classmethod
    def run_not_cancelled(cls) -> _UnexpectedCallError:
        return cls("run must be cancelled, not completed")


class DummySessionContext:
    def __init__(self, session: FakeSession | None) -> None:
        self.session = session

    async def __aenter__(self) -> FakeSession | None:
        return self.session

    async def __aexit__(
        self,
        exc_type: type[BaseException] | None,
        exc: BaseException | None,
        tb: TracebackType | None,
    ) -> bool:
        return False


class FakeSession:
    def __init__(self) -> None:
        self.commit_called = False
        self.rollback_called = False

    async def commit(self) -> None:
        self.commit_called = True

    async def rollback(self) -> None:
        self.rollback_called = True


class FakeDAO:
    def __init__(self, session: FakeSession | None) -> None:
        self.session = session
        self.inserted: list[object] = []
        self.idempotent_record: RunRecord | None = None
        self.running = False
        self.fetched_record: RunRecord | None = None
        self.persisted_clusters: tuple[int, list[object]] | None = None
        self.diagnostic_entries: tuple[int, list[object]] | None = None
        self.success_status: tuple[int, int, str] | None = None
        self.failure_status: tuple[int, str, str] | None = None
        self.orphan_sweeps: list[str] = []

    async def find_run_by_idempotency(
        self,
        *args: object,
        **kwargs: object,
    ) -> RunRecord | None:
        return self.idempotent_record

    async def fail_orphaned_runs(self, error_message: str) -> int:
        self.orphan_sweeps.append(error_message)
        if self.idempotent_record is None or self.idempotent_record.status != "running":
            return 0
        self.idempotent_record = replace(
            self.idempotent_record, status="failed", error_message=error_message
        )
        return 1

    async def has_running_run(
        self,
        *args: object,
        **kwargs: object,
    ) -> bool:
        return self.running

    async def insert_run(
        self,
        run: RunRecord | RunSubmission,
    ) -> int:
        self.inserted.append(run)
        return INSERTED_RUN_ID

    async def fetch_run(self, run_id: int) -> RunRecord | None:
        return self.fetched_record

    async def insert_clusters(self, run_id: int, clusters: Sequence[object]) -> None:
        self.persisted_clusters = (run_id, list(clusters))

    async def upsert_diagnostics(self, run_id: int, entries: Iterable[object]) -> None:
        self.diagnostic_entries = (run_id, list(entries))

    async def insert_system_metrics(
        self, metric_type: str, metrics: dict[str, object], job_id: UUID | None = None
    ) -> None:
        # No-op for tests; just store last call
        self.system_metrics = (metric_type, metrics, job_id)

    async def mark_run_success(
        self, run_id: int, cluster_count: int, payload: object, status: str = "succeeded"
    ) -> None:
        self.success_status = (run_id, cluster_count, status)

    async def mark_run_failure(self, run_id: int, status: str, error_message: str) -> None:
        self.failure_status = (run_id, status, error_message)


class PipelineRunnerStub:
    def __init__(self, response: EvidenceResponse, delay: float = 0.0) -> None:
        self.response = response
        self.delay = delay
        self.called_with: list[EvidenceRequest] = []

    async def run(self, request: EvidenceRequest) -> EvidenceResponse:
        self.called_with.append(request)
        if self.delay:
            await asyncio.sleep(self.delay)
        return self.response

    async def warmup(self) -> None:  # pragma: no cover - not used in tests
        return None


class ClassificationRunnerStub:
    """Minimal ClassificationRunnerPort stand-in for tests that only need a
    non-None sentinel (e.g. idempotency short-circuit paths that never
    actually invoke predict_batch)."""

    async def predict_batch(
        self, texts: list[str]
    ) -> Never:  # pragma: no cover - not expected to run
        raise _UnexpectedCallError.predict_batch_unexpected()


@pytest.fixture
def payload() -> ClusterJobPayload:
    params = ClusterJobParams(
        max_sentences_total=2000, umap_n_components=25, hdbscan_min_cluster_size=5, mmr_lambda=0.35
    )
    docs = [
        ClusterDocument(article_id=f"art-{i}", title="T", paragraphs=["x" * 35]) for i in range(10)
    ]
    return ClusterJobPayload(params=params, documents=docs)


def make_manager(
    dao: FakeDAO,
    session: FakeSession,
    *,
    settings: Settings | None = None,
    pipeline: EvidencePipeline | None = None,
    pipeline_runner: PipelineTaskRunner | None = None,
) -> RunManager:
    def session_factory() -> DummySessionContext:
        return DummySessionContext(session)

    def dao_factory(session: object) -> FakeDAO:
        return dao

    return RunManager(
        settings or Settings(),
        session_factory,
        dao_factory,  # pyrefly: ignore[bad-argument-type]
        pipeline=pipeline,
        pipeline_runner=pipeline_runner,
    )


@pytest.mark.asyncio
async def test_create_run_inserts_and_schedules(payload: ClusterJobPayload) -> None:
    session = FakeSession()
    dao = FakeDAO(session)
    manager = make_manager(dao, session)
    scheduled = {}

    def recorder(record: RunRecord) -> None:
        scheduled["run_id"] = record.run_id

    manager._schedule_background = recorder  # type: ignore[attr-defined]
    submission = RunSubmission(job_id=uuid4(), genre="ai", payload=payload, idempotency_key=None)

    record = await manager.create_run(submission)

    assert record.run_id == INSERTED_RUN_ID
    assert session.commit_called
    assert scheduled["run_id"] == INSERTED_RUN_ID
    assert dao.inserted


@pytest.mark.asyncio
async def test_create_run_reuses_existing_idempotent(payload: ClusterJobPayload) -> None:
    session = FakeSession()
    dao = FakeDAO(session)
    job_id = uuid4()
    manager = make_manager(dao, session)
    manager._schedule_background = lambda record: None  # type: ignore[attr-defined]
    submission = RunSubmission(job_id=job_id, genre="ai", payload=payload, idempotency_key="k")
    # The first submission runs the orphan sweep, after which a 'running'
    # row can only belong to a run this process still owns.
    await manager.create_run(submission)
    existing = RunRecord(
        run_id=7,
        job_id=job_id,
        genre="ai",
        status="running",
        cluster_count=0,
        request_payload={"request_hash": None},
        response_payload=None,
        error_message=None,
    )
    dao.idempotent_record = existing
    dao.inserted.clear()
    manager._schedule_background = lambda record: (_ for _ in ()).throw(AssertionError())  # type: ignore[attr-defined]

    record = await manager.create_run(submission)

    assert record.run_id == existing.run_id
    assert session.rollback_called
    assert not dao.inserted


@pytest.mark.asyncio
async def test_create_run_retries_failed_idempotent(payload: ClusterJobPayload) -> None:
    session = FakeSession()
    dao = FakeDAO(session)
    existing = RunRecord(
        run_id=3,
        job_id=uuid4(),
        genre="ai",
        status="failed",
        cluster_count=0,
        request_payload={"request_hash": None},
        response_payload=None,
        error_message="transient failure",
    )
    dao.idempotent_record = existing
    manager = make_manager(dao, session)
    scheduled = {}

    def recorder(record: RunRecord) -> None:
        scheduled["run_id"] = record.run_id

    manager._schedule_background = recorder  # type: ignore[attr-defined]
    submission = RunSubmission(
        job_id=existing.job_id, genre="ai", payload=payload, idempotency_key="k"
    )

    record = await manager.create_run(submission)

    assert record.run_id == INSERTED_RUN_ID
    assert session.commit_called
    assert dao.inserted
    assert scheduled["run_id"] == INSERTED_RUN_ID


@pytest.mark.asyncio
async def test_create_run_detects_idempotency_mismatch(payload: ClusterJobPayload) -> None:
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
    submission = RunSubmission(
        job_id=dao.idempotent_record.job_id, genre="ai", payload=payload, idempotency_key="k"
    )

    with pytest.raises(IdempotencyMismatchError):
        await manager.create_run(submission)


@pytest.mark.asyncio
async def test_create_run_blocks_when_running(payload: ClusterJobPayload) -> None:
    session = FakeSession()
    dao = FakeDAO(session)
    dao.running = True
    manager = make_manager(dao, session)
    submission = RunSubmission(job_id=uuid4(), genre="ai", payload=payload, idempotency_key=None)

    with pytest.raises(ConcurrentRunError):
        await manager.create_run(submission)


@pytest.mark.asyncio
async def test_create_classification_run_retries_failed_idempotent() -> None:
    session = FakeSession()
    dao = FakeDAO(session)
    payload = ClassificationJobPayload(texts=["classification text"])
    request_hash = _hash_payload(payload.model_dump(mode="json"))
    existing = RunRecord(
        run_id=11,
        job_id=uuid4(),
        genre="classification",
        status="failed",
        cluster_count=0,
        request_payload={"request_hash": request_hash},
        response_payload=None,
        error_message="stuck: process pool OOM",
    )
    dao.idempotent_record = existing
    manager = make_manager(dao, session)
    manager._classification_runner = ClassificationRunnerStub()

    submission = ClassificationRunSubmission(
        job_id=existing.job_id,
        payload=payload,
        idempotency_key="same-key",
    )
    record = await manager.create_classification_run(submission)

    assert record.run_id == INSERTED_RUN_ID
    assert session.commit_called
    assert dao.inserted


@pytest.mark.asyncio
async def test_get_run_uses_dao(payload: ClusterJobPayload) -> None:
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
async def test_process_run_persists_clusters(payload: ClusterJobPayload) -> None:
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
        def __init__(self, response: EvidenceResponse) -> None:
            self.response = response

        def run(self, request: EvidenceRequest) -> EvidenceResponse:
            return self.response

    manager = RunManager(
        Settings(),
        lambda: DummySessionContext(session),
        lambda _session: dao,  # pyrefly: ignore[bad-argument-type]
        pipeline=PipelineStub(pipeline_response),  # pyrefly: ignore[bad-argument-type]
    )

    await manager._process_run(1)

    assert dao.persisted_clusters is not None
    assert dao.success_status == (1, 1, "succeeded")
    assert dao.diagnostic_entries is not None


@pytest.mark.asyncio
async def test_process_run_uses_pipeline_runner(payload: ClusterJobPayload) -> None:
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
    manager = make_manager(
        dao, session, settings=settings, pipeline_runner=cast("PipelineTaskRunner", runner)
    )

    await manager._process_run(9)

    assert runner.called_with
    assert dao.success_status == (9, 0, "succeeded")


@pytest.mark.asyncio
async def test_process_run_times_out(payload: ClusterJobPayload) -> None:
    session = FakeSession()
    dao = FakeDAO(session)
    job_id = uuid4()
    record = RunRecord(
        run_id=TIMED_OUT_RUN_ID,
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
    manager = make_manager(
        dao, session, settings=settings, pipeline_runner=cast("PipelineTaskRunner", runner)
    )
    manager._run_timeout = 0.01

    await manager._process_run(TIMED_OUT_RUN_ID)

    assert dao.failure_status is not None
    assert dao.failure_status[0] == TIMED_OUT_RUN_ID
    assert "timed out" in dao.failure_status[2]


@pytest.mark.asyncio
async def test_process_run_filters_short_sentences_in_response(payload: ClusterJobPayload) -> None:
    """Test that run manager filters out short sentences from API response without failing."""
    session = FakeSession()
    dao = FakeDAO(session)
    job_id = uuid4()
    record = RunRecord(
        run_id=12,
        job_id=job_id,
        genre="tech",
        status="running",
        cluster_count=0,
        request_payload={"payload": payload.model_dump(mode="json")},
        response_payload=None,
        error_message=None,
    )
    dao.fetched_record = record

    # Create response with a short sentence that would cause ValidationError
    pipeline_response = EvidenceResponse(
        job_id="job",
        genre="tech",
        clusters=[
            EvidenceCluster(
                cluster_id=1,
                size=2,
                label=ClusterLabel(top_terms=["test"]),
                representatives=[
                    RepresentativeSentence(
                        text="This is a valid sentence with sufficient length.",
                        lang="en",
                        source=RepresentativeSource(source_id="art1"),
                    ),
                    RepresentativeSentence(
                        text="short",  # This should be filtered out
                        lang="en",
                        source=RepresentativeSource(source_id="art2"),
                    ),
                ],
                supporting_ids=["art1", "art2"],
                stats=ClusterStats(),
            ),
        ],
        evidence_budget=EvidenceBudget(sentences=2, tokens_estimated=100),
        diagnostics=Diagnostics(),
    )
    runner = PipelineRunnerStub(pipeline_response)
    settings = Settings(max_background_runs=1, pipeline_mode="processpool")
    manager = make_manager(
        dao, session, settings=settings, pipeline_runner=cast("PipelineTaskRunner", runner)
    )

    # Should not raise ValidationError
    await manager._process_run(12)

    # Should succeed and filter out the short sentence
    assert dao.success_status == (12, 1, "succeeded")
    # Verify that the response payload was created (would fail if ValidationError occurred)
    assert dao.success_status is not None


@pytest.mark.asyncio
async def test_convert_cluster_to_api_filters_short_sentences() -> None:
    """Test that _convert_cluster_to_api filters out short sentences."""
    session = FakeSession()
    dao = FakeDAO(session)
    manager = make_manager(dao, session)

    cluster = EvidenceCluster(
        cluster_id=1,
        size=3,
        label=ClusterLabel(top_terms=["test"]),
        representatives=[
            RepresentativeSentence(
                text="This is a valid sentence with sufficient length.",
                lang="en",
                source=RepresentativeSource(source_id="art1"),
            ),
            RepresentativeSentence(
                text="short",  # Should be filtered out
                lang="en",
                source=RepresentativeSource(source_id="art2"),
            ),
            RepresentativeSentence(
                text="Another valid sentence that exceeds minimum length.",
                lang="en",
                source=RepresentativeSource(source_id="art3"),
            ),
        ],
        supporting_ids=["art1", "art2", "art3"],
        stats=ClusterStats(),
    )

    result = manager._convert_cluster_to_api(cluster)

    # Should only include sentences with length >= 20
    expected_valid_count = 2
    assert len(result.representatives) == expected_valid_count
    for rep in result.representatives:
        assert len(rep.sentence_text) >= MIN_REPRESENTATIVE_CHARS


class BlockingClassificationRunnerStub:
    """Classification runner that never returns, to model an in-flight run."""

    def __init__(self) -> None:
        self.started = asyncio.Event()

    async def predict_batch(self, texts: list[str]) -> Never:
        self.started.set()
        await asyncio.sleep(3600)
        raise _UnexpectedCallError.predict_batch_not_cancelled()


class BlockingPipelineRunnerStub:
    """Pipeline runner that never returns, to model an in-flight run."""

    def __init__(self) -> None:
        self.started = asyncio.Event()

    async def run(self, request: EvidenceRequest) -> Never:
        self.started.set()
        await asyncio.sleep(3600)
        raise _UnexpectedCallError.run_not_cancelled()

    async def warmup(self) -> None:  # pragma: no cover - not used in tests
        return None


@pytest.mark.asyncio
async def test_create_run_sweeps_orphaned_running_run_from_previous_process(
    payload: ClusterJobPayload,
) -> None:
    session = FakeSession()
    dao = FakeDAO(session)
    job_id = uuid4()
    request_hash = _hash_payload(payload.model_dump(mode="json"))
    dao.idempotent_record = RunRecord(
        run_id=3,
        job_id=job_id,
        genre="ai",
        status="running",
        cluster_count=0,
        request_payload={"request_hash": request_hash},
        response_payload=None,
        error_message=None,
    )
    manager = make_manager(dao, session)
    manager._schedule_background = lambda record: None  # type: ignore[attr-defined]
    submission = RunSubmission(job_id=job_id, genre="ai", payload=payload, idempotency_key="k")

    record = await manager.create_run(submission)

    assert dao.orphan_sweeps, "orphaned 'running' rows must be swept before the idempotency lookup"
    assert record.run_id == INSERTED_RUN_ID, "a zombie 'running' run must not be reused"
    assert dao.inserted

    await manager.create_run(submission)

    assert len(dao.orphan_sweeps) == 1, "the sweep must run once per process"


@pytest.mark.asyncio
async def test_create_classification_run_sweeps_orphaned_running_run_from_previous_process() -> (
    None
):
    session = FakeSession()
    dao = FakeDAO(session)
    job_id = uuid4()
    payload = ClassificationJobPayload(texts=["classification text"])
    request_hash = _hash_payload(payload.model_dump(mode="json"))
    dao.idempotent_record = RunRecord(
        run_id=4,
        job_id=job_id,
        genre="classification",
        status="running",
        cluster_count=0,
        request_payload={"request_hash": request_hash},
        response_payload=None,
        error_message=None,
    )
    manager = make_manager(dao, session)
    manager._classification_runner = ClassificationRunnerStub()
    submission = ClassificationRunSubmission(
        job_id=job_id, payload=payload, idempotency_key="chunk-1"
    )

    record = await manager.create_classification_run(submission)

    assert dao.orphan_sweeps, "orphaned 'running' rows must be swept before the idempotency lookup"
    assert record.run_id == INSERTED_RUN_ID, "a zombie 'running' run must not be reused"
    assert dao.inserted

    await manager.shutdown()


@pytest.mark.asyncio
async def test_shutdown_marks_cancelled_classification_run_failed() -> None:
    session = FakeSession()
    dao = FakeDAO(session)
    job_id = uuid4()
    payload = ClassificationJobPayload(texts=["classification text"])
    dao.fetched_record = RunRecord(
        run_id=INSERTED_RUN_ID,
        job_id=job_id,
        genre="classification",
        status="running",
        cluster_count=0,
        request_payload={"payload": payload.model_dump(mode="json"), "type": "classification"},
        response_payload=None,
        error_message=None,
    )
    manager = make_manager(dao, session)
    runner = BlockingClassificationRunnerStub()
    manager._classification_runner = runner
    submission = ClassificationRunSubmission(
        job_id=job_id, payload=payload, idempotency_key="chunk-1"
    )

    await manager.create_classification_run(submission)
    await asyncio.wait_for(runner.started.wait(), timeout=5)
    await manager.shutdown()

    assert dao.failure_status is not None, "a cancelled run must not stay 'running'"
    assert dao.failure_status[0] == INSERTED_RUN_ID
    assert dao.failure_status[1] == "failed"


@pytest.mark.asyncio
async def test_shutdown_marks_cancelled_cluster_run_failed(payload: ClusterJobPayload) -> None:
    session = FakeSession()
    dao = FakeDAO(session)
    job_id = uuid4()
    dao.fetched_record = RunRecord(
        run_id=INSERTED_RUN_ID,
        job_id=job_id,
        genre="ai",
        status="running",
        cluster_count=0,
        request_payload={"payload": payload.model_dump(mode="json")},
        response_payload=None,
        error_message=None,
    )
    runner = BlockingPipelineRunnerStub()
    settings = Settings(max_background_runs=1, pipeline_mode="processpool")
    manager = make_manager(
        dao, session, settings=settings, pipeline_runner=cast("PipelineTaskRunner", runner)
    )
    submission = RunSubmission(job_id=job_id, genre="ai", payload=payload, idempotency_key=None)

    await manager.create_run(submission)
    await asyncio.wait_for(runner.started.wait(), timeout=5)
    await manager.shutdown()

    assert dao.failure_status is not None, "a cancelled run must not stay 'running'"
    assert dao.failure_status[0] == INSERTED_RUN_ID
    assert dao.failure_status[1] == "failed"


@pytest.mark.asyncio
async def test_process_run_marks_failed_when_connection_closed_on_mark_success(
    payload: ClusterJobPayload,
) -> None:
    sessions: list[FakeSession] = []

    def session_factory() -> DummySessionContext:
        s = FakeSession()
        sessions.append(s)
        return DummySessionContext(s)

    dao = FakeDAO(None)
    job_id = uuid4()
    dao.fetched_record = RunRecord(
        run_id=1,
        job_id=job_id,
        genre="ai",
        status="running",
        cluster_count=0,
        request_payload={"payload": payload.model_dump(mode="json")},
        response_payload=None,
        error_message=None,
    )

    async def _failing_mark_success(
        run_id: int, cluster_count: int, response_payload: object, status: str = "succeeded"
    ) -> Never:
        raise InterfaceError("UPDATE", {}, Exception("connection is closed"))

    async def _recording_mark_failure(run_id: int, status: str, error_message: str) -> None:
        dao.failure_status = (run_id, status, error_message)
        if dao.fetched_record is not None:
            dao.fetched_record = replace(
                dao.fetched_record, status=status, error_message=error_message
            )

    dao.mark_run_success = _failing_mark_success  # type: ignore[method-assign]
    dao.mark_run_failure = _recording_mark_failure  # type: ignore[method-assign]

    pipeline_response = EvidenceResponse(
        job_id="job",
        genre="ai",
        clusters=[],
        evidence_budget=EvidenceBudget(sentences=0, tokens_estimated=0),
        diagnostics=Diagnostics(),
    )
    runner = PipelineRunnerStub(pipeline_response)
    settings = Settings(max_background_runs=1, pipeline_mode="processpool")
    manager = RunManager(
        settings,
        session_factory,
        lambda _s: dao,  # pyrefly: ignore[bad-argument-type]
        pipeline_runner=runner,  # pyrefly: ignore[bad-argument-type]
    )

    with contextlib.suppress(InterfaceError):
        await manager._process_run(1)

    assert dao.fetched_record is not None
    assert dao.fetched_record.status == "failed"
    assert dao.failure_status is not None
    assert dao.failure_status[0] == 1
    assert dao.failure_status[1] == "failed"
    assert len(sessions) == EXPECTED_SESSION_COUNT
    assert sessions[-1].commit_called


@pytest.mark.asyncio
async def test_process_classification_run_marks_failed_when_connection_closed_on_mark_success() -> (
    None
):
    sessions: list[FakeSession] = []

    def session_factory() -> DummySessionContext:
        s = FakeSession()
        sessions.append(s)
        return DummySessionContext(s)

    dao = FakeDAO(None)
    job_id = uuid4()
    target_run_id = 2
    payload = ClassificationJobPayload(texts=["classification text"])
    dao.fetched_record = RunRecord(
        run_id=target_run_id,
        job_id=job_id,
        genre="classification",
        status="running",
        cluster_count=0,
        request_payload={"payload": payload.model_dump(mode="json"), "type": "classification"},
        response_payload=None,
        error_message=None,
    )

    async def _failing_mark_success(
        run_id: int, cluster_count: int, response_payload: object, status: str = "succeeded"
    ) -> Never:
        raise InterfaceError("UPDATE", {}, Exception("underlying connection is closed"))

    async def _recording_mark_failure(run_id: int, status: str, error_message: str) -> None:
        dao.failure_status = (run_id, status, error_message)
        if dao.fetched_record is not None:
            dao.fetched_record = replace(
                dao.fetched_record, status=status, error_message=error_message
            )

    dao.mark_run_success = _failing_mark_success  # type: ignore[method-assign]
    dao.mark_run_failure = _recording_mark_failure  # type: ignore[method-assign]

    class _OkClassificationRunner:
        async def predict_batch(self, texts: list[str]) -> list[dict[str, object]]:
            return [{"top_genre": "ai", "confidence": 0.9, "scores": {"ai": 0.9}}]

    manager = RunManager(
        Settings(),
        session_factory,
        lambda _s: dao,  # pyrefly: ignore[bad-argument-type]
        classification_runner=_OkClassificationRunner(),
    )

    with contextlib.suppress(InterfaceError):
        await manager._process_classification_run(target_run_id)

    assert dao.fetched_record is not None
    assert dao.fetched_record.status == "failed"
    assert dao.failure_status is not None
    assert dao.failure_status[0] == target_run_id
    assert dao.failure_status[1] == "failed"
    assert len(sessions) == EXPECTED_SESSION_COUNT
    assert sessions[-1].commit_called


@pytest.mark.asyncio
async def test_handle_failure_does_not_overwrite_succeeded_run() -> None:
    target_run_id = 3
    row: dict[str, object] = {"id": target_run_id, "status": "succeeded", "error_message": None}

    class _StatusAwareSession(FakeSession):
        async def execute(
            self,
            stmt: Executable,
        ) -> MagicMock:
            assert isinstance(stmt, ClauseElement)
            compiled = stmt.compile()
            sql = str(compiled)
            params = compiled.params
            assert params is not None
            result = MagicMock()
            if "AND recap_subworker_runs.status = " in sql and "running" in params.values():
                result.all = MagicMock(return_value=[])
                return result
            row["status"] = "failed"
            row["error_message"] = "connection is closed"
            result.all = MagicMock(return_value=[(target_run_id,)])
            return result

    session = _StatusAwareSession()
    manager = RunManager(
        Settings(),
        lambda: DummySessionContext(session),  # pyrefly: ignore[bad-argument-type]
        SubworkerDAO,  # pyrefly: ignore[bad-argument-type]
    )

    with capture_logs() as cap_logs:
        await manager._handle_failure(target_run_id, "connection is closed")

    assert row["status"] == "succeeded"
    assert row["error_message"] is None
    assert session.commit_called
    assert any(log.get("run_id") == target_run_id for log in cap_logs)
