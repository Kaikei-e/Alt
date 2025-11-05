"""JSON helpers wrapping orjson."""

from __future__ import annotations

import orjson


def dumps(value) -> str:
    """Serialize python object to JSON string."""

    return orjson.dumps(value).decode()


def dumps_bytes(value) -> bytes:
    """Serialize python object to JSON bytes."""

    return orjson.dumps(value)
