"""Async SQLAlchemy session helpers with connection pooling.

Phase 1 refactor: ownership of the engine and session factory is delegated
to ``ServiceContainer``. This module only exposes a pure builder
(``create_database_resources``) and a dataclass that binds engine + factory
together with a lifecycle. Module-level singletons were removed.

Legacy ``get_session_factory(settings)`` is preserved as a thin wrapper
for subprocess callers (learning scheduler) that run outside the FastAPI
application and therefore cannot read from ``app.state``.
"""

from __future__ import annotations

from collections.abc import AsyncIterator
from dataclasses import dataclass

import structlog
from sqlalchemy.ext.asyncio import (
    AsyncEngine,
    AsyncSession,
    async_sessionmaker,
    create_async_engine,
)

from ..config import Settings

logger = structlog.get_logger(__name__)


@dataclass(slots=True)
class DatabaseResources:
    """Engine + session factory bound together with a lifecycle.

    Owned by ``ServiceContainer``. ``aclose()`` must be awaited on lifespan
    shutdown to release pooled connections cleanly.
    """

    engine: AsyncEngine
    session_factory: async_sessionmaker[AsyncSession]

    async def aclose(self) -> None:
        await self.engine.dispose()


def create_database_resources(settings: Settings) -> DatabaseResources:
    """Build a fresh engine and session factory from settings.

    No caching. The caller (ServiceContainer or a subprocess scheduler)
    owns the returned resources and must call ``aclose()`` at shutdown.
    """

    engine = create_async_engine(
        settings.db_url_str,
        pool_size=3,
        max_overflow=2,
        pool_timeout=30,
        pool_recycle=1800,
        pool_pre_ping=True,
        connect_args={
            "command_timeout": 60,
            "server_settings": {
                "application_name": "recap-subworker",
            },
        },
    )
    factory = async_sessionmaker(engine, expire_on_commit=False, class_=AsyncSession)
    logger.info(
        "database resources created",
        pool_size=3,
        max_overflow=2,
        pool_recycle=1800,
    )
    return DatabaseResources(engine=engine, session_factory=factory)


def get_session_factory(settings: Settings) -> async_sessionmaker[AsyncSession]:
    """Legacy builder for subprocess callers.

    NOTE: Every call returns a freshly-built factory bound to a new engine.
    Main-process code paths should instead use
    ``ServiceContainer.db.session_factory`` which is owned by the lifespan.
    """

    return create_database_resources(settings).session_factory


async def get_session() -> AsyncIterator[AsyncSession]:  # pragma: no cover - legacy
    """Legacy FastAPI dependency for callers that have not yet migrated.

    New code should inject ``ServiceContainer`` via ``deps.get_container``
    and take sessions from ``container.db.session_factory``.
    """
    from ..config import get_settings

    resources = create_database_resources(get_settings())
    try:
        async with resources.session_factory() as session:
            yield session
    finally:
        await resources.aclose()
