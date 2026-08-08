"""Unit tests for the outbox claim/finalise SQL.

The claim must commit before the relay makes its outbound call — holding a
transaction open across a network hop to another service is how one slow
DataHub turns into an acolyte-db connection famine.
"""

from __future__ import annotations

from collections.abc import AsyncIterator, Sequence
from contextlib import AbstractAsyncContextManager, asynccontextmanager
from datetime import UTC, datetime
from typing import Any
from uuid import UUID, uuid4

import pytest

from acolyte.domain.notification import MAX_ATTEMPTS
from acolyte.gateway.postgres_notification_outbox_gw import PostgresNotificationOutboxGateway

_USER = UUID("55555555-5555-5555-5555-555555555555")


class _FakeCursor:
    def __init__(self, rows: Sequence[Sequence[Any]] | None) -> None:
        self._rows = rows

    async def fetchone(self) -> Sequence[Any] | None:
        if not self._rows:
            return None
        return self._rows[0]

    async def fetchall(self) -> Sequence[Sequence[Any]]:
        return self._rows or []


class _FakeConnection:
    def __init__(self, results: list[Sequence[Sequence[Any]] | None]) -> None:
        self._results = list(results)
        self.executed: list[tuple[str, list[Any] | None]] = []
        self.open_transactions = 0

    async def execute(self, query: str, params: list[Any] | None = None) -> _FakeCursor:
        self.executed.append((query, params))
        result = self._results.pop(0) if self._results else None
        return _FakeCursor(result)

    def transaction(self) -> AbstractAsyncContextManager[None]:
        return self._tx()

    @asynccontextmanager
    async def _tx(self) -> AsyncIterator[None]:
        self.open_transactions += 1
        try:
            yield
        finally:
            self.open_transactions -= 1


class _FakePool:
    def __init__(self, conn: _FakeConnection) -> None:
        self._conn = conn
        self.checked_out = 0

    def connection(self) -> AbstractAsyncContextManager[_FakeConnection]:
        return self._cm()

    @asynccontextmanager
    async def _cm(self) -> AsyncIterator[_FakeConnection]:
        self.checked_out += 1
        try:
            yield self._conn
        finally:
            self.checked_out -= 1


def _gw(conn: _FakeConnection, pool: _FakePool | None = None) -> PostgresNotificationOutboxGateway:
    return PostgresNotificationOutboxGateway(pool or _FakePool(conn))  # type: ignore[arg-type]


@pytest.mark.asyncio
async def test_claim_skips_locked_rows_and_leases_them() -> None:
    row = (
        uuid4(),
        "acolyte:abc",
        _USER,
        "acolyte_report_ready",
        {"kind": "acolyte_report_ready", "url": "/acolyte/reports/x"},
        datetime(2026, 8, 8, tzinfo=UTC),
        1,
    )
    conn = _FakeConnection([None, [row]])

    claimed = await _gw(conn).claim_batch("acolyte-1", 10)

    query, params = conn.executed[1]
    assert "FOR UPDATE SKIP LOCKED" in query
    assert "state IN ('pending', 'forwarding')" in query
    assert "next_attempt_at <= clock_timestamp()" in query
    assert "state = 'forwarding'" in query
    assert "attempts = notification_outbox.attempts + 1" in query
    assert params is not None
    assert "acolyte-1" in params
    assert 10 in params

    assert len(claimed) == 1
    assert claimed[0].dedupe_key == "acolyte:abc"
    assert claimed[0].user_id == _USER
    assert claimed[0].attempts == 1


@pytest.mark.asyncio
async def test_claim_first_buries_rows_whose_lease_outlived_their_attempts() -> None:
    conn = _FakeConnection([None, []])

    await _gw(conn).claim_batch("acolyte-1", 10)

    # Death only happens through mark_failed, which a relay that crashes
    # mid-forward never reaches — without this the row is re-claimed forever.
    reap_query, reap_params = conn.executed[0]
    assert "state = 'dead'" in reap_query
    assert "state = 'forwarding'" in reap_query
    assert "next_attempt_at <= clock_timestamp()" in reap_query
    assert reap_params == [MAX_ATTEMPTS]


@pytest.mark.asyncio
async def test_claim_releases_its_transaction_before_returning() -> None:
    conn = _FakeConnection([None, []])
    pool = _FakePool(conn)

    await _gw(conn, pool).claim_batch("acolyte-1", 10)

    # Both the transaction and the pooled connection are gone by the time the
    # caller gets the batch, so the outbound RPC cannot run inside either.
    assert conn.open_transactions == 0
    assert pool.checked_out == 0


@pytest.mark.asyncio
async def test_mark_forwarded_is_terminal_and_stamps_when() -> None:
    conn = _FakeConnection([None])
    notification_id = uuid4()

    await _gw(conn).mark_forwarded(notification_id)

    query, params = conn.executed[0]
    assert "state = 'forwarded'" in query
    assert "forwarded_at = clock_timestamp()" in query
    assert params == [notification_id]


@pytest.mark.asyncio
async def test_a_retryable_failure_goes_back_to_pending_with_a_jittered_delay() -> None:
    conn = _FakeConnection([None])
    notification_id = uuid4()

    await _gw(conn).mark_failed(notification_id, attempts=1, error="connection refused")

    query, params = conn.executed[0]
    assert "state = 'pending'" in query
    assert "next_attempt_at = clock_timestamp() + make_interval(secs => %s)" in query
    assert params is not None
    delay = params[0]
    assert isinstance(delay, float)
    # attempt 0 (first failure) → full jitter over [0, base=10s]
    assert 0.0 <= delay <= 10.0
    assert params[1] == "connection refused"
    assert params[2] == notification_id


@pytest.mark.asyncio
async def test_an_exhausted_row_is_declared_dead_instead_of_retried_forever() -> None:
    conn = _FakeConnection([None])
    notification_id = uuid4()

    await _gw(conn).mark_failed(notification_id, attempts=MAX_ATTEMPTS, error="410 gone")

    query, params = conn.executed[0]
    assert "state = 'dead'" in query
    assert "make_interval" not in query
    assert params == ["410 gone", notification_id]


@pytest.mark.asyncio
async def test_error_text_is_truncated_so_one_row_cannot_bloat_the_table() -> None:
    conn = _FakeConnection([None])

    await _gw(conn).mark_failed(uuid4(), attempts=1, error="x" * 5000)

    _, params = conn.executed[0]
    assert params is not None
    assert len(params[1]) <= 1000


@pytest.mark.asyncio
async def test_oldest_pending_age_measures_business_time_and_is_zero_when_idle() -> None:
    conn = _FakeConnection([[(0.0,)]])

    age = await _gw(conn).oldest_pending_age_seconds()

    query, _ = conn.executed[0]
    assert "MIN(occurred_at)" in query
    assert "state IN ('pending', 'forwarding')" in query
    assert age == 0.0


@pytest.mark.asyncio
async def test_oldest_pending_age_returns_the_backlog_age() -> None:
    conn = _FakeConnection([[(123.5,)]])
    assert await _gw(conn).oldest_pending_age_seconds() == 123.5
