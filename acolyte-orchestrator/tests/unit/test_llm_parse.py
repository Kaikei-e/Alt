"""Unit tests for generate_validated — Pydantic-validated LLM structured output."""

from __future__ import annotations

import json

import pytest
from pydantic import BaseModel

from acolyte.port.llm_provider import LLMResponse
from acolyte.usecase.graph.llm_parse import generate_validated


class SampleOutput(BaseModel):
    reasoning: str
    sections: list[dict]


class FakeLLMForParse:
    def __init__(self, responses: list[str]) -> None:
        self._responses = list(responses)
        self._call_count = 0

    async def generate(self, prompt: str, **kwargs: object) -> LLMResponse:
        idx = min(self._call_count, len(self._responses) - 1)
        self._call_count += 1
        return LLMResponse(text=self._responses[idx], model="fake")


@pytest.mark.asyncio
async def test_valid_json_passes_pydantic() -> None:
    valid = json.dumps({"reasoning": "ok", "sections": [{"key": "a"}]})
    llm = FakeLLMForParse([valid])
    result = await generate_validated(llm, "prompt", SampleOutput)
    assert result.reasoning == "ok"
    assert len(result.sections) == 1


@pytest.mark.asyncio
async def test_invalid_json_retries_once() -> None:
    invalid = "not json at all"
    valid = json.dumps({"reasoning": "retry ok", "sections": []})
    llm = FakeLLMForParse([invalid, valid])
    result = await generate_validated(llm, "prompt", SampleOutput, retries=1)
    assert result.reasoning == "retry ok"
    assert llm._call_count == 2


@pytest.mark.asyncio
async def test_all_retries_exhausted_returns_fallback() -> None:
    invalid = "bad"
    llm = FakeLLMForParse([invalid, invalid, invalid])
    fallback = SampleOutput(reasoning="fallback", sections=[])
    result = await generate_validated(llm, "prompt", SampleOutput, retries=1, fallback=fallback)
    assert result.reasoning == "fallback"


@pytest.mark.asyncio
async def test_pydantic_validation_failure_retries() -> None:
    # Valid JSON but wrong schema (missing required field)
    wrong_schema = json.dumps({"reasoning": "ok"})  # missing 'sections'
    valid = json.dumps({"reasoning": "good", "sections": [{"key": "b"}]})
    llm = FakeLLMForParse([wrong_schema, valid])
    result = await generate_validated(llm, "prompt", SampleOutput, retries=1)
    assert result.reasoning == "good"


@pytest.mark.asyncio
async def test_no_fallback_raises_on_exhaustion() -> None:
    invalid = "bad"
    llm = FakeLLMForParse([invalid, invalid])
    with pytest.raises(ValueError, match="LLM output validation failed"):
        await generate_validated(llm, "prompt", SampleOutput, retries=1)


@pytest.mark.asyncio
async def test_passes_format_and_kwargs() -> None:
    """generate_validated should pass format schema and extra kwargs to LLM."""
    valid = json.dumps({"reasoning": "ok", "sections": []})

    class CaptureLLM:
        captured_kwargs: dict = {}

        async def generate(self, prompt: str, **kwargs: object) -> LLMResponse:
            self.captured_kwargs = dict(kwargs)
            return LLMResponse(text=valid, model="fake")

    llm = CaptureLLM()
    await generate_validated(llm, "prompt", SampleOutput, temperature=0, num_predict=512)
    assert llm.captured_kwargs["temperature"] == 0
    assert llm.captured_kwargs["num_predict"] == 512
    assert "format" in llm.captured_kwargs


class ErrorThenSuccessLLM:
    """LLM that raises on first call, succeeds on second."""

    def __init__(self, error: Exception, success_response: str) -> None:
        self._error = error
        self._success = success_response
        self._call_count = 0

    async def generate(self, prompt: str, **kwargs: object) -> LLMResponse:
        self._call_count += 1
        if self._call_count == 1:
            raise self._error
        return LLMResponse(text=self._success, model="fake")


@pytest.mark.asyncio
async def test_generate_exception_retries_and_succeeds() -> None:
    """If llm.generate() raises (e.g. ReadTimeout), it should retry and succeed."""
    valid = json.dumps({"reasoning": "recovered", "sections": []})
    llm = ErrorThenSuccessLLM(TimeoutError("ReadTimeout"), valid)
    result = await generate_validated(llm, "prompt", SampleOutput, retries=1)
    assert result.reasoning == "recovered"
    assert llm._call_count == 2


@pytest.mark.asyncio
async def test_generate_exception_exhausted_uses_fallback() -> None:
    """If llm.generate() raises on all attempts, fallback is returned."""

    class AlwaysErrorLLM:
        async def generate(self, prompt: str, **kwargs: object) -> LLMResponse:
            raise TimeoutError("ReadTimeout")

    fallback = SampleOutput(reasoning="fallback", sections=[])
    result = await generate_validated(AlwaysErrorLLM(), "prompt", SampleOutput, retries=1, fallback=fallback)
    assert result.reasoning == "fallback"


# --- Truncation detection tests ---


class TruncationLLM:
    """LLM that returns truncated JSON with high completion_tokens, then succeeds."""

    def __init__(self, responses: list[LLMResponse]) -> None:
        self._responses = list(responses)
        self._idx = 0
        self.calls: list[dict] = []

    async def generate(self, prompt: str, **kwargs: object) -> LLMResponse:
        self.calls.append({"prompt": prompt, **kwargs})
        resp = self._responses[min(self._idx, len(self._responses) - 1)]
        self._idx += 1
        return resp


@pytest.mark.asyncio
async def test_detects_truncation_and_increases_budget() -> None:
    """When completion_tokens >= 95% of num_predict and JSON is truncated, increase budget +25%."""
    truncated = LLMResponse(
        text='{"reasoning": "thinking...', model="test", completion_tokens=1900,
    )
    valid = LLMResponse(
        text=json.dumps({"reasoning": "ok", "sections": []}), model="test", completion_tokens=500,
    )
    llm = TruncationLLM([truncated, valid])

    result = await generate_validated(llm, "prompt", SampleOutput, num_predict=2000, temperature=0)

    assert result.reasoning == "ok"
    assert llm.calls[1]["num_predict"] == 2500  # 2000 * 1.25


@pytest.mark.asyncio
async def test_does_not_increase_budget_on_non_truncation() -> None:
    """When completion_tokens is well below num_predict, don't increase budget."""
    bad_json = LLMResponse(text="not json", model="test", completion_tokens=100)
    valid = LLMResponse(
        text=json.dumps({"reasoning": "ok", "sections": []}), model="test", completion_tokens=50,
    )
    llm = TruncationLLM([bad_json, valid])

    result = await generate_validated(llm, "prompt", SampleOutput, num_predict=2000, temperature=0)

    assert result.reasoning == "ok"
    assert llm.calls[1]["num_predict"] == 2000  # unchanged


@pytest.mark.asyncio
async def test_budget_increase_bounded_once() -> None:
    """Budget increases at most once across retries."""
    truncated = LLMResponse(text='{"reasoning": "...', model="test", completion_tokens=1950)
    llm = TruncationLLM([truncated, truncated])

    fallback = SampleOutput(reasoning="fallback", sections=[])
    result = await generate_validated(
        llm, "prompt", SampleOutput, num_predict=2000, temperature=0, fallback=fallback,
    )

    assert result.reasoning == "fallback"
    # Second call should have 2500, not 3125
    assert llm.calls[1]["num_predict"] == 2500
