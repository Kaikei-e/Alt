"""Unit tests for checkpoint-aware Connect service pipeline execution."""

from __future__ import annotations

from types import SimpleNamespace
from unittest.mock import AsyncMock, MagicMock

import pytest

from acolyte.handler.connect_service import AcolyteConnectService


@pytest.mark.asyncio
async def test_run_pipeline_passes_namespaced_thread_id() -> None:
    """Pipeline invocations must use a stable thread_id derived from run_id."""
    settings = SimpleNamespace(checkpoint_enabled=False)
    graph = MagicMock()
    graph.ainvoke = AsyncMock(return_value={"final_version_no": 2})
    graph.aget_state = AsyncMock()
    service = AcolyteConnectService(settings, MagicMock(), graph=graph)  # type: ignore[bad-argument-type]

    await service._run_pipeline("report-1", "run-1", {"topic": "AI"})

    graph.aget_state.assert_not_awaited()
    graph.ainvoke.assert_awaited_once_with(
        {
            "report_id": "report-1",
            "run_id": "run-1",
            "brief": {"topic": "AI"},
            "revision_count": 0,
        },
        config={"configurable": {"thread_id": "acolyte-run:run-1"}},
        durability=None,
    )


@pytest.mark.asyncio
async def test_run_pipeline_resumes_from_pending_checkpoint() -> None:
    """Existing pending checkpoint should resume with None input."""
    settings = SimpleNamespace(checkpoint_enabled=True)
    snapshot = SimpleNamespace(values={"report_id": "report-1"}, next=("writer",))
    graph = MagicMock()
    graph.aget_state = AsyncMock(return_value=snapshot)
    graph.ainvoke = AsyncMock(return_value={"final_version_no": 2})
    service = AcolyteConnectService(settings, MagicMock(), graph=graph)  # type: ignore[bad-argument-type]

    await service._run_pipeline("report-1", "run-1", {"topic": "AI"})

    graph.aget_state.assert_awaited_once_with({"configurable": {"thread_id": "acolyte-run:run-1"}})
    graph.ainvoke.assert_awaited_once_with(
        None,
        config={"configurable": {"thread_id": "acolyte-run:run-1"}},
        durability="sync",
    )


@pytest.mark.asyncio
async def test_run_pipeline_skips_completed_checkpoint() -> None:
    """Completed terminal checkpoint should not re-run the graph."""
    settings = SimpleNamespace(checkpoint_enabled=True)
    snapshot = SimpleNamespace(values={"final_version_no": 7}, next=())
    graph = MagicMock()
    graph.aget_state = AsyncMock(return_value=snapshot)
    graph.ainvoke = AsyncMock()
    service = AcolyteConnectService(settings, MagicMock(), graph=graph)  # type: ignore[bad-argument-type]

    await service._run_pipeline("report-1", "run-1", {"topic": "AI"})

    graph.aget_state.assert_awaited_once_with({"configurable": {"thread_id": "acolyte-run:run-1"}})
    graph.ainvoke.assert_not_called()


@pytest.mark.asyncio
async def test_run_pipeline_uses_sync_durability_when_checkpoint_enabled() -> None:
    """When checkpoint is enabled, ainvoke must pass durability='sync'."""
    settings = SimpleNamespace(checkpoint_enabled=True)
    snapshot = SimpleNamespace(values={}, next=())  # no prior checkpoint
    graph = MagicMock()
    graph.aget_state = AsyncMock(return_value=snapshot)
    graph.ainvoke = AsyncMock(return_value={"final_version_no": 1})
    service = AcolyteConnectService(settings, MagicMock(), graph=graph)  # type: ignore[bad-argument-type]

    await service._run_pipeline("report-1", "run-1", {"topic": "AI"})

    call_kwargs = graph.ainvoke.call_args
    assert call_kwargs.kwargs.get("durability") == "sync" or (
        len(call_kwargs.args) > 2 and call_kwargs.args[2] == "sync"
    ), "ainvoke must be called with durability='sync' when checkpointing is enabled"


@pytest.mark.asyncio
async def test_run_pipeline_completed_checkpoint_requires_final_version() -> None:
    """A terminal checkpoint without final_version_no should not be treated as completed.

    If next==() but final_version_no is missing, the pipeline errored at a terminal
    state and should log a warning rather than silently returning success.
    """
    settings = SimpleNamespace(checkpoint_enabled=True)
    # Terminal state but no final_version_no — incomplete/error state
    snapshot = SimpleNamespace(values={"report_id": "report-1"}, next=())
    graph = MagicMock()
    graph.aget_state = AsyncMock(return_value=snapshot)
    graph.ainvoke = AsyncMock(return_value={"final_version_no": 2})
    service = AcolyteConnectService(settings, MagicMock(), graph=graph)  # type: ignore[bad-argument-type]

    await service._run_pipeline("report-1", "run-1", {"topic": "AI"})

    # Should NOT short-circuit — should invoke the graph since final_version_no is missing
    graph.ainvoke.assert_awaited_once()
