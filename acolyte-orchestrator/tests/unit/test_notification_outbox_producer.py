"""The notification outbox INSERT rides the run-completion transaction.

That co-location is the entire guarantee: the notification is enqueued if and
only if the completion commits. These tests pin the statement to the same
connection *and* the same open transaction as the run update, plus the payload
hygiene rules (no titles, no section text) that keep the notification a signal
to come and look rather than a delivery channel.

The transactional half against a real Postgres lives in
tests/integration/test_notification_outbox_tx.py — this module doubles the
psycopg surface the gateway uses, as tests/unit/test_postgres_job_gw.py does.
"""

from __future__ import annotations

import json
from collections.abc import AsyncIterator, Sequence
from contextlib import AbstractAsyncContextManager, asynccontextmanager
from datetime import UTC, datetime
from typing import Any
from uuid import UUID, uuid4

import pytest

from acolyte.domain.notification import REPORT_READY_KIND
from acolyte.gateway.postgres_job_gw import PostgresJobGateway

_NOTIFY_USER = UUID("33333333-3333-3333-3333-333333333333")


class _FakeCursor:
    def __init__(self, row: Sequence[Any] | None) -> None:
        self._row = row

    async def fetchone(self) -> Sequence[Any] | None:
        return self._row


class _Executed:
    def __init__(self, query: str, params: list[Any] | None, *, in_transaction: bool) -> None:
        self.query = query
        self.params = params
        self.in_transaction = in_transaction


class _FakeConnection:
    """Answers with one canned row per execute, in order, and records whether
    each statement ran while a transaction block was open."""

    def __init__(self, rows: list[Sequence[Any] | None]) -> None:
        self._rows = list(rows)
        self.executed: list[_Executed] = []
        self._depth = 0

    async def execute(self, query: str, params: list[Any] | None = None) -> _FakeCursor:
        self.executed.append(_Executed(query, params, in_transaction=self._depth > 0))
        row = self._rows.pop(0) if self._rows else None
        return _FakeCursor(row)

    def transaction(self) -> AbstractAsyncContextManager[None]:
        return self._tx()

    @asynccontextmanager
    async def _tx(self) -> AsyncIterator[None]:
        self._depth += 1
        try:
            yield
        finally:
            self._depth -= 1


class _FakePool:
    def __init__(self, conn: _FakeConnection) -> None:
        self._conn = conn
        self.connections_handed_out = 0

    def connection(self) -> AbstractAsyncContextManager[_FakeConnection]:
        self.connections_handed_out += 1
        return _conn_cm(self._conn)


@asynccontextmanager
async def _conn_cm(conn: _FakeConnection) -> AsyncIterator[_FakeConnection]:
    yield conn


def _outbox_statement(conn: _FakeConnection) -> _Executed:
    matches = [e for e in conn.executed if "notification_outbox" in e.query]
    assert len(matches) == 1, f"expected exactly one outbox INSERT, got {len(matches)}"
    return matches[0]


@pytest.mark.asyncio
async def test_complete_run_enqueues_the_notification_in_the_same_transaction() -> None:
    run_id = uuid4()
    report_id = uuid4()
    finished_at = datetime(2026, 8, 8, 9, 30, tzinfo=UTC)
    conn = _FakeConnection(rows=[(report_id, finished_at), None, None])
    pool = _FakePool(conn)
    gw = PostgresJobGateway(pool, notification_user_id=_NOTIFY_USER)  # type: ignore[arg-type]

    await gw.complete_run(run_id)

    # One connection, one transaction: a second connection would be a dual
    # write wearing an outbox's name.
    assert pool.connections_handed_out == 1
    outbox = _outbox_statement(conn)
    assert outbox.in_transaction is True
    assert all(e.in_transaction for e in conn.executed)


@pytest.mark.asyncio
async def test_outbox_row_is_keyed_by_run_and_points_at_the_report() -> None:
    run_id = uuid4()
    report_id = uuid4()
    finished_at = datetime(2026, 8, 8, 9, 30, tzinfo=UTC)
    conn = _FakeConnection(rows=[(report_id, finished_at), None, None])
    gw = PostgresJobGateway(_FakePool(conn), notification_user_id=_NOTIFY_USER)  # type: ignore[arg-type]

    await gw.complete_run(run_id)

    outbox = _outbox_statement(conn)
    assert outbox.params is not None
    dedupe_key, user_id, kind, payload_json, occurred_at = outbox.params
    assert dedupe_key == f"acolyte:{run_id}"
    assert user_id == _NOTIFY_USER
    assert kind == REPORT_READY_KIND
    assert json.loads(payload_json) == {
        "kind": REPORT_READY_KIND,
        "url": f"/acolyte/reports/{report_id}",
    }
    # Business time, taken from the row the completion just wrote — not a
    # second clock read that would record when the INSERT ran.
    assert occurred_at == finished_at


@pytest.mark.asyncio
async def test_payload_carries_no_report_content() -> None:
    conn = _FakeConnection(rows=[(uuid4(), datetime(2026, 8, 8, tzinfo=UTC)), None, None])
    gw = PostgresJobGateway(_FakePool(conn), notification_user_id=_NOTIFY_USER)  # type: ignore[arg-type]

    await gw.complete_run(uuid4())

    outbox = _outbox_statement(conn)
    assert outbox.params is not None
    payload = json.loads(outbox.params[3])
    assert set(payload) == {"kind", "url"}


@pytest.mark.asyncio
async def test_reenqueue_of_the_same_run_is_absorbed_by_the_unique_key() -> None:
    conn = _FakeConnection(rows=[(uuid4(), datetime(2026, 8, 8, tzinfo=UTC)), None, None])
    gw = PostgresJobGateway(_FakePool(conn), notification_user_id=_NOTIFY_USER)  # type: ignore[arg-type]

    await gw.complete_run(uuid4())

    # Without this the second completion of the same run aborts the whole
    # transaction on the UNIQUE violation and un-completes the run.
    assert "ON CONFLICT (dedupe_key) DO NOTHING" in _outbox_statement(conn).query


@pytest.mark.asyncio
async def test_no_notification_when_the_run_does_not_exist() -> None:
    conn = _FakeConnection(rows=[None])
    gw = PostgresJobGateway(_FakePool(conn), notification_user_id=_NOTIFY_USER)  # type: ignore[arg-type]

    await gw.complete_run(uuid4())

    assert [e for e in conn.executed if "notification_outbox" in e.query] == []


@pytest.mark.asyncio
async def test_notifications_off_is_an_explicit_decision_not_a_missing_argument() -> None:
    # `notification_user_id` is keyword-required: a caller cannot forget to
    # decide, and None means the composition root decided "off" (and logged it).
    with pytest.raises(TypeError):
        PostgresJobGateway(_FakePool(_FakeConnection(rows=[])))  # type: ignore[call-arg, arg-type]

    conn = _FakeConnection(rows=[(uuid4(), datetime(2026, 8, 8, tzinfo=UTC)), None])
    gw = PostgresJobGateway(_FakePool(conn), notification_user_id=None)  # type: ignore[arg-type]
    await gw.complete_run(uuid4())
    assert [e for e in conn.executed if "notification_outbox" in e.query] == []
