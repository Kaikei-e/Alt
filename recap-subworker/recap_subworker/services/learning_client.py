"""HTTP client for dispatching genre learning summaries to recap-worker."""

from __future__ import annotations

import asyncio
from dataclasses import dataclass
from typing import Any

import httpx

from ..domain.errors import LearningClientTimeoutError

# Per-stage httpx timeout budget. connect is kept short so an unreachable
# recap-worker surfaces quickly; read absorbs slow downstream Bayesian
# optimization bursts; write/pool are modest constants.
# Total budget is bounded by an outer asyncio.timeout() at the call site.
_CONNECT_TIMEOUT_SECONDS = 2.0
_WRITE_TIMEOUT_SECONDS = 10.0
_POOL_TIMEOUT_SECONDS = 5.0


def _build_timeout(read_timeout_seconds: float) -> httpx.Timeout:
    return httpx.Timeout(
        connect=_CONNECT_TIMEOUT_SECONDS,
        read=read_timeout_seconds,
        write=_WRITE_TIMEOUT_SECONDS,
        pool=_POOL_TIMEOUT_SECONDS,
    )


@dataclass
class LearningClient:
    """Simple wrapper around an httpx AsyncClient."""

    base_url: str
    timeout_seconds: float
    _client: httpx.AsyncClient

    @classmethod
    def create(cls, base_url: str, timeout_seconds: float) -> LearningClient:
        # Enforce a floor on the read stage so the connect < read invariant
        # from the Phase 5 tests holds even if a caller passes a tiny budget.
        read_timeout = max(timeout_seconds, _CONNECT_TIMEOUT_SECONDS + 0.5)
        client = httpx.AsyncClient(timeout=_build_timeout(read_timeout))
        sanitized = base_url.rstrip("/")
        return cls(
            base_url=sanitized,
            timeout_seconds=read_timeout,
            _client=client,
        )

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
        # Outer asyncio budget: double the read budget so a misbehaving
        # peer is cancelled even if the per-stage timeouts collude.
        outer_budget = self.timeout_seconds * 2.0
        try:
            async with asyncio.timeout(outer_budget):
                response = await self._client.post(endpoint, json=payload)
            logger.debug(
                "received response",
                status_code=response.status_code,
                endpoint=endpoint,
            )
            response.raise_for_status()
            return response
        except (TimeoutError, httpx.TimeoutException) as exc:
            logger.error(
                "learning client timed out",
                endpoint=endpoint,
                outer_budget_seconds=outer_budget,
                error_type=type(exc).__name__,
            )
            raise LearningClientTimeoutError(
                f"{endpoint} exceeded {outer_budget:.1f}s budget"
            ) from exc
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
