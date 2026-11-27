"""Ollama Gateway - implements LLMProviderPort."""

import asyncio
import logging
import uuid
import weakref
from collections.abc import AsyncGenerator, AsyncIterator
from contextlib import aclosing, asynccontextmanager, suppress
from dataclasses import dataclass
from typing import Any

from news_creator.config.config import NewsCreatorConfig
from news_creator.domain.errors import PreemptedException
from news_creator.domain.models import LLMGenerateResponse
from news_creator.driver.ollama_driver import OllamaDriver
from news_creator.driver.ollama_stream_driver import (
    _RUNNER_STARTUP_PARAMS,
    OllamaStreamDriver,
)
from news_creator.gateway.hybrid_priority_semaphore import HybridPrioritySemaphore
from news_creator.gateway.model_router import ModelRouter
from news_creator.gateway.oom_detector import OOMDetector
from news_creator.gateway.payload_builder import prepare_raw_prompt
from news_creator.port.llm_provider_port import LLMProviderPort

logger = logging.getLogger(__name__)
_LARGE_UNKNOWN_MODEL_TOKEN_THRESHOLD = 16000
_ABNORMAL_PROMPT_PREVIEW_MIN_CHARS = 1000
_ABNORMAL_PROMPT_THRESHOLD_CHARS = 100_000
_COLD_START_THRESHOLD_SECONDS = 0.1
_SLOW_GENERATION_THRESHOLD_TPS = 30
_MIN_EVAL_COUNT_FOR_SPEED_WARNING = 20
_NANOSECONDS_PER_SECOND = 1_000_000_000


def _nanos_to_seconds(duration_ns: int | None) -> float | None:
    return round(duration_ns / _NANOSECONDS_PER_SECOND, 2) if duration_ns else None


def _tokens_per_second(count: int | None, duration_ns: int | None) -> float | None:
    return (
        round(count / (duration_ns / _NANOSECONDS_PER_SECOND), 2)
        if count and duration_ns
        else None
    )


@dataclass(frozen=True)
class _OomRetryContext:
    prompt: str
    model: str | None
    num_predict: int | None
    cancel_event: asyncio.Event | None
    task_id: str | None


class OllamaGateway(LLMProviderPort):
    """Gateway for Ollama LLM service - Anti-Corruption Layer."""

    def __init__(self, config: NewsCreatorConfig) -> None:
        """Initialize Ollama gateway."""
        self.config = config
        self.driver = OllamaDriver(config)
        self.stream_driver = OllamaStreamDriver(config)
        # Hybrid RT/BE semaphore for controlling concurrent requests to Ollama
        # RT (streaming) requests get reserved slots; BE (batch) uses remaining slots
        # Aging mechanism prevents starvation of low-priority batch requests
        # Priority promotion: BE is promoted to RT after long wait (prevents starvation)
        # Guaranteed bandwidth: BE gets guaranteed slot after N consecutive RT releases
        # Preemption allows RT to interrupt long-running BE requests
        self._semaphore = HybridPrioritySemaphore(
            total_slots=config.ollama_request_concurrency,
            rt_reserved_slots=config.scheduling_rt_reserved_slots,
            aging_threshold_seconds=config.scheduling_aging_threshold_seconds,
            aging_boost=config.scheduling_aging_boost,
            preemption_enabled=config.scheduling_preemption_enabled,
            preemption_wait_threshold=config.scheduling_preemption_wait_threshold_seconds,
            priority_promotion_threshold_seconds=config.scheduling_priority_promotion_threshold_seconds,
            guaranteed_be_ratio=config.scheduling_guaranteed_be_ratio,
            max_queue_depth=config.max_queue_depth,
            rt_scheduling_mode=config.scheduling_rt_mode,
            aging_sweep_interval_releases=config.scheduling_aging_sweep_interval_releases,
        )
        # OOM detector and model router
        self.oom_detector = OOMDetector(enabled=config.oom_detection_enabled)
        self.model_router = ModelRouter(config, self.oom_detector)

    async def initialize(self) -> None:
        """Initialize the Ollama drivers."""
        await self.driver.initialize()
        await self.stream_driver.initialize()
        logger.info("Ollama gateway initialized")

    async def cleanup(self) -> None:
        """Cleanup Ollama driver resources."""
        await self.driver.cleanup()
        await self.stream_driver.cleanup()
        logger.info("Ollama gateway cleaned up")

    async def generate(  # noqa: PLR0913 - implements LLMProviderPort interface
        self,
        prompt: str,
        *,
        model: str | None = None,
        num_predict: int | None = None,
        stream: bool = False,
        keep_alive: int | str | None = None,
        format: str | dict[str, Any] | None = None,  # noqa: A002 - implements LLMProviderPort keyword API
        options: dict[str, Any] | None = None,
        priority: str = "low",
    ) -> LLMGenerateResponse | AsyncIterator[LLMGenerateResponse]:
        """
        Generate text using Ollama.

        Args:
            prompt: Input prompt
            model: Optional model name override
            num_predict: Optional max tokens override
            stream: Whether to stream response
            keep_alive: Keep-alive duration
            format: Optional output format (e.g., "json" for structured output)
            options: Additional generation options
            priority: Request priority ("high" or "low"). High priority requests
                      (or streaming requests) bypass the low priority queue.

        Returns:
            LLMGenerateResponse with generated text

        Raises:
            ValueError: If prompt is empty
            RuntimeError: If Ollama service fails
        """
        prepared_prompt = prepare_raw_prompt(prompt)
        model = self._resolve_model(prompt, model, num_predict)
        llm_options = self._build_llm_options(options, num_predict)
        final_keep_alive = self._resolve_keep_alive(model, keep_alive)

        # Build payload for Ollama API
        # raw=True: prompts already contain <|turn>/<turn|> (Gemma 4) chat template.
        # This bypasses Ollama's built-in RENDERER/PARSER which enables Gemma4 thinking by default.
        payload: dict[str, Any] = {
            "model": model,
            "prompt": prepared_prompt,
            "stream": stream,
            "raw": True,
            "keep_alive": final_keep_alive,
            "options": llm_options,
        }

        # Add format parameter if provided (Ollama structured output)
        if format is not None:
            payload["format"] = format
            logger.debug("Using structured output format", extra={"format": format})

        prompt_length, context_window = self._log_generate_request_info(prompt, payload)

        # Determine if this request should be HIGH PRIORITY
        # HIGH PRIORITY: streaming OR explicit priority="high"
        is_high_priority = stream or priority == "high"

        # Handle streaming requests
        if stream:
            # EAGER SEMAPHORE ACQUISITION (TTFT optimization)
            # Acquire semaphore BEFORE returning the generator to ensure:
            # 1. RT requests get priority slots immediately (not delayed until iteration)
            # 2. ADR 140's Hybrid RT/BE Priority Semaphore works as designed
            # 3. Predictable queue ordering for streaming requests
            wait_time, acquired_slot_id = await self._semaphore.acquire(
                high_priority=is_high_priority
            )
            priority_label = "HIGH PRIORITY" if is_high_priority else "LOW PRIORITY"
            logger.info(
                "Acquired semaphore (%s), returning streaming generator",
                priority_label,
                extra={
                    "model": payload["model"],
                    "prompt_length": len(prompt),
                    "stream": True,
                    "priority": priority,
                    "is_high_priority": is_high_priority,
                    "queue_wait_time_seconds": round(wait_time, 4),
                    "slot_id": acquired_slot_id,
                },
            )
            return self._build_streaming_generator(
                payload, acquired_slot_id, is_high_priority
            )

        # Handle non-streaming requests
        # For BE requests, create a cancel event for preemption support
        task_id = str(uuid.uuid4()) if not is_high_priority else None
        cancel_event = asyncio.Event() if not is_high_priority else None

        # Acquire semaphore based on priority
        wait_time, acquired_slot_id = await self._semaphore.acquire(
            high_priority=is_high_priority
        )
        priority_label = "HIGH PRIORITY" if is_high_priority else "LOW PRIORITY"
        try:
            # Register BE request as active for preemption tracking
            if task_id and cancel_event:
                self._semaphore.register_active_request(
                    task_id, cancel_event, is_high_priority
                )

            logger.info(
                "Acquired semaphore (%s), processing Ollama request (non-streaming)",
                priority_label,
                extra={
                    "model": payload["model"],
                    "prompt_length": len(prompt),
                    "stream": False,
                    "priority": priority,
                    "is_high_priority": is_high_priority,
                    "queue_wait_time_seconds": round(wait_time, 4),
                    "task_id": task_id,
                    "slot_id": acquired_slot_id,
                },
            )
            # Call appropriate driver based on stream flag
            response_data = await self._generate_with_oom_retry(
                payload,
                _OomRetryContext(
                    prompt=prompt,
                    model=model,
                    num_predict=num_predict,
                    cancel_event=cancel_event,
                    task_id=task_id,
                ),
            )
        finally:
            # Unregister BE request from active tracking
            if task_id:
                self._semaphore.unregister_active_request(task_id)
            self._semaphore.release(
                slot_id=acquired_slot_id, was_high_priority=is_high_priority
            )

        return self._build_generate_response(
            response_data, payload, prompt_length, context_window
        )

    def _resolve_model(
        self, prompt: str, model: str | None, num_predict: int | None
    ) -> str:
        """Select model using router when enabled, or fall back to explicit/default model."""
        # - If routing enabled and model is None: use router
        # - If routing enabled and model is base model name (e.g., "gemma4-e4b-q4km"): use router to auto-map
        # - If routing enabled and model is bucket model name (e.g., "gemma4-e4b-12k"): use as-is
        # - If routing disabled: use provided model or default
        if not self.config.model_routing_enabled:
            resolved = model or self.config.model_name
            logger.debug(
                "Model routing disabled, using: %s",
                resolved,
                extra={"model": resolved},
            )
            return resolved

        if model is None:
            selected_model, bucket_size = self.model_router.select_model(
                prompt, max_new_tokens=num_predict
            )
            logger.debug(
                "Model router selected: %s (bucket: %s)",
                selected_model,
                bucket_size,
                extra={"model": selected_model, "bucket": bucket_size},
            )
            return selected_model

        if self.config.is_base_model_name(model):
            selected_model, bucket_size = self.model_router.select_model(
                prompt, max_new_tokens=num_predict
            )
            logger.info(
                "Base model name '%s' auto-mapped to bucket model: %s (bucket: %s)",
                model,
                selected_model,
                bucket_size,
                extra={
                    "original_model": model,
                    "mapped_model": selected_model,
                    "bucket": bucket_size,
                },
            )
            return selected_model

        if self.config.is_bucket_model_name(model):
            logger.debug(
                "Bucket model explicitly specified, using as-is: %s",
                model,
                extra={"model": model, "prompt_length": len(prompt)},
            )
            return model

        prompt_length = len(prompt)
        estimated_tokens = prompt_length // 4
        if estimated_tokens > _LARGE_UNKNOWN_MODEL_TOKEN_THRESHOLD:  # >16K tokens
            logger.warning(
                "Large prompt detected with unknown model: %s. "
                "Prompt length: %s chars, estimated tokens: %s. "
                "Consider using base model name or bucket model name for automatic routing.",
                model,
                prompt_length,
                estimated_tokens,
                extra={
                    "model": model,
                    "prompt_length": prompt_length,
                    "estimated_tokens": estimated_tokens,
                },
            )
        logger.debug(
            "Unknown model name specified, using as-is: %s",
            model,
            extra={"model": model, "prompt_length": prompt_length},
        )
        return model

    def _build_llm_options(
        self, options: dict[str, Any] | None, num_predict: int | None
    ) -> dict[str, Any]:
        """Build Ollama options from config, stripping runner startup parameters."""
        llm_options = self.config.get_llm_options()
        if options:
            # CRITICAL: Strip runner startup params (num_ctx/num_batch/num_keep)
            # from caller options. They are fixed by config/Modelfile; a caller
            # value that differs from the base triggers an Ollama runner reload
            # (GPU contention), mirroring OllamaStreamDriver._merge_options.
            options_filtered = {
                k: v for k, v in options.items() if k not in _RUNNER_STARTUP_PARAMS
            }
            llm_options.update(options_filtered)

        if num_predict is not None:
            llm_options["num_predict"] = num_predict
        return llm_options

    def _resolve_keep_alive(
        self, model: str, keep_alive: int | str | None
    ) -> int | str:
        """Resolve keep_alive duration from explicit override or model-specific default."""
        if keep_alive is not None:
            return keep_alive
        # Use model-specific keep_alive based on best practices
        # 16K model: 24h (keep in GPU memory)
        final_keep_alive = self.config.get_keep_alive_for_model(model)
        logger.debug(
            "Using model-specific keep_alive: %s for model: %s",
            final_keep_alive,
            model,
            extra={"model": model, "keep_alive": final_keep_alive},
        )
        return final_keep_alive

    def _log_generate_request_info(
        self, prompt: str, payload: dict[str, Any]
    ) -> tuple[int, int]:
        """Validate prompt length, log request metrics, and return (prompt_length, context_window)."""
        prompt_length = len(prompt)
        estimated_tokens = prompt_length // 4  # Rough estimate: 1 token ≈ 4 chars
        # Context window is now fixed in Modelfile, so we don't include it in options
        # Use bucket size from router if available, otherwise use config default
        context_window = (
            self.model_router.current_bucket
            if self.model_router.current_bucket
            else self.config.llm_num_ctx
        )

        # Validate prompt size - detect abnormal amplification
        if prompt_length > _ABNORMAL_PROMPT_THRESHOLD_CHARS:
            logger.error(
                "ABNORMAL PROMPT SIZE DETECTED in ollama_gateway",
                extra={
                    "prompt_length": prompt_length,
                    "estimated_tokens": estimated_tokens,
                    "context_window": context_window,
                    "model": payload["model"],
                    "prompt_preview_start": prompt[:500],
                    "prompt_preview_end": prompt[-500:]
                    if prompt_length > _ABNORMAL_PROMPT_PREVIEW_MIN_CHARS
                    else "",
                    "num_predict": payload["options"].get("num_predict"),
                },
            )

        # Log model loading strategy (16K/60K on-demand)
        model_loading_strategy = "on-demand"
        logger.info(
            "Generating with Ollama: model=%s, loading_strategy=%s, "
            "prompt_length=%s chars, estimated_tokens=%s, "
            "num_predict=%s, context_window=%s, "
            "usage_percent=%s%%",
            payload["model"],
            model_loading_strategy,
            prompt_length,
            estimated_tokens,
            payload["options"].get("num_predict"),
            context_window,
            round((estimated_tokens / context_window) * 100, 1)
            if context_window > 0
            else 0,
        )
        return prompt_length, context_window

    def _build_streaming_generator(
        self,
        payload: dict[str, Any],
        acquired_slot_id: int,
        is_high_priority: bool,
    ) -> AsyncIterator[LLMGenerateResponse]:
        """Create the streaming response generator with deterministic and GC-backed slot release."""
        released = False

        def _release_slot() -> None:
            nonlocal released
            if released:
                return
            released = True
            self._semaphore.release(
                slot_id=acquired_slot_id, was_high_priority=is_high_priority
            )
            logger.info(
                "Released semaphore after streaming",
                extra={
                    "model": payload["model"],
                    "is_high_priority": is_high_priority,
                    "slot_id": acquired_slot_id,
                },
            )

        async def response_generator() -> AsyncIterator[LLMGenerateResponse]:
            try:
                logger.debug(
                    "Starting stream iteration (semaphore pre-acquired)",
                    extra={
                        "model": payload["model"],
                        "is_high_priority": is_high_priority,
                    },
                )
                stream_iterator = self.stream_driver.generate_stream(payload)
                async with aclosing(  # pyrefly: ignore[bad-specialization]
                    stream_iterator
                ):
                    async for chunk in stream_iterator:
                        yield LLMGenerateResponse(
                            response=chunk.get("response", ""),
                            model=str(chunk.get("model", payload["model"])),
                            done=chunk.get("done", False),
                            done_reason=chunk.get("done_reason"),
                            prompt_eval_count=chunk.get("prompt_eval_count"),
                            eval_count=chunk.get("eval_count"),
                            total_duration=chunk.get("total_duration"),
                            load_duration=chunk.get("load_duration"),
                            prompt_eval_duration=chunk.get("prompt_eval_duration"),
                            eval_duration=chunk.get("eval_duration"),
                        )
            except GeneratorExit:
                logger.info(
                    "Stream generator closed by client disconnect or explicit aclose()",
                    extra={
                        "model": payload["model"],
                        "is_high_priority": is_high_priority,
                    },
                )
                raise
            except Exception:
                logger.exception(
                    "Stream generator error",
                    extra={
                        "model": payload["model"],
                        "is_high_priority": is_high_priority,
                    },
                )
                raise
            finally:
                _release_slot()

        gen = response_generator()
        # An async generator's own `try/finally` never runs if the caller
        # discards it without ever calling __anext__() -- the function
        # body is never entered, so nothing is there to clean up. Guard
        # against that leaking this slot for the full leak-check window
        # by also releasing when `gen` itself is garbage collected.
        weakref.finalize(gen, _release_slot)
        return gen

    def _build_generate_response(
        self,
        response_data: dict[str, Any],
        payload: dict[str, Any],
        prompt_length: int,
        context_window: int,
    ) -> LLMGenerateResponse:
        """Validate non-streaming response, log TTFT/performance metrics, and map to domain model."""
        if "response" not in response_data:
            logger.error(
                "Ollama response missing 'response' field",
                extra={"keys": list(response_data.keys())},
            )
            msg = "Invalid Ollama response format"
            raise RuntimeError(msg)

        prompt_eval_count = response_data.get("prompt_eval_count")
        eval_count = response_data.get("eval_count")
        total_duration = response_data.get("total_duration")
        load_duration = response_data.get("load_duration")
        prompt_eval_duration = response_data.get("prompt_eval_duration")
        eval_duration = response_data.get("eval_duration")
        requested_model = payload["model"]
        actual_model = str(response_data.get("model", requested_model))

        if actual_model != requested_model:
            logger.warning(
                "Model mismatch detected: requested=%s, actual=%s. "
                "This may indicate that Ollama used a different model than requested, "
                "possibly due to model loading strategy or OLLAMA_MAX_LOADED_MODELS setting.",
                requested_model,
                actual_model,
                extra={
                    "requested_model": requested_model,
                    "actual_model": actual_model,
                    "prompt_length": prompt_length,
                },
            )
        else:
            logger.debug(
                "Model match confirmed: %s",
                requested_model,
                extra={"model": requested_model},
            )

        load_duration_seconds = _nanos_to_seconds(load_duration)
        prompt_eval_duration_seconds = _nanos_to_seconds(prompt_eval_duration)
        eval_duration_seconds = _nanos_to_seconds(eval_duration)
        decode_tokens_per_second = _tokens_per_second(eval_count, eval_duration)

        queue_wait_time_seconds = round(self._semaphore.last_wait_time, 4)
        ttft_seconds = queue_wait_time_seconds
        if load_duration_seconds:
            ttft_seconds += load_duration_seconds
        if prompt_eval_duration_seconds:
            ttft_seconds += prompt_eval_duration_seconds
        ttft_seconds = round(ttft_seconds, 2)

        logger.debug(
            "TTFT breakdown: ttft=%ss "
            "(queue_wait=%ss + "
            "load_duration=%ss + "
            "prompt_eval_duration=%ss), "
            "model=%s, prompt_tokens=%s",
            ttft_seconds,
            queue_wait_time_seconds,
            load_duration_seconds,
            prompt_eval_duration_seconds,
            actual_model,
            prompt_eval_count,
            extra={
                "ttft_seconds": ttft_seconds,
                "queue_wait_time_seconds": queue_wait_time_seconds,
                "load_duration_seconds": load_duration_seconds,
                "prompt_eval_duration_seconds": prompt_eval_duration_seconds,
                "model": actual_model,
                "prompt_eval_count": prompt_eval_count,
            },
        )

        if (
            load_duration_seconds
            and load_duration_seconds > _COLD_START_THRESHOLD_SECONDS
        ):
            logger.warning(
                "COLD_START detected: load_duration=%ss > %ss. "
                "Model '%s' was not in VRAM and required disk-to-VRAM loading. "
                "This adds %ss to TTFT. "
                "Consider: (1) increasing OLLAMA_MAX_LOADED_MODELS, "
                "(2) extending keep_alive duration, or "
                "(3) ensuring warmup runs on startup.",
                load_duration_seconds,
                _COLD_START_THRESHOLD_SECONDS,
                actual_model,
                load_duration_seconds,
                extra={
                    "load_duration_seconds": load_duration_seconds,
                    "cold_start_threshold": _COLD_START_THRESHOLD_SECONDS,
                    "model": actual_model,
                    "ttft_seconds": ttft_seconds,
                },
            )

        logger.debug(
            "Ollama generation completed: requested_model=%s, actual_model=%s, "
            "prompt_length=%s chars, prompt_eval_count=%s tokens, "
            "eval_count=%s tokens, num_predict=%s, "
            "duration=%ss, tokens_per_second=%s, "
            "load_duration=%ss, prompt_eval_duration=%ss "
            "(prefill: %s tok/s), eval_duration=%ss "
            "(decode: %s tok/s)",
            requested_model,
            actual_model,
            prompt_length,
            prompt_eval_count,
            eval_count,
            payload["options"].get("num_predict"),
            _nanos_to_seconds(total_duration),
            _tokens_per_second(eval_count, total_duration),
            load_duration_seconds,
            prompt_eval_duration_seconds,
            _tokens_per_second(prompt_eval_count, prompt_eval_duration),
            eval_duration_seconds,
            decode_tokens_per_second,
        )

        is_slow_decode = (
            decode_tokens_per_second is not None
            and decode_tokens_per_second < _SLOW_GENERATION_THRESHOLD_TPS
            and eval_count is not None
            and eval_count >= _MIN_EVAL_COUNT_FOR_SPEED_WARNING
        )
        if is_slow_decode:
            estimated_tokens = prompt_length // 4
            warning_msg = (
                f"Slow LLM generation detected: decode_speed={decode_tokens_per_second} tok/s "
                f"(expected: 50-100 for RTX 4060), eval_count={eval_count}, "
                f"eval_duration={eval_duration_seconds}s, "
                f"prompt_eval_count={prompt_eval_count}, "
                f"prompt_length={prompt_length} chars, estimated_tokens={estimated_tokens}, "
                f"context_window={context_window}, requested_model={requested_model}, "
                f"actual_model={actual_model}. "
                f"Low decode speed detected. "
                f"Possible causes: VRAM bandwidth bottleneck, suboptimal batch size, "
                f"or model loading issues. Consider checking OLLAMA_NUM_BATCH and "
                f"OLLAMA_MAX_LOADED_MODELS settings."
            )
            logger.warning(
                warning_msg,
                extra={
                    "decode_tokens_per_second": decode_tokens_per_second,
                    "eval_duration_seconds": eval_duration_seconds,
                    "prompt_eval_count": prompt_eval_count,
                    "eval_count": eval_count,
                    "prompt_length": prompt_length,
                    "estimated_tokens": estimated_tokens,
                    "context_window": context_window,
                    "requested_model": requested_model,
                    "actual_model": actual_model,
                },
            )

        return LLMGenerateResponse(
            response=response_data.get("response", ""),
            model=actual_model,
            done=response_data.get("done"),
            done_reason=response_data.get("done_reason"),
            prompt_eval_count=prompt_eval_count,
            eval_count=eval_count,
            total_duration=total_duration,
            load_duration=load_duration,
            prompt_eval_duration=prompt_eval_duration,
            eval_duration=eval_duration,
        )

    @asynccontextmanager
    async def hold_slot(
        self, is_high_priority: bool = False
    ) -> AsyncIterator[tuple[float, asyncio.Event | None, str | None]]:
        """Hold a semaphore slot for the duration of the context.

        Use this when you need to make multiple LLM calls (e.g., retries)
        while holding the same slot, avoiding re-queuing between retries.

        Args:
            is_high_priority: Whether to use high priority queue

        Yields:
            Tuple of (wait_time, cancel_event, task_id)
        """
        task_id = str(uuid.uuid4()) if not is_high_priority else None
        cancel_event = asyncio.Event() if not is_high_priority else None

        wait_time, acquired_slot_id = await self._semaphore.acquire(
            high_priority=is_high_priority
        )
        if task_id and cancel_event:
            self._semaphore.register_active_request(
                task_id, cancel_event, is_high_priority
            )
        try:
            yield wait_time, cancel_event, task_id
        finally:
            if task_id:
                self._semaphore.unregister_active_request(task_id)
            self._semaphore.release(
                slot_id=acquired_slot_id, was_high_priority=is_high_priority
            )

    async def generate_raw(  # noqa: PLR0913 - implements LLMProviderPort interface
        self,
        prompt: str,
        *,
        cancel_event: asyncio.Event | None = None,
        task_id: str | None = None,
        model: str | None = None,
        num_predict: int | None = None,
        keep_alive: int | str | None = None,
        format: str | dict[str, Any] | None = None,  # noqa: A002 - implements LLMProviderPort keyword API
        options: dict[str, Any] | None = None,
    ) -> LLMGenerateResponse:
        """Generate text without acquiring semaphore (for use inside hold_slot).

        This performs model routing, payload construction, OOM detection, and
        TTFT logging, but skips semaphore acquisition.

        Args:
            prompt: Input prompt
            cancel_event: Optional cancel event for preemption
            task_id: Optional task ID for tracking
            model: Optional model name override
            num_predict: Optional max tokens override
            keep_alive: Keep-alive duration
            format: Optional output format
            options: Additional generation options

        Returns:
            LLMGenerateResponse with generated text
        """
        prepared_prompt = prepare_raw_prompt(prompt)

        # Model routing
        if self.config.model_routing_enabled:
            if model is None or self.config.is_base_model_name(model):
                selected_model, _ = self.model_router.select_model(
                    prompt, max_new_tokens=num_predict
                )
                model = selected_model
        else:
            model = model or self.config.model_name

        # Build options
        llm_options = self.config.get_llm_options()
        if options:
            options_filtered = {
                k: v for k, v in options.items() if k not in _RUNNER_STARTUP_PARAMS
            }
            llm_options.update(options_filtered)
        if num_predict is not None:
            llm_options["num_predict"] = num_predict

        # Keep-alive
        if keep_alive is not None:
            final_keep_alive = keep_alive
        else:
            final_keep_alive = self.config.get_keep_alive_for_model(model)

        payload: dict[str, Any] = {
            "model": model,
            "prompt": prepared_prompt,
            "stream": False,
            "raw": True,
            "keep_alive": final_keep_alive,
            "options": llm_options,
        }
        if format is not None:
            payload["format"] = format

        response_data = await self._generate_with_oom_retry(
            payload,
            _OomRetryContext(
                prompt=prompt,
                model=model,
                num_predict=num_predict,
                cancel_event=cancel_event,
                task_id=task_id,
            ),
        )

        if "response" not in response_data:
            msg = "Invalid Ollama response format"
            raise RuntimeError(msg)

        return LLMGenerateResponse(
            response=response_data.get("response", ""),
            model=str(response_data.get("model", payload["model"])),
            done=response_data.get("done"),
            done_reason=response_data.get("done_reason"),
            prompt_eval_count=response_data.get("prompt_eval_count"),
            eval_count=response_data.get("eval_count"),
            total_duration=response_data.get("total_duration"),
            load_duration=response_data.get("load_duration"),
            prompt_eval_duration=response_data.get("prompt_eval_duration"),
            eval_duration=response_data.get("eval_duration"),
        )

    async def _generate_with_oom_retry(
        self,
        payload: dict[str, Any],
        ctx: _OomRetryContext,
    ) -> dict[str, Any]:
        try:
            response_data = await self._generate_with_cancellation(
                payload, ctx.cancel_event, ctx.task_id
            )
            if self.oom_detector.detect_oom_from_response(response_data):
                logger.warning(
                    "OOM detected in response, retrying with 2-model mode",
                    extra={"original_model": ctx.model},
                )
                selected_model, _ = self.model_router.select_model(
                    ctx.prompt, max_new_tokens=ctx.num_predict
                )
                if selected_model != ctx.model:
                    payload["model"] = selected_model
                    logger.info(
                        "Retrying with model %s (2-model mode)",
                        selected_model,
                        extra={
                            "original_model": ctx.model,
                            "new_model": selected_model,
                        },
                    )
                    response_data = await self._generate_with_cancellation(
                        payload, ctx.cancel_event, ctx.task_id
                    )
        except PreemptedException:
            logger.info(
                "Request %s preempted, will retry later",
                ctx.task_id,
                extra={"task_id": ctx.task_id, "model": payload["model"]},
            )
            raise
        except Exception as e:
            if self.oom_detector.detect_oom_from_exception(e):
                logger.warning(
                    "OOM detected from exception, retrying with 2-model mode",
                    extra={"original_model": ctx.model, "error": str(e)},
                )
                selected_model, _ = self.model_router.select_model(
                    ctx.prompt, max_new_tokens=ctx.num_predict
                )
                if selected_model != ctx.model:
                    payload["model"] = selected_model
                    logger.info(
                        "Retrying with model %s (2-model mode)",
                        selected_model,
                        extra={
                            "original_model": ctx.model,
                            "new_model": selected_model,
                        },
                    )
                    payload["stream"] = False
                    return await self._generate_with_cancellation(
                        payload, ctx.cancel_event, ctx.task_id
                    )
                raise
            raise
        else:
            return response_data

    async def _generate_with_cancellation(
        self,
        payload: dict[str, Any],
        cancel_event: asyncio.Event | None,
        task_id: str | None,
    ) -> dict[str, Any]:
        """
        Generate with cancellation support for preemption.

        For BE requests, this races driver.generate() against cancel_event.wait()
        using asyncio.wait(FIRST_COMPLETED). When preemption is signaled,
        the in-flight HTTP request to Ollama is cancelled via task.cancel(),
        which closes the aiohttp connection and causes Ollama to stop inference.

        For RT requests (cancel_event is None), this is a direct passthrough.

        Args:
            payload: Ollama API payload
            cancel_event: Event signaling preemption (None for RT requests)
            task_id: Task identifier for logging

        Returns:
            Ollama response dictionary

        Raises:
            PreemptedException: If the request is preempted
        """
        if cancel_event is None:
            # RT request - no cancellation check needed
            return await self.driver.generate(payload)

        # BE request - check for preemption before generation
        if cancel_event.is_set():
            msg = f"Request {task_id} preempted before generation"
            raise PreemptedException(msg)

        # Race generate() against cancel_event.wait() so preemption
        # can interrupt an in-flight Ollama HTTP request.
        generate_task = asyncio.create_task(self.driver.generate(payload))
        cancel_task = asyncio.create_task(cancel_event.wait())

        try:
            done, _pending = await asyncio.wait(
                {generate_task, cancel_task},
                return_when=asyncio.FIRST_COMPLETED,
            )

            if cancel_task in done:
                # Preempted — cancel the HTTP request.
                # task.cancel() raises CancelledError inside aiohttp's session.post(),
                # which closes the HTTP connection. Ollama detects the disconnect
                # and stops inference, freeing GPU resources immediately.
                generate_task.cancel()
                with suppress(asyncio.CancelledError):
                    await generate_task
                logger.warning(
                    "Request %s preempted during generation — HTTP connection cancelled",
                    task_id,
                    extra={"task_id": task_id},
                )
                msg = f"Request {task_id} preempted during generation"
                raise PreemptedException(msg)

            return generate_task.result()
        finally:
            for task in (generate_task, cancel_task):
                if not task.done():
                    task.cancel()
            await asyncio.gather(generate_task, cancel_task, return_exceptions=True)

    def queue_status(self) -> dict[str, Any]:
        """Current semaphore queue depth/availability for monitoring."""
        return self._semaphore.queue_status()

    async def list_models(self) -> list[dict[str, Any]]:
        """
        List available Ollama models.

        Returns:
            List of model dictionaries with name and metadata

        Raises:
            RuntimeError: If Ollama service fails
        """
        try:
            tags_response = await self.driver.list_tags()
            models = tags_response.get("models", [])
            logger.debug(
                "Found %s models in Ollama", len(models), extra={"count": len(models)}
            )
        except Exception as err:
            logger.exception("Failed to list Ollama models", extra={"error": str(err)})
            msg = f"Failed to list Ollama models: {err}"
            raise RuntimeError(msg) from err
        else:
            return models

    async def chat_stream(
        self,
        payload: dict[str, Any],
    ) -> AsyncGenerator[dict[str, Any]]:
        """Stream Ollama /api/chat through semaphore with HIGH priority.

        This is a transparent proxy for Ask Augur chat requests. It acquires
        a HIGH PRIORITY semaphore slot, forwards the request to Ollama's
        /api/chat endpoint, and yields raw NDJSON chunks.

        Args:
            payload: Ollama chat request payload (messages, model, options, etc.)

        Yields:
            Response chunk dicts from Ollama stream

        Raises:
            QueueFullError: If semaphore queue is at capacity
            RuntimeError: If Ollama returns an error
        """
        wait_time, acquired_slot_id = await self._semaphore.acquire(high_priority=True)
        logger.info(
            "Acquired semaphore (HIGH PRIORITY) for chat stream proxy",
            extra={
                "model": payload.get("model", "unknown"),
                "message_count": len(payload.get("messages", [])),
                "queue_wait_time_seconds": round(wait_time, 4),
                "slot_id": acquired_slot_id,
            },
        )

        async def _stream() -> AsyncGenerator[dict[str, Any]]:
            try:
                stream_iterator = self.stream_driver.chat_stream(payload)
                async with aclosing(stream_iterator):
                    async for chunk in stream_iterator:
                        yield chunk
            except GeneratorExit:
                logger.info("Chat stream closed by client disconnect")
                raise
            except Exception:
                logger.exception("Chat stream error")
                raise
            finally:
                _release_slot()

        released = False

        def _release_slot() -> None:
            nonlocal released
            if released:
                return
            released = True
            self._semaphore.release(slot_id=acquired_slot_id, was_high_priority=True)
            logger.info("Released semaphore after chat stream")

        gen = _stream()
        # See generate()'s response_generator for why this is needed: an
        # async generator's own finally never runs if it's discarded before
        # ever being iterated.
        weakref.finalize(gen, _release_slot)
        return gen

    async def chat_generate(
        self,
        payload: dict[str, Any],
        *,
        priority: str = "high",
    ) -> dict[str, Any]:
        """Non-streaming Ollama /api/chat through the priority semaphore.

        Used by morning letter, plan-query and other non-streaming chat
        requests. Defaults to HIGH priority (interactive callers); batch
        callers (e.g. recap) must pass priority="low" so they don't consume
        RT bandwidth reserved for Ask Augur chat.

        Args:
            payload: Ollama chat request payload (messages, model, options, etc.)
            priority: "high" (default) or "low"

        Returns:
            Response dict in /api/chat format

        Raises:
            QueueFullError: If semaphore queue is at capacity
            RuntimeError: If Ollama returns an error
        """
        is_high_priority = priority == "high"
        wait_time, acquired_slot_id = await self._semaphore.acquire(
            high_priority=is_high_priority
        )
        logger.info(
            "Acquired semaphore (%s PRIORITY) for chat generate proxy",
            "HIGH" if is_high_priority else "LOW",
            extra={
                "model": payload.get("model", "unknown"),
                "message_count": len(payload.get("messages", [])),
                "queue_wait_time_seconds": round(wait_time, 4),
                "slot_id": acquired_slot_id,
                "priority": priority,
            },
        )

        try:
            return await self.stream_driver.chat_generate(payload)
        except Exception:
            logger.exception("Chat generate error")
            raise
        finally:
            self._semaphore.release(
                slot_id=acquired_slot_id, was_high_priority=is_high_priority
            )
            logger.info("Released semaphore after chat generate")
