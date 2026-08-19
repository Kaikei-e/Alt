"""Classifier / artefact readiness for /health/deep (PM-2026-036 class).

Never include filesystem paths in raised errors — the deep envelope must
stay opaque.
"""

from __future__ import annotations

from collections.abc import Sequence
from pathlib import Path
from typing import Protocol


class ArtefactSettings(Protocol):
    """Read-only view of classifier artefact paths.

    Attributes are properties so a Settings dataclass with Literal backend
    remains assignable (writable ``str`` would reject the narrower type).
    """

    @property
    def classification_backend(self) -> str: ...

    @property
    def genre_classifier_model_path(self) -> str: ...

    @property
    def genre_classifier_model_path_ja(self) -> str: ...

    @property
    def genre_classifier_model_path_en(self) -> str: ...

    @property
    def tfidf_vectorizer_path_ja(self) -> str: ...

    @property
    def tfidf_vectorizer_path_en(self) -> str: ...

    @property
    def genre_thresholds_path_ja(self) -> str: ...

    @property
    def genre_thresholds_path_en(self) -> str: ...

    @property
    def learning_machine_student_ja_dir(self) -> str | None: ...

    @property
    def learning_machine_student_en_dir(self) -> str | None: ...


_JOBLIB_FIELDS: tuple[str, ...] = (
    "genre_classifier_model_path",
    "genre_classifier_model_path_ja",
    "genre_classifier_model_path_en",
    "tfidf_vectorizer_path_ja",
    "tfidf_vectorizer_path_en",
    "genre_thresholds_path_ja",
    "genre_thresholds_path_en",
)


def assert_paths_ready(paths: Sequence[str], *, must_be_file: bool) -> None:
    """Raise RuntimeError if a configured path is missing or the wrong type."""
    seen = False
    for value in paths:
        if not value:
            continue
        seen = True
        path = Path(value)
        if must_be_file:
            if not path.is_file():
                raise RuntimeError("unavailable")
        elif not path.is_dir():
            raise RuntimeError("unavailable")
    if not seen:
        raise RuntimeError("unavailable")


def assert_classifier_artefacts(settings: ArtefactSettings) -> None:
    """Fail closed when classifier artefacts are missing or directory-shaped."""
    if settings.classification_backend == "joblib":
        assert_paths_ready(
            [str(getattr(settings, field) or "") for field in _JOBLIB_FIELDS],
            must_be_file=True,
        )
        return
    assert_paths_ready(
        [
            settings.learning_machine_student_ja_dir or "",
            settings.learning_machine_student_en_dir or "",
        ],
        must_be_file=False,
    )
