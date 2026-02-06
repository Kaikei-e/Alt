"""Domain value objects for recap-subworker.

Value objects are immutable, equality-by-value types that encapsulate
domain concepts with validation.
"""

from __future__ import annotations

from dataclasses import dataclass


@dataclass(frozen=True, slots=True)
class SentenceText:
    """A validated sentence text that meets minimum length requirements."""

    text: str

    def __post_init__(self) -> None:
        if len(self.text) < 2:
            raise ValueError(f"Sentence text must be at least 2 characters, got {len(self.text)}")

    def __str__(self) -> str:
        return self.text

    def __len__(self) -> int:
        return len(self.text)


@dataclass(frozen=True, slots=True)
class GenreName:
    """A validated genre name."""

    value: str

    def __post_init__(self) -> None:
        if not self.value or len(self.value) > 32:
            raise ValueError(f"Genre name must be 1-32 characters, got '{self.value}'")

    def __str__(self) -> str:
        return self.value


@dataclass(frozen=True, slots=True)
class IdempotencyKey:
    """An idempotency key for deduplicating requests."""

    value: str

    def __post_init__(self) -> None:
        if not self.value:
            raise ValueError("Idempotency key cannot be empty")

    def __str__(self) -> str:
        return self.value
