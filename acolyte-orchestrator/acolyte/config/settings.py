"""Application settings loaded from environment variables."""

from __future__ import annotations

import json
from dataclasses import dataclass
from pathlib import Path
from urllib.parse import urlparse, urlunparse
from uuid import UUID

from pydantic_settings import BaseSettings


@dataclass(frozen=True, slots=True)
class NotificationRelayConfig:
    """Everything the relay needs, validated once at startup."""

    user_id: UUID
    datahub_url: str
    interval_seconds: float
    batch_size: int
    ttl_seconds: int
    cert_file: str
    key_file: str
    ca_file: str


def _safe_load_quota_json(raw: str) -> dict[str, object] | None:
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
    acolyte_db_dsn: str
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

    # Peer identity middleware — strict=True rejects missing/unknown peer CN.
    # False during mTLS rollout; flip via PEER_IDENTITY_STRICT=true at cutover.
    peer_identity_strict: bool = False

    # Content store — bounded LRU cache for article bodies (process-global,
    # shared across every run; see MemoryContentStore docstring).
    content_store_max_size: int = 2000

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

    # Notification outbox — the producer (report completion) and the relay to
    # alt-data-hub are one switch: a producer without a relay only grows a
    # backlog, and a relay without a producer has nothing to forward.
    notifications_enabled: bool = False
    # acolyte-db has no owner column — reports are not per-user here — so the
    # recipient of a report-ready ping is configuration, not a row.
    notification_user_id: str = ""
    datahub_url: str = ""
    notification_relay_interval_seconds: float = 5.0
    notification_relay_batch_size: int = 20
    # A report-ready ping nobody delivered within a day is not worth waking a
    # device for; DataHub expires it instead.
    notification_ttl_seconds: int = 86400

    # Outbound mTLS material (also read directly by acolyte.infra.mtls_client
    # for the httpx callers).
    mtls_enforce: bool = False
    mtls_cert_file: str = ""
    mtls_key_file: str = ""
    mtls_ca_file: str = ""

    model_config = {"env_prefix": "", "case_sensitive": False}

    def resolve_notification_relay_config(self) -> NotificationRelayConfig | None:
        """Validate the relay configuration, or explain exactly what is missing.

        Returns None when notifications are switched off — an answer, not a
        failure. Everything else raises: a relay that boots without a target,
        without a recipient or without a client certificate cannot forward
        anything, and the only symptom would be an outbox that quietly grows.
        """
        if not self.notifications_enabled:
            return None

        try:
            user_id = UUID(self.notification_user_id)
        except ValueError as exc:
            raise RuntimeError(  # noqa: TRY003 — startup config error, single call site
                f"NOTIFICATIONS_ENABLED=true requires NOTIFICATION_USER_ID to be a UUID "
                f"(got {self.notification_user_id!r})"
            ) from exc

        if not self.datahub_url:
            raise RuntimeError(  # noqa: TRY003 — startup config error, single call site
                "NOTIFICATIONS_ENABLED=true requires DATAHUB_URL (e.g. https://alt-data-hub:9443)"
            )

        if not self.mtls_enforce:
            raise RuntimeError(  # noqa: TRY003 — startup config error, single call site
                "NOTIFICATIONS_ENABLED=true requires MTLS_ENFORCE=true: alt-data-hub always "
                "verifies a client certificate, so an unauthenticated relay fails every handshake"
            )

        for env_name, value in (
            ("MTLS_CERT_FILE", self.mtls_cert_file),
            ("MTLS_KEY_FILE", self.mtls_key_file),
            ("MTLS_CA_FILE", self.mtls_ca_file),
        ):
            if not value or not Path(value).is_file():
                raise RuntimeError(  # noqa: TRY003 — startup config error, single call site
                    f"NOTIFICATIONS_ENABLED=true requires a readable {env_name} (got {value!r})"
                )

        return NotificationRelayConfig(
            user_id=user_id,
            datahub_url=self.datahub_url.rstrip("/"),
            interval_seconds=self.notification_relay_interval_seconds,
            batch_size=self.notification_relay_batch_size,
            ttl_seconds=self.notification_ttl_seconds,
            cert_file=self.mtls_cert_file,
            key_file=self.mtls_key_file,
            ca_file=self.mtls_ca_file,
        )

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
                password = Path(self.acolyte_db_password_file).read_text().strip()
            except OSError as exc:
                raise RuntimeError(  # noqa: TRY003 — fail-fast startup config error, single call site
                    f"Failed to read acolyte_db_password_file={self.acolyte_db_password_file!r}: {exc}"
                ) from exc
            # Replace password placeholder in DSN
            parsed = urlparse(self.acolyte_db_dsn)
            replaced = parsed._replace(netloc=f"{parsed.username}:{password}@{parsed.hostname}:{parsed.port}")
            return str(urlunparse(replaced))
        return self.acolyte_db_dsn
