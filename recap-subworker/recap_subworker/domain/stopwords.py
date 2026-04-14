"""Backward-compat re-export for stopword helpers.

The canonical implementation moved to :mod:`recap_subworker.infra.stopwords`
in Phase 3A so that third-party ML / NLP dependencies (scikit-learn,
nltk) stop leaking into the domain layer. Existing imports are kept
alive through this shim; new code should import from ``infra`` directly.
"""

from __future__ import annotations

from ..infra.stopwords import (
    NLTK_AVAILABLE,
    get_stopwords,
)

__all__ = [
    "NLTK_AVAILABLE",
    "get_stopwords",
]
