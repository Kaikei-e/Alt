"""Topic extraction helpers using c-TF-IDF."""

from __future__ import annotations

from typing import Iterable

import numpy as np
from sklearn.feature_extraction.text import TfidfVectorizer


def extract_topics(
    corpus_by_cluster: list[str],
    top_n: int = 5,
    *,
    bm25_weighting: bool = False,
) -> list[list[str]]:
    """Return top terms per cluster using c-TF-IDF."""

    if not corpus_by_cluster:
        return []
    vectorizer = TfidfVectorizer(
        ngram_range=(1, 2),
        min_df=1,
        max_df=0.95,
        lowercase=True,
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
        top_indices = np.argsort(-row)[:top_n]
        terms = [features[idx] for idx in top_indices if row[idx] > 0]
        topics.append(terms)
    return topics
