"""Configuration module for News Creator service.

This module composes specialized config dataclasses while maintaining
backward compatibility with existing attribute access patterns.
"""

from __future__ import annotations

import os
import logging
from typing import Union

from news_creator.config.llm_config import LLMConfig
from news_creator.config.scheduling_config import SchedulingConfig
from news_creator.config.hierarchical_config import HierarchicalConfig
from news_creator.config.model_routing_config import ModelRoutingConfig

logger = logging.getLogger(__name__)


class NewsCreatorConfig:
    """Configuration for News Creator service from environment variables.

    This class composes specialized config dataclasses (LLMConfig, SchedulingConfig,
    HierarchicalConfig, ModelRoutingConfig) while maintaining backward compatibility
    with direct attribute access (e.g., config.llm_service_url).
    """

    def __init__(self):
        """Initialize configuration from environment variables."""
        # Authentication settings (remain at top level)
        self.auth_service_url = os.getenv(
            "AUTH_SERVICE_URL",
            "http://auth-service.alt-auth.svc.cluster.local:8080",
        )
        self.service_name = "news-creator"
        self.service_secret = os.getenv("SERVICE_SECRET", "")
        if not self.service_secret:
            secret_file = os.getenv("SERVICE_SECRET_FILE")
            if secret_file:
                try:
                    with open(secret_file, "r") as f:
                        self.service_secret = f.read().strip()
                except Exception as e:
                    logger.error(f"Failed to read SERVICE_SECRET_FILE: {e}")

        self.token_ttl = 3600

        if not self.service_secret:
            raise ValueError("SERVICE_SECRET environment variable is required")

        # Compose specialized config dataclasses
        self.llm = LLMConfig.from_env()
        self.scheduling = SchedulingConfig.from_env()
        self.hierarchical = HierarchicalConfig.from_env()
        self.model_routing = ModelRoutingConfig.from_env()

        # Distributed BE dispatch (remains at top level for now)
        self.distributed_be_enabled = (
            os.getenv("DISTRIBUTED_BE_ENABLED", "false").lower() == "true"
        )
        self.distributed_be_remotes = self._parse_remote_urls(
            os.getenv("DISTRIBUTED_BE_REMOTES", "")
        )
        self.distributed_be_health_interval_seconds = self._get_int(
            "DISTRIBUTED_BE_HEALTH_INTERVAL_SECONDS", 30
        )
        self.distributed_be_timeout_seconds = self._get_int(
            "DISTRIBUTED_BE_TIMEOUT_SECONDS", 300
        )
        self.distributed_be_cooldown_seconds = self._get_int(
            "DISTRIBUTED_BE_COOLDOWN_SECONDS", 60
        )
        self.distributed_be_remote_model = os.getenv(
            "DISTRIBUTED_BE_REMOTE_MODEL", "gemma4-e4b-q4km"
        ).strip()
        self.distributed_be_model_overrides = self._parse_model_overrides(
            os.getenv("DISTRIBUTED_BE_MODEL_OVERRIDES", "")
        )

        if self.distributed_be_enabled and not self.distributed_be_remotes:
            logger.warning(
                "DISTRIBUTED_BE_ENABLED=true but DISTRIBUTED_BE_REMOTES is empty; "
                "distributed dispatch will be effectively disabled"
            )

        # Cache settings (remains at top level for now)
        self.cache_enabled = os.getenv("CACHE_ENABLED", "false").lower() == "true"
        self.cache_redis_url = os.getenv("CACHE_REDIS_URL", "redis://localhost:6379/0")
        self.cache_ttl_seconds = self._get_int("CACHE_TTL_SECONDS", 86400)

        logger.info(
            "News creator configuration initialized",
            extra={
                "auth_service_url": self.auth_service_url,
                "service_name": self.service_name,
                "llm_service_url": self.llm.service_url,
                "model": self.llm.model_name,
                "ollama_request_concurrency": self.scheduling.request_concurrency,
                "ollama_concurrency_source": self.scheduling.concurrency_source,
                "ollama_num_parallel": os.getenv("OLLAMA_NUM_PARALLEL"),
                "scheduling_rt_reserved_slots": self.scheduling.rt_reserved_slots,
                "scheduling_aging_threshold_seconds": self.scheduling.aging_threshold_seconds,
                "scheduling_priority_promotion_threshold_seconds": self.scheduling.priority_promotion_threshold_seconds,
                "scheduling_guaranteed_be_ratio": self.scheduling.guaranteed_be_ratio,
                "scheduling_rt_mode": self.scheduling.rt_mode,
            },
        )

    # =========================================================================
    # Backward Compatibility Properties - LLM Config
    # =========================================================================

    @property
    def llm_service_url(self) -> str:
        """Backward compatible access to LLM service URL."""
        return self.llm.service_url

    @property
    def model_name(self) -> str:
        """Backward compatible access to model name."""
        return self.llm.model_name

    @property
    def llm_timeout_seconds(self) -> int:
        """Backward compatible access to LLM timeout."""
        return self.llm.timeout_seconds

    @property
    def llm_keep_alive(self) -> str:
        """Backward compatible access to keep alive."""
        return self.llm.keep_alive

    @property
    def llm_keep_alive_8k(self) -> str:
        """Backward compatible access to 8K keep alive."""
        return self.llm.keep_alive_8k

    @property
    def llm_keep_alive_60k(self) -> str:
        """Backward compatible access to 60K keep alive."""
        return self.llm.keep_alive_60k

    @property
    def llm_num_ctx(self) -> int:
        """Backward compatible access to num_ctx."""
        return self.llm.num_ctx

    @property
    def llm_num_batch(self) -> int:
        """Backward compatible access to num_batch."""
        return self.llm.num_batch

    @property
    def llm_num_predict(self) -> int:
        """Backward compatible access to num_predict."""
        return self.llm.num_predict

    @property
    def llm_temperature(self) -> float:
        """Backward compatible access to temperature."""
        return self.llm.temperature

    @property
    def llm_top_p(self) -> float:
        """Backward compatible access to top_p."""
        return self.llm.top_p

    @property
    def llm_top_k(self) -> int:
        """Backward compatible access to top_k."""
        return self.llm.top_k

    @property
    def llm_repeat_penalty(self) -> float:
        """Backward compatible access to repeat_penalty."""
        return self.llm.repeat_penalty

    @property
    def llm_num_keep(self) -> int:
        """Backward compatible access to num_keep."""
        return self.llm.num_keep

    @property
    def llm_stop_tokens(self) -> list[str]:
        """Backward compatible access to stop_tokens."""
        return list(self.llm.stop_tokens)

    @property
    def summary_num_predict(self) -> int:
        """Backward compatible access to summary_num_predict."""
        return self.llm.summary_num_predict

    @property
    def recap_summary_num_predict(self) -> int:
        """Recap-specific output token budget (separate from article summary)."""
        return self.llm.recap_summary_num_predict

    @property
    def recap_min_avg_bullet_length(self) -> int:
        """Minimum average bullet length (chars) to pass quality gate."""
        return self.llm.recap_min_avg_bullet_length

    @property
    def summary_temperature(self) -> float:
        """Backward compatible access to summary_temperature."""
        return self.llm.summary_temperature

    @property
    def max_repetition_retries(self) -> int:
        """Backward compatible access to max_repetition_retries."""
        return self.llm.max_repetition_retries

    @property
    def repetition_threshold(self) -> float:
        """Backward compatible access to repetition_threshold."""
        return self.llm.repetition_threshold

    @property
    def recap_min_source_articles_for_llm(self) -> int:
        """Backward compatible access to recap_min_source_articles_for_llm."""
        return self.llm.recap_min_source_articles_for_llm

    @property
    def recap_min_representative_sentences_for_llm(self) -> int:
        """Backward compatible access to recap_min_representative_sentences_for_llm."""
        return self.llm.recap_min_representative_sentences_for_llm

    @property
    def recap_summary_temperature(self) -> float:
        """Backward compatible access to recap_summary_temperature."""
        return self.llm.recap_summary_temperature

    @property
    def recap_ja_ratio_threshold(self) -> float:
        """Backward compatible access to recap_ja_ratio_threshold."""
        return self.llm.recap_ja_ratio_threshold

    @property
    def recap_summary_repair_attempts(self) -> int:
        """Backward compatible access to recap_summary_repair_attempts."""
        return self.llm.recap_summary_repair_attempts

    # =========================================================================
    # Backward Compatibility Properties - Scheduling Config
    # =========================================================================

    @property
    def ollama_request_concurrency(self) -> int:
        """Backward compatible access to request concurrency."""
        return self.scheduling.request_concurrency

    @property
    def _ollama_concurrency_source(self) -> str:
        """Backward compatible access to concurrency source."""
        return self.scheduling.concurrency_source

    @property
    def scheduling_rt_reserved_slots(self) -> int:
        """Backward compatible access to RT reserved slots."""
        return self.scheduling.rt_reserved_slots

    @property
    def scheduling_aging_threshold_seconds(self) -> float:
        """Backward compatible access to aging threshold."""
        return self.scheduling.aging_threshold_seconds

    @property
    def scheduling_aging_boost(self) -> float:
        """Backward compatible access to aging boost."""
        return self.scheduling.aging_boost

    @property
    def scheduling_preemption_enabled(self) -> bool:
        """Backward compatible access to preemption enabled."""
        return self.scheduling.preemption_enabled

    @property
    def scheduling_preemption_wait_threshold_seconds(self) -> float:
        """Backward compatible access to preemption wait threshold."""
        return self.scheduling.preemption_wait_threshold_seconds

    @property
    def scheduling_priority_promotion_threshold_seconds(self) -> float:
        """Backward compatible access to priority promotion threshold."""
        return self.scheduling.priority_promotion_threshold_seconds

    @property
    def scheduling_guaranteed_be_ratio(self) -> int:
        """Backward compatible access to guaranteed BE ratio."""
        return self.scheduling.guaranteed_be_ratio

    @property
    def max_queue_depth(self) -> int:
        """Backward compatible access to max queue depth."""
        return self.scheduling.max_queue_depth

    @property
    def scheduling_rt_mode(self) -> str:
        """Backward compatible access to RT mode."""
        return self.scheduling.rt_mode

    # =========================================================================
    # Backward Compatibility Properties - Hierarchical Config
    # =========================================================================

    @property
    def hierarchical_threshold_chars(self) -> int:
        """Backward compatible access to hierarchical threshold chars."""
        return self.hierarchical.threshold_chars

    @property
    def hierarchical_threshold_clusters(self) -> int:
        """Backward compatible access to hierarchical threshold clusters."""
        return self.hierarchical.threshold_clusters

    @property
    def hierarchical_chunk_max_chars(self) -> int:
        """Backward compatible access to chunk max chars."""
        return self.hierarchical.chunk_max_chars

    @property
    def hierarchical_chunk_overlap_ratio(self) -> float:
        """Backward compatible access to chunk overlap ratio."""
        return self.hierarchical.chunk_overlap_ratio

    @property
    def recursive_reduce_max_chars(self) -> int:
        """Backward compatible access to recursive reduce max chars."""
        return self.hierarchical.recursive_reduce_max_chars

    @property
    def recursive_reduce_max_depth(self) -> int:
        """Backward compatible access to recursive reduce max depth."""
        return self.hierarchical.recursive_reduce_max_depth

    @property
    def hierarchical_single_article_threshold(self) -> int:
        """Backward compatible access to single article threshold."""
        return self.hierarchical.single_article_threshold

    @property
    def hierarchical_single_article_chunk_size(self) -> int:
        """Backward compatible access to single article chunk size."""
        return self.hierarchical.single_article_chunk_size

    @property
    def hierarchical_token_budget_percent(self) -> int:
        """Backward compatible access to token budget percent."""
        return self.hierarchical.token_budget_percent

    # =========================================================================
    # Backward Compatibility Properties - Model Routing Config
    # =========================================================================

    @property
    def model_routing_enabled(self) -> bool:
        """Backward compatible access to model routing enabled."""
        return self.model_routing.enabled

    @property
    def model_base_name(self) -> str:
        """Backward compatible access to model base name."""
        return self.model_routing.base_name

    @property
    def model_8k_name(self) -> str:
        """Backward compatible access to 8K model name."""
        return self.model_routing.model_8k_name

    @property
    def model_60k_name(self) -> str:
        """Backward compatible access to 60K model name."""
        return self.model_routing.model_60k_name

    @property
    def model_60k_enabled(self) -> bool:
        """Backward compatible access to 60K model enabled."""
        return self.model_routing.model_60k_enabled

    @property
    def token_safety_margin_percent(self) -> int:
        """Backward compatible access to token safety margin percent."""
        return self.model_routing.token_safety_margin_percent

    @property
    def token_safety_margin_fixed(self) -> int:
        """Backward compatible access to token safety margin fixed."""
        return self.model_routing.token_safety_margin_fixed

    @property
    def oom_detection_enabled(self) -> bool:
        """Backward compatible access to OOM detection enabled."""
        return self.model_routing.oom_detection_enabled

    @property
    def warmup_enabled(self) -> bool:
        """Backward compatible access to warmup enabled."""
        return self.model_routing.warmup_enabled

    @property
    def warmup_keep_alive_minutes(self) -> int:
        """Backward compatible access to warmup keep alive minutes."""
        return self.model_routing.warmup_keep_alive_minutes

    @property
    def _bucket_model_names(self) -> set[str]:
        """Backward compatible access to bucket model names set."""
        return set(self.model_routing._bucket_model_names)

    # =========================================================================
    # Backward Compatibility Methods
    # =========================================================================

    def is_base_model_name(self, model_name: str) -> bool:
        """Check if the given model name is the base model name."""
        return self.model_routing.is_base_model_name(model_name)

    def is_bucket_model_name(self, model_name: str) -> bool:
        """Check if the given model name is a bucket model name."""
        return self.model_routing.is_bucket_model_name(model_name)

    def get_keep_alive_for_model(self, model_name: str) -> Union[int, str]:
        """Get keep_alive value for a specific model."""
        return self.llm.get_keep_alive_for_model(
            model_name,
            self.model_routing.model_8k_name,
            self.model_routing.model_60k_name,
        )

    def get_llm_options(self) -> dict:
        """Get LLM options as a dictionary (for Ollama 'options')."""
        return self.llm.get_options()

    # =========================================================================
    # Helper Methods
    # =========================================================================

    def _parse_model_overrides(self, raw: str) -> dict[str, str]:
        """Parse comma-separated url=model overrides into a dict."""
        overrides: dict[str, str] = {}
        for part in raw.split(","):
            part = part.strip()
            if not part or "=" not in part:
                continue
            url, model = part.split("=", 1)
            url = url.strip().rstrip("/")
            model = model.strip()
            if url and model:
                overrides[url] = model
        return overrides

    def _parse_remote_urls(self, raw: str) -> list[str]:
        """Parse comma-separated remote URLs, stripping whitespace and trailing slashes."""
        urls = []
        seen: set[str] = set()
        for part in raw.split(","):
            url = part.strip().rstrip("/")
            if not url:
                continue
            if url in seen:
                logger.warning("Duplicate remote URL ignored: %s", url)
                continue
            if not url.startswith("http://") and not url.startswith("https://"):
                logger.warning("Remote URL missing scheme, ignored: %s", url)
                continue
            seen.add(url)
            urls.append(url)
        return urls

    def _get_int(self, name: str, default: int) -> int:
        """Get integer value from environment variable with fallback."""
        try:
            return int(os.getenv(name, default))
        except ValueError:
            logger.warning("Invalid int for %s. Using default %s", name, default)
            return default

    def _get_float(self, name: str, default: float) -> float:
        """Get float value from environment variable with fallback."""
        try:
            return float(os.getenv(name, default))
        except ValueError:
            logger.warning("Invalid float for %s. Using default %s", name, default)
            return default
