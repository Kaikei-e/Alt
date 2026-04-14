"""Phase 5B: narrow exception boundaries for LearningClient.

When the outer asyncio budget trips or the underlying httpx call times
out, the service must raise a domain-specific
``LearningClientTimeoutError`` so upstream handlers can map it to a
503 without catching the bare asyncio / httpx hierarchy.
"""

from __future__ import annotations

import asyncio

import httpx
import pytest

from recap_subworker.domain.errors import LearningClientTimeoutError
from recap_subworker.services.learning_client import LearningClient


@pytest.mark.asyncio
async def test_asyncio_timeout_is_translated_to_domain_error() -> None:
    class _NeverRespondTransport(httpx.AsyncBaseTransport):
        async def handle_async_request(self, request: httpx.Request) -> httpx.Response:
            await asyncio.sleep(60)
            raise AssertionError("transport should have been cancelled")

    client = LearningClient.create(
        "http://recap-worker/learning",
        timeout_seconds=1.0,
    )
    client._client = httpx.AsyncClient(
        transport=_NeverRespondTransport(),
        timeout=client._client.timeout,
    )

    with pytest.raises(LearningClientTimeoutError):
        await client.send_learning_payload({"ping": "pong"})

    await client.close()
