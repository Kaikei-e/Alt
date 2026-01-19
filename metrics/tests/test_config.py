"""config.py のテスト"""

from __future__ import annotations

import os
from pathlib import Path
from tempfile import NamedTemporaryFile

import pytest

from alt_metrics.config import (
    AppConfig,
    ClickHouseConfig,
    HealthThresholds,
    ReportConfig,
)


class TestHealthThresholds:
    """HealthThresholds のテスト"""

    def test_default_values(self) -> None:
        """デフォルト値が正しく設定される"""
        t = HealthThresholds()
        assert t.error_rate_critical == 10.0
        assert t.error_rate_high == 5.0
        assert t.error_rate_warning == 1.0
        assert t.error_rate_minor == 0.5
        assert t.latency_critical_ms == 10000
        assert t.latency_high_ms == 5000
        assert t.latency_warning_ms == 1000
        assert t.latency_minor_ms == 500
        assert t.log_gap_critical_min == 10
        assert t.log_gap_warning_min == 5
        assert t.slo_error_rate_threshold == 1.0

    def test_from_env_with_no_env_vars(self, monkeypatch: pytest.MonkeyPatch) -> None:
        """環境変数がない場合はデフォルト値を使用"""
        # 環境変数をクリア
        for key in list(os.environ.keys()):
            if key.startswith("METRICS_THRESHOLD_"):
                monkeypatch.delenv(key, raising=False)

        t = HealthThresholds.from_env()
        assert t.error_rate_critical == 10.0

    def test_from_env_with_custom_values(self, monkeypatch: pytest.MonkeyPatch) -> None:
        """環境変数からカスタム値を読み込む"""
        monkeypatch.setenv("METRICS_THRESHOLD_ERROR_RATE_CRITICAL", "20.0")
        monkeypatch.setenv("METRICS_THRESHOLD_LATENCY_CRITICAL_MS", "15000")

        t = HealthThresholds.from_env()
        assert t.error_rate_critical == 20.0
        assert t.latency_critical_ms == 15000

    def test_from_env_ignores_invalid_values(self, monkeypatch: pytest.MonkeyPatch) -> None:
        """不正な値は無視してデフォルト値を使用"""
        monkeypatch.setenv("METRICS_THRESHOLD_ERROR_RATE_CRITICAL", "not_a_number")

        t = HealthThresholds.from_env()
        assert t.error_rate_critical == 10.0


class TestClickHouseConfig:
    """ClickHouseConfig のテスト"""

    def test_default_values(self) -> None:
        """デフォルト値が正しく設定される"""
        c = ClickHouseConfig()
        assert c.host == "localhost"
        assert c.port == 8123
        assert c.user == "default"
        assert c.password == ""
        assert c.database == "rask_logs"

    def test_from_env_with_no_env_vars(self, monkeypatch: pytest.MonkeyPatch) -> None:
        """環境変数がない場合はデフォルト値を使用"""
        for key in list(os.environ.keys()):
            if key.startswith("APP_CLICKHOUSE_"):
                monkeypatch.delenv(key, raising=False)

        c = ClickHouseConfig.from_env()
        assert c.host == "localhost"
        assert c.port == 8123

    def test_from_env_with_custom_values(self, monkeypatch: pytest.MonkeyPatch) -> None:
        """環境変数からカスタム値を読み込む"""
        monkeypatch.setenv("APP_CLICKHOUSE_HOST", "clickhouse.example.com")
        monkeypatch.setenv("APP_CLICKHOUSE_PORT", "9000")
        monkeypatch.setenv("APP_CLICKHOUSE_USER", "admin")
        monkeypatch.setenv("APP_CLICKHOUSE_PASSWORD", "secret")
        monkeypatch.setenv("APP_CLICKHOUSE_DATABASE", "custom_db")

        c = ClickHouseConfig.from_env()
        assert c.host == "clickhouse.example.com"
        assert c.port == 9000
        assert c.user == "admin"
        assert c.password == "secret"
        assert c.database == "custom_db"

    def test_from_env_reads_password_from_file(self, monkeypatch: pytest.MonkeyPatch) -> None:
        """_FILE環境変数からパスワードを読み込む"""
        with NamedTemporaryFile(mode="w", suffix=".txt", delete=False) as f:
            f.write("file_password\n")
            f.flush()

            monkeypatch.setenv("APP_CLICKHOUSE_PASSWORD_FILE", f.name)
            monkeypatch.delenv("APP_CLICKHOUSE_PASSWORD", raising=False)

            c = ClickHouseConfig.from_env()
            assert c.password == "file_password"

            # クリーンアップ
            Path(f.name).unlink()

    def test_file_password_takes_precedence(self, monkeypatch: pytest.MonkeyPatch) -> None:
        """_FILEはプレーン環境変数より優先される"""
        with NamedTemporaryFile(mode="w", suffix=".txt", delete=False) as f:
            f.write("file_password")
            f.flush()

            monkeypatch.setenv("APP_CLICKHOUSE_PASSWORD", "env_password")
            monkeypatch.setenv("APP_CLICKHOUSE_PASSWORD_FILE", f.name)

            c = ClickHouseConfig.from_env()
            assert c.password == "file_password"

            Path(f.name).unlink()


class TestReportConfig:
    """ReportConfig のテスト"""

    def test_default_values(self) -> None:
        """デフォルト値が正しく設定される"""
        r = ReportConfig()
        assert r.language == "ja"
        assert r.output_dir == Path("./reports")
        assert r.include_raw_data is False

    def test_from_env_with_custom_values(self, monkeypatch: pytest.MonkeyPatch) -> None:
        """環境変数からカスタム値を読み込む"""
        monkeypatch.setenv("METRICS_REPORT_LANGUAGE", "en")
        monkeypatch.setenv("METRICS_OUTPUT_DIR", "/tmp/reports")
        monkeypatch.setenv("METRICS_INCLUDE_RAW_DATA", "true")

        r = ReportConfig.from_env()
        assert r.language == "en"
        assert r.output_dir == Path("/tmp/reports")
        assert r.include_raw_data is True


class TestAppConfig:
    """AppConfig のテスト"""

    def test_from_env_creates_all_configs(self, monkeypatch: pytest.MonkeyPatch) -> None:
        """from_envですべての設定が作成される"""
        # 環境変数をクリア
        for key in list(os.environ.keys()):
            if key.startswith(("APP_CLICKHOUSE_", "METRICS_")):
                monkeypatch.delenv(key, raising=False)

        config = AppConfig.from_env()

        assert isinstance(config.clickhouse, ClickHouseConfig)
        assert isinstance(config.thresholds, HealthThresholds)
        assert isinstance(config.report, ReportConfig)
