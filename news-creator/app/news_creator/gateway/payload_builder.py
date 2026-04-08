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
from typing import Any, Protocol, Union

logger = logging.getLogger(__name__)


@dataclass(frozen=True)
class GeneratePayload:
    """Immutable representation of Ollama generate API payload.

    This dataclass encapsulates all parameters needed for an Ollama
    generate request, providing type safety and immutability.
    """

    model: str
    prompt: str
    options: dict[str, Any]
    keep_alive: Union[int, str]
    stream: bool = False
    raw: bool = True  # Default True for Gemma 4 compatibility
    format: Union[str, dict[str, Any], None] = None

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
        keep_alive: Union[int, str],
        stream: bool = False,
        raw: bool = True,
        format: Union[str, dict[str, Any], None] = None,
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
        keep_alive: Union[int, str],
        stream: bool = False,
        raw: bool = True,
        format: Union[str, dict[str, Any], None] = None,
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
            prompt=prompt.strip(),
            options=options,
            keep_alive=keep_alive,
            stream=stream,
            raw=raw,
            format=format,
        )
