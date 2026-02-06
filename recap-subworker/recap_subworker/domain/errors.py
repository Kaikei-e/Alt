"""Domain error hierarchy for recap-subworker."""

from __future__ import annotations


class SubworkerError(Exception):
    """Base error for all recap-subworker domain exceptions."""


# --- Pipeline errors ---

class PipelineError(SubworkerError):
    """Error during evidence pipeline execution."""


class EvidenceProcessingError(PipelineError):
    """Raised when the pipeline fails irrecoverably."""


class InsufficientDataError(PipelineError):
    """Raised when input data is insufficient for processing."""


class WarmupError(PipelineError):
    """Raised during warmup failures."""


# --- Clustering errors ---

class ClusteringError(SubworkerError):
    """Error during clustering operation."""


class ClusteringTimeoutError(ClusteringError):
    """Raised when clustering exceeds the configured timeout."""

    def __init__(self, timeout_seconds: int) -> None:
        self.timeout_seconds = timeout_seconds
        super().__init__(f"Clustering timed out after {timeout_seconds}s")


class InvalidEmbeddingsError(ClusteringError):
    """Raised when embeddings contain NaN, Inf, or zero vectors."""


# --- Embedding errors ---

class EmbeddingError(SubworkerError):
    """Error during embedding generation."""


class ModelNotLoadedError(EmbeddingError):
    """Raised when the embedding model is not available."""


class OllamaConnectionError(EmbeddingError):
    """Raised when the Ollama remote API is unreachable."""

    def __init__(self, url: str, detail: str) -> None:
        self.url = url
        super().__init__(f"Ollama API at {url} failed: {detail}")


# --- Classification errors ---

class ClassificationError(SubworkerError):
    """Error during genre classification."""


class ModelArtifactNotFoundError(ClassificationError):
    """Raised when a required model artifact file is missing."""

    def __init__(self, path: str) -> None:
        self.path = path
        super().__init__(f"Model artifact not found: {path}")


# --- Run management errors ---

class ConcurrentRunError(SubworkerError):
    """Raised when a job+genre pair already has a running run."""


class IdempotencyMismatchError(SubworkerError):
    """Raised when an idempotency key was reused with a different payload."""


# --- Repository errors ---

class RepositoryError(SubworkerError):
    """Error during database operations."""


class RunNotFoundError(RepositoryError):
    """Raised when a requested run does not exist."""

    def __init__(self, run_id: int) -> None:
        self.run_id = run_id
        super().__init__(f"Run {run_id} not found")
