"""Async database connection management using psycopg3 connection pool."""

from __future__ import annotations

import os
from typing import TYPE_CHECKING

import structlog
from psycopg_pool import AsyncConnectionPool

from tag_generator.domain.errors import DatabaseConnectionError

if TYPE_CHECKING:
    from tag_generator.infra.config import DatabaseConfig

logger = structlog.get_logger(__name__)


class AsyncDatabaseManager:
    """Manages async database connections via psycopg3 AsyncConnectionPool."""

    def __init__(self, config: DatabaseConfig | None = None) -> None:
        self._pool: AsyncConnectionPool | None = None
        self._config = config

    def _build_dsn(self) -> str:
        """Build DSN from config or environment variables."""
        if self._config:
            password = self._config.tag_generator_password
            if not password and self._config.tag_generator_password_file:
                try:
                    with open(self._config.tag_generator_password_file) as f:
                        password = f.read().strip()
                except (OSError, ValueError) as e:
                    logger.error("Failed to read password file", error=str(e))
                    raise DatabaseConnectionError("Failed to read password file") from e

            return (
                f"postgresql://{self._config.tag_generator_user}:"
                f"{password}@"
                f"{self._config.host}:{self._config.port}/"
                f"{self._config.name}"
                f"?sslmode={self._config.sslmode}"
            )

        # Fallback to environment variables for backward compatibility
        password: str | None = os.getenv("DB_TAG_GENERATOR_PASSWORD")
        password_file = os.getenv("DB_TAG_GENERATOR_PASSWORD_FILE")
        if not password and password_file:
            try:
                with open(password_file) as f:
                    password = f.read().strip()
            except (OSError, ValueError) as e:
                logger.error("Failed to read password file", error=str(e))

        if not password:
            raise DatabaseConnectionError("No database password configured")

        return (
            f"postgresql://{os.getenv('DB_TAG_GENERATOR_USER', 'tag_generator')}:"
            f"{password}@"
            f"{os.getenv('DB_HOST', 'localhost')}:{os.getenv('DB_PORT', '5432')}/"
            f"{os.getenv('DB_NAME', 'alt')}"
        )

    async def initialize(self, min_size: int = 2, max_size: int = 10) -> None:
        """Open the connection pool."""
        dsn = self._build_dsn()
        self._pool = AsyncConnectionPool(
            conninfo=dsn,
            min_size=min_size,
            max_size=max_size,
            open=False,
        )
        await self._pool.open()
        logger.info("Async database pool opened", min_size=min_size, max_size=max_size)

    async def close(self) -> None:
        """Close the connection pool."""
        if self._pool:
            await self._pool.close()
            logger.info("Async database pool closed")

    @property
    def pool(self) -> AsyncConnectionPool:
        """Return the connection pool, raising if not initialized."""
        if self._pool is None:
            raise DatabaseConnectionError("Database pool not initialized. Call initialize() first.")
        return self._pool
