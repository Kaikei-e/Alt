"""Remote Ollama HTTP driver for distributed BE dispatch.

Stateless driver that sends generation requests to a remote Ollama instance.
The base URL is passed per-call, allowing a single driver instance to target
multiple remotes.
"""

import asyncio
import json
import logging
from typing import Any, Dict, Optional

import aiohttp

from news_creator.domain.models import LLMGenerateResponse

logger = logging.getLogger(__name__)


class RemoteOllamaDriver:
    """HTTP client for remote Ollama instances."""

    def __init__(self, timeout_seconds: int = 300):
        self._timeout_seconds = timeout_seconds
        self._session: Optional[aiohttp.ClientSession] = None

    async def initialize(self) -> None:
        timeout = aiohttp.ClientTimeout(
            total=self._timeout_seconds,
            connect=30,
        )
        self._session = aiohttp.ClientSession(timeout=timeout)
        logger.info(
            "Remote Ollama driver initialized",
            extra={"timeout_seconds": self._timeout_seconds},
        )

    async def cleanup(self) -> None:
        if self._session and not self._session.closed:
            await self._session.close()
            logger.info("Remote Ollama driver cleaned up")

    async def generate(
        self, base_url: str, payload: Dict[str, Any]
    ) -> LLMGenerateResponse:
        """Send a generate request to a remote Ollama instance.

        Args:
            base_url: Remote Ollama base URL (e.g. http://remote-a:11434)
            payload: Ollama-compatible generate payload

        Returns:
            LLMGenerateResponse

        Raises:
            RuntimeError: On timeout, connection error, HTTP error, or bad JSON
        """
        if self._session is None or self._session.closed:
            await self.initialize()

        url = f"{base_url.rstrip('/')}/api/generate"
        model = payload.get("model", "unknown")

        logger.info(
            "Sending request to remote Ollama",
            extra={
                "url": url,
                "model": model,
                "dispatch_target": "remote",
                "remote_url": base_url,
            },
        )

        try:
            assert self._session is not None, (
                "Session not initialized. Call initialize() first."
            )
            async with self._session.post(url, json=payload) as response:
                text_body = await response.text()

                if response.status != 200:
                    error_msg = (
                        f"Remote Ollama API error: HTTP {response.status} "
                        f"from {base_url} - {text_body[:200]}"
                    )
                    logger.error(
                        error_msg,
                        extra={
                            "status": response.status,
                            "remote_url": base_url,
                            "model": model,
                        },
                    )
                    raise RuntimeError(error_msg)

                try:
                    data = json.loads(text_body)
                except json.JSONDecodeError as err:
                    error_msg = (
                        f"Failed to decode remote Ollama response from {base_url}: "
                        f"{err}. Body: {text_body[:200]}"
                    )
                    logger.error(error_msg, extra={"remote_url": base_url})
                    raise RuntimeError(error_msg) from err

                return LLMGenerateResponse(
                    response=data.get("response", ""),
                    model=data.get("model", model),
                    done=data.get("done"),
                    done_reason=data.get("done_reason"),
                    prompt_eval_count=data.get("prompt_eval_count"),
                    eval_count=data.get("eval_count"),
                    total_duration=data.get("total_duration"),
                    load_duration=data.get("load_duration"),
                    prompt_eval_duration=data.get("prompt_eval_duration"),
                    eval_duration=data.get("eval_duration"),
                )

        except (aiohttp.ClientError, asyncio.TimeoutError) as err:
            error_type = type(err).__name__
            is_timeout = isinstance(
                err, (aiohttp.ServerTimeoutError, asyncio.TimeoutError)
            )
            if is_timeout:
                error_msg = (
                    f"Remote Ollama at {base_url} timed out "
                    f"(limit: {self._timeout_seconds}s): {err}"
                )
            else:
                error_msg = (
                    f"Remote Ollama at {base_url} request failed: {error_type} - {err}"
                )
            logger.error(
                error_msg,
                extra={
                    "remote_url": base_url,
                    "error_type": error_type,
                    "model": model,
                },
            )
            raise RuntimeError(error_msg) from err
