"""Domain models for News Creator service."""

import re
from dataclasses import dataclass
from datetime import date
from typing import Annotated, Any, Literal
from uuid import UUID

from pydantic import (
    BaseModel,
    BeforeValidator,
    ConfigDict,
    Field,
    field_validator,
    model_validator,
)

_KEEP_ALIVE_RE = re.compile(r"^(?:\d+[smh]|-1|0)$")
_ALLOWED_RERANK_MODELS = frozenset({"BAAI/bge-reranker-v2-m3"})


def _parse_uuid(value: object) -> UUID:
    """Accept UUID instances or RFC-4122 strings at the JSON API boundary."""
    if isinstance(value, UUID):
        return value
    if isinstance(value, str):
        return UUID(value)
    msg = "UUID input should be a UUID instance or string"
    raise TypeError(msg)


ApiUUID = Annotated[UUID, BeforeValidator(_parse_uuid)]


class StrictFrozenModel(BaseModel):
    """API-boundary BaseModel: strict + frozen (DECREE §5)."""

    model_config = ConfigDict(strict=True, frozen=True)


class SummarizeRequest(StrictFrozenModel):
    """Request model for article summarization."""

    article_id: str = Field(min_length=1)
    content: str = Field(min_length=1)
    stream: bool = False
    priority: str = Field(default="low", pattern="^(high|low)$")


class SummarizeResponse(StrictFrozenModel):
    """Response model for article summarization."""

    success: bool
    article_id: str
    summary: str
    model: str
    prompt_tokens: int | None = None
    completion_tokens: int | None = None
    total_duration_ms: float | None = None


@dataclass(frozen=True, slots=True)
class SummaryMetadata:
    """Metadata about how a summary was generated (SummarizeUsecase internal
    result), replacing an untyped ``dict[str, Any]`` passed across the
    usecase/handler boundary."""

    model: str
    prompt_tokens: int | None = None
    completion_tokens: int | None = None
    total_duration_ms: float | None = None
    strategy: str | None = None


class GenerateRequest(StrictFrozenModel):
    """Request model for generic LLM generation."""

    prompt: str = Field(min_length=1)
    model: str | None = None
    stream: bool = False
    keep_alive: int | str | None = None
    options: dict[str, Any] = Field(default_factory=dict)

    @field_validator("keep_alive")
    @classmethod
    def _validate_keep_alive(cls, v: int | str | None) -> int | str | None:
        if isinstance(v, str) and not _KEEP_ALIVE_RE.fullmatch(v):
            msg = r"keep_alive string must match ^(?:\d+[smh]|-1|0)$"
            raise ValueError(msg)
        return v


@dataclass
class NewsGenerationRequest:
    """Request model for personalized news content generation."""

    topic: str
    style: str = "news"  # news, blog, summary
    max_length: int = 500
    language: str = "en"
    metadata: dict[str, Any] | None = None


@dataclass
class GeneratedContent:
    """Domain model for generated content with metadata."""

    content: str
    title: str
    summary: str
    confidence: float
    word_count: int
    language: str
    metadata: dict[str, Any]


@dataclass
class LLMGenerateResponse:
    """Response model from LLM service (e.g., Ollama)."""

    response: str
    model: str
    done: bool | None = None
    done_reason: str | None = None
    prompt_eval_count: int | None = None
    eval_count: int | None = None
    total_duration: int | None = None  # in nanoseconds
    load_duration: int | None = None  # in nanoseconds (model reload time)
    prompt_eval_duration: int | None = None  # in nanoseconds (prefill time)
    eval_duration: int | None = None  # in nanoseconds (decode time)


class RepresentativeSentence(StrictFrozenModel):
    """Representative sentence with metadata."""

    text: str = Field(min_length=1, description="Sentence text")
    published_at: str | None = Field(
        default=None, description="Publication date in RFC3339 format"
    )
    source_url: str | None = Field(default=None, description="Source article URL")
    article_id: str | None = Field(default=None, description="Source article ID")
    is_centroid: bool = Field(
        default=False, description="Whether this is the centroid sentence"
    )


class RecapClusterInput(StrictFrozenModel):
    """Cluster information passed from recap-worker."""

    cluster_id: int = Field(ge=0)
    representative_sentences: list[RepresentativeSentence] = Field(
        min_length=1,
        max_length=20,
        description="Representative sentences extracted by the subworker",
    )
    top_terms: list[str] | None = Field(default=None)

    @field_validator("representative_sentences", mode="after")
    @classmethod
    def strip_sentences(
        cls, sentences: list[RepresentativeSentence]
    ) -> list[RepresentativeSentence]:
        cleaned: list[RepresentativeSentence] = []
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
            msg = "representative_sentences must contain at least one sentence"
            raise ValueError(msg)
        return cleaned


class RecapSummaryOptions(StrictFrozenModel):
    """Optional parameters to steer recap summary generation."""

    max_bullets: int | None = Field(default=5, ge=1, le=15)
    temperature: float | None = Field(default=None, ge=0.0, le=2.0)


class RecapSummaryRequest(StrictFrozenModel):
    """Request payload posted by recap-worker."""

    job_id: ApiUUID
    genre: str = Field(min_length=1)
    clusters: list[RecapClusterInput] = Field(min_length=1, max_length=300)
    genre_highlights: list[RepresentativeSentence] | None = None
    options: RecapSummaryOptions | None = None
    window_days: int | None = Field(default=None, ge=1, le=30)


class IntermediateSummary(StrictFrozenModel):
    """Lightweight intermediate summary for map phase (hierarchical summarization)."""

    bullets: list[str] = Field(min_length=1, max_length=10)
    language: str = Field(pattern="^ja$", default="ja")

    @field_validator("bullets", mode="after")
    @classmethod
    def validate_bullets(cls, bullets: list[str]) -> list[str]:
        cleaned: list[str] = []
        for bullet in bullets:
            stripped = bullet.strip()
            if stripped:
                cleaned.append(stripped)
        if not cleaned:
            msg = "bullets must contain at least one non-empty item"
            raise ValueError(msg)
        return cleaned


class Reference(StrictFrozenModel):
    """Reference to a source article."""

    id: int = Field(
        ge=1, description="Reference ID (1-indexed, matches [n] in bullets)"
    )
    url: str = Field(min_length=1, description="Source article URL")
    domain: str = Field(min_length=1, description="Source domain")
    article_id: str | None = Field(
        default=None, description="Source article ID if available"
    )


class RecapSummary(StrictFrozenModel):
    """Structured summary expected by recap-worker."""

    title: str = Field(min_length=1, max_length=200)
    bullets: list[str] = Field(min_length=1, max_length=15)
    language: str = Field(pattern="^ja$")
    references: list[Reference] | None = Field(
        default=None,
        max_length=50,
        description="List of references cited in bullets (e.g., [1], [2])",
    )

    @field_validator("bullets", mode="after")
    @classmethod
    def validate_bullets(cls, bullets: list[str]) -> list[str]:
        cleaned: list[str] = []
        for bullet in bullets:
            stripped = bullet.strip()
            if stripped:
                cleaned.append(stripped)
        if not cleaned:
            msg = "bullets must contain at least one non-empty item"
            raise ValueError(msg)
        return cleaned


class RecapSummaryMetadata(StrictFrozenModel):
    """Metadata describing the generation."""

    model: str = Field(min_length=1)
    temperature: float | None = None
    prompt_tokens: int | None = Field(default=None, ge=0)
    completion_tokens: int | None = Field(default=None, ge=0)
    processing_time_ms: int | None = Field(default=None, ge=0)
    json_validation_errors: int = Field(default=0, ge=0)
    summary_length_bullets: int = Field(default=0, ge=0)
    reduce_depth: int | None = Field(default=0, ge=0)
    reduce_info_retention: float | None = Field(default=None, ge=0.0, le=1.0)
    is_degraded: bool = Field(default=False)
    degradation_reason: str | None = Field(default=None, max_length=500)


class RecapSummaryResponse(StrictFrozenModel):
    """Response returned to recap-worker."""

    job_id: ApiUUID
    genre: str
    summary: RecapSummary
    metadata: RecapSummaryMetadata


# ============================================================================
# Query Expansion Models (for RAG-Orchestrator)
# ============================================================================


class ConversationMessage(StrictFrozenModel):
    """A single message in a conversation history."""

    role: str = Field(description="Message role: 'user' or 'assistant'")
    content: str = Field(description="Message content")


class ExpandQueryRequest(StrictFrozenModel):
    """Request model for query expansion."""

    query: str = Field(min_length=1, description="Original user query to expand")
    japanese_count: int = Field(
        default=1, ge=0, le=5, description="Number of Japanese query variations"
    )
    english_count: int = Field(
        default=3, ge=0, le=10, description="Number of English query variations"
    )
    conversation_history: list[ConversationMessage] | None = Field(
        default=None, description="Recent conversation turns for coreference resolution"
    )
    priority: str = Field(
        default="high",
        pattern="^(high|low)$",
        description="Request priority. Defaults to high so query expansion bypasses summarization queue.",
    )


class ExpandQueryResponse(StrictFrozenModel):
    """Response model for query expansion."""

    expanded_queries: list[str] = Field(description="List of expanded query variations")
    original_query: str = Field(description="Original input query")
    model: str = Field(description="Model used for generation")
    processing_time_ms: float | None = Field(
        default=None, description="Processing time in milliseconds"
    )


# ============================================================================
# Re-ranking Models (for RAG-Orchestrator Cross-Encoder Re-ranking)
# ============================================================================


class RerankRequest(StrictFrozenModel):
    """Request model for cross-encoder re-ranking.

    Research basis:
    - Pinecone: +15-30% NDCG@10 improvement with cross-encoder
    - ZeroEntropy: -35% LLM hallucinations with re-ranking
    - Recommended: Rerank 50 candidates down to 10
    """

    query: str = Field(
        min_length=1, max_length=4096, description="Query to score candidates against"
    )
    candidates: list[str] = Field(
        min_length=1, max_length=100, description="List of candidate texts to re-rank"
    )
    model: str | None = Field(
        default=None,
        description="Cross-encoder model name (default: bge-reranker-v2-m3)",
    )
    top_k: int | None = Field(
        default=None,
        ge=1,
        le=100,
        description="Return only top-k results (default: return all)",
    )

    @field_validator("model")
    @classmethod
    def _validate_model(cls, v: str | None) -> str | None:
        if v is not None and v not in _ALLOWED_RERANK_MODELS:
            msg = f"Unsupported rerank model: {v}"
            raise ValueError(msg)
        return v


class RerankResultItem(StrictFrozenModel):
    """Single re-ranking result with index and score."""

    index: int = Field(ge=0, description="Original index of the candidate")
    score: float = Field(description="Cross-encoder relevance score (0.0 to 1.0)")


class RerankResponse(StrictFrozenModel):
    """Response model for cross-encoder re-ranking."""

    results: list[RerankResultItem] = Field(
        description="Re-ranked results sorted by score descending"
    )
    model: str = Field(description="Model used for re-ranking")
    processing_time_ms: float | None = Field(
        default=None, description="Processing time in milliseconds"
    )


# ============================================================================
# Batch Processing Models (for chatty microservice anti-pattern fix)
# ============================================================================


class BatchRecapSummaryRequest(StrictFrozenModel):
    """Batch request for multiple recap summaries."""

    requests: list[RecapSummaryRequest] = Field(
        min_length=1,
        max_length=50,
        description="List of individual recap summary requests",
    )


class BatchRecapSummaryError(StrictFrozenModel):
    """Error details for a failed request in a batch."""

    job_id: ApiUUID
    genre: str
    error: str = Field(description="Error message describing the failure")


class BatchRecapSummaryResponse(StrictFrozenModel):
    """Response for batch recap summary processing."""

    responses: list[RecapSummaryResponse] = Field(
        default_factory=list, description="List of successful recap summary responses"
    )
    errors: list[BatchRecapSummaryError] = Field(
        default_factory=list, description="List of errors for failed requests"
    )


# ============================================================================
# Query Planning Models (for Augur Conversational RAG)
# ============================================================================


class PlanQueryRequest(StrictFrozenModel):
    """Request model for structured query planning."""

    query: str = Field(min_length=1, description="User query to plan retrieval for")
    conversation_history: list[ConversationMessage] | None = Field(
        default=None, description="Recent conversation turns for coreference resolution"
    )
    article_id: str | None = Field(
        default=None, description="Article ID if query is article-scoped"
    )
    article_title: str | None = Field(
        default=None, description="Article title if query is article-scoped"
    )
    last_answer_scope: str | None = Field(
        default=None,
        description="Scope of the last answer: summary, detail, evidence, etc.",
    )
    priority: str = Field(
        default="high",
        pattern="^(high|low)$",
        description="Request priority. Defaults to high for real-time Augur queries.",
    )


class QueryPlan(StrictFrozenModel):
    """Structured output from query planning. Tells the retrieval layer what to do.

    Field ordering matters for small model accuracy (DSdev 2025):
    reasoning MUST come first so the model thinks before deciding.
    """

    reasoning: str = Field(
        description="Step-by-step reasoning: What is the user asking about? What topic? Is it a follow-up referencing prior context? What retrieval strategy fits best?"
    )
    resolved_query: str = Field(
        description="Self-contained search query with all pronouns and references resolved into explicit terms. Must contain the actual topic words."
    )
    search_queries: list[str] = Field(
        description="3-5 diverse search queries covering Japanese AND English variations, synonyms, and related terms"
    )
    intent: str = Field(
        description="Exactly one of: causal_explanation, temporal, synthesis, comparison, fact_check, topic_deep_dive, general"
    )
    retrieval_policy: str = Field(
        description="Exactly one of: global_only, article_only. Default: global_only"
    )
    answer_format: str = Field(
        description="Exactly one of: causal_analysis, summary, list, detail, comparison. Default: summary"
    )
    should_clarify: bool = Field(
        description="ALMOST ALWAYS false. Set true ONLY for bare ambiguous phrases like just 'もっと詳しく' with NO conversation history to resolve from."
    )
    topic_entities: list[str] = Field(
        default_factory=list, description="Key named entities extracted from the query"
    )


class PlanQueryResponse(StrictFrozenModel):
    """Response model for query planning."""

    plan: QueryPlan
    original_query: str = Field(description="Original input query")
    model: str = Field(description="Model used for planning")
    processing_time_ms: float | None = Field(
        default=None, description="Processing time in milliseconds"
    )


# ============================================================================
# Morning Letter Models
# ============================================================================


class MorningLetterRecapInput(StrictFrozenModel):
    """Recap summary data used as input for Morning Letter generation."""

    genre: str = Field(min_length=1)
    title: str = Field(min_length=1)
    bullets: list[str] = Field(min_length=1)
    window_days: int = Field(default=3, ge=1, le=30)


class MorningLetterGroupInput(StrictFrozenModel):
    """Overnight article group for Morning Letter."""

    group_id: ApiUUID
    articles: list[RepresentativeSentence] = Field(min_length=1)


class MorningLetterRequest(StrictFrozenModel):
    """Request for Morning Letter generation."""

    target_date: str = Field(min_length=1)
    edition_timezone: str = Field(default="Asia/Tokyo")
    recap_summaries: list[MorningLetterRecapInput] | None = None
    overnight_groups: list[MorningLetterGroupInput]


class MorningLetterSection(StrictFrozenModel):
    """A section in the Morning Letter."""

    key: str = Field(
        min_length=1, pattern=r"^(lead|top3|what_changed|by_genre:[a-z0-9_-]+)$"
    )
    title: str = Field(min_length=1)
    bullets: list[str] = Field(min_length=1)
    genre: str | None = None
    # Optional short prose paragraph written by the LLM. Empty when not
    # produced; the projector's bullets remain renderable without it.
    narrative: str | None = None


class MorningLetterContent(StrictFrozenModel):
    """Generated Morning Letter content."""

    schema_version: int = Field(default=1, ge=1)
    lead: str = Field(min_length=1)
    sections: list[MorningLetterSection] = Field(min_length=1)
    source_recap_window_days: int | None = None


class MorningLetterResponse(StrictFrozenModel):
    """Response for Morning Letter generation."""

    target_date: str
    edition_timezone: str
    content: MorningLetterContent
    metadata: RecapSummaryMetadata


# ============================================================================
# Card Generation Models
# ============================================================================

Card422Reason = Literal["parse_failed", "empty_output", "unknown_ref", "language"]
CardParseDetail = Literal[
    "missing_tag",
    "sources_tag",
    "headline_too_long",
    "sentence_count",
    "missing_citation",
    "unknown_citation_format",
    "why_format",
]


class CardItemInput(StrictFrozenModel):
    """Input item for card generation."""

    n: int = Field(ge=1, description="Sequential reference index (1..6)")
    feed_id: ApiUUID = Field(description="Feed ID")
    title: str = Field(min_length=1, description="Article title")
    host: str = Field(min_length=1, description="Host domain name")
    url: str = Field(min_length=1, description="Article URL")
    pub_date: str | None = Field(
        default=None, description="Publication date RFC3339 or null"
    )
    lede: str = Field(default="", description="Article lead paragraph")


class CardGenerateRequest(StrictFrozenModel):
    """Request model for card generation (POST /v1/cards/generate)."""

    job_id: ApiUUID = Field(description="Job ID")
    candidate_id: ApiUUID = Field(description="Candidate ID")
    prompt_version: str = Field(
        default="recap_card.v1", description="Prompt template version"
    )
    items: list[CardItemInput] = Field(
        min_length=1, max_length=6, description="1..6 input items"
    )
    revision_note: str | None = Field(
        default=None, description="Optional revision note for regeneration"
    )


class CardSentence(StrictFrozenModel):
    """Individual sentence with associated reference citations."""

    text: str = Field(min_length=1, description="Sentence text ending with citations")
    refs: list[int] = Field(description="Citation reference indices (e.g., [1])")


class CardContent(StrictFrozenModel):
    """Structured topic card content."""

    headline_ja: str = Field(
        min_length=1, max_length=60, description="Japanese headline <= 60 chars"
    )
    what_ja: list[CardSentence] = Field(
        min_length=2,
        max_length=3,
        description="2..3 sentences describing what happened",
    )
    why_ja: CardSentence | None = Field(
        default=None,
        description="Exactly 1 sentence describing why it matters, or null",
    )
    used_refs: list[int] = Field(description="Unique reference indices used in card")


class CardGenerationMetadata(StrictFrozenModel):
    """Execution metadata for card generation."""

    model: str
    prompt_version: str
    cache_hit: bool
    prompt_tokens: int
    completion_tokens: int
    ms: int
    raw_text: str


class CardGenerateResponse(StrictFrozenModel):
    """Response model for card generation (200 OK)."""

    card: CardContent
    generation: CardGenerationMetadata
    ja_ratio: float = Field(description="Measured Japanese character ratio (G1 gate)")


class CardGenerate422Response(StrictFrozenModel):
    """Error response model for rejected card generation (422 Unprocessable Entity)."""

    reason: Card422Reason
    attempts: int
    raw_text: str
    detail: CardParseDetail | None = None
    measured_ratio: float | None = None
    character_counts: dict[str, int] | None = None


class CardGenerationRejectedError(Exception):
    """Exception raised when card generation fails all validation/parsing attempts."""

    def __init__(  # noqa: PLR0913 - public exception payload fields constructed directly in existing tests
        self,
        reason: Card422Reason,
        attempts: int,
        raw_text: str,
        detail: CardParseDetail | None = None,
        measured_ratio: float | None = None,
        character_counts: dict[str, int] | None = None,
    ) -> None:
        super().__init__(
            f"Card generation rejected: reason={reason}, attempts={attempts}"
        )
        self.reason = reason
        self.attempts = attempts
        self.raw_text = raw_text
        self.detail = detail
        self.measured_ratio = measured_ratio
        self.character_counts = character_counts

    def to_response_dict(self) -> dict[str, Any]:
        data: dict[str, Any] = {
            "reason": self.reason,
            "attempts": self.attempts,
            "raw_text": self.raw_text,
            "detail": self.detail if self.reason == "parse_failed" else None,
        }
        if self.reason == "language":
            if self.measured_ratio is not None:
                data["measured_ratio"] = self.measured_ratio
            if self.character_counts is not None:
                data["character_counts"] = self.character_counts
        return data


# ============================================================================
# Theme Page Generation Models
# ============================================================================

THEME_MAX_ITEMS = 8
THEME_MAX_ITEM_TITLE_CHARS = 120
THEME_MAX_ITEM_TEXT_CHARS = 400
THEME_MAX_PREVIOUS_TITLE_CHARS = 120
THEME_MAX_PREVIOUS_KEY_POINTS = 7
THEME_MAX_PREVIOUS_OPEN_QUESTIONS = 4
THEME_MAX_PREVIOUS_ENTRY_CHARS = 200
THEME_MAX_FAILURE_NOTES = 5
THEME_MAX_FAILURE_NOTE_CHARS = 120

THEME_MAX_OUTPUT_TITLE_CHARS = 60
THEME_MAX_OUTPUT_TEXT_CHARS = 200
THEME_MAX_KEY_POINTS = 7
THEME_MAX_TIMELINE = 10
THEME_MAX_WHAT_CHANGED = 5
THEME_MAX_OPEN_QUESTIONS = 4

_THEME_DATE_RE = re.compile(r"^[0-9]{4}-[0-9]{2}-[0-9]{2}$")


def _require_non_blank(value: str, field_name: str) -> str:
    if not value.strip():
        msg = f"{field_name} must not be empty or whitespace-only"
        raise ValueError(msg)
    return value


def _validate_iso_date(value: str | None) -> str | None:
    if value is None:
        return None
    if not _THEME_DATE_RE.fullmatch(value):
        msg = "published_at must match YYYY-MM-DD"
        raise ValueError(msg)
    date.fromisoformat(value)
    return value


class ThemeItemInput(StrictFrozenModel):
    """Source item for theme page generation."""

    n: int = Field(ge=1, description="Unique positive item index")
    title: str = Field(
        min_length=1,
        max_length=THEME_MAX_ITEM_TITLE_CHARS,
        description="Item title",
    )
    published_at: str | None = Field(
        default=None,
        description="Publication date in YYYY-MM-DD format or null",
    )
    text: str = Field(
        min_length=1,
        max_length=THEME_MAX_ITEM_TEXT_CHARS,
        description="Item passage text",
    )

    @field_validator("title", mode="after")
    @classmethod
    def _validate_title(cls, v: str) -> str:
        return _require_non_blank(v, "title")

    @field_validator("text", mode="after")
    @classmethod
    def _validate_text(cls, v: str) -> str:
        return _require_non_blank(v, "text")

    @field_validator("published_at", mode="after")
    @classmethod
    def _validate_published_at(cls, v: str | None) -> str | None:
        return _validate_iso_date(v)


class ThemePreviousVersion(StrictFrozenModel):
    """Previous version summary used for incremental update context."""

    title: str = Field(
        min_length=1,
        max_length=THEME_MAX_PREVIOUS_TITLE_CHARS,
        description="Previous version title",
    )
    key_points: list[str] = Field(
        max_length=THEME_MAX_PREVIOUS_KEY_POINTS,
        description="Previous version key points",
    )
    open_questions: list[str] = Field(
        max_length=THEME_MAX_PREVIOUS_OPEN_QUESTIONS,
        description="Previous version open questions",
    )

    @field_validator("title", mode="after")
    @classmethod
    def _validate_title(cls, v: str) -> str:
        return _require_non_blank(v, "previous.title")

    @field_validator("key_points", mode="after")
    @classmethod
    def _validate_key_points(cls, entries: list[str]) -> list[str]:
        for idx, entry in enumerate(entries):
            if not entry.strip() or len(entry) > THEME_MAX_PREVIOUS_ENTRY_CHARS:
                msg = f"previous.key_points[{idx}] must be 1..={THEME_MAX_PREVIOUS_ENTRY_CHARS} non-blank chars"
                raise ValueError(msg)
        return entries

    @field_validator("open_questions", mode="after")
    @classmethod
    def _validate_open_questions(cls, entries: list[str]) -> list[str]:
        for idx, entry in enumerate(entries):
            if not entry.strip() or len(entry) > THEME_MAX_PREVIOUS_ENTRY_CHARS:
                msg = f"previous.open_questions[{idx}] must be 1..={THEME_MAX_PREVIOUS_ENTRY_CHARS} non-blank chars"
                raise ValueError(msg)
        return entries


class ThemeGenerateRequest(StrictFrozenModel):
    """Request payload for POST /v1/themes/generate."""

    job_id: ApiUUID = Field(description="Job UUID")
    theme_id: ApiUUID = Field(description="Theme UUID")
    language: Literal["ja"] = Field(description="Target language (must be 'ja')")
    items: list[ThemeItemInput] = Field(
        min_length=1,
        max_length=THEME_MAX_ITEMS,
        description="1..=8 source items",
    )
    new_item_ns: list[int] = Field(
        max_length=THEME_MAX_ITEMS,
        description="Subset of item indices that are newly added",
    )
    previous: ThemePreviousVersion | None = Field(
        default=None,
        description="Previous theme page version or null",
    )
    failure_notes: list[str] = Field(
        default_factory=list,
        max_length=THEME_MAX_FAILURE_NOTES,
        description="Optional failure notes for regeneration",
    )

    @field_validator("failure_notes", mode="after")
    @classmethod
    def _validate_failure_notes(cls, notes: list[str]) -> list[str]:
        for idx, note in enumerate(notes):
            if not note.strip() or len(note) > THEME_MAX_FAILURE_NOTE_CHARS:
                msg = f"failure_notes[{idx}] must be 1..={THEME_MAX_FAILURE_NOTE_CHARS} non-blank chars"
                raise ValueError(msg)
        return notes

    @model_validator(mode="after")
    def _validate_item_indices(self) -> ThemeGenerateRequest:
        ns = [item.n for item in self.items]
        if len(set(ns)) != len(ns):
            msg = "items[].n values must be unique"
            raise ValueError(msg)
        valid_ns = set(ns)
        if not set(self.new_item_ns).issubset(valid_ns):
            msg = "new_item_ns must be a subset of items[].n"
            raise ValueError(msg)
        return self


class ThemeSentence(StrictFrozenModel):
    """Cited single sentence entry in a theme page section."""

    text: str = Field(
        min_length=1,
        max_length=THEME_MAX_OUTPUT_TEXT_CHARS,
        description="Single sentence <= 200 chars",
    )
    refs: list[int] = Field(
        min_length=1,
        description="Non-empty list of cited item n values",
    )


class ThemeTimelineEntry(StrictFrozenModel):
    """Dated single sentence entry in a theme page timeline."""

    date: str = Field(
        min_length=10,
        max_length=10,
        description="Date in YYYY-MM-DD format",
    )
    text: str = Field(
        min_length=1,
        max_length=THEME_MAX_OUTPUT_TEXT_CHARS,
        description="Single sentence <= 200 chars",
    )
    refs: list[int] = Field(
        min_length=1,
        description="Non-empty list of cited item n values",
    )


class ThemeGenerateResponse(StrictFrozenModel):
    """Response payload for POST /v1/themes/generate (200 OK)."""

    title: str = Field(
        min_length=1,
        max_length=THEME_MAX_OUTPUT_TITLE_CHARS,
        description="Theme page title (1..=60 chars)",
    )
    key_points: list[ThemeSentence] = Field(
        min_length=1,
        max_length=THEME_MAX_KEY_POINTS,
        description="Up to 7 key points",
    )
    timeline: list[ThemeTimelineEntry] = Field(
        max_length=THEME_MAX_TIMELINE,
        description="Up to 10 timeline entries",
    )
    what_changed: list[ThemeSentence] = Field(
        max_length=THEME_MAX_WHAT_CHANGED,
        description="Up to 5 what-changed entries",
    )
    open_questions: list[ThemeSentence] = Field(
        max_length=THEME_MAX_OPEN_QUESTIONS,
        description="Up to 4 open questions",
    )
    model: str = Field(min_length=1, description="Model name used for generation")


class ThemeLLMItem(BaseModel):
    """LLM structured output item for sentence sections."""

    text: str
    refs: list[int]


class ThemeLLMTimelineItem(BaseModel):
    """LLM structured output item for timeline entries."""

    date: str
    text: str
    refs: list[int]


class ThemePageLLMOutput(BaseModel):
    """JSON Schema forwarded to Ollama format= for theme page structured output."""

    title: str
    key_points: list[ThemeLLMItem]
    timeline: list[ThemeLLMTimelineItem]
    what_changed: list[ThemeLLMItem]
    open_questions: list[ThemeLLMItem]
