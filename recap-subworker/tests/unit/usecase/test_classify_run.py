"""Unit tests for SubmitClassificationRunUsecase / GetClassificationRunUsecase."""

from __future__ import annotations

from uuid import uuid4

import pytest

from recap_subworker.db.dao import RunRecord
from recap_subworker.domain.models import ClassificationJobPayload
from recap_subworker.services.run_manager import (
    ClassificationRunSubmission,
    ConcurrentRunError,
)
from recap_subworker.usecase.submit_run import (
    GetClassificationRunUsecase,
    SubmitClassificationRunUsecase,
)


class _FakeClassifySubmitter:
    def __init__(self, record: RunRecord | None = None, raise_exc: Exception | None = None) -> None:
        self.record = record
        self.raise_exc = raise_exc
        self.submissions: list[ClassificationRunSubmission] = []

    async def create_classification_run(
        self, submission: ClassificationRunSubmission
    ) -> RunRecord:
        self.submissions.append(submission)
        if self.raise_exc is not None:
            raise self.raise_exc
        assert self.record is not None
        return self.record


class _FakeReader:
    def __init__(self, record: RunRecord | None = None) -> None:
        self.record = record

    async def get_run(self, run_id: int) -> RunRecord | None:
        return self.record


def _make_submission() -> ClassificationRunSubmission:
    payload = ClassificationJobPayload.model_construct(texts=["sample"])
    return ClassificationRunSubmission(
        job_id=uuid4(),
        payload=payload,
        idempotency_key=None,
    )


def _make_record(status: str = "running") -> RunRecord:
    return RunRecord(
        run_id=1,
        job_id=uuid4(),
        genre="classification",
        status=status,
        cluster_count=0,
        request_payload={},
        response_payload=None,
        error_message=None,
    )


@pytest.mark.asyncio
async def test_submit_classification_run_delegates() -> None:
    record = _make_record()
    fake = _FakeClassifySubmitter(record=record)
    uc = SubmitClassificationRunUsecase(submitter=fake)

    submission = _make_submission()
    result = await uc.execute(submission)

    assert result is record
    assert fake.submissions == [submission]


@pytest.mark.asyncio
async def test_submit_classification_run_propagates_concurrent() -> None:
    fake = _FakeClassifySubmitter(raise_exc=ConcurrentRunError("busy"))
    uc = SubmitClassificationRunUsecase(submitter=fake)

    with pytest.raises(ConcurrentRunError):
        await uc.execute(_make_submission())


@pytest.mark.asyncio
async def test_get_classification_run_returns_record() -> None:
    record = _make_record()
    fake = _FakeReader(record=record)
    uc = GetClassificationRunUsecase(reader=fake)

    result = await uc.execute(1)

    assert result is record


@pytest.mark.asyncio
async def test_get_classification_run_returns_none() -> None:
    fake = _FakeReader(record=None)
    uc = GetClassificationRunUsecase(reader=fake)

    result = await uc.execute(999)

    assert result is None
