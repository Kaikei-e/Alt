"""Configuration module for News Creator service."""

import os
import logging
from typing import Union

logger = logging.getLogger(__name__)


class NewsCreatorConfig:
    """Configuration for News Creator service from environment variables."""

    def __init__(self):
        """Initialize configuration from environment variables."""
        # Authentication settings
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
                    with open(secret_file, 'r') as f:
                        self.service_secret = f.read().strip()
                except Exception as e:
                    logger.error(f"Failed to read SERVICE_SECRET_FILE: {e}")

        self.token_ttl = 3600

        if not self.service_secret:
            raise ValueError("SERVICE_SECRET environment variable is required")

        # LLM service settings
        self.llm_service_url = os.getenv("LLM_SERVICE_URL", "http://localhost:11435")
        self.model_name = os.getenv("LLM_MODEL", "gemma4-e4b-q4km")
        self.llm_timeout_seconds = self._get_int("LLM_TIMEOUT_SECONDS", 300)  # 5分に増加（1000トークン生成 + 続き生成に対応）
        self.llm_keep_alive = os.getenv("LLM_KEEP_ALIVE_SECONDS", "24h")
        # Model-specific keep_alive settings (primary bucket / 60K on-demand)
        # Primary local model: 24h to keep the warmed RAG bucket resident
        # 60K model: 15m to allow quick unloading after use to save VRAM
        self.llm_keep_alive_8k = os.getenv("LLM_KEEP_ALIVE_8K", "24h")
        self.llm_keep_alive_60k = os.getenv("LLM_KEEP_ALIVE_60K", "15m")

        # Concurrency settings:
        # - OLLAMA_REQUEST_CONCURRENCY が明示的に設定されている場合はそれを優先
        # - 未設定の場合は OLLAMA_NUM_PARALLEL に自動追従（なければ最終的に 1 にフォールバック）
        env_concurrency = os.getenv("OLLAMA_REQUEST_CONCURRENCY")
        if env_concurrency is not None:
            self.ollama_request_concurrency = self._get_int(
                "OLLAMA_REQUEST_CONCURRENCY", 1
            )
            self._ollama_concurrency_source = "OLLAMA_REQUEST_CONCURRENCY"
        else:
            # NOTE: entrypoint.sh で OLLAMA_NUM_PARALLEL のデフォルトを設定しているため、
            # ここでは OLLAMA_NUM_PARALLEL が優先される（未設定時のみ 1 にフォールバック）
            self.ollama_request_concurrency = self._get_int("OLLAMA_NUM_PARALLEL", 1)
            self._ollama_concurrency_source = "OLLAMA_NUM_PARALLEL"

        # ---- Generation parameters (Gemma4 E4B + Ollama options) ----
        # Default: compose/runtime should override this to the tuned 12K profile.
        # Keep the code default conservative so local tests can boot without compose env.
        self.llm_num_ctx = self._get_int("LLM_NUM_CTX", 8192)
        # RTX 4060最適化: バッチサイズ1024（entrypoint.shのOLLAMA_NUM_BATCHと統一）
        self.llm_num_batch = self._get_int("LLM_NUM_BATCH", 1024)
        self.llm_num_predict = self._get_int("LLM_NUM_PREDICT", 1200)  # 復活
        # Gemma4 E4B: CJKテキスト向けサンプリング（公式推奨は1.0だが、要約安定性のため0.7を維持）
        self.llm_temperature = self._get_float("LLM_TEMPERATURE", 0.7)
        self.llm_top_p = self._get_float("LLM_TOP_P", 0.85)
        self.llm_top_k = self._get_int("LLM_TOP_K", 40)
        self.llm_repeat_penalty = self._get_float("LLM_REPEAT_PENALTY", 1.15)  # Gemma4公式は1.0だが、CJKループ防止のため維持
        self.llm_num_keep = self._get_int("LLM_NUM_KEEP", -1)          # system保持

        # Stop tokens（Gemma3/4 共通: <start_of_turn>/<end_of_turn>）
        stop_tokens_str = os.getenv("LLM_STOP_TOKENS", "<end_of_turn>")
        self.llm_stop_tokens = [
            token.strip() for token in stop_tokens_str.split(",") if token.strip()
        ]
        if not self.llm_stop_tokens:
            self.llm_stop_tokens = ["<end_of_turn>"]

        # Summary-specific settings
        # Increased from 500 to 1000 tokens to support 1000-1500 character summaries with safety margin
        # Japanese text: 1 character ≈ 1 token, so 1500 chars needs ~1500 tokens + safety margin
        self.summary_num_predict = self._get_int("SUMMARY_NUM_PREDICT", 1000)
        self.summary_temperature = self._get_float("SUMMARY_TEMPERATURE", 0.5)

        # Repetition detection and retry settings
        self.max_repetition_retries = self._get_int("MAX_REPETITION_RETRIES", 2)
        self.repetition_threshold = self._get_float("REPETITION_THRESHOLD", 0.3)
        self.recap_min_source_articles_for_llm = self._get_int(
            "RECAP_MIN_SOURCE_ARTICLES_FOR_LLM", 3
        )
        self.recap_min_representative_sentences_for_llm = self._get_int(
            "RECAP_MIN_REPRESENTATIVE_SENTENCES_FOR_LLM", 4
        )
        self.recap_ja_ratio_threshold = self._get_float("RECAP_JA_RATIO_THRESHOLD", 0.6)
        self.recap_summary_repair_attempts = self._get_int(
            "RECAP_SUMMARY_REPAIR_ATTEMPTS", 1
        )

        # 60K model enable/disable flag (single primary-bucket mode by default)
        # When disabled, hierarchical map-reduce is used for large documents
        self.model_60k_enabled = os.getenv("MODEL_60K_ENABLED", "false").lower() == "true"

        # Hierarchical summarization settings (single primary-bucket mode)
        # These thresholds remain conservative to fit the default local bucket safely.
        # Best practice: 1,500-3,000 tokens per chunk with 10-20% overlap
        # Reference: https://www.pinecone.io/learn/chunking-strategies/
        self.hierarchical_threshold_chars = self._get_int("HIERARCHICAL_THRESHOLD_CHARS", 8_000)  # ~2K tokens - trigger map-reduce for larger inputs
        self.hierarchical_threshold_clusters = self._get_int("HIERARCHICAL_THRESHOLD_CLUSTERS", 5)  # trigger map-reduce for many clusters
        self.hierarchical_chunk_max_chars = self._get_int("HIERARCHICAL_CHUNK_MAX_CHARS", 6_000)  # ~1.5K tokens per chunk
        self.hierarchical_chunk_overlap_ratio = self._get_float("HIERARCHICAL_CHUNK_OVERLAP_RATIO", 0.15)  # 15% overlap for context preservation

        # Recursive reduce settings for hierarchical summarization
        # When intermediate summaries exceed this limit, recursively reduce them
        self.recursive_reduce_max_chars = self._get_int("RECURSIVE_REDUCE_MAX_CHARS", 6_000)  # ~1.5K tokens, safe for the primary bucket
        self.recursive_reduce_max_depth = self._get_int("RECURSIVE_REDUCE_MAX_DEPTH", 3)  # Max recursion depth

        # Hierarchical summarization settings for single large articles
        self.hierarchical_single_article_threshold = self._get_int("HIERARCHICAL_SINGLE_ARTICLE_THRESHOLD", 20_000)
        self.hierarchical_single_article_chunk_size = self._get_int("HIERARCHICAL_SINGLE_ARTICLE_CHUNK_SIZE", 6_000)
        self.hierarchical_token_budget_percent = self._get_int("HIERARCHICAL_TOKEN_BUDGET_PERCENT", 75)

        # Model routing settings (primary bucket + 60K expansion bucket)
        self.model_routing_enabled = os.getenv("MODEL_ROUTING_ENABLED", "true").lower() == "true"
        # Base model name (e.g., "gemma4-e4b-q4km") - will be auto-mapped to bucket models
        self.model_base_name = os.getenv("MODEL_BASE_NAME", "gemma4-e4b-q4km")
        # MODEL_8K_NAME: use base model directly; num_ctx is set via API options (get_llm_options)
        # Gemma4 E4B (8B params) is too large for Modelfile-based num_ctx=12K on 8GB VRAM
        self.model_8k_name = os.getenv("MODEL_8K_NAME", "gemma4-e4b-q4km")
        self.model_60k_name = os.getenv("MODEL_60K_NAME", "gemma4-e4b-60k")
        self.token_safety_margin_percent = self._get_int("TOKEN_SAFETY_MARGIN_PERCENT", 10)
        self.token_safety_margin_fixed = self._get_int("TOKEN_SAFETY_MARGIN_FIXED", 512)
        self.oom_detection_enabled = os.getenv("OOM_DETECTION_ENABLED", "true").lower() == "true"
        self.warmup_enabled = os.getenv("WARMUP_ENABLED", "true").lower() == "true"
        self.warmup_keep_alive_minutes = self._get_int("WARMUP_KEEP_ALIVE_MINUTES", 30)

        # Cache settings (Redis)
        self.cache_enabled = os.getenv("CACHE_ENABLED", "false").lower() == "true"
        self.cache_redis_url = os.getenv("CACHE_REDIS_URL", "redis://localhost:6379/0")
        self.cache_ttl_seconds = self._get_int("CACHE_TTL_SECONDS", 86400)  # 24 hours

        # Hybrid Scheduling Configuration (RT/BE with reserved slots and aging)
        # See: https://arxiv.org/html/2504.09590v1 (Hybrid RT/BE Scheduling)
        self.scheduling_rt_reserved_slots = self._get_int("SCHEDULING_RT_RESERVED_SLOTS", 1)
        self.scheduling_aging_threshold_seconds = self._get_float(
            "SCHEDULING_AGING_THRESHOLD_SECONDS", 60.0
        )
        self.scheduling_aging_boost = self._get_float("SCHEDULING_AGING_BOOST", 0.5)

        # Preemption settings (application-level preemption for RT priority)
        # See: https://arxiv.org/html/2503.09304v1 (QLLM Preemption)
        self.scheduling_preemption_enabled = (
            os.getenv("SCHEDULING_PREEMPTION_ENABLED", "true").lower() == "true"
        )
        self.scheduling_preemption_wait_threshold_seconds = self._get_float(
            "SCHEDULING_PREEMPTION_WAIT_THRESHOLD_SECONDS", 2.0
        )

        # Priority promotion settings (BE -> RT after long wait)
        # After this threshold, BE requests are promoted to RT queue to prevent starvation
        # Default: 120 seconds - must be well below backend timeout (300s) to allow
        # promoted requests time to complete before upstream cancellation
        self.scheduling_priority_promotion_threshold_seconds = self._get_float(
            "SCHEDULING_PRIORITY_PROMOTION_THRESHOLD_SECONDS", 120.0
        )

        # Guaranteed bandwidth settings (anti-starvation for BE requests)
        # BE request is guaranteed to be processed after this many consecutive RT releases
        # Default: 5 (80% RT, 20% BE guaranteed bandwidth)
        # Set to 0 to disable guaranteed bandwidth
        self.scheduling_guaranteed_be_ratio = self._get_int(
            "SCHEDULING_GUARANTEED_BE_RATIO", 5
        )

        # Queue depth limit (backpressure)
        # When set > 0, reject new requests with QueueFullError when queue exceeds this depth
        # Default: 10 (fail-fast to prevent long queue waits with limited GPU slots)
        self.max_queue_depth = self._get_int("MAX_QUEUE_DEPTH", 10)

        # RT queue scheduling mode: "fifo" (default) or "lifo"
        # LIFO processes newest requests first, optimizing for swipe-feed UIs
        # where the user's current view should get priority
        self.scheduling_rt_mode = os.getenv("SCHEDULING_RT_MODE", "fifo").lower()

        # Distributed BE dispatch (default OFF for OSS compatibility)
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

        # Build bucket model names set for quick lookup
        self._bucket_model_names = {
            self.model_8k_name,
            self.model_60k_name,
        }

        logger.info(
            "News creator configuration initialized",
            extra={
                "auth_service_url": self.auth_service_url,
                "service_name": self.service_name,
                "llm_service_url": self.llm_service_url,
                "model": self.model_name,
                "ollama_request_concurrency": self.ollama_request_concurrency,
                "ollama_concurrency_source": self._ollama_concurrency_source,
                "ollama_num_parallel": os.getenv("OLLAMA_NUM_PARALLEL"),
                "scheduling_rt_reserved_slots": self.scheduling_rt_reserved_slots,
                "scheduling_aging_threshold_seconds": self.scheduling_aging_threshold_seconds,
                "scheduling_priority_promotion_threshold_seconds": self.scheduling_priority_promotion_threshold_seconds,
                "scheduling_guaranteed_be_ratio": self.scheduling_guaranteed_be_ratio,
                "scheduling_rt_mode": self.scheduling_rt_mode,
            },
        )

    def is_base_model_name(self, model_name: str) -> bool:
        """
        Check if the given model name is the base model name.

        Args:
            model_name: Model name to check

        Returns:
            True if the model name is the base model name
        """
        return model_name == self.model_base_name

    def is_bucket_model_name(self, model_name: str) -> bool:
        """
        Check if the given model name is a bucket model name.

        Args:
            model_name: Model name to check

        Returns:
            True if the model name is a bucket model name
        """
        return model_name in self._bucket_model_names

    def get_keep_alive_for_model(self, model_name: str) -> Union[int, str]:
        """
        Get keep_alive value for a specific model based on best practices.

        Args:
            model_name: Model name to get keep_alive for

        Returns:
            keep_alive value (int for seconds, str for duration like "24h", "30m")
        """
        if model_name == self.model_8k_name:
            # 8K model: on-demand, use 24h to allow unloading after use
            return self.llm_keep_alive_8k
        elif model_name == self.model_60k_name:
            # 60K model: on-demand, use 15m to allow quick unloading after use
            return self.llm_keep_alive_60k
        else:
            # Unknown model: use default keep_alive (backward compatibility)
            return self.llm_keep_alive

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

    def get_llm_options(self) -> dict:
        """Get LLM options as a dictionary (for Ollama 'options')."""
        return {
            "num_ctx": self.llm_num_ctx,
            "num_predict": self.llm_num_predict,
            "num_batch": self.llm_num_batch,  # バッチサイズ追加
            "temperature": self.llm_temperature,
            "top_p": self.llm_top_p,
            "top_k": self.llm_top_k,
            "repeat_penalty": self.llm_repeat_penalty,
            "num_keep": self.llm_num_keep,
            "stop": list(self.llm_stop_tokens),
        }
