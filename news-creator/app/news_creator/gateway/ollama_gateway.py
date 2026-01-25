"""Ollama Gateway - implements LLMProviderPort."""

import logging
from typing import Dict, Any, Optional, Union, AsyncIterator

from news_creator.config.config import NewsCreatorConfig
from news_creator.domain.models import LLMGenerateResponse
from news_creator.driver.ollama_driver import OllamaDriver
from news_creator.driver.ollama_stream_driver import OllamaStreamDriver
from news_creator.gateway.hybrid_priority_semaphore import HybridPrioritySemaphore
from news_creator.gateway.model_router import ModelRouter
from news_creator.gateway.oom_detector import OOMDetector
from news_creator.port.llm_provider_port import LLMProviderPort

logger = logging.getLogger(__name__)


class OllamaGateway(LLMProviderPort):
    """Gateway for Ollama LLM service - Anti-Corruption Layer."""

    def __init__(self, config: NewsCreatorConfig):
        """Initialize Ollama gateway."""
        self.config = config
        self.driver = OllamaDriver(config)
        self.stream_driver = OllamaStreamDriver(config)
        # Hybrid RT/BE semaphore for controlling concurrent requests to Ollama
        # RT (streaming) requests get reserved slots; BE (batch) uses remaining slots
        # Aging mechanism prevents starvation of low-priority batch requests
        self._semaphore = HybridPrioritySemaphore(
            total_slots=config.ollama_request_concurrency,
            rt_reserved_slots=config.scheduling_rt_reserved_slots,
            aging_threshold_seconds=config.scheduling_aging_threshold_seconds,
            aging_boost=config.scheduling_aging_boost,
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

    async def generate(
        self,
        prompt: str,
        *,
        model: Optional[str] = None,
        num_predict: Optional[int] = None,
        stream: bool = False,
        keep_alive: Optional[Union[int, str]] = None,
        format: Optional[Union[str, Dict[str, Any]]] = None,
        options: Optional[Dict[str, Any]] = None,
        priority: str = "low",
    ) -> Union[LLMGenerateResponse, AsyncIterator[LLMGenerateResponse]]:
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
        if not prompt or not prompt.strip():
            raise ValueError("prompt cannot be empty")

        # Select model using router
        # - If routing enabled and model is None: use router
        # - If routing enabled and model is base model name (e.g., "gemma3:4b"): use router to auto-map
        # - If routing enabled and model is bucket model name (e.g., "gemma3-4b-8k"): use as-is
        # - If routing disabled: use provided model or default
        if self.config.model_routing_enabled:
            if model is None:
                # No model specified - use router
                selected_model, bucket_size = self.model_router.select_model(
                    prompt, max_new_tokens=num_predict
                )
                model = selected_model
                logger.debug(
                    f"Model router selected: {selected_model} (bucket: {bucket_size})",
                    extra={"model": selected_model, "bucket": bucket_size},
                )
            elif self.config.is_base_model_name(model):
                # Base model name specified (e.g., "gemma3:4b") - auto-map using router
                original_model = model
                selected_model, bucket_size = self.model_router.select_model(
                    prompt, max_new_tokens=num_predict
                )
                model = selected_model
                logger.info(
                    f"Base model name '{original_model}' auto-mapped to bucket model: {selected_model} (bucket: {bucket_size})",
                    extra={
                        "original_model": original_model,
                        "mapped_model": selected_model,
                        "bucket": bucket_size,
                    },
                )
            elif self.config.is_bucket_model_name(model):
                # Bucket model name explicitly specified (e.g., "gemma3-4b-8k") - use as-is
                logger.debug(
                    f"Bucket model explicitly specified, using as-is: {model}",
                    extra={"model": model, "prompt_length": len(prompt)},
                )
            else:
                # Unknown model name - use as-is but warn if prompt is very large
                prompt_length = len(prompt)
                estimated_tokens = prompt_length // 4
                if estimated_tokens > 16000:  # >16K tokens
                    logger.warning(
                        f"Large prompt detected with unknown model: {model}. "
                        f"Prompt length: {prompt_length} chars, estimated tokens: {estimated_tokens}. "
                        f"Consider using base model name or bucket model name for automatic routing.",
                        extra={
                            "model": model,
                            "prompt_length": prompt_length,
                            "estimated_tokens": estimated_tokens,
                        },
                    )
                logger.debug(
                    f"Unknown model name specified, using as-is: {model}",
                    extra={"model": model, "prompt_length": prompt_length},
                )
        else:
            # Routing disabled - use provided model or default
            model = model or self.config.model_name
            logger.debug(
                f"Model routing disabled, using: {model}",
                extra={"model": model},
            )

        # Build options from config and overrides
        llm_options = self.config.get_llm_options()
        if options:
            # CRITICAL: Remove num_ctx from options to prevent override
            # num_ctx is fixed in Modelfile, so we never send it in API requests
            options_filtered = {k: v for k, v in options.items() if k != "num_ctx"}
            llm_options.update(options_filtered)

        # CRITICAL: We previously removed num_ctx, but now we allow it to be passed
        # if explicitly set in config, to override Modelfile defaults when necessary.
        # This is important for performance tuning (e.g. reducing context to 16k).
        # if "num_ctx" in llm_options:
        #     del llm_options["num_ctx"]
        #     logger.debug("Removed num_ctx from options (fixed in Modelfile)")

        # Apply num_predict override if provided
        if num_predict is not None:
            llm_options["num_predict"] = num_predict

        # Determine keep_alive value based on model (best practice: model-specific keep_alive)
        # If keep_alive is explicitly provided, use it; otherwise, use model-specific default
        if keep_alive is not None:
            # Explicit keep_alive provided - use it
            final_keep_alive = keep_alive
        else:
            # Use model-specific keep_alive based on best practices
            # 16K model: 24h (keep in GPU memory)
            final_keep_alive = self.config.get_keep_alive_for_model(model)
            logger.debug(f"Using model-specific keep_alive: {final_keep_alive} for model: {model}", extra={"model": model, "keep_alive": final_keep_alive})

        # Build payload for Ollama API
        payload: Dict[str, Any] = {
            "model": model,
            "prompt": prompt.strip(),
            "stream": stream,
            "keep_alive": final_keep_alive,
            "options": llm_options,
        }

        # Add format parameter if provided (Ollama structured output)
        if format is not None:
            payload["format"] = format
            logger.debug("Using structured output format", extra={"format": format})

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
        ABNORMAL_PROMPT_THRESHOLD = 100_000  # characters (>25K tokens)
        if prompt_length > ABNORMAL_PROMPT_THRESHOLD:
            logger.error(
                "ABNORMAL PROMPT SIZE DETECTED in ollama_gateway",
                extra={
                    "prompt_length": prompt_length,
                    "estimated_tokens": estimated_tokens,
                    "context_window": context_window,
                    "model": payload["model"],
                    "prompt_preview_start": prompt[:500],
                    "prompt_preview_end": prompt[-500:] if prompt_length > 1000 else "",
                    "num_predict": llm_options.get("num_predict"),
                }
            )

        # Log model loading strategy (16K/60K on-demand)
        # model_loading_strategy = "always-loaded" if payload['model'] == self.config.model_8k_name else "on-demand"  # 8kモデルは使用しない
        model_loading_strategy = "on-demand"
        logger.info(
            f"Generating with Ollama: model={payload['model']}, loading_strategy={model_loading_strategy}, "
            f"prompt_length={prompt_length} chars, estimated_tokens={estimated_tokens}, "
            f"num_predict={llm_options.get('num_predict')}, context_window={context_window}, "
            f"usage_percent={round((estimated_tokens / context_window) * 100, 1) if context_window > 0 else 0}%"
        )

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
            wait_time = await self._semaphore.acquire(high_priority=is_high_priority)
            priority_label = "HIGH PRIORITY" if is_high_priority else "LOW PRIORITY"
            logger.info(
                f"Acquired semaphore ({priority_label}), returning streaming generator",
                extra={
                    "model": payload["model"],
                    "prompt_length": len(prompt),
                    "stream": True,
                    "priority": priority,
                    "is_high_priority": is_high_priority,
                    "queue_wait_time_seconds": round(wait_time, 4),
                }
            )

            async def response_generator():
                try:
                    logger.debug(
                        "Starting stream iteration (semaphore pre-acquired)",
                        extra={"model": payload["model"], "is_high_priority": is_high_priority}
                    )
                    # Use stream driver for streaming requests
                    stream_iterator = self.stream_driver.generate_stream(payload)
                    # Handle streaming response
                    async for chunk in stream_iterator:
                        # Map chunk to LLMGenerateResponse
                        yield LLMGenerateResponse(
                            response=chunk.get("response", ""),
                            model=chunk.get("model", payload["model"]),
                            done=chunk.get("done", False),
                            done_reason=chunk.get("done_reason"),
                            prompt_eval_count=chunk.get("prompt_eval_count"),
                            eval_count=chunk.get("eval_count"),
                            total_duration=chunk.get("total_duration"),
                            load_duration=chunk.get("load_duration"),
                            prompt_eval_duration=chunk.get("prompt_eval_duration"),
                            eval_duration=chunk.get("eval_duration"),
                        )
                finally:
                    # Release semaphore when generator completes (normal, abort, or GC)
                    self._semaphore.release(was_high_priority=is_high_priority)
                    logger.debug(
                        "Released semaphore after streaming completion",
                        extra={"model": payload["model"], "is_high_priority": is_high_priority}
                    )

            # Return the generator (semaphore already acquired)
            return response_generator()

        # Handle non-streaming requests
        # Acquire semaphore based on priority
        wait_time = await self._semaphore.acquire(high_priority=is_high_priority)
        priority_label = "HIGH PRIORITY" if is_high_priority else "LOW PRIORITY"
        try:
            logger.info(
                f"Acquired semaphore ({priority_label}), processing Ollama request (non-streaming)",
                extra={
                    "model": payload["model"],
                    "prompt_length": len(prompt),
                    "stream": False,
                    "priority": priority,
                    "is_high_priority": is_high_priority,
                    "queue_wait_time_seconds": round(wait_time, 4),
                }
            )
            # Call appropriate driver based on stream flag
            try:
                # Use regular driver for non-streaming requests
                response_data = await self.driver.generate(payload)

                # Non-streaming response handling (existing logic)
                # Check for OOM in response
                if self.oom_detector.detect_oom_from_response(response_data):
                    # OOM detected - retry with 2-model mode
                    logger.warning(
                        "OOM detected in response, retrying with 2-model mode",
                        extra={"original_model": model},
                    )
                    # Retry with router in 2-model mode (OOM detector already set two_model_mode)
                    selected_model, _ = self.model_router.select_model(
                        prompt, max_new_tokens=num_predict
                    )
                    if selected_model != model:
                        payload["model"] = selected_model
                        logger.info(
                            f"Retrying with model {selected_model} (2-model mode)",
                            extra={"original_model": model, "new_model": selected_model},
                        )
                        response_data = await self.driver.generate(payload)
            except Exception as e:
                # Check if exception indicates OOM
                if self.oom_detector.detect_oom_from_exception(e):
                    logger.warning(
                        "OOM detected from exception, retrying with 2-model mode",
                        extra={"original_model": model, "error": str(e)},
                    )
                    # Retry with router in 2-model mode (OOM detector already set two_model_mode)
                    selected_model, _ = self.model_router.select_model(
                        prompt, max_new_tokens=num_predict
                    )
                    if selected_model != model:
                        payload["model"] = selected_model
                        logger.info(
                            f"Retrying with model {selected_model} (2-model mode)",
                            extra={"original_model": model, "new_model": selected_model},
                        )
                        # Ensure stream is False for retry to avoid complexity
                        payload["stream"] = False
                        response_data = await self.driver.generate(payload)
                    else:
                        # Same model selected, re-raise original exception
                        raise
                else:
                    raise
        finally:
            self._semaphore.release(was_high_priority=is_high_priority)

        # Validate response (Non-streaming only)
        if "response" not in response_data:
            logger.error("Ollama response missing 'response' field", extra={"keys": list(response_data.keys())})
            raise RuntimeError("Invalid Ollama response format")

        # Extract performance metrics from response
        prompt_eval_count = response_data.get("prompt_eval_count")
        eval_count = response_data.get("eval_count")
        total_duration = response_data.get("total_duration")
        load_duration = response_data.get("load_duration")
        prompt_eval_duration = response_data.get("prompt_eval_duration")
        eval_duration = response_data.get("eval_duration")
        actual_model = response_data.get("model", payload["model"])

        # Check if actual model matches requested model (model loading strategy validation)
        requested_model = payload["model"]
        if actual_model != requested_model:
            logger.warning(
                f"Model mismatch detected: requested={requested_model}, actual={actual_model}. "
                f"This may indicate that Ollama used a different model than requested, "
                f"possibly due to model loading strategy or OLLAMA_MAX_LOADED_MODELS setting.",
                extra={
                    "requested_model": requested_model,
                    "actual_model": actual_model,
                    "prompt_length": prompt_length,
                },
            )
        else:
            logger.debug(
                f"Model match confirmed: {requested_model}",
                extra={"model": requested_model},
            )

        # Log performance metrics to understand why inference takes time
        duration_seconds = round(total_duration / 1_000_000_000, 2) if total_duration else None
        tokens_per_second = round(eval_count / (total_duration / 1_000_000_000), 2) if eval_count and total_duration else None

        # Calculate detailed timing metrics
        load_duration_seconds = round(load_duration / 1_000_000_000, 2) if load_duration else None
        prompt_eval_duration_seconds = round(prompt_eval_duration / 1_000_000_000, 2) if prompt_eval_duration else None
        eval_duration_seconds = round(eval_duration / 1_000_000_000, 2) if eval_duration else None

        # Calculate prefill and decode speeds
        prefill_tokens_per_second = round(
            prompt_eval_count / (prompt_eval_duration / 1_000_000_000), 2
        ) if prompt_eval_count and prompt_eval_duration else None
        decode_tokens_per_second = round(
            eval_count / (eval_duration / 1_000_000_000), 2
        ) if eval_count and eval_duration else None

        # TTFT (Time To First Token) Breakdown Logging
        # TTFT = queue_wait_time + load_duration + prompt_eval_duration
        # Get queue wait time from semaphore
        queue_wait_time_seconds = round(self._semaphore.last_wait_time, 4)

        # Calculate TTFT (in seconds)
        ttft_seconds = queue_wait_time_seconds
        if load_duration_seconds:
            ttft_seconds += load_duration_seconds
        if prompt_eval_duration_seconds:
            ttft_seconds += prompt_eval_duration_seconds
        ttft_seconds = round(ttft_seconds, 2)

        # Log TTFT breakdown for diagnostics
        logger.info(
            f"TTFT breakdown: ttft={ttft_seconds}s "
            f"(queue_wait={queue_wait_time_seconds}s + "
            f"load_duration={load_duration_seconds}s + "
            f"prompt_eval_duration={prompt_eval_duration_seconds}s), "
            f"model={actual_model}, prompt_tokens={prompt_eval_count}",
            extra={
                "ttft_seconds": ttft_seconds,
                "queue_wait_time_seconds": queue_wait_time_seconds,
                "load_duration_seconds": load_duration_seconds,
                "prompt_eval_duration_seconds": prompt_eval_duration_seconds,
                "model": actual_model,
                "prompt_eval_count": prompt_eval_count,
            },
        )

        # Cold start detection: warn if load_duration > 0.1s (100ms)
        # load_duration > 0.1s indicates model was not in VRAM (cold start)
        COLD_START_THRESHOLD_SECONDS = 0.1
        if load_duration_seconds and load_duration_seconds > COLD_START_THRESHOLD_SECONDS:
            logger.warning(
                f"COLD_START detected: load_duration={load_duration_seconds}s > {COLD_START_THRESHOLD_SECONDS}s. "
                f"Model '{actual_model}' was not in VRAM and required disk-to-VRAM loading. "
                f"This adds {load_duration_seconds}s to TTFT. "
                f"Consider: (1) increasing OLLAMA_MAX_LOADED_MODELS, "
                f"(2) extending keep_alive duration, or "
                f"(3) ensuring warmup runs on startup.",
                extra={
                    "load_duration_seconds": load_duration_seconds,
                    "cold_start_threshold": COLD_START_THRESHOLD_SECONDS,
                    "model": actual_model,
                    "ttft_seconds": ttft_seconds,
                },
            )

        logger.info(
            f"Ollama generation completed: requested_model={requested_model}, actual_model={actual_model}, "
            f"prompt_length={prompt_length} chars, prompt_eval_count={prompt_eval_count} tokens, "
            f"eval_count={eval_count} tokens, num_predict={llm_options.get('num_predict')}, "
            f"duration={duration_seconds}s, tokens_per_second={tokens_per_second}, "
            f"load_duration={load_duration_seconds}s, prompt_eval_duration={prompt_eval_duration_seconds}s "
            f"(prefill: {prefill_tokens_per_second} tok/s), eval_duration={eval_duration_seconds}s "
            f"(decode: {decode_tokens_per_second} tok/s)"
        )

        # Performance monitoring: Warn if generation is slow
        # RTX 4060 expected performance: 50-100 tokens/sec
        # Threshold: <30 tokens/sec is considered slow for RTX 4060
        SLOW_GENERATION_THRESHOLD_TPS = 30  # tokens per second
        SLOW_GENERATION_THRESHOLD_DURATION = 5_000_000_000  # 5 seconds in nanoseconds

        is_slow_duration = total_duration and total_duration > SLOW_GENERATION_THRESHOLD_DURATION
        is_slow_tps = tokens_per_second is not None and tokens_per_second < SLOW_GENERATION_THRESHOLD_TPS

        if is_slow_duration or is_slow_tps:
            warning_msg = (
                f"Slow LLM generation detected: duration={duration_seconds}s, "
                f"tokens_per_second={tokens_per_second} (expected: 50-100 for RTX 4060), "
                f"prompt_eval_count={prompt_eval_count}, eval_count={eval_count}, "
                f"prompt_length={prompt_length} chars, estimated_tokens={estimated_tokens}, "
                f"context_window={context_window}, requested_model={requested_model}, "
                f"actual_model={actual_model}. "
            )

            # Add specific recommendations based on the issue
            if is_slow_tps:
                warning_msg += (
                    f"Low token generation speed detected. "
                    f"Possible causes: VRAM bandwidth bottleneck, suboptimal batch size, "
                    f"or model loading issues. Consider checking OLLAMA_NUM_BATCH and "
                    f"OLLAMA_MAX_LOADED_MODELS settings."
                )
            else:
                warning_msg += (
                    f"Long generation duration. "
                    f"Possible causes: Large context window ({context_window}), "
                    f"large prompt size ({estimated_tokens} tokens), "
                    f"many tokens to generate ({llm_options.get('num_predict')} tokens), "
                    f"or hardware resource constraints."
                )

            logger.warning(warning_msg, extra={
                "duration_seconds": duration_seconds,
                "tokens_per_second": tokens_per_second,
                "prompt_eval_count": prompt_eval_count,
                "eval_count": eval_count,
                "prompt_length": prompt_length,
                "estimated_tokens": estimated_tokens,
                "context_window": context_window,
                "requested_model": requested_model,
                "actual_model": actual_model,
                "is_slow_tps": is_slow_tps,
                "is_slow_duration": is_slow_duration,
            })

        # Map to domain model (use actual model from response, not requested model)
        # This ensures we track which model was actually used
        return LLMGenerateResponse(
            response=response_data.get("response", ""),
            model=actual_model,  # Use actual model from response
            done=response_data.get("done"),
            done_reason=response_data.get("done_reason"),
            prompt_eval_count=prompt_eval_count,
            eval_count=eval_count,
            total_duration=total_duration,
            load_duration=load_duration,
            prompt_eval_duration=prompt_eval_duration,
            eval_duration=eval_duration,
        )

    async def list_models(self) -> list[Dict[str, Any]]:
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
            logger.debug(f"Found {len(models)} models in Ollama", extra={"count": len(models)})
            return models
        except Exception as err:
            logger.error("Failed to list Ollama models", extra={"error": str(err)})
            raise RuntimeError(f"Failed to list Ollama models: {err}") from err
