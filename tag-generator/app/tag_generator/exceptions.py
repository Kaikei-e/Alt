"""Domain exception hierarchy for the tag-generator service.

Provides typed exceptions for distinct failure categories, enabling
callers to handle errors with appropriate granularity instead of
catching bare ``Exception``.
"""


class TagGeneratorError(Exception):
    """Base exception for all tag-generator domain errors."""


class TagExtractionError(TagGeneratorError):
    """Tag extraction failed for a single article."""


class ModelLoadError(TagGeneratorError):
    """ML model loading or initialisation failed."""


class BatchProcessingError(TagGeneratorError):
    """Batch processing failed (partial or complete)."""


class DatabaseConnectionError(TagGeneratorError):
    """Database connection could not be established."""


class CursorError(TagGeneratorError):
    """Cursor pagination error (poisoned, missing, or invalid)."""
