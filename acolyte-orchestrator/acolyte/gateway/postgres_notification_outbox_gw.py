"""PostgreSQL notification outbox gateway — the relay's half of the table.

Claim, forward, finalise. The claim is a single statement in its own
transaction that is committed before the batch is handed back, so the outbound
RPC never runs with a row lock or a pooled connection held.
"""

from __future__ import annotations

from typing import TYPE_CHECKING, Any

import structlog

from acolyte.domain.notification import (
    MAX_ATTEMPTS,
    PendingNotification,
    backoff_delay_seconds,
    is_exhausted,
)

if TYPE_CHECKING:
    from uuid import UUID

    from psycopg_pool import AsyncConnectionPool

logger = structlog.get_logger(__name__)

# Long enough that an in-flight forward finishes first, short enough that a
# crashed relay's rows re-enter the claim query the same minute. The column
# doubles as the lease, so there is no separate reclaim sweeper to forget.
CLAIM_LEASE_SECONDS = 300.0

# last_error is diagnostic, not a log sink: a multi-megabyte upstream error
# body would otherwise be stored per attempt, per row.
_MAX_ERROR_CHARS = 1000

# mark_failed is what normally kills an exhausted row, and a relay that dies
# mid-forward never reaches it: the row's lease simply expires and it is
# claimed again, forever. This buries those before each claim, so the attempt
# ceiling holds on the crash path too.
_REAP_SQL = (
    "UPDATE notification_outbox SET state = 'dead', "
    "  last_error = COALESCE(last_error, 'lease expired with attempts exhausted') "
    "WHERE state = 'forwarding' AND attempts >= %s AND next_attempt_at <= clock_timestamp()"
)

_CLAIM_SQL = (
    "WITH due AS ("
    "  SELECT id FROM notification_outbox"
    "  WHERE state IN ('pending', 'forwarding')"
    "    AND next_attempt_at <= clock_timestamp()"
    "  ORDER BY next_attempt_at, id"
    "  LIMIT %s"
    "  FOR UPDATE SKIP LOCKED"
    ") "
    "UPDATE notification_outbox SET "
    "  state = 'forwarding', "
    "  attempts = notification_outbox.attempts + 1, "
    "  locked_by = %s, "
    "  next_attempt_at = clock_timestamp() + make_interval(secs => %s) "
    "FROM due WHERE notification_outbox.id = due.id "
    "RETURNING notification_outbox.id, notification_outbox.dedupe_key, notification_outbox.user_id, "
    "  notification_outbox.kind, notification_outbox.payload, notification_outbox.occurred_at, "
    "  notification_outbox.attempts"
)


class PostgresNotificationOutboxGateway:
    """NotificationOutboxPort implementation over acolyte-db."""

    def __init__(self, pool: AsyncConnectionPool) -> None:
        self._pool = pool

    async def claim_batch(self, worker_id: str, limit: int) -> list[PendingNotification]:
        # The transaction and the pooled connection are both released here, so
        # the caller's outbound RPC runs with neither held.
        async with self._pool.connection() as conn, conn.transaction():
            await conn.execute(_REAP_SQL, [MAX_ATTEMPTS])
            cur = await conn.execute(_CLAIM_SQL, [limit, worker_id, CLAIM_LEASE_SECONDS])
            rows = await cur.fetchall()
        return [
            PendingNotification(
                notification_id=r[0],
                dedupe_key=r[1],
                user_id=r[2],
                kind=r[3],
                payload=dict(r[4]),
                occurred_at=r[5],
                attempts=r[6],
            )
            for r in rows
        ]

    async def mark_forwarded(self, notification_id: UUID) -> None:
        async with self._pool.connection() as conn:
            await conn.execute(
                "UPDATE notification_outbox SET state = 'forwarded', "
                "forwarded_at = clock_timestamp(), last_error = NULL WHERE id = %s",
                [notification_id],
            )

    async def mark_failed(self, notification_id: UUID, attempts: int, error: str) -> None:
        """Reschedule with full jitter, or declare the row dead once exhausted."""
        reason = error[:_MAX_ERROR_CHARS]
        params: list[Any]
        if is_exhausted(attempts):
            query = "UPDATE notification_outbox SET state = 'dead', last_error = %s WHERE id = %s"
            params = [reason, notification_id]
            logger.error(
                "notification_outbox_row_dead",
                notification_id=str(notification_id),
                attempts=attempts,
                last_error=reason,
            )
        else:
            query = (
                "UPDATE notification_outbox SET state = 'pending', "
                "next_attempt_at = clock_timestamp() + make_interval(secs => %s), "
                "last_error = %s WHERE id = %s"
            )
            params = [backoff_delay_seconds(attempts - 1), reason, notification_id]
        async with self._pool.connection() as conn:
            await conn.execute(query, params)

    async def oldest_pending_age_seconds(self) -> float:
        """Backlog age measured from business time, so the gauge answers "how
        stale is the oldest thing a user has not been told about", not "how
        long has a row sat here"."""
        async with self._pool.connection() as conn:
            cur = await conn.execute(
                "SELECT COALESCE(EXTRACT(EPOCH FROM (clock_timestamp() - MIN(occurred_at))), 0) "
                "FROM notification_outbox WHERE state IN ('pending', 'forwarding')",
            )
            row = await cur.fetchone()
        return float(row[0]) if row is not None else 0.0
