"""Async SQLAlchemy session helpers with connection pooling.

Replaces NullPool with proper connection pooling for better performance.
uvicorn does not fork workers, so connection pooling is safe within
a single process. PID detection logic is preserved for Gunicorn compatibility.
"""

from __future__ import annotations

import os
from typing import AsyncIterator

import structlog
from sqlalchemy.ext.asyncio import AsyncEngine, AsyncSession, create_async_engine
from sqlalchemy.orm import sessionmaker

from ..config import Settings, get_settings

logger = structlog.get_logger(__name__)

_ENGINE: AsyncEngine | None = None
_SESSION_FACTORY: sessionmaker | None = None
_ENGINE_PID: int | None = None


def get_engine(settings: Settings) -> AsyncEngine:
    """Return a lazily initialized async engine with connection pooling."""

    global _ENGINE, _ENGINE_PID
    current_pid = os.getpid()

    if _ENGINE is not None and _ENGINE_PID != current_pid:
        logger.info(
            "detected pid change, resetting database engine",
            old_pid=_ENGINE_PID,
            new_pid=current_pid,
        )
        _ENGINE = None
        _ENGINE_PID = None

    if _ENGINE is None:
        _ENGINE = create_async_engine(
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
        _ENGINE_PID = current_pid
        logger.info(
            "database engine created with connection pooling",
            pool_size=3,
            max_overflow=2,
            pool_recycle=1800,
            pid=current_pid,
        )

    return _ENGINE


def get_session_factory(settings: Settings) -> sessionmaker:
    """Return a session factory bound to the configured engine."""

    global _SESSION_FACTORY, _ENGINE_PID

    current_pid = os.getpid()

    if _SESSION_FACTORY is not None and _ENGINE_PID is not None and _ENGINE_PID != current_pid:
        logger.info(
            "detected pid change, resetting session factory",
            old_pid=_ENGINE_PID,
            new_pid=current_pid,
        )
        _SESSION_FACTORY = None

    if _SESSION_FACTORY is None:
        engine = get_engine(settings)
        _SESSION_FACTORY = sessionmaker(engine, expire_on_commit=False, class_=AsyncSession)
    return _SESSION_FACTORY


async def get_session() -> AsyncIterator[AsyncSession]:
    """FastAPI dependency that yields an AsyncSession."""
    settings = get_settings()
    factory = get_session_factory(settings)
    async with factory() as session:
        yield session
