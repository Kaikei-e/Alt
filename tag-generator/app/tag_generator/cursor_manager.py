"""Cursor position management for article pagination."""

from datetime import UTC, datetime
from typing import Any

import structlog

logger = structlog.get_logger(__name__)


class CursorManager:
    """Manages cursor positions for article pagination."""

    def __init__(self) -> None:
        """Initialize cursor manager."""
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
                    return self._get_fallback_cursor_position()
                else:
                    # Continue from where we left off
                    last_created_at = self.last_processed_created_at
                    last_id = self.last_processed_id
                    logger.info(f"Continuing article processing from cursor: {last_created_at}, ID: {last_id}")
                    return last_created_at, last_id

            except (ValueError, TypeError) as e:
                logger.warning(f"Invalid cursor timestamp format: {self.last_processed_created_at}, error: {e}")
                logger.warning("Switching to newest article start mode due to invalid format")
                return self._get_fallback_cursor_position()
        else:
            # First run - start from current time (API mode)
            logger.info("First run - starting from current time")
            return self._get_fallback_cursor_position()

    def _get_fallback_cursor_position(self) -> tuple[str, str]:
        """Get a fallback cursor position using current time (API mode)."""
        current_time = datetime.now(UTC).isoformat()
        logger.info("Using current time as cursor position (API mode)", cursor=current_time)
        return current_time, "ffffffff-ffff-ffff-ffff-ffffffffffff"

    def get_newest_article_cursor_position(self) -> tuple[str, str]:
        """Get cursor position starting from the newest article.

        In API mode, uses current time as a fallback since there is no direct DB access.
        """
        return self._get_fallback_cursor_position()

    def get_recovery_cursor_position(self) -> tuple[str, str]:
        """Get cursor position for recovery mode.

        In API mode, uses current time as a fallback since there is no direct DB access.
        """
        return self._get_fallback_cursor_position()

    def get_forward_cursor_position(self, conn: Any) -> tuple[str, str]:
        """Get the cursor for forward processing starting point."""
        if self.forward_cursor_created_at and self.forward_cursor_id:
            return self.forward_cursor_created_at, self.forward_cursor_id

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
