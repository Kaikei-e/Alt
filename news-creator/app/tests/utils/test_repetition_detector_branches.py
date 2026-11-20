"""Branch coverage tests for detect_repetition patterns."""

from __future__ import annotations

import time

import pytest

from news_creator.utils.repetition_detector import detect_repetition

ADVERSARIAL_REPETITIONS = 50_000
MAX_ADVERSARIAL_SECONDS = 2.0

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


@pytest.mark.parametrize(
    ("text", "expected"),
    [
        ("", (False, 0.0, [])),
        ("   ", (False, 0.0, [])),
        ("short", (False, 0.0, [])),
        (
            "https://example.com https://example.com https://example.com",
            (True, 0.3, ["URL repetition: 1 patterns found"]),
        ),
        (
            "http://example.com http://example.com http://example.com",
            (True, 0.3, ["URL repetition: 1 patterns found"]),
        ),
        ("http://a http://a", (False, 0.0, [])),
        (
            "http:// http:// http://",
            (True, 0.15, ["Short string repetition: 1 patterns found"]),
        ),
        (
            "https:// https:// https://",
            (True, 0.15, ["Short string repetition: 1 patterns found"]),
        ),
        (
            "http://a  http://a\nhttp://a\thttp://a",
            (
                True,
                0.3,
                [
                    "Short string repetition: 1 patterns found",
                    "URL repetition: 1 patterns found",
                ],
            ),
        ),
        (
            "http://a http://a http://a http://b http://b http://b",
            (
                True,
                0.6,
                [
                    "Short string repetition: 2 patterns found",
                    "URL repetition: 2 patterns found",
                ],
            ),
        ),
        (
            "prefixhttp://a http://a http://a",
            (
                True,
                0.3,
                [
                    "Short string repetition: 1 patterns found",
                    "URL repetition: 1 patterns found",
                ],
            ),
        ),
        (
            "prefixhttp://example-a http://example-a http://example-a",
            (True, 0.3, ["URL repetition: 1 patterns found"]),
        ),
        (
            "http://a http://a http://ahttp://b http://b http://b",
            (
                True,
                0.6,
                [
                    "Short string repetition: 2 patterns found",
                    "URL repetition: 2 patterns found",
                ],
            ),
        ),
        (
            "http://a http://a http://afoo",
            (
                True,
                0.3,
                [
                    "Short string repetition: 1 patterns found",
                    "URL repetition: 1 patterns found",
                ],
            ),
        ),
        (
            "http://日本語.jp/パス http://日本語.jp/パス http://日本語.jp/パス",
            (True, 0.3, ["URL repetition: 1 patterns found"]),
        ),
        ("http://" * 50, (False, 0.0, [])),
        (
            "http://a\nhttp://a\nhttp://a extra",
            (
                True,
                0.3,
                [
                    "Short string repetition: 1 patterns found",
                    "URL repetition: 1 patterns found",
                ],
            ),
        ),
    ],
)
def test_detect_repetition_url_characterization(
    text: str, expected: tuple[bool, float, list[str]]
) -> None:
    """Pin current detect_repetition output across ordinary, edge, and URL inputs."""
    assert detect_repetition(text, threshold=0.1) == expected


def test_detect_repetition_url_linear_on_adversarial_input() -> None:
    """Adversarial repeated 'http://' prefixes must complete in linear time."""
    payload = "http://" * ADVERSARIAL_REPETITIONS
    start = time.perf_counter()
    result = detect_repetition(payload)
    elapsed = time.perf_counter() - start

    assert result == (False, 0.0, [])
    assert elapsed < MAX_ADVERSARIAL_SECONDS
