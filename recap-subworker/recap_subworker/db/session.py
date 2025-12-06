"""Async SQLAlchemy session helpers."""

from __future__ import annotations

from typing import AsyncIterator

from sqlalchemy.ext.asyncio import AsyncEngine, AsyncSession, create_async_engine
from sqlalchemy.orm import sessionmaker

from ..infra.config import Settings, get_settings


_ENGINE: AsyncEngine | None = None
_SESSION_FACTORY: sessionmaker | None = None


def get_engine(settings: Settings) -> AsyncEngine:
    """Return a lazily initialized async engine."""

    global _ENGINE
    if _ENGINE is None:
        # 接続プール設定: ワーカー数が多い場合でもPostgreSQLの接続上限を超えないように
        # pool_size=5, max_overflow=5 で各ワーカーが最大10接続まで使用可能
        # ワーカー数が9の場合、最大90接続となり、recap-workerの100接続と合わせても190接続で、
        # recap-dbのmax_connections=250の範囲内に収まる
        _ENGINE = create_async_engine(
            settings.db_url,
            pool_pre_ping=True,
            pool_size=5,  # 基本接続プールサイズ
            max_overflow=5,  # オーバーフロー接続数
            pool_recycle=1800,  # 接続の最大ライフタイム（30分）- イベントループエラーを防ぐ
            pool_timeout=30,  # 接続取得のタイムアウト（秒）
            # asyncpg固有の設定: 接続タイムアウトとコマンドタイムアウトを設定
            connect_args={
                "command_timeout": 60,  # コマンド実行のタイムアウト（秒）
                "server_settings": {
                    "application_name": "recap-subworker",
                },
            },
        )
    return _ENGINE


def get_session_factory(settings: Settings) -> sessionmaker:
    """Return a session factory bound to the configured engine."""

    global _SESSION_FACTORY
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
