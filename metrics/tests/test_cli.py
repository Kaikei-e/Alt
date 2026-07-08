"""cli.py のテスト

analyze/validateコマンドの部分失敗継続とexit codeを検証します。
"""

from __future__ import annotations

import argparse
from pathlib import Path
from typing import Any
from unittest.mock import MagicMock

import pytest
from clickhouse_connect.driver.exceptions import OperationalError

from alt_metrics import cli
from alt_metrics.config import ClickHouseConfig, HealthThresholds
from alt_metrics.exceptions import CollectorError


def _make_empty_client() -> MagicMock:
    """全コレクターが空データを取得できるモッククライアント"""
    client = MagicMock()
    client.query.return_value.column_names = []
    client.query.return_value.result_rows = []
    return client


class TestRunAnalysis:
    """run_analysis関数のテスト"""

    def test_continues_after_a_single_collector_error(self, monkeypatch: pytest.MonkeyPatch) -> None:
        """一部のコレクターがCollectorErrorを送出しても分析全体は継続する"""
        client = _make_empty_client()
        monkeypatch.setattr(cli.clickhouse_connect, "get_client", lambda **kwargs: client)
        monkeypatch.setattr(
            cli,
            "collect_error_types",
            MagicMock(side_effect=CollectorError("error_types", "boom")),
        )

        result = cli.run_analysis(ClickHouseConfig(), HealthThresholds(), hours=24)

        # 失敗したコレクターはデフォルト値のまま、他は正常に収集される
        assert result.error_types == []
        assert result.service_stats == []
        assert result.hours_analyzed == 24

    def test_closes_client_after_analysis(self, monkeypatch: pytest.MonkeyPatch) -> None:
        """分析完了後にClickHouseクライアントをcloseする"""
        client = _make_empty_client()
        monkeypatch.setattr(cli.clickhouse_connect, "get_client", lambda **kwargs: client)

        cli.run_analysis(ClickHouseConfig(), HealthThresholds(), hours=24)

        client.close.assert_called_once()

    def test_slo_violation_collector_error_does_not_crash(self, monkeypatch: pytest.MonkeyPatch) -> None:
        """SLO違反コレクターの失敗も継続する"""
        client = _make_empty_client()
        monkeypatch.setattr(cli.clickhouse_connect, "get_client", lambda **kwargs: client)
        monkeypatch.setattr(
            cli,
            "collect_slo_violations",
            MagicMock(side_effect=CollectorError("slo_violations", "boom")),
        )

        result = cli.run_analysis(ClickHouseConfig(), HealthThresholds(), hours=24)

        assert result.slo_violations == []


class TestCmdAnalyze:
    """cmd_analyzeコマンドのexit codeテスト"""

    def _args(self, tmp_path: Path, **overrides: Any) -> argparse.Namespace:
        defaults: dict[str, Any] = {"hours": 24, "output_dir": tmp_path, "lang": "ja", "verbose": False}
        defaults.update(overrides)
        return argparse.Namespace(**defaults)

    def test_returns_0_on_success(self, monkeypatch: pytest.MonkeyPatch, tmp_path: Path) -> None:
        """正常終了時はexit code 0"""
        client = _make_empty_client()
        monkeypatch.setattr(cli.clickhouse_connect, "get_client", lambda **kwargs: client)

        exit_code = cli.cmd_analyze(self._args(tmp_path))

        assert exit_code == 0
        assert list(tmp_path.glob("system_health_*.md"))

    def test_returns_1_on_connection_error(self, monkeypatch: pytest.MonkeyPatch, tmp_path: Path) -> None:
        """ClickHouse接続失敗時はexit code 1"""

        def _raise(**kwargs: Any) -> None:
            raise OperationalError("connection refused")

        monkeypatch.setattr(cli.clickhouse_connect, "get_client", _raise)

        exit_code = cli.cmd_analyze(self._args(tmp_path))

        assert exit_code == 1

    def test_returns_1_on_unexpected_exception(self, monkeypatch: pytest.MonkeyPatch, tmp_path: Path) -> None:
        """予期しない例外もexit code 1で終了する"""
        monkeypatch.setattr(
            cli,
            "run_analysis",
            MagicMock(side_effect=RuntimeError("unexpected")),
        )

        exit_code = cli.cmd_analyze(self._args(tmp_path))

        assert exit_code == 1


class TestCmdValidate:
    """cmd_validateコマンドのexit codeテスト"""

    def _args(self, **overrides: Any) -> argparse.Namespace:
        defaults: dict[str, Any] = {"verbose": False}
        defaults.update(overrides)
        return argparse.Namespace(**defaults)

    def test_returns_0_on_success(self, monkeypatch: pytest.MonkeyPatch) -> None:
        """接続・テーブル確認が成功すればexit code 0"""
        client = MagicMock()
        client.query.return_value.result_rows = [(1,)]
        monkeypatch.setattr(cli.clickhouse_connect, "get_client", lambda **kwargs: client)

        exit_code = cli.cmd_validate(self._args())

        assert exit_code == 0

    def test_returns_1_on_connection_error(self, monkeypatch: pytest.MonkeyPatch) -> None:
        """接続失敗時はexit code 1"""

        def _raise(**kwargs: Any) -> None:
            raise OperationalError("connection refused")

        monkeypatch.setattr(cli.clickhouse_connect, "get_client", _raise)

        exit_code = cli.cmd_validate(self._args())

        assert exit_code == 1

    def test_continues_when_a_single_table_check_fails(self, monkeypatch: pytest.MonkeyPatch) -> None:
        """個別テーブルのアクセス失敗は握りつぶして全体は継続する"""
        client = MagicMock()
        # 初回(SELECT 1)は成功、以降のテーブル確認は失敗させる
        client.query.side_effect = [
            MagicMock(result_rows=[(1,)]),
            OperationalError("table not found"),
            OperationalError("table not found"),
            OperationalError("table not found"),
            OperationalError("table not found"),
            OperationalError("table not found"),
            OperationalError("table not found"),
        ]
        monkeypatch.setattr(cli.clickhouse_connect, "get_client", lambda **kwargs: client)

        exit_code = cli.cmd_validate(self._args())

        assert exit_code == 0
