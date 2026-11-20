"""Ollama HTTP client driver."""

import asyncio
import json
import logging
from dataclasses import dataclass
from http import HTTPStatus
from typing import Any

import aiohttp

from news_creator.config.config import NewsCreatorConfig

logger = logging.getLogger(__name__)

# Rough overhead in bytes for the non-prompt payload fields (model, options,
# keep_alive, stream, raw) when estimating payload size for logging. Not
# exact -- the field is diagnostic only, so a cheap approximation beats
# re-serializing the whole payload just to log its size.
_PAYLOAD_OVERHEAD_ESTIMATE_BYTES = 256
_PROMPT_PREVIEW_EDGE_CHARS = 200


@dataclass(frozen=True)
class _GenerateAttemptMeta:
    url: str
    model: str
    prompt_length: int
    payload_size_estimate: int
    max_retries: int
    base_delay: float


class OllamaDriver:
    """HTTP client for Ollama API (non-streaming requests only)."""

    def __init__(self, config: NewsCreatorConfig) -> None:
        """Initialize Ollama driver with configuration."""
        self.config = config
        self.session: aiohttp.ClientSession | None = None

    async def initialize(self) -> None:
        """Initialize HTTP client session for non-streaming requests."""
        timeout = aiohttp.ClientTimeout(
            total=None,
            connect=60,  # Connection timeout
            sock_read=self.config.llm_timeout_seconds,  # Bound per-attempt read stalls
        )
        headers = {}
        token = self.config.llm.inference_service_token
        if token:
            headers["Authorization"] = f"Bearer {token}"

        self.session = aiohttp.ClientSession(timeout=timeout, headers=headers)
        logger.info(
            "Ollama driver initialized (non-streaming)",
            extra={
                "url": self.config.llm_service_url,
                "timeout_seconds": self.config.llm_timeout_seconds,
            },
        )

    async def cleanup(self) -> None:
        """Cleanup HTTP client session."""
        if self.session and not self.session.closed:
            await self.session.close()
            logger.info("Ollama driver cleaned up")

    @staticmethod
    def _log_prompt_diagnostics(
        prompt: str,
        payload_size_estimate: int,
        model: str,
    ) -> None:
        prompt_length = len(prompt)
        estimated_tokens = prompt_length // 4  # Rough estimate: 1 token ≈ 4 chars
        prompt_preview_start = (
            prompt[:_PROMPT_PREVIEW_EDGE_CHARS]
            if prompt_length > _PROMPT_PREVIEW_EDGE_CHARS
            else prompt
        )
        prompt_preview_end = (
            prompt[-_PROMPT_PREVIEW_EDGE_CHARS:]
            if prompt_length > _PROMPT_PREVIEW_EDGE_CHARS
            else ""
        )

        abnormal_prompt_threshold = 100_000  # characters
        if prompt_length > abnormal_prompt_threshold:
            logger.error(
                "ABNORMAL PROMPT SIZE DETECTED - Investigating cause",
                extra={
                    "prompt_length": prompt_length,
                    "estimated_tokens": estimated_tokens,
                    "prompt_preview_start": prompt_preview_start,
                    "prompt_preview_end": prompt_preview_end,
                    "prompt_middle_sample": prompt[
                        prompt_length // 2 - 100 : prompt_length // 2 + 100
                    ]
                    if prompt_length > _PROMPT_PREVIEW_EDGE_CHARS
                    else "",
                    "payload_size_bytes": payload_size_estimate,
                    "model": model,
                },
            )
        else:
            logger.info(
                "Sending prompt to Ollama: prompt_length=%s chars, estimated_tokens=%s, "
                "model=%s, payload_size=%s bytes",
                prompt_length,
                estimated_tokens,
                model,
                payload_size_estimate,
            )

    @staticmethod
    async def _sleep_before_attempt(
        attempt: int,
        max_retries: int,
        base_delay: float,
        url: str,
        model: str,
    ) -> None:
        if attempt > 0:
            delay = base_delay * (2 ** (attempt - 1))
            jitter = delay * 0.1  # 10% jitter
            wait_time = delay + jitter
            logger.info(
                "Retrying Ollama API call (attempt %s/%s)",
                attempt + 1,
                max_retries + 1,
                extra={
                    "url": url,
                    "model": model,
                    "wait_time_seconds": wait_time,
                },
            )
            await asyncio.sleep(wait_time)
        else:
            logger.debug(
                "Calling Ollama API (non-streaming)",
                extra={"url": url, "model": model},
            )

    @staticmethod
    def _parse_generate_json(text_body: str) -> dict[str, Any]:
        try:
            return json.loads(text_body)
        except json.JSONDecodeError as err:
            error_msg = (
                f"Failed to decode Ollama response: {err!s}. Body: {text_body[:200]}"
            )
            logger.exception(
                error_msg,
                extra={"error": str(err), "body_preview": text_body[:200]},
            )
            raise RuntimeError(error_msg) from err

    @staticmethod
    async def _handle_non_ok_response(
        response: aiohttp.ClientResponse,
        url: str,
        model: str,
        attempt: int,
        max_retries: int,
    ) -> bool:
        text_body = await response.text()
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
        if (
            response.status >= HTTPStatus.INTERNAL_SERVER_ERROR
            or response.status
            in (HTTPStatus.BAD_GATEWAY, HTTPStatus.SERVICE_UNAVAILABLE)
        ) and attempt < max_retries:
            logger.warning(
                "Retryable error %s, will retry",
                response.status,
            )
            return True
        msg = f"Ollama API error: HTTP {response.status} - {text_body[:200]}"
        raise RuntimeError(msg)

    def _handle_network_error(
        self,
        err: TimeoutError | aiohttp.ClientError,
        attempt: int,
        meta: _GenerateAttemptMeta,
    ) -> None:
        error_type = type(err).__name__
        is_timeout = isinstance(err, (aiohttp.ServerTimeoutError, asyncio.TimeoutError))
        error_msg = f"Ollama API request failed: {err}"

        logger.exception(
            error_msg,
            extra={
                "error": str(err),
                "error_type": error_type,
                "is_timeout": is_timeout,
                "url": meta.url,
                "model": meta.model,
                "prompt_length": meta.prompt_length,
                "payload_size_estimate_bytes": meta.payload_size_estimate,
                "attempt": attempt + 1,
                "max_retries": meta.max_retries + 1,
                "timeout_seconds": self.config.llm_timeout_seconds,
                "total_timeout_seconds": self.config.llm_timeout_seconds
                * (meta.max_retries + 1),
            },
        )

        if attempt < meta.max_retries:
            delay = meta.base_delay * (2**attempt)
            jitter = delay * 0.1
            wait_time = delay + jitter

            logger.warning(
                "Connection error, will retry",
                extra={
                    "error": str(err),
                    "error_type": error_type,
                    "is_timeout": is_timeout,
                    "url": meta.url,
                    "model": meta.model,
                    "prompt_length": meta.prompt_length,
                    "payload_size_estimate_bytes": meta.payload_size_estimate,
                    "attempt": attempt + 1,
                    "max_retries": meta.max_retries + 1,
                    "wait_time_seconds": wait_time,
                    "timeout_seconds": self.config.llm_timeout_seconds,
                    "next_attempt_timeout_seconds": self.config.llm_timeout_seconds,
                },
            )
            return

        final_error_msg = (
            f"Ollama API failed after {meta.max_retries + 1} attempts. "
            f"Last error: {error_type} - {err!s}. "
            f"Timeout setting: {self.config.llm_timeout_seconds}s per attempt, "
            f"Total time spent: ~{self.config.llm_timeout_seconds * (meta.max_retries + 1)}s. "
            f"Model: {meta.model}, Prompt length: {meta.prompt_length} chars, "
            f"Payload size: ~{meta.payload_size_estimate} bytes"
        )
        logger.exception(
            "Ollama API failed after all retries",
            extra={
                "error": str(err),
                "error_type": error_type,
                "is_timeout": is_timeout,
                "url": meta.url,
                "model": meta.model,
                "prompt_length": meta.prompt_length,
                "payload_size_estimate_bytes": meta.payload_size_estimate,
                "total_attempts": meta.max_retries + 1,
                "timeout_seconds": self.config.llm_timeout_seconds,
                "total_timeout_seconds": self.config.llm_timeout_seconds
                * (meta.max_retries + 1),
            },
        )
        raise RuntimeError(final_error_msg) from err

    async def generate(self, payload: dict[str, Any]) -> dict[str, Any]:
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
            msg = "payload must contain 'prompt'"
            raise ValueError(msg)

        if self.session is None or self.session.closed:
            await self.initialize()

        url = f"{self.config.llm_service_url.rstrip('/')}/api/generate"
        model = payload.get("model", "unknown")
        prompt = payload.get("prompt", "")
        prompt_length = len(prompt)
        payload_size_estimate = prompt_length + _PAYLOAD_OVERHEAD_ESTIMATE_BYTES

        self._log_prompt_diagnostics(prompt, payload_size_estimate, model)

        meta = _GenerateAttemptMeta(
            url=url,
            model=model,
            prompt_length=prompt_length,
            payload_size_estimate=payload_size_estimate,
            max_retries=3,
            base_delay=1.0,
        )

        for attempt in range(meta.max_retries + 1):
            try:
                await self._sleep_before_attempt(
                    attempt, meta.max_retries, meta.base_delay, url, model
                )

                if payload.get("stream", False):
                    msg = "OllamaDriver does not support streaming. Use OllamaStreamDriver instead."
                    raise ValueError(msg)

                if self.session is None:
                    msg = "Session not initialized. Call initialize() first."
                    raise AssertionError(msg)
                async with self.session.post(
                    url, json=payload, allow_redirects=False
                ) as response:
                    if response.status != HTTPStatus.OK:
                        await self._handle_non_ok_response(
                            response, url, model, attempt, meta.max_retries
                        )
                        continue

                    text_body = await response.text()
                    return self._parse_generate_json(text_body)

            except (TimeoutError, aiohttp.ClientError) as err:
                self._handle_network_error(err, attempt, meta)

        msg = (
            f"Ollama API failed after {meta.max_retries + 1} attempts. "
            f"Timeout setting: {self.config.llm_timeout_seconds}s"
        )
        raise RuntimeError(msg)

    async def list_tags(self) -> dict[str, Any]:
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
            if not (self.session is not None):
                msg = "Session not initialized. Call initialize() first."
                raise AssertionError(msg)
            async with self.session.get(url, allow_redirects=False) as response:
                text_body = await response.text()

                if response.status != HTTPStatus.OK:
                    msg = f"Ollama tags API error: HTTP {response.status}"
                    logger.error(
                        "Ollama tags API returned error",
                        extra={
                            "status": response.status,
                            "body": text_body[:500],
                        },
                    )
                    raise RuntimeError(msg)

                try:
                    return json.loads(text_body)
                except json.JSONDecodeError as err:
                    logger.exception(
                        "Failed to decode Ollama tags response",
                        extra={"error": str(err)},
                    )
                    msg = "Failed to decode Ollama tags response"
                    raise RuntimeError(msg) from err

        except aiohttp.ClientError as err:
            logger.exception(
                "Ollama tags API request failed", extra={"error": str(err)}
            )
            msg = f"Ollama tags API request failed: {err}"
            raise RuntimeError(msg) from err
