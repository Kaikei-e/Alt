"""Branch coverage tests for detect_repetition patterns."""

from __future__ import annotations

from news_creator.utils.repetition_detector import detect_repetition

REPETITION_THRESHOLD = 0.3


def test_detect_repetition_all_pattern_types() -> None:
    has_rep, score, patterns = detect_repetition("short")
    assert has_rep is False
    assert score == 0.0
    assert patterns == []

    has_html, _, html_patterns = detect_repetition("<div> <div> <div> content")
    assert has_html is True
    assert any("HTML tag repetition" in p for p in html_patterns)

    has_attr, _, attr_patterns = detect_repetition(
        'id="box" id="box" id="box" src="x" src="x" src="x"'
    )
    assert has_attr is True
    assert any("Attribute repetition" in p for p in attr_patterns)

    has_url, _, url_patterns = detect_repetition(
        "https://example.com https://example.com https://example.com"
    )
    assert has_url is True
    assert any("URL repetition" in p for p in url_patterns)

    has_common, common_score, common_patterns = detect_repetition(
        "the the the extra padding text"
    )
    assert has_common is False
    assert common_score < REPETITION_THRESHOLD
    assert not any("Short string repetition" in p for p in common_patterns)
