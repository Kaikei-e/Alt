"""Port interface for LLM provider."""

from abc import ABC, abstractmethod
from contextlib import asynccontextmanager
from typing import AsyncIterator, Dict, Any, Optional, Tuple, Union

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
        format: Optional[Union[str, Dict[str, Any]]] = None,
        options: Optional[Dict[str, Any]] = None,
        priority: str = "low",
    ) -> Union[LLMGenerateResponse, AsyncIterator[LLMGenerateResponse]]:
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
            priority: Request priority ("high" or "low"). High priority requests
                      bypass the low priority queue for faster processing.

        Returns:
            LLMGenerateResponse with generated text and metadata

        Raises:
            ValueError: If prompt is empty
            RuntimeError: If LLM service fails
        """
        pass

    @abstractmethod
    async def generate_raw(
        self,
        prompt: str,
        *,
        cancel_event: Optional[Any] = None,
        task_id: Optional[str] = None,
        model: Optional[str] = None,
        num_predict: Optional[int] = None,
        keep_alive: Optional[Union[int, str]] = None,
        format: Optional[Union[str, Dict[str, Any]]] = None,
        options: Optional[Dict[str, Any]] = None,
    ) -> LLMGenerateResponse:
        """
        Generate text without acquiring semaphore (for use inside hold_slot).

        Args:
            prompt: The input prompt for generation
            cancel_event: Optional cancellation event for preemption
            task_id: Optional task ID for tracking
            model: Optional model name override
            num_predict: Optional max tokens override
            keep_alive: Keep-alive duration
            format: Optional output format
            options: Additional generation options

        Returns:
            LLMGenerateResponse with generated text and metadata
        """
        pass

    @asynccontextmanager
    async def hold_slot(self, is_high_priority: bool = False):
        """Hold a semaphore slot for the duration of the context.

        Yields:
            Tuple of (wait_time, cancel_event, task_id)
        """
        # Default implementation for backward compat - subclasses override
        yield 0.0, None, None  # pragma: no cover

    @abstractmethod
    async def initialize(self) -> None:
        """Initialize the LLM provider (e.g., create client session)."""
        pass

    @abstractmethod
    async def cleanup(self) -> None:
        """Cleanup resources (e.g., close client session)."""
        pass
