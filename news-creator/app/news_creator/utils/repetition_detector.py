"""Repetition detection utility for LLM-generated text."""

import re
import logging
from typing import Tuple, List

logger = logging.getLogger(__name__)


def detect_repetition(text: str, threshold: float = 0.3) -> Tuple[bool, float, List[str]]:
    """
    Detect repetitive patterns in text.

    Args:
        text: Text to analyze
        threshold: Repetition score threshold (0.0-1.0). If score >= threshold, repetition is detected.

    Returns:
        Tuple of (has_repetition: bool, score: float, patterns: List[str])
        - has_repetition: True if repetition is detected
        - score: Repetition score (0.0-1.0), higher means more repetition
        - patterns: List of detected repetitive patterns
    """
    if not text or len(text.strip()) < 10:
        return False, 0.0, []

    patterns: List[str] = []
    scores: List[float] = []

    # 1. Word-level repetition: same word repeated 3+ times consecutively
    word_pattern = r'\b(\w+)(?:\s+\1){2,}\b'
    word_matches = re.findall(word_pattern, text, re.IGNORECASE)
    if word_matches:
        word_score = min(1.0, len(word_matches) * 0.2)
        scores.append(word_score)
        patterns.append(f"Word repetition: {len(word_matches)} patterns found")
        logger.debug(f"Word repetition detected: {word_matches[:5]}")

    # 2. HTML tag repetition: </div></div></div> or <tag><tag><tag>
    html_pattern = r'(</?\w+[^>]*>)(?:\s*\1){2,}'
    html_matches = re.findall(html_pattern, text)
    if html_matches:
        html_score = min(1.0, len(html_matches) * 0.3)
        scores.append(html_score)
        patterns.append(f"HTML tag repetition: {len(html_matches)} patterns found")
        logger.debug(f"HTML tag repetition detected: {html_matches[:5]}")

    # 3. JSON/attribute pattern repetition: id=" id=" id=" or src=" src=" src="
    attr_pattern = r'(\w+="[^"]*")(?:\s*\1){2,}'
    attr_matches = re.findall(attr_pattern, text)
    if attr_matches:
        attr_score = min(1.0, len(attr_matches) * 0.25)
        scores.append(attr_score)
        patterns.append(f"Attribute repetition: {len(attr_matches)} patterns found")
        logger.debug(f"Attribute repetition detected: {attr_matches[:5]}")

    # 4. Short string repetition: "4" 4" 4" or "word word word" (same short string 3+ times)
    short_pattern = r'([^\s]{1,10})(?:\s+\1){2,}'
    short_matches = re.findall(short_pattern, text)
    if short_matches:
        # Filter out common words that might legitimately repeat
        common_words = {'the', 'and', 'or', 'but', 'in', 'on', 'at', 'to', 'for', 'of', 'with'}
        filtered_matches = [m for m in short_matches if m.lower() not in common_words]
        if filtered_matches:
            short_score = min(1.0, len(filtered_matches) * 0.15)
            scores.append(short_score)
            patterns.append(f"Short string repetition: {len(filtered_matches)} patterns found")
            logger.debug(f"Short string repetition detected: {filtered_matches[:5]}")

    # 5. URL pattern repetition: http://... http://... http://...
    url_pattern = r'(https?://[^\s]+)(?:\s+\1){2,}'
    url_matches = re.findall(url_pattern, text)
    if url_matches:
        url_score = min(1.0, len(url_matches) * 0.3)
        scores.append(url_score)
        patterns.append(f"URL repetition: {len(url_matches)} patterns found")
        logger.debug(f"URL repetition detected: {url_matches[:3]}")

    # 6. Character-level repetition: "aaaa" or "----" (4+ same characters)
    char_pattern = r'(.)\1{3,}'
    char_matches = re.findall(char_pattern, text)
    if char_matches:
        char_score = min(1.0, len(char_matches) * 0.1)
        scores.append(char_score)
        patterns.append(f"Character repetition: {len(char_matches)} patterns found")

    # Calculate overall score (weighted average, with higher weight for more severe patterns)
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
            }
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

