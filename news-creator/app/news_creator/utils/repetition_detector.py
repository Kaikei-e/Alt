"""Repetition detection utility for LLM-generated text."""

import logging
import re
from dataclasses import dataclass

logger = logging.getLogger(__name__)


@dataclass(frozen=True)
class _RepetitionRule:
    pattern: str
    weight: float
    label: str
    flags: int = 0


_REPETITION_RULES: tuple[_RepetitionRule, ...] = (
    _RepetitionRule(r"\b(\w+)(?:\s+\1){2,}\b", 0.2, "Word repetition", re.IGNORECASE),
    _RepetitionRule(r"(</?\w+[^>]*>)(?:\s*\1){2,}", 0.3, "HTML tag repetition"),
    _RepetitionRule(r'(\w+="[^"]*")(?:\s*\1){2,}', 0.25, "Attribute repetition"),
)

_COMMON_SHORT_WORDS = frozenset(
    {"the", "and", "or", "but", "in", "on", "at", "to", "for", "of", "with"}
)
_MIN_TEXT_LENGTH = 10
_MIN_URL_REPETITIONS = 3
_HTTP_PREFIX = "http://"
_HTTPS_PREFIX = "https://"


def _is_url_token(token: str) -> bool:
    """Return True if token starts with http(s):// followed by at least one char."""
    if token.startswith(_HTTP_PREFIX):
        return len(token) > len(_HTTP_PREFIX)
    if token.startswith(_HTTPS_PREFIX):
        return len(token) > len(_HTTPS_PREFIX)
    return False


def _find_repeated_urls(text: str) -> list[str]:
    """Find ``(https?://[^\\s]+)(?:\\s+\\1){2,}`` matches in linear time."""
    if _HTTP_PREFIX not in text and _HTTPS_PREFIX not in text:
        return []
    tokens = re.findall(r"\S+", text)
    count = len(tokens)
    if count < _MIN_URL_REPETITIONS:
        return []

    matches: list[str] = []
    idx = 0
    offset = 0
    while idx + 2 < count:
        candidate = tokens[idx + 1]
        head = tokens[idx][offset:] if offset else tokens[idx]
        if (
            _is_url_token(candidate)
            and head.endswith(candidate)
            and tokens[idx + 2].startswith(candidate)
        ):
            matches.append(candidate)
            end_idx = idx + 2
            while end_idx < count and tokens[end_idx] == candidate:
                end_idx += 1
            if end_idx < count and tokens[end_idx].startswith(candidate):
                idx = end_idx
                offset = len(candidate)
            else:
                idx = end_idx
                offset = 0
            continue
        idx += 1
        offset = 0
    return matches


def _apply_rule(
    text: str,
    rule: _RepetitionRule,
    scores: list[float],
    patterns: list[str],
) -> None:
    matches = re.findall(rule.pattern, text, rule.flags)
    if matches:
        scores.append(min(1.0, len(matches) * rule.weight))
        patterns.append(f"{rule.label}: {len(matches)} patterns found")
        logger.debug("%s detected: %s", rule.label, matches[:5])


def detect_repetition(
    text: str, threshold: float = 0.3
) -> tuple[bool, float, list[str]]:
    """
    Detect repetitive patterns in text.

    Args:
        text: Text to analyze
        threshold: Repetition score threshold (0.0-1.0). If score >= threshold, repetition is detected.

    Returns:
        Tuple of (has_repetition: bool, score: float, patterns: list[str])
        - has_repetition: True if repetition is detected
        - score: Repetition score (0.0-1.0), higher means more repetition
        - patterns: List of detected repetitive patterns
    """
    if not text or len(text.strip()) < _MIN_TEXT_LENGTH:
        return False, 0.0, []

    patterns: list[str] = []
    scores: list[float] = []

    for rule in _REPETITION_RULES:
        _apply_rule(text, rule, scores, patterns)

    # Short string repetition excluding common stopwords
    short_matches = re.findall(r"([^\s]{1,10})(?:\s+\1){2,}", text)
    if short_matches:
        filtered_matches = [
            m for m in short_matches if m.lower() not in _COMMON_SHORT_WORDS
        ]
        if filtered_matches:
            scores.append(min(1.0, len(filtered_matches) * 0.15))
            patterns.append(
                f"Short string repetition: {len(filtered_matches)} patterns found"
            )
            logger.debug("Short string repetition detected: %s", filtered_matches[:5])

    url_matches = _find_repeated_urls(text)
    if url_matches:
        scores.append(min(1.0, len(url_matches) * 0.3))
        patterns.append(f"URL repetition: {len(url_matches)} patterns found")
        logger.debug("URL repetition detected: %s", url_matches[:3])

    char_matches = re.findall(r"(.)\1{3,}", text)
    if char_matches:
        scores.append(min(1.0, len(char_matches) * 0.1))
        patterns.append(f"Character repetition: {len(char_matches)} patterns found")

    if not scores:
        return False, 0.0, []

    # Use maximum score rather than average (if any pattern is severe, flag it)
    overall_score = max(scores)
    has_repetition = overall_score >= threshold

    if has_repetition:
        logger.warning(
            "Repetition detected in text",
            extra={
                "score": overall_score,
                "threshold": threshold,
                "patterns": patterns,
                "text_preview": text[:200],
            },
        )

    return has_repetition, overall_score, patterns


def has_severe_repetition(text: str, threshold: float = 0.3) -> bool:
    """
    Quick check if text has severe repetition.

    Args:
        text: Text to check
        threshold: Repetition score threshold

    Returns:
        True if repetition is detected
    """
    has_rep, _, _ = detect_repetition(text, threshold)
    return has_rep
