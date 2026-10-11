"""Payload Builder for Ollama LLM requests (Phase 3 refactoring).

This module extracts payload construction logic from OllamaGateway.generate()
following SOLID principles (Single Responsibility Principle).

Following Python 3.14 best practices:
- Frozen dataclass for immutable payload representation
- Protocol for structural typing
"""

from __future__ import annotations

import logging
from dataclasses import dataclass
from typing import Any, Protocol

logger = logging.getLogger(__name__)


GEMMA_MODEL_TURN_HEADER: str = "<|turn>model"


def prepare_raw_prompt(prompt: str) -> str:
    """Strip surrounding whitespace, keeping the newline that ends a Gemma 4 model-turn header."""
    stripped = prompt.strip()
    # Without this newline the model must emit "\n" first; repeat_penalty suppresses it,
    # so <turn|> gets sampled as the first token and the response comes back empty.
    if stripped.endswith(GEMMA_MODEL_TURN_HEADER):
        return f"{stripped}\n"
    return stripped


@dataclass(frozen=True)
class GeneratePayload:
    """Immutable representation of Ollama generate API payload.

    This dataclass encapsulates all parameters needed for an Ollama
    generate request, providing type safety and immutability.
    """

    model: str
    prompt: str
    options: dict[str, Any]
    keep_alive: int | str
    stream: bool = False
    raw: bool = True  # Default True for Gemma 4 compatibility
    format: str | dict[str, Any] | None = None

    def to_dict(self) -> dict[str, Any]:
        """Convert to dictionary for Ollama API call.

        Returns:
            Dictionary suitable for Ollama generate API
        """
        result: dict[str, Any] = {
            "model": self.model,
            "prompt": self.prompt,
            "stream": self.stream,
            "raw": self.raw,
            "keep_alive": self.keep_alive,
            "options": self.options,
        }

        # Only include format if it's set
        if self.format is not None:
            result["format"] = self.format
            logger.debug(
                "Using structured output format", extra={"format": self.format}
            )

        return result


class PayloadBuilderProtocol(Protocol):
    """Protocol for payload building strategies."""

    def build(
        self,
        prompt: str,
        model: str,
        options: dict[str, Any],
        keep_alive: int | str,
        stream: bool = False,
        raw: bool = True,
        format: str | dict[str, Any] | None = None,
    ) -> GeneratePayload:
        """Build a generate payload."""
        ...


class PayloadBuilder:
    """Builds Ollama generate API payloads.

    Responsibilities:
    - Create GeneratePayload from parameters
    - Strip whitespace from prompts
    - Handle optional format parameter

    This class extracts lines 196-211 from OllamaGateway.generate().
    """

    def build(
        self,
        prompt: str,
        model: str,
        options: dict[str, Any],
        keep_alive: int | str,
        stream: bool = False,
        raw: bool = True,
        format: str | dict[str, Any] | None = None,
    ) -> GeneratePayload:
        """Build a generate payload for Ollama API.

        Args:
            prompt: Input prompt (will be stripped of whitespace)
            model: Model name to use
            options: LLM generation options
            keep_alive: Keep-alive duration
            stream: Whether to stream response
            raw: Whether to use raw mode (bypasses chat template)
            format: Optional output format (e.g., "json" or schema dict)

        Returns:
            Immutable GeneratePayload instance
        """
        return GeneratePayload(
            model=model,
            prompt=prepare_raw_prompt(prompt),
            options=options,
            keep_alive=keep_alive,
            stream=stream,
            raw=raw,
            format=format,
        )
