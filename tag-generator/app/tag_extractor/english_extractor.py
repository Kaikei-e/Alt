"""English keyword extraction using KeyBERT and NLTK tokenization."""

import re
from collections import Counter
from typing import Any, cast

import nltk
import structlog

from .config import TagExtractionConfig
from .tag_validator import is_valid_english_tag

logger = structlog.get_logger(__name__)

_ENGLISH_TOKEN_PATTERN = re.compile(r"[A-Za-z0-9]+(?:[+#._/-][A-Za-z0-9+#]+)*|[A-Za-z][A-Za-z0-9]*(?:\+\+|#)")


def _is_allowed_short_english_term(word: str, source_text: str) -> bool:
    """Reject ambiguous 2-letter lowercase prose words unless capitalized in source text."""
    if word == "go":
        return bool(re.search(r"\b(?:Go|GO)\b", source_text))
    if word == "us":
        return bool(re.search(r"\bUS\b", source_text))
    return True


def _is_duplicate_english_keyword(candidate: str, existing: str) -> bool:
    """Check phrase-token or stem duplication without conflating short tags like 'go'/'google' or 'ai'/'containers'."""
    if candidate == existing:
        return True
    shorter, longer = (candidate, existing) if len(candidate) <= len(existing) else (existing, candidate)
    if " " in longer:
        shorter_words = shorter.split()
        longer_words = longer.split()
        width = len(shorter_words)
        if any(longer_words[i : i + width] == shorter_words for i in range(len(longer_words) - width + 1)):
            return True
    if len(shorter) >= 3 and longer in (f"{shorter}s", f"{shorter}es"):
        return True
    if len(shorter) >= 5 and longer.startswith(shorter):
        return True
    return False


def extract_keywords_english(
    text: str,
    keybert: Any,
    config: TagExtractionConfig,
    en_stopwords: set[str] | None = None,
) -> tuple[list[str], dict[str, float]]:
    """Extract keywords specifically for English text using KeyBERT.

    Args:
        text: Input English text
        keybert: KeyBERT instance
        config: Tag extraction configuration
        en_stopwords: Optional English stopword set

    Returns:
        Tuple of (tag_list, tag_confidences_dict)
    """
    if keybert is None:
        raise RuntimeError("KeyBERT not initialized")

    active_stopwords = {w.lower() for w in en_stopwords} if en_stopwords else set()
    stop_words_arg: str | list[str] = sorted(active_stopwords) if active_stopwords else "english"

    # Single call covering both single words and phrases (1-3 ngrams)
    all_raw = keybert.extract_keywords(
        text,
        keyphrase_ngram_range=(1, 3),
        stop_words=stop_words_arg,
        top_n=config.top_keywords * 3,
        use_mmr=True,
        diversity=0.4,
    )

    # Separate into phrases and single words, then merge
    all_keywords = []
    seen_words: set[str] = set()
    min_single_length = max(2, config.min_token_length)
    proper_nouns = {
        match.group(1).lower()
        for match in re.finditer(r"(?<=[a-z,;:]\s)([A-Z][A-Za-z0-9+#._-]{1,})(?![A-Za-z0-9])", text)
    }

    # Process phrases first to identify important compound terms
    for phrase_tuple in cast(list[tuple[str, float]], all_raw):
        phrase = phrase_tuple[0].strip().lower()
        score = phrase_tuple[1]
        words = phrase.split()
        if len(words) >= 2:
            # Only keep phrases with high scores or specific patterns
            if (
                score >= config.min_score_threshold * 1.5
                and any(w[0].isupper() for w in phrase_tuple[0].strip().split() if w)
                and is_valid_english_tag(phrase, stopwords=active_stopwords, min_length=3)
            ):
                all_keywords.append((phrase, score))
                # Mark individual words as seen to avoid duplication
                seen_words.update(w for w in words if w not in active_stopwords)

    # Then add important single words not already in phrases
    for word_tuple in cast(list[tuple[str, float]], all_raw):
        word = word_tuple[0].strip().lower()
        score = word_tuple[1]
        words = word.split()
        if (
            len(words) == 1
            and score >= config.min_score_threshold
            and word not in seen_words
            and is_valid_english_tag(word, stopwords=active_stopwords, min_length=min_single_length)
            and _is_allowed_short_english_term(word, text)
        ):
            adjusted_score = min(1.0, score + 0.05) if word in proper_nouns else score
            all_keywords.append((word, adjusted_score))
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
            is_duplicate = any(_is_duplicate_english_keyword(keyword_lower, seen) for seen in seen_final)

            if not is_duplicate:
                result.append(keyword_clean)
                # Normalize score to 0.0-1.0 range and store
                normalized_score = min(max(score, 0.0), 1.0)
                tag_confidences[keyword_clean] = round(normalized_score, 3)
                seen_final.add(keyword_lower)

                if len(result) >= config.top_keywords:
                    break

    return result, tag_confidences


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
    raw_tokens = nltk.word_tokenize(text)
    merged_tokens: list[str] = []
    idx = 0
    while idx < len(raw_tokens):
        tok = raw_tokens[idx]
        if idx + 1 < len(raw_tokens) and re.fullmatch(r"[A-Za-z]", tok) and raw_tokens[idx + 1] in ("++", "#"):
            merged_tokens.append(f"{tok}{raw_tokens[idx + 1]}")
            idx += 2
        else:
            merged_tokens.append(tok)
            idx += 1

    result = []
    min_length = max(2, config.min_token_length)

    for token in merged_tokens:
        if not _ENGLISH_TOKEN_PATTERN.fullmatch(token):
            continue
        is_short_tech_token = len(token) == 2 and any(ch.isupper() or ch.isdigit() or ch in "+#" for ch in token)
        if len(token) > config.min_token_length or is_short_tech_token:
            normalized = _normalize_text_english(token)
            if is_valid_english_tag(normalized, stopwords=en_stopwords, min_length=min_length) and (
                _is_allowed_short_english_term(normalized, text)
            ):
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
