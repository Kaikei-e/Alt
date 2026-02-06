"""Joblib classifier gateway implementing ClassifierPort.

Delegates to the existing services/classifier.py GenreClassifierService.
"""

from __future__ import annotations

from typing import Any

from ..services.classifier import GenreClassifierService
from ..services.embedder import Embedder


class JoblibClassifierGateway:
    """Gateway wrapping GenreClassifierService for ClassifierPort compliance."""

    def __init__(
        self,
        model_path: str,
        embedder: Embedder,
        vectorizer_path: str | None = None,
        thresholds_path: str | None = None,
    ) -> None:
        self._classifier = GenreClassifierService(
            model_path=model_path,
            embedder=embedder,
            vectorizer_path=vectorizer_path,
            thresholds_path=thresholds_path,
        )

    def predict_batch(
        self,
        texts: list[str],
        multi_label: bool = False,
        top_k: int = 5,
        threshold_overrides: dict[str, float] | None = None,
    ) -> list[dict[str, Any]]:
        return self._classifier.predict_batch(
            texts,
            multi_label=multi_label,
            top_k=top_k,
            threshold_overrides=threshold_overrides,
        )
