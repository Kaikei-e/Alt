"""Application settings loaded from environment variables."""

from __future__ import annotations

import json
from dataclasses import dataclass
from pathlib import Path
from urllib.parse import urlparse, urlunparse
from uuid import UUID

from cryptography.exceptions import UnsupportedAlgorithm
from cryptography.hazmat.primitives import serialization
from cryptography.hazmat.primitives.asymmetric.ed25519 import Ed25519PublicKey
from pydantic import AliasChoices, Field
from pydantic_settings import BaseSettings


@dataclass(frozen=True, slots=True)
class NotificationRelayConfig:
    """Everything the relay needs, validated once at startup."""

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
    host: str = "0.0.0.0"  # nosec B104
    port: int = 8090
    log_level: str = "info"

    # Database
    acolyte_db_dsn: str
    acolyte_db_password_file: str = ""

    # External services
    news_creator_url: str = "https://news-creator:9443"
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

    # Wave 4 in-process inbound mTLS. Default off so the Pattern B sidecar
    # can keep owning :9443 until compose/pki.yaml drops PROXY_LISTEN.
    # When true, the process binds INBOUND_TLS_HOST:INBOUND_TLS_PORT
    # (default 0.0.0.0:9443) with a long-lived SSLContext. The existing
    # host:port plaintext listener stays for in-container health and is
    # published on loopback only — it is not an external mTLS fallback.
    inbound_tls_enabled: bool = False
    inbound_tls_host: str = "0.0.0.0"  # nosec B104
    inbound_tls_port: int = 9443
    inbound_tls_cert_file: str = ""
    inbound_tls_key_file: str = ""
    inbound_tls_ca_file: str = ""

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

    # Backend token verification for user identity (X-Alt-Backend-Token).
    # Startup fails loudly when BACKEND_TOKEN_VERIFICATION is enabled and
    # BACKEND_TOKEN_VERIFY_KEY_FILE is missing, unreadable, or not an Ed25519 public key.
    backend_token_verify_key_file: str = "/run/secrets/backend_token_verify_key"  # noqa: S105 — filesystem path to a public key file
    backend_token_issuer: str = "auth-hub"  # noqa: S105 — JWT issuer claim, not a secret
    backend_token_audience: str = "alt-backend"  # noqa: S105 — JWT audience claim, not a secret
    backend_token_verification: str = "enabled"  # noqa: S105 — verification mode flag, not a secret
    user_identity_dev_user_id: str = ""

    # Legacy report owner backfill & startup gate
    legacy_report_owner_id: str = Field(
        default="",
        validation_alias=AliasChoices(
            "ACOLYTE_LEGACY_REPORT_OWNER_ID",
            "acolyte_legacy_report_owner_id",
            "legacy_report_owner_id",
        ),
    )
    legacy_report_mapping_file: str = Field(
        default="",
        validation_alias=AliasChoices(
            "ACOLYTE_LEGACY_REPORT_MAPPING_FILE",
            "acolyte_legacy_report_mapping_file",
            "legacy_report_mapping_file",
        ),
    )

    model_config = {"env_prefix": "", "case_sensitive": False}

    def resolve_backend_token_verify_key(self) -> Ed25519PublicKey | None:
        """Load the Ed25519 public key that verifies the backend token.

        Returns None only when backend_token_verification is explicitly
        'disabled'. Otherwise requires a readable SPKI PEM file holding an
        Ed25519 public key.
        """
        if self.backend_token_verification.strip().lower() == "disabled":
            return None

        if not self.backend_token_verify_key_file:
            raise RuntimeError(  # noqa: TRY003 — startup config error, single call site
                "BACKEND_TOKEN_VERIFY_KEY_FILE is not configured: "
                "user identity verification requires a public key file unless BACKEND_TOKEN_VERIFICATION=disabled"
            )

        key_path = Path(self.backend_token_verify_key_file)
        try:
            pem = key_path.read_bytes()
        except OSError as exc:
            raise RuntimeError(  # noqa: TRY003 — startup config error, single call site
                f"BACKEND_TOKEN_VERIFY_KEY_FILE is missing or unreadable: {self.backend_token_verify_key_file}"
            ) from exc

        try:
            key = serialization.load_pem_public_key(pem)
        except (ValueError, UnsupportedAlgorithm) as exc:
            raise RuntimeError(  # noqa: TRY003 — startup config error, single call site
                f"BACKEND_TOKEN_VERIFY_KEY_FILE is not an Ed25519 SPKI public key: {self.backend_token_verify_key_file}"
            ) from exc
        if not isinstance(key, Ed25519PublicKey):
            raise RuntimeError(  # noqa: TRY003, TRY004 — startup config error, single call site
                f"BACKEND_TOKEN_VERIFY_KEY_FILE is not an Ed25519 public key: {self.backend_token_verify_key_file}"
            )
        return key

    def resolve_dev_user_id(self) -> UUID:
        """Resolve the dev user identity UUID when backend token verification is disabled.

        Required when BACKEND_TOKEN_VERIFICATION=disabled; raises RuntimeError if unset or invalid.
        """
        raw = self.user_identity_dev_user_id.strip()
        if not raw:
            raise RuntimeError(  # noqa: TRY003 — startup config error, single call site
                "USER_IDENTITY_DEV_USER_ID must be set when BACKEND_TOKEN_VERIFICATION=disabled"
            )
        try:
            return UUID(raw)
        except ValueError as exc:
            raise RuntimeError(  # noqa: TRY003 — startup config error, single call site
                f"USER_IDENTITY_DEV_USER_ID is not a valid UUID: {raw}"
            ) from exc

    def resolve_legacy_report_owner_id(self) -> UUID | None:
        """Resolve legacy report owner UUID when single-owner backfill is configured."""
        raw = self.legacy_report_owner_id.strip()
        if not raw:
            return None
        try:
            return UUID(raw)
        except ValueError as exc:
            raise RuntimeError(  # noqa: TRY003 — fail-fast startup config error, single call site
                f"ACOLYTE_LEGACY_REPORT_OWNER_ID is not a valid UUID: {raw!r}"
            ) from exc

    def resolve_legacy_report_mapping(self) -> dict[UUID, UUID] | None:
        """Resolve per-report mapping from JSON file when multi-tenant backfill is configured."""
        raw_path = self.legacy_report_mapping_file.strip()
        if not raw_path:
            return None
        path = Path(raw_path)
        if not path.is_file():
            raise RuntimeError(  # noqa: TRY003 — fail-fast startup config error, single call site
                f"ACOLYTE_LEGACY_REPORT_MAPPING_FILE is missing or unreadable: {raw_path!r}"
            )
        try:
            raw_data = json.loads(path.read_text())
        except Exception as exc:
            raise RuntimeError(  # noqa: TRY003 — fail-fast startup config error, single call site
                f"ACOLYTE_LEGACY_REPORT_MAPPING_FILE contains invalid JSON: {exc}"
            ) from exc
        if not isinstance(raw_data, dict):
            raise TypeError(  # noqa: TRY003 — non-dict mapping is a type error
                "ACOLYTE_LEGACY_REPORT_MAPPING_FILE must contain a JSON object mapping report_id to user_id"
            )
        mapping: dict[UUID, UUID] = {}
        for k, v in raw_data.items():
            if not isinstance(k, str) or not isinstance(v, str):
                raise TypeError(  # noqa: TRY003 — non-string key or value in mapping
                    f"ACOLYTE_LEGACY_REPORT_MAPPING_FILE keys and values must be string UUIDs, got key {k!r} "
                    f"({type(k).__name__}) and value {v!r} ({type(v).__name__})"
                )
            try:
                k_uuid = UUID(k)
                v_uuid = UUID(v)
            except ValueError as exc:
                raise ValueError(  # noqa: TRY003 — malformed UUID string
                    f"ACOLYTE_LEGACY_REPORT_MAPPING_FILE keys and values must be valid UUIDs: {exc}"
                ) from exc
            mapping[k_uuid] = v_uuid
        return mapping

    def resolve_notification_relay_config(self) -> NotificationRelayConfig | None:
        """Validate the relay configuration, or explain exactly what is missing.

        Returns None when notifications are switched off — an answer, not a
        failure. Everything else raises: a relay that boots without a target
        or without a client certificate cannot forward
        anything, and the only symptom would be an outbox that quietly grows.
        """
        if not self.notifications_enabled:
            return None

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
