"""Unit tests for ListReports and ListReportVersions cursor validation."""

from __future__ import annotations

from collections.abc import AsyncIterator, Sequence
from contextlib import AbstractAsyncContextManager, asynccontextmanager
from datetime import UTC, datetime
from typing import Any
from unittest.mock import MagicMock
from uuid import UUID, uuid4

import pytest
from connectrpc.code import Code
from connectrpc.errors import ConnectError

from acolyte.gateway.postgres_report_gw import PostgresReportGateway
from acolyte.gen.proto.alt.acolyte.v1 import acolyte_pb2
from acolyte.handler.connect_service import AcolyteConnectService
from tests.conftest import make_request_ctx

_DEFAULT_USER_ID = uuid4()


class _FakeCursor:
    def __init__(self, rows: list[Sequence[Any]]) -> None:
        self._rows = rows

    async def fetchone(self) -> Sequence[Any] | None:
        return self._rows[0] if self._rows else None

    async def fetchall(self) -> list[Sequence[Any]]:
        return list(self._rows)


class _FakeConnection:
    def __init__(self, *, report_row: Sequence[Any] | None = None, rows: list[Sequence[Any]] | None = None) -> None:
        self._report_row = report_row
        self._rows = rows or []
        self.executed: list[tuple[str, list[Any] | None]] = []

    async def execute(self, query: str, params: list[Any] | None = None) -> _FakeCursor:
        self.executed.append((query, params))
        if "FROM reports WHERE report_id = %s" in query:
            return _FakeCursor([self._report_row] if self._report_row is not None else [])
        return _FakeCursor(self._rows)


class _FakePool:
    def __init__(self, conn: _FakeConnection) -> None:
        self._conn = conn

    def connection(self) -> AbstractAsyncContextManager[_FakeConnection]:
        return _conn_cm(self._conn)


@asynccontextmanager
async def _conn_cm(conn: _FakeConnection) -> AsyncIterator[_FakeConnection]:
    yield conn


def _report_row(report_id: UUID, user_id: UUID = _DEFAULT_USER_ID) -> Sequence[Any]:
    return (report_id, "Briefing", "weekly_briefing", 2, None, datetime(2026, 8, 14, tzinfo=UTC), user_id)


def _service(conn: _FakeConnection) -> AcolyteConnectService:
    return AcolyteConnectService(MagicMock(), PostgresReportGateway(_FakePool(conn)))  # type: ignore[arg-type]


@pytest.mark.asyncio
async def test_list_reports_rejects_malformed_cursor_with_invalid_argument() -> None:
    conn = _FakeConnection()
    service = _service(conn)

    with pytest.raises(ConnectError) as exc_info:
        await service.list_reports(
            acolyte_pb2.ListReportsRequest(cursor="abc", limit=10),
            ctx=make_request_ctx("ListReports", _DEFAULT_USER_ID),
        )

    assert exc_info.value.code == Code.INVALID_ARGUMENT
    assert conn.executed == []


@pytest.mark.asyncio
async def test_list_report_versions_rejects_malformed_cursor_with_invalid_argument() -> None:
    report_id = uuid4()
    conn = _FakeConnection(report_row=_report_row(report_id))
    service = _service(conn)

    with pytest.raises(ConnectError) as exc_info:
        await service.list_report_versions(
            acolyte_pb2.ListReportVersionsRequest(report_id=str(report_id), cursor="abc", limit=10),
            ctx=make_request_ctx("ListReportVersions", _DEFAULT_USER_ID),
        )

    assert exc_info.value.code == Code.INVALID_ARGUMENT
    assert not [q for q, _ in conn.executed if "FROM report_versions" in q]
