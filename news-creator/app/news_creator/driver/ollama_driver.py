"""Ollama HTTP client driver."""

import asyncio
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
        Call Ollama generate API with retry logic.

        Args:
            payload: Request payload for /api/generate endpoint

        Returns:
            Response dictionary from Ollama

        Raises:
            ValueError: If payload is invalid
            RuntimeError: If Ollama service returns error after retries
        """
        if not payload.get("prompt"):
            raise ValueError("payload must contain 'prompt'")

        if self.session is None or self.session.closed:
            await self.initialize()

        url = f"{self.config.llm_service_url.rstrip('/')}/api/generate"
        model = payload.get("model", "unknown")

        # Retry configuration
        max_retries = 3
        base_delay = 1.0  # seconds

        for attempt in range(max_retries + 1):
            try:
                if attempt > 0:
                    # Exponential backoff with jitter
                    delay = base_delay * (2 ** (attempt - 1))
                    jitter = delay * 0.1  # 10% jitter
                    wait_time = delay + jitter
                    logger.info(
                        f"Retrying Ollama API call (attempt {attempt + 1}/{max_retries + 1})",
                        extra={
                            "url": url,
                            "model": model,
                            "wait_time_seconds": wait_time,
                        },
                    )
                    await asyncio.sleep(wait_time)
                else:
                    logger.debug("Calling Ollama API", extra={"url": url, "model": model})

                async with self.session.post(url, json=payload) as response:
                    text_body = await response.text()

                    if response.status != 200:
                        error_msg = (
                            f"Ollama API returned error: HTTP {response.status}. "
                            f"Response body: {text_body[:500]}"
                        )
                        logger.error(
                            error_msg,
                            extra={
                                "status": response.status,
                                "body": text_body[:500],
                                "url": url,
                                "model": model,
                                "attempt": attempt + 1,
                            },
                        )

                        # Retry on 5xx errors (server errors) or 502/503 (bad gateway/service unavailable)
                        if response.status >= 500 or response.status in (502, 503):
                            if attempt < max_retries:
                                logger.warning(
                                    f"Retryable error {response.status}, will retry",
                                    extra={
                                        "status": response.status,
                                        "attempt": attempt + 1,
                                        "max_retries": max_retries + 1,
                                    },
                                )
                                continue

                        raise RuntimeError(f"Ollama API error: HTTP {response.status} - {text_body[:200]}")

                    try:
                        return json.loads(text_body)
                    except json.JSONDecodeError as err:
                        error_msg = f"Failed to decode Ollama response: {str(err)}. Body: {text_body[:200]}"
                        logger.error(error_msg, extra={"error": str(err), "body_preview": text_body[:200]})
                        raise RuntimeError(error_msg) from err

            except aiohttp.ClientError as err:
                error_msg = f"Ollama API request failed: {err}"
                logger.error(
                    error_msg,
                    extra={
                        "error": str(err),
                        "url": url,
                        "model": model,
                        "attempt": attempt + 1,
                    },
                )

                # Retry on connection errors
                if attempt < max_retries:
                    logger.warning(
                        f"Connection error, will retry",
                        extra={
                            "error": str(err),
                            "attempt": attempt + 1,
                            "max_retries": max_retries + 1,
                        },
                    )
                    continue

                raise RuntimeError(error_msg) from err

        # Should not reach here, but just in case
        raise RuntimeError(f"Ollama API failed after {max_retries + 1} attempts")

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
