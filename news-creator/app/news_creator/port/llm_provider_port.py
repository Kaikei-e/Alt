"""Port interface for LLM provider."""

from abc import ABC, abstractmethod
from asyncio import Event
from contextlib import asynccontextmanager
from typing import AsyncGenerator, AsyncIterator, Any, Optional, Tuple, Union

from news_creator.domain.models import LLMGenerateResponse

class LLMProviderPort(ABC):
    """Abstract interface for LLM providers."""

    @abstractmethod
    async def generate(
        self,
        prompt: str,
        *,
        model: str | None = None,
        num_predict: int | None = None,
        stream: bool = False,
        keep_alive: int | str | None = None,
        format: str | dict[str, Any] | None = None,
        options: dict[str, Any] | None = None,
        priority: str = "low",
    ) -> LLMGenerateResponse | AsyncIterator[LLMGenerateResponse]:
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
        cancel_event: Any | None = None,
        task_id: str | None = None,
        model: str | None = None,
        num_predict: int | None = None,
        keep_alive: int | str | None = None,
        format: str | dict[str, Any] | None = None,
        options: dict[str, Any] | None = None,
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
    async def hold_slot(
        self, is_high_priority: bool = False
    ) -> AsyncGenerator[tuple[float, Event | None, str | None], None]:
        """Hold a semaphore slot for the duration of the context.

        Yields:
            Tuple of (wait_time, cancel_event, task_id)
        """
        # Default implementation for backward compat - subclasses override
        yield (0.0, None, None)  # pragma: no cover

    async def list_models(self) -> list[dict[str, Any]]:
        """List available models. Optional — not all providers support this."""
        return []  # pragma: no cover

    def queue_status(self) -> dict[str, Any]:
        """Queue depth/availability status for monitoring.

        Optional — not all providers expose semaphore internals. Default
        implementation reports an always-accepting, slot-less queue.
        """
        return {
            "rt_queue": 0,
            "be_queue": 0,
            "total_slots": 0,
            "available_slots": 0,
            "accepting": True,
            "max_queue_depth": 0,
        }  # pragma: no cover

    async def chat_generate(
        self,
        payload: dict[str, Any],
        *,
        priority: str = "high",
    ) -> dict[str, Any]:
        """Non-streaming /api/chat call through the priority semaphore.

        Used for structured output with thinking mode (plan-query, morning
        letter, etc.) as well as batch-oriented callers (recap). The payload
        follows Ollama /api/chat format: messages, model, format, options.

        Args:
            payload: Ollama chat request payload
            priority: "high" (default) reserves an RT slot for interactive
                callers (Ask Augur chat proxy); batch callers must pass
                "low" so they don't compete with RT chat for bandwidth.

        Returns:
            Response dict in /api/chat format
        """
        raise NotImplementedError  # pragma: no cover

    @abstractmethod
    async def initialize(self) -> None:
        """Initialize the LLM provider (e.g., create client session)."""
        pass

    @abstractmethod
    async def cleanup(self) -> None:
        """Cleanup resources (e.g., close client session)."""
        pass
