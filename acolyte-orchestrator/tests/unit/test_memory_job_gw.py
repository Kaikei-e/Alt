"""Unit tests for MemoryJobGateway's JobQueuePort conformance.

The in-memory double stands in for PostgresJobGateway in every unit/e2e test
that does not talk to a DB, so any place the two disagree about the port
contract is a place those tests assert behaviour production does not have.
"""

from __future__ import annotations

from uuid import uuid4

import pytest

from acolyte.gateway.memory_job_gw import MemoryJobGateway


@pytest.mark.asyncio
async def test_list_running_runs_includes_pending_runs() -> None:
    """create_run commits 'pending' before the pipeline reaches mark_running, so a
    reconciliation scoped to 'running' leaves a run orphaned in that window wedging
    get_active_run_for_report forever. PostgresJobGateway selects
    run_status IN ('pending', 'running'); the double must match."""
    gw = MemoryJobGateway()
    pending = await gw.create_run(uuid4(), 1)

    result = await gw.list_running_runs()

    assert [r.run_id for r in result] == [pending.run_id]


@pytest.mark.asyncio
async def test_list_running_runs_includes_running_runs() -> None:
    gw = MemoryJobGateway()
    run = await gw.create_run(uuid4(), 1)
    await gw.mark_running(run.run_id, "planner", "writer", "critic")

    result = await gw.list_running_runs()

    assert [r.run_id for r in result] == [run.run_id]
    assert result[0].run_status == "running"


@pytest.mark.asyncio
async def test_list_running_runs_excludes_finished_runs() -> None:
    gw = MemoryJobGateway()
    succeeded = await gw.create_run(uuid4(), 1)
    await gw.complete_run(succeeded.run_id)
    failed = await gw.create_run(uuid4(), 1)
    await gw.fail_run(failed.run_id, "boom", "boom")

    result = await gw.list_running_runs()

    assert result == []
