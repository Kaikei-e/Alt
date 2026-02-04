"""Tests for CJK text handling in input sanitizer."""

import pytest

from tag_extractor.input_sanitizer import InputSanitizer


class TestCJKTextHandling:
    """Test CJK (Japanese/Chinese/Korean) text handling."""

    @pytest.fixture
    def sanitizer(self):
        """Create a sanitizer instance for testing."""
        return InputSanitizer()

    def test_japanese_text_not_blocked(self, sanitizer):
        """Japanese text should not be blocked by unusual character check."""
        result = sanitizer.sanitize(
            title="AIが変える未来の働き方",
            content="人工知能（AI）技術の発展により、私たちの働き方は大きく変化しています。「働き方改革」が叫ばれる中、AIツールの活用は避けて通れない課題となっています。",
        )
        assert result.is_valid, f"Japanese text blocked: {result.violations}"

    def test_japanese_with_punctuation(self, sanitizer):
        """Japanese text with heavy punctuation should pass."""
        result = sanitizer.sanitize(
            title="「新機能」のお知らせ",
            content="【重要】本日より、新機能「タグ生成」が使えるようになりました！詳細は、設定画面→「タグ設定」をご確認ください。",
        )
        assert result.is_valid, f"Japanese with punctuation blocked: {result.violations}"

    def test_japanese_with_many_quotes(self, sanitizer):
        """Japanese text with many quotation marks should pass."""
        result = sanitizer.sanitize(
            title="「」『』の使い方",
            content="日本語では「かぎ括弧」や『二重かぎ括弧』をよく使います。「引用」や『作品名』の表記に使われます。「テスト」「データ」「機能」「設定」など。",
        )
        assert result.is_valid, f"Japanese with quotes blocked: {result.violations}"

    def test_chinese_text_not_blocked(self, sanitizer):
        """Chinese text should not be blocked by unusual character check."""
        result = sanitizer.sanitize(
            title="机器学习入门指南",
            content="本文将介绍机器学习的基本概念和常用算法，包括监督学习、无监督学习和强化学习。",
        )
        assert result.is_valid, f"Chinese text blocked: {result.violations}"

    def test_korean_text_not_blocked(self, sanitizer):
        """Korean text should not be blocked by unusual character check."""
        result = sanitizer.sanitize(
            title="인공지능 기술 동향",
            content="인공지능 기술이 빠르게 발전하고 있습니다. 머신러닝과 딥러닝을 활용한 다양한 애플리케이션이 개발되고 있습니다.",
        )
        assert result.is_valid, f"Korean text blocked: {result.violations}"

    def test_english_text_still_checked(self, sanitizer):
        """English text should still be checked for unusual patterns."""
        # Text with >30% special characters should be blocked
        result = sanitizer.sanitize(
            title="Normal Title",
            content="!!!!!@@@@@#####$$$$$%%%%%^^^^^&&&&&*****((((()))))",
        )
        assert not result.is_valid
        assert any("suspicious" in v.lower() for v in result.violations)

    def test_mixed_cjk_and_english(self, sanitizer):
        """Mixed CJK and English text should pass."""
        result = sanitizer.sanitize(
            title="AI/人工知能の最新動向",
            content="ChatGPTやGPT-4などの大規模言語モデル（LLM）が注目を集めています。「プロンプトエンジニアリング」という新しい分野も生まれました。",
        )
        assert result.is_valid, f"Mixed text blocked: {result.violations}"

    def test_cjk_detection_threshold(self, sanitizer):
        """CJK detection should work with >10% CJK characters."""
        # 90% English, 10% CJK - should be treated as CJK
        result = sanitizer.sanitize(
            title="This is a test title",
            content="This is mostly English text but contains 日本語 which is Japanese.",
        )
        assert result.is_valid, f"Text with small CJK portion blocked: {result.violations}"

    def test_is_cjk_text_method(self, sanitizer):
        """Test _is_cjk_text method directly."""
        # Pure Japanese
        assert sanitizer._is_cjk_text("これは日本語です") is True

        # Pure English
        assert sanitizer._is_cjk_text("This is English") is False

        # Mixed (>10% CJK)
        assert sanitizer._is_cjk_text("Hello 世界") is True

        # Empty string
        assert sanitizer._is_cjk_text("") is False

        # CJK punctuation only (should count)
        assert sanitizer._is_cjk_text("「」『』【】") is True
