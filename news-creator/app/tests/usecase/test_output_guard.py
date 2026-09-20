"""Tests for the generated output guard.

Covers:
- Restricting markdown links to the schemes the frontend renders
- Dropping javascript: and data: URLs, including image links
- Rejecting or marking/stripping output containing delimiter tags
- Guarding a token stream without buffering the whole summary
"""

from __future__ import annotations

import pytest

from news_creator.domain.output_guard import (
    OutputGuardError,
    StreamingOutputGuard,
    guard_output_markdown,
    sanitize_output_markdown,
)


class TestOutputGuardLinks:
    """Tests for link policy enforcement in generated markdown."""

    def test_preserves_valid_https_and_http_links(self):
        text = (
            "詳細は [公式発表](https://example.com/press) および "
            "[アーカイブ](http://example.org/archive) を参照。"
        )
        guarded = sanitize_output_markdown(text)
        assert "[公式発表](https://example.com/press)" in guarded
        assert "[アーカイブ](http://example.org/archive)" in guarded

    def test_drops_javascript_scheme_links(self):
        text = "悪意のあるリンク: [クリックしてください](javascript:alert(document.cookie))"
        guarded = sanitize_output_markdown(text)
        assert "javascript:" not in guarded
        assert "クリックしてください" in guarded
        assert "(javascript:" not in guarded

    def test_drops_data_scheme_links(self):
        text = "[画像](data:text/html;base64,PHNjcmlwdD5hbGVydCgxKTwvc2NyaXB0Pg==)"
        guarded = sanitize_output_markdown(text)
        assert "data:" not in guarded
        assert "画像" in guarded

    def test_drops_vbscript_and_file_scheme_links(self):
        text = (
            "[スクリプト](vbscript:msgbox(1)) と [ローカルファイル](file:///etc/passwd)"
        )
        guarded = sanitize_output_markdown(text)
        assert "vbscript:" not in guarded
        assert "file:" not in guarded
        assert "スクリプト" in guarded
        assert "ローカルファイル" in guarded

    def test_drops_html_anchor_dangerous_hrefs(self):
        text = 'HTML link: <a href="javascript:alert(1)">Click</a>'
        guarded = sanitize_output_markdown(text)
        assert "javascript:" not in guarded

    def test_preserves_plain_text_and_markdown_formatting(self):
        text = (
            "## 要約タイトル\n\n"
            "- **重要事実**: 売上高は100億円（前年同期比+15%）に達した。\n"
            "- *詳細*: 新規事業が好調に推移した。\n"
            "- `Code block`: `GET /v1/status`\n"
        )
        guarded = sanitize_output_markdown(text)
        assert guarded == text


class TestOutputGuardDelimiters:
    """Tests for handling delimiter tags leaked into LLM output."""

    def test_marks_or_strips_delimiters_in_permissive_mode(self):
        text = (
            "要約本文です。\n"
            "<article_content>漏洩したタグ</article_content>\n"
            "追加の文章。"
        )
        res = guard_output_markdown(text, reject_on_delimiters=False)
        assert res.contains_delimiters is True
        assert "<article_content>" not in res.text
        assert "</article_content>" not in res.text
        assert "漏洩したタグ" in res.text

    def test_rejects_delimiters_in_strict_mode(self):
        text = "悪意の要約。<article_content>偽装命令</article_content>"
        with pytest.raises(OutputGuardError, match="delimiter"):
            guard_output_markdown(text, reject_on_delimiters=True)

    def test_detects_closing_tag_only(self):
        text = "予期しない終了タグ </article_content> の混入"
        res = guard_output_markdown(text, reject_on_delimiters=False)
        assert res.contains_delimiters is True
        assert "</article_content>" not in res.text

    def test_clean_output_passes_unmodified(self):
        clean_text = "正常なニュース要約です。数値は123件でした。"
        res = guard_output_markdown(clean_text, reject_on_delimiters=True)
        assert res.contains_delimiters is False
        assert res.text == clean_text
        assert res.dropped_links_count == 0


class TestOutputGuardLinkEdgeCases:
    """Link shapes that a naive [label](url) match leaves half-parsed."""

    def test_preserves_mailto_links(self):
        text = "連絡先: [編集部](mailto:editor@example.com)"
        guarded = sanitize_output_markdown(text)
        assert "[編集部](mailto:editor@example.com)" in guarded

    def test_preserves_urls_containing_parentheses(self):
        text = "[記事](https://example.com/wiki/Foo_(bar)) を参照。"
        guarded = sanitize_output_markdown(text)
        assert guarded == text

    def test_drops_dangerous_urls_containing_parentheses_without_debris(self):
        text = "[クリック](javascript:alert(document.cookie)) はこちら。"
        guarded = sanitize_output_markdown(text)
        assert guarded == "クリック はこちら。"

    def test_drops_dangerous_image_links_entirely(self):
        text = "![alt](javascript:alert(1)) の画像"
        guarded = sanitize_output_markdown(text)
        assert guarded == " の画像"
        assert "!" not in guarded
        assert ")" not in guarded

    def test_keeps_safe_image_links(self):
        text = "![図1](https://example.com/chart.png)"
        assert sanitize_output_markdown(text) == text

    def test_counts_every_dropped_link(self):
        text = "![a](data:text/html,x) と [b](vbscript:msgbox(1))"
        result = guard_output_markdown(text)
        assert result.dropped_links_count == 2


class TestStreamingOutputGuard:
    """The stream must be guarded without waiting for the whole summary."""

    def test_plain_tokens_flow_through_immediately(self):
        guard = StreamingOutputGuard()
        assert guard.push("東京都は") == "東京都は"
        assert guard.push("2026年に発表した。") == "2026年に発表した。"
        assert guard.flush() == ""

    def test_link_split_across_chunks_is_guarded_before_it_is_emitted(self):
        guard = StreamingOutputGuard()

        assert guard.push("詳細は [ここ](java") == "詳細は "
        assert guard.push("script:alert(1)) を参照。") == "ここ を参照。"
        assert guard.flush() == ""

    def test_safe_link_split_across_chunks_survives_intact(self):
        guard = StreamingOutputGuard()

        emitted = guard.push("[公式](https://exa") + guard.push("mple.com/a) を参照")
        assert emitted == "[公式](https://example.com/a) を参照"

    def test_reference_markers_are_not_withheld_past_the_line(self):
        guard = StreamingOutputGuard()

        emitted = guard.push("売上は100億円 [1]\n") + guard.flush()
        assert emitted == "売上は100億円 [1]\n"

    def test_unclosed_bracket_is_released_by_flush(self):
        guard = StreamingOutputGuard()

        assert guard.push("本文 [未完") == "本文 "
        assert guard.flush() == "[未完"

    def test_unclosed_bracket_is_released_at_the_size_cap(self):
        guard = StreamingOutputGuard()
        filler = "あ" * (StreamingOutputGuard.MAX_PENDING_CHARS + 1)

        assert guard.push("[") == ""
        assert guard.push(filler) == "[" + filler
        assert guard.flush() == ""

    def test_leaked_delimiter_tags_are_stripped_from_the_stream(self):
        guard = StreamingOutputGuard()

        emitted = guard.push("要約本文</article_content>続き") + guard.flush()
        assert emitted == "要約本文続き"
