"""Pydantic models for validating event payloads.

Provides size limits and type safety for incoming Redis Stream events.
"""

from pydantic import BaseModel, Field, field_validator


class TagGenerationRequestPayload(BaseModel):
    """Validated payload for TagGenerationRequested events.

    Enforces max lengths to guard against oversized payloads.
    """

    article_id: str = Field(max_length=36)
    title: str = Field(max_length=2000)
    content: str = Field(max_length=100_000)
    feed_id: str = Field(default="", max_length=36)

    @field_validator("article_id")
    @classmethod
    def article_id_not_empty(cls, v: str) -> str:
        if not v.strip():
            msg = "article_id must not be empty"
            raise ValueError(msg)
        return v
