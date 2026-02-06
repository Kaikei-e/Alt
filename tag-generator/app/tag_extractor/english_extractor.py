"""English keyword extraction using KeyBERT and NLTK tokenization."""

import re
from collections import Counter
from typing import Any, cast

import nltk
import structlog

from .config import TagExtractionConfig

logger = structlog.get_logger(__name__)


def extract_keywords_english(
    text: str,
    keybert: Any,
    config: TagExtractionConfig,
) -> tuple[list[str], dict[str, float]]:
    """Extract keywords specifically for English text using KeyBERT.

    Args:
        text: Input English text
        keybert: KeyBERT instance
        config: Tag extraction configuration

    Returns:
        Tuple of (tag_list, tag_confidences_dict)
    """
    if keybert is None:
        raise RuntimeError("KeyBERT not initialized")

    try:
        # First extract both single words and phrases
        single_keywords = keybert.extract_keywords(
            text,
            keyphrase_ngram_range=(1, 1),  # Single words only
            top_n=config.top_keywords * 3,
            use_mmr=True,
            diversity=0.3,
        )

        phrase_keywords = keybert.extract_keywords(
            text,
            keyphrase_ngram_range=(2, 3),  # Phrases only
            top_n=config.top_keywords,
            use_mmr=True,
            diversity=0.5,
        )

        # Combine and process keywords
        all_keywords = []
        seen_words: set[str] = set()

        # Process phrases first to identify important compound terms
        for phrase_tuple in cast(list[tuple[str, float]], phrase_keywords):
            phrase = phrase_tuple[0].strip().lower()
            score = phrase_tuple[1]
            # Only keep phrases with high scores or specific patterns
            if score >= config.min_score_threshold * 1.5:  # Higher threshold for phrases
                # Check if it's a meaningful compound (e.g., "apple intelligence", "mac mini")
                words = phrase.split()
                if len(words) >= 2:
                    # Check for tech terms, product names, or proper nouns
                    if any(w[0].isupper() for w in phrase.split() if w):
                        all_keywords.append((phrase, score))
                        # Mark individual words as seen to avoid duplication
                        seen_words.update(words)

        # Then add important single words not already in phrases
        for word_tuple in cast(list[tuple[str, float]], single_keywords):
            word = word_tuple[0].strip().lower()
            score = word_tuple[1]
            if score >= config.min_score_threshold and word not in seen_words:
                # Skip generic words
                if len(word) > 2 and not word.isdigit():
                    all_keywords.append((word, score))
                    seen_words.add(word)

        # Sort by score and filter
        all_keywords.sort(key=lambda x: x[1], reverse=True)

        # Final filtering and cleaning
        result: list[str] = []
        tag_confidences: dict[str, float] = {}
        seen_final: set[str] = set()

        for keyword, score in all_keywords:
            # Clean and check for duplicates
            keyword_clean = keyword.strip()
            keyword_lower = keyword_clean.lower()

            # Skip if we've seen this or a very similar variant
            if keyword_lower not in seen_final:
                # Check for substring relationships
                is_substring = False
                for seen in seen_final:
                    if keyword_lower in seen or seen in keyword_lower:
                        # Only skip if the longer one has higher score
                        is_substring = True
                        break

                if not is_substring:
                    result.append(keyword_clean)
                    # Normalize score to 0.0-1.0 range and store
                    normalized_score = min(max(score, 0.0), 1.0)
                    tag_confidences[keyword_clean] = round(normalized_score, 3)
                    seen_final.add(keyword_lower)

                    if len(result) >= config.top_keywords:
                        break

        return result, tag_confidences

    except Exception as e:
        logger.error("KeyBERT extraction failed for English", error=e)
        return [], {}


def tokenize_english(
    text: str,
    en_stopwords: set[str],
    config: TagExtractionConfig,
) -> list[str]:
    """Tokenize English text using NLTK.

    Args:
        text: Input English text
        en_stopwords: Set of English stopwords
        config: Tag extraction configuration

    Returns:
        List of filtered and normalized tokens
    """
    tokens = nltk.word_tokenize(text)
    result = []

    for token in tokens:
        if re.fullmatch(r"\w+", token) and len(token) > config.min_token_length:
            normalized = _normalize_text_english(token)
            if normalized not in en_stopwords:
                result.append(normalized)

    return result


def get_candidate_tokens(
    text: str,
    en_stopwords: set[str],
    config: TagExtractionConfig,
) -> list[str]:
    """Get candidate tokens for fallback extraction (primarily English text).

    This helper exists mainly for clarity and testability and currently
    delegates to the English tokenizer.

    Args:
        text: Input English text
        en_stopwords: Set of English stopwords
        config: Tag extraction configuration

    Returns:
        List of candidate tokens
    """
    return tokenize_english(text, en_stopwords, config)


def fallback_english(
    text: str,
    en_stopwords: set[str],
    config: TagExtractionConfig,
) -> list[str]:
    """Fallback extraction for English text using tokenization and frequency.

    Args:
        text: Input English text
        en_stopwords: Set of English stopwords
        config: Tag extraction configuration

    Returns:
        List of extracted keywords
    """
    tokens = get_candidate_tokens(text, en_stopwords, config)
    if tokens:
        token_freq = Counter(tokens)
        return [term for term, _ in token_freq.most_common(config.top_keywords)]
    return []


def _normalize_text_english(text: str) -> str:
    """Normalize English text to lowercase."""
    return text.lower()
