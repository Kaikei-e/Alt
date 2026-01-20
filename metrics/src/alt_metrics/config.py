"""設定管理

ClickHouse接続設定とヘルススコア閾値を管理します。
環境変数とDocker Secretsに対応しています。
"""

from __future__ import annotations

import os
from dataclasses import dataclass, field
from pathlib import Path


@dataclass(frozen=True)
class HealthThresholds:
    """ヘルススコア閾値設定

    各閾値を超えた場合にスコアから減点されます。
    環境変数で上書き可能です。
    """

    # エラー率閾値 (%)
    error_rate_critical: float = 10.0  # >10%: -40点
    error_rate_high: float = 5.0  # >5%: -25点
    error_rate_warning: float = 1.0  # >1%: -10点
    error_rate_minor: float = 0.5  # >0.5%: -5点

    # レイテンシ閾値 (ms)
    latency_critical_ms: float = 10000  # >10s: -30点
    latency_high_ms: float = 5000  # >5s: -20点
    latency_warning_ms: float = 1000  # >1s: -10点
    latency_minor_ms: float = 500  # >500ms: -5点

    # ログ欠落閾値 (分)
    log_gap_critical_min: float = 10  # >10分: -30点
    log_gap_warning_min: float = 5  # >5分: -15点

    # SLO閾値
    slo_error_rate_threshold: float = 1.0  # エラー率 > 1% で違反
    slo_availability_target: float = 99.9  # SLO目標（%）- エラーバジェット計算に使用

    @classmethod
    def from_env(cls) -> HealthThresholds:
        """環境変数から閾値を読み込む"""

        def get_float(key: str, default: float) -> float:
            value = os.getenv(f"METRICS_THRESHOLD_{key}")
            if value:
                try:
                    return float(value)
                except ValueError:
                    pass
            return default

        return cls(
            error_rate_critical=get_float("ERROR_RATE_CRITICAL", 10.0),
            error_rate_high=get_float("ERROR_RATE_HIGH", 5.0),
            error_rate_warning=get_float("ERROR_RATE_WARNING", 1.0),
            error_rate_minor=get_float("ERROR_RATE_MINOR", 0.5),
            latency_critical_ms=get_float("LATENCY_CRITICAL_MS", 10000),
            latency_high_ms=get_float("LATENCY_HIGH_MS", 5000),
            latency_warning_ms=get_float("LATENCY_WARNING_MS", 1000),
            latency_minor_ms=get_float("LATENCY_MINOR_MS", 500),
            log_gap_critical_min=get_float("LOG_GAP_CRITICAL_MIN", 10),
            log_gap_warning_min=get_float("LOG_GAP_WARNING_MIN", 5),
            slo_error_rate_threshold=get_float("SLO_ERROR_RATE", 1.0),
            slo_availability_target=get_float("SLO_AVAILABILITY_TARGET", 99.9),
        )


@dataclass(frozen=True)
class ClickHouseConfig:
    """ClickHouse接続設定

    Docker Secretsパターン (_FILE サフィックス) に対応しています。
    """

    host: str = "localhost"
    port: int = 8123
    user: str = "default"
    password: str = ""
    database: str = "rask_logs"

    @classmethod
    def from_env(cls) -> ClickHouseConfig:
        """環境変数から設定を読み込む"""

        def get_env_or_file(name: str, default: str = "") -> str:
            """環境変数または _FILE で指定されたファイルから値を取得"""
            file_path = os.getenv(f"{name}_FILE")
            if file_path and Path(file_path).exists():
                return Path(file_path).read_text().strip()
            return os.getenv(name, default)

        return cls(
            host=os.getenv("APP_CLICKHOUSE_HOST", "localhost"),
            port=int(os.getenv("APP_CLICKHOUSE_PORT", "8123")),
            user=os.getenv("APP_CLICKHOUSE_USER", "default"),
            password=get_env_or_file("APP_CLICKHOUSE_PASSWORD", ""),
            database=os.getenv("APP_CLICKHOUSE_DATABASE", "rask_logs"),
        )


@dataclass
class ReportConfig:
    """レポート設定"""

    language: str = "ja"
    output_dir: Path = field(default_factory=lambda: Path("./reports"))
    include_raw_data: bool = False

    @classmethod
    def from_env(cls) -> ReportConfig:
        """環境変数から設定を読み込む"""
        return cls(
            language=os.getenv("METRICS_REPORT_LANGUAGE", "ja"),
            output_dir=Path(os.getenv("METRICS_OUTPUT_DIR", "./scripts/reports")),
            include_raw_data=os.getenv("METRICS_INCLUDE_RAW_DATA", "").lower() == "true",
        )


@dataclass
class AppConfig:
    """アプリケーション全体の設定"""

    clickhouse: ClickHouseConfig
    thresholds: HealthThresholds
    report: ReportConfig

    @classmethod
    def from_env(cls) -> AppConfig:
        """環境変数からすべての設定を読み込む"""
        return cls(
            clickhouse=ClickHouseConfig.from_env(),
            thresholds=HealthThresholds.from_env(),
            report=ReportConfig.from_env(),
        )
