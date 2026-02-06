"""Cache port: Protocol for key-value caching."""

from __future__ import annotations

from typing import Hashable, Protocol, TypeVar, runtime_checkable

K = TypeVar("K", bound=Hashable, contravariant=True)
V = TypeVar("V", covariant=True)


@runtime_checkable
class CachePort(Protocol):
    """Port for a bounded key-value cache.

    Note: Uses plain Protocol without generic params to keep structural
    subtyping simple. Concrete types are checked at the call site.
    """

    def get(self, key: Hashable) -> object | None:
        """Retrieve a cached value, or None if absent."""
        ...

    def set(self, key: Hashable, value: object) -> None:
        """Store a value in the cache, evicting oldest if full."""
        ...

    def __len__(self) -> int:
        """Return the number of cached entries."""
        ...
