"""Model router for selecting appropriate model bucket based on token count."""

import logging
from typing import Optional, Tuple

from news_creator.config.config import NewsCreatorConfig
from news_creator.gateway.oom_detector import OOMDetector
from news_creator.utils.token_counter import count_tokens

logger = logging.getLogger(__name__)


class ModelRouter:
    """Routes requests to appropriate model bucket (12K, 60K) based on token count."""

    # Bucket definitions (context window sizes)
    # BUCKET_8K = 8192  # 8kモデルは使用しない
    BUCKET_12K = 12288
    BUCKET_60K = 61440

    def __init__(
        self,
        config: NewsCreatorConfig,
        oom_detector: Optional[OOMDetector] = None,
    ):
        """
        Initialize model router.

        Args:
            config: News Creator configuration
            oom_detector: Optional OOM detector for fallback mode
        """
        self.config = config
        self.oom_detector = oom_detector or OOMDetector(
            enabled=config.oom_detection_enabled
        )
        self._current_bucket: Optional[int] = None

    @property
    def current_bucket(self) -> Optional[int]:
        """Get current bucket size."""
        return self._current_bucket

    def select_model(
        self, prompt: str, max_new_tokens: Optional[int] = None
    ) -> Tuple[str, int]:
        """
        Select appropriate model based on token count.

        Args:
            prompt: Input prompt text
            max_new_tokens: Maximum number of tokens to generate (default from config)

        Returns:
            Tuple of (model_name, bucket_size)
        """
        if not self.config.model_routing_enabled:
            # Routing disabled, use default model
            return self.config.model_name, self.config.llm_num_ctx

        # Check if 2-model mode is active (OOM fallback - same as normal mode now)
        if self.oom_detector.two_model_mode:
            return self._select_model_3mode(prompt, max_new_tokens)

        # Normal 2-model mode (12K, 60K)
        return self._select_model_3mode(prompt, max_new_tokens)

    def _select_model_3mode(
        self, prompt: str, max_new_tokens: Optional[int] = None
    ) -> Tuple[str, int]:
        """Select model in 2-model mode (12K, 60K) or 12K-only mode."""
        # Calculate token count
        prompt_tokens = count_tokens(prompt)
        max_new = max_new_tokens or self.config.llm_num_predict

        # Log token count for debugging
        logger.debug(
            "Token count calculation",
            extra={
                "prompt_length_chars": len(prompt),
                "prompt_tokens": prompt_tokens,
                "max_new_tokens": max_new,
            },
        )

        # Calculate safety margin (use larger of percentage or fixed)
        safety_margin_percent = (
            prompt_tokens * self.config.token_safety_margin_percent // 100
        )
        safety_margin = max(
            safety_margin_percent, self.config.token_safety_margin_fixed
        )

        needed_tokens = prompt_tokens + max_new + safety_margin

        logger.debug(
            "Model selection calculation",
            extra={
                "prompt_tokens": prompt_tokens,
                "max_new_tokens": max_new,
                "safety_margin": safety_margin,
                "needed_tokens": needed_tokens,
            },
        )

        # Check if 60K model is disabled (12K-only mode)
        model_60k_enabled = getattr(self.config, 'model_60k_enabled', True)

        if not model_60k_enabled:
            # 12K-only mode: always use 12K model
            # Hierarchical summarization should handle large inputs upstream
            if needed_tokens > self.BUCKET_12K:
                logger.warning(
                    f"Token count ({needed_tokens}) exceeds 12K bucket but 60K is disabled. "
                    f"Hierarchical summarization should handle this.",
                    extra={"needed_tokens": needed_tokens, "prompt_tokens": prompt_tokens},
                )
            selected_bucket = self.BUCKET_12K
            selected_model = self.config.model_12k_name
        # Select bucket (12K, or 60K) - only when 60K is enabled
        # if needed_tokens <= self.BUCKET_8K:  # 8kモデルは使用しない
        #     selected_bucket = self.BUCKET_8K
        #     selected_model = self.config.model_8k_name
        elif needed_tokens <= self.BUCKET_12K:
            selected_bucket = self.BUCKET_12K
            selected_model = self.config.model_12k_name
        elif needed_tokens <= self.BUCKET_60K:
            selected_bucket = self.BUCKET_60K
            selected_model = self.config.model_60k_name
        else:
            # Exceeds 60K, use 60K anyway (will need hierarchical summarization)
            logger.warning(
                f"Token count ({needed_tokens}) exceeds 60K bucket. Using 60K model.",
                extra={"needed_tokens": needed_tokens, "prompt_tokens": prompt_tokens},
            )
            selected_bucket = self.BUCKET_60K
            selected_model = self.config.model_60k_name

        # Apply 2x rule: only switch if current bucket is 2x or more larger
        if self._current_bucket is not None:
            if selected_bucket < self._current_bucket:
                # Switching to smaller bucket - check 2x rule
                if self._current_bucket >= selected_bucket * 2:
                    logger.info(
                        f"Switching bucket: {self._current_bucket} -> {selected_bucket} "
                        f"(2x rule satisfied)",
                        extra={
                            "old_bucket": self._current_bucket,
                            "new_bucket": selected_bucket,
                            "needed_tokens": needed_tokens,
                        },
                    )
                    self._current_bucket = selected_bucket
                else:
                    # Keep current bucket (2x rule not satisfied)
                    logger.debug(
                        f"Keeping current bucket {self._current_bucket} "
                        f"(2x rule: {selected_bucket} * 2 = {selected_bucket * 2} > {self._current_bucket})",
                        extra={
                            "current_bucket": self._current_bucket,
                            "requested_bucket": selected_bucket,
                            "needed_tokens": needed_tokens,
                        },
                    )
                    # Find model for current bucket
                    # if self._current_bucket == self.BUCKET_8K:  # 8kモデルは使用しない
                    #     selected_model = self.config.model_8k_name
                    if self._current_bucket == self.BUCKET_12K:
                        selected_model = self.config.model_12k_name
                    else:
                        selected_model = self.config.model_60k_name
                    selected_bucket = self._current_bucket
            else:
                # Switching to larger bucket or same bucket
                if selected_bucket > self._current_bucket:
                    # Switching to larger bucket - always allowed
                    logger.info(
                        f"Switching bucket: {self._current_bucket} -> {selected_bucket} "
                        f"(upgrade allowed)",
                        extra={
                            "old_bucket": self._current_bucket,
                            "new_bucket": selected_bucket,
                            "needed_tokens": needed_tokens,
                        },
                    )
                    self._current_bucket = selected_bucket
                else:
                    # Same bucket - no switch needed
                    logger.debug(
                        f"Using current bucket: {self._current_bucket}",
                        extra={
                            "bucket": self._current_bucket,
                            "needed_tokens": needed_tokens,
                        },
                    )
        else:
            # First selection
            self._current_bucket = selected_bucket

        # Log model loading strategy (16K/60K on-demand)
        # loading_strategy = "always-loaded" if selected_model == self.config.model_8k_name else "on-demand"  # 8kモデルは使用しない
        loading_strategy = "on-demand"
        logger.info(
            f"Selected model: {selected_model} (bucket: {selected_bucket}, "
            f"needed: {needed_tokens} tokens, loading_strategy: {loading_strategy})",
            extra={
                "model": selected_model,
                "bucket": selected_bucket,
                "prompt_tokens": prompt_tokens,
                "max_new_tokens": max_new,
                "needed_tokens": needed_tokens,
                "loading_strategy": loading_strategy,
            },
        )

        return selected_model, selected_bucket

