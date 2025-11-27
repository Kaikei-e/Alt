"""Summarize usecase - business logic for article summarization."""

import logging
from typing import Tuple, Dict, Any, List, Optional

from news_creator.config.config import NewsCreatorConfig
from news_creator.domain.prompts import SUMMARY_PROMPT_TEMPLATE
from news_creator.port.llm_provider_port import LLMProviderPort

logger = logging.getLogger(__name__)


class SummarizeUsecase:
    """Usecase for generating Japanese article summaries."""

    def __init__(self, config: NewsCreatorConfig, llm_provider: LLMProviderPort):
        """Initialize summarize usecase."""
        self.config = config
        self.llm_provider = llm_provider

    async def generate_summary(self, article_id: str, content: str) -> Tuple[str, Dict[str, Any]]:
        """
        Generate a Japanese summary for an article.

        Args:
            article_id: Article identifier
            content: Article content to summarize

        Returns:
            Tuple of (summary text, metadata dict)

        Raises:
            ValueError: If article_id or content is empty
            RuntimeError: If LLM generation fails
        """
        if not article_id or not article_id.strip():
            raise ValueError("article_id cannot be empty")
        if not content or not content.strip():
            raise ValueError("content cannot be empty")

        # Truncate content to fit within 64K context window
        # 64K tokens ≈ 256K-512K chars, but we need to reserve space for prompt template
        # Using ~200K chars (≈50K tokens) for content to leave room for prompt template
        MAX_CONTENT_LENGTH = 200_000  # characters
        original_length = len(content)
        truncated_content = content.strip()[:MAX_CONTENT_LENGTH]

        if original_length > MAX_CONTENT_LENGTH:
            logger.warning(
                "Input content truncated to fit context window",
                extra={
                    "article_id": article_id,
                    "original_length": original_length,
                    "truncated_length": len(truncated_content),
                    "max_length": MAX_CONTENT_LENGTH,
                }
            )

        logger.info(
            "Generating summary",
            extra={
                "article_id": article_id,
                "content_length": len(truncated_content),
                "was_truncated": original_length > MAX_CONTENT_LENGTH,
            }
        )

        # Build prompt from template
        prompt = SUMMARY_PROMPT_TEMPLATE.format(content=truncated_content)

        # Call LLM provider
        llm_response = await self.llm_provider.generate(
            prompt,
            num_predict=self.config.summary_num_predict,
        )

        # Clean and validate summary
        raw_summary = llm_response.response
        cleaned_summary = self._clean_summary_text(raw_summary)

        if not cleaned_summary:
            raise RuntimeError("LLM returned an empty summary")

        # Enforce 500 character max as per prompt guidance
        truncated_summary = cleaned_summary[:600]

        # Build metadata
        metadata = {
            "model": llm_response.model,
            "prompt_tokens": llm_response.prompt_eval_count,
            "completion_tokens": llm_response.eval_count,
            "total_duration_ms": self._nanoseconds_to_milliseconds(llm_response.total_duration),
        }

        logger.info(
            "Summary generated successfully",
            extra={
                "article_id": article_id,
                "summary_length": len(truncated_summary),
                "model": metadata["model"],
            }
        )

        return truncated_summary, metadata

    @staticmethod
    def _clean_summary_text(content: str) -> str:
        """
        Clean LLM output to extract clean summary text.

        Args:
            content: Raw LLM output

        Returns:
            Cleaned summary text
        """
        if not content:
            return ""

        # Remove special tokens
        cleaned = (
            content.replace("<|system|>", "")
            .replace("<|user|>", "")
            .replace("<|assistant|>", "")
        )

        # Remove markdown code blocks (```...```)
        import re
        # Remove code blocks with triple backticks
        cleaned = re.sub(r'```[^`]*```', '', cleaned, flags=re.DOTALL)
        # Remove standalone triple backticks
        cleaned = re.sub(r'```+', '', cleaned)
        # Remove any remaining backticks
        cleaned = cleaned.replace('`', '')

        # Remove excessive whitespace and special characters
        # Replace multiple spaces/tabs with single space
        cleaned = re.sub(r'[ \t]+', ' ', cleaned)
        # Remove excessive newlines
        cleaned = re.sub(r'\n{3,}', '\n\n', cleaned)

        # Process line by line
        lines = cleaned.splitlines()
        final_lines: List[str] = []

        for line in lines:
            stripped = line.strip()
            if not stripped:
                continue
            # Skip separator lines and bold markdown
            if stripped.startswith("---") or stripped.startswith("**"):
                continue
            # Remove "Summary:" prefix if present
            if stripped.lower().startswith("summary:") or "要約:" in stripped:
                stripped = stripped.replace("Summary:", "").replace("要約:", "").strip()
                if not stripped:
                    continue
            final_lines.append(stripped)

        result = " ".join(final_lines).strip()

        # Final cleanup: remove any remaining repetitive patterns
        # Check for patterns like "word-word-word" (3+ repetitions)
        result = re.sub(r'\b(\w+)(-\1){2,}\b', '', result, flags=re.IGNORECASE)

        return result

    @staticmethod
    def _nanoseconds_to_milliseconds(value: Optional[int]) -> Optional[float]:
        """Convert nanoseconds to milliseconds."""
        if value is None:
            return None
        try:
            return value / 1_000_000
        except TypeError:
            return None
