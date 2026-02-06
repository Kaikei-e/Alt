"""Database lock utilities for preventing duplicate scheduler execution."""

from __future__ import annotations

from sqlalchemy import text
from sqlalchemy.ext.asyncio import AsyncSession

# Fixed lock ID for the learning scheduler
# This value should not conflict with other advisory locks in the system
SCHEDULER_LOCK_ID = 1234567890


async def acquire_scheduler_lock(session: AsyncSession) -> bool:
    """Acquire an advisory lock for the learning scheduler (non-blocking).

    Returns True if the lock was acquired, False if another process already holds it.
    The lock is automatically released when the database connection is closed.
    """
    result = await session.execute(
        text("SELECT pg_try_advisory_lock(:lock_id)"),
        {"lock_id": SCHEDULER_LOCK_ID},
    )
    return result.scalar() is True


async def release_scheduler_lock(session: AsyncSession) -> bool:
    """Release the advisory lock for the learning scheduler.

    Note: The lock is automatically released when the database connection
    is closed, so this is mainly useful for explicit cleanup.
    """
    result = await session.execute(
        text("SELECT pg_advisory_unlock(:lock_id)"),
        {"lock_id": SCHEDULER_LOCK_ID},
    )
    return result.scalar() is True
