"""Port interface for LLM provider."""

from abc import ABC, abstractmethod
from typing import Dict, Any, Optional, Union
from news_creator.domain.models import LLMGenerateResponse


class LLMProviderPort(ABC):
    """Abstract interface for LLM providers."""

    @abstractmethod
    async def generate(
        self,
        prompt: str,
        *,
        model: Optional[str] = None,
        num_predict: Optional[int] = None,
        stream: bool = False,
        keep_alive: Optional[Union[int, str]] = None,
        format: Optional[str] = None,
        options: Optional[Dict[str, Any]] = None,
    ) -> LLMGenerateResponse:
        """
        Generate text using the LLM.

        Args:
            prompt: The input prompt for generation
            model: Optional model name override
            num_predict: Optional max tokens to generate override
            stream: Whether to stream the response
            keep_alive: Keep-alive duration
            format: Optional output format (e.g., "json" for structured output)
            options: Additional generation options

        Returns:
            LLMGenerateResponse with generated text and metadata

        Raises:
            ValueError: If prompt is empty
            RuntimeError: If LLM service fails
        """
        pass

    @abstractmethod
    async def initialize(self) -> None:
        """Initialize the LLM provider (e.g., create client session)."""
        pass

    @abstractmethod
    async def cleanup(self) -> None:
        """Cleanup resources (e.g., close client session)."""
        pass
