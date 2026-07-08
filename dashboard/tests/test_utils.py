"""Tests for utils.py's pure helper functions."""

from datetime import datetime, timedelta, timezone

import pandas as pd
import pytest

import utils


class TestTimeWindows:
    def test_known_windows_map_to_seconds(self) -> None:
        assert utils.TIME_WINDOWS["4h"] == 4 * 3600
        assert utils.TIME_WINDOWS["24h"] == 24 * 3600
        assert utils.TIME_WINDOWS["3d"] == 72 * 3600


class TestIntervalParams:
    def test_positive_window_passed_through(self) -> None:
        assert utils._interval_params(3600) == {"window_seconds": 3600}

    def test_negative_window_clamped_to_zero(self) -> None:
        assert utils._interval_params(-10) == {"window_seconds": 0}

    def test_zero_window(self) -> None:
        assert utils._interval_params(0) == {"window_seconds": 0}


class TestFilterFrameByWindow:
    def test_empty_dataframe_returned_unchanged(self) -> None:
        df = pd.DataFrame()
        result = utils.filter_frame_by_window(df, "timestamp", 3600)
        assert result.empty

    def test_missing_column_returned_unchanged(self) -> None:
        df = pd.DataFrame({"other": [1, 2, 3]})
        result = utils.filter_frame_by_window(df, "timestamp", 3600)
        assert result.equals(df)

    def test_filters_rows_outside_window(self, monkeypatch: pytest.MonkeyPatch) -> None:
        fixed_now = datetime(2026, 1, 1, 12, 0, 0, tzinfo=timezone.utc)
        monkeypatch.setattr(utils, "now_utc", lambda: fixed_now)

        df = pd.DataFrame(
            {
                "timestamp": [
                    fixed_now - timedelta(seconds=10),  # within 1h window
                    fixed_now - timedelta(hours=2),  # outside 1h window
                ],
                "value": [1, 2],
            }
        )

        result = utils.filter_frame_by_window(df, "timestamp", 3600)

        assert len(result) == 1
        assert result.iloc[0]["value"] == 1

    def test_unparseable_timestamps_are_dropped(self, monkeypatch: pytest.MonkeyPatch) -> None:
        fixed_now = datetime(2026, 1, 1, 12, 0, 0, tzinfo=timezone.utc)
        monkeypatch.setattr(utils, "now_utc", lambda: fixed_now)

        df = pd.DataFrame({"timestamp": ["not-a-date", fixed_now.isoformat()], "value": [1, 2]})

        result = utils.filter_frame_by_window(df, "timestamp", 3600)

        assert len(result) == 1
        assert result.iloc[0]["value"] == 2


class TestReadSecret:
    def test_reads_plain_env_var(self, monkeypatch: pytest.MonkeyPatch) -> None:
        monkeypatch.setenv("SOME_SECRET", "plain-value")
        monkeypatch.delenv("SOME_SECRET_FILE", raising=False)

        assert utils._read_secret("SOME_SECRET") == "plain-value"

    def test_file_variant_takes_precedence(self, monkeypatch: pytest.MonkeyPatch, tmp_path) -> None:
        secret_file = tmp_path / "secret.txt"
        secret_file.write_text("file-value\n")
        monkeypatch.setenv("SOME_SECRET", "plain-value")
        monkeypatch.setenv("SOME_SECRET_FILE", str(secret_file))

        assert utils._read_secret("SOME_SECRET") == "file-value"

    def test_returns_none_when_unset(self, monkeypatch: pytest.MonkeyPatch) -> None:
        monkeypatch.delenv("MISSING_SECRET", raising=False)
        monkeypatch.delenv("MISSING_SECRET_FILE", raising=False)

        assert utils._read_secret("MISSING_SECRET") is None


class TestBuildDbUri:
    def test_prefers_explicit_dsn(self, monkeypatch: pytest.MonkeyPatch) -> None:
        monkeypatch.setenv("RECAP_DB_DSN", "postgresql://explicit:dsn@host/db")

        assert utils._build_db_uri() == "postgresql://explicit:dsn@host/db"

    def test_builds_from_discrete_vars_when_no_dsn(self, monkeypatch: pytest.MonkeyPatch) -> None:
        monkeypatch.delenv("RECAP_DB_DSN", raising=False)
        monkeypatch.setenv("RECAP_DB_PASSWORD", "secret123")
        monkeypatch.setenv("RECAP_DB_HOST", "myhost")
        monkeypatch.setenv("RECAP_DB_PORT", "5555")
        monkeypatch.setenv("RECAP_DB_USER", "myuser")
        monkeypatch.setenv("RECAP_DB_NAME", "mydb")

        assert utils._build_db_uri() == "postgresql://myuser:secret123@myhost:5555/mydb"

    def test_raises_when_no_dsn_and_no_password(self, monkeypatch: pytest.MonkeyPatch) -> None:
        monkeypatch.delenv("RECAP_DB_DSN", raising=False)
        monkeypatch.delenv("RECAP_DB_PASSWORD", raising=False)
        monkeypatch.delenv("RECAP_DB_PASSWORD_FILE", raising=False)

        with pytest.raises(RuntimeError):
            utils._build_db_uri()
