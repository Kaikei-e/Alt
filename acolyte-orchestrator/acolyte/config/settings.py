"""Application settings loaded from environment variables."""

from __future__ import annotations

import json

from pydantic_settings import BaseSettings


def _safe_load_quota_json(raw: str) -> dict | None:
    """Parse the section-quota JSON and return None on any structural issue.

    Wrapping the two-type ``except`` avoids ``ruff format`` 0.15.9 removing
    the parens from ``except (ValueError, TypeError):`` (an upstream bug).
    """
    try:
        parsed = json.loads(raw)
    except ValueError:
        return None
    if not isinstance(parsed, dict):
        return None
    return parsed


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

    # Language quota applied by Curator after LLM ranking.
    # Format: {"<bcp47_short>": <min_share_0_to_1>}; 0.0 disables enforcement.
    language_quota_en: float = 0.2

    # Per-section quota overrides: JSON encoding of
    #   {"{report_type}:{section_role}": {"en": 0.3}, "_default": {"en": 0.2}}
    # Unknown keys fall through to ``_default``, then to ``language_quota_en``.
    # An empty string disables per-section routing.
    section_language_quota_json: str = ""

    # HyDE (Hypothetical Document Embedding) for cross-lingual recall.
    # When enabled, the Gatherer asks Gemma4 for a short target-language
    # passage per topic and injects it as an extra multi-query variant.
    hyde_enabled: bool = True
    hyde_timeout_s: float = 8.0
    hyde_max_chars: int = 600
    hyde_num_predict: int = 400

    model_config = {"env_prefix": "", "case_sensitive": False}

    def get_language_quota(
        self,
        section_role: str | None = None,
        report_type: str | None = None,
    ) -> dict[str, float]:
        """Return a fresh language quota mapping for the curator to apply.

        Lookup order when ``section_role`` and/or ``report_type`` are
        provided:
          1. Exact key ``{report_type}:{section_role}``
          2. ``_default`` entry in the JSON config
          3. Global ``language_quota_en`` fallback

        ``section_role`` and ``report_type`` are validated against short
        allowlists so malformed outlines cannot reach an attacker-controlled
        lookup.
        """
        if not self.section_language_quota_json:
            return {"en": self.language_quota_en}

        parsed = _safe_load_quota_json(self.section_language_quota_json)
        if parsed is None:
            return {"en": self.language_quota_en}

        allowed_sections = {"analysis", "conclusion", "executive_summary"}
        allowed_report_types = {"weekly_briefing", "market_analysis", "market_analysis_japan", "trend_report"}
        if section_role in allowed_sections and report_type in allowed_report_types:
            key = f"{report_type}:{section_role}"
            entry = parsed.get(key)
            if isinstance(entry, dict):
                return {str(k): float(v) for k, v in entry.items()}

        default_entry = parsed.get("_default")
        if isinstance(default_entry, dict):
            return {str(k): float(v) for k, v in default_entry.items()}

        return {"en": self.language_quota_en}

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
