"""Unit tests for CursorManager pagination and cursor poisoning protection."""

from datetime import UTC, datetime, timedelta

from tag_generator.cursor_manager import CursorManager

_FALLBACK_SENTINEL_ID = "ffffffff-ffff-ffff-ffff-ffffffffffff"
_FORWARD_INITIAL_ID = "00000000-0000-0000-0000-000000000000"


class TestCursorManagerInitialPosition:
    """Tests for CursorManager.get_initial_cursor_position."""

    def test_first_run_returns_fallback_cursor(self):
        """When no cursor has been recorded yet, returns current UTC time and sentinel ID."""
        manager = CursorManager()

        created_at, article_id = manager.get_initial_cursor_position()

        assert article_id == _FALLBACK_SENTINEL_ID
        parsed = datetime.fromisoformat(created_at)
        assert abs((datetime.now(UTC) - parsed).total_seconds()) < 5.0

    def test_rejects_cursor_more_than_one_hour_in_future_as_poisoned(self):
        """Cursor > 3600s in the future is treated as poisoned and falls back to current time."""
        manager = CursorManager()
        future_time = (datetime.now(UTC) + timedelta(hours=2)).isoformat()
        manager.update_cursor_position(future_time, "art-future")

        created_at, article_id = manager.get_initial_cursor_position()

        assert article_id == _FALLBACK_SENTINEL_ID
        assert created_at != future_time

    def test_rejects_cursor_more_than_365_days_old_as_poisoned(self):
        """Cursor older than 365 days is treated as poisoned and falls back to current time."""
        manager = CursorManager()
        old_time = (datetime.now(UTC) - timedelta(days=366)).isoformat()
        manager.update_cursor_position(old_time, "art-ancient")

        created_at, article_id = manager.get_initial_cursor_position()

        assert article_id == _FALLBACK_SENTINEL_ID
        assert created_at != old_time

    def test_preserves_valid_cursor_with_z_suffix(self):
        """Valid recent ISO timestamp ending with 'Z' is preserved as-is."""
        manager = CursorManager()
        valid_z = (datetime.now(UTC) - timedelta(hours=6)).strftime("%Y-%m-%dT%H:%M:%SZ")
        manager.update_cursor_position(valid_z, "art-valid-z")

        created_at, article_id = manager.get_initial_cursor_position()

        assert created_at == valid_z
        assert article_id == "art-valid-z"

    def test_preserves_valid_timezone_naive_cursor_assuming_utc(self):
        """Valid timezone-naive ISO timestamp is interpreted as UTC and preserved."""
        manager = CursorManager()
        valid_naive = (datetime.now(UTC) - timedelta(days=30)).replace(tzinfo=None).isoformat()
        manager.update_cursor_position(valid_naive, "art-valid-naive")

        created_at, article_id = manager.get_initial_cursor_position()

        assert created_at == valid_naive
        assert article_id == "art-valid-naive"

    def test_falls_back_on_malformed_cursor_timestamp(self):
        """Invalid timestamp string catches ValueError and falls back to current time."""
        manager = CursorManager()
        manager.update_cursor_position("not-an-iso-timestamp", "art-corrupt")

        created_at, article_id = manager.get_initial_cursor_position()

        assert article_id == _FALLBACK_SENTINEL_ID
        parsed = datetime.fromisoformat(created_at)
        assert abs((datetime.now(UTC) - parsed).total_seconds()) < 5.0


class TestCursorManagerForwardAndHelperPositions:
    """Tests for forward, newest, and recovery cursor methods."""

    def test_get_forward_cursor_position_initializes_on_first_call(self):
        """First call to get_forward_cursor_position initializes state with zero UUID."""
        manager = CursorManager()
        assert manager.forward_cursor_created_at is None
        assert manager.forward_cursor_id is None

        created_at, article_id = manager.get_forward_cursor_position(None)

        assert article_id == _FORWARD_INITIAL_ID
        assert manager.forward_cursor_created_at == created_at
        assert manager.forward_cursor_id == _FORWARD_INITIAL_ID

    def test_update_and_get_forward_cursor_position_preserves_updated_values(self):
        """Subsequent calls to get_forward_cursor_position return the updated cursor."""
        manager = CursorManager()
        manager.update_forward_cursor_position("2026-10-20T08:30:00Z", "art-fwd-123")

        created_at, article_id = manager.get_forward_cursor_position(None)

        assert created_at == "2026-10-20T08:30:00Z"
        assert article_id == "art-fwd-123"

    def test_newest_and_recovery_cursor_positions_return_fallback_sentinel(self):
        """Newest-article and recovery cursor positions both return current time with sentinel ID."""
        manager = CursorManager()

        newest_ts, newest_id = manager.get_newest_article_cursor_position()
        recovery_ts, recovery_id = manager.get_recovery_cursor_position()

        assert newest_id == _FALLBACK_SENTINEL_ID
        assert recovery_id == _FALLBACK_SENTINEL_ID
        assert datetime.fromisoformat(newest_ts).tzinfo is not None
        assert datetime.fromisoformat(recovery_ts).tzinfo is not None
