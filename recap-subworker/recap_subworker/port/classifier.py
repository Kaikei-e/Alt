"""Classifier port: Protocol for genre classification."""

from __future__ import annotations

from typing import Any, Protocol, runtime_checkable


@runtime_checkable
class ClassifierPort(Protocol):
    """Port for genre classification of texts."""

    def predict_batch(
        self,
        texts: list[str],
        multi_label: bool = False,
        top_k: int = 5,
        threshold_overrides: dict[str, float] | None = None,
    ) -> list[dict[str, Any]]:
        """Classify a batch of texts into genres.

        Args:
            texts: List of texts to classify.
            multi_label: If True, return multiple genres per text.
            top_k: Maximum number of genres per text.
            threshold_overrides: Per-genre threshold overrides.

        Returns:
            List of dicts with keys: top_genre, confidence, scores,
            candidates, below_threshold.
        """
        ...
