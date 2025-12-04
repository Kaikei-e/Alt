"""Summarize usecase - business logic for article summarization."""

import logging
from typing import Tuple, Dict, Any, List, Optional

from news_creator.config.config import NewsCreatorConfig
from news_creator.domain.prompts import SUMMARY_PROMPT_TEMPLATE
from news_creator.port.llm_provider_port import LLMProviderPort
from news_creator.utils.repetition_detector import detect_repetition
from news_creator.utils.html_cleaner import clean_html_content

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

        # Clean HTML from content if present
        original_content_length = len(content)
        cleaned_content, was_html = clean_html_content(content, article_id)
        if was_html:
            logger.warning(
                "HTML detected and removed from article content",
                extra={
                    "article_id": article_id,
                    "original_length": original_content_length,
                    "cleaned_length": len(cleaned_content),
                }
            )
            content = cleaned_content

        # Validate that we have meaningful content after cleaning
        if not content or not content.strip() or len(content.strip()) < 100:
            error_msg = (
                f"Content is empty or too short after HTML cleaning. "
                f"Original length: {original_content_length}, "
                f"Cleaned length: {len(content)}"
            )
            logger.error(
                error_msg,
                extra={
                    "article_id": article_id,
                    "was_html": was_html,
                    "original_length": original_content_length,
                    "cleaned_length": len(content),
                }
            )
            raise ValueError(error_msg)

        # Truncate content to fit within context window
        # Context window is now 80K tokens (81920), configured in entrypoint.sh and config.py
        # We need to account for prompt template (~500 chars ≈ ~200 tokens)
        # Conservative estimate: 1 char ≈ 0.25-0.5 tokens (Japanese text)
        # Reserve ~5K tokens for prompt template and safety margin, leaving ~75K tokens for content
        # Using ~280K chars (≈70K tokens) for content to avoid truncation and stay within 80K limit
        MAX_CONTENT_LENGTH = 280_000  # characters (conservative estimate for ~70K tokens in 80K context)
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

        # Retry loop with repetition detection
        max_retries = self.config.max_repetition_retries
        last_error = None
        last_metadata = None
        has_repetition = False
        rep_score = 0.0
        rep_patterns: List[str] = []
        attempt = 0
        raw_summary = ""
        llm_response = None

        for attempt in range(max_retries + 1):
            # Adjust temperature and repetition penalty for retries
            current_temp = self.config.summary_temperature
            current_repeat_penalty = self.config.llm_repeat_penalty

            if attempt > 0:
                # Progressively lower temperature and increase repetition penalty
                current_temp = max(0.05, current_temp - (0.05 * attempt))
                current_repeat_penalty = min(1.2, current_repeat_penalty + (0.05 * attempt))

                logger.warning(
                    "Retrying summary generation due to repetition",
                    extra={
                        "article_id": article_id,
                        "attempt": attempt + 1,
                        "max_retries": max_retries + 1,
                        "temperature": current_temp,
                        "repeat_penalty": current_repeat_penalty,
                    }
                )

            # Call LLM provider with adjusted parameters
            llm_options = {
                "temperature": current_temp,
                "repeat_penalty": current_repeat_penalty,
            }

            llm_response = await self.llm_provider.generate(
                prompt,
                num_predict=self.config.summary_num_predict,
                options=llm_options,
            )

            # Clean and validate summary
            raw_summary = llm_response.response

            # Check for repetition
            has_repetition, rep_score, rep_patterns = detect_repetition(
                raw_summary,
                threshold=self.config.repetition_threshold
            )

            if has_repetition and attempt < max_retries:
                logger.warning(
                    "Repetition detected in generated summary, will retry",
                    extra={
                        "article_id": article_id,
                        "attempt": attempt + 1,
                        "repetition_score": rep_score,
                        "patterns": rep_patterns,
                        "raw_summary_preview": raw_summary[:200],
                    }
                )
                last_error = f"Repetition detected (score: {rep_score:.2f})"
                last_metadata = {
                    "model": llm_response.model,
                    "prompt_tokens": llm_response.prompt_eval_count,
                    "completion_tokens": llm_response.eval_count,
                    "total_duration_ms": self._nanoseconds_to_milliseconds(llm_response.total_duration),
                }
                continue  # Retry

            # No repetition detected or max retries reached, proceed with cleaning
            break

        # Log raw summary for debugging
        if attempt > 0:
            logger.info(
                "Summary generation succeeded after retry",
                extra={
                    "article_id": article_id,
                    "attempts": attempt + 1,
                    "raw_summary_length": len(raw_summary) if raw_summary else 0,
                }
            )

        logger.debug(
            "Raw summary received from LLM",
            extra={
                "article_id": article_id,
                "raw_summary_length": len(raw_summary) if raw_summary else 0,
                "raw_summary_preview": raw_summary[:200] if raw_summary else "",
            }
        )

        # If we exhausted retries and still have repetition, log warning but proceed
        if has_repetition and attempt >= max_retries and llm_response:
            logger.error(
                "Repetition still detected after all retries, using summary anyway",
                extra={
                    "article_id": article_id,
                    "repetition_score": rep_score,
                    "patterns": rep_patterns,
                    "max_retries": max_retries,
                }
            )

        cleaned_summary = self._clean_summary_text(raw_summary, article_id)

        # Log cleaned summary for debugging
        logger.debug(
            "Cleaned summary after processing",
            extra={
                "article_id": article_id,
                "cleaned_summary_length": len(cleaned_summary) if cleaned_summary else 0,
                "cleaned_summary_preview": cleaned_summary[:200] if cleaned_summary else "",
            }
        )

        if not cleaned_summary:
            # Fallback: try to extract from raw_summary with minimal cleaning
            logger.warning(
                "Cleaned summary is empty, attempting fallback extraction",
                extra={
                    "article_id": article_id,
                    "raw_summary_length": len(raw_summary) if raw_summary else 0,
                    "raw_summary": raw_summary[:500] if raw_summary else "",
                }
            )

            # Minimal fallback cleaning: just remove turn tokens and trim
            fallback_summary = raw_summary
            if fallback_summary:
                fallback_summary = (
                    fallback_summary.replace("<start_of_turn>", "")
                    .replace("<end_of_turn>", "")
                    .replace("<|system|>", "")
                    .replace("<|user|>", "")
                    .replace("<|assistant|>", "")
                    .strip()
                )

            if not fallback_summary or not fallback_summary.strip():
                error_msg = (
                    f"LLM returned an empty summary after cleaning. "
                    f"Raw summary length: {len(raw_summary) if raw_summary else 0}, "
                    f"Raw preview: {raw_summary[:300] if raw_summary else 'None'}"
                )
                logger.error(
                    error_msg,
                    extra={
                        "article_id": article_id,
                        "raw_summary": raw_summary[:500] if raw_summary else "",
                    }
                )
                raise RuntimeError(error_msg)

            cleaned_summary = fallback_summary
            logger.info(
                "Fallback extraction succeeded",
                extra={
                    "article_id": article_id,
                    "fallback_summary_length": len(cleaned_summary),
                }
            )

        # Enforce 500 character max as per prompt guidance
        truncated_summary = cleaned_summary[:600]

        # Build metadata
        if llm_response:
            metadata = {
                "model": llm_response.model,
                "prompt_tokens": llm_response.prompt_eval_count,
                "completion_tokens": llm_response.eval_count,
                "total_duration_ms": self._nanoseconds_to_milliseconds(llm_response.total_duration),
            }
        elif last_metadata:
            metadata = last_metadata
        else:
            # Fallback metadata if something went wrong
            metadata = {
                "model": "unknown",
                "prompt_tokens": None,
                "completion_tokens": None,
                "total_duration_ms": None,
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
    def _clean_summary_text(content: str, article_id: str = "") -> str:
        """
        Clean LLM output to extract clean summary text.

        Args:
            content: Raw LLM output
            article_id: Article ID for logging (optional)

        Returns:
            Cleaned summary text
        """
        if not content:
            if article_id:
                logger.warning(
                    "Empty content provided to _clean_summary_text",
                    extra={"article_id": article_id}
                )
            return ""

        original_length = len(content)

        # Remove Gemma3 turn tokens first (most important)
        cleaned = (
            content.replace("<start_of_turn>", "")
            .replace("<end_of_turn>", "")
            .replace("<|system|>", "")
            .replace("<|user|>", "")
            .replace("<|assistant|>", "")
        )

        # Log removal of turn tokens
        if len(cleaned) != original_length:
            logger.debug(
                "Removed turn tokens from summary",
                extra={
                    "article_id": article_id,
                    "original_length": original_length,
                    "after_token_removal": len(cleaned),
                }
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
        before_final = result
        result = re.sub(r'\b(\w+)(-\1){2,}\b', '', result, flags=re.IGNORECASE)

        # Log if final cleanup removed significant content
        if len(result) < len(before_final) * 0.9:  # More than 10% removed
            logger.debug(
                "Final cleanup removed significant content",
                extra={
                    "article_id": article_id,
                    "before_final_length": len(before_final),
                    "after_final_length": len(result),
                }
            )

        # Warn if result is empty after all cleaning
        if not result and original_length > 0:
            logger.warning(
                "Summary became empty after cleaning",
                extra={
                    "article_id": article_id,
                    "original_length": original_length,
                    "original_preview": content[:200],
                }
            )

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
