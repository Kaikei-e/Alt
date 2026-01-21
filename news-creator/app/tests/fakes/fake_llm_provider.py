"""
Fake LLM provider implementation for integration testing.

This module provides a fake implementation of LLMProviderPort that can be used
for integration tests without requiring an actual Ollama server. Unlike mocks,
fakes implement the full interface and can have configurable behavior.

Usage:
    fake_provider = FakeLLMProvider()
    fake_provider.add_response("summarize this", '{"title": "Test", "bullets": []}')

    # Use in tests
    usecase = RecapSummaryUsecase(config=config, llm_provider=fake_provider)
    result = await usecase.generate_summary(request)

    # Verify calls
    assert len(fake_provider.call_history) == 1
    assert "summarize" in fake_provider.call_history[0].lower()
"""

import json
from dataclasses import dataclass, field
from typing import Any, AsyncIterator, Dict, List, Optional, Union

from news_creator.domain.models import LLMGenerateResponse
from news_creator.port.llm_provider_port import LLMProviderPort


@dataclass
class FakeCallRecord:
    """Record of a call made to the fake provider."""

    prompt: str
    model: Optional[str] = None
    num_predict: Optional[int] = None
    stream: bool = False
    format: Optional[Union[str, Dict[str, Any]]] = None
    options: Optional[Dict[str, Any]] = None


class FakeLLMProvider(LLMProviderPort):
    """
    Fake LLM provider for integration testing.

    This implementation stores all calls and returns configurable responses.
    It's useful for integration tests where you want to verify the interaction
    between multiple components without mocking individual methods.

    Attributes:
        call_history: List of all calls made to generate()
        responses: Dictionary mapping prompt patterns to responses
        default_response: Response to return when no pattern matches
        initialized: Whether initialize() has been called
    """

    def __init__(
        self,
        default_response: Optional[str] = None,
        default_model: str = "gemma3-4b-12k",
    ):
        """
        Initialize the fake provider.

        Args:
            default_response: Default response text when no pattern matches.
                             If None, a generic JSON response is used.
            default_model: Model name to return in responses.
        """
        self.call_history: List[FakeCallRecord] = []
        self.responses: Dict[str, str] = {}
        self.default_model = default_model
        self.default_response = default_response or json.dumps({
            "title": "Default Fake Title",
            "bullets": ["Default bullet point from fake provider."],
            "language": "en"
        })
        self.initialized = False
        self._should_fail = False
        self._fail_message = ""
        self._latency_ms = 0

    async def initialize(self) -> None:
        """Initialize the fake provider."""
        self.initialized = True

    async def cleanup(self) -> None:
        """Cleanup the fake provider."""
        self.initialized = False

    def add_response(self, pattern: str, response: str) -> None:
        """
        Add a response for a specific prompt pattern.

        Args:
            pattern: Substring to match in the prompt
            response: Response text to return
        """
        self.responses[pattern] = response

    def add_json_response(
        self,
        pattern: str,
        title: str,
        bullets: List[str],
        language: str = "ja"
    ) -> None:
        """
        Add a JSON-formatted response for recap summary testing.

        Args:
            pattern: Substring to match in the prompt
            title: Summary title
            bullets: List of bullet points
            language: Language code (default: "ja")
        """
        self.responses[pattern] = json.dumps({
            "title": title,
            "bullets": bullets,
            "language": language
        })

    def set_failure(self, message: str = "Fake LLM failure") -> None:
        """
        Configure the fake to raise an error on next generate() call.

        Args:
            message: Error message to raise
        """
        self._should_fail = True
        self._fail_message = message

    def clear_failure(self) -> None:
        """Clear the failure state."""
        self._should_fail = False
        self._fail_message = ""

    def set_latency(self, latency_ms: int) -> None:
        """
        Set simulated latency for generate() calls.

        Args:
            latency_ms: Latency in milliseconds
        """
        self._latency_ms = latency_ms

    def reset(self) -> None:
        """Reset call history and configuration."""
        self.call_history.clear()
        self.responses.clear()
        self._should_fail = False
        self._fail_message = ""
        self._latency_ms = 0

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
    ) -> Union[LLMGenerateResponse, AsyncIterator[LLMGenerateResponse]]:
        """
        Generate a fake response.

        Records the call and returns a configurable response.
        """
        # Record the call
        self.call_history.append(FakeCallRecord(
            prompt=prompt,
            model=model,
            num_predict=num_predict,
            stream=stream,
            format=format,
            options=options,
        ))

        # Simulate failure if configured
        if self._should_fail:
            self._should_fail = False  # Reset after one failure
            raise RuntimeError(self._fail_message)

        # Simulate latency if configured
        if self._latency_ms > 0:
            import asyncio
            await asyncio.sleep(self._latency_ms / 1000)

        # Find matching response
        response_text = self.default_response
        for pattern, response in self.responses.items():
            if pattern.lower() in prompt.lower():
                response_text = response
                break

        # Handle streaming (simplified)
        if stream:
            async def stream_generator():
                yield LLMGenerateResponse(
                    response=response_text,
                    model=model or self.default_model,
                    done=True,
                )
            return stream_generator()

        # Return non-streaming response
        return LLMGenerateResponse(
            response=response_text,
            model=model or self.default_model,
            done=True,
            done_reason="stop",
            prompt_eval_count=len(prompt) // 4,  # Rough token estimate
            eval_count=len(response_text) // 4,
            total_duration=max(self._latency_ms * 1_000_000, 500_000_000),  # ns
            load_duration=10_000_000,
            prompt_eval_duration=100_000_000,
            eval_duration=300_000_000,
        )

    def get_last_call(self) -> Optional[FakeCallRecord]:
        """Get the most recent call record."""
        return self.call_history[-1] if self.call_history else None

    def get_call_count(self) -> int:
        """Get the total number of calls."""
        return len(self.call_history)

    def assert_called_once(self) -> None:
        """Assert that generate() was called exactly once."""
        if len(self.call_history) != 1:
            raise AssertionError(
                f"Expected 1 call, got {len(self.call_history)}"
            )

    def assert_not_called(self) -> None:
        """Assert that generate() was never called."""
        if self.call_history:
            raise AssertionError(
                f"Expected no calls, got {len(self.call_history)}"
            )

    def assert_prompt_contains(self, text: str) -> None:
        """Assert that the last prompt contains the given text."""
        last_call = self.get_last_call()
        if not last_call:
            raise AssertionError("No calls recorded")
        if text.lower() not in last_call.prompt.lower():
            raise AssertionError(
                f"Expected prompt to contain '{text}', got: {last_call.prompt[:200]}..."
            )
