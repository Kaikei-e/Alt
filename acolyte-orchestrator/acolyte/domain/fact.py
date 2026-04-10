"""ExtractedFact — atomic factual claim extracted from evidence."""

from __future__ import annotations

from dataclasses import dataclass

from pydantic import BaseModel


@dataclass(frozen=True)
class ExtractedFact:
    """An atomic factual claim extracted from an evidence source.

    is_fallback=True when LLM normalization failed and the raw quote
    was preserved as the claim (confidence=0.3, data_type="quote").
    """

    claim: str
    source_id: str
    source_title: str
    verbatim_quote: str
    confidence: float
    data_type: str  # statistic | date | quote | trend | comparison
    is_fallback: bool = False


class ExtractedFactModel(BaseModel):
    """Pydantic model for LLM structured output validation."""

    claim: str
    source_id: str
    source_title: str = ""
    verbatim_quote: str = ""
    confidence: float = 0.5
    data_type: str = "quote"


class ExtractorOutput(BaseModel):
    """Pydantic model for full extractor LLM response.

    The 'reasoning' field absorbs Gemma4 thinking tokens into the JSON structure.
    Without it, thinking flows to the separate 'thinking' response field,
    consuming num_predict budget without contributing to output (ADR-632).
    """

    reasoning: str = ""
    facts: list[ExtractedFactModel]
