"""config.py のテスト"""

import os
from pathlib import Path

import pytest

from alt_metrics.config import (
    AppConfig,
    ClickHouseConfig,
    HealthThresholds,
    ReportConfig,
)

_DEFAULT_ERROR_RATE_CRITICAL = 10.0
_DEFAULT_ERROR_RATE_HIGH = 5.0
_DEFAULT_ERROR_RATE_WARNING = 1.0
_DEFAULT_ERROR_RATE_MINOR = 0.5
_DEFAULT_LATENCY_CRITICAL_MS = 10000
_DEFAULT_LATENCY_HIGH_MS = 5000
_DEFAULT_LATENCY_WARNING_MS = 1000
_DEFAULT_LATENCY_MINOR_MS = 500
_DEFAULT_LOG_GAP_CRITICAL_MIN = 10
_DEFAULT_LOG_GAP_WARNING_MIN = 5

_CUSTOM_ERROR_RATE_CRITICAL = 20.0
_CUSTOM_LATENCY_CRITICAL_MS = 15000

_DEFAULT_CH_PORT = 8123
_CUSTOM_CH_PORT = 9000


class TestHealthThresholds:
    """HealthThresholds のテスト"""

    def test_default_values(self) -> None:
        """デフォルト値が正しく設定される"""
        t = HealthThresholds()
        assert t.error_rate_critical == _DEFAULT_ERROR_RATE_CRITICAL
        assert t.error_rate_high == _DEFAULT_ERROR_RATE_HIGH
        assert t.error_rate_warning == 1.0
        assert t.error_rate_minor == _DEFAULT_ERROR_RATE_MINOR
        assert t.latency_critical_ms == _DEFAULT_LATENCY_CRITICAL_MS
        assert t.latency_high_ms == _DEFAULT_LATENCY_HIGH_MS
        assert t.latency_warning_ms == _DEFAULT_LATENCY_WARNING_MS
        assert t.latency_minor_ms == _DEFAULT_LATENCY_MINOR_MS
        assert t.log_gap_critical_min == _DEFAULT_LOG_GAP_CRITICAL_MIN
        assert t.log_gap_warning_min == _DEFAULT_LOG_GAP_WARNING_MIN
        assert t.slo_error_rate_threshold == 1.0

    def test_from_env_with_no_env_vars(self, monkeypatch: pytest.MonkeyPatch) -> None:
        """環境変数がない場合はデフォルト値を使用"""
        # 環境変数をクリア
        for key in list(os.environ.keys()):
            if key.startswith("METRICS_THRESHOLD_"):
                monkeypatch.delenv(key, raising=False)

        t = HealthThresholds.from_env()
        assert t.error_rate_critical == _DEFAULT_ERROR_RATE_CRITICAL

    def test_from_env_with_custom_values(self, monkeypatch: pytest.MonkeyPatch) -> None:
        """環境変数からカスタム値を読み込む"""
        monkeypatch.setenv("METRICS_THRESHOLD_ERROR_RATE_CRITICAL", str(_CUSTOM_ERROR_RATE_CRITICAL))
        monkeypatch.setenv("METRICS_THRESHOLD_LATENCY_CRITICAL_MS", str(_CUSTOM_LATENCY_CRITICAL_MS))

        t = HealthThresholds.from_env()
        assert t.error_rate_critical == _CUSTOM_ERROR_RATE_CRITICAL
        assert t.latency_critical_ms == _CUSTOM_LATENCY_CRITICAL_MS

    def test_from_env_ignores_invalid_values(self, monkeypatch: pytest.MonkeyPatch) -> None:
        """不正な値は無視してデフォルト値を使用"""
        monkeypatch.setenv("METRICS_THRESHOLD_ERROR_RATE_CRITICAL", "not_a_number")

        t = HealthThresholds.from_env()
        assert t.error_rate_critical == _DEFAULT_ERROR_RATE_CRITICAL


class TestClickHouseConfig:
    """ClickHouseConfig のテスト"""

    def test_default_values(self) -> None:
        """デフォルト値が正しく設定される"""
        c = ClickHouseConfig()
        assert c.host == "localhost"
        assert c.port == _DEFAULT_CH_PORT
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
        assert c.port == _DEFAULT_CH_PORT

    def test_from_env_with_custom_values(self, monkeypatch: pytest.MonkeyPatch) -> None:
        """環境変数からカスタム値を読み込む"""
        monkeypatch.setenv("APP_CLICKHOUSE_HOST", "clickhouse.example.com")
        monkeypatch.setenv("APP_CLICKHOUSE_PORT", str(_CUSTOM_CH_PORT))
        monkeypatch.setenv("APP_CLICKHOUSE_USER", "admin")
        monkeypatch.setenv("APP_CLICKHOUSE_PASSWORD", "secret")
        monkeypatch.setenv("APP_CLICKHOUSE_DATABASE", "custom_db")

        c = ClickHouseConfig.from_env()
        assert c.host == "clickhouse.example.com"
        assert c.port == _CUSTOM_CH_PORT
        assert c.user == "admin"
        assert c.password == "secret"
        assert c.database == "custom_db"

    def test_from_env_reads_password_from_file(self, monkeypatch: pytest.MonkeyPatch, tmp_path: Path) -> None:
        """_FILE環境変数からパスワードを読み込む"""
        secret_file = tmp_path / "password.txt"
        secret_file.write_text("file_password\n")

        monkeypatch.setenv("APP_CLICKHOUSE_PASSWORD_FILE", str(secret_file))
        monkeypatch.delenv("APP_CLICKHOUSE_PASSWORD", raising=False)

        c = ClickHouseConfig.from_env()
        assert c.password == "file_password"

    def test_file_password_takes_precedence(self, monkeypatch: pytest.MonkeyPatch, tmp_path: Path) -> None:
        """_FILEはプレーン環境変数より優先される"""
        secret_file = tmp_path / "password.txt"
        secret_file.write_text("file_password")

        monkeypatch.setenv("APP_CLICKHOUSE_PASSWORD", "env_password")
        monkeypatch.setenv("APP_CLICKHOUSE_PASSWORD_FILE", str(secret_file))

        c = ClickHouseConfig.from_env()
        assert c.password == "file_password"


class TestReportConfig:
    """ReportConfig のテスト"""

    def test_default_values(self) -> None:
        """デフォルト値が正しく設定される"""
        r = ReportConfig()
        assert r.language == "ja"
        assert r.output_dir == Path("./scripts/reports")
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
