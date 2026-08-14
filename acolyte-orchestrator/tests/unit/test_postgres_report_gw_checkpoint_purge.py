"""DeleteReport must also reclaim the LangGraph checkpoints of the report's runs.

The checkpoint tables are created by AsyncPostgresSaver.setup(), not by Atlas,
and they carry no FK back to reports — so a `DELETE FROM reports` leaves the
per-super-step state (article bodies included) behind forever. These tests
drive the RPC seam (DeleteReport → PostgresReportGateway) so the purge is
pinned where the caller sees it.

As in tests/unit/test_postgres_job_gw.py, no live Postgres exists for unit
tests: `_FakeConnection` doubles the slice of psycopg the gateway uses, with
the checkpoint tables present or absent to mirror CHECKPOINT_ENABLED=true
(compose/acolyte.yaml) and =false (compose/compose.staging.yaml).
"""

from __future__ import annotations

from collections.abc import AsyncIterator, Sequence
from contextlib import AbstractAsyncContextManager, asynccontextmanager
from datetime import UTC, datetime
from typing import Any
from unittest.mock import MagicMock
from uuid import UUID, uuid4

import psycopg
import pytest

from acolyte.gateway.postgres_report_gw import PostgresReportGateway
from acolyte.gen.proto.alt.acolyte.v1 import acolyte_pb2
from acolyte.handler.connect_service import AcolyteConnectService

_CHECKPOINT_TABLES = ("checkpoints", "checkpoint_blobs", "checkpoint_writes")
_UNDEFINED_TABLE = 'relation "checkpoints" does not exist'


class _FakeCursor:
    def __init__(self, rows: list[Sequence[Any]]) -> None:
        self._rows = rows

    async def fetchone(self) -> Sequence[Any] | None:
        return self._rows[0] if self._rows else None

    async def fetchall(self) -> list[Sequence[Any]]:
        return list(self._rows)


class _FakeConnection:
    """Answers the queries the DeleteReport path issues, and records every one."""

    def __init__(
        self,
        *,
        report_row: Sequence[Any],
        run_ids: list[UUID],
        checkpoint_tables: bool = True,
    ) -> None:
        self._report_row = report_row
        self._run_ids = run_ids
        self._checkpoint_tables = checkpoint_tables
        self.executed: list[tuple[str, list[Any] | None]] = []

    async def execute(self, query: str, params: list[Any] | None = None) -> _FakeCursor:
        self.executed.append((query, params))
        if any(table in query for table in _CHECKPOINT_TABLES) and not self._checkpoint_tables:
            # A database where the checkpointer never ran has no such tables.
            if "to_regclass" in query:
                return _FakeCursor([(False,)])
            raise psycopg.errors.UndefinedTable(_UNDEFINED_TABLE)
        if "to_regclass" in query:
            return _FakeCursor([(True,)])
        if "FROM reports WHERE report_id" in query:
            return _FakeCursor([self._report_row])
        if "SELECT EXISTS" in query:
            return _FakeCursor([(False,)])
        if "FROM report_runs" in query:
            return _FakeCursor([(run_id,) for run_id in self._run_ids])
        return _FakeCursor([])

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


def _report_row(report_id: UUID) -> Sequence[Any]:
    return (report_id, "Disposable", "custom", 1, None, datetime(2026, 8, 14, tzinfo=UTC))


def _service(conn: _FakeConnection) -> AcolyteConnectService:
    return AcolyteConnectService(MagicMock(), PostgresReportGateway(_FakePool(conn)))  # type: ignore[arg-type]


def _deletes_for(conn: _FakeConnection, table: str) -> list[tuple[str, list[Any] | None]]:
    prefix = f"DELETE FROM {table} "  # noqa: S608 — matches recorded SQL, never executed
    return [(q, p) for q, p in conn.executed if q.startswith(prefix)]


@pytest.mark.asyncio
async def test_delete_report_purges_checkpoints_of_every_run() -> None:
    report_id, run_a, run_b = uuid4(), uuid4(), uuid4()
    conn = _FakeConnection(report_row=_report_row(report_id), run_ids=[run_a, run_b])

    await _service(conn).delete_report(
        acolyte_pb2.DeleteReportRequest(report_id=str(report_id)),
        ctx=None,  # type: ignore[bad-argument-type]
    )

    expected_threads = {f"acolyte-run:{run_a}", f"acolyte-run:{run_b}"}
    for table in _CHECKPOINT_TABLES:
        deletes = _deletes_for(conn, table)
        assert deletes, f"no DELETE issued against {table}"
        purged = {thread for _, params in deletes for param in (params or []) for thread in _as_threads(param)}
        assert purged == expected_threads, f"{table} purge covered {purged}"


def _as_threads(param: object) -> list[str]:
    """Accept either one thread id per statement or a list bound to ANY(%s)."""
    if isinstance(param, str):
        return [param]
    if isinstance(param, list):
        return [p for p in param if isinstance(p, str)]
    return []


@pytest.mark.asyncio
async def test_delete_report_reads_run_ids_before_the_cascade_removes_them() -> None:
    """DELETE FROM reports cascades report_runs away — the thread ids must be read first."""
    report_id = uuid4()
    conn = _FakeConnection(report_row=_report_row(report_id), run_ids=[uuid4()])

    await _service(conn).delete_report(
        acolyte_pb2.DeleteReportRequest(report_id=str(report_id)),
        ctx=None,  # type: ignore[bad-argument-type]
    )

    queries = [q for q, _ in conn.executed]
    runs_read = next(i for i, q in enumerate(queries) if "FROM report_runs" in q)
    report_deleted = next(i for i, q in enumerate(queries) if q.startswith("DELETE FROM reports "))
    checkpoints_deleted = next(i for i, q in enumerate(queries) if q.startswith("DELETE FROM checkpoints "))
    assert runs_read < checkpoints_deleted < report_deleted


@pytest.mark.asyncio
async def test_delete_report_without_runs_touches_no_checkpoint_table() -> None:
    report_id = uuid4()
    conn = _FakeConnection(report_row=_report_row(report_id), run_ids=[])

    await _service(conn).delete_report(
        acolyte_pb2.DeleteReportRequest(report_id=str(report_id)),
        ctx=None,  # type: ignore[bad-argument-type]
    )

    assert not [q for q, _ in conn.executed if "checkpoint" in q]
    assert [q for q, _ in conn.executed if q.startswith("DELETE FROM reports ")]


@pytest.mark.asyncio
async def test_delete_report_succeeds_when_checkpointing_was_never_enabled() -> None:
    """CHECKPOINT_ENABLED=false never runs setup(), so the tables do not exist."""
    report_id = uuid4()
    conn = _FakeConnection(report_row=_report_row(report_id), run_ids=[uuid4()], checkpoint_tables=False)

    resp = await _service(conn).delete_report(
        acolyte_pb2.DeleteReportRequest(report_id=str(report_id)),
        ctx=None,  # type: ignore[bad-argument-type]
    )

    assert isinstance(resp, acolyte_pb2.DeleteReportResponse)
    assert [q for q, _ in conn.executed if q.startswith("DELETE FROM reports ")]
