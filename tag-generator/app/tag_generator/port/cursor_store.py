"""Port for cursor position persistence."""

from __future__ import annotations

from typing import Any, Protocol


class CursorStorePort(Protocol):
    """Port for managing cursor positions for article pagination."""

    def get_initial_cursor_position(self) -> tuple[str, str]: ...

    def get_forward_cursor_position(self, conn: Any) -> tuple[str, str]: ...

    def update_cursor_position(self, created_at: str, article_id: str) -> None: ...

    def update_forward_cursor_position(self, created_at: str, article_id: str) -> None: ...
