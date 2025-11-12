"""Stopword utilities supporting Recap topic extraction."""

from __future__ import annotations

import os
import warnings
from functools import lru_cache
from pathlib import Path
from typing import Iterable, Set

from sklearn.feature_extraction.text import ENGLISH_STOP_WORDS

try:
    import nltk

    # Download stopwords data if not already present
    try:
        nltk.data.find("corpora/stopwords")
    except LookupError:
        nltk.download("stopwords", quiet=True)

    from nltk.corpus import stopwords as nltk_stopwords

    NLTK_AVAILABLE = True
except (ImportError, OSError):
    NLTK_AVAILABLE = False
    nltk_stopwords = None

# News article domain-specific stopwords that frequently appear but lack semantic value
_NEWS_DOMAIN_STOPWORDS: Set[str] = {
    # Reporting verbs and phrases
    "according",
    "reported",
    "reports",
    "reporting",
    "said",
    "says",
    "saying",
    "sources",
    "source",
    "officials",
    "official",
    "spokesperson",
    "spokesman",
    "spokeswoman",
    "announced",
    "announcement",
    "announces",
    "announcing",
    # Common discourse markers in news
    "meanwhile",
    "however",
    "furthermore",
    "moreover",
    "nevertheless",
    "nonetheless",
    "therefore",
    "thus",
    "hence",
    "consequently",
    "accordingly",
    # Generic action verbs that lack specificity
    "demonstrate",
    "demonstrated",
    "demonstrates",
    "demonstrating",
    "demonstration",
    "showing",
    "shown",
    "shows",
    "show",
    "including",
    "includes",
    "include",
    "regarding",
    "providing",
    "provides",
    "provide",
    "making",
    "makes",
    "made",
    "taking",
    "takes",
    "took",
    "having",
    "has",
    "have",
    "using",
    "uses",
    "used",
    # Prepositions and particles that slip through
    "across",
    "within",
    "among",
    "amongst",
    "throughout",
    "overall",
    # Pronouns that occasionally survive defaults
    "hers",
    "herself",
    "himself",
    "ours",
    "ourselves",
    "themselves",
    "yours",
    "yourself",
    "yourselves",
}

_STOPWORD_FILE_ENV = "RECAP_SUBWORKER_STOPWORDS_FILE"
_STOPWORD_EXTRA_ENV = "RECAP_SUBWORKER_STOPWORDS_EXTRA"


def _iterate_extra_terms() -> Iterable[str]:
    extra_terms = os.getenv(_STOPWORD_EXTRA_ENV)
    if extra_terms:
        for term in extra_terms.split(","):
            stripped = term.strip().lower()
            if stripped:
                yield stripped

    file_path = os.getenv(_STOPWORD_FILE_ENV)
    if not file_path:
        return

    path = Path(file_path).expanduser()
    try:
        with path.open("r", encoding="utf-8") as handle:
            for line in handle:
                stripped = line.strip().lower()
                if stripped and not stripped.startswith("#"):
                    yield stripped
    except OSError as exc:
        warnings.warn(f"Failed to load stopword file '{path}': {exc}", stacklevel=2)


@lru_cache(maxsize=1)
def get_stopwords() -> Set[str]:
    """Return comprehensive stopword set combining multiple sources.

    Combines:
    - scikit-learn's ENGLISH_STOP_WORDS (179 words)
    - NLTK's English stopwords (if available, ~179 words with some overlap)
    - News domain-specific stopwords
    - Operator-provided extensions via environment variables
    """

    # Start with scikit-learn's stopwords (most comprehensive base)
    stopwords = set(ENGLISH_STOP_WORDS)

    # Add NLTK stopwords if available (provides additional coverage)
    if NLTK_AVAILABLE and nltk_stopwords:
        try:
            nltk_words = set(nltk_stopwords.words("english"))
            stopwords.update(nltk_words)
        except (LookupError, OSError) as exc:
            warnings.warn(f"Failed to load NLTK stopwords: {exc}", stacklevel=2)

    # Add news domain-specific stopwords
    stopwords.update(_NEWS_DOMAIN_STOPWORDS)

    # Add operator-provided extensions
    stopwords.update(_iterate_extra_terms())

    return stopwords
