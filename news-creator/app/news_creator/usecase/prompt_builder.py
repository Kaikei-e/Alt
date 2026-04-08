"""Prompt Builders for LLM requests (Phase 5 refactoring).

This module extracts prompt building logic from usecases
following SOLID principles (Single Responsibility Principle).

Following Python 3.14 best practices:
- Protocol for structural typing
- Separation of templates from formatting logic
"""

from __future__ import annotations

import logging
from datetime import datetime
from typing import Protocol

from news_creator.domain.prompts import (
    SUMMARY_PROMPT_TEMPLATE,
    CHUNK_SUMMARY_PROMPT_TEMPLATE,
    RECAP_CLUSTER_SUMMARY_PROMPT,
)

logger = logging.getLogger(__name__)


class PromptBuilderProtocol(Protocol):
    """Protocol for prompt builders."""

    def build(self, **kwargs) -> str:
        """Build a prompt string."""
        ...


class SummaryPromptBuilder:
    """Builds prompts for article summarization.

    Responsibilities:
    - Format SUMMARY_PROMPT_TEMPLATE with content and date
    - Handle default date formatting

    This class extracts prompt building from SummarizeUsecase.
    """

    def build(
        self,
        content: str,
        current_date: str | None = None,
    ) -> str:
        """Build a summary prompt.

        Args:
            content: Article content to summarize
            current_date: Optional date string (defaults to today)

        Returns:
            Formatted prompt string
        """
        if current_date is None:
            current_date = datetime.now().strftime("%Y年%m月%d日")

        return SUMMARY_PROMPT_TEMPLATE.format(
            current_date=current_date,
            content=content,
        )


class ChunkPromptBuilder:
    """Builds prompts for chunk summarization in hierarchical processing.

    Responsibilities:
    - Format CHUNK_SUMMARY_PROMPT_TEMPLATE with chunk content

    This class extracts prompt building from SummarizeUsecase._generate_hierarchical_summary().
    """

    def build(self, content: str) -> str:
        """Build a chunk summary prompt.

        Args:
            content: Chunk content to extract facts from

        Returns:
            Formatted prompt string
        """
        return CHUNK_SUMMARY_PROMPT_TEMPLATE.format(content=content)


class RecapPromptBuilder:
    """Builds prompts for recap summary generation.

    Responsibilities:
    - Format RECAP_CLUSTER_SUMMARY_PROMPT with job details and clusters

    This class extracts prompt building from RecapSummaryUsecase.
    """

    def build(
        self,
        job_id: str,
        genre: str,
        cluster_section: str,
        max_bullets: int = 7,
    ) -> str:
        """Build a recap summary prompt.

        Args:
            job_id: Job identifier
            genre: News genre/category
            cluster_section: Formatted cluster content
            max_bullets: Maximum number of bullets (default 7)

        Returns:
            Formatted prompt string
        """
        return RECAP_CLUSTER_SUMMARY_PROMPT.format(
            job_id=job_id,
            genre=genre,
            cluster_section=cluster_section,
            max_bullets=max_bullets,
        )


class PromptBuilderFactory:
    """Factory for creating prompt builders.

    Provides a centralized way to create prompt builders
    based on the type of summarization task.
    """

    @staticmethod
    def summary() -> SummaryPromptBuilder:
        """Create a summary prompt builder."""
        return SummaryPromptBuilder()

    @staticmethod
    def chunk() -> ChunkPromptBuilder:
        """Create a chunk prompt builder."""
        return ChunkPromptBuilder()

    @staticmethod
    def recap() -> RecapPromptBuilder:
        """Create a recap prompt builder."""
        return RecapPromptBuilder()
