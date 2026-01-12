"""Port interface for caching."""

from abc import ABC, abstractmethod
from typing import Optional


class CachePort(ABC):
    """Abstract interface for caching services.

    Used to reduce LLM API calls by caching recap summary results.
    """

    @abstractmethod
    async def get(self, key: str) -> Optional[str]:
        """
        Retrieve a cached value by key.

        Args:
            key: Cache key

        Returns:
            Cached value as string, or None if not found
        """
        pass

    @abstractmethod
    async def set(self, key: str, value: str, ttl_seconds: Optional[int] = None) -> bool:
        """
        Store a value in the cache.

        Args:
            key: Cache key
            value: Value to cache (serialized as string)
            ttl_seconds: Optional TTL in seconds (None = use default)

        Returns:
            True if successfully cached, False otherwise
        """
        pass

    @abstractmethod
    async def delete(self, key: str) -> bool:
        """
        Delete a cached value.

        Args:
            key: Cache key to delete

        Returns:
            True if key was deleted, False if key didn't exist
        """
        pass

    @abstractmethod
    async def initialize(self) -> None:
        """Initialize the cache connection."""
        pass

    @abstractmethod
    async def cleanup(self) -> None:
        """Cleanup cache connection resources."""
        pass
