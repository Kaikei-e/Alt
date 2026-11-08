"""OpenTelemetry log exporter and structlog bridge for acolyte-orchestrator (TDD RED stub)."""

from __future__ import annotations

from dataclasses import dataclass
from typing import TYPE_CHECKING

import requests as _requests
from opentelemetry.exporter.otlp.proto.http._log_exporter import OTLPLogExporter
from opentelemetry.sdk._logs import LoggerProvider
from opentelemetry.sdk._logs.export import BatchLogRecordProcessor

if TYPE_CHECKING:
    from collections.abc import Callable, MutableMapping

__all__ = [
    "BatchLogRecordProcessor",
    "LoggerProvider",
    "OTLPLogExporter",
    "OTelConfig",
    "OTelHandle",
    "_NoRedirectSession",
    "init_otel_provider",
    "load_rask_ingest_token",
    "make_otel_log_processor",
]


@dataclass(frozen=True)
class OTelHandle:
    """Scoped OpenTelemetry handle returned by init_otel_provider."""

    logger_provider: LoggerProvider | None
    shutdown: Callable[[], None]


class _NoRedirectSession(_requests.Session):
    """requests.Session subclass that never follows HTTP redirects (D02 contract)."""

    def request(self, method: str | bytes, url: str | bytes, **kwargs: object) -> _requests.Response:  # type: ignore[override]
        raise NotImplementedError


def load_rask_ingest_token() -> str:
    """Load nonempty startup token from RASK_INGEST_TOKEN_FILE (D02 contract)."""
    raise NotImplementedError


class OTelConfig:
    """OpenTelemetry configuration loaded from environment variables."""

    service_name: str
    service_version: str
    environment: str
    otlp_endpoint: str
    enabled: bool

    def __init__(self) -> None:
        raise NotImplementedError


def make_otel_log_processor(
    logger_provider: LoggerProvider,
) -> Callable[[object, str, MutableMapping[str, object]], MutableMapping[str, object]]:
    """Create a structlog processor that forwards log events to the given LoggerProvider."""
    raise NotImplementedError


def init_otel_provider(config: OTelConfig | None = None) -> OTelHandle:
    """Initialize OpenTelemetry LoggerProvider and OTLP HTTP log exporter."""
    raise NotImplementedError
