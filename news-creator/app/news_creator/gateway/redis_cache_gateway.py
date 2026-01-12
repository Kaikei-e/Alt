"""Redis Cache Gateway - implements CachePort."""

import logging
from typing import Optional

try:
    import redis.asyncio as redis
except ImportError:
    redis = None  # type: ignore

from news_creator.config.config import NewsCreatorConfig
from news_creator.port.cache_port import CachePort

logger = logging.getLogger(__name__)


class RedisCacheGateway(CachePort):
    """Gateway for Redis caching service - Anti-Corruption Layer."""

    def __init__(self, config: NewsCreatorConfig):
        """Initialize Redis cache gateway.

        Args:
            config: NewsCreator configuration containing Redis settings
        """
        self.config = config
        self._client: Optional["redis.Redis"] = None
        self._enabled = config.cache_enabled

        if not self._enabled:
            logger.info("Redis cache is disabled")
        elif redis is None:
            logger.warning("redis package not installed, cache will be disabled")
            self._enabled = False

    async def initialize(self) -> None:
        """Initialize the Redis connection."""
        if not self._enabled:
            logger.info("Cache disabled, skipping Redis initialization")
            return

        try:
            self._client = redis.Redis.from_url(
                self.config.cache_redis_url,
                decode_responses=True,
            )
            # Test connection
            await self._client.ping()
            logger.info(
                "Redis cache gateway initialized",
                extra={"url": self._sanitize_url(self.config.cache_redis_url)},
            )
        except Exception as e:
            logger.error(
                "Failed to connect to Redis, cache will be disabled",
                extra={"error": str(e)},
            )
            self._enabled = False
            self._client = None

    async def cleanup(self) -> None:
        """Cleanup Redis connection resources."""
        if self._client:
            await self._client.close()
            logger.info("Redis cache gateway cleaned up")

    async def get(self, key: str) -> Optional[str]:
        """
        Retrieve a cached value by key.

        Args:
            key: Cache key

        Returns:
            Cached value as string, or None if not found or cache disabled
        """
        if not self._enabled or not self._client:
            return None

        try:
            value = await self._client.get(key)
            if value is not None:
                logger.debug("Cache hit", extra={"key": key})
            else:
                logger.debug("Cache miss", extra={"key": key})
            return value
        except Exception as e:
            logger.warning(
                "Cache get failed",
                extra={"key": key, "error": str(e)},
            )
            return None

    async def set(
        self, key: str, value: str, ttl_seconds: Optional[int] = None
    ) -> bool:
        """
        Store a value in the cache.

        Args:
            key: Cache key
            value: Value to cache (serialized as string)
            ttl_seconds: Optional TTL in seconds (None = use default)

        Returns:
            True if successfully cached, False otherwise
        """
        if not self._enabled or not self._client:
            return False

        ttl = ttl_seconds if ttl_seconds is not None else self.config.cache_ttl_seconds

        try:
            await self._client.set(key, value, ex=ttl)
            logger.debug(
                "Cache set",
                extra={"key": key, "ttl_seconds": ttl, "value_length": len(value)},
            )
            return True
        except Exception as e:
            logger.warning(
                "Cache set failed",
                extra={"key": key, "error": str(e)},
            )
            return False

    async def delete(self, key: str) -> bool:
        """
        Delete a cached value.

        Args:
            key: Cache key to delete

        Returns:
            True if key was deleted, False if key didn't exist or cache disabled
        """
        if not self._enabled or not self._client:
            return False

        try:
            result = await self._client.delete(key)
            deleted = result > 0
            logger.debug(
                "Cache delete",
                extra={"key": key, "deleted": deleted},
            )
            return deleted
        except Exception as e:
            logger.warning(
                "Cache delete failed",
                extra={"key": key, "error": str(e)},
            )
            return False

    @staticmethod
    def _sanitize_url(url: str) -> str:
        """Remove password from URL for logging."""
        if "@" in url:
            # URL format: redis://user:password@host:port/db
            parts = url.split("@")
            return f"redis://***@{parts[-1]}"
        return url


class NullCacheGateway(CachePort):
    """Null implementation of CachePort for when caching is disabled."""

    async def initialize(self) -> None:
        """No-op initialization."""
        pass

    async def cleanup(self) -> None:
        """No-op cleanup."""
        pass

    async def get(self, key: str) -> Optional[str]:
        """Always returns None (cache disabled)."""
        return None

    async def set(
        self, key: str, value: str, ttl_seconds: Optional[int] = None
    ) -> bool:
        """Always returns False (cache disabled)."""
        return False

    async def delete(self, key: str) -> bool:
        """Always returns False (cache disabled)."""
        return False
