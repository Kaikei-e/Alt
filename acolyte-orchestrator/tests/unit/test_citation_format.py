"""Tests for inline citation format validator."""

import pytest

from acolyte.domain.citation_format import validate_citation_format


class TestValidateCitationFormat:
    def test_accepts_Sn_form_only(self):
        body = "主張の根拠は第一次情報にある [S1]。別の点 [S2]。"
        ok, reason = validate_citation_format(body)
        assert ok is True
        assert reason == ""

    def test_accepts_multi_digit_Sn(self):
        body = "Reference [S10] and [S42]."
        ok, _ = validate_citation_format(body)
        assert ok is True

    def test_accepts_body_with_no_citations(self):
        body = "普通の文章に記号はなし。"
        ok, _ = validate_citation_format(body)
        assert ok is True

    def test_rejects_inline_title_bracket(self):
        body = "（[山梨 山林火災 延焼範囲が西側に拡大 約180ha焼ける | NHKニュース | 火災、山梨県]）"
        ok, reason = validate_citation_format(body)
        assert ok is False
        assert "inline_title" in reason

    def test_rejects_numeric_bracket_like_perplexity(self):
        body = "古い形式 [1] はもはや許されない。"
        ok, reason = validate_citation_format(body)
        assert ok is False
        assert reason

    def test_rejects_bare_url(self):
        body = "ソース https://example.com/foo を参照。"
        ok, reason = validate_citation_format(body)
        assert ok is False
        assert "bare_url" in reason

    def test_rejects_pipe_separated_bracket(self):
        body = "[foo | bar | baz] の形はタイトル+出典の混入パターン。"
        ok, reason = validate_citation_format(body)
        assert ok is False

    @pytest.mark.parametrize(
        "body",
        [
            "[S1]",
            "[S99]",
            "短い [S3] のみ。",
        ],
    )
    def test_parametrized_valid_cases(self, body):
        ok, _ = validate_citation_format(body)
        assert ok is True
