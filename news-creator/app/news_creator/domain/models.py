"""Domain models for News Creator service."""

from dataclasses import dataclass, field
from typing import Dict, Any, Optional, Union
from pydantic import BaseModel, Field, field_validator


class SummarizeRequest(BaseModel):
    """Request model for article summarization."""

    article_id: str = Field(min_length=1)
    content: str = Field(min_length=1)


class SummarizeResponse(BaseModel):
    """Response model for article summarization."""

    success: bool
    article_id: str
    summary: str
    model: str
    prompt_tokens: Optional[int] = None
    completion_tokens: Optional[int] = None
    total_duration_ms: Optional[float] = None


class GenerateRequest(BaseModel):
    """Request model for generic LLM generation."""

    prompt: str = Field(min_length=1)
    model: Optional[str] = None
    stream: bool = False
    keep_alive: Optional[Union[int, str]] = None
    options: Dict[str, Any] = Field(default_factory=dict)


@dataclass
class NewsGenerationRequest:
    """Request model for personalized news content generation."""

    topic: str
    style: str = "news"  # news, blog, summary
    max_length: int = 500
    language: str = "en"
    metadata: Optional[Dict[str, Any]] = None


@dataclass
class GeneratedContent:
    """Domain model for generated content with metadata."""

    content: str
    title: str
    summary: str
    confidence: float
    word_count: int
    language: str
    metadata: Dict[str, Any]


@dataclass
class LLMGenerateResponse:
    """Response model from LLM service (e.g., Ollama)."""

    response: str
    model: str
    done: Optional[bool] = None
    done_reason: Optional[str] = None
    prompt_eval_count: Optional[int] = None
    eval_count: Optional[int] = None
    total_duration: Optional[int] = None  # in nanoseconds
