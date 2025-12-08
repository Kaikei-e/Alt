"""Domain models for recap-subworker."""

from __future__ import annotations

from datetime import datetime
from typing import Any, Literal, Optional

from pydantic import BaseModel, Field, HttpUrl, ConfigDict, field_validator


RunStatusLiteral = Literal["running", "succeeded", "partial", "failed"]


# Classification models
class ClassificationJobPayload(BaseModel):
    """Request body for POST /v1/classify-runs."""

    model_config = ConfigDict(extra="forbid")

    texts: list[str] = Field(..., min_length=1)


class ClassificationResult(BaseModel):
    """Single classification result."""

    top_genre: str
    confidence: float
    scores: dict[str, float]


class ClassificationJobResponse(BaseModel):
    """Response for classification run endpoints."""

    run_id: int
    job_id: str
    status: RunStatusLiteral
    result_count: int = Field(default=0)
    results: Optional[list[ClassificationResult]] = None
    error_message: Optional[str] = None


class ClusterJobParams(BaseModel):
    """Incoming clustering parameters exposed via the public API."""

    model_config = ConfigDict(extra="forbid")

    max_sentences_total: int = Field(..., ge=50, le=10_000)
    max_sentences_per_cluster: int = Field(default=20, ge=1, le=50)
    umap_n_components: int = Field(..., ge=0, le=128)
    hdbscan_min_cluster_size: int = Field(..., ge=3, le=500)
    mmr_lambda: float = Field(..., ge=0.0, le=1.0)


class ClusterDocument(BaseModel):
    """HTTP payload describing an article/document to be clustered."""

    model_config = ConfigDict(str_strip_whitespace=True, extra="forbid")

    article_id: str = Field(..., max_length=128)
    title: Optional[str] = Field(default=None, max_length=512)
    lang_hint: Optional[str] = Field(default=None, max_length=8)
    published_at: Optional[datetime] = Field(default=None)
    source_url: Optional[HttpUrl] = Field(default=None)
    paragraphs: list[str] = Field(..., min_length=1)
    genre_scores: Optional[dict[str, int]] = Field(default=None)
    confidence: Optional[float] = Field(default=None, ge=0.0, le=1.0)
    signals: Optional["ArticleSignals"] = Field(default=None)

    @field_validator("paragraphs")
    @classmethod
    def _validate_paragraphs(cls, value: list[str]) -> list[str]:
        cleaned = [paragraph.strip() for paragraph in value if paragraph and paragraph.strip()]
        if not cleaned or any(len(paragraph) < 30 for paragraph in cleaned):
            raise ValueError("paragraph text must be at least 30 characters")
        return cleaned


class ClusterJobPayload(BaseModel):
    """Request body for POST /v1/runs."""

    params: ClusterJobParams
    documents: list[ClusterDocument] = Field(..., min_length=3)
    metadata: Optional["CorpusMetadata"] = Field(default=None)


class ArticleSignals(BaseModel):
    """Lightweight feature diagnostics from recap-worker."""

    model_config = ConfigDict(extra="forbid")

    tfidf_sum: Optional[float] = Field(default=None)
    bm25_peak: Optional[float] = Field(default=None)
    token_count: Optional[int] = Field(default=None, ge=0)
    keyword_hits: Optional[int] = Field(default=None, ge=0)


class CorpusClassifierStats(BaseModel):
    """Aggregate classifier statistics for confidence-aware tuning."""

    model_config = ConfigDict(extra="forbid")

    avg_confidence: float = Field(default=0.0, ge=0.0, le=1.0)
    max_confidence: float = Field(default=0.0, ge=0.0, le=1.0)
    min_confidence: float = Field(default=0.0, ge=0.0, le=1.0)
    coverage_ratio: float = Field(default=0.0, ge=0.0, le=1.0)


class CorpusMetadata(BaseModel):
    """Corpus-level metadata captured upstream."""

    model_config = ConfigDict(extra="forbid")

    article_count: int = Field(..., ge=0)
    sentence_count: int = Field(..., ge=0)
    primary_language: str = Field(..., max_length=16)
    language_distribution: dict[str, int] = Field(default_factory=dict)
    character_count: int = Field(..., ge=0)
    classifier: Optional[CorpusClassifierStats] = Field(default=None)


class ClusterSentencePayload(BaseModel):
    """Representative sentence returned to recap-worker."""

    article_id: str
    paragraph_idx: Optional[int] = Field(default=None, ge=0)
    sentence_text: str = Field(..., min_length=20)
    lang: Optional[str] = Field(default=None, max_length=8)
    score: float = Field(default=0.0)


class ClusterInfo(BaseModel):
    """Cluster information serialized in API responses."""

    cluster_id: int
    size: int = Field(..., ge=1)
    label: Optional[str] = Field(default=None, max_length=128)
    top_terms: list[str] = Field(default_factory=list)
    stats: dict[str, Any] = Field(default_factory=dict)
    representatives: list[ClusterSentencePayload] = Field(default_factory=list)


class ClusterJobResponse(BaseModel):
    """Response payload for GET/POST run endpoints."""

    run_id: int
    job_id: str
    genre: str
    status: RunStatusLiteral
    cluster_count: int = Field(..., ge=0)
    clusters: list[ClusterInfo] = Field(default_factory=list)
    diagnostics: dict[str, Any] = Field(default_factory=dict)


class EvidenceConstraints(BaseModel):
    """Processing constraints supplied by the caller."""

    model_config = ConfigDict(str_strip_whitespace=True, extra="forbid")

    max_sentences_per_cluster: int = Field(7, ge=1, le=50)
    max_total_sentences: int = Field(120, ge=1, le=20_000)
    max_tokens_budget: int = Field(6000, ge=256, le=200_000)
    dedup_threshold: float = Field(0.92, ge=0.0, le=1.0)
    mmr_lambda: float = Field(0.3, ge=0.0, le=1.0)
    hdbscan_min_cluster_size: int = Field(5, ge=2)
    hdbscan_min_samples: Optional[int] = Field(default=None, ge=1)
    umap_n_components: int = Field(0, ge=0)


class TelemetryEnvelope(BaseModel):
    """Trace & telemetry metadata passed by the caller."""

    request_id: Optional[str] = Field(default=None, max_length=128)
    prompt_version: Optional[str] = Field(default=None, max_length=64)


class EvidenceRequest(BaseModel):
    """HTTP request payload for the evidence generation endpoint."""

    model_config = ConfigDict(str_strip_whitespace=True)

    job_id: str = Field(..., max_length=64)
    genre: str = Field(..., max_length=32)
    documents: list[ClusterDocument] = Field(..., min_length=1)
    constraints: EvidenceConstraints = Field(default_factory=EvidenceConstraints)
    telemetry: Optional[TelemetryEnvelope] = Field(default=None)
    metadata: Optional["CorpusMetadata"] = Field(default=None)

    def total_paragraphs(self) -> int:
        return sum(len(document.paragraphs) for document in self.documents)


class RepresentativeSource(BaseModel):
    """Source metadata for a representative sentence."""

    source_id: str = Field(...)
    url: Optional[HttpUrl] = Field(default=None)
    paragraph_idx: Optional[int] = Field(default=None, ge=0)


class RepresentativeSentence(BaseModel):
    """Representative sentence selected for the cluster."""

    text: str
    lang: Optional[str] = Field(default=None, max_length=8)
    embedding_ref: Optional[str] = Field(default=None, max_length=64)
    reasons: list[str] = Field(default_factory=list)
    source: RepresentativeSource


class ClusterLabel(BaseModel):
    """Topic label for a cluster."""

    top_terms: list[str]
    method: str = Field(default="ctfidf")


class ClusterStats(BaseModel):
    """Aggregate statistics for a cluster."""

    avg_sim: Optional[float] = Field(default=None)
    token_count: Optional[int] = Field(default=None, ge=0)


class EvidenceCluster(BaseModel):
    """Cluster level result."""

    cluster_id: int
    size: int
    label: ClusterLabel
    representatives: list[RepresentativeSentence]
    supporting_ids: list[str]
    stats: ClusterStats = Field(default_factory=ClusterStats)


class EvidenceBudget(BaseModel):
    """Budget consumption metadata."""

    sentences: int
    tokens_estimated: int


class HDBSCANSettings(BaseModel):
    """Settings used by the clustering stage."""

    min_cluster_size: int
    min_samples: int


class Diagnostics(BaseModel):
    """Diagnostics payload for downstream monitoring."""

    dedup_pairs: int = 0
    umap_used: bool = False
    hdbscan: Optional[HDBSCANSettings] = None
    partial: bool = False
    total_sentences: int = 0
    embedding_ms: Optional[float] = None
    hdbscan_ms: Optional[float] = None
    noise_ratio: Optional[float] = None
    dbcv_score: Optional[float] = None
    silhouette_score: Optional[float] = None


class EvidenceResponse(BaseModel):
    """HTTP response payload containing cluster evidence."""

    job_id: str
    genre: str
    clusters: list[EvidenceCluster]
    genre_highlights: Optional[list[RepresentativeSentence]] = None
    evidence_budget: EvidenceBudget
    diagnostics: Diagnostics = Field(default_factory=Diagnostics)


class WarmupResponse(BaseModel):
    """Response for warmup endpoint."""

    warmed: bool
    batches: int
    backend: str


class HealthResponse(BaseModel):
    """Response schema for /health endpoint."""

    status: Literal["ok"]
    model_id: str
    backend: str


ClusterDocument.model_rebuild()
ClusterJobPayload.model_rebuild()
EvidenceRequest.model_rebuild()


def build_response_template(request: EvidenceRequest) -> EvidenceResponse:
    """Return an empty response using request metadata."""

    return EvidenceResponse(
        job_id=request.job_id,
        genre=request.genre,
        clusters=[],
        evidence_budget=EvidenceBudget(sentences=0, tokens_estimated=0),
    )
