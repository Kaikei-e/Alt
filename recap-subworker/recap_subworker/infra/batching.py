"""Utilities for batching sentences based on token budgets."""

from __future__ import annotations

from collections.abc import Iterable, Iterator, Sequence
from typing import TypeVar

T = TypeVar("T")


def sliding_batches[T](items: Sequence[T], batch_size: int) -> Iterator[Sequence[T]]:
    """Yield slices of *items* with at most *batch_size* elements."""

    if batch_size <= 0:
        raise ValueError("batch_size must be > 0")
    for index in range(0, len(items), batch_size):
        yield items[index : index + batch_size]


def adaptive_batches[T](
    items: Sequence[T], estimate_tokens: Iterable[int], budget: int
) -> Iterator[list[T]]:
    """Yield batches where the sum of estimated tokens stays under *budget*."""

    current: list[T] = []
    current_tokens = 0
    for item, tokens in zip(items, estimate_tokens, strict=False):
        if tokens > budget:
            if current:
                yield current
                current = []
                current_tokens = 0
            yield [item]
            continue
        if current_tokens + tokens > budget and current:
            yield current
            current = []
            current_tokens = 0
        current.append(item)
        current_tokens += tokens
    if current:
        yield current
