"""JSON helpers wrapping orjson."""

from __future__ import annotations

import orjson


def dumps(value: object) -> str:
    """Serialize python object to JSON string."""

    return orjson.dumps(value).decode()


def dumps_bytes(value: object) -> bytes:
    """Serialize python object to JSON bytes."""

    return orjson.dumps(value)
