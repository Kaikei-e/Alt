"""GeneratedParagraph — per-claim paragraph for micro-generation Writer.

Each claim in a claim_plan produces exactly one paragraph via a single LLM call.
Accepted paragraphs are immutable during revision; only rejected ones are regenerated.
"""

from __future__ import annotations

from typing import Literal

from pydantic import BaseModel, Field


class GeneratedParagraph(BaseModel):
    """Single paragraph generated from one claim."""

    claim_id: str
    claim_text: str
    body: str = ""
    status: Literal["pending", "accepted", "rejected"] = "pending"
    citations: list[dict] = Field(default_factory=list)
    revision_feedback: str = ""
