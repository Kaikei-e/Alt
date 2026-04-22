"""Defense-in-depth guard for the case where GenreClassifierService.model_path
resolves to a directory rather than a file.

The Settings validator in infra/config.py is the primary gate, but if the
model_path is ever set after Settings construction (or the validator is
bypassed, e.g., in a direct instantiation), joblib.load() would raise
IsADirectoryError — unhelpful and flags as "worker pool init timeout" 300s
later. Keep the guard tight at the load site too.
"""

from __future__ import annotations

from unittest.mock import MagicMock

import pytest

from recap_subworker.services.classifier import GenreClassifierService


@pytest.fixture
def mock_embedder() -> MagicMock:
    embedder = MagicMock()
    embedder.config.batch_size = 32
    return embedder


class TestModelPathGuard:
    def test_directory_at_model_path_raises_filenotfound(
        self, mock_embedder: MagicMock, tmp_path
    ) -> None:
        # Reproduces the docker file-scoped bind-mount footgun: the target
        # path exists but is an empty directory, so Path.exists() is True
        # while the file isn't loadable.
        stub = tmp_path / "genre_classifier.joblib"
        stub.mkdir()

        service = GenreClassifierService(str(stub), mock_embedder)

        with pytest.raises(FileNotFoundError) as exc_info:
            service._ensure_model()

        assert str(stub) in str(exc_info.value)

    def test_missing_path_raises_filenotfound(self, mock_embedder: MagicMock, tmp_path) -> None:
        missing = tmp_path / "never_created.joblib"

        service = GenreClassifierService(str(missing), mock_embedder)

        with pytest.raises(FileNotFoundError):
            service._ensure_model()
