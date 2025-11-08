"""Topic extraction helpers using c-TF-IDF."""

from __future__ import annotations

import re
from functools import lru_cache
from typing import Callable

import numpy as np
from sklearn.feature_extraction.text import TfidfVectorizer

from .stopwords import get_stopwords

try:
    from nltk.tokenize import RegexpTokenizer

    NLTK_AVAILABLE = True
except ImportError:
    NLTK_AVAILABLE = False
    RegexpTokenizer = None

try:
    import spacy

    SPACY_AVAILABLE = True
except ImportError:
    SPACY_AVAILABLE = False
    spacy = None

_TOKEN_SPLIT_RE = re.compile(r"[^\w']+")
_ALLOWED_SHORT_TOKENS = {"ai", "vr", "uk", "us", "eu", "ux"}
# Pattern to detect alphanumeric mixed strings (digits and letters together)
_ALPHANUMERIC_MIXED_PATTERN = re.compile(r"\d.*[a-zA-Z]|[a-zA-Z].*\d")


def _tokenize_feature(term: str) -> list[str]:
    return [part for part in _TOKEN_SPLIT_RE.split(term.lower()) if part]


@lru_cache(maxsize=1)
def _get_nltk_tokenizer() -> Callable[[str], list[str]] | None:
    """Get NLTK RegexpTokenizer for extracting alphabetic tokens only."""
    if not NLTK_AVAILABLE or RegexpTokenizer is None:
        return None
    # Pattern: word boundaries with alphabetic characters only (no digits)
    tokenizer = RegexpTokenizer(r"\b[a-zA-Z]+\b")
    return tokenizer.tokenize


@lru_cache(maxsize=1)
def _get_spacy_nlp():
    """Lazy load spaCy model for token validation."""
    if not SPACY_AVAILABLE or spacy is None:
        return None
    try:
        # Try to load the English model
        nlp = spacy.load("en_core_web_sm", disable=["parser", "ner"])
        return nlp
    except OSError:
        # Model not installed, return None (will use fallback)
        return None


def _has_alphanumeric_mixed(text: str) -> bool:
    """Check if text contains digits and letters mixed together."""
    return bool(_ALPHANUMERIC_MIXED_PATTERN.search(text))


def _is_informative(term: str, stopwords: set[str]) -> bool:
    """Check if a term is informative and should be included.

    This function performs multi-stage validation:
    1. Basic checks (empty, digits only)
    2. Alphanumeric mixed detection (regex)
    3. spaCy validation (if available)
    4. Stopword and length checks
    """
    stripped = term.strip()
    if not stripped:
        return False

    # Stage 1: Basic digit check
    if stripped.isdigit():
        return False

    # Stage 2: Regex-based alphanumeric mixed detection
    if _has_alphanumeric_mixed(stripped):
        return False

    # Stage 3: spaCy validation (high-precision check)
    nlp = _get_spacy_nlp()
    if nlp is not None:
        try:
            # Process the term as a single token
            doc = nlp(stripped)
            if doc:
                token = doc[0]
                # Reject if token is alphanumeric (contains both digits and letters)
                if token.is_alnum and not token.is_alpha and not token.is_digit:
                    return False
                # Reject if it's a space-separated ngram containing digits
                if " " in stripped:
                    parts = stripped.split()
                    for part in parts:
                        if part.isdigit() or _has_alphanumeric_mixed(part):
                            return False
        except Exception:
            # If spaCy fails, fall back to regex validation
            pass

    # Stage 4: Token-based validation
    tokens = _tokenize_feature(stripped)
    if not tokens:
        return False
    if all(token in stopwords for token in tokens):
        return False
    if all(len(token) <= 2 and token not in _ALLOWED_SHORT_TOKENS for token in tokens):
        return False

    return True


def extract_topics(
    corpus_by_cluster: list[str],
    top_n: int = 5,
    *,
    bm25_weighting: bool = False,
) -> list[list[str]]:
    """Return top terms per cluster using c-TF-IDF.

    Uses hybrid NLP filtering approach:
    1. NLTK RegexpTokenizer for fast pre-filtering (alphabetic tokens only)
    2. spaCy validation for high-precision alphanumeric detection
    3. Regex fallback for comprehensive coverage
    """

    if not corpus_by_cluster:
        return []
    stopword_set = get_stopwords()

    # Stage 1: Use NLTK RegexpTokenizer for fast filtering
    nltk_tokenizer = _get_nltk_tokenizer()
    custom_tokenizer = nltk_tokenizer if nltk_tokenizer else None

    vectorizer = TfidfVectorizer(
        ngram_range=(1, 2),
        min_df=1,
        max_df=0.95,
        lowercase=True,
        stop_words=sorted(stopword_set),
        tokenizer=custom_tokenizer,
    )
    matrix = vectorizer.fit_transform(corpus_by_cluster)
    dense = matrix.toarray()
    if bm25_weighting:
        k1, b = 1.5, 0.75
        doc_lengths = dense.sum(axis=1, keepdims=True)
        avgdl = float(doc_lengths.mean() or 1.0)
        denom = dense + k1 * (1 - b + b * doc_lengths / avgdl)
        dense = dense * ((k1 + 1) / np.where(denom == 0, 1, denom))
    features = np.array(vectorizer.get_feature_names_out())
    topics: list[list[str]] = []
    for row in dense:
        if not np.any(row):
            topics.append([])
            continue
        sorted_indices = np.argsort(-row)
        terms: list[str] = []
        for idx in sorted_indices:
            if row[idx] <= 0:
                continue
            candidate = features[idx]
            if not _is_informative(candidate, stopword_set):
                continue
            terms.append(candidate)
            if len(terms) == top_n:
                break
        topics.append(terms)
    return topics
