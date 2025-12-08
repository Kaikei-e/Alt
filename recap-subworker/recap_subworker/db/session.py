"""Async SQLAlchemy session helpers."""

from __future__ import annotations

from typing import AsyncIterator

from sqlalchemy.ext.asyncio import AsyncEngine, AsyncSession, create_async_engine
from sqlalchemy.orm import sessionmaker

from ..infra.config import Settings, get_settings


_ENGINE: AsyncEngine | None = None
_SESSION_FACTORY: sessionmaker | None = None


import os

# Store the PID where the engine was created
_ENGINE_PID: int | None = None

def get_engine(settings: Settings) -> AsyncEngine:
    """Return a lazily initialized async engine."""

    global _ENGINE, _ENGINE_PID
    current_pid = os.getpid()

    # If engine exists but was created in a different process (PID mismatch),
    # discard it and create a new one. This handles forking (e.g. Gunicorn).
    if _ENGINE is not None and _ENGINE_PID != current_pid:
        import structlog
        logger = structlog.get_logger(__name__)
        logger.info(
            "detected pid change, resetting database engine",
            old_pid=_ENGINE_PID,
            new_pid=current_pid
        )
        # We can't safely dispose the old engine because it belongs to another loop/process
        # Just dereference it.
        _ENGINE = None
        _ENGINE_PID = None

    from sqlalchemy.pool import NullPool

    if _ENGINE is None:
        # NullPoolを使用: コネクションプールを無効化し、リクエストごとに新しい接続を作成・破棄する
        # これにより "Task attached to a different loop" エラー（イベントループの不一致）を回避する
        # パフォーマンスへの影響はあるが、各ワーカー/非同期タスクの独立性を保証する最も確実な方法
        _ENGINE = create_async_engine(
            settings.db_url,
            poolclass=NullPool,
            # asyncpg固有の設定: 接続タイムアウトとコマンドタイムアウトを設定
            connect_args={
                "command_timeout": 60,  # コマンド実行のタイムアウト（秒）
                "server_settings": {
                    "application_name": "recap-subworker",
                },
            },
        )
        _ENGINE_PID = current_pid

    return _ENGINE


def get_session_factory(settings: Settings) -> sessionmaker:
    """Return a session factory bound to the configured engine."""

    global _SESSION_FACTORY, _ENGINE_PID

    current_pid = os.getpid()

    # Check if factory was inherited from a parent process
    if _SESSION_FACTORY is not None and _ENGINE_PID is not None and _ENGINE_PID != current_pid:
        import structlog
        logger = structlog.get_logger(__name__)
        logger.info(
            "detected pid change, resetting session factory",
            old_pid=_ENGINE_PID,
            new_pid=current_pid
        )
        _SESSION_FACTORY = None

    if _SESSION_FACTORY is None:
        engine = get_engine(settings)
        _SESSION_FACTORY = sessionmaker(engine, expire_on_commit=False, class_=AsyncSession)
    return _SESSION_FACTORY


async def get_session() -> AsyncIterator[AsyncSession]:
    """FastAPI dependency that yields an AsyncSession."""
    import structlog

    logger = structlog.get_logger(__name__)
    logger.debug("creating database session")
    settings = get_settings()
    factory = get_session_factory(settings)
    async with factory() as session:
        logger.debug("database session created")
        yield session
        logger.debug("database session closed")
