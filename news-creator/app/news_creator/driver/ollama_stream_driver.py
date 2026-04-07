"""Ollama HTTP client driver for streaming requests."""

import json
import logging
from typing import Dict, Any, Optional, AsyncIterator
import aiohttp

from news_creator.config.config import NewsCreatorConfig

logger = logging.getLogger(__name__)


class OllamaStreamDriver:
    """HTTP client for Ollama API (streaming requests only)."""

    def __init__(self, config: NewsCreatorConfig):
        """Initialize Ollama stream driver with configuration."""
        self.config = config
        self.session: Optional[aiohttp.ClientSession] = None

    async def initialize(self) -> None:
        """Initialize HTTP client session for streaming requests."""
        # For streaming, disable read timeout to allow long-running streams
        timeout = aiohttp.ClientTimeout(
            total=None,  # No total timeout
            connect=30,  # Connection timeout only
            sock_read=None,  # No read timeout for streaming
        )
        self.session = aiohttp.ClientSession(timeout=timeout)
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

    def _merge_options(self, caller_options: Optional[Dict[str, Any]]) -> Dict[str, Any]:
        """Merge config base options with caller options.

        Config base options (num_batch, num_keep, stop, etc.) are used as
        defaults. Caller options (num_predict, temperature, etc.) override.
        This prevents Ollama model reload from parameter mismatch between
        batch summarization and chat requests.
        """
        base = self.config.get_llm_options()
        if caller_options:
            base.update(caller_options)
        return base

    async def chat_stream(self, payload: Dict[str, Any]) -> AsyncIterator[Dict[str, Any]]:
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
            raise ValueError("payload must contain 'messages'")

        if self.session is None or self.session.closed:
            await self.initialize()

        model = payload.get("model", "unknown")
        chat_payload: Dict[str, Any] = {
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

        assert self.session is not None
        async with self.session.post(url, json=chat_payload) as response:
            if response.status != 200:
                text_body = await response.text()
                raise RuntimeError(f"Ollama chat API error: HTTP {response.status} - {text_body[:200]}")

            async for line_bytes in response.content:
                line = line_bytes.decode("utf-8").strip()
                if not line:
                    continue
                try:
                    chunk = json.loads(line)
                    yield chunk
                except json.JSONDecodeError:
                    logger.warning("Failed to decode chat stream line", extra={"line": line[:200]})

    async def chat_generate(self, payload: Dict[str, Any]) -> Dict[str, Any]:
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
            raise ValueError("payload must contain 'messages'")

        if self.session is None or self.session.closed:
            await self.initialize()

        model = payload.get("model", "unknown")

        chat_payload: Dict[str, Any] = {
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

        assert self.session is not None
        async with self.session.post(url, json=chat_payload) as response:
            if response.status != 200:
                text_body = await response.text()
                raise RuntimeError(
                    f"Ollama chat API error: HTTP {response.status} - {text_body[:200]}"
                )

            return await response.json()

    async def generate_stream(self, payload: Dict[str, Any]) -> AsyncIterator[Dict[str, Any]]:
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
        if not payload.get("prompt"):
            raise ValueError("payload must contain 'prompt'")

        if not payload.get("stream", False):
            raise ValueError("OllamaStreamDriver requires stream=True. Use OllamaDriver for non-streaming requests.")

        if self.session is None or self.session.closed:
            await self.initialize()

        url = f"{self.config.llm_service_url.rstrip('/')}/api/generate"
        model = payload.get("model", "unknown")
        prompt = payload.get("prompt", "")
        prompt_length = len(prompt)
        payload_size_estimate = len(json.dumps(payload))
        estimated_tokens = prompt_length // 4  # Rough estimate: 1 token ≈ 4 chars

        logger.info(
            f"Sending streaming prompt to Ollama: prompt_length={prompt_length} chars, "
            f"estimated_tokens={estimated_tokens}, model={model}, payload_size={payload_size_estimate} bytes"
        )

        assert self.session is not None, "Session not initialized. Call initialize() first."
        async with self.session.post(url, json=payload) as response:

            if response.status != 200:
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
                raise RuntimeError(f"Ollama API error: HTTP {response.status} - {text_body[:200]}")

            logger.info(
                "Starting to read streaming response from Ollama",
                extra={
                    "url": url,
                    "model": model,
                }
            )

            lines_read = 0
            chunks_yielded = 0
            has_data = False
            connection_closed_gracefully = False

            try:
                # Read line by line (Ollama returns NDJSON - newline-delimited JSON)
                while True:
                    try:
                        line_bytes = await response.content.readline()
                        if not line_bytes:
                            # EOF reached
                            connection_closed_gracefully = True
                            break

                        lines_read += 1
                        # Decode bytes to string and strip whitespace
                        line = line_bytes.decode('utf-8').strip()

                        if not line:
                            # Empty line, skip
                            continue

                        has_data = True
                        try:
                            parsed = json.loads(line)
                            chunks_yielded += 1
                            if chunks_yielded <= 3 or chunks_yielded % 50 == 0:
                                logger.info(
                                    "Yielding chunk from Ollama stream",
                                    extra={
                                        "chunk_number": chunks_yielded,
                                        "lines_read": lines_read,
                                        "url": url,
                                        "model": model,
                                    }
                                )
                            yield parsed
                        except json.JSONDecodeError as e:
                            logger.error(
                                "Failed to decode stream line",
                                extra={
                                    "line_preview": line[:200] if line else None,
                                    "lines_read": lines_read,
                                    "url": url,
                                    "model": model,
                                },
                                exc_info=True
                            )

                    except aiohttp.ClientConnectionError as conn_err:
                        # Connection was closed - this can happen during streaming
                        # If we have data, log warning but don't raise - let the stream end naturally
                        if has_data:
                            logger.warning(
                                "Connection closed during streaming, but data was received",
                                extra={
                                    "error": str(conn_err),
                                    "error_type": type(conn_err).__name__,
                                    "lines_read": lines_read,
                                    "chunks_yielded": chunks_yielded,
                                    "url": url,
                                    "model": model,
                                    "has_data": has_data,
                                }
                            )
                            # Break the loop to end the stream gracefully
                            connection_closed_gracefully = True
                            break
                        else:
                            # No data received, this is a real error
                            logger.error(
                                "Connection closed before any data was received",
                                extra={
                                    "error": str(conn_err),
                                    "error_type": type(conn_err).__name__,
                                    "url": url,
                                    "model": model,
                                },
                                exc_info=True
                            )
                            raise

                if not has_data:
                    logger.warning(
                        "Stream completed but no data was received",
                        extra={
                            "lines_read": lines_read,
                            "url": url,
                            "model": model,
                            "connection_closed_gracefully": connection_closed_gracefully,
                        }
                    )
                else:
                    logger.info(
                        "Stream completed successfully",
                        extra={
                            "chunks_yielded": chunks_yielded,
                            "lines_read": lines_read,
                            "url": url,
                            "model": model,
                            "connection_closed_gracefully": connection_closed_gracefully,
                        }
                    )

            except aiohttp.ClientConnectionError as conn_err:
                # Re-raise connection errors if no data was received
                if not has_data:
                    logger.error(
                        "Connection error during streaming with no data received",
                        extra={
                            "error": str(conn_err),
                            "error_type": type(conn_err).__name__,
                            "lines_read": lines_read,
                            "chunks_yielded": chunks_yielded,
                            "url": url,
                            "model": model,
                        },
                        exc_info=True
                    )
                    raise
                # If we have data, log and let the stream end naturally
                logger.warning(
                    "Connection error during streaming, but partial data was received",
                    extra={
                        "error": str(conn_err),
                        "error_type": type(conn_err).__name__,
                        "lines_read": lines_read,
                        "chunks_yielded": chunks_yielded,
                        "url": url,
                        "model": model,
                    }
                )

            except Exception as stream_err:
                logger.error(
                    "Error reading from Ollama stream",
                    extra={
                        "error": str(stream_err),
                        "error_type": type(stream_err).__name__,
                        "lines_read": lines_read,
                        "chunks_yielded": chunks_yielded,
                        "url": url,
                        "model": model,
                    },
                    exc_info=True
                )
                raise

