"""Async SQLAlchemy session helpers (compat shim).

Re-exports the Phase 1 ``DatabaseResources`` API from ``infra.db.session``.
Retained for backwards compatibility with existing imports.
"""

from __future__ import annotations

from ..infra.db.session import (
    DatabaseResources,
    create_database_resources,
    get_session,
    get_session_factory,
)

__all__ = [
    "DatabaseResources",
    "create_database_resources",
    "get_session",
    "get_session_factory",
]
