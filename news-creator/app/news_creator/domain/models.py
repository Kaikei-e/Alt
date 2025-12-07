"""Domain models for News Creator service."""

from dataclasses import dataclass
from typing import Dict, Any, List, Optional, Union
from uuid import UUID
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


class RepresentativeSentence(BaseModel):
    """Representative sentence with metadata."""

    text: str = Field(min_length=1, description="Sentence text")
    published_at: Optional[str] = Field(
        default=None, description="Publication date in RFC3339 format"
    )
    source_url: Optional[str] = Field(default=None, description="Source article URL")
    article_id: Optional[str] = Field(default=None, description="Source article ID")
    is_centroid: bool = Field(default=False, description="Whether this is the centroid sentence")


class RecapClusterInput(BaseModel):
    """Cluster information passed from recap-worker."""

    cluster_id: int = Field(ge=0)
    representative_sentences: List[RepresentativeSentence] = Field(
        min_length=1,
        max_length=20,
        description="Representative sentences extracted by the subworker",
    )
    top_terms: Optional[List[str]] = Field(default=None)

    @field_validator("representative_sentences", mode="after")
    @classmethod
    def strip_sentences(cls, sentences: List[RepresentativeSentence]) -> List[RepresentativeSentence]:
        cleaned: List[RepresentativeSentence] = []
        for sentence in sentences:
            stripped_text = sentence.text.strip()
            if stripped_text:
                cleaned.append(
                    RepresentativeSentence(
                        text=stripped_text,
                        published_at=sentence.published_at,
                        source_url=sentence.source_url,
                        article_id=sentence.article_id,
                        is_centroid=sentence.is_centroid,
                    )
                )
        if not cleaned:
            raise ValueError("representative_sentences must contain at least one sentence")
        return cleaned


class RecapSummaryOptions(BaseModel):
    """Optional parameters to steer recap summary generation."""

    max_bullets: Optional[int] = Field(default=5, ge=1, le=15)
    temperature: Optional[float] = Field(default=None, ge=0.0, le=2.0)


class RecapSummaryRequest(BaseModel):
    """Request payload posted by recap-worker."""

    job_id: UUID
    genre: str = Field(min_length=1)
    clusters: List[RecapClusterInput] = Field(min_length=1, max_length=300)
    genre_highlights: Optional[List[RepresentativeSentence]] = None
    options: Optional[RecapSummaryOptions] = None


class RecapSummary(BaseModel):
    """Structured summary expected by recap-worker."""

    title: str = Field(min_length=1, max_length=200)
    bullets: List[str] = Field(min_length=1, max_length=15)
    language: str = Field(pattern="^ja$")

    @field_validator("bullets", mode="after")
    @classmethod
    def validate_bullets(cls, bullets: List[str]) -> List[str]:
        cleaned: List[str] = []
        for bullet in bullets:
            stripped = bullet.strip()
            if stripped:
                cleaned.append(stripped)
        if not cleaned:
            raise ValueError("bullets must contain at least one non-empty item")
        return cleaned


class RecapSummaryMetadata(BaseModel):
    """Metadata describing the generation."""

    model: str = Field(min_length=1)
    temperature: Optional[float] = None
    prompt_tokens: Optional[int] = Field(default=None, ge=0)
    completion_tokens: Optional[int] = Field(default=None, ge=0)
    processing_time_ms: Optional[int] = Field(default=None, ge=0)
    json_validation_errors: int = Field(default=0, ge=0)
    summary_length_bullets: int = Field(default=0, ge=0)


class RecapSummaryResponse(BaseModel):
    """Response returned to recap-worker."""

    job_id: UUID
    genre: str
    summary: RecapSummary
    metadata: RecapSummaryMetadata
