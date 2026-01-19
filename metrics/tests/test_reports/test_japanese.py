"""reports/japanese.py のテスト"""

from __future__ import annotations

from alt_metrics.models import AnalysisResult
from alt_metrics.reports.japanese import format_table, generate_japanese_report


class TestFormatTable:
    """format_table関数のテスト"""

    def test_empty_data_returns_no_data_message(self) -> None:
        """空データは「データがありません」を返す"""
        result = format_table([])
        assert "データがありません" in result

    def test_formats_data_as_markdown_table(self) -> None:
        """データをMarkdownテーブルにフォーマット"""
        data = [
            {"name": "Alice", "age": 30},
            {"name": "Bob", "age": 25},
        ]
        result = format_table(data)

        assert "| name | age |" in result
        assert "|---|---|" in result
        assert "| Alice | 30 |" in result
        assert "| Bob | 25 |" in result

    def test_uses_specified_columns(self) -> None:
        """指定されたカラムのみを使用"""
        data = [
            {"name": "Alice", "age": 30, "email": "alice@example.com"},
        ]
        result = format_table(data, columns=["name", "email"])

        assert "| name | email |" in result
        assert "age" not in result

    def test_truncates_long_values(self) -> None:
        """長い値は60文字で切り詰め"""
        long_value = "a" * 100
        data = [{"message": long_value}]
        result = format_table(data)

        # 60文字に切り詰められる
        assert "a" * 60 in result
        assert "a" * 61 not in result


class TestGenerateJapaneseReport:
    """generate_japanese_report関数のテスト"""

    def test_report_contains_title(self, sample_analysis_result: AnalysisResult) -> None:
        """レポートにタイトルが含まれる"""
        report = generate_japanese_report(sample_analysis_result)
        assert "Alt システム健全性レポート" in report

    def test_report_contains_generation_time(self, sample_analysis_result: AnalysisResult) -> None:
        """レポートに生成日時が含まれる"""
        report = generate_japanese_report(sample_analysis_result)
        assert "生成日時" in report
        assert "2026-01-19" in report

    def test_report_contains_analysis_period(self, sample_analysis_result: AnalysisResult) -> None:
        """レポートに分析期間が含まれる"""
        report = generate_japanese_report(sample_analysis_result)
        assert "過去24時間" in report

    def test_report_contains_health_score(self, sample_analysis_result: AnalysisResult) -> None:
        """レポートに健全性スコアが含まれる"""
        report = generate_japanese_report(sample_analysis_result)
        assert "65/100" in report

    def test_report_contains_summary_stats(self, sample_analysis_result: AnalysisResult) -> None:
        """レポートにサマリー統計が含まれる"""
        report = generate_japanese_report(sample_analysis_result)
        assert "総ログエントリ数" in report
        assert "総エラー数" in report
        assert "監視サービス数" in report

    def test_report_contains_critical_issues(self, sample_analysis_result: AnalysisResult) -> None:
        """レポートに重大な問題が含まれる"""
        report = generate_japanese_report(sample_analysis_result)
        assert "重大な問題" in report
        assert "auth-hub" in report

    def test_report_contains_warnings(self, sample_analysis_result: AnalysisResult) -> None:
        """レポートに警告が含まれる"""
        report = generate_japanese_report(sample_analysis_result)
        assert "警告" in report
        assert "エラー率が高いサービス" in report

    def test_report_contains_recommendations(self, sample_analysis_result: AnalysisResult) -> None:
        """レポートに推奨事項が含まれる"""
        report = generate_japanese_report(sample_analysis_result)
        assert "推奨事項" in report

    def test_report_contains_service_health_table(self, sample_analysis_result: AnalysisResult) -> None:
        """レポートにサービス健全性テーブルが含まれる"""
        report = generate_japanese_report(sample_analysis_result)
        assert "サービス健全性ダッシュボード" in report
        assert "alt-backend" in report
        assert "auth-hub" in report

    def test_report_contains_footer(self, sample_analysis_result: AnalysisResult) -> None:
        """レポートにフッターが含まれる"""
        report = generate_japanese_report(sample_analysis_result)
        assert "Alt システム健全性アナライザー" in report

    def test_report_shows_correct_health_status(self, sample_analysis_result: AnalysisResult) -> None:
        """レポートに正しい健全性ステータスが表示される"""
        report = generate_japanese_report(sample_analysis_result)
        # スコア65は「劣化」
        assert "劣化" in report

    def test_empty_result_generates_valid_report(self) -> None:
        """空の結果でも有効なレポートを生成"""
        empty_result = AnalysisResult(hours_analyzed=24)
        report = generate_japanese_report(empty_result)

        assert "Alt システム健全性レポート" in report
        assert "データがありません" in report or "検出されませんでした" in report
