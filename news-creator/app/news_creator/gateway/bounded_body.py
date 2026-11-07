"""Bounded HTTP response body reader for remote OpenAI-compatible gateways."""

from __future__ import annotations

from typing import Any


class BoundedBodyError(Exception):
    """Raised when an HTTP response body exceeds the configured byte cap."""


async def read_bounded_text(response: Any, cap: int) -> str:
    """Read at most `cap` bytes from `response.content` and decode as UTF-8."""
    chunks: list[bytes] = []
    total = 0
    while total <= cap:
        chunk = await response.content.read(cap + 1 - total)
        if not chunk:
            break
        chunks.append(chunk)
        total += len(chunk)

    if total > cap:
        raise BoundedBodyError(f"Response body exceeds {cap} bytes")

    return b"".join(chunks).decode("utf-8", errors="replace")
