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
        import structlog
        logger = structlog.get_logger(__name__)

        # base_url is already a complete URL (e.g., http://recap-worker:9005/admin/genre-learning)
        endpoint = self.base_url
        logger.debug(
            "sending POST request",
            endpoint=endpoint,
            timeout_seconds=self.timeout_seconds,
        )
        try:
            response = await self._client.post(endpoint, json=payload)
            logger.debug(
                "received response",
                status_code=response.status_code,
                endpoint=endpoint,
            )
            response.raise_for_status()
            return response
        except httpx.HTTPStatusError as exc:
            logger.error(
                "HTTP error response",
                status_code=exc.response.status_code,
                endpoint=endpoint,
                response_text=exc.response.text[:500] if exc.response.text else None,
            )
            raise
        except httpx.RequestError as exc:
            logger.error(
                "HTTP request error",
                error=str(exc),
                error_type=type(exc).__name__,
                endpoint=endpoint,
            )
            raise

    async def close(self) -> None:
        await self._client.aclose()
