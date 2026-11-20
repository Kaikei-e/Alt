"""Ollama HTTP client driver for streaming requests."""

import json
import logging
from collections.abc import AsyncGenerator, AsyncIterator
from dataclasses import dataclass
from http import HTTPStatus
from typing import Any

import aiohttp

from news_creator.config.config import NewsCreatorConfig

logger = logging.getLogger(__name__)

# Runner startup params: changing these mid-session forces Ollama to reload
# the model runner. They are fixed by config/Modelfile, never caller-supplied,
# mirroring the /api/generate path's defense (ollama_gateway.py).
_RUNNER_STARTUP_PARAMS = frozenset({"num_ctx", "num_batch", "num_keep", "num_gpu"})
_INITIAL_CHUNK_LOG_LIMIT = 3


@dataclass(frozen=True)
class _StreamSummary:
    has_data: bool
    received_done: bool
    chunks_yielded: int
    lines_read: int
    url: str
    model: str
    connection_closed_gracefully: bool


def _ensure_stream_completed(summary: _StreamSummary) -> None:
    if not summary.has_data:
        logger.warning(
            "Stream completed but no data was received",
            extra={
                "lines_read": summary.lines_read,
                "url": summary.url,
                "model": summary.model,
                "connection_closed_gracefully": summary.connection_closed_gracefully,
            },
        )
        return
    if not summary.received_done:
        # The stream ended (EOF or a mid-stream connection drop
        # that was previously treated as a graceful close) without
        # ever yielding a done=true chunk. Without this check the
        # partial response looked identical to a complete one to
        # every downstream consumer.
        logger.error(
            "Stream ended without a done=true chunk; response is truncated",
            extra={
                "chunks_yielded": summary.chunks_yielded,
                "lines_read": summary.lines_read,
                "url": summary.url,
                "model": summary.model,
                "connection_closed_gracefully": summary.connection_closed_gracefully,
            },
        )
        msg = (
            f"Ollama stream for model={summary.model} ended before a done=true chunk was "
            f"received (chunks_yielded={summary.chunks_yielded}); response is truncated"
        )
        raise RuntimeError(msg)
    logger.info(
        "Stream completed successfully",
        extra={
            "chunks_yielded": summary.chunks_yielded,
            "lines_read": summary.lines_read,
            "url": summary.url,
            "model": summary.model,
            "connection_closed_gracefully": summary.connection_closed_gracefully,
        },
    )


@dataclass
class _StreamProgress:
    url: str
    model: str
    lines_read: int = 0
    chunks_yielded: int = 0
    has_data: bool = False
    received_done: bool = False
    connection_closed_gracefully: bool = False


def _validate_stream_payload(payload: dict[str, Any]) -> tuple[str, str]:
    if not payload.get("prompt"):
        msg = "payload must contain 'prompt'"
        raise ValueError(msg)

    if not payload.get("stream", False):
        msg = "OllamaStreamDriver requires stream=True. Use OllamaDriver for non-streaming requests."
        raise ValueError(msg)

    model = str(payload.get("model", "unknown"))
    prompt = str(payload.get("prompt", ""))
    prompt_length = len(prompt)
    payload_size_estimate = len(json.dumps(payload))
    estimated_tokens = prompt_length // 4  # Rough estimate: 1 token ≈ 4 chars

    logger.info(
        "Sending streaming prompt to Ollama: prompt_length=%s chars, "
        "estimated_tokens=%s, model=%s, payload_size=%s bytes",
        prompt_length,
        estimated_tokens,
        model,
        payload_size_estimate,
    )
    return model, prompt


async def _raise_for_stream_error(
    response: aiohttp.ClientResponse, url: str, model: str
) -> None:
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
        },
    )
    msg = f"Ollama API error: HTTP {response.status} - {text_body[:200]}"
    raise RuntimeError(msg)


def _decode_stream_line(line: str, progress: _StreamProgress) -> dict[str, Any] | None:
    try:
        parsed = json.loads(line)
    except json.JSONDecodeError:
        logger.exception(
            "Failed to decode stream line",
            extra={
                "line_preview": line[:200] if line else None,
                "lines_read": progress.lines_read,
                "url": progress.url,
                "model": progress.model,
            },
        )
        return None
    progress.chunks_yielded += 1
    if parsed.get("done"):
        progress.received_done = True
    if (
        progress.chunks_yielded <= _INITIAL_CHUNK_LOG_LIMIT
        or progress.chunks_yielded % 50 == 0
    ):
        logger.info(
            "Yielding chunk from Ollama stream",
            extra={
                "chunk_number": progress.chunks_yielded,
                "lines_read": progress.lines_read,
                "url": progress.url,
                "model": progress.model,
            },
        )
    return parsed


def _handle_inner_connection_error(
    conn_err: aiohttp.ClientConnectionError, progress: _StreamProgress
) -> None:
    if progress.has_data:
        logger.warning(
            "Connection closed during streaming, but data was received",
            extra={
                "error": str(conn_err),
                "error_type": type(conn_err).__name__,
                "lines_read": progress.lines_read,
                "chunks_yielded": progress.chunks_yielded,
                "url": progress.url,
                "model": progress.model,
                "has_data": progress.has_data,
            },
        )
        progress.connection_closed_gracefully = True
        return
    logger.exception(
        "Connection closed before any data was received",
        extra={
            "error": str(conn_err),
            "error_type": type(conn_err).__name__,
            "url": progress.url,
            "model": progress.model,
        },
    )
    raise conn_err


def _handle_outer_connection_error(
    conn_err: aiohttp.ClientConnectionError, progress: _StreamProgress
) -> None:
    if not progress.has_data:
        logger.exception(
            "Connection error during streaming with no data received",
            extra={
                "error": str(conn_err),
                "error_type": type(conn_err).__name__,
                "lines_read": progress.lines_read,
                "chunks_yielded": progress.chunks_yielded,
                "url": progress.url,
                "model": progress.model,
            },
        )
        raise conn_err
    logger.warning(
        "Connection error during streaming, but partial data was received",
        extra={
            "error": str(conn_err),
            "error_type": type(conn_err).__name__,
            "lines_read": progress.lines_read,
            "chunks_yielded": progress.chunks_yielded,
            "url": progress.url,
            "model": progress.model,
        },
    )


class OllamaStreamDriver:
    """HTTP client for Ollama API (streaming requests only)."""

    def __init__(self, config: NewsCreatorConfig) -> None:
        """Initialize Ollama stream driver with configuration."""
        self.config = config
        self.session: aiohttp.ClientSession | None = None

    async def initialize(self) -> None:
        """Initialize HTTP client session for streaming requests."""
        # For streaming, disable read timeout to allow long-running streams
        timeout = aiohttp.ClientTimeout(
            total=None,  # No total timeout
            connect=30,  # Connection timeout only
            sock_read=None,  # No read timeout for streaming
        )
        headers = {}
        token = self.config.llm.inference_service_token
        if token:
            headers["Authorization"] = f"Bearer {token}"

        self.session = aiohttp.ClientSession(timeout=timeout, headers=headers)
        logger.info(
            "Ollama stream driver initialized",
            extra={
                "url": self.config.llm_service_url,
            },
        )

    async def cleanup(self) -> None:
        """Cleanup HTTP client session."""
        if self.session and not self.session.closed:
            await self.session.close()
            logger.info("Ollama stream driver cleaned up")

    def _merge_options(self, caller_options: dict[str, Any] | None) -> dict[str, Any]:
        """Merge config base options with caller options.

        Config base options (num_batch, num_keep, stop, etc.) are used as
        defaults. Caller options (num_predict, temperature, etc.) override.
        This prevents Ollama model reload from parameter mismatch between
        batch summarization and chat requests.

        Runner startup params (num_ctx, num_batch, num_keep) are stripped
        from caller options before merging -- they are fixed by config/
        Modelfile, so a caller-supplied value that differs from the base
        would otherwise trigger an Ollama runner reload. Sampling params
        stay caller-priority.
        """
        base = self.config.get_llm_options()
        if caller_options:
            sampling_options = {
                k: v
                for k, v in caller_options.items()
                if k not in _RUNNER_STARTUP_PARAMS
            }
            base.update(sampling_options)
        return base

    async def chat_stream(
        self, payload: dict[str, Any]
    ) -> AsyncGenerator[dict[str, Any]]:
        """Proxy chat requests through Ollama /api/chat with think=false.

        Forwards messages directly to Ollama's /api/chat endpoint, letting
        Ollama handle the chat template natively.  think=false disables
        Gemma4's thinking mode without bypassing the template engine.

        Options are merged with config base options (num_batch, num_keep, stop)
        to prevent Ollama model reload from parameter mismatch.

        Args:
            payload: Request payload in /api/chat format (must contain 'messages')

        Yields:
            Response chunks in /api/chat format (with message.role and message.content)
        """
        if not payload.get("messages"):
            msg = "payload must contain 'messages'"
            raise ValueError(msg)

        if self.session is None or self.session.closed:
            await self.initialize()

        model = payload.get("model", "unknown")
        chat_payload: dict[str, Any] = {
            "model": model,
            "messages": payload["messages"],
            "stream": True,
            "think": False,
            "options": self._merge_options(payload.get("options")),
        }
        if payload.get("keep_alive") is not None:
            chat_payload["keep_alive"] = payload["keep_alive"]
        if payload.get("format") is not None:
            chat_payload["format"] = payload["format"]

        url = f"{self.config.llm_service_url.rstrip('/')}/api/chat"

        logger.info(
            "Sending chat stream to Ollama",
            extra={
                "model": model,
                "message_count": len(payload["messages"]),
                "url": url,
            },
        )

        if self.session is None:
            msg = "HTTP session is not initialized"
            raise RuntimeError(msg)
        async with self.session.post(
            url, json=chat_payload, allow_redirects=False
        ) as response:
            if response.status != HTTPStatus.OK:
                text_body = await response.text()
                msg = (
                    f"Ollama chat API error: HTTP {response.status} - {text_body[:200]}"
                )
                raise RuntimeError(msg)

            async for line_bytes in response.content:
                line = line_bytes.decode("utf-8").strip()
                if not line:
                    continue
                try:
                    chunk = json.loads(line)
                    yield chunk
                except json.JSONDecodeError:
                    logger.warning(
                        "Failed to decode chat stream line", extra={"line": line[:200]}
                    )

    async def chat_generate(self, payload: dict[str, Any]) -> dict[str, Any]:
        """Non-streaming chat via Ollama /api/chat with think=false.

        Forwards messages directly to Ollama's /api/chat endpoint.
        Options are merged with config base options to prevent Ollama model reload.

        Args:
            payload: Request payload in /api/chat format (must contain 'messages')

        Returns:
            Response dict in /api/chat format

        Raises:
            ValueError: If messages are missing
            RuntimeError: If Ollama returns an error
        """
        if not payload.get("messages"):
            msg = "payload must contain 'messages'"
            raise ValueError(msg)

        if self.session is None or self.session.closed:
            await self.initialize()

        model = payload.get("model", "unknown")

        chat_payload: dict[str, Any] = {
            "model": model,
            "messages": payload["messages"],
            "stream": False,
            "options": self._merge_options(payload.get("options")),
        }
        # Only set think parameter if explicitly provided in payload.
        # Omitting think lets Gemma 4 default to thinking mode (better quality).
        # Setting think=false breaks format constraint for gemma4 (Ollama #15260).
        if "think" in payload:
            chat_payload["think"] = payload["think"]
        if payload.get("keep_alive") is not None:
            chat_payload["keep_alive"] = payload["keep_alive"]
        if payload.get("format") is not None:
            chat_payload["format"] = payload["format"]

        url = f"{self.config.llm_service_url.rstrip('/')}/api/chat"

        logger.info(
            "Sending chat (non-streaming) to Ollama",
            extra={
                "model": model,
                "message_count": len(payload["messages"]),
                "url": url,
            },
        )

        if self.session is None:
            msg = "HTTP session is not initialized"
            raise RuntimeError(msg)
        async with self.session.post(
            url, json=chat_payload, allow_redirects=False
        ) as response:
            if response.status != HTTPStatus.OK:
                text_body = await response.text()
                msg = (
                    f"Ollama chat API error: HTTP {response.status} - {text_body[:200]}"
                )
                raise RuntimeError(msg)

            return await response.json()

    async def generate_stream(
        self, payload: dict[str, Any]
    ) -> AsyncIterator[dict[str, Any]]:
        """
        Call Ollama generate API with streaming support.

        Args:
            payload: Request payload for /api/generate endpoint (must have stream=True)

        Yields:
            Response chunks (dictionaries) from Ollama stream

        Raises:
            ValueError: If payload is invalid or stream is not True
            RuntimeError: If Ollama service returns error
        """
        model, _ = _validate_stream_payload(payload)

        if self.session is None or self.session.closed:
            await self.initialize()

        url = f"{self.config.llm_service_url.rstrip('/')}/api/generate"
        if self.session is None:
            msg = "Session not initialized. Call initialize() first."
            raise AssertionError(msg)

        async with self.session.post(
            url, json=payload, allow_redirects=False
        ) as response:
            if response.status != HTTPStatus.OK:
                await _raise_for_stream_error(response, url, model)

            logger.info(
                "Starting to read streaming response from Ollama",
                extra={"url": url, "model": model},
            )
            progress = _StreamProgress(url=url, model=model)

            try:
                while True:
                    try:
                        line_bytes = await response.content.readline()
                        if not line_bytes:
                            progress.connection_closed_gracefully = True
                            break

                        progress.lines_read += 1
                        line = line_bytes.decode("utf-8").strip()
                        if not line:
                            continue

                        progress.has_data = True
                        parsed = _decode_stream_line(line, progress)
                        if parsed is not None:
                            yield parsed
                    except aiohttp.ClientConnectionError as conn_err:
                        _handle_inner_connection_error(conn_err, progress)
                        break

                _ensure_stream_completed(
                    _StreamSummary(
                        has_data=progress.has_data,
                        received_done=progress.received_done,
                        chunks_yielded=progress.chunks_yielded,
                        lines_read=progress.lines_read,
                        url=url,
                        model=model,
                        connection_closed_gracefully=progress.connection_closed_gracefully,
                    )
                )

            except aiohttp.ClientConnectionError as conn_err:
                _handle_outer_connection_error(conn_err, progress)

            except Exception as stream_err:
                logger.exception(
                    "Error reading from Ollama stream",
                    extra={
                        "error": str(stream_err),
                        "error_type": type(stream_err).__name__,
                        "lines_read": progress.lines_read,
                        "chunks_yielded": progress.chunks_yielded,
                        "url": url,
                        "model": model,
                    },
                )
                raise
