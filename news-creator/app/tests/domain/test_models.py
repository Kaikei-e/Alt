"""Tests for domain models."""

import pytest
from pydantic import ValidationError

from news_creator.domain.models import (
    GeneratedContent,
    GenerateRequest,
    LLMGenerateResponse,
    NewsGenerationRequest,
    RerankRequest,
    SummarizeRequest,
    SummarizeResponse,
)


def test_summarize_request_validation() -> None:
    """Test SummarizeRequest validates required fields."""
    # Valid request
    request = SummarizeRequest(article_id="test-123", content="This is test content")
    assert request.article_id == "test-123"
    assert request.content == "This is test content"
    assert request.priority == "low"  # Default value


def test_summarize_request_priority_field() -> None:
    """Test SummarizeRequest priority field validation."""
    # Valid high priority
    request_high = SummarizeRequest(
        article_id="test-123", content="This is test content", priority="high"
    )
    assert request_high.priority == "high"

    # Valid low priority
    request_low = SummarizeRequest(
        article_id="test-123", content="This is test content", priority="low"
    )
    assert request_low.priority == "low"


def test_summarize_request_rejects_invalid_priority() -> None:
    """Test SummarizeRequest rejects invalid priority values."""
    with pytest.raises(ValidationError):
        SummarizeRequest(
            article_id="test-123", content="This is test content", priority="invalid"
        )


def test_summarize_request_rejects_empty_article_id() -> None:
    """Test SummarizeRequest rejects empty article_id."""
    with pytest.raises(ValidationError):
        SummarizeRequest(article_id="", content="content")


def test_summarize_request_rejects_empty_content() -> None:
    """Test SummarizeRequest rejects empty content."""
    with pytest.raises(ValidationError):
        SummarizeRequest(article_id="test-123", content="")


def test_summarize_response_creation() -> None:
    """Test SummarizeResponse can be created with required fields."""
    prompt_tokens = 100
    completion_tokens = 50
    total_duration_ms = 1500.5
    response = SummarizeResponse(
        success=True,
        article_id="test-123",
        summary="Test summary",
        model="test-model",
        prompt_tokens=prompt_tokens,
        completion_tokens=completion_tokens,
        total_duration_ms=total_duration_ms,
    )
    assert response.success is True
    assert response.article_id == "test-123"
    assert response.summary == "Test summary"
    assert response.model == "test-model"
    assert response.prompt_tokens == prompt_tokens
    assert response.completion_tokens == completion_tokens
    assert response.total_duration_ms == total_duration_ms


def test_summarize_response_allows_optional_fields() -> None:
    """Test SummarizeResponse allows optional fields to be None."""
    response = SummarizeResponse(
        success=False, article_id="test-123", summary="", model="test-model"
    )
    assert response.prompt_tokens is None
    assert response.completion_tokens is None
    assert response.total_duration_ms is None


def test_generate_request_with_defaults() -> None:
    """Test GenerateRequest with default values."""
    request = GenerateRequest(prompt="Test prompt")
    assert request.prompt == "Test prompt"
    assert request.model is None
    assert request.stream is False
    assert request.keep_alive is None
    assert request.options == {}


def test_generate_request_with_options() -> None:
    """Test GenerateRequest with custom options."""
    options = {"temperature": 0.7, "top_p": 0.9}
    keep_alive = 300
    request = GenerateRequest(
        prompt="Test prompt",
        model="custom-model",
        stream=True,
        keep_alive=keep_alive,
        options=options,
    )
    assert request.prompt == "Test prompt"
    assert request.model == "custom-model"
    assert request.stream is True
    assert request.keep_alive == keep_alive
    assert request.options == options


def test_generate_request_rejects_empty_prompt() -> None:
    """Test GenerateRequest rejects empty prompt."""
    with pytest.raises(ValidationError):
        GenerateRequest(prompt="")


def test_news_generation_request_with_defaults() -> None:
    """Test NewsGenerationRequest with default values."""
    default_max_length = 500
    request = NewsGenerationRequest(topic="AI technology")
    assert request.topic == "AI technology"
    assert request.style == "news"
    assert request.max_length == default_max_length
    assert request.language == "en"
    assert request.metadata is None


def test_news_generation_request_with_custom_values() -> None:
    """Test NewsGenerationRequest with custom values."""
    metadata = {"source": "test"}
    custom_max_length = 1000
    request = NewsGenerationRequest(
        topic="Climate change",
        style="blog",
        max_length=custom_max_length,
        language="ja",
        metadata=metadata,
    )
    assert request.topic == "Climate change"
    assert request.style == "blog"
    assert request.max_length == custom_max_length
    assert request.language == "ja"
    assert request.metadata == metadata


def test_generated_content_creation() -> None:
    """Test GeneratedContent model creation."""
    metadata = {"model": "test-model"}
    confidence = 0.95
    word_count = 5
    content = GeneratedContent(
        content="This is generated content.",
        title="Test Title",
        summary="Test summary",
        confidence=confidence,
        word_count=word_count,
        language="en",
        metadata=metadata,
    )
    assert content.content == "This is generated content."
    assert content.title == "Test Title"
    assert content.summary == "Test summary"
    assert content.confidence == confidence
    assert content.word_count == word_count
    assert content.language == "en"
    assert content.metadata == metadata


def test_llm_generate_response_creation() -> None:
    """Test LLMGenerateResponse model creation."""
    prompt_eval_count = 100
    eval_count = 50
    total_duration = 1500000000
    response = LLMGenerateResponse(
        response="Generated text",
        model="test-model",
        done=True,
        done_reason="stop",
        prompt_eval_count=prompt_eval_count,
        eval_count=eval_count,
        total_duration=total_duration,
    )
    assert response.response == "Generated text"
    assert response.model == "test-model"
    assert response.done is True
    assert response.done_reason == "stop"
    assert response.prompt_eval_count == prompt_eval_count
    assert response.eval_count == eval_count
    assert response.total_duration == total_duration


def test_llm_generate_response_with_optional_fields() -> None:
    """Test LLMGenerateResponse allows optional fields."""
    response = LLMGenerateResponse(response="Generated text", model="test-model")
    assert response.done is None
    assert response.done_reason is None
    assert response.prompt_eval_count is None
    assert response.eval_count is None
    assert response.total_duration is None


def test_generate_request_keep_alive_string_validation() -> None:
    """G7-05: GenerateRequest validates keep_alive duration strings."""
    for valid in ("5m", "30s", "1h", "0", "-1", 300, None):
        req = GenerateRequest(prompt="hello", keep_alive=valid)
        assert req.keep_alive == valid

    for invalid in ("999999h; rm -rf /", "forever", "10d", "abc", "-5m"):
        with pytest.raises(ValidationError):
            GenerateRequest(prompt="hello", keep_alive=invalid)


def test_rerank_request_model_and_query_bounds() -> None:
    """G7-02: RerankRequest bounds query length and restricts model to allowlist."""
    req_default = RerankRequest(query="valid query", candidates=["c1"])
    assert req_default.model is None

    req_explicit = RerankRequest(
        query="valid query",
        candidates=["c1"],
        model="BAAI/bge-reranker-v2-m3",
    )
    assert req_explicit.model == "BAAI/bge-reranker-v2-m3"

    with pytest.raises(ValidationError):
        RerankRequest(
            query="valid query",
            candidates=["c1"],
            model="attacker/malicious-hub-model",
        )

    with pytest.raises(ValidationError):
        RerankRequest(
            query="q" * 4097,
            candidates=["c1"],
        )
