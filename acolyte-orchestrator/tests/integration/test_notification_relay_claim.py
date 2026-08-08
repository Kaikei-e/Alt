"""The claim/finalise state machine, exercised against a real PostgreSQL.

The claim is one statement with a CTE, ``FOR UPDATE SKIP LOCKED`` and a lease
written into ``next_attempt_at``; none of that can be verified against a fake
cursor. Setup for this module is described in
tests/integration/test_notification_outbox_tx.py — same disposable database,
same always-rolled-back transaction.
"""

from __future__ import annotations

import json
import os
from collections.abc import AsyncIterator
from contextlib import AbstractAsyncContextManager, asynccontextmanager
from datetime import UTC, datetime, timedelta
from typing import Any
from uuid import UUID, uuid4

import psycopg
import pytest

from acolyte.gateway.postgres_notification_outbox_gw import PostgresNotificationOutboxGateway

_DSN = os.getenv("ACOLYTE_TEST_DB_DSN", "")

pytestmark = pytest.mark.skipif(
    not _DSN,
    reason="ACOLYTE_TEST_DB_DSN not set (needs a disposable Postgres with the acolyte schema)",
)


class _SingleConnectionPool:
    def __init__(self, conn: psycopg.AsyncConnection[Any]) -> None:
        self._conn = conn

    def connection(self) -> AbstractAsyncContextManager[psycopg.AsyncConnection[Any]]:
        return _conn_cm(self._conn)


@asynccontextmanager
async def _conn_cm(conn: psycopg.AsyncConnection[Any]) -> AsyncIterator[psycopg.AsyncConnection[Any]]:
    yield conn


class _RollbackError(Exception):
    pass


async def _insert_row(conn: psycopg.AsyncConnection[Any], dedupe_key: str, occurred_at: datetime) -> UUID:
    cur = await conn.execute(
        "INSERT INTO notification_outbox (dedupe_key, user_id, kind, payload, occurred_at) "
        "VALUES (%s, %s, %s, %s::jsonb, %s) RETURNING id",
        [
            dedupe_key,
            uuid4(),
            "acolyte_report_ready",
            json.dumps({"kind": "acolyte_report_ready", "url": "/acolyte/reports/x"}),
            occurred_at,
        ],
    )
    row = await cur.fetchone()
    assert row is not None
    return row[0]


async def _state_of(conn: psycopg.AsyncConnection[Any], notification_id: UUID) -> tuple[Any, ...]:
    cur = await conn.execute(
        "SELECT state, attempts, locked_by, last_error, forwarded_at, "
        "next_attempt_at > clock_timestamp() AS deferred "
        "FROM notification_outbox WHERE id = %s",
        [notification_id],
    )
    row = await cur.fetchone()
    assert row is not None
    return tuple(row)


@pytest.mark.asyncio
async def test_claim_leases_finalise_and_backlog_age() -> None:
    async with await psycopg.AsyncConnection.connect(_DSN, autocommit=False) as conn:
        gw = PostgresNotificationOutboxGateway(_SingleConnectionPool(conn))  # type: ignore[arg-type]
        occurred_at = datetime.now(UTC) - timedelta(seconds=90)

        with pytest.raises(_RollbackError):
            async with conn.transaction():
                first = await _insert_row(conn, f"acolyte:{uuid4()}", occurred_at)
                second = await _insert_row(conn, f"acolyte:{uuid4()}", occurred_at)

                claimed = await gw.claim_batch("relay-1", 10)
                assert {c.notification_id for c in claimed} == {first, second}
                assert all(c.attempts == 1 for c in claimed)
                assert all(c.payload["kind"] == "acolyte_report_ready" for c in claimed)

                state, attempts, locked_by, _, _, deferred = await _state_of(conn, first)
                assert (state, attempts, locked_by, deferred) == ("forwarding", 1, "relay-1", True)

                # The lease is the whole reclaim mechanism: a second relay must
                # not pick the same rows up while the first is still forwarding.
                assert await gw.claim_batch("relay-2", 10) == []

                await gw.mark_forwarded(first)
                state, _, _, last_error, forwarded_at, _ = await _state_of(conn, first)
                assert state == "forwarded"
                assert last_error is None
                assert forwarded_at is not None

                await gw.mark_failed(second, attempts=1, error="connection refused")
                state, attempts, _, last_error, _, deferred = await _state_of(conn, second)
                assert (state, attempts, last_error) == ("pending", 1, "connection refused")
                # Retry is scheduled, but full jitter may draw ~0s, so only the
                # state transition is asserted, not the delay.

                # A backlog exists (one pending row, business time 90s old).
                assert await gw.oldest_pending_age_seconds() >= 90.0

                await gw.mark_failed(second, attempts=8, error="410 gone")
                state, _, _, last_error, _, _ = await _state_of(conn, second)
                assert (state, last_error) == ("dead", "410 gone")

                # Terminal rows leave the claim query, so the gauge falls back
                # to zero instead of reporting a backlog nobody can drain.
                assert await gw.oldest_pending_age_seconds() == 0.0
                assert await gw.claim_batch("relay-1", 10) == []

                raise _RollbackError


@pytest.mark.asyncio
async def test_a_row_a_crashed_relay_left_behind_eventually_dies() -> None:
    async with await psycopg.AsyncConnection.connect(_DSN, autocommit=False) as conn:
        gw = PostgresNotificationOutboxGateway(_SingleConnectionPool(conn))  # type: ignore[arg-type]

        with pytest.raises(_RollbackError):
            async with conn.transaction():
                stranded = await _insert_row(conn, f"acolyte:{uuid4()}", datetime.now(UTC))
                # A relay that died mid-forward, eight times over: 'forwarding'
                # with an expired lease and no mark_failed ever reached.
                await conn.execute(
                    "UPDATE notification_outbox SET state = 'forwarding', attempts = 8, "
                    "next_attempt_at = clock_timestamp() - make_interval(secs => 1) WHERE id = %s",
                    [stranded],
                )

                assert await gw.claim_batch("relay-1", 10) == []

                state, _, _, last_error, _, _ = await _state_of(conn, stranded)
                assert state == "dead"
                assert last_error == "lease expired with attempts exhausted"

                raise _RollbackError
