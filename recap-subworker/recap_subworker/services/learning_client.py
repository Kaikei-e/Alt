"""HTTP client for dispatching genre learning summaries to recap-worker."""

from __future__ import annotations

from dataclasses import dataclass
from typing import Any

import httpx


@dataclass
class LearningClient:
    """Simple wrapper around an httpx AsyncClient."""

    base_url: str
    timeout_seconds: float
    _client: httpx.AsyncClient

    @classmethod
    def create(cls, base_url: str, timeout_seconds: float) -> "LearningClient":
        client = httpx.AsyncClient(timeout=httpx.Timeout(timeout_seconds))
        sanitized = base_url.rstrip("/")
        return cls(base_url=sanitized, timeout_seconds=timeout_seconds, _client=client)

    async def send_learning_payload(self, payload: dict[str, Any]) -> httpx.Response:
        endpoint = f"{self.base_url}/admin/genre-learning"
        response = await self._client.post(endpoint, json=payload)
        response.raise_for_status()
        return response

    async def close(self) -> None:
        await self._client.aclose()
