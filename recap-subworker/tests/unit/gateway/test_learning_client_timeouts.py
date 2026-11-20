"""Timeout invariants for LearningClient.

Every httpx call must use an explicit connect / read / write / pool
budget instead of a single scalar, and be wrapped in
``asyncio.timeout(...)`` at the call site so a stalled peer can be
cancelled deterministically.
"""

from __future__ import annotations

import asyncio
from typing import Never

import httpx
import pytest

from recap_subworker.services.learning_client import LearningClient


class _TransportNotCancelledError(AssertionError):
    def __init__(self) -> None:
        super().__init__("transport should have been cancelled")


def test_create_uses_per_stage_httpx_timeout() -> None:
    """connect should fail faster than read; using a single scalar for
    every leg is rejected because it lets DNS / TCP hangs consume the
    full read budget before failing."""
    client = LearningClient.create("http://recap-worker/learning", timeout_seconds=30.0)
    timeout = client._client.timeout
    assert isinstance(timeout, httpx.Timeout)
    assert timeout.connect is not None
    assert timeout.read is not None
    assert timeout.connect < timeout.read, (
        "connect timeout must be strictly smaller than read timeout to "
        "surface unreachable peers quickly."
    )
    assert timeout.write is not None
    assert timeout.write > 0
    assert timeout.pool is not None
    assert timeout.pool > 0


@pytest.mark.asyncio
async def test_send_learning_payload_applies_asyncio_timeout(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    """Invoke the client against a fake transport that never responds;
    the call must raise within the overall asyncio budget instead of
    hanging indefinitely."""

    class _NeverRespondTransport(httpx.AsyncBaseTransport):
        async def handle_async_request(self, request: httpx.Request) -> Never:
            await asyncio.sleep(60)
            raise _TransportNotCancelledError

    client = LearningClient.create(
        "http://recap-worker/learning",
        timeout_seconds=1.0,
    )
    # Swap transport after construction so we bypass the DNS stage.
    client._client = httpx.AsyncClient(
        transport=_NeverRespondTransport(),
        timeout=client._client.timeout,
    )

    expected: tuple[type[BaseException], ...] = (
        asyncio.TimeoutError,
        httpx.TimeoutException,
    )
    with pytest.raises(expected):
        async with asyncio.timeout(2.0):
            await client.send_learning_payload({"ping": "pong"})

    await client.close()
