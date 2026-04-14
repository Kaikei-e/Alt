"""Unit tests for SubmitRunUsecase and GetRunUsecase.

Phase 2 introduces per-operation usecases between handlers and the
orchestrator (RunManager). The usecases depend only on port protocols
so handler tests can inject fakes without touching heavyweight services.
"""

from __future__ import annotations

from uuid import uuid4

import pytest

from recap_subworker.db.dao import RunRecord
from recap_subworker.domain.models import ClusterJobPayload
from recap_subworker.services.run_manager import (
    ConcurrentRunError,
    IdempotencyMismatchError,
    RunSubmission,
)
from recap_subworker.usecase.submit_run import (
    GetRunUsecase,
    SubmitRunUsecase,
)


class _FakeSubmitter:
    def __init__(self, record: RunRecord | None = None, raise_exc: Exception | None = None) -> None:
        self.record = record
        self.raise_exc = raise_exc
        self.submissions: list[RunSubmission] = []

    async def create_run(self, submission: RunSubmission) -> RunRecord:
        self.submissions.append(submission)
        if self.raise_exc is not None:
            raise self.raise_exc
        assert self.record is not None
        return self.record


class _FakeReader:
    def __init__(self, record: RunRecord | None = None) -> None:
        self.record = record
        self.requested_ids: list[int] = []

    async def get_run(self, run_id: int) -> RunRecord | None:
        self.requested_ids.append(run_id)
        return self.record


def _make_submission() -> RunSubmission:
    payload = ClusterJobPayload.model_construct(params=None, documents=[])
    return RunSubmission(
        job_id=uuid4(),
        genre="ai",
        payload=payload,
        idempotency_key=None,
    )


def _make_record(run_id: int = 1, job_id=None, status: str = "running") -> RunRecord:
    return RunRecord(
        run_id=run_id,
        job_id=job_id or uuid4(),
        genre="ai",
        status=status,
        cluster_count=0,
        request_payload={},
        response_payload=None,
        error_message=None,
    )


@pytest.mark.asyncio
async def test_submit_run_delegates_to_port() -> None:
    record = _make_record()
    fake = _FakeSubmitter(record=record)
    uc = SubmitRunUsecase(submitter=fake)

    submission = _make_submission()
    result = await uc.execute(submission)

    assert result is record
    assert fake.submissions == [submission]


@pytest.mark.asyncio
async def test_submit_run_propagates_idempotency_mismatch() -> None:
    fake = _FakeSubmitter(raise_exc=IdempotencyMismatchError("payload differs"))
    uc = SubmitRunUsecase(submitter=fake)

    with pytest.raises(IdempotencyMismatchError):
        await uc.execute(_make_submission())


@pytest.mark.asyncio
async def test_submit_run_propagates_concurrent_error() -> None:
    fake = _FakeSubmitter(raise_exc=ConcurrentRunError("already running"))
    uc = SubmitRunUsecase(submitter=fake)

    with pytest.raises(ConcurrentRunError):
        await uc.execute(_make_submission())


@pytest.mark.asyncio
async def test_get_run_returns_record_when_found() -> None:
    record = _make_record(run_id=42)
    fake = _FakeReader(record=record)
    uc = GetRunUsecase(reader=fake)

    result = await uc.execute(42)

    assert result is record
    assert fake.requested_ids == [42]


@pytest.mark.asyncio
async def test_get_run_returns_none_when_missing() -> None:
    fake = _FakeReader(record=None)
    uc = GetRunUsecase(reader=fake)

    result = await uc.execute(404)

    assert result is None
    assert fake.requested_ids == [404]
