"""Application settings loaded from environment variables."""

from __future__ import annotations

from pydantic_settings import BaseSettings


class Settings(BaseSettings):
    """Acolyte orchestrator configuration."""

    # Service
    host: str = "0.0.0.0"
    port: int = 8090
    log_level: str = "info"

    # Database
    acolyte_db_dsn: str = "postgresql://postgres:password@localhost:5432/alt_db"
    acolyte_db_password_file: str = ""

    # External services
    news_creator_url: str = "http://news-creator:11434"
    search_indexer_url: str = "http://search-indexer:9300"

    # Auth
    service_secret: str = ""
    service_token_file: str = ""

    # DB pool
    db_pool_min_size: int = 2
    db_pool_max_size: int = 10

    # Job worker
    job_poll_interval_seconds: float = 5.0
    worker_id: str = "acolyte-1"

    # LLM provider selection ("ollama" or "vllm")
    llm_provider: str = "ollama"
    vllm_api_key: str = ""

    # LLM defaults
    default_model: str = "gemma4-e4b-12k"
    default_num_predict: int = 2000
    llm_num_ctx: int = 12288
    llm_stop_tokens: str = ""  # comma-separated; empty = model default

    # LLM mode defaults
    structured_temperature: float = 0.0
    structured_num_predict: int = 1024
    longform_temperature: float = 0.7
    longform_num_predict: int = 4000
    longform_think: bool = False

    # Paragraph-level generation — per-role num_predict
    paragraph_num_predict: int = 1000
    paragraph_num_predict_analysis: int = 1200
    paragraph_num_predict_conclusion: int = 1500
    paragraph_num_predict_es: int = 600

    # Fact normalization
    fact_num_predict: int = 512
    max_facts_total: int = 20

    # Checkpointer
    checkpoint_enabled: bool = False

    model_config = {"env_prefix": "", "case_sensitive": False}

    def resolve_db_dsn(self) -> str:
        """Resolve DB DSN, replacing password from file if configured."""
        if self.acolyte_db_password_file:
            try:
                with open(self.acolyte_db_password_file) as f:
                    password = f.read().strip()
                # Replace password placeholder in DSN
                from urllib.parse import urlparse, urlunparse

                parsed = urlparse(self.acolyte_db_dsn)
                replaced = parsed._replace(netloc=f"{parsed.username}:{password}@{parsed.hostname}:{parsed.port}")
                return str(urlunparse(replaced))
            except OSError:
                pass
        return self.acolyte_db_dsn

    def resolve_service_secret(self) -> str:
        """Resolve service secret from file or env var."""
        if self.service_token_file:
            try:
                with open(self.service_token_file) as f:
                    return f.read().strip()
            except OSError:
                pass
        return self.service_secret
