"""Ollama HTTP client driver."""

import json
import logging
from typing import Dict, Any, Optional
import aiohttp

from news_creator.config.config import NewsCreatorConfig

logger = logging.getLogger(__name__)


class OllamaDriver:
    """HTTP client for Ollama API."""

    def __init__(self, config: NewsCreatorConfig):
        """Initialize Ollama driver with configuration."""
        self.config = config
        self.session: Optional[aiohttp.ClientSession] = None

    async def initialize(self) -> None:
        """Initialize HTTP client session."""
        # より詳細なタイムアウト設定
        timeout = aiohttp.ClientTimeout(
            total=self.config.llm_timeout_seconds,
            connect=30,  # 接続タイムアウト
            sock_read=120,  # ソケット読み取りタイムアウト（LLM生成時間を考慮）
        )
        self.session = aiohttp.ClientSession(timeout=timeout)
        logger.info("Ollama driver initialized", extra={"url": self.config.llm_service_url})

    async def cleanup(self) -> None:
        """Cleanup HTTP client session."""
        if self.session and not self.session.closed:
            await self.session.close()
            logger.info("Ollama driver cleaned up")

    async def generate(self, payload: Dict[str, Any]) -> Dict[str, Any]:
        """
        Call Ollama generate API.

        Args:
            payload: Request payload for /api/generate endpoint

        Returns:
            Response dictionary from Ollama

        Raises:
            ValueError: If payload is invalid
            RuntimeError: If Ollama service returns error
        """
        if not payload.get("prompt"):
            raise ValueError("payload must contain 'prompt'")

        if self.session is None or self.session.closed:
            await self.initialize()

        url = f"{self.config.llm_service_url.rstrip('/')}/api/generate"
        logger.debug("Calling Ollama API", extra={"url": url, "model": payload.get("model")})

        try:
            async with self.session.post(url, json=payload) as response:
                text_body = await response.text()

                if response.status != 200:
                    logger.error(
                        "Ollama API returned error",
                        extra={
                            "status": response.status,
                            "body": text_body[:500],
                        },
                    )
                    raise RuntimeError(f"Ollama API error: HTTP {response.status}")

                try:
                    return json.loads(text_body)
                except json.JSONDecodeError as err:
                    logger.error("Failed to decode Ollama response", extra={"error": str(err)})
                    raise RuntimeError("Failed to decode Ollama response") from err

        except aiohttp.ClientError as err:
            logger.error("Ollama API request failed", extra={"error": str(err)})
            raise RuntimeError(f"Ollama API request failed: {err}") from err

    async def list_tags(self) -> Dict[str, Any]:
        """
        Call Ollama tags API to list available models.

        Returns:
            Response dictionary from Ollama containing models list

        Raises:
            RuntimeError: If Ollama service returns error
        """
        if self.session is None or self.session.closed:
            await self.initialize()

        url = f"{self.config.llm_service_url.rstrip('/')}/api/tags"
        logger.debug("Calling Ollama tags API", extra={"url": url})

        try:
            async with self.session.get(url) as response:
                text_body = await response.text()

                if response.status != 200:
                    logger.error(
                        "Ollama tags API returned error",
                        extra={
                            "status": response.status,
                            "body": text_body[:500],
                        },
                    )
                    raise RuntimeError(f"Ollama tags API error: HTTP {response.status}")

                try:
                    return json.loads(text_body)
                except json.JSONDecodeError as err:
                    logger.error("Failed to decode Ollama tags response", extra={"error": str(err)})
                    raise RuntimeError("Failed to decode Ollama tags response") from err

        except aiohttp.ClientError as err:
            logger.error("Ollama tags API request failed", extra={"error": str(err)})
            raise RuntimeError(f"Ollama tags API request failed: {err}") from err
