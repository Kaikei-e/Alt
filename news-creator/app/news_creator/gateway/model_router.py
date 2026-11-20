"""Model router for selecting appropriate model bucket based on token count."""

import logging

from news_creator.config.config import NewsCreatorConfig
from news_creator.gateway.oom_detector import OOMDetector
from news_creator.utils.token_counter import count_tokens

logger = logging.getLogger(__name__)


class ModelRouter:
    """Routes requests to appropriate model bucket (8K, 60K) based on token count."""

    # Bucket definitions (context window sizes)
    BUCKET_8K = 8192
    BUCKET_60K = 61440

    def __init__(
        self,
        config: NewsCreatorConfig,
        oom_detector: OOMDetector | None = None,
    ) -> None:
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
        self._current_bucket: int | None = None

    @property
    def current_bucket(self) -> int | None:
        """Get current bucket size."""
        return self._current_bucket

    def select_model(
        self, prompt: str, max_new_tokens: int | None = None
    ) -> tuple[str, int]:
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

        # Normal 2-model mode (8K, 60K)
        return self._select_model_3mode(prompt, max_new_tokens)

    def _select_bucket_for_tokens(
        self, needed_tokens: int, prompt_tokens: int
    ) -> tuple[str, int]:
        model_60k_enabled = getattr(self.config, "model_60k_enabled", True)
        if not model_60k_enabled:
            if needed_tokens > self.BUCKET_8K:
                logger.warning(
                    "Token count (%s) exceeds 8K bucket but 60K is disabled. "
                    "Hierarchical summarization should handle this.",
                    needed_tokens,
                    extra={
                        "needed_tokens": needed_tokens,
                        "prompt_tokens": prompt_tokens,
                    },
                )
            return self.config.model_8k_name, self.BUCKET_8K
        if needed_tokens <= self.BUCKET_8K:
            return self.config.model_8k_name, self.BUCKET_8K
        if needed_tokens <= self.BUCKET_60K:
            return self.config.model_60k_name, self.BUCKET_60K
        logger.warning(
            "Token count (%s) exceeds 60K bucket. Using 60K model.",
            needed_tokens,
            extra={"needed_tokens": needed_tokens, "prompt_tokens": prompt_tokens},
        )
        return self.config.model_60k_name, self.BUCKET_60K

    def _apply_hysteresis(
        self, selected_model: str, selected_bucket: int, needed_tokens: int
    ) -> tuple[str, int]:
        if self._current_bucket is None:
            self._current_bucket = selected_bucket
            return selected_model, selected_bucket

        if selected_bucket < self._current_bucket:
            if self._current_bucket >= selected_bucket * 2:
                logger.info(
                    "Switching bucket: %s -> %s (2x rule satisfied)",
                    self._current_bucket,
                    selected_bucket,
                    extra={
                        "old_bucket": self._current_bucket,
                        "new_bucket": selected_bucket,
                        "needed_tokens": needed_tokens,
                    },
                )
                self._current_bucket = selected_bucket
                return selected_model, selected_bucket

            logger.debug(
                "Keeping current bucket %s (2x rule: %s * 2 = %s > %s)",
                self._current_bucket,
                selected_bucket,
                selected_bucket * 2,
                self._current_bucket,
                extra={
                    "current_bucket": self._current_bucket,
                    "requested_bucket": selected_bucket,
                    "needed_tokens": needed_tokens,
                },
            )
            held_model = (
                self.config.model_8k_name
                if self._current_bucket == self.BUCKET_8K
                else self.config.model_60k_name
            )
            return held_model, self._current_bucket

        if selected_bucket > self._current_bucket:
            logger.info(
                "Switching bucket: %s -> %s (upgrade allowed)",
                self._current_bucket,
                selected_bucket,
                extra={
                    "old_bucket": self._current_bucket,
                    "new_bucket": selected_bucket,
                    "needed_tokens": needed_tokens,
                },
            )
            self._current_bucket = selected_bucket
            return selected_model, selected_bucket

        logger.debug(
            "Using current bucket: %s",
            self._current_bucket,
            extra={
                "bucket": self._current_bucket,
                "needed_tokens": needed_tokens,
            },
        )
        return selected_model, selected_bucket

    def _select_model_3mode(
        self, prompt: str, max_new_tokens: int | None = None
    ) -> tuple[str, int]:
        """Select model in 2-model mode (8K, 60K) or 8K-only mode."""
        prompt_tokens = count_tokens(prompt)
        max_new = max_new_tokens or self.config.llm_num_predict

        logger.debug(
            "Token count calculation",
            extra={
                "prompt_length_chars": len(prompt),
                "prompt_tokens": prompt_tokens,
                "max_new_tokens": max_new,
            },
        )

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

        selected_model, selected_bucket = self._select_bucket_for_tokens(
            needed_tokens, prompt_tokens
        )
        selected_model, selected_bucket = self._apply_hysteresis(
            selected_model, selected_bucket, needed_tokens
        )

        loading_strategy = "on-demand"
        logger.info(
            "Selected model: %s (bucket: %s, needed: %s tokens, loading_strategy: %s)",
            selected_model,
            selected_bucket,
            needed_tokens,
            loading_strategy,
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
