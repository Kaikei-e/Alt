"""Tests for domain models."""

import pytest
from news_creator.domain.models import (
    SummarizeRequest,
    SummarizeResponse,
    GenerateRequest,
    NewsGenerationRequest,
    GeneratedContent,
    LLMGenerateResponse,
)


def test_summarize_request_validation():
    """Test SummarizeRequest validates required fields."""
    # Valid request
    request = SummarizeRequest(
        article_id="test-123",
        content="This is test content"
    )
    assert request.article_id == "test-123"
    assert request.content == "This is test content"


def test_summarize_request_rejects_empty_article_id():
    """Test SummarizeRequest rejects empty article_id."""
    with pytest.raises(ValueError):
        SummarizeRequest(article_id="", content="content")


def test_summarize_request_rejects_empty_content():
    """Test SummarizeRequest rejects empty content."""
    with pytest.raises(ValueError):
        SummarizeRequest(article_id="test-123", content="")


def test_summarize_response_creation():
    """Test SummarizeResponse can be created with required fields."""
    response = SummarizeResponse(
        success=True,
        article_id="test-123",
        summary="Test summary",
        model="test-model",
        prompt_tokens=100,
        completion_tokens=50,
        total_duration_ms=1500.5
    )
    assert response.success is True
    assert response.article_id == "test-123"
    assert response.summary == "Test summary"
    assert response.model == "test-model"
    assert response.prompt_tokens == 100
    assert response.completion_tokens == 50
    assert response.total_duration_ms == 1500.5


def test_summarize_response_allows_optional_fields():
    """Test SummarizeResponse allows optional fields to be None."""
    response = SummarizeResponse(
        success=False,
        article_id="test-123",
        summary="",
        model="test-model"
    )
    assert response.prompt_tokens is None
    assert response.completion_tokens is None
    assert response.total_duration_ms is None


def test_generate_request_with_defaults():
    """Test GenerateRequest with default values."""
    request = GenerateRequest(prompt="Test prompt")
    assert request.prompt == "Test prompt"
    assert request.model is None
    assert request.stream is False
    assert request.keep_alive is None
    assert request.options == {}


def test_generate_request_with_options():
    """Test GenerateRequest with custom options."""
    options = {"temperature": 0.7, "top_p": 0.9}
    request = GenerateRequest(
        prompt="Test prompt",
        model="custom-model",
        stream=True,
        keep_alive=300,
        options=options
    )
    assert request.prompt == "Test prompt"
    assert request.model == "custom-model"
    assert request.stream is True
    assert request.keep_alive == 300
    assert request.options == options


def test_generate_request_rejects_empty_prompt():
    """Test GenerateRequest rejects empty prompt."""
    with pytest.raises(ValueError):
        GenerateRequest(prompt="")


def test_news_generation_request_with_defaults():
    """Test NewsGenerationRequest with default values."""
    request = NewsGenerationRequest(topic="AI technology")
    assert request.topic == "AI technology"
    assert request.style == "news"
    assert request.max_length == 500
    assert request.language == "en"
    assert request.metadata is None


def test_news_generation_request_with_custom_values():
    """Test NewsGenerationRequest with custom values."""
    metadata = {"source": "test"}
    request = NewsGenerationRequest(
        topic="Climate change",
        style="blog",
        max_length=1000,
        language="ja",
        metadata=metadata
    )
    assert request.topic == "Climate change"
    assert request.style == "blog"
    assert request.max_length == 1000
    assert request.language == "ja"
    assert request.metadata == metadata


def test_generated_content_creation():
    """Test GeneratedContent model creation."""
    metadata = {"model": "test-model"}
    content = GeneratedContent(
        content="This is generated content.",
        title="Test Title",
        summary="Test summary",
        confidence=0.95,
        word_count=5,
        language="en",
        metadata=metadata
    )
    assert content.content == "This is generated content."
    assert content.title == "Test Title"
    assert content.summary == "Test summary"
    assert content.confidence == 0.95
    assert content.word_count == 5
    assert content.language == "en"
    assert content.metadata == metadata


def test_llm_generate_response_creation():
    """Test LLMGenerateResponse model creation."""
    response = LLMGenerateResponse(
        response="Generated text",
        model="test-model",
        done=True,
        done_reason="stop",
        prompt_eval_count=100,
        eval_count=50,
        total_duration=1500000000
    )
    assert response.response == "Generated text"
    assert response.model == "test-model"
    assert response.done is True
    assert response.done_reason == "stop"
    assert response.prompt_eval_count == 100
    assert response.eval_count == 50
    assert response.total_duration == 1500000000


def test_llm_generate_response_with_optional_fields():
    """Test LLMGenerateResponse allows optional fields."""
    response = LLMGenerateResponse(
        response="Generated text",
        model="test-model"
    )
    assert response.done is None
    assert response.done_reason is None
    assert response.prompt_eval_count is None
    assert response.eval_count is None
    assert response.total_duration is None
