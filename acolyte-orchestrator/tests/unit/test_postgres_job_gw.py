"""Unit tests for PostgresJobGateway — the audit-flagged unwired observation
fields (mark_running, latest_successful_run_id) and the new circuit-breaker
lookup query.

No fixture in this repo boots a live Postgres for unit tests (see
tests/conftest.py, which wires the in-memory gateways for the app-level
`client` fixture instead), so these tests double the small slice of the
psycopg_pool.AsyncConnectionPool surface PostgresJobGateway actually uses:
`pool.connection()` as an async context manager, `conn.execute(query,
params)` returning a cursor with `.fetchone()`, and `conn.transaction()`.
"""

from __future__ import annotations

from collections.abc import AsyncIterator, Sequence
from contextlib import AbstractAsyncContextManager, asynccontextmanager
from datetime import UTC, datetime
from typing import Any
from uuid import uuid4

import pytest

from acolyte.gateway.postgres_job_gw import PostgresJobGateway


class _FakeCursor:
    def __init__(self, row: Sequence[Any] | None) -> None:
        self._row = row

    async def fetchone(self) -> Sequence[Any] | None:
        return self._row

    async def fetchall(self) -> Sequence[Sequence[Any]]:
        # `row` doubles as the full result set for fetchall-style queries —
        # the test constructs it as a list of rows in that case.
        return self._row if self._row is not None else []


class _FakeConnection:
    """Records every conn.execute() call; answers with one canned row per call, in order."""

    def __init__(self, rows: list[Sequence[Any] | None]) -> None:
        self._rows = list(rows)
        self.executed: list[tuple[str, list[Any] | None]] = []

    async def execute(self, query: str, params: list[Any] | None = None) -> _FakeCursor:
        self.executed.append((query, params))
        row = self._rows.pop(0) if self._rows else None
        return _FakeCursor(row)

    def transaction(self) -> AbstractAsyncContextManager[None]:
        return _noop_cm()


@asynccontextmanager
async def _noop_cm() -> AsyncIterator[None]:
    yield


class _FakePool:
    def __init__(self, conn: _FakeConnection) -> None:
        self._conn = conn

    def connection(self) -> AbstractAsyncContextManager[_FakeConnection]:
        return _conn_cm(self._conn)


@asynccontextmanager
async def _conn_cm(conn: _FakeConnection) -> AsyncIterator[_FakeConnection]:
    yield conn


def _gw(conn: _FakeConnection) -> PostgresJobGateway:
    # Notifications off — the outbox producer has its own module,
    # tests/unit/test_notification_outbox_producer.py.
    return PostgresJobGateway(_FakePool(conn), notification_user_id=None)  # type: ignore[arg-type]


@pytest.mark.asyncio
async def test_mark_running_sets_status_started_at_and_model_names() -> None:
    run_id = uuid4()
    conn = _FakeConnection(rows=[None])
    gw = _gw(conn)

    await gw.mark_running(run_id, "gemma4-e4b-12k", "gemma4-e4b-12k", "gemma4-e4b-12k")

    assert len(conn.executed) == 1
    query, params = conn.executed[0]
    assert "run_status = 'running'" in query
    assert "started_at = NOW()" in query
    assert "planner_model = %s" in query
    assert "writer_model = %s" in query
    assert "critic_model = %s" in query
    assert "WHERE run_id = %s" in query
    assert params == ["gemma4-e4b-12k", "gemma4-e4b-12k", "gemma4-e4b-12k", run_id]


@pytest.mark.asyncio
async def test_complete_run_updates_reports_latest_successful_run_id_same_transaction() -> None:
    run_id = uuid4()
    report_id = uuid4()
    # The completion also returns finished_at, which the outbox producer uses
    # as business time (see test_notification_outbox_producer.py).
    conn = _FakeConnection(rows=[(report_id, datetime(2026, 8, 8, tzinfo=UTC)), None])
    gw = _gw(conn)

    await gw.complete_run(run_id)

    assert len(conn.executed) == 2

    run_query, run_params = conn.executed[0]
    assert "run_status = 'succeeded'" in run_query
    assert "RETURNING report_id" in run_query
    assert run_params == [run_id]

    report_query, report_params = conn.executed[1]
    assert "UPDATE reports" in report_query
    assert "latest_successful_run_id = %s" in report_query
    assert report_params == [run_id, report_id]


@pytest.mark.asyncio
async def test_complete_run_skips_reports_update_when_run_id_unknown() -> None:
    """If the run_id doesn't exist, there's no report to point latest_successful_run_id at."""
    run_id = uuid4()
    conn = _FakeConnection(rows=[None])
    gw = _gw(conn)

    await gw.complete_run(run_id)

    assert len(conn.executed) == 1


@pytest.mark.asyncio
async def test_get_latest_run_for_report_returns_most_recently_created_run() -> None:
    report_id = uuid4()
    run_id = uuid4()
    row = (
        run_id,
        report_id,
        3,
        "failed",
        "gemma4-e4b-12k",
        "gemma4-e4b-12k",
        "gemma4-e4b-12k",
        None,
        datetime(2026, 7, 23, 9, 0, tzinfo=UTC),
        "pipeline_error",
        "boom",
    )
    conn = _FakeConnection(rows=[row])
    gw = _gw(conn)

    result = await gw.get_latest_run_for_report(report_id)

    assert result is not None
    assert result.run_id == run_id
    assert result.run_status == "failed"
    assert result.failure_code == "pipeline_error"
    assert result.finished_at == datetime(2026, 7, 23, 9, 0, tzinfo=UTC)

    query, params = conn.executed[0]
    # report_runs has no created_at; ordering must ride on report_jobs.created_at
    # (one row per run, inserted alongside it in create_run) instead of the
    # nullable started_at/finished_at columns.
    assert "report_jobs" in query
    assert "ORDER BY" in query
    assert "j.created_at DESC" in query
    assert params == [report_id]


@pytest.mark.asyncio
async def test_get_latest_run_for_report_returns_none_when_report_has_no_runs() -> None:
    conn = _FakeConnection(rows=[None])
    gw = _gw(conn)

    result = await gw.get_latest_run_for_report(uuid4())

    assert result is None


@pytest.mark.asyncio
async def test_list_running_runs_returns_every_running_row() -> None:
    """Startup reconciliation needs every orphaned run, not just one per report."""
    run_id_1, run_id_2 = uuid4(), uuid4()
    report_id_1, report_id_2 = uuid4(), uuid4()
    rows = [
        (
            run_id_1,
            report_id_1,
            1,
            "running",
            None,
            None,
            None,
            datetime(2026, 7, 24, 8, 0, tzinfo=UTC),
            None,
            None,
            None,
        ),
        (
            run_id_2,
            report_id_2,
            2,
            "running",
            None,
            None,
            None,
            datetime(2026, 7, 24, 9, 0, tzinfo=UTC),
            None,
            None,
            None,
        ),
    ]
    conn = _FakeConnection(rows=[rows])
    gw = _gw(conn)

    result = await gw.list_running_runs()

    assert {r.run_id for r in result} == {run_id_1, run_id_2}
    assert all(r.run_status == "running" for r in result)
    query, params = conn.executed[0]
    assert "run_status = 'running'" in query
    assert params is None


@pytest.mark.asyncio
async def test_list_running_runs_returns_empty_list_when_none_running() -> None:
    conn = _FakeConnection(rows=[[]])
    gw = _gw(conn)

    result = await gw.list_running_runs()

    assert result == []
