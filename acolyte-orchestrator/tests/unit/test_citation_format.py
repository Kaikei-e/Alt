"""Tests for inline citation format validator."""

import pytest

from acolyte.domain.citation_format import validate_citation_format, validate_citation_grounding


class TestValidateCitationFormat:
    def test_accepts_sn_form_only(self) -> None:
        body = "主張の根拠は第一次情報にある [S1]。別の点 [S2]。"
        ok, reason = validate_citation_format(body)
        assert ok is True
        assert reason == ""

    def test_accepts_multi_digit_sn(self) -> None:
        body = "Reference [S10] and [S42]."
        ok, _ = validate_citation_format(body)
        assert ok is True

    def test_accepts_body_with_no_citations(self) -> None:
        body = "普通の文章に記号はなし。"
        ok, _ = validate_citation_format(body)
        assert ok is True

    def test_rejects_inline_title_bracket(self) -> None:
        body = "（[山梨 山林火災 延焼範囲が西側に拡大 約180ha焼ける | NHKニュース | 火災、山梨県]）"
        ok, reason = validate_citation_format(body)
        assert ok is False
        assert "inline_title" in reason

    def test_rejects_numeric_bracket_like_perplexity(self) -> None:
        body = "古い形式 [1] はもはや許されない。"
        ok, reason = validate_citation_format(body)
        assert ok is False
        assert reason

    def test_rejects_bare_url(self) -> None:
        body = "ソース https://example.com/foo を参照。"
        ok, reason = validate_citation_format(body)
        assert ok is False
        assert "bare_url" in reason

    def test_rejects_pipe_separated_bracket(self) -> None:
        body = "[foo | bar | baz] の形はタイトル+出典の混入パターン。"
        ok, _reason = validate_citation_format(body)
        assert ok is False

    @pytest.mark.parametrize(
        "body",
        [
            "[S1]",
            "[S99]",
            "短い [S3] のみ。",
        ],
    )
    def test_parametrized_valid_cases(self, body: str) -> None:
        ok, _ = validate_citation_format(body)
        assert ok is True


class TestValidateCitationGrounding:
    """Guards against hallucinated [Sn] markers not backed by real evidence."""

    def test_accepts_when_all_markers_are_registered(self) -> None:
        body = "主張の根拠は [S1] にある。補足は [S2]。"
        ok, reason = validate_citation_grounding(body, {"S1", "S2"})
        assert ok is True
        assert reason == ""

    def test_rejects_marker_outside_valid_id_set(self) -> None:
        body = "本文中に架空の [S3] を引用。"
        ok, reason = validate_citation_grounding(body, {"S1", "S2"})
        assert ok is False
        assert "S3" in reason
        assert "unknown_citation_id" in reason

    def test_accepts_body_with_no_markers_regardless_of_valid_ids(self) -> None:
        body = "マーカーを一切含まない文章。"
        ok, reason = validate_citation_grounding(body, set())
        assert ok is True
        assert reason == ""

    def test_rejects_any_marker_when_valid_ids_empty(self) -> None:
        body = "根拠がないのに引用する [S1]。"
        ok, reason = validate_citation_grounding(body, set())
        assert ok is False
        assert "S1" in reason

    def test_reports_all_unknown_markers(self) -> None:
        body = "複数の幻覚引用 [S3] と [S9]。"
        ok, reason = validate_citation_grounding(body, {"S1"})
        assert ok is False
        assert "S3" in reason
        assert "S9" in reason

    def test_deduplicates_repeated_unknown_marker(self) -> None:
        body = "同じ幻覚引用を繰り返す [S5] ... [S5]。"
        ok, reason = validate_citation_grounding(body, {"S1"})
        assert ok is False
        assert reason.count("S5") == 1

    def test_ignores_markers_already_flagged_as_bad_bracket_format(self) -> None:
        """[1] (no S-prefix) is not an [Sn] marker — grounding does not see it."""
        body = "旧式の引用 [1] は grounding の対象外。"
        ok, reason = validate_citation_grounding(body, {"S1"})
        assert ok is True
        assert reason == ""
