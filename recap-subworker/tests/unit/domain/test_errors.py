"""Unit tests for domain error hierarchy."""

from __future__ import annotations

from recap_subworker.domain.errors import (
    ClassificationError,
    ClusteringError,
    ClusteringTimeoutError,
    ConcurrentRunError,
    EmbeddingError,
    EvidenceProcessingError,
    IdempotencyMismatchError,
    InsufficientDataError,
    InvalidEmbeddingsError,
    ModelArtifactNotFoundError,
    ModelNotLoadedError,
    OllamaConnectionError,
    PipelineError,
    RepositoryError,
    RunNotFoundError,
    SubworkerError,
    WarmupError,
)


class TestErrorHierarchy:
    def test_all_errors_inherit_from_subworker_error(self):
        errors = [
            PipelineError(),
            EvidenceProcessingError(),
            InsufficientDataError(),
            WarmupError(),
            ClusteringError(),
            ClusteringTimeoutError(timeout_seconds=30),
            InvalidEmbeddingsError(),
            EmbeddingError(),
            ModelNotLoadedError(),
            OllamaConnectionError(url="http://localhost:11434", detail="timeout"),
            ClassificationError(),
            ModelArtifactNotFoundError(path="/models/genre.pkl"),
            ConcurrentRunError(),
            IdempotencyMismatchError(),
            RepositoryError(),
            RunNotFoundError(run_id=42),
        ]
        for err in errors:
            assert isinstance(err, SubworkerError)

    def test_pipeline_hierarchy(self):
        assert isinstance(EvidenceProcessingError(), PipelineError)
        assert isinstance(WarmupError(), PipelineError)
        assert isinstance(InsufficientDataError(), PipelineError)

    def test_clustering_hierarchy(self):
        assert isinstance(ClusteringTimeoutError(10), ClusteringError)
        assert isinstance(InvalidEmbeddingsError(), ClusteringError)

    def test_embedding_hierarchy(self):
        assert isinstance(ModelNotLoadedError(), EmbeddingError)
        assert isinstance(
            OllamaConnectionError("http://localhost", "err"), EmbeddingError
        )

    def test_classification_hierarchy(self):
        assert isinstance(ModelArtifactNotFoundError("/path"), ClassificationError)

    def test_repository_hierarchy(self):
        assert isinstance(RunNotFoundError(1), RepositoryError)


class TestSpecificErrors:
    def test_clustering_timeout_stores_seconds(self):
        err = ClusteringTimeoutError(timeout_seconds=60)
        assert err.timeout_seconds == 60
        assert "60s" in str(err)

    def test_ollama_connection_stores_url(self):
        err = OllamaConnectionError(url="http://ollama:11434", detail="refused")
        assert err.url == "http://ollama:11434"
        assert "refused" in str(err)

    def test_model_artifact_stores_path(self):
        err = ModelArtifactNotFoundError(path="/models/genre.pkl")
        assert err.path == "/models/genre.pkl"
        assert "/models/genre.pkl" in str(err)

    def test_run_not_found_stores_id(self):
        err = RunNotFoundError(run_id=99)
        assert err.run_id == 99
        assert "99" in str(err)
