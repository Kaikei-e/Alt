"""ExtractedFact — atomic factual claim extracted from evidence."""

from __future__ import annotations

from dataclasses import dataclass

from pydantic import BaseModel


@dataclass(frozen=True)
class ExtractedFact:
    """An atomic factual claim extracted from an evidence source."""

    claim: str
    source_id: str
    source_title: str
    verbatim_quote: str
    confidence: float
    data_type: str  # statistic | date | quote | trend | comparison


class ExtractedFactModel(BaseModel):
    """Pydantic model for LLM structured output validation."""

    claim: str
    source_id: str
    source_title: str = ""
    verbatim_quote: str = ""
    confidence: float = 0.5
    data_type: str = "quote"


class ExtractorOutput(BaseModel):
    """Pydantic model for full extractor LLM response."""

    facts: list[ExtractedFactModel]
