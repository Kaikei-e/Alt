"""Branch coverage tests for recap_card_parser edge cases."""

from __future__ import annotations

from news_creator.usecase.recap_card_parser import parse_card_output, split_sentences


def test_recap_card_parser_why_ja_citation_errors() -> None:
    missing_why_ref = """
【見出し】
ソラリス社、分散ログ基盤の次期版を公開
【何が起きた】
ソラリス社はログ収集エンジンを公開した。[1]
メモリ使用量が従来比で40%削減された。[1]
【なぜ重要】
運用コストが削減される見込みである。
【出典】
[1]
"""
    res_missing = parse_card_output(missing_why_ref, valid_refs={1})
    assert res_missing.success is False
    assert res_missing.reason == "parse_failed"
    assert res_missing.detail == "why_format"

    unknown_why_ref = """
【見出し】
ソラリス社、分散ログ基盤の次期版を公開
【何が起きた】
ソラリス社はログ収集エンジンを公開した。[1]
メモリ使用量が従来比で40%削減された。[1]
【なぜ重要】
運用コストが削減される見込みである。[99]
【出典】
[1] [99]
"""
    res_unknown = parse_card_output(unknown_why_ref, valid_refs={1})
    assert res_unknown.success is False
    assert res_unknown.reason == "unknown_ref"


def test_split_sentences_edge_cases() -> None:
    assert split_sentences("   \n\t ") == []
    assert split_sentences("一文目。   [abc] 二文目。") == [
        "一文目。",
        "[abc] 二文目。",
    ]
    assert split_sentences("一文目。[1]。二文目。") == ["一文目。[1]。", "二文目。"]
    assert split_sentences("一文目[1]  [2] 二文目[3]") == [
        "一文目[1]  [2]",
        "二文目[3]",
    ]
    assert split_sentences("配列[abc]テスト。残りテキスト") == [
        "配列[abc]テスト。",
        "残りテキスト",
    ]
