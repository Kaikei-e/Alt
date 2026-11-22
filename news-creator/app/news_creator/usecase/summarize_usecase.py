"""Summarize usecase - business logic for article summarization."""

import logging
import re
import time
from collections.abc import AsyncGenerator, AsyncIterator
from contextlib import AsyncExitStack, aclosing
from dataclasses import dataclass, field
from datetime import datetime, timedelta, timezone
from typing import Protocol, runtime_checkable

import aiohttp

from news_creator.config.config import NewsCreatorConfig
from news_creator.domain.errors import PreemptedException
from news_creator.domain.models import LLMGenerateResponse, SummaryMetadata
from news_creator.domain.output_guard import (
    StreamingOutputGuard,
    sanitize_output_markdown,
)
from news_creator.domain.prompts import (
    _THINKING_BLOCK_RE,
    CHUNK_SUMMARY_PROMPT_TEMPLATE,
    SUMMARY_PROMPT_TEMPLATE,
)
from news_creator.port.llm_provider_port import LLMProviderPort
from news_creator.usecase.control_token_filter import ControlTokenFilter
from news_creator.utils.html_cleaner import clean_html_content, sanitize_for_prompt
from news_creator.utils.repetition_detector import detect_repetition

logger = logging.getLogger(__name__)


@runtime_checkable
class _SupportsAclose(Protocol):
    async def aclose(self) -> None: ...


_ABNORMAL_PROMPT_END_MIN_CHARS = 1000
_CONTENT_PREVIEW_MAX_CHARS = 500
_MIN_SUMMARY_STRIPPED_CHARS = 10
_MAX_CONSECUTIVE_EMPTY_RESPONSES = 2
_INITIAL_STREAM_LOG_TOKENS = 3
_PERIODIC_STREAM_LOG_INTERVAL = 50
_STREAM_TOKEN_PREVIEW_CHARS = 50
_MIN_CONTENT_LENGTH = 100
_MAX_CONTENT_LENGTH = 60_000
_ABNORMAL_PROMPT_THRESHOLD = 100_000
_PROMPT_REPETITION_THRESHOLD = 0.3
_AMPLIFICATION_RATIO_LIMIT = 10
_CHARS_PER_TOKEN_ESTIMATE = 4
_PERCENT_SCALE = 100
_CONTEXT_USAGE_WARNING_RATIO = 0.9
_MAX_FINAL_SUMMARY_CHARS = 2000
_RETRY_MIN_TEMPERATURE = 0.05
_RETRY_TEMP_STEP = 0.05
_RETRY_MAX_REPEAT_PENALTY = 1.2
_RETRY_PENALTY_STEP = 0.05
_RAW_SUMMARY_PREVIEW_CHARS = 200
_RAW_ERROR_PREVIEW_CHARS = 300
_JST_TZ = timezone(timedelta(hours=9))


@dataclass(frozen=True)
class _PreparedSummaryPrompt:
    prompt: str
    prompt_length: int
    estimated_prompt_tokens: int
    needs_hierarchical: bool


@dataclass
class _RetryLoopState:
    last_metadata: SummaryMetadata | None = None
    has_repetition: bool = False
    rep_score: float = 0.0
    rep_patterns: list[str] = field(default_factory=list)
    attempt: int = 0
    raw_summary: str = ""
    llm_response: LLMGenerateResponse | None = None
    consecutive_empty_count: int = 0
    retry_reason: str | None = None
    repetition_retry_count: int = 0


@dataclass
class _StreamMetrics:
    tokens_yielded: int = 0
    bytes_yielded: int = 0
    chunks_received: int = 0
    has_data: bool = False
    received_done: bool = False


def _validate_and_clean_summary_input(article_id: str, content: str) -> str:
    if not article_id or not article_id.strip():
        msg = "article_id cannot be empty"
        raise ValueError(msg)
    if not content or not content.strip():
        msg = "content cannot be empty"
        raise ValueError(msg)

    # Zero Trust: Always clean HTML from content, even if it appears to be plain text
    # This ensures we never process raw HTML, even if upstream services already extracted it
    original_content_length = len(content)
    logger.info(
        "Cleaning content (Zero Trust validation)",
        extra={
            "article_id": article_id,
            "original_length": original_content_length,
        },
    )

    cleaned_content, was_html = clean_html_content(content, article_id)
    cleaned_length = len(cleaned_content)

    if was_html:
        reduction_ratio = (
            (1.0 - (cleaned_length / original_content_length)) * _PERCENT_SCALE
            if original_content_length > 0
            else 0.0
        )
        logger.warning(
            "HTML detected and removed from article content",
            extra={
                "article_id": article_id,
                "original_length": original_content_length,
                "cleaned_length": cleaned_length,
                "reduction_ratio": round(reduction_ratio, 2),
            },
        )
    else:
        logger.info(
            "Content appears to be plain text (no HTML detected)",
            extra={
                "article_id": article_id,
                "content_length": cleaned_length,
            },
        )

    # Validate that we have meaningful content after cleaning
    if (
        not cleaned_content
        or not cleaned_content.strip()
        or len(cleaned_content.strip()) < _MIN_CONTENT_LENGTH
    ):
        error_msg = (
            f"Content is empty or too short after HTML cleaning. "
            f"Original length: {original_content_length}, "
            f"Cleaned length: {cleaned_length}, "
            f"Minimum required: {_MIN_CONTENT_LENGTH} characters. "
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
                "cleaned_length": cleaned_length,
                "min_required": _MIN_CONTENT_LENGTH,
                "content_preview": cleaned_content[:_MIN_CONTENT_LENGTH]
                if cleaned_content
                else "",
            },
        )
        raise ValueError(error_msg)

    return cleaned_content


def _check_abnormal_prompt_size(
    article_id: str,
    prompt: str,
    truncated_content: str,
    estimated_prompt_tokens: int,
    context_window: int,
) -> None:
    prompt_length = len(prompt)
    if prompt_length <= _ABNORMAL_PROMPT_THRESHOLD:
        return

    template_length = prompt_length - len(truncated_content)
    has_repetition, repetition_score, repetition_patterns = detect_repetition(
        prompt, threshold=_PROMPT_REPETITION_THRESHOLD
    )

    logger.error(
        "ABNORMAL PROMPT SIZE DETECTED in summarize_usecase",
        extra={
            "article_id": article_id,
            "prompt_length": prompt_length,
            "content_length": len(truncated_content),
            "template_length": template_length,
            "estimated_prompt_tokens": estimated_prompt_tokens,
            "context_window": context_window,
            "prompt_preview_start": prompt[:_CONTENT_PREVIEW_MAX_CHARS],
            "prompt_preview_end": prompt[-_CONTENT_PREVIEW_MAX_CHARS:]
            if prompt_length > _ABNORMAL_PROMPT_END_MIN_CHARS
            else "",
            "content_preview": truncated_content[:_CONTENT_PREVIEW_MAX_CHARS]
            if len(truncated_content) > _CONTENT_PREVIEW_MAX_CHARS
            else truncated_content,
            "has_repetition": has_repetition,
            "repetition_score": repetition_score,
            "repetition_patterns": repetition_patterns,
        },
    )
    if len(truncated_content) * _AMPLIFICATION_RATIO_LIMIT < prompt_length:
        logger.error(
            "Prompt size is much larger than content - possible repetition or amplification",
            extra={
                "article_id": article_id,
                "content_length": len(truncated_content),
                "prompt_length": prompt_length,
                "ratio": prompt_length / len(truncated_content)
                if truncated_content
                else 0,
                "has_repetition": has_repetition,
                "repetition_score": repetition_score,
            },
        )


def _prepare_stream_prompt(article_id: str, content: str) -> str:
    logger.info(
        "Starting stream summary generation",
        extra={
            "article_id": article_id,
            "content_length": len(content) if content else 0,
        },
    )

    if not article_id or not article_id.strip():
        msg = "article_id cannot be empty"
        raise ValueError(msg)
    if not content or not content.strip():
        msg = "content cannot be empty"
        raise ValueError(msg)

    # Zero Trust: Clean HTML
    cleaned_content, _ = clean_html_content(content, article_id)

    # Basic validation (same as sync method)
    if (
        not cleaned_content
        or not cleaned_content.strip()
        or len(cleaned_content.strip()) < _MIN_CONTENT_LENGTH
    ):
        error_msg = (
            f"Content is too short for summarization. "
            f"Content length: {len(cleaned_content) if cleaned_content else 0}, "
            f"Minimum required: {_MIN_CONTENT_LENGTH} characters."
        )
        logger.warning(
            "Article content too short for summarization",
            extra={
                "article_id": article_id,
                "content_length": len(cleaned_content) if cleaned_content else 0,
                "min_required": _MIN_CONTENT_LENGTH,
            },
        )
        # Don't yield empty string - raise error instead to prevent empty streams
        raise ValueError(error_msg)

    # Truncate content
    truncated_content = cleaned_content.strip()[:_MAX_CONTENT_LENGTH]

    # DEBUG: Log exact lengths to diagnose 1M char prompt issue
    logger.debug(
        "Truncation check",
        extra={
            "article_id": article_id,
            "original_length": len(cleaned_content),
            "truncated_length": len(truncated_content),
            "limit": _MAX_CONTENT_LENGTH,
            "original_type": str(type(cleaned_content)),
            "truncated_type": str(type(truncated_content)),
        },
    )

    if len(cleaned_content) > _MAX_CONTENT_LENGTH:
        logger.warning(
            "Content truncated for streaming",
            extra={
                "article_id": article_id,
                "original_length": len(cleaned_content),
                "truncated_length": len(truncated_content),
            },
        )

    # Build prompt: Ensure we use truncated_content and enforce limit again just in case
    safe_content = sanitize_for_prompt(truncated_content[:_MAX_CONTENT_LENGTH])
    current_date_str = datetime.now(_JST_TZ).strftime("%Y-%m-%d")
    prompt = SUMMARY_PROMPT_TEMPLATE.format(
        content=safe_content, current_date=current_date_str
    )
    prompt_length = len(prompt)

    logger.info(
        "Prompt generated for streaming (len=%s)",
        prompt_length,
        extra={
            "article_id": article_id,
            "prompt_length": prompt_length,
            "content_length": len(truncated_content),
        },
    )
    return prompt


def _record_emitted_token(
    article_id: str,
    token_text: str,
    metrics: _StreamMetrics,
    *,
    log_token: bool,
) -> None:
    metrics.has_data = True
    metrics.tokens_yielded += 1
    metrics.bytes_yielded += len(token_text.encode("utf-8"))

    # Log first few tokens and periodically
    if log_token and (
        metrics.tokens_yielded <= _INITIAL_STREAM_LOG_TOKENS
        or metrics.tokens_yielded % _PERIODIC_STREAM_LOG_INTERVAL == 0
    ):
        logger.info(
            "Yielding token from stream",
            extra={
                "article_id": article_id,
                "token_number": metrics.tokens_yielded,
                "token_preview": token_text[:_STREAM_TOKEN_PREVIEW_CHARS]
                if len(token_text) > _STREAM_TOKEN_PREVIEW_CHARS
                else token_text,
                "bytes_yielded": metrics.bytes_yielded,
                "chunks_received": metrics.chunks_received,
            },
        )


def _finalize_stream_status(article_id: str, metrics: _StreamMetrics) -> None:
    if not metrics.has_data:
        logger.warning(
            "Stream completed but no data was yielded",
            extra={
                "article_id": article_id,
                "chunks_received": metrics.chunks_received,
                "tokens_yielded": metrics.tokens_yielded,
            },
        )
    elif not metrics.received_done:
        # Tokens were already yielded to the client above, but the
        # stream ended without Ollama's done=true chunk -- this
        # summary is truncated. Raising here (instead of ending
        # the generator normally) lets the SSE handler surface an
        # `event: error` instead of the truncated text looking
        # like a complete summary.
        logger.error(
            "Stream ended without a done=true chunk; summary is truncated",
            extra={
                "article_id": article_id,
                "tokens_yielded": metrics.tokens_yielded,
                "bytes_yielded": metrics.bytes_yielded,
                "chunks_received": metrics.chunks_received,
            },
        )
        msg = (
            f"Streaming summary for article {article_id} ended before a "
            f"done=true chunk was received; response is truncated"
        )
        raise RuntimeError(msg)
    else:
        logger.info(
            "Stream completed successfully",
            extra={
                "article_id": article_id,
                "tokens_yielded": metrics.tokens_yielded,
                "bytes_yielded": metrics.bytes_yielded,
                "chunks_received": metrics.chunks_received,
            },
        )


async def _consume_summary_stream(
    stream_gen: AsyncIterator[LLMGenerateResponse],
    article_id: str,
    metrics: _StreamMetrics,
) -> AsyncGenerator[str]:
    # Control tokens may be split across chunks — buffer at boundaries.
    token_filter = ControlTokenFilter()
    # Links have to clear the output guard before the client renders
    # them, and a link is only judgeable once its URL is complete.
    output_guard = StreamingOutputGuard()

    async for chunk in stream_gen:
        metrics.chunks_received += 1
        if chunk.done:
            metrics.received_done = True
        token = chunk.response
        if token:
            filtered = output_guard.push(token_filter.push(token))
            if filtered:
                _record_emitted_token(article_id, filtered, metrics, log_token=True)
                yield filtered

    # Flush any buffered prefix that never completed a control
    # token or a markdown link
    remaining = output_guard.push(token_filter.flush()) + output_guard.flush()
    if remaining:
        _record_emitted_token(article_id, remaining, metrics, log_token=False)
        yield remaining

    _finalize_stream_status(article_id, metrics)


def _raise_stream_connection_error(
    article_id: str,
    conn_err: aiohttp.ClientConnectionError,
    metrics: _StreamMetrics,
) -> None:
    if metrics.tokens_yielded > 0:
        # Tokens were already yielded to the client, but a connection
        # drop mid-stream means no done=true chunk was ever received --
        # this is truncated, not a completed summary. Raise instead of
        # returning silently so the SSE handler can surface an error.
        logger.exception(
            "Connection error during streaming after partial summary was generated; response is truncated",
            extra={
                "article_id": article_id,
                "error": str(conn_err),
                "error_type": type(conn_err).__name__,
                "tokens_yielded": metrics.tokens_yielded,
                "bytes_yielded": metrics.bytes_yielded,
                "chunks_received": metrics.chunks_received,
            },
        )
        msg = (
            f"Streaming summary generation for article {article_id} failed: "
            f"connection closed after partial data was received; response is truncated"
        )
        raise RuntimeError(msg) from conn_err

    logger.exception(
        "Connection error during streaming with no data received",
        extra={
            "article_id": article_id,
            "error": str(conn_err),
            "error_type": type(conn_err).__name__,
            "chunks_received": metrics.chunks_received,
        },
    )
    msg = "Streaming summary generation failed: connection closed before any data was received"
    raise RuntimeError(msg) from conn_err


def _ensure_stream_iterator(
    result: LLMGenerateResponse | AsyncIterator[LLMGenerateResponse],
) -> AsyncIterator[LLMGenerateResponse]:
    if isinstance(result, LLMGenerateResponse):
        msg = "Expected AsyncIterator for streaming"
        raise TypeError(msg)
    return result


class SummarizeUsecase:
    """Usecase for generating Japanese article summaries."""

    def __init__(
        self, config: NewsCreatorConfig, llm_provider: LLMProviderPort
    ) -> None:
        """Initialize summarize usecase."""
        self.config = config
        self.llm_provider = llm_provider

    def _prepare_summary_prompt(
        self, article_id: str, content: str
    ) -> _PreparedSummaryPrompt:
        # Truncate content to fit within context window
        # Context window is 16K tokens (16384) for normal AI Summary
        # We need to account for prompt template (~500 chars ≈ ~200 tokens)
        # Conservative estimate: 1 char ≈ 0.25-0.5 tokens (Japanese text)
        # Reserve ~1K tokens for prompt template and safety margin, leaving ~15K tokens for content
        # Using ~60K chars (≈15K tokens) for content to avoid truncation and stay within 16K limit
        original_length = len(content)

        # Check for Hierarchical Summarization needed
        if original_length > self.config.hierarchical_single_article_threshold:
            logger.info(
                "Content exceeds threshold, switching to hierarchical summarization",
                extra={
                    "article_id": article_id,
                    "length": original_length,
                    "threshold": self.config.hierarchical_single_article_threshold,
                },
            )
            return _PreparedSummaryPrompt(
                prompt="",
                prompt_length=0,
                estimated_prompt_tokens=0,
                needs_hierarchical=True,
            )

        truncated_content = content.strip()[:_MAX_CONTENT_LENGTH]

        if original_length > _MAX_CONTENT_LENGTH:
            logger.warning(
                "Input content truncated to fit context window",
                extra={
                    "article_id": article_id,
                    "original_length": original_length,
                    "truncated_length": len(truncated_content),
                    "max_length": _MAX_CONTENT_LENGTH,
                },
            )

        logger.info(
            "Generating summary for article: %s",
            article_id,
            extra={
                "article_id": article_id,
                "content_length": len(truncated_content),
                "was_truncated": original_length > _MAX_CONTENT_LENGTH,
            },
        )

        # Build prompt from template
        current_date_str = datetime.now(_JST_TZ).strftime("%Y-%m-%d")
        sanitized_content = sanitize_for_prompt(truncated_content)
        prompt = SUMMARY_PROMPT_TEMPLATE.format(
            content=sanitized_content, current_date=current_date_str
        )
        prompt_length = len(prompt)
        template_length = prompt_length - len(truncated_content)

        # Validate prompt size - detect abnormal amplification
        estimated_prompt_tokens = prompt_length // _CHARS_PER_TOKEN_ESTIMATE
        context_window = self.config.llm_num_ctx

        _check_abnormal_prompt_size(
            article_id,
            prompt,
            truncated_content,
            estimated_prompt_tokens,
            context_window,
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
                "usage_percent": round(
                    (estimated_prompt_tokens / context_window) * _PERCENT_SCALE, 1
                )
                if context_window > 0
                else 0,
            },
        )

        if (
            estimated_prompt_tokens > context_window * _CONTEXT_USAGE_WARNING_RATIO
        ):  # Warn if using >90% of context window
            logger.warning(
                "Prompt may be close to context window limit",
                extra={
                    "article_id": article_id,
                    "estimated_prompt_tokens": estimated_prompt_tokens,
                    "context_window": context_window,
                    "usage_percent": round(
                        (estimated_prompt_tokens / context_window) * _PERCENT_SCALE, 1
                    ),
                },
            )

        # Token-budget-based hierarchical fallback
        total_token_budget = estimated_prompt_tokens + self.config.summary_num_predict
        budget_limit = int(
            context_window
            * self.config.hierarchical_token_budget_percent
            / _PERCENT_SCALE
        )

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
                },
            )
            return _PreparedSummaryPrompt(
                prompt="",
                prompt_length=0,
                estimated_prompt_tokens=0,
                needs_hierarchical=True,
            )

        return _PreparedSummaryPrompt(
            prompt=prompt,
            prompt_length=prompt_length,
            estimated_prompt_tokens=estimated_prompt_tokens,
            needs_hierarchical=False,
        )

    def _build_attempt_options(
        self,
        article_id: str,
        attempt: int,
        retry_reason: str | None,
        repetition_retry_count: int,
    ) -> dict[str, float]:
        current_temp = self.config.summary_temperature
        current_repeat_penalty = self.config.llm_repeat_penalty

        if attempt > 0:
            if retry_reason == "repetition":
                # Progressively lower temperature and increase repetition penalty
                current_temp = max(
                    _RETRY_MIN_TEMPERATURE,
                    self.config.summary_temperature
                    - (_RETRY_TEMP_STEP * repetition_retry_count),
                )
                current_repeat_penalty = min(
                    _RETRY_MAX_REPEAT_PENALTY,
                    self.config.llm_repeat_penalty
                    + (_RETRY_PENALTY_STEP * repetition_retry_count),
                )

            logger.warning(
                "Retrying summary generation due to %s",
                retry_reason,
                extra={
                    "article_id": article_id,
                    "attempt": attempt + 1,
                    "max_retries": self.config.max_repetition_retries + 1,
                    "temperature": current_temp,
                    "repeat_penalty": current_repeat_penalty,
                    "reason": retry_reason,
                },
            )

        return {
            "temperature": current_temp,
            "repeat_penalty": current_repeat_penalty,
        }

    def _log_generation_performance(
        self,
        article_id: str,
        llm_response: LLMGenerateResponse,
        prepared: _PreparedSummaryPrompt,
        elapsed_time: float,
    ) -> None:
        tokens_per_second = (
            round(llm_response.eval_count / elapsed_time, 2)
            if llm_response.eval_count and elapsed_time > 0
            else None
        )
        logger.info(
            "LLM generation completed: article_id=%s, elapsed=%ss, "
            "prompt_eval_count=%s, eval_count=%s, "
            "prompt_length=%s chars, estimated_tokens=%s, "
            "num_predict=%s, tokens_per_second=%s",
            article_id,
            round(elapsed_time, 2),
            llm_response.prompt_eval_count,
            llm_response.eval_count,
            prepared.prompt_length,
            prepared.estimated_prompt_tokens,
            self.config.summary_num_predict,
            tokens_per_second,
        )

    def _response_to_metadata(
        self, llm_response: LLMGenerateResponse
    ) -> SummaryMetadata:
        return SummaryMetadata(
            model=llm_response.model,
            prompt_tokens=llm_response.prompt_eval_count,
            completion_tokens=llm_response.eval_count,
            total_duration_ms=self._nanoseconds_to_milliseconds(
                llm_response.total_duration
            ),
        )

    async def _generate_with_repetition_retries(
        self,
        article_id: str,
        prepared: _PreparedSummaryPrompt,
        priority: str,
    ) -> _RetryLoopState:
        max_retries = self.config.max_repetition_retries
        state = _RetryLoopState()
        is_high_priority = priority == "high"

        # Hold one slot across repetition retries so retries do not re-queue.
        async with self.llm_provider.hold_slot(is_high_priority=is_high_priority) as (
            _wait_time,
            cancel_event,
            task_id,
        ):
            for attempt in range(max_retries + 1):
                state.attempt = attempt
                llm_options = self._build_attempt_options(
                    article_id,
                    attempt,
                    state.retry_reason,
                    state.repetition_retry_count,
                )

                start_time = time.time()
                llm_response = await self.llm_provider.generate_raw(
                    prepared.prompt,
                    cancel_event=cancel_event,
                    task_id=task_id,
                    num_predict=self.config.summary_num_predict,
                    options=llm_options,
                )
                elapsed_time = time.time() - start_time
                self._log_generation_performance(
                    article_id, llm_response, prepared, elapsed_time
                )

                state.llm_response = llm_response
                state.raw_summary = llm_response.response
                raw_text_stripped = (
                    state.raw_summary.strip() if state.raw_summary else ""
                )
                if len(raw_text_stripped) < _MIN_SUMMARY_STRIPPED_CHARS:
                    state.consecutive_empty_count += 1
                    logger.warning(
                        "LLM returned insufficient content (empty or whitespace-only)",
                        extra={
                            "article_id": article_id,
                            "attempt": attempt + 1,
                            "raw_length": len(state.raw_summary)
                            if state.raw_summary
                            else 0,
                            "stripped_length": len(raw_text_stripped),
                            "raw_preview": repr(state.raw_summary[:_MIN_CONTENT_LENGTH])
                            if state.raw_summary
                            else "None",
                            "consecutive_empty_count": state.consecutive_empty_count,
                        },
                    )
                    # Bail early on consecutive empty responses to release the slot
                    if (
                        state.consecutive_empty_count
                        >= _MAX_CONSECUTIVE_EMPTY_RESPONSES
                    ):
                        msg = (
                            f"LLM returned empty/whitespace summary {state.consecutive_empty_count} "
                            f"times consecutively for article {article_id}. "
                            f"Model may be in a bad state."
                        )
                        raise RuntimeError(msg)
                    if attempt < max_retries:
                        state.retry_reason = "empty_output"
                        state.last_metadata = self._response_to_metadata(llm_response)
                        continue
                else:
                    state.consecutive_empty_count = 0

                state.has_repetition, state.rep_score, state.rep_patterns = (
                    detect_repetition(
                        state.raw_summary, threshold=self.config.repetition_threshold
                    )
                )

                if state.has_repetition and attempt < max_retries:
                    state.retry_reason = "repetition"
                    state.repetition_retry_count += 1
                    logger.warning(
                        "Repetition detected in generated summary, will retry",
                        extra={
                            "article_id": article_id,
                            "attempt": attempt + 1,
                            "repetition_score": state.rep_score,
                            "patterns": state.rep_patterns,
                            "raw_summary_preview": state.raw_summary[
                                :_RAW_SUMMARY_PREVIEW_CHARS
                            ],
                        },
                    )
                    state.last_metadata = self._response_to_metadata(llm_response)
                    continue

                break

        return state

    def _fallback_extract_summary(
        self, article_id: str, prompt: str, raw_summary: str
    ) -> str:
        logger.warning(
            "Cleaned summary is empty, attempting fallback extraction",
            extra={
                "article_id": article_id,
                "raw_summary_length": len(raw_summary) if raw_summary else 0,
                "raw_summary": raw_summary[:_CONTENT_PREVIEW_MAX_CHARS]
                if raw_summary
                else "",
            },
        )

        # Minimal fallback cleaning: strip thinking blocks, remove turn tokens, and sanitize
        fallback_summary = raw_summary
        if fallback_summary:
            fallback_summary = _THINKING_BLOCK_RE.sub("", fallback_summary)
            fallback_summary = (
                fallback_summary.replace("<start_of_turn>", "")
                .replace("<end_of_turn>", "")
                .replace("<|turn>", "")
                .replace("<turn|>", "")
                .replace("<channel|>", "")
                .replace("<|system|>", "")
                .replace("<|user|>", "")
                .replace("<|assistant|>", "")
                .strip()
            )
            fallback_summary = sanitize_output_markdown(fallback_summary).strip()

        if not fallback_summary or not fallback_summary.strip():
            # Check if prompt might have been truncated
            estimated_prompt_tokens = len(prompt) // _CHARS_PER_TOKEN_ESTIMATE
            context_window = self.config.llm_num_ctx
            prompt_truncated = estimated_prompt_tokens > context_window

            error_msg = (
                f"LLM returned an empty summary after cleaning. "
                f"Raw summary length: {len(raw_summary) if raw_summary else 0}, "
                f"Raw preview: {raw_summary[:_RAW_ERROR_PREVIEW_CHARS] if raw_summary else 'None'}"
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
                    "raw_summary": raw_summary[:_CONTENT_PREVIEW_MAX_CHARS]
                    if raw_summary
                    else "",
                    "estimated_prompt_tokens": estimated_prompt_tokens,
                    "context_window": context_window,
                    "prompt_truncated": prompt_truncated,
                },
            )
            raise RuntimeError(error_msg)

        logger.info(
            "Fallback extraction succeeded",
            extra={
                "article_id": article_id,
                "fallback_summary_length": len(fallback_summary),
            },
        )
        return fallback_summary

    def _finalize_generated_summary(
        self,
        article_id: str,
        prompt: str,
        state: _RetryLoopState,
    ) -> tuple[str, SummaryMetadata]:
        max_retries = self.config.max_repetition_retries
        raw_summary = state.raw_summary
        llm_response = state.llm_response

        if state.attempt > 0:
            logger.info(
                "Summary generation succeeded after retry",
                extra={
                    "article_id": article_id,
                    "attempts": state.attempt + 1,
                    "raw_summary_length": len(raw_summary) if raw_summary else 0,
                },
            )

        logger.debug(
            "Raw summary received from LLM",
            extra={
                "article_id": article_id,
                "raw_summary_length": len(raw_summary) if raw_summary else 0,
                "raw_summary_preview": raw_summary[:_RAW_SUMMARY_PREVIEW_CHARS]
                if raw_summary
                else "",
            },
        )

        # If we exhausted retries and still have repetition, log warning but proceed
        if state.has_repetition and state.attempt >= max_retries and llm_response:
            logger.error(
                "Repetition still detected after all retries, using summary anyway",
                extra={
                    "article_id": article_id,
                    "repetition_score": state.rep_score,
                    "patterns": state.rep_patterns,
                    "max_retries": max_retries,
                },
            )

        cleaned_summary = self._clean_summary_text(raw_summary, article_id)

        logger.debug(
            "Cleaned summary after processing",
            extra={
                "article_id": article_id,
                "cleaned_summary_length": len(cleaned_summary)
                if cleaned_summary
                else 0,
                "cleaned_summary_preview": cleaned_summary[:_RAW_SUMMARY_PREVIEW_CHARS]
                if cleaned_summary
                else "",
            },
        )

        if not cleaned_summary:
            cleaned_summary = self._fallback_extract_summary(
                article_id, prompt, raw_summary
            )

        # Enforce 2000 character max (quality priority - allow slight exceed of 1500 target)
        truncated_summary = cleaned_summary[:_MAX_FINAL_SUMMARY_CHARS]

        if llm_response:
            metadata = self._response_to_metadata(llm_response)
        elif state.last_metadata:
            metadata = state.last_metadata
        else:
            metadata = SummaryMetadata(model="unknown")

        logger.info(
            "Summary generated successfully for article: %s",
            article_id,
            extra={
                "article_id": article_id,
                "summary_length": len(truncated_summary),
                "model": metadata.model,
            },
        )

        return truncated_summary, metadata

    async def generate_summary(
        self, article_id: str, content: str, priority: str = "low"
    ) -> tuple[str, SummaryMetadata]:
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
        cleaned_content = _validate_and_clean_summary_input(article_id, content)
        prepared = self._prepare_summary_prompt(article_id, cleaned_content)
        if prepared.needs_hierarchical:
            return await self._generate_hierarchical_summary(
                article_id, cleaned_content, priority=priority
            )
        state = await self._generate_with_repetition_retries(
            article_id, prepared, priority
        )
        return self._finalize_generated_summary(article_id, prepared.prompt, state)

    async def _generate_hierarchical_summary(
        self, article_id: str, content: str, priority: str = "low"
    ) -> tuple[str, SummaryMetadata]:
        """
        Generate a summary for a large article using Hierarchical (Map-Reduce) strategy.

        Uses hold_slot() + generate_raw() (BE path) so DistributingGateway can
        offload chunk and reduce calls to configured remote targets when enabled.

        Args:
            article_id: Article identifier
            content: Full article content
            priority: Request priority ("high" or "low")

        Returns:
            Tuple of (summary text, metadata dict)
        """
        start_time = time.time()
        is_high_priority = priority == "high"

        chunk_size = self.config.hierarchical_single_article_chunk_size
        chunks = [
            content[i : i + chunk_size] for i in range(0, len(content), chunk_size)
        ]

        logger.info(
            "Starting hierarchical summarization (Map-Reduce)",
            extra={
                "article_id": article_id,
                "content_length": len(content),
                "chunk_count": len(chunks),
                "chunk_size": chunk_size,
            },
        )

        # Map Phase: Summarize each chunk via hold_slot + generate_raw (BE path)
        chunk_summaries = []
        total_prompt_tokens = 0
        total_completion_tokens = 0

        for i, chunk in enumerate(chunks):
            logger.info(
                "Map phase: Processing chunk %s/%s",
                i + 1,
                len(chunks),
                extra={"article_id": article_id, "chunk_index": i},
            )

            prompt = CHUNK_SUMMARY_PROMPT_TEMPLATE.format(
                content=sanitize_for_prompt(chunk)
            )

            # Use somewhat higher temperature for extraction to avoid rigid repetition
            llm_options = {
                "temperature": 0.2,
                "repeat_penalty": self.config.llm_repeat_penalty,
            }

            try:
                # Use hold_slot + generate_raw so DistributingGateway can dispatch to remote
                async with self.llm_provider.hold_slot(
                    is_high_priority=is_high_priority
                ) as (
                    _wait_time,
                    cancel_event,
                    task_id,
                ):
                    chunk_resp = await self.llm_provider.generate_raw(
                        prompt,
                        cancel_event=cancel_event,
                        task_id=task_id,
                        num_predict=500,
                        options=llm_options,
                    )
                chunk_text = chunk_resp.response

                # Cleanup chunk summary
                chunk_text = self._clean_summary_text(
                    chunk_text, f"{article_id}-chunk-{i}"
                )

                if chunk_text and chunk_text != "なし":
                    chunk_summaries.append(chunk_text)

                if chunk_resp.prompt_eval_count:
                    total_prompt_tokens += chunk_resp.prompt_eval_count
                if chunk_resp.eval_count:
                    total_completion_tokens += chunk_resp.eval_count

            except PreemptedException:
                raise
            except Exception as e:
                # Best-effort map chunk summary skips failing chunk.
                logger.exception(
                    "Failed to summarize chunk %s",
                    i,
                    extra={"article_id": article_id, "error": str(e)},
                )
                # Continue best effort

        if not chunk_summaries:
            msg = (
                "Hierarchical summarization failed: no valid chunk summaries generated"
            )
            raise RuntimeError(msg)

        # Reduce Phase: Summarize the combined chunk summaries
        combined_text = "\n\n".join(chunk_summaries)

        logger.info(
            "Reduce phase: Summarizing combined content",
            extra={
                "article_id": article_id,
                "combined_length": len(combined_text),
                "reduction_ratio": f"{(1 - len(combined_text) / len(content)) * 100:.1f}%",
            },
        )

        prompt = SUMMARY_PROMPT_TEMPLATE.format(
            content=sanitize_for_prompt(combined_text),
            current_date=datetime.now(timezone(timedelta(hours=9))).strftime(
                "%Y-%m-%d"
            ),
        )

        # Use standard summary configuration
        llm_options = {
            "temperature": self.config.summary_temperature,
            "repeat_penalty": self.config.llm_repeat_penalty,
        }

        # Reduce phase also uses hold_slot + generate_raw (BE path)
        async with self.llm_provider.hold_slot(is_high_priority=is_high_priority) as (
            _wait_time,
            cancel_event,
            task_id,
        ):
            final_resp = await self.llm_provider.generate_raw(
                prompt,
                cancel_event=cancel_event,
                task_id=task_id,
                num_predict=self.config.summary_num_predict,
                options=llm_options,
            )

        raw_summary = final_resp.response
        cleaned_summary = self._clean_summary_text(raw_summary, article_id)

        # Enforce character limit
        truncated_summary = cleaned_summary[:2000]

        elapsed_ms = (time.time() - start_time) * 1000
        total_prompt_tokens += final_resp.prompt_eval_count or 0
        total_completion_tokens += final_resp.eval_count or 0

        metadata = SummaryMetadata(
            model=final_resp.model,
            prompt_tokens=total_prompt_tokens,
            completion_tokens=total_completion_tokens,
            total_duration_ms=elapsed_ms,
            strategy="hierarchical",
        )

        return truncated_summary, metadata

    async def generate_summary_stream(
        self, article_id: str, content: str, priority: str = "low"
    ) -> AsyncGenerator[str]:
        """
        Generate a Japanese summary for an article as a stream of tokens.

        Args:
            article_id: Article identifier
            content: Article content to summarize

        Yields:
            Summary tokens
        """
        prompt = _prepare_stream_prompt(article_id, content)

        # Call LLM provider with streaming
        llm_options = {
            "temperature": self.config.summary_temperature,
            "repeat_penalty": self.config.llm_repeat_penalty,
        }
        metrics = _StreamMetrics()

        try:
            logger.info(
                "Calling LLM provider with streaming",
                extra={
                    "article_id": article_id,
                    "stream": True,
                    "num_predict": self.config.summary_num_predict,
                },
            )

            result = await self.llm_provider.generate(
                prompt,
                num_predict=self.config.summary_num_predict,
                stream=True,
                options=llm_options,
                priority=priority,
            )

            # Narrow union type: streaming returns AsyncIterator
            stream_gen = _ensure_stream_iterator(result)

            logger.info(
                "Stream generator obtained from LLM provider",
                extra={"article_id": article_id},
            )

            try:
                async with AsyncExitStack() as stack:
                    if isinstance(stream_gen, _SupportsAclose):
                        await stack.enter_async_context(aclosing(stream_gen))
                    async for filtered in _consume_summary_stream(
                        stream_gen, article_id, metrics
                    ):
                        yield filtered
            finally:
                logger.info(
                    "Inner stream generator closed (aclose)",
                    extra={"article_id": article_id},
                )

        except aiohttp.ClientConnectionError as conn_err:
            _raise_stream_connection_error(article_id, conn_err, metrics)
        except Exception as e:
            logger.exception(
                "Streaming summary generation failed",
                extra={
                    "article_id": article_id,
                    "error": str(e),
                    "error_type": type(e).__name__,
                    "tokens_yielded": metrics.tokens_yielded,
                    "bytes_yielded": metrics.bytes_yielded,
                    "chunks_received": metrics.chunks_received,
                },
            )
            msg = f"Streaming summary generation failed: {e}"
            raise RuntimeError(msg) from e

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
                    extra={"article_id": article_id},
                )
            return ""

        original_length = len(content)

        # Remove Gemma turn tokens (Gemma 4: <|turn>/<turn|>, legacy Gemma 3: <start_of_turn>/<end_of_turn>)
        cleaned = (
            content.replace("<start_of_turn>", "")
            .replace("<end_of_turn>", "")
            .replace("<|turn>", "")
            .replace("<turn|>", "")
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
                },
            )

        # Strip Gemma 4 thinking blocks (<|channel>thought\n...<channel|> or unterminated)
        cleaned = _THINKING_BLOCK_RE.sub("", cleaned)

        # Remove markdown code blocks (```...```)
        # Remove code blocks with triple backticks
        cleaned = re.sub(r"```[^`]*```", "", cleaned, flags=re.DOTALL)
        # Remove standalone triple backticks
        cleaned = re.sub(r"```+", "", cleaned)
        # Remove any remaining backticks
        cleaned = cleaned.replace("`", "")

        # Remove excessive whitespace and special characters
        # Replace multiple spaces/tabs with single space
        cleaned = re.sub(r"[ \t]+", " ", cleaned)
        # Remove excessive newlines
        cleaned = re.sub(r"\n{3,}", "\n\n", cleaned)

        # Process line by line
        lines = cleaned.splitlines()
        final_lines: list[str] = []

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
        result = re.sub(r"\b(\w+)(-\1){2,}\b", "", result, flags=re.IGNORECASE)

        # Log if final cleanup removed significant content
        if len(result) < len(before_final) * 0.9:  # More than 10% removed
            logger.debug(
                "Final cleanup removed significant content",
                extra={
                    "article_id": article_id,
                    "before_final_length": len(before_final),
                    "after_final_length": len(result),
                },
            )

        # Warn if result is empty after all cleaning
        if not result and original_length > 0:
            logger.warning(
                "Summary became empty after cleaning",
                extra={
                    "article_id": article_id,
                    "original_length": original_length,
                    "original_preview": content[:200],
                },
            )

        return sanitize_output_markdown(result)

    @staticmethod
    def _nanoseconds_to_milliseconds(value: int | None) -> float | None:
        """Convert nanoseconds to milliseconds."""
        if value is None:
            return None
        try:
            return value / 1_000_000
        except TypeError:
            return None
