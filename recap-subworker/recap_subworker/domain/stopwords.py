"""Stopword utilities supporting Recap topic extraction."""

from __future__ import annotations

import os
import warnings
from functools import lru_cache
from pathlib import Path
from typing import Iterable, Set

from sklearn.feature_extraction.text import ENGLISH_STOP_WORDS

_ADDITIONAL_STOPWORDS: Set[str] = {
    # Common verbs and discourse markers that surfaced as noisy top terms.
    "demonstrate",
    "demonstrated",
    "demonstrates",
    "demonstrating",
    "demonstration",
    "including",
    "regarding",
    "providing",
    "making",
    "taking",
    "having",
    "using",
    "across",
    "within",
    "among",
    "however",
    "meanwhile",
    "overall",
    # Pronouns that occasionally survive scikit-learn defaults.
    "hers",
    "herself",
    "himself",
    "ours",
    "ourselves",
    "themselves",
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
    """Return the base stopword set with operator-provided extensions."""

    stopwords = set(ENGLISH_STOP_WORDS)
    stopwords.update(_ADDITIONAL_STOPWORDS)
    stopwords.update(_iterate_extra_terms())
    return stopwords

