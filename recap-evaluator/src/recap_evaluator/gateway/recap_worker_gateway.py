"""Recap-Worker API gateway — implements RecapWorkerPort."""

from typing import Any

import httpx
import structlog

from recap_evaluator.config import Settings

logger = structlog.get_logger()


class RecapWorkerGateway:
    """HTTP client for recap-worker API."""

    def __init__(self, client: httpx.AsyncClient, settings: Settings) -> None:
        self._client = client
        self._base_url = settings.recap_worker_url

    async def trigger_genre_evaluation(self) -> dict[str, Any] | None:
        try:
            response = await self._client.post(
                f"{self._base_url}/v1/evaluation/genres",
            )
            response.raise_for_status()
            return response.json()
        except httpx.HTTPStatusError as e:
            logger.error(
                "Failed to trigger genre evaluation",
                status_code=e.response.status_code,
            )
            return None
        except Exception as e:
            logger.error("Genre evaluation request failed", error=str(e))
            return None

    async def fetch_latest_genre_evaluation(self) -> dict[str, Any] | None:
        try:
            response = await self._client.get(
                f"{self._base_url}/v1/evaluation/genres/latest",
            )
            response.raise_for_status()
            return response.json()
        except httpx.HTTPStatusError as e:
            logger.error(
                "Failed to fetch genre evaluation",
                status_code=e.response.status_code,
            )
            return None
        except Exception as e:
            logger.error("Genre evaluation fetch failed", error=str(e))
            return None

    async def fetch_genre_evaluation_by_id(
        self, run_id: str
    ) -> dict[str, Any] | None:
        try:
            response = await self._client.get(
                f"{self._base_url}/v1/evaluation/genres/{run_id}",
            )
            response.raise_for_status()
            return response.json()
        except httpx.HTTPStatusError as e:
            logger.error(
                "Failed to fetch genre evaluation",
                run_id=run_id,
                status_code=e.response.status_code,
            )
            return None
        except Exception as e:
            logger.error(
                "Genre evaluation fetch failed", run_id=run_id, error=str(e)
            )
            return None
