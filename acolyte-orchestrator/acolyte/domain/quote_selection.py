"""Quote selection domain models for 2-stage extraction pipeline.

Stage 1: QuoteSelector selects verbatim quotes from articles.
Stage 2: FactNormalizer normalizes each quote into an atomic fact.
"""

from __future__ import annotations

from typing import Literal

from pydantic import BaseModel


class SelectedQuote(BaseModel):
    """A verbatim quote selected from an article for a specific section."""

    text: str
    source_id: str
    source_title: str = ""
    section_key: str = ""
    start_offset: int = -1  # position in article body, -1 if not found
    end_offset: int = -1


class QuoteSelectorOutput(BaseModel):
    """LLM output for quote selection from a single article.

    reasoning-first per ADR-632. reasoning is debug-only.
    """

    reasoning: str = ""
    quotes: list[SelectedQuote]


class FactNormalizerOutput(BaseModel):
    """LLM output for normalizing a single quote into a fact.

    Tiny schema per exec3.md Issue 2.
    reasoning kept per ADR-632 (A/B gated — removal requires validation).
    data_type constrained to Literal enum for stronger schema enforcement.
    """

    reasoning: str = ""
    claim: str
    confidence: float = 0.5
    data_type: Literal["statistic", "date", "quote", "trend", "comparison"] = "quote"
