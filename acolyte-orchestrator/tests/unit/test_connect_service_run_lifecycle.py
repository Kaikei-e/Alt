"""Unit tests for connect_service.py's run-lifecycle wiring:

- mark_running is called via JobQueuePort at the start of _run_pipeline_locked.
- fail_run receives the pipeline's own failure_code (e.g. "no_evidence") instead
  of the generic hardcoded "pipeline_error" whenever the graph result carries one.
- The existing "final_version_no => terminal checkpoint" resume logic
  (connect_service.py:273-296) still re-runs an aborted (no-version) checkpoint
  instead of treating it as complete.
"""

from __future__ import annotations

from types import SimpleNamespace
from unittest.mock import AsyncMock, MagicMock
from uuid import UUID, uuid4

import pytest

from acolyte.domain.run import ReportJob, ReportRun
from acolyte.handler.connect_service import AcolyteConnectService


class FakeJobQueue:
    def __init__(self) -> None:
        self.mark_running_calls: list[tuple[UUID, str, str, str]] = []
        self.fail_run_calls: list[tuple[UUID, str, str]] = []
        self.complete_run_calls: list[UUID] = []

    async def mark_running(self, run_id: UUID, planner_model: str, writer_model: str, critic_model: str) -> None:
        self.mark_running_calls.append((run_id, planner_model, writer_model, critic_model))

    async def fail_run(self, run_id: UUID, failure_code: str, failure_message: str) -> None:
        self.fail_run_calls.append((run_id, failure_code, failure_message))

    async def complete_run(self, run_id: UUID) -> None:
        self.complete_run_calls.append(run_id)

    # Unused stubs for the rest of JobQueuePort.
    async def create_run(self, report_id: UUID, target_version_no: int) -> ReportRun:
        raise NotImplementedError

    async def get_run(self, run_id: UUID) -> ReportRun | None:
        return None

    async def get_active_run_for_report(self, report_id: UUID) -> ReportRun | None:
        return None

    async def get_latest_run_for_report(self, report_id: UUID) -> ReportRun | None:
        return None

    async def claim_job(self, worker_id: str) -> ReportJob | None:
        return None

    async def update_job_status(self, job_id: UUID, status: str) -> None:
        return None

    async def complete_job(self, job_id: UUID) -> None:
        return None

    async def fail_job(self, job_id: UUID, failure_message: str) -> None:
        return None


@pytest.mark.asyncio
async def test_run_pipeline_marks_running_before_invoking_graph() -> None:
    settings = SimpleNamespace(checkpoint_enabled=False, default_model="gemma4-e4b-12k")
    graph = MagicMock()
    graph.ainvoke = AsyncMock(return_value={"final_version_no": 2})
    jobs = FakeJobQueue()
    service = AcolyteConnectService(settings, MagicMock(), job_queue=jobs, graph=graph)  # type: ignore[bad-argument-type]
    run_id = str(uuid4())

    await service._run_pipeline("report-1", run_id, {"topic": "AI"})

    assert len(jobs.mark_running_calls) == 1
    call_run_id, planner, writer, critic = jobs.mark_running_calls[0]
    assert str(call_run_id) == run_id
    assert planner == writer == critic == "gemma4-e4b-12k"
    # mark_running must happen before the graph is invoked.
    graph.ainvoke.assert_awaited_once()


@pytest.mark.asyncio
async def test_run_pipeline_skips_mark_running_when_job_queue_is_none() -> None:
    """Backward-compat: no job queue wired must not crash the pipeline."""
    settings = SimpleNamespace(checkpoint_enabled=False, default_model="gemma4-e4b-12k")
    graph = MagicMock()
    graph.ainvoke = AsyncMock(return_value={"final_version_no": 1})
    service = AcolyteConnectService(settings, MagicMock(), graph=graph)  # type: ignore[bad-argument-type]

    await service._run_pipeline("report-1", str(uuid4()), {"topic": "AI"})

    graph.ainvoke.assert_awaited_once()


@pytest.mark.asyncio
async def test_run_pipeline_propagates_failure_code_from_graph_result() -> None:
    """fail_run must receive the graph's own failure_code, not a generic default."""
    settings = SimpleNamespace(checkpoint_enabled=False, default_model="gemma4-e4b-12k")
    graph = MagicMock()
    graph.ainvoke = AsyncMock(return_value={"error": "No curated evidence available", "failure_code": "no_evidence"})
    jobs = FakeJobQueue()
    service = AcolyteConnectService(settings, MagicMock(), job_queue=jobs, graph=graph)  # type: ignore[bad-argument-type]

    await service._run_pipeline("report-1", str(uuid4()), {"topic": "AI"})

    assert len(jobs.fail_run_calls) == 1
    _run_id, failure_code, failure_message = jobs.fail_run_calls[0]
    assert failure_code == "no_evidence"
    assert failure_message == "No curated evidence available"


@pytest.mark.asyncio
async def test_run_pipeline_falls_back_to_generic_failure_code_when_absent() -> None:
    """Backward-compat: an error without failure_code still fails the run."""
    settings = SimpleNamespace(checkpoint_enabled=False, default_model="gemma4-e4b-12k")
    graph = MagicMock()
    graph.ainvoke = AsyncMock(return_value={"error": "boom"})
    jobs = FakeJobQueue()
    service = AcolyteConnectService(settings, MagicMock(), job_queue=jobs, graph=graph)  # type: ignore[bad-argument-type]

    await service._run_pipeline("report-1", str(uuid4()), {"topic": "AI"})

    assert jobs.fail_run_calls[0][1] == "pipeline_error"


@pytest.mark.asyncio
async def test_run_pipeline_resumes_aborted_checkpoint_without_final_version() -> None:
    """A finalize_guard abort (failure_code set, no final_version_no) leaves a
    terminal checkpoint (next == ()) that must still be re-run on resume —
    it must not be mistaken for a completed run."""
    settings = SimpleNamespace(checkpoint_enabled=True, default_model="gemma4-e4b-12k")
    snapshot = SimpleNamespace(
        values={"report_id": "report-1", "error": "No curated evidence available", "failure_code": "no_evidence"},
        next=(),
    )
    graph = MagicMock()
    graph.aget_state = AsyncMock(return_value=snapshot)
    graph.ainvoke = AsyncMock(return_value={"final_version_no": 1})
    jobs = FakeJobQueue()
    service = AcolyteConnectService(settings, MagicMock(), job_queue=jobs, graph=graph)  # type: ignore[bad-argument-type]
    run_id = str(uuid4())

    await service._run_pipeline("report-1", run_id, {"topic": "AI"})

    # Must re-invoke the graph rather than short-circuit as "already completed".
    graph.ainvoke.assert_awaited_once()
    assert jobs.fail_run_calls == []
    # The re-run succeeds this time, so it completes normally.
    assert jobs.complete_run_calls == [UUID(run_id)]
