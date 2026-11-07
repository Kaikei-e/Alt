"""OpenAI-compatible `/v1/chat/completions` driver for distributed BE dispatch."""

from __future__ import annotations

import asyncio
import json
import logging
from typing import Any

import aiohttp

from news_creator.config.distributed_be_config import RemoteTarget
from news_creator.domain.models import LLMGenerateResponse

logger = logging.getLogger(__name__)

MAX_CHAT_RESPONSE_BYTES = 4 * 1024 * 1024


class RemoteRequestRejectedError(RuntimeError):
    """Raised when a remote target rejects a request (non-408/429 4xx or empty content)."""


async def _read_bounded_text(response: Any, max_bytes: int, base_url: str) -> str:
    content_length = getattr(response, "content_length", None)
    if isinstance(content_length, int) and content_length > max_bytes:
        raise RuntimeError(
            f"Remote chat response from {base_url} exceeds {max_bytes} bytes "
            f"(Content-Length: {content_length})"
        )

    headers = getattr(response, "headers", None)
    if isinstance(headers, dict) or hasattr(headers, "get"):
        raw_cl = headers.get("Content-Length") if headers is not None else None
        if isinstance(raw_cl, str):
            try:
                parsed_cl = int(raw_cl)
            except ValueError:
                parsed_cl = None
            if parsed_cl is not None and parsed_cl > max_bytes:
                raise RuntimeError(
                    f"Remote chat response from {base_url} exceeds {max_bytes} bytes "
                    f"(Content-Length: {parsed_cl})"
                )

    content = getattr(response, "content", None)
    if isinstance(content, aiohttp.StreamReader):
        chunks: list[bytes] = []
        total = 0
        while total <= max_bytes:
            chunk = await content.read(max_bytes + 1 - total)
            if not chunk:
                break
            chunks.append(chunk)
            total += len(chunk)
        raw_bytes = b"".join(chunks)
    elif content is not None and callable(getattr(content, "read", None)):
        maybe_bytes = await content.read(max_bytes + 1)
        if isinstance(maybe_bytes, (bytes, bytearray)):
            raw_bytes = bytes(maybe_bytes)
        else:
            text_val = await response.text(errors="replace")
            raw_bytes = text_val.encode("utf-8")
    else:
        try:
            text_val = await response.text(errors="replace")
        except TypeError:
            text_val = await response.text()
        except UnicodeDecodeError as err:
            raise RuntimeError(
                f"Failed to decode remote chat response from {base_url}: {err}"
            ) from err
        if len(text_val.encode("utf-8")) > max_bytes:
            raise RuntimeError(
                f"Remote chat response from {base_url} exceeds {max_bytes} bytes"
            )
        return text_val

    if len(raw_bytes) > max_bytes:
        raise RuntimeError(
            f"Remote chat response from {base_url} exceeds {max_bytes} bytes"
        )
    return raw_bytes.decode("utf-8", errors="replace")


class RemoteChatDriver:
    """HTTP client for OpenAI-compatible `/v1/chat/completions` targets."""

    def __init__(
        self,
        timeout_seconds: int = 300,
        reasoning_effort: str | None = None,
    ):
        self._timeout_seconds = timeout_seconds
        self._reasoning_effort = reasoning_effort
        self._session: aiohttp.ClientSession | None = None

    async def initialize(self) -> None:
        timeout = aiohttp.ClientTimeout(
            total=self._timeout_seconds,
            connect=30,
        )
        self._session = aiohttp.ClientSession(timeout=timeout)
        logger.info(
            "Remote chat driver initialized",
            extra={
                "timeout_seconds": self._timeout_seconds,
                "reasoning_effort": self._reasoning_effort,
            },
        )

    async def cleanup(self) -> None:
        if self._session and not self._session.closed:
            await self._session.close()
            logger.info("Remote chat driver cleaned up")

    async def chat(
        self,
        target: RemoteTarget,
        messages: list[dict[str, str]],
        *,
        max_tokens: int,
        temperature: float,
        top_p: float,
    ) -> LLMGenerateResponse:
        if self._session is None or self._session.closed:
            raise RuntimeError("RemoteChatDriver session not initialized")

        url = f"{target.base_url.rstrip('/')}/v1/chat/completions"
        payload: dict[str, Any] = {
            "model": target.model,
            "messages": messages,
            "stream": False,
            "max_tokens": max_tokens,
            "temperature": temperature,
            "top_p": top_p,
        }
        if self._reasoning_effort is not None:
            payload["reasoning_effort"] = self._reasoning_effort

        try:
            async with self._session.post(
                url,
                json=payload,
                allow_redirects=False,
            ) as response:
                text_body = await _read_bounded_text(
                    response, MAX_CHAT_RESPONSE_BYTES, target.base_url
                )

                if response.status != 200:
                    error_msg = (
                        f"Remote chat API error: HTTP {response.status} "
                        f"from {target.base_url} - {text_body[:200]}"
                    )
                    logger.error(
                        error_msg,
                        extra={
                            "status": response.status,
                            "remote_url": target.base_url,
                            "model": target.model,
                        },
                    )
                    if 400 <= response.status < 500 and response.status not in (
                        408,
                        429,
                    ):
                        raise RemoteRequestRejectedError(error_msg)
                    raise RuntimeError(error_msg)

                try:
                    data = json.loads(text_body)
                except json.JSONDecodeError as err:
                    error_msg = (
                        f"Failed to decode remote chat response from {target.base_url}: "
                        f"{err}. Body: {text_body[:200]}"
                    )
                    logger.error(error_msg, extra={"remote_url": target.base_url})
                    raise RuntimeError(error_msg) from err

                if not isinstance(data, dict):
                    raise RuntimeError(
                        f"Invalid remote chat response structure from {target.base_url}"
                    )

                choices = data.get("choices")
                if not isinstance(choices, list) or not choices:
                    raise RuntimeError(
                        f"Missing 'choices' in remote chat response from {target.base_url}"
                    )

                first_choice = choices[0]
                if not isinstance(first_choice, dict):
                    raise RuntimeError(
                        f"Invalid choice item in remote chat response from {target.base_url}"
                    )

                message = first_choice.get("message")
                if not isinstance(message, dict):
                    raise RuntimeError(
                        f"Missing 'message' in remote chat response from {target.base_url}"
                    )

                raw_content = message.get("content")
                if not isinstance(raw_content, str):
                    raise RuntimeError(
                        f"Missing 'content' in remote chat response from {target.base_url}"
                    )

                content = raw_content.strip()
                if not content:
                    raise RemoteRequestRejectedError(
                        f"Empty 'content' in remote chat response from {target.base_url}"
                    )

                raw_model = data.get("model")
                served_model = (
                    raw_model.strip()
                    if isinstance(raw_model, str) and raw_model.strip()
                    else target.model
                )
                finish_reason = first_choice.get("finish_reason")
                usage = data.get("usage") if isinstance(data.get("usage"), dict) else {}

                return LLMGenerateResponse(
                    response=content,
                    model=served_model,
                    done=True,
                    done_reason=finish_reason,
                    prompt_eval_count=usage.get("prompt_tokens"),
                    eval_count=usage.get("completion_tokens"),
                )

        except (aiohttp.ClientError, asyncio.TimeoutError) as err:
            error_type = type(err).__name__
            is_timeout = isinstance(
                err, (aiohttp.ServerTimeoutError, asyncio.TimeoutError)
            )
            if is_timeout:
                error_msg = (
                    f"Remote chat target at {target.base_url} timed out "
                    f"(limit: {self._timeout_seconds}s): {err}"
                )
            else:
                error_msg = (
                    f"Remote chat target at {target.base_url} request failed: "
                    f"{error_type} - {err}"
                )
            logger.error(
                error_msg,
                extra={
                    "remote_url": target.base_url,
                    "error_type": error_type,
                    "model": target.model,
                },
            )
            raise RuntimeError(error_msg) from err
