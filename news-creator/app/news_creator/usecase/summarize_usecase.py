"""Summarize usecase - business logic for article summarization."""

import logging
from datetime import datetime, timedelta, timezone
from typing import Tuple, Dict, Any, List, Optional, AsyncIterator
import aiohttp

from news_creator.config.config import NewsCreatorConfig
from news_creator.domain.prompts import (
    SUMMARY_PROMPT_TEMPLATE,
    CHUNK_SUMMARY_PROMPT_TEMPLATE
)
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

    async def generate_summary(self, article_id: str, content: str, priority: str = "low") -> Tuple[str, Dict[str, Any]]:
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

        # Zero Trust: Always clean HTML from content, even if it appears to be plain text
        # This ensures we never process raw HTML, even if upstream services already extracted it
        original_content_length = len(content)
        logger.info(
            "Cleaning content (Zero Trust validation)",
            extra={
                "article_id": article_id,
                "original_length": original_content_length,
            }
        )

        cleaned_content, was_html = clean_html_content(content, article_id)
        cleaned_length = len(cleaned_content)

        if was_html:
            reduction_ratio = (1.0 - (cleaned_length / original_content_length)) * 100.0 if original_content_length > 0 else 0.0
            logger.warning(
                "HTML detected and removed from article content",
                extra={
                    "article_id": article_id,
                    "original_length": original_content_length,
                    "cleaned_length": cleaned_length,
                    "reduction_ratio": round(reduction_ratio, 2),
                }
            )
        else:
            logger.info(
                "Content appears to be plain text (no HTML detected)",
                extra={
                    "article_id": article_id,
                    "content_length": cleaned_length,
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
        # Context window is 16K tokens (16384) for normal AI Summary
        # We need to account for prompt template (~500 chars ≈ ~200 tokens)
        # Conservative estimate: 1 char ≈ 0.25-0.5 tokens (Japanese text)
        # Reserve ~1K tokens for prompt template and safety margin, leaving ~15K tokens for content
        # Using ~60K chars (≈15K tokens) for content to avoid truncation and stay within 16K limit
        MAX_CONTENT_LENGTH = 60_000  # characters (conservative estimate for ~15K tokens in 16K context)
        original_length = len(content)

        # Check for Hierarchical Summarization needed
        if original_length > self.config.hierarchical_single_article_threshold:
            logger.info(
                "Content exceeds threshold, switching to hierarchical summarization",
                extra={
                    "article_id": article_id,
                    "length": original_length,
                    "threshold": self.config.hierarchical_single_article_threshold
                }
            )
            return await self._generate_hierarchical_summary(article_id, content)

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
        jst = timezone(timedelta(hours=9))
        current_date_str = datetime.now(jst).strftime("%Y-%m-%d")
        prompt = SUMMARY_PROMPT_TEMPLATE.format(content=truncated_content, current_date=current_date_str)
        prompt_length = len(prompt)
        template_length = prompt_length - len(truncated_content)

        # Validate prompt size - detect abnormal amplification
        ABNORMAL_PROMPT_THRESHOLD = 100_000  # characters (>25K tokens)
        estimated_prompt_tokens = prompt_length // 4
        context_window = self.config.llm_num_ctx

        if prompt_length > ABNORMAL_PROMPT_THRESHOLD:
            # Abnormal prompt size detected - log detailed information for investigation
            # Check for repetition in the prompt
            has_repetition, repetition_score, repetition_patterns = detect_repetition(prompt, threshold=0.3)

            logger.error(
                "ABNORMAL PROMPT SIZE DETECTED in summarize_usecase",
                extra={
                    "article_id": article_id,
                    "prompt_length": prompt_length,
                    "content_length": len(truncated_content),
                    "template_length": template_length,
                    "estimated_prompt_tokens": estimated_prompt_tokens,
                    "context_window": context_window,
                    "prompt_preview_start": prompt[:500],
                    "prompt_preview_end": prompt[-500:] if prompt_length > 1000 else "",
                    "content_preview": truncated_content[:500] if len(truncated_content) > 500 else truncated_content,
                    "has_repetition": has_repetition,
                    "repetition_score": repetition_score,
                    "repetition_patterns": repetition_patterns,
                }
            )
            # Check if prompt contains repeated content
            if len(truncated_content) * 10 < prompt_length:
                logger.error(
                    "Prompt size is much larger than content - possible repetition or amplification",
                    extra={
                        "article_id": article_id,
                        "content_length": len(truncated_content),
                        "prompt_length": prompt_length,
                        "ratio": prompt_length / len(truncated_content) if truncated_content else 0,
                        "has_repetition": has_repetition,
                        "repetition_score": repetition_score,
                    }
                )

        logger.info(
            "Prompt generated",
            extra={
                "article_id": article_id,
                "prompt_length": prompt_length,
                "content_length": len(truncated_content),
                "template_length": template_length,
                "estimated_prompt_tokens": estimated_prompt_tokens,
                "context_window": context_window,
                "usage_percent": round((estimated_prompt_tokens / context_window) * 100, 1) if context_window > 0 else 0,
            }
        )

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

        # Token-budget-based hierarchical fallback
        total_token_budget = estimated_prompt_tokens + self.config.summary_num_predict
        budget_limit = int(context_window * self.config.hierarchical_token_budget_percent / 100)

        if total_token_budget > budget_limit:
            logger.warning(
                "Token budget exceeded, falling back to hierarchical summarization",
                extra={
                    "article_id": article_id,
                    "estimated_prompt_tokens": estimated_prompt_tokens,
                    "summary_num_predict": self.config.summary_num_predict,
                    "total_token_budget": total_token_budget,
                    "budget_limit": budget_limit,
                    "context_window": context_window,
                }
            )
            return await self._generate_hierarchical_summary(article_id, content)

        # Retry loop with repetition detection
        # IMPORTANT: Use hold_slot to acquire semaphore ONCE for all retries.
        # Previously, generate() was called per retry, re-acquiring the semaphore each time,
        # causing retries to wait 3500s+ in the queue again.
        max_retries = self.config.max_repetition_retries
        last_error = None
        last_metadata = None
        has_repetition = False
        rep_score = 0.0
        rep_patterns: List[str] = []
        attempt = 0
        raw_summary = ""
        llm_response = None
        consecutive_empty_count = 0

        is_high_priority = priority == "high"

        import time

        async with self.llm_provider.hold_slot(is_high_priority=is_high_priority) as (wait_time, cancel_event, task_id):
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

                # Call LLM provider with adjusted parameters (no semaphore re-acquisition)
                llm_options = {
                    "temperature": current_temp,
                    "repeat_penalty": current_repeat_penalty,
                }

                start_time = time.time()

                llm_response = await self.llm_provider.generate_raw(
                    prompt,
                    cancel_event=cancel_event,
                    task_id=task_id,
                    num_predict=self.config.summary_num_predict,
                    options=llm_options,
                )

                elapsed_time = time.time() - start_time

                # Log generation performance
                tokens_per_second = round(llm_response.eval_count / elapsed_time, 2) if llm_response.eval_count and elapsed_time > 0 else None
                logger.info(
                    f"LLM generation completed: article_id={article_id}, elapsed={round(elapsed_time, 2)}s, "
                    f"prompt_eval_count={llm_response.prompt_eval_count}, eval_count={llm_response.eval_count}, "
                    f"prompt_length={prompt_length} chars, estimated_tokens={estimated_prompt_tokens}, "
                    f"num_predict={self.config.summary_num_predict}, tokens_per_second={tokens_per_second}"
                )

                # Clean and validate summary
                raw_summary = llm_response.response

                # Check for empty or whitespace-only response BEFORE cleaning
                # LLM sometimes returns only whitespace (e.g., 60 spaces), which becomes empty after cleaning
                raw_text_stripped = raw_summary.strip() if raw_summary else ""
                if len(raw_text_stripped) < 10:
                    consecutive_empty_count += 1
                    logger.warning(
                        "LLM returned insufficient content (empty or whitespace-only)",
                        extra={
                            "article_id": article_id,
                            "attempt": attempt + 1,
                            "raw_length": len(raw_summary) if raw_summary else 0,
                            "stripped_length": len(raw_text_stripped),
                            "raw_preview": repr(raw_summary[:100]) if raw_summary else "None",
                            "consecutive_empty_count": consecutive_empty_count,
                        }
                    )
                    # Bail early on consecutive empty responses to release the slot
                    if consecutive_empty_count >= 2:
                        raise RuntimeError(
                            f"LLM returned empty/whitespace summary {consecutive_empty_count} "
                            f"times consecutively for article {article_id}. "
                            f"Model may be in a bad state."
                        )
                    if attempt < max_retries:
                        last_error = f"LLM returned insufficient content (length: {len(raw_text_stripped)})"
                        last_metadata = {
                            "model": llm_response.model,
                            "prompt_tokens": llm_response.prompt_eval_count,
                            "completion_tokens": llm_response.eval_count,
                            "total_duration_ms": self._nanoseconds_to_milliseconds(llm_response.total_duration),
                        }
                        continue  # Retry with adjusted temperature
                else:
                    consecutive_empty_count = 0  # Reset on non-empty response

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

    async def _generate_hierarchical_summary(self, article_id: str, content: str) -> Tuple[str, Dict[str, Any]]:
        """
        Generate a summary for a large article using Hierarchical (Map-Reduce) strategy.

        Args:
            article_id: Article identifier
            content: Full article content

        Returns:
            Tuple of (summary text, metadata dict)
        """
        import time
        start_time = time.time()

        chunk_size = self.config.hierarchical_single_article_chunk_size
        chunks = []
        for i in range(0, len(content), chunk_size):
            chunks.append(content[i:i + chunk_size])

        logger.info(
            "Starting hierarchical summarization (Map-Reduce)",
            extra={
                "article_id": article_id,
                "content_length": len(content),
                "chunk_count": len(chunks),
                "chunk_size": chunk_size
            }
        )

        # Map Phase: Summarize each chunk
        chunk_summaries = []
        total_prompt_tokens = 0
        total_completion_tokens = 0

        for i, chunk in enumerate(chunks):
            logger.info(
                f"Map phase: Processing chunk {i+1}/{len(chunks)}",
                extra={"article_id": article_id, "chunk_index": i}
            )

            prompt = CHUNK_SUMMARY_PROMPT_TEMPLATE.format(content=chunk)

            # Use somewhat higher temperature for extraction to avoid rigid repetition
            llm_options = {
                "temperature": 0.2,
                "repeat_penalty": self.config.llm_repeat_penalty
            }

            try:
                # Use a smaller num_predict for chunks to save time
                chunk_resp = await self.llm_provider.generate(
                    prompt,
                    num_predict=500,
                    options=llm_options
                )
                chunk_text = chunk_resp.response

                # Cleanup chunk summary
                chunk_text = self._clean_summary_text(chunk_text, f"{article_id}-chunk-{i}")

                if chunk_text and chunk_text != "なし":
                    chunk_summaries.append(chunk_text)

                if chunk_resp.prompt_eval_count:
                    total_prompt_tokens += chunk_resp.prompt_eval_count
                if chunk_resp.eval_count:
                    total_completion_tokens += chunk_resp.eval_count

            except Exception as e:
                logger.error(
                    f"Failed to summarize chunk {i}",
                    extra={"article_id": article_id, "error": str(e)}
                )
                # Continue best effort

        if not chunk_summaries:
            raise RuntimeError("Hierarchical summarization failed: no valid chunk summaries generated")

        # Reduce Phase: Summarize the combined chunk summaries
        combined_text = "\n\n".join(chunk_summaries)

        logger.info(
            "Reduce phase: Summarizing combined content",
            extra={
                "article_id": article_id,
                "combined_length": len(combined_text),
                "reduction_ratio": f"{(1 - len(combined_text)/len(content))*100:.1f}%"
            }
        )

        # We can reuse the standard generate_summary logic for the reduce phase,
        # but we need to bypass the length check to avoid recursion if combined text is huge
        # (unlikely given 500 char limit per chunk, but possible).
        # Instead, just call LLM directly with standard summary prompt.

        prompt = SUMMARY_PROMPT_TEMPLATE.format(content=combined_text, current_date=datetime.now(timezone(timedelta(hours=9))).strftime("%Y-%m-%d"))

        # Use standard summary configuration
        llm_options = {
            "temperature": self.config.summary_temperature,
            "repeat_penalty": self.config.llm_repeat_penalty
        }

        final_resp = await self.llm_provider.generate(
            prompt,
            num_predict=self.config.summary_num_predict,
            options=llm_options
        )

        raw_summary = final_resp.response
        cleaned_summary = self._clean_summary_text(raw_summary, article_id)

        # Enforce character limit
        truncated_summary = cleaned_summary[:2000]

        elapsed_ms = (time.time() - start_time) * 1000
        total_prompt_tokens += final_resp.prompt_eval_count or 0
        total_completion_tokens += final_resp.eval_count or 0

        metadata = {
            "model": final_resp.model,
            "prompt_tokens": total_prompt_tokens,
            "completion_tokens": total_completion_tokens,
            "total_duration_ms": elapsed_ms,
            "strategy": "hierarchical"
        }

        return truncated_summary, metadata

    async def generate_summary_stream(self, article_id: str, content: str, priority: str = "low") -> AsyncIterator[str]:
        """
        Generate a Japanese summary for an article as a stream of tokens.

        Args:
            article_id: Article identifier
            content: Article content to summarize

        Yields:
            Summary tokens
        """
        logger.info(
            "Starting stream summary generation",
            extra={
                "article_id": article_id,
                "content_length": len(content) if content else 0,
            }
        )

        if not article_id or not article_id.strip():
            raise ValueError("article_id cannot be empty")
        if not content or not content.strip():
            raise ValueError("content cannot be empty")

        # Zero Trust: Clean HTML
        cleaned_content, _ = clean_html_content(content, article_id)
        content = cleaned_content

        # Basic validation (same as sync method)
        min_content_length = 100
        if not content or not content.strip() or len(content.strip()) < min_content_length:
            error_msg = (
                f"Content is too short for summarization. "
                f"Content length: {len(content) if content else 0}, "
                f"Minimum required: {min_content_length} characters."
            )
            logger.warning(
                "Article content too short for summarization",
                extra={
                    "article_id": article_id,
                    "content_length": len(content) if content else 0,
                    "min_required": min_content_length,
                }
            )
            # Don't yield empty string - raise error instead to prevent empty streams
            raise ValueError(error_msg)

        # Truncate content
        MAX_CONTENT_LENGTH = 60_000
        truncated_content = content.strip()[:MAX_CONTENT_LENGTH]

        # DEBUG: Log exact lengths to diagnose 1M char prompt issue
        logger.warning(
            f"DEBUG: Truncation check - Original: {len(content)}, Truncated: {len(truncated_content)}, Limit: {MAX_CONTENT_LENGTH}",
            extra={
                "article_id": article_id,
                "original_type": str(type(content)),
                "truncated_type": str(type(truncated_content)),
            }
        )

        if len(content) > MAX_CONTENT_LENGTH:
            logger.warning(
                "Content truncated for streaming",
                extra={
                    "article_id": article_id,
                    "original_length": len(content),
                    "truncated_length": len(truncated_content),
                }
            )

        # Build prompt: Ensure we use truncated_content and enforce limit again just in case
        safe_content = truncated_content[:MAX_CONTENT_LENGTH]
        jst = timezone(timedelta(hours=9))
        current_date_str = datetime.now(jst).strftime("%Y-%m-%d")
        prompt = SUMMARY_PROMPT_TEMPLATE.format(content=safe_content, current_date=current_date_str)
        prompt_length = len(prompt)

        logger.info(
            f"Prompt generated for streaming (len={prompt_length})",
            extra={
                "article_id": article_id,
                "prompt_length": prompt_length,
                "content_length": len(truncated_content),
            }
        )

        # Call LLM provider with streaming
        llm_options = {
            "temperature": self.config.summary_temperature,
            "repeat_penalty": self.config.llm_repeat_penalty,
        }

        try:
            logger.info(
                "Calling LLM provider with streaming",
                extra={
                    "article_id": article_id,
                    "stream": True,
                    "num_predict": self.config.summary_num_predict,
                }
            )

            stream_gen = await self.llm_provider.generate(
                prompt,
                num_predict=self.config.summary_num_predict,
                stream=True,
                options=llm_options,
                priority=priority,
            )

            logger.info(
                "Stream generator obtained from LLM provider",
                extra={
                    "article_id": article_id,
                }
            )

            # Control tokens to filter
            # Note: A real robust filter would buffer to handle split tokens,
            # but for now we assume tokens come in reasonable chunks or at least not split control tokens often.
            # We'll just filter valid exact matches or basic substring checks if it's a single token.
            ignored_tokens = {
                "<start_of_turn>", "<end_of_turn>",
                "<|system|>", "<|user|>", "<|assistant|>"
            }

            tokens_yielded = 0
            bytes_yielded = 0
            chunks_received = 0
            has_data = False

            async for chunk in stream_gen:
                chunks_received += 1
                token = chunk.response
                if token and token not in ignored_tokens:
                    has_data = True
                    tokens_yielded += 1
                    bytes_yielded += len(token.encode('utf-8'))

                    # Log first few tokens and periodically
                    if tokens_yielded <= 3 or tokens_yielded % 50 == 0:
                        logger.info(
                            "Yielding token from stream",
                            extra={
                                "article_id": article_id,
                                "token_number": tokens_yielded,
                                "token_preview": token[:50] if len(token) > 50 else token,
                                "bytes_yielded": bytes_yielded,
                                "chunks_received": chunks_received,
                            }
                        )

                    # Very basic filtering of partial control tokens could be added here if needed
                    # For now, yield as is
                    yield token
                elif token:
                    # Log ignored tokens for debugging
                    if chunks_received <= 5:
                        logger.debug(
                            "Ignored control token",
                            extra={
                                "article_id": article_id,
                                "token": token,
                                "chunks_received": chunks_received,
                            }
                        )

            if not has_data:
                logger.warning(
                    "Stream completed but no data was yielded",
                    extra={
                        "article_id": article_id,
                        "chunks_received": chunks_received,
                        "tokens_yielded": tokens_yielded,
                    }
                )
            else:
                logger.info(
                    "Stream completed successfully",
                    extra={
                        "article_id": article_id,
                        "tokens_yielded": tokens_yielded,
                        "bytes_yielded": bytes_yielded,
                        "chunks_received": chunks_received,
                    }
                )

        except aiohttp.ClientConnectionError as conn_err:
            # Connection error during streaming - check if we have partial data
            if tokens_yielded > 0:
                logger.warning(
                    "Connection error during streaming, but partial summary was generated",
                    extra={
                        "article_id": article_id,
                        "error": str(conn_err),
                        "error_type": type(conn_err).__name__,
                        "tokens_yielded": tokens_yielded,
                        "bytes_yielded": bytes_yielded,
                        "chunks_received": chunks_received,
                    }
                )
                # Don't raise - the partial data has already been yielded
                return
            else:
                logger.error(
                    "Connection error during streaming with no data received",
                    extra={
                        "article_id": article_id,
                        "error": str(conn_err),
                        "error_type": type(conn_err).__name__,
                        "chunks_received": chunks_received,
                    },
                    exc_info=True
                )
                raise RuntimeError(f"Streaming summary generation failed: connection closed before any data was received") from conn_err
        except Exception as e:
            logger.error(
                "Streaming summary generation failed",
                extra={
                    "article_id": article_id,
                    "error": str(e),
                    "error_type": type(e).__name__,
                    "tokens_yielded": tokens_yielded,
                    "bytes_yielded": bytes_yielded,
                    "chunks_received": chunks_received,
                },
                exc_info=True
            )
            raise RuntimeError(f"Streaming summary generation failed: {e}") from e


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
