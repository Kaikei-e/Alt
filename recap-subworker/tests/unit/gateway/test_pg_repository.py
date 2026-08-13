"""Tests for PgRunRepository's conformance to RunRepositoryPort."""

from __future__ import annotations

from unittest.mock import AsyncMock

import pytest

from recap_subworker.gateway.pg_repository import PgRunRepository
from recap_subworker.port.repository import RunRepositoryPort


def test_pg_run_repository_satisfies_port() -> None:
    repo = PgRunRepository(AsyncMock())

    assert isinstance(repo, RunRepositoryPort)


@pytest.mark.asyncio
async def test_fail_orphaned_runs_delegates_to_dao() -> None:
    repo = PgRunRepository(AsyncMock())
    repo._dao = AsyncMock()
    repo._dao.fail_orphaned_runs.return_value = 3

    swept = await repo.fail_orphaned_runs("orphaned by restart")

    assert swept == 3
    repo._dao.fail_orphaned_runs.assert_awaited_once_with("orphaned by restart")
