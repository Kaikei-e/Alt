"""Cursor position management for article pagination."""

from datetime import UTC, datetime, timedelta
from typing import TYPE_CHECKING, cast

import structlog
from psycopg2.extensions import connection as Connection

logger = structlog.get_logger(__name__)

if TYPE_CHECKING:
    from tag_generator.database import DatabaseManager


class CursorManager:
    """Manages cursor positions for article pagination."""

    def __init__(self, database_manager: "DatabaseManager"):
        """Initialize cursor manager with database manager."""
        self.database_manager = database_manager

        # Persistent cursor position for pagination between cycles
        self.last_processed_created_at: str | None = None
        self.last_processed_id: str | None = None
        self.forward_cursor_created_at: str | None = None
        self.forward_cursor_id: str | None = None

    def get_initial_cursor_position(self) -> tuple[str, str]:
        """Get initial cursor position for pagination, starting from newest articles."""
        if self.last_processed_created_at and self.last_processed_id:
            # Check for cursor poisoning (timestamp in the future or too old)
            try:
                # Handle timezone-aware and timezone-naive timestamps
                cursor_str = self.last_processed_created_at
                if cursor_str.endswith("Z"):
                    cursor_str = cursor_str.replace("Z", "+00:00")

                cursor_time = datetime.fromisoformat(cursor_str)

                # If timezone-naive, assume UTC
                if cursor_time.tzinfo is None:
                    cursor_time = cursor_time.replace(tzinfo=UTC)

                current_time = datetime.now(UTC)
                time_diff = cursor_time - current_time

                # Check for various cursor poisoning scenarios
                cursor_is_poisoned = False
                reason = ""

                if time_diff.total_seconds() > 3600:  # More than 1 hour in future
                    cursor_is_poisoned = True
                    reason = f"cursor {time_diff.total_seconds() / 3600:.1f} hours in future"
                # Note: Backfill mode may process articles older than 30 days, so use 365 days threshold
                elif time_diff.total_seconds() < -86400 * 365:  # More than 365 days old
                    cursor_is_poisoned = True
                    reason = f"cursor {abs(time_diff.total_seconds()) / 86400:.1f} days old"

                if cursor_is_poisoned:
                    logger.warning(f"Detected cursor poisoning: {reason}")
                    logger.warning("Switching to newest article start mode")
                    return self.get_newest_article_cursor_position()
                else:
                    # Continue from where we left off
                    last_created_at = self.last_processed_created_at
                    last_id = self.last_processed_id
                    logger.info(f"Continuing article processing from cursor: {last_created_at}, ID: {last_id}")
                    return last_created_at, last_id

            except (ValueError, TypeError) as e:
                logger.warning(f"Invalid cursor timestamp format: {self.last_processed_created_at}, error: {e}")
                logger.warning("Switching to newest article start mode due to invalid format")
                return self.get_newest_article_cursor_position()
        else:
            # First run - start from newest article
            logger.info("First run - starting from newest article")
            return self.get_newest_article_cursor_position()

    def get_newest_article_cursor_position(self) -> tuple[str, str]:
        """Get cursor position starting from the newest article (regardless of tag status)."""
        try:
            with self.database_manager.get_connection() as conn:
                # Get the newest article (tagged or untagged)
                query = """
                    SELECT
                        a.id::text AS id,
                        a.created_at
                    FROM articles a
                    ORDER BY a.created_at DESC, a.id DESC
                    LIMIT 1
                """

                with conn.cursor() as cursor:
                    cursor.execute(query)
                    result = cursor.fetchone()

                    if result:
                        # Start from the newest article
                        newest_time = result[1]
                        newest_id = result[0]
                        if isinstance(newest_time, str):
                            start_time = newest_time
                        else:
                            start_time = newest_time.isoformat()

                        logger.info(f"Starting from newest article at {start_time}, ID: {newest_id}")
                        return start_time, newest_id
                    else:
                        # No articles found, start from current time
                        current_time = datetime.now(UTC).isoformat()
                        logger.info(f"No articles found, starting from current time: {current_time}")
                        return current_time, "ffffffff-ffff-ffff-ffff-ffffffffffff"

        except Exception as e:
            logger.error(f"Failed to determine newest article cursor position: {e}")
            # Fallback to current time
            current_time = datetime.now(UTC).isoformat()
            logger.warning(f"Using fallback cursor: {current_time}")
            return current_time, "ffffffff-ffff-ffff-ffff-ffffffffffff"

    def get_recovery_cursor_position(self) -> tuple[str, str]:
        """Get cursor position for recovery mode - prioritizes untagged articles."""
        try:
            with self.database_manager.get_connection() as conn:
                # Try to find the most recent untagged article
                query = """
                    SELECT
                        a.id::text AS id,
                        a.created_at
                    FROM articles a
                    LEFT JOIN article_tags at ON a.id = at.article_id
                    WHERE at.article_id IS NULL
                    ORDER BY a.created_at DESC, a.id DESC
                    LIMIT 1
                """

                with conn.cursor() as cursor:
                    cursor.execute(query)
                    result = cursor.fetchone()

                    if result:
                        # Start from just after the most recent untagged article
                        most_recent_untagged_time = result[1]
                        if isinstance(most_recent_untagged_time, str):
                            start_time = most_recent_untagged_time
                        else:
                            start_time = most_recent_untagged_time.isoformat()

                        # Add a small buffer to ensure we catch this article
                        # Handle timezone-aware and timezone-naive timestamps
                        start_time_str = start_time
                        if start_time_str.endswith("Z"):
                            start_time_str = start_time_str.replace("Z", "+00:00")

                        start_time_dt = datetime.fromisoformat(start_time_str)

                        # If timezone-naive, assume UTC
                        if start_time_dt.tzinfo is None:
                            start_time_dt = start_time_dt.replace(tzinfo=UTC)

                        start_time_dt += timedelta(microseconds=1)
                        start_time = start_time_dt.isoformat()

                        logger.info(f"Recovery mode: Starting from most recent untagged article at {start_time}")
                        return start_time, "ffffffff-ffff-ffff-ffff-ffffffffffff"
                    else:
                        # No untagged articles found, start from a reasonable past date
                        past_date = datetime.now(UTC) - timedelta(days=7)  # Look back 7 days
                        start_time = past_date.isoformat()
                        logger.info(f"Recovery mode: No untagged articles found, starting from {start_time}")
                        return start_time, "ffffffff-ffff-ffff-ffff-ffffffffffff"

        except Exception as e:
            logger.error(f"Failed to determine recovery cursor position: {e}")
            # Fallback to a reasonable past date
            past_date = datetime.now(UTC) - timedelta(days=1)
            start_time = past_date.isoformat()
            logger.warning(f"Using fallback recovery cursor: {start_time}")
            return start_time, "ffffffff-ffff-ffff-ffff-ffffffffffff"

    def get_forward_cursor_position(self, conn: Connection) -> tuple[str, str]:
        """Get the cursor for forward processing starting point."""
        if self.forward_cursor_created_at and self.forward_cursor_id:
            return self.forward_cursor_created_at, self.forward_cursor_id

        try:
            with conn.cursor() as cursor:
                cursor.execute(
                    """
                    SELECT a.created_at, a.id::text
                    FROM articles a
                    JOIN article_tags at ON a.id = at.article_id
                    ORDER BY a.created_at DESC, a.id DESC
                    LIMIT 1
                    """
                )
                result = cursor.fetchone()
                if result:
                    created_at = result[0]
                    created_at_str = created_at if isinstance(created_at, str) else created_at.isoformat()
                    self.forward_cursor_created_at = created_at_str
                    self.forward_cursor_id = result[1]
                    return created_at_str, cast(str, result[1])
        except Exception as exc:
            logger.warning("Failed to derive forward cursor from tags", error=str(exc))

        from datetime import UTC, datetime

        fallback_time = datetime.now(UTC).isoformat()
        self.forward_cursor_created_at = fallback_time
        self.forward_cursor_id = "00000000-0000-0000-0000-000000000000"
        return fallback_time, "00000000-0000-0000-0000-000000000000"

    def update_cursor_position(self, created_at: str, article_id: str) -> None:
        """Update the last processed cursor position."""
        self.last_processed_created_at = created_at
        self.last_processed_id = article_id

    def update_forward_cursor_position(self, created_at: str, article_id: str) -> None:
        """Update the forward cursor position."""
        self.forward_cursor_created_at = created_at
        self.forward_cursor_id = article_id
