"""Characterization tests for RunManager inner execution branches."""

from __future__ import annotations

from collections.abc import Iterable, Sequence
from types import TracebackType
from typing import Any
from uuid import UUID, uuid4

import pytest

from recap_subworker.db.dao import RunRecord
from recap_subworker.domain.models import (
    ClassificationJobPayload,
    ClusterDocument,
    ClusterJobParams,
    ClusterJobPayload,
    Diagnostics,
    EvidenceBudget,
    EvidenceRequest,
    EvidenceResponse,
)
from recap_subworker.infra.config import Settings
from recap_subworker.services.run_manager import RunManager

MISSING_PAYLOAD_RUN_ID = 101
DIAG_FAILURE_RUN_ID = 102
PARTIAL_RUN_ID = 103
DIRECT_CLASSIFIER_RUN_ID = 104
EXPECTED_CONFIDENCE = 0.88


class _FakeSession:
    def __init__(self) -> None:
        self.commit_called = False
        self.rollback_called = False

    async def commit(self) -> None:
        self.commit_called = True

    async def rollback(self) -> None:
        self.rollback_called = True


class _SessionCtx:
    def __init__(self, session: _FakeSession) -> None:
        self.session = session

    async def __aenter__(self) -> _FakeSession:
        return self.session

    async def __aexit__(
        self,
        exc_type: type[BaseException] | None,
        exc: BaseException | None,
        tb: TracebackType | None,
    ) -> bool:
        return False


class _StubDAO:
    def __init__(self, record: RunRecord | None) -> None:
        self.record = record
        self.failure_status: tuple[int, str, str] | None = None
        self.success_status: tuple[int, int, dict[str, Any], str] | None = None
        self.raise_on_run_diagnostics = False

    async def fetch_run(self, run_id: int) -> RunRecord | None:
        return self.record

    async def mark_run_failure(self, run_id: int, status: str, error_message: str) -> None:
        self.failure_status = (run_id, status, error_message)

    async def insert_clusters(self, run_id: int, clusters: Sequence[object]) -> None:
        return None

    async def upsert_diagnostics(self, run_id: int, entries: Iterable[object]) -> None:
        return None

    async def upsert_run_diagnostics(self, **_kwargs: object) -> None:
        if self.raise_on_run_diagnostics:
            msg = "diagnostics table missing"
            raise RuntimeError(msg)

    async def insert_system_metrics(
        self, metric_type: str, metrics: dict[str, object], job_id: UUID | None = None
    ) -> None:
        return None

    async def mark_run_success(
        self, run_id: int, cluster_count: int, payload: dict[str, Any], status: str = "succeeded"
    ) -> None:
        self.success_status = (run_id, cluster_count, payload, status)


class _StubPipelineRunner:
    def __init__(self, response: EvidenceResponse) -> None:
        self.response = response

    async def run(self, request: EvidenceRequest) -> EvidenceResponse:
        return self.response


class _DirectClassifierStub:
    def __init__(self) -> None:
        self.called_with: list[list[str]] = []

    def predict_batch(
        self,
        texts: list[str],
        multi_label: bool = False,
        top_k: int = 5,
        threshold_overrides: dict[str, float] | None = None,
    ) -> list[dict[str, Any]]:
        self.called_with.append(texts)
        return [
            {
                "top_genre": "ai_data",
                "confidence": EXPECTED_CONFIDENCE,
                "scores": {"ai_data": EXPECTED_CONFIDENCE},
                "candidates": [],
                "below_threshold": False,
            }
            for _ in texts
        ]


def _make_cluster_payload() -> ClusterJobPayload:
    return ClusterJobPayload(
        params=ClusterJobParams(
            max_sentences_total=100,
            umap_n_components=10,
            hdbscan_min_cluster_size=3,
            mmr_lambda=0.4,
        ),
        documents=[
            ClusterDocument(
                article_id=f"art-{i}",
                title="Title",
                paragraphs=["Sentence long enough to be valid for clustering test."],
            )
            for i in range(3)
        ],
    )


@pytest.mark.asyncio
async def test_process_run_inner_marks_failure_when_payload_missing() -> None:
    session = _FakeSession()
    record = RunRecord(
        run_id=MISSING_PAYLOAD_RUN_ID,
        job_id=uuid4(),
        genre="ai",
        status="running",
        cluster_count=0,
        request_payload={},
        response_payload=None,
        error_message=None,
    )
    dao = _StubDAO(record)
    runner = _StubPipelineRunner(
        EvidenceResponse(
            job_id="j",
            genre="ai",
            clusters=[],
            evidence_budget=EvidenceBudget(sentences=0, tokens_estimated=0),
            diagnostics=Diagnostics(),
        )
    )
    manager = RunManager(
        Settings(),
        lambda: _SessionCtx(session),  # pyrefly: ignore[bad-argument-type]
        lambda _s: dao,  # pyrefly: ignore[bad-argument-type]
        pipeline_runner=runner,  # pyrefly: ignore[bad-argument-type]
    )

    await manager._process_run_inner(MISSING_PAYLOAD_RUN_ID)

    assert dao.failure_status == (MISSING_PAYLOAD_RUN_ID, "failed", "request payload missing")
    assert session.commit_called


@pytest.mark.asyncio
async def test_process_run_inner_continues_when_run_diagnostics_fail_and_marks_partial() -> None:
    session = _FakeSession()
    payload = _make_cluster_payload()
    record = RunRecord(
        run_id=PARTIAL_RUN_ID,
        job_id=uuid4(),
        genre="ai",
        status="running",
        cluster_count=0,
        request_payload={"payload": payload.model_dump(mode="json")},
        response_payload=None,
        error_message=None,
    )
    dao = _StubDAO(record)
    dao.raise_on_run_diagnostics = True
    runner = _StubPipelineRunner(
        EvidenceResponse(
            job_id="j",
            genre="ai",
            clusters=[],
            evidence_budget=EvidenceBudget(sentences=0, tokens_estimated=0),
            diagnostics=Diagnostics(partial=True),
        )
    )
    manager = RunManager(
        Settings(),
        lambda: _SessionCtx(session),  # pyrefly: ignore[bad-argument-type]
        lambda _s: dao,  # pyrefly: ignore[bad-argument-type]
        pipeline_runner=runner,  # pyrefly: ignore[bad-argument-type]
    )

    await manager._process_run_inner(PARTIAL_RUN_ID)

    assert dao.success_status is not None
    assert dao.success_status[0] == PARTIAL_RUN_ID
    assert dao.success_status[3] == "partial"


@pytest.mark.asyncio
async def test_process_classification_run_inner_marks_failure_when_payload_missing() -> None:
    session = _FakeSession()
    record = RunRecord(
        run_id=DIAG_FAILURE_RUN_ID,
        job_id=uuid4(),
        genre="classification",
        status="running",
        cluster_count=0,
        request_payload={},
        response_payload=None,
        error_message=None,
    )
    dao = _StubDAO(record)
    classifier = _DirectClassifierStub()
    manager = RunManager(
        Settings(),
        lambda: _SessionCtx(session),  # pyrefly: ignore[bad-argument-type]
        lambda _s: dao,  # pyrefly: ignore[bad-argument-type]
        classifier=classifier,
    )

    await manager._process_classification_run_inner(DIAG_FAILURE_RUN_ID)

    assert dao.failure_status == (DIAG_FAILURE_RUN_ID, "failed", "request payload missing")
    assert not classifier.called_with


@pytest.mark.asyncio
async def test_process_classification_run_inner_uses_direct_classifier_fallback() -> None:
    session = _FakeSession()
    payload = ClassificationJobPayload(texts=["first text", "second text"])
    record = RunRecord(
        run_id=DIRECT_CLASSIFIER_RUN_ID,
        job_id=uuid4(),
        genre="classification",
        status="running",
        cluster_count=0,
        request_payload={"payload": payload.model_dump(mode="json")},
        response_payload=None,
        error_message=None,
    )
    dao = _StubDAO(record)
    classifier = _DirectClassifierStub()
    manager = RunManager(
        Settings(),
        lambda: _SessionCtx(session),  # pyrefly: ignore[bad-argument-type]
        lambda _s: dao,  # pyrefly: ignore[bad-argument-type]
        classifier=classifier,
    )

    await manager._process_classification_run_inner(DIRECT_CLASSIFIER_RUN_ID)

    assert classifier.called_with == [["first text", "second text"]]
    assert dao.success_status is not None
    expected_count = 2
    assert dao.success_status[1] == expected_count
    assert dao.success_status[3] == "succeeded"
