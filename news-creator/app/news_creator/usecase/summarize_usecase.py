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
        min_content_length = 100
        if not content or not content.strip() or len(content.strip()) < min_content_length:
            error_msg = (
                f"Content is empty or too short after HTML cleaning. "
                f"Original length: {original_content_length}, "
                f"Cleaned length: {len(content)}, "
                f"Minimum required: {min_content_length} characters. "
                f"This article may not have enough content to generate a meaningful summary."
            )
            # Short content is a normal business case, not an error
            # Log as warning to reduce noise in error logs
            logger.warning(
                "Article content too short for summarization",
                extra={
                    "article_id": article_id,
                    "was_html": was_html,
                    "original_length": original_content_length,
                    "cleaned_length": len(content),
                    "min_required": min_content_length,
                    "content_preview": content[:100] if content else "",
                }
            )
            raise ValueError(error_msg)

        # Truncate content to fit within context window
        # Context window is now 71K tokens (71000), configured in entrypoint.sh and config.py
        # We need to account for prompt template (~500 chars ≈ ~200 tokens)
        # Conservative estimate: 1 char ≈ 0.25-0.5 tokens (Japanese text)
        # Reserve ~1K tokens for prompt template and safety margin, leaving ~70K tokens for content
        # Using ~280K chars (≈70K tokens) for content to avoid truncation and stay within 71K limit
        MAX_CONTENT_LENGTH = 280_000  # characters (conservative estimate for ~70K tokens in 71K context)
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
            f"Generating summary for article: {article_id}",
            extra={
                "article_id": article_id,
                "content_length": len(truncated_content),
                "was_truncated": original_length > MAX_CONTENT_LENGTH,
            }
        )

        # Build prompt from template
        prompt = SUMMARY_PROMPT_TEMPLATE.format(content=truncated_content)

        # Estimate prompt tokens (rough estimate: 1 token ≈ 4 characters for Japanese/English mixed)
        estimated_prompt_tokens = len(prompt) // 4
        context_window = self.config.llm_num_ctx

        if estimated_prompt_tokens > context_window * 0.9:  # Warn if using >90% of context window
            logger.warning(
                "Prompt may be close to context window limit",
                extra={
                    "article_id": article_id,
                    "estimated_prompt_tokens": estimated_prompt_tokens,
                    "context_window": context_window,
                    "usage_percent": round((estimated_prompt_tokens / context_window) * 100, 1),
                }
            )

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

        # Check if output is truncated and attempt continuation generation
        # Only generate continuation if:
        # 1. Summary is less than 1000 characters, OR
        # 2. Clearly truncated (mid-sentence)
        # Skip continuation if summary is close to 1500 characters (1200+) to prioritize quality
        is_truncated, truncation_reason = self._detect_truncation(cleaned_summary)
        summary_length = len(cleaned_summary) if cleaned_summary else 0
        should_generate_continuation = (
            is_truncated
            and cleaned_summary
            and summary_length < 1200  # Skip if already close to target (quality priority)
            and (summary_length < 1000 or "does not end with a proper sentence ending" in truncation_reason or "incomplete pattern" in truncation_reason)
        )

        if should_generate_continuation:
            logger.warning(
                "Output appears to be truncated, attempting continuation generation",
                extra={
                    "article_id": article_id,
                    "summary_length": summary_length,
                    "reason": truncation_reason,
                }
            )
            # Attempt to generate continuation (only once to prevent infinite loops)
            continuation = await self._generate_continuation(
                article_id, truncated_content, cleaned_summary, prompt
            )
            if continuation:
                cleaned_summary = cleaned_summary + continuation
                logger.info(
                    "Continuation generated successfully",
                    extra={
                        "article_id": article_id,
                        "original_length": summary_length,
                        "continuation_length": len(continuation),
                        "final_length": len(cleaned_summary),
                    }
                )
            else:
                logger.warning(
                    "Failed to generate continuation, using truncated output",
                    extra={"article_id": article_id}
                )
        elif is_truncated and summary_length >= 1200:
            logger.info(
                "Skipping continuation generation - summary is close to target length",
                extra={
                    "article_id": article_id,
                    "summary_length": summary_length,
                    "reason": truncation_reason,
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
                # Check if prompt might have been truncated
                estimated_prompt_tokens = len(prompt) // 4
                context_window = self.config.llm_num_ctx
                prompt_truncated = estimated_prompt_tokens > context_window

                error_msg = (
                    f"LLM returned an empty summary after cleaning. "
                    f"Raw summary length: {len(raw_summary) if raw_summary else 0}, "
                    f"Raw preview: {raw_summary[:300] if raw_summary else 'None'}"
                )
                if prompt_truncated:
                    error_msg += (
                        f" WARNING: Prompt may have been truncated. "
                        f"Estimated tokens: {estimated_prompt_tokens}, "
                        f"Context window: {context_window}. "
                        f"Consider increasing LLM_NUM_CTX or reducing content length."
                    )

                logger.error(
                    error_msg,
                    extra={
                        "article_id": article_id,
                        "raw_summary": raw_summary[:500] if raw_summary else "",
                        "estimated_prompt_tokens": estimated_prompt_tokens,
                        "context_window": context_window,
                        "prompt_truncated": prompt_truncated,
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

        # Enforce 2000 character max (quality priority - allow slight exceed of 1500 target)
        truncated_summary = cleaned_summary[:2000]

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
            f"Summary generated successfully for article: {article_id}",
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
    def _detect_truncation(summary: str) -> Tuple[bool, str]:
        """
        Detect if the summary output is truncated.

        Args:
            summary: The cleaned summary text

        Returns:
            Tuple of (is_truncated: bool, reason: str)
        """
        if not summary or len(summary.strip()) == 0:
            return False, ""

        # Check 1: Minimum length check (less than 1000 characters suggests truncation)
        min_expected_length = 1000
        if len(summary) < min_expected_length:
            return True, f"Summary length ({len(summary)}) is below minimum expected ({min_expected_length})"

        # Check 2: Sentence completeness check
        # Japanese sentence endings: 。、！、？
        import re
        # Remove trailing whitespace
        trimmed = summary.rstrip()
        if not trimmed:
            return False, ""

        # Check if ends with proper sentence ending
        sentence_endings = ['。', '！', '？', '.', '!', '?']
        last_char = trimmed[-1] if trimmed else ''

        # If doesn't end with sentence ending, might be truncated
        if last_char not in sentence_endings:
            # Check if it's a valid ending (like quote mark or parenthesis)
            valid_endings = sentence_endings + ['」', '）', ')', ']', '】']
            if last_char not in valid_endings:
                return True, f"Summary does not end with a proper sentence ending (ends with: '{last_char}')"

        # Check 3: Check if last sentence appears incomplete
        # Look for incomplete patterns at the end
        incomplete_patterns = [
            r'[、，]$',  # Ends with comma
            r'[（(]$',  # Ends with opening parenthesis
            r'[「\[]$',  # Ends with opening quote/bracket
        ]
        for pattern in incomplete_patterns:
            if re.search(pattern, trimmed):
                return True, f"Summary ends with incomplete pattern: {pattern}"

        return False, ""

    async def _generate_continuation(
        self,
        article_id: str,
        content: str,
        existing_summary: str,
        original_prompt: str
    ) -> Optional[str]:
        """
        Generate continuation for a truncated summary.

        Args:
            article_id: Article identifier
            content: Original article content
            existing_summary: The truncated summary that needs continuation
            original_prompt: The original prompt used

        Returns:
            Continuation text, or None if generation fails
        """
        continuation_prompt = f"""<start_of_turn>user
You are continuing a Japanese news summary that was cut off mid-sentence.

The original article summary you started writing is:
---
{existing_summary}
---

TASK:
- Continue from where the summary was cut off
- Complete the current sentence if it's incomplete
- Add the remaining paragraphs to reach 1000-1500 characters total
- Maintain the same style: 常体（〜だ／である）、見出しなし、箇条書き禁止、本文のみ
- Include specific facts, numbers, dates, and proper nouns from the original article
- End with a complete sentence (ending with 。、！、or ？)

CRITICAL:
- Do NOT repeat what was already written
- Continue naturally from the last sentence
- Complete the summary to reach 1000-1500 characters total (including what was already written)
- Always end with a complete sentence

Original article content (for reference):
---
{content[:50000]}
---

Continue the summary from where it was cut off. Write only the continuation, not the entire summary.
<end_of_turn>
<start_of_turn>model
"""

        try:
            llm_options = {
                "temperature": self.config.summary_temperature,
                "repeat_penalty": self.config.llm_repeat_penalty,
            }

            # Use smaller num_predict for continuation (remaining tokens needed)
            # Optimize: calculate based on remaining chars, max 300 tokens
            remaining_chars = max(0, 1500 - len(existing_summary))
            continuation_tokens = min(remaining_chars + 200, 300)  # Safety margin, max 300 tokens

            llm_response = await self.llm_provider.generate(
                continuation_prompt,
                num_predict=continuation_tokens,
                options=llm_options,
            )

            continuation = self._clean_summary_text(llm_response.response, article_id)

            # Remove any repetition from the beginning (in case model repeated existing text)
            if continuation and existing_summary:
                # Check if continuation starts with text from existing summary
                existing_words = existing_summary[-50:].split()  # Last 50 chars as words
                continuation_words = continuation[:100].split()  # First 100 chars as words

                # Simple check: if first few words match, skip them
                if len(existing_words) > 0 and len(continuation_words) > 0:
                    if existing_words[-1] in continuation_words[:3]:
                        # Find where new content starts
                        for i, word in enumerate(continuation_words):
                            if word not in existing_words[-3:]:
                                continuation = ' '.join(continuation_words[i:])
                                break

            return continuation if continuation else None

        except Exception as e:
            logger.error(
                "Failed to generate continuation",
                extra={
                    "article_id": article_id,
                    "error": str(e),
                }
            )
            return None

    @staticmethod
    def _nanoseconds_to_milliseconds(value: Optional[int]) -> Optional[float]:
        """Convert nanoseconds to milliseconds."""
        if value is None:
            return None
        try:
            return value / 1_000_000
        except TypeError:
            return None
