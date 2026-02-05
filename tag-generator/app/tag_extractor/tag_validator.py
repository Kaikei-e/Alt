"""
Shared tag validation utilities for Japanese text.

This module provides unified tag validation logic used across all extractors
(extract.py, hybrid_extractor.py, ginza_extractor.py) to ensure consistent
quality filtering of Japanese tags.

Key functions:
- is_valid_japanese_tag: Validate a tag for quality
- clean_noun_phrase: Remove trailing particles/verbs from noun phrases
"""

import re

# Default maximum tag length (characters)
DEFAULT_MAX_TAG_LENGTH = 15

# Verb/auxiliary verb endings that indicate sentence fragments
VERB_ENDINGS_PATTERN = re.compile(
    r"(です|ます|ました|ている|した|する|ない|ある|いる|れる|られる|います|ています|しょう|でしょう)$"
)

# Particle endings (Japanese grammatical particles)
# These at the end of a phrase indicate incomplete noun phrases
PARTICLE_ENDINGS_PATTERN = re.compile(r"[はがをにでとのへやもかな]$")

# URL/HTML fragments that should not be tags
URL_HTML_FRAGMENTS_PATTERN = re.compile(r"^(https?|www|com|org|net|html|gt|lt|amp|nbsp)$", re.IGNORECASE)

# Number-only pattern
NUMBER_ONLY_PATTERN = re.compile(r"^\d+$")


def is_valid_japanese_tag(tag: str, max_length: int = DEFAULT_MAX_TAG_LENGTH) -> bool:
    """
    Validate a Japanese tag for quality.

    This function filters out:
    - Tags too short (<2 chars) or too long (>max_length chars)
    - Tags ending with verbs/auxiliary verbs (sentence fragments)
    - Tags ending with particles (incomplete phrases)
    - Tags that are numbers only
    - URL/HTML fragments

    Args:
        tag: The candidate tag to validate
        max_length: Maximum allowed tag length (default: 15)

    Returns:
        True if the tag is valid, False otherwise
    """
    # Length check
    if not (2 <= len(tag) <= max_length):
        return False

    # Verb/auxiliary verb endings (sentence fragments)
    if VERB_ENDINGS_PATTERN.search(tag):
        return False

    # Particle endings (incomplete phrases) - apply to ALL lengths
    if PARTICLE_ENDINGS_PATTERN.search(tag):
        return False

    # Number-only tags
    if NUMBER_ONLY_PATTERN.fullmatch(tag):
        return False

    # URL/HTML fragments
    if URL_HTML_FRAGMENTS_PATTERN.fullmatch(tag):
        return False

    return True


def clean_noun_phrase(phrase: str) -> str:
    """
    Clean a noun phrase by removing trailing particles and verb endings.

    This is used to post-process noun phrases extracted by GiNZA or other
    methods that may include trailing grammatical elements.

    Args:
        phrase: The noun phrase to clean

    Returns:
        Cleaned noun phrase with trailing particles/verbs removed
    """
    phrase = phrase.strip()

    if not phrase:
        return phrase

    # Remove trailing particles
    phrase = PARTICLE_ENDINGS_PATTERN.sub("", phrase)

    # Remove verb/auxiliary verb endings
    phrase = VERB_ENDINGS_PATTERN.sub("", phrase)

    return phrase.strip()
