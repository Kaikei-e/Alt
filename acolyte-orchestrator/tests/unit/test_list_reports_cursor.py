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


def _report_row(
    report_id: UUID,
    user_id: UUID = _DEFAULT_USER_ID,
    *,
    created_at: datetime = datetime(2026, 8, 14, tzinfo=UTC),
) -> Sequence[Any]:
    return (report_id, "Briefing", "weekly_briefing", 2, None, created_at, user_id)


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


@pytest.mark.asyncio
async def test_list_reports_paginates_by_composite_created_at_and_report_id_cursor() -> None:
    newer_ts = datetime(2026, 8, 15, 12, 0, tzinfo=UTC)
    tied_ts = datetime(2026, 8, 14, 9, 30, tzinfo=UTC)
    rid_a = UUID("00000000-0000-0000-0000-000000000003")
    rid_b = UUID("00000000-0000-0000-0000-000000000002")
    rid_c = UUID("00000000-0000-0000-0000-000000000001")

    page1_conn = _FakeConnection(
        rows=[
            _report_row(rid_a, created_at=newer_ts),
            _report_row(rid_b, created_at=tied_ts),
            _report_row(rid_c, created_at=tied_ts),
        ]
    )
    page1_gw = PostgresReportGateway(_FakePool(page1_conn))  # type: ignore[arg-type]
    page1_reports, next_cursor = await page1_gw.list_reports(cursor=None, limit=2, user_id=_DEFAULT_USER_ID)

    assert [r.report_id for r in page1_reports] == [rid_a, rid_b]
    assert next_cursor is not None
    assert len(page1_conn.executed) == 1
    q1, p1 = page1_conn.executed[0]
    assert "WHERE user_id = %s" in q1
    assert "ORDER BY created_at DESC, report_id DESC LIMIT %s" in q1
    assert p1 == [_DEFAULT_USER_ID, 3]

    page2_conn = _FakeConnection(rows=[_report_row(rid_c, created_at=tied_ts)])
    page2_gw = PostgresReportGateway(_FakePool(page2_conn))  # type: ignore[arg-type]
    page2_reports, page2_cursor = await page2_gw.list_reports(cursor=next_cursor, limit=2, user_id=_DEFAULT_USER_ID)

    assert [r.report_id for r in page2_reports] == [rid_c]
    assert page2_cursor is None
    assert len(page2_conn.executed) == 1
    q2, p2 = page2_conn.executed[0]
    assert "WHERE user_id = %s AND (created_at, report_id) < (%s, %s)" in q2
    assert "ORDER BY created_at DESC, report_id DESC LIMIT %s" in q2
    assert p2 == [_DEFAULT_USER_ID, tied_ts, rid_b, 3]
