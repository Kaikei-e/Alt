"""Domain models for recap-subworker."""

from __future__ import annotations

from datetime import datetime
from typing import Iterable, Literal, Optional

from pydantic import BaseModel, Field, HttpUrl, ConfigDict


class ArticlePayload(BaseModel):
    """Incoming article with paragraphs and metadata."""

    model_config = ConfigDict(str_strip_whitespace=True)

    source_id: str = Field(..., max_length=128)
    url: Optional[HttpUrl] = Field(default=None)
    lang_hint: Optional[str] = Field(default=None, max_length=8)
    title: Optional[str] = Field(default=None, max_length=512)
    published_at: Optional[datetime] = Field(default=None)
    paragraphs: list[str] = Field(default_factory=list, min_length=1)


class EvidenceConstraints(BaseModel):
    """Processing constraints supplied by the caller."""

    model_config = ConfigDict(str_strip_whitespace=True)

    max_sentences_per_cluster: int = Field(7, ge=1, le=50)
    max_total_sentences: int = Field(120, ge=1, le=20_000)
    max_tokens_budget: int = Field(6000, ge=256, le=200_000)
    dedup_threshold: float = Field(0.92, ge=0.0, le=1.0)
    mmr_lambda: float = Field(0.3, ge=0.0, le=1.0)
    heading_terms: int = Field(5, ge=1, le=20)
    umap: dict = Field(default_factory=lambda: {"enabled": False})


class TelemetryEnvelope(BaseModel):
    """Trace & telemetry metadata passed by the caller."""

    request_id: Optional[str] = Field(default=None, max_length=128)
    prompt_version: Optional[str] = Field(default=None, max_length=64)


class EvidenceRequest(BaseModel):
    """HTTP request payload for the evidence generation endpoint."""

    model_config = ConfigDict(str_strip_whitespace=True)

    job_id: str = Field(..., max_length=64)
    genre: str = Field(..., max_length=32)
    articles: list[ArticlePayload] = Field(..., min_length=1)
    constraints: EvidenceConstraints = Field(default_factory=EvidenceConstraints)
    telemetry: Optional[TelemetryEnvelope] = Field(default=None)

    def total_paragraphs(self) -> int:
        return sum(len(article.paragraphs) for article in self.articles)


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


class EvidenceResponse(BaseModel):
    """HTTP response payload containing cluster evidence."""

    job_id: str
    genre: str
    clusters: list[EvidenceCluster]
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


def build_response_template(request: EvidenceRequest) -> EvidenceResponse:
    """Return an empty response using request metadata."""

    return EvidenceResponse(
        job_id=request.job_id,
        genre=request.genre,
        clusters=[],
        evidence_budget=EvidenceBudget(sentences=0, tokens_estimated=0),
    )
