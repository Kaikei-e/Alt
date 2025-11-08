"""Async SQLAlchemy session helpers."""

from __future__ import annotations

from typing import AsyncIterator

from sqlalchemy.ext.asyncio import AsyncEngine, AsyncSession, create_async_engine
from sqlalchemy.orm import sessionmaker

from ..infra.config import Settings


_ENGINE: AsyncEngine | None = None
_SESSION_FACTORY: sessionmaker | None = None


def get_engine(settings: Settings) -> AsyncEngine:
    """Return a lazily initialized async engine."""

    global _ENGINE
    if _ENGINE is None:
        _ENGINE = create_async_engine(settings.db_url, pool_pre_ping=True)
    return _ENGINE


def get_session_factory(settings: Settings) -> sessionmaker:
    """Return a session factory bound to the configured engine."""

    global _SESSION_FACTORY
    if _SESSION_FACTORY is None:
        engine = get_engine(settings)
        _SESSION_FACTORY = sessionmaker(engine, expire_on_commit=False, class_=AsyncSession)
    return _SESSION_FACTORY


async def get_session(settings: Settings) -> AsyncIterator[AsyncSession]:
    """FastAPI dependency that yields an AsyncSession."""

    factory = get_session_factory(settings)
    async with factory() as session:
        yield session
