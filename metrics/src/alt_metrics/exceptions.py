"""カスタム例外クラス

メトリクス収集・分析・レポート生成時のエラーを適切に分類します。
"""

from __future__ import annotations


class MetricsError(Exception):
    """メトリクス関連エラーの基底クラス"""


class CollectorError(MetricsError):
    """データ収集エラー

    ClickHouseからのデータ取得時に発生するエラーを表します。
    """

    def __init__(self, collector_name: str, message: str) -> None:
        self.collector_name = collector_name
        super().__init__(f"[{collector_name}] {message}")


class ClickHouseConnectionError(MetricsError):
    """ClickHouse接続エラー

    ClickHouseサーバーへの接続に失敗した場合に発生します。
    """


class ConfigurationError(MetricsError):
    """設定エラー

    環境変数や設定ファイルの読み込みに失敗した場合に発生します。
    """


class ReportGenerationError(MetricsError):
    """レポート生成エラー

    Markdownレポートの生成時に発生するエラーを表します。
    """
