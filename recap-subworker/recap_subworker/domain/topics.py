"""Topic extraction helpers using c-TF-IDF."""

from __future__ import annotations

import re
import numpy as np
from sklearn.feature_extraction.text import TfidfVectorizer

from .stopwords import get_stopwords

_TOKEN_SPLIT_RE = re.compile(r"[^\w']+")
_ALLOWED_SHORT_TOKENS = {"ai", "vr", "uk", "us", "eu", "ux"}


def _tokenize_feature(term: str) -> list[str]:
    return [part for part in _TOKEN_SPLIT_RE.split(term.lower()) if part]


def _is_informative(term: str, stopwords: set[str]) -> bool:
    stripped = term.strip()
    if not stripped:
        return False
    if stripped.isdigit():
        return False
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
    """Return top terms per cluster using c-TF-IDF."""

    if not corpus_by_cluster:
        return []
    stopword_set = get_stopwords()
    vectorizer = TfidfVectorizer(
        ngram_range=(1, 2),
        min_df=1,
        max_df=0.95,
        lowercase=True,
        stop_words=sorted(stopword_set),
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
