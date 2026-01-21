"""Configuration module for News Creator service."""

import os
import logging
from typing import List, Union

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
        self.model_name = os.getenv("LLM_MODEL", "gemma3:4b-it-qat")
        self.llm_timeout_seconds = self._get_int("LLM_TIMEOUT_SECONDS", 300)  # 5分に増加（1000トークン生成 + 続き生成に対応）
        self.llm_keep_alive = self._get_int("LLM_KEEP_ALIVE_SECONDS", "24h")
        # Model-specific keep_alive settings (best practice: 12K/60K on-demand)
        # 12K model: 24h to allow unloading after use to save VRAM
        # 60K model: 15m to allow quick unloading after use to save VRAM
        # self.llm_keep_alive_8k = os.getenv("LLM_KEEP_ALIVE_8K", "0")  # 8kモデルは使用しない
        self.llm_keep_alive_12k = os.getenv("LLM_KEEP_ALIVE_12K", "24h")
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

        # ---- Generation parameters (Gemma3 + Ollama options) ----
        # Default: 12K context for normal AI Summary (60K is used only for Recap)
        self.llm_num_ctx = self._get_int("LLM_NUM_CTX", 12288)
        # RTX 4060最適化: バッチサイズ1024（entrypoint.shのOLLAMA_NUM_BATCHと統一）
        self.llm_num_batch = self._get_int("LLM_NUM_BATCH", 1024)
        self.llm_num_predict = self._get_int("LLM_NUM_PREDICT", 1200)  # 復活
        # 調査に基づく推奨値に更新: 繰り返し問題対策
        self.llm_temperature = self._get_float("LLM_TEMPERATURE", 0.15)  # 0.2 → 0.15
        self.llm_top_p = self._get_float("LLM_TOP_P", 0.85)             # 0.9 → 0.85
        self.llm_top_k = self._get_int("LLM_TOP_K", 40)                 # 50 → 40
        self.llm_repeat_penalty = self._get_float("LLM_REPEAT_PENALTY", 1.15)  # 1.07 → 1.15
        self.llm_num_keep = self._get_int("LLM_NUM_KEEP", -1)          # system保持

        # Stop tokens（Gemma3 は <start_of_turn>/<end_of_turn>）
        # 既定は Gemma3 正式トークンのみ。空なら安全に補充。
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
        self.summary_temperature = self._get_float("SUMMARY_TEMPERATURE", 0.1)  # サマリー生成専用の低い温度

        # Repetition detection and retry settings
        self.max_repetition_retries = self._get_int("MAX_REPETITION_RETRIES", 3)
        self.repetition_threshold = self._get_float("REPETITION_THRESHOLD", 0.3)

        # Hierarchical summarization settings
        # Threshold for switching to hierarchical (map-reduce) summarization
        self.hierarchical_threshold_chars = self._get_int("HIERARCHICAL_THRESHOLD_CHARS", 200_000)  # ~50K tokens
        self.hierarchical_threshold_clusters = self._get_int("HIERARCHICAL_THRESHOLD_CLUSTERS", 15)
        self.hierarchical_chunk_max_chars = self._get_int("HIERARCHICAL_CHUNK_MAX_CHARS", 100_000)  # ~25K tokens per chunk

        # Hierarchical summarization settings for single large articles
        self.hierarchical_single_article_threshold = self._get_int("HIERARCHICAL_SINGLE_ARTICLE_THRESHOLD", 25_000)
        self.hierarchical_single_article_chunk_size = self._get_int("HIERARCHICAL_SINGLE_ARTICLE_CHUNK_SIZE", 10_000)

        # Model routing settings (2-model bucket system: 12K, 60K)
        self.model_routing_enabled = os.getenv("MODEL_ROUTING_ENABLED", "true").lower() == "true"
        # Base model name (e.g., "gemma3:4b") - will be auto-mapped to bucket models
        self.model_base_name = os.getenv("MODEL_BASE_NAME", "gemma3:4b-it-qat")
        # self.model_8k_name = os.getenv("MODEL_8K_NAME", "gemma3-4b-8k")  # 8kモデルは使用しない
        self.model_12k_name = os.getenv("MODEL_12K_NAME", "gemma3-4b-12k")
        self.model_60k_name = os.getenv("MODEL_60K_NAME", "gemma3-4b-60k")
        self.token_safety_margin_percent = self._get_int("TOKEN_SAFETY_MARGIN_PERCENT", 10)
        self.token_safety_margin_fixed = self._get_int("TOKEN_SAFETY_MARGIN_FIXED", 512)
        self.oom_detection_enabled = os.getenv("OOM_DETECTION_ENABLED", "true").lower() == "true"
        self.warmup_enabled = os.getenv("WARMUP_ENABLED", "true").lower() == "true"
        self.warmup_keep_alive_minutes = self._get_int("WARMUP_KEEP_ALIVE_MINUTES", 30)

        # Cache settings (Redis)
        self.cache_enabled = os.getenv("CACHE_ENABLED", "false").lower() == "true"
        self.cache_redis_url = os.getenv("CACHE_REDIS_URL", "redis://localhost:6379/0")
        self.cache_ttl_seconds = self._get_int("CACHE_TTL_SECONDS", 86400)  # 24 hours

        # Build bucket model names set for quick lookup
        self._bucket_model_names = {
            # self.model_8k_name,  # 8kモデルは使用しない
            self.model_12k_name,
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
        # if model_name == self.model_8k_name:  # 8kモデルは使用しない
        #     # 8K model: always loaded, use 24h or -1 (forever)
        #     return self.llm_keep_alive_8k
        if model_name == self.model_12k_name:
            # 12K model: on-demand, use 30m to allow unloading after use
            return self.llm_keep_alive_12k
        elif model_name == self.model_60k_name:
            # 60K model: on-demand, use 15m to allow quick unloading after use
            return self.llm_keep_alive_60k
        else:
            # Unknown model: use default keep_alive (backward compatibility)
            return self.llm_keep_alive

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
