"""Type definitions used by tag insertion ports."""

from typing import TypedDict


class DatabaseError(Exception):
    """Custom exception for database-related errors."""

    pass


class BatchResult(TypedDict):
    success: bool
    processed_articles: int
    failed_articles: int
    errors: list[str]
    message: str | None
