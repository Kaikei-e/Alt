"""OpenTelemetry log exporter and structlog bridge for acolyte-orchestrator."""

from __future__ import annotations

import json
import os
import re
import sys
import threading
import time
from contextlib import suppress
from dataclasses import dataclass
from pathlib import Path
from typing import TYPE_CHECKING

import requests as _requests
import structlog
from opentelemetry._logs import SeverityNumber
from opentelemetry.exporter.otlp.proto.http._log_exporter import OTLPLogExporter
from opentelemetry.sdk._logs import LoggerProvider
from opentelemetry.sdk._logs.export import BatchLogRecordProcessor
from opentelemetry.sdk.resources import Resource

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

# RFC 7235 token68: base64url chars followed by optional '=' padding.
_TOKEN68_RE = re.compile(r"^[A-Za-z0-9\-._~+/]+=*\Z")

_DEFAULT_TIMEOUT_S = 7.0  # finite cap; SDK supplies its own value when calling post()
_EXPORTER_TIMEOUT_S = 2.0  # per-batch export budget so unreachable aggregator fails fast
_SHUTDOWN_TIMEOUT_S = 3.0  # hard cap on LoggerProvider.shutdown() drain inside 10s stop grace

_SEVERITY_MAP: dict[str, tuple[SeverityNumber, str]] = {
    "debug": (SeverityNumber.DEBUG, "DEBUG"),
    "info": (SeverityNumber.INFO, "INFO"),
    "warning": (SeverityNumber.WARN, "WARN"),
    "warn": (SeverityNumber.WARN, "WARN"),
    "error": (SeverityNumber.ERROR, "ERROR"),
    "exception": (SeverityNumber.ERROR, "ERROR"),
    "critical": (SeverityNumber.FATAL, "FATAL"),
    "fatal": (SeverityNumber.FATAL, "FATAL"),
}

_EXCLUDED_ATTR_KEYS = frozenset({"event", "level", "timestamp", "exc_info"})
_EXC_INFO_TUPLE_LEN = 3


def _noop_shutdown() -> None:
    return None


def _cap_timeout(timeout: object) -> float | tuple[float, ...]:
    """Cap finite connection and read timeouts at 7.0 seconds.

    Preserves shorter requested values. Avoids unbounded fallback.
    """
    if timeout is None:
        return _DEFAULT_TIMEOUT_S
    if isinstance(timeout, int | float):
        return min(float(timeout), _DEFAULT_TIMEOUT_S)
    if isinstance(timeout, tuple | list):
        return tuple(
            min(float(t), _DEFAULT_TIMEOUT_S) if isinstance(t, int | float) else _DEFAULT_TIMEOUT_S for t in timeout
        )
    return _DEFAULT_TIMEOUT_S


@dataclass(frozen=True)
class OTelHandle:
    """Scoped OpenTelemetry handle returned by init_otel_provider."""

    logger_provider: LoggerProvider | None
    shutdown: Callable[[], None]


class _NoRedirectSession(_requests.Session):
    """requests.Session subclass that never follows HTTP redirects (D02 contract)."""

    def request(self, method: str | bytes, url: str | bytes, **kwargs: object) -> _requests.Response:  # type: ignore[override]
        kwargs["allow_redirects"] = False
        t = kwargs.get("timeout")
        if t is None and "timeout" not in kwargs:
            kwargs["timeout"] = _DEFAULT_TIMEOUT_S
        else:
            kwargs["timeout"] = _cap_timeout(t)
        return super().request(method, url, **kwargs)  # type: ignore[arg-type]


def load_rask_ingest_token() -> str:
    """Load nonempty startup token from RASK_INGEST_TOKEN_FILE (D02 contract).

    Never logs or includes the credential in exception messages.
    """
    token_file = os.getenv("RASK_INGEST_TOKEN_FILE", "/run/secrets/rask_ingest_token")
    try:
        raw = Path(token_file).read_text(encoding="utf-8")
    except OSError as exc:
        msg = f"RASK_INGEST_TOKEN_FILE ({token_file}) could not be read: {exc}"
        raise RuntimeError(msg) from exc
    except UnicodeDecodeError:
        msg = f"RASK_INGEST_TOKEN_FILE ({token_file}) contains invalid UTF-8"
        raise RuntimeError(msg) from None

    token = raw.strip()
    if not token or not _TOKEN68_RE.match(token):
        msg = f"RASK_INGEST_TOKEN_FILE ({token_file}) contains invalid characters"
        raise RuntimeError(msg)
    return token


class OTelConfig:
    """OpenTelemetry configuration loaded from environment variables."""

    service_name: str
    service_version: str
    environment: str
    otlp_endpoint: str
    enabled: bool

    def __init__(self) -> None:
        raw_enabled = os.getenv("OTEL_ENABLED")
        if raw_enabled is None:
            msg = "OTEL_ENABLED must be explicitly set to 'true' or 'false'"
            raise RuntimeError(msg)
        normalized = raw_enabled.strip().lower()
        if normalized == "true":
            self.enabled = True
        elif normalized == "false":
            self.enabled = False
        else:
            msg = f"OTEL_ENABLED must be 'true' or 'false', got {raw_enabled!r}"
            raise RuntimeError(msg)

        self.service_name = os.getenv("OTEL_SERVICE_NAME", "acolyte-orchestrator")
        self.service_version = os.getenv("SERVICE_VERSION", "0.1.0")
        self.environment = os.getenv("DEPLOYMENT_ENV", "development")

        raw_endpoint = os.getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
        if self.enabled:
            if raw_endpoint is None or not raw_endpoint.strip():
                msg = "OTEL_EXPORTER_OTLP_ENDPOINT must be set when OTEL_ENABLED=true"
                raise RuntimeError(msg)
            self.otlp_endpoint = raw_endpoint.strip().rstrip("/")
        else:
            self.otlp_endpoint = (raw_endpoint or "").strip().rstrip("/")


def _format_exception_value(exc: BaseException) -> str:
    module = type(exc).__module__
    qualname = type(exc).__qualname__
    exc_type = f"{module}.{qualname}" if module and module != "builtins" else qualname
    text = str(exc)
    return f"{exc_type}: {text}" if text else exc_type


def _resolve_exception(exc_info: object) -> BaseException | None:
    if isinstance(exc_info, BaseException):
        return exc_info
    if isinstance(exc_info, tuple) and len(exc_info) == _EXC_INFO_TUPLE_LEN:
        _, exc_val, _ = exc_info
        if isinstance(exc_val, BaseException):
            return exc_val
        return None
    if exc_info is True:
        _, current_exc, _ = sys.exc_info()
        return current_exc
    return None


def _serialize_attr_value(value: object) -> bool | int | float | str:
    if isinstance(value, bool | int | float | str):
        return value
    if isinstance(value, BaseException):
        return _format_exception_value(value)
    if isinstance(value, bytes):
        return value.decode("utf-8", "replace")
    try:
        return json.dumps(value, default=str) if isinstance(value, dict | list | tuple | set) else str(value)
    except Exception:  # noqa: BLE001 — fall back to repr when __str__ raises
        try:
            return repr(value)
        except Exception:  # noqa: BLE001
            return "<unprintable>"


def make_otel_log_processor(
    logger_provider: LoggerProvider,
) -> Callable[[object, str, MutableMapping[str, object]], MutableMapping[str, object]]:
    """Create a structlog processor that forwards log events to the given LoggerProvider."""
    otel_logger = logger_provider.get_logger("acolyte")

    def _processor(
        _logger: object,
        method_name: str,
        event_dict: MutableMapping[str, object],
    ) -> MutableMapping[str, object]:
        with suppress(Exception):
            level_raw = str(event_dict.get("level") or method_name).lower()
            severity_number, severity_text = _SEVERITY_MAP.get(level_raw, (SeverityNumber.INFO, "INFO"))
            body = str(event_dict.get("event", ""))
            resolved_exc = _resolve_exception(event_dict.get("exc_info"))
            attributes: dict[str, bool | int | float | str] = {}
            for key, value in event_dict.items():
                if key in _EXCLUDED_ATTR_KEYS or value is None:
                    continue
                if isinstance(value, BaseException) and resolved_exc is None:
                    resolved_exc = value
                attributes[key] = _serialize_attr_value(value)

            now_ns = time.time_ns()
            otel_logger.emit(
                timestamp=now_ns,
                observed_timestamp=now_ns,
                severity_number=severity_number,
                severity_text=severity_text,
                body=body,
                attributes=attributes,
                exception=resolved_exc,
            )
        return event_dict

    return _processor


def init_otel_provider(config: OTelConfig | None = None) -> OTelHandle:
    """Initialize OpenTelemetry LoggerProvider and OTLP HTTP log exporter."""
    if config is None:
        config = OTelConfig()

    log = structlog.get_logger(__name__)
    if not config.enabled:
        log.info("otel_logs_disabled", service_name=config.service_name)
        return OTelHandle(logger_provider=None, shutdown=_noop_shutdown)

    rask_token = load_rask_ingest_token()
    otlp_headers = {"Authorization": f"Bearer {rask_token}"}

    resource = Resource.create(
        {
            "service.name": config.service_name,
            "service.version": config.service_version,
            "deployment.environment": config.environment,
        }
    )

    log_session = _NoRedirectSession()
    logger_provider = LoggerProvider(resource=resource)
    log_exporter = OTLPLogExporter(
        endpoint=f"{config.otlp_endpoint}/v1/logs",
        headers=otlp_headers,
        timeout=_EXPORTER_TIMEOUT_S,
        session=log_session,
    )
    logger_provider.add_log_record_processor(BatchLogRecordProcessor(log_exporter))

    log.info(
        "otel_logs_enabled",
        service_name=config.service_name,
        otlp_endpoint=config.otlp_endpoint,
    )

    def shutdown() -> None:
        worker = threading.Thread(
            target=logger_provider.shutdown,
            name="acolyte-otel-shutdown",
            daemon=True,
        )
        try:
            worker.start()
            worker.join(timeout=_SHUTDOWN_TIMEOUT_S)
            if worker.is_alive():
                with suppress(Exception):
                    log_exporter.shutdown()
        finally:
            log_session.close()

    return OTelHandle(logger_provider=logger_provider, shutdown=shutdown)
