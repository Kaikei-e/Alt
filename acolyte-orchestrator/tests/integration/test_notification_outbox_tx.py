"""The outbox row and the run completion share one commit — proved against a
real PostgreSQL, because that is the only place the claim can be proved.

Skipped unless ``ACOLYTE_TEST_DB_DSN`` points at a database with the
acolyte-migration-atlas schema applied (``notification_outbox`` included).
Run it against a disposable instance, never acolyte-db itself:

    docker run --rm -d --name acolyte-test-db -p 15439:5432 \
      -e POSTGRES_PASSWORD=test -e POSTGRES_DB=acolyte postgres:18-alpine
    cat acolyte-migration-atlas/migrations/*.sql | \
      docker exec -i acolyte-test-db psql -U postgres -d acolyte
    ACOLYTE_TEST_DB_DSN=postgresql://postgres:test@127.0.0.1:15439/acolyte \
      uv run pytest tests/integration -q --no-cov

Everything the test writes happens inside one transaction that is always rolled
back, so a run leaves no rows behind even when it fails.
"""

from __future__ import annotations

import os
from collections.abc import AsyncIterator
from contextlib import AbstractAsyncContextManager, asynccontextmanager
from typing import Any
from uuid import UUID, uuid4

import psycopg
import pytest

from acolyte.gateway.postgres_job_gw import PostgresJobGateway

_DSN = os.getenv("ACOLYTE_TEST_DB_DSN", "")

pytestmark = pytest.mark.skipif(
    not _DSN,
    reason="ACOLYTE_TEST_DB_DSN not set (needs a disposable Postgres with the acolyte schema)",
)

_NOTIFY_USER = uuid4()


class _SingleConnectionPool:
    """Hands the gateway the same live connection the test already holds open,
    so the gateway's ``conn.transaction()`` nests as a savepoint inside the
    test's outer transaction instead of committing behind its back."""

    def __init__(self, conn: psycopg.AsyncConnection[Any]) -> None:
        self._conn = conn

    def connection(self) -> AbstractAsyncContextManager[psycopg.AsyncConnection[Any]]:
        return _conn_cm(self._conn)


@asynccontextmanager
async def _conn_cm(conn: psycopg.AsyncConnection[Any]) -> AsyncIterator[psycopg.AsyncConnection[Any]]:
    yield conn


class _RollbackError(Exception):
    """Sentinel that unwinds the outer transaction once the assertions ran."""


async def _row_counts(conn: psycopg.AsyncConnection[Any], run_id: UUID, dedupe_key: str) -> tuple[int, int]:
    run_cur = await conn.execute(
        "SELECT count(*) FROM report_runs WHERE run_id = %s AND run_status = 'succeeded'",
        [run_id],
    )
    run_row = await run_cur.fetchone()
    outbox_cur = await conn.execute(
        "SELECT count(*) FROM notification_outbox WHERE dedupe_key = %s",
        [dedupe_key],
    )
    outbox_row = await outbox_cur.fetchone()
    assert run_row is not None
    assert outbox_row is not None
    return run_row[0], outbox_row[0]


@pytest.mark.asyncio
async def test_completion_and_notification_commit_or_vanish_together() -> None:
    report_id = uuid4()
    run_id = uuid4()
    dedupe_key = f"acolyte:{run_id}"

    async with await psycopg.AsyncConnection.connect(_DSN, autocommit=False) as conn:
        gw = PostgresJobGateway(_SingleConnectionPool(conn), notification_user_id=_NOTIFY_USER)  # type: ignore[arg-type]

        with pytest.raises(_RollbackError):
            async with conn.transaction():
                await conn.execute(
                    "INSERT INTO reports (report_id, title, report_type) VALUES (%s, %s, %s)",
                    [report_id, "tx proof", "weekly_briefing"],
                )
                await conn.execute(
                    "INSERT INTO report_runs (run_id, report_id, target_version_no, run_status) "
                    "VALUES (%s, %s, %s, 'running')",
                    [run_id, report_id, 1],
                )

                await gw.complete_run(run_id)

                completed, enqueued = await _row_counts(conn, run_id, dedupe_key)
                assert completed == 1
                assert enqueued == 1

                payload_cur = await conn.execute(
                    "SELECT payload, occurred_at, state, attempts FROM notification_outbox WHERE dedupe_key = %s",
                    [dedupe_key],
                )
                payload_row = await payload_cur.fetchone()
                assert payload_row is not None
                payload, occurred_at, state, attempts = payload_row
                assert payload == {"kind": "acolyte_report_ready", "url": f"/acolyte/reports/{report_id}"}
                assert state == "pending"
                assert attempts == 0

                finished_cur = await conn.execute(
                    "SELECT finished_at FROM report_runs WHERE run_id = %s",
                    [run_id],
                )
                finished_row = await finished_cur.fetchone()
                assert finished_row is not None
                # Business time, not a second clock read at INSERT time.
                assert occurred_at == finished_row[0]

                raise _RollbackError

        completed, enqueued = await _row_counts(conn, run_id, dedupe_key)
        assert completed == 0
        assert enqueued == 0
