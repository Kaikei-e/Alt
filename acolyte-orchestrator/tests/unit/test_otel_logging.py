"""Unit tests for acolyte-orchestrator OpenTelemetry log export and structlog bridge."""

from __future__ import annotations

import importlib
import json
import sys
import time
import tomllib
from datetime import UTC, datetime
from pathlib import Path
from typing import TYPE_CHECKING
from unittest.mock import MagicMock, patch
from uuid import UUID

import pytest
import requests
import structlog
from opentelemetry._logs import SeverityNumber
from opentelemetry.sdk._logs import LoggerProvider
from opentelemetry.sdk._logs.export import InMemoryLogRecordExporter, SimpleLogRecordProcessor
from packaging.requirements import Requirement
from packaging.version import Version
from structlog.testing import capture_logs

import main as main_module
from acolyte.infra.logging import configure_logging
from acolyte.infra.otel import (
    OTelConfig,
    OTelHandle,
    _NoRedirectSession,
    init_otel_provider,
    load_rask_ingest_token,
)

if TYPE_CHECKING:
    from collections.abc import Iterator

_VALID_TOKEN = "ValidRaskIngestToken1234567890abcdefgh="


@pytest.fixture(autouse=True)
def _isolate_structlog_config() -> Iterator[None]:
    """Snapshot and restore global structlog configuration around each test."""
    prev_config = structlog.get_config().copy()
    processors_ref = prev_config.get("processors")
    saved_processors = list(processors_ref) if isinstance(processors_ref, list) else []
    try:
        yield
    finally:
        structlog.contextvars.clear_contextvars()
        if isinstance(processors_ref, list):
            processors_ref.clear()
            processors_ref.extend(saved_processors)
        structlog.configure(
            processors=processors_ref if isinstance(processors_ref, list) else saved_processors,
            wrapper_class=prev_config.get("wrapper_class"),
            context_class=prev_config.get("context_class"),
            logger_factory=prev_config.get("logger_factory"),
            cache_logger_on_first_use=prev_config.get("cache_logger_on_first_use", True),
        )


def test_otel_config_reads_explicit_enabled_switch(monkeypatch: pytest.MonkeyPatch) -> None:
    """OTelConfig requires explicit OTEL_ENABLED=true|false and rejects unset or invalid values."""
    for false_value in ("false", "FALSE", "False"):
        monkeypatch.setenv("OTEL_ENABLED", false_value)
        cfg_disabled = OTelConfig()
        assert cfg_disabled.enabled is False
        assert cfg_disabled.service_name == "acolyte-orchestrator"

    for true_value in ("true", "TRUE", "True"):
        monkeypatch.setenv("OTEL_ENABLED", true_value)
        monkeypatch.setenv("OTEL_SERVICE_NAME", "acolyte-orchestrator")
        monkeypatch.setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://rask-log-aggregator:4318")
        cfg_enabled = OTelConfig()
        assert cfg_enabled.enabled is True
        assert cfg_enabled.service_name == "acolyte-orchestrator"
        assert cfg_enabled.otlp_endpoint == "http://rask-log-aggregator:4318"

    monkeypatch.delenv("OTEL_ENABLED", raising=False)
    with pytest.raises(RuntimeError, match="OTEL_ENABLED") as exc_unset:
        OTelConfig()
    assert exc_unset.type is RuntimeError

    for invalid_value in ("", "1", "0", "yes", "no", "invalid"):
        monkeypatch.setenv("OTEL_ENABLED", invalid_value)
        with pytest.raises(RuntimeError, match="OTEL_ENABLED") as exc_invalid:
            OTelConfig()
        assert exc_invalid.type is RuntimeError

    monkeypatch.setenv("OTEL_ENABLED", "true")
    monkeypatch.delenv("OTEL_EXPORTER_OTLP_ENDPOINT", raising=False)
    with pytest.raises(RuntimeError, match="OTEL_EXPORTER_OTLP_ENDPOINT") as exc_no_ep:
        OTelConfig()
    assert exc_no_ep.type is RuntimeError

    monkeypatch.setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "   ")
    with pytest.raises(RuntimeError, match="OTEL_EXPORTER_OTLP_ENDPOINT") as exc_blank_ep:
        OTelConfig()
    assert exc_blank_ep.type is RuntimeError


def test_otel_disabled_logs_otel_logs_disabled_and_skips_token_file(
    monkeypatch: pytest.MonkeyPatch,
    tmp_path: Path,
) -> None:
    """When OTEL_ENABLED=false, startup logs otel_logs_disabled, skips token file, and returns handle with None provider."""
    monkeypatch.setenv("OTEL_ENABLED", "false")
    monkeypatch.setenv("RASK_INGEST_TOKEN_FILE", str(tmp_path / "missing_rask_token"))

    with capture_logs() as cap_logs:
        handle = init_otel_provider()

    assert isinstance(handle, OTelHandle)
    assert handle.logger_provider is None
    assert callable(handle.shutdown)
    handle.shutdown()
    events = [entry.get("event") for entry in cap_logs]
    assert "otel_logs_disabled" in events


def test_otel_enabled_missing_or_invalid_token_fails_fast_without_leaking_secret(
    monkeypatch: pytest.MonkeyPatch,
    tmp_path: Path,
) -> None:
    """When OTEL_ENABLED=true, missing, empty, or malformed token file raises RuntimeError."""
    monkeypatch.setenv("OTEL_ENABLED", "true")
    monkeypatch.setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://rask-log-aggregator:4318")
    monkeypatch.setenv("RASK_INGEST_TOKEN_FILE", str(tmp_path / "nonexistent_token"))
    with pytest.raises(RuntimeError, match="RASK_INGEST_TOKEN_FILE") as exc_missing:
        init_otel_provider()
    assert exc_missing.type is RuntimeError

    empty_file = tmp_path / "empty_token"
    empty_file.write_text("   \n", encoding="utf-8")
    monkeypatch.setenv("RASK_INGEST_TOKEN_FILE", str(empty_file))
    with pytest.raises(RuntimeError, match="RASK_INGEST_TOKEN_FILE") as exc_empty:
        load_rask_ingest_token()
    assert exc_empty.type is RuntimeError

    bad_file = tmp_path / "bad_token"
    bad_secret = "secret with spaces not allowed in token68"
    bad_file.write_text(bad_secret, encoding="utf-8")
    monkeypatch.setenv("RASK_INGEST_TOKEN_FILE", str(bad_file))
    with pytest.raises(RuntimeError, match="RASK_INGEST_TOKEN_FILE") as exc_info:
        init_otel_provider()
    assert exc_info.type is RuntimeError
    assert bad_secret not in str(exc_info.value)


def test_otel_enabled_wires_bearer_header_and_logs_otel_logs_enabled(
    monkeypatch: pytest.MonkeyPatch,
    tmp_path: Path,
) -> None:
    """When OTEL_ENABLED=true, OTLPLogExporter gets Bearer header, handle holds LoggerProvider, and startup logs otel_logs_enabled."""
    token_file = tmp_path / "rask_ingest_token"
    token_file.write_text(f"{_VALID_TOKEN}\n", encoding="utf-8")
    monkeypatch.setenv("OTEL_ENABLED", "true")
    monkeypatch.setenv("OTEL_SERVICE_NAME", "acolyte-orchestrator")
    monkeypatch.setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://rask-log-aggregator:4318")
    monkeypatch.setenv("RASK_INGEST_TOKEN_FILE", str(token_file))

    with (
        patch("acolyte.infra.otel.OTLPLogExporter") as mock_log_exporter,
        patch("acolyte.infra.otel.BatchLogRecordProcessor") as mock_batch_processor,
        patch("acolyte.infra.otel.LoggerProvider") as mock_provider_cls,
        capture_logs() as cap_logs,
    ):
        handle = init_otel_provider()
        try:
            assert isinstance(handle, OTelHandle)
            assert handle.logger_provider is mock_provider_cls.return_value

            mock_log_exporter.assert_called_once()
            _, kwargs = mock_log_exporter.call_args
            assert kwargs["endpoint"] == "http://rask-log-aggregator:4318/v1/logs"
            assert kwargs["headers"] == {"Authorization": f"Bearer {_VALID_TOKEN}"}
            assert isinstance(kwargs.get("session"), _NoRedirectSession)
            assert 2.0 <= float(kwargs["timeout"]) <= 3.0
            mock_batch_processor.assert_called_once_with(mock_log_exporter.return_value)
            mock_provider_cls.return_value.add_log_record_processor.assert_called_once_with(
                mock_batch_processor.return_value
            )

            enabled_logs = [entry for entry in cap_logs if entry.get("event") == "otel_logs_enabled"]
            assert len(enabled_logs) == 1
            assert enabled_logs[0].get("service_name") == "acolyte-orchestrator"
            assert enabled_logs[0].get("otlp_endpoint") == "http://rask-log-aggregator:4318"
        finally:
            handle.shutdown()


def test_no_redirect_session_disables_redirects_and_caps_timeout(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    """_NoRedirectSession forces allow_redirects=False and caps timeout at 7.0s (D02)."""
    captured: dict[str, object] = {}

    def _fake_request(
        self: requests.Session,
        method: str | bytes,
        url: str | bytes,
        **kwargs: object,
    ) -> MagicMock:
        captured["method"] = method
        captured["url"] = url
        captured.update(kwargs)
        return MagicMock(spec=requests.Response)

    monkeypatch.setattr(requests.Session, "request", _fake_request)
    session = _NoRedirectSession()
    try:
        session.request("POST", "http://rask-log-aggregator:4318/v1/logs", allow_redirects=True, timeout=30.0)
        assert captured["allow_redirects"] is False
        assert captured["timeout"] == 7.0
    finally:
        session.close()


def test_structlog_events_reach_otel_logger_provider_with_attributes_and_preserve_stdout_json(
    capsys: pytest.CaptureFixture[str],
) -> None:
    """structlog events emit to the passed LoggerProvider without process-global state; logger_provider=None exports nothing."""
    exporter = InMemoryLogRecordExporter()
    provider = LoggerProvider()
    provider.add_log_record_processor(SimpleLogRecordProcessor(exporter))

    try:
        structlog.contextvars.clear_contextvars()
        configure_logging(log_level="info", logger_provider=None)
        structlog.get_logger("acolyte.pipeline").info("Unbridged event", report_id="rep-000")

        unbridged_json = json.loads(capsys.readouterr().out.strip())
        assert unbridged_json["event"] == "Unbridged event"
        assert unbridged_json["level"] == "info"
        assert unbridged_json["report_id"] == "rep-000"
        assert "timestamp" in unbridged_json
        assert len(exporter.get_finished_logs()) == 0

        configure_logging(log_level="info", logger_provider=provider)
        structlog.contextvars.bind_contextvars(run_id="run-456")

        log = structlog.get_logger("acolyte.pipeline")
        log.info("Pipeline completed", report_id="rep-123", final_version=2)
        log.warning("Critic retry requested", report_id="rep-123")

        stdout_lines = [line for line in capsys.readouterr().out.strip().splitlines() if line.strip()]
        assert len(stdout_lines) == 2

        first_json = json.loads(stdout_lines[0])
        assert first_json["event"] == "Pipeline completed"
        assert first_json["level"] == "info"
        assert first_json["report_id"] == "rep-123"
        assert first_json["run_id"] == "run-456"
        assert first_json["final_version"] == 2
        assert "timestamp" in first_json

        finished = exporter.get_finished_logs()
        assert len(finished) == 2

        first_record = finished[0].log_record
        assert first_record.body == "Pipeline completed"
        assert first_record.severity_number == SeverityNumber.INFO
        assert first_record.severity_text == "INFO"
        assert first_record.attributes is not None
        assert first_record.attributes["report_id"] == "rep-123"
        assert first_record.attributes["run_id"] == "run-456"
        assert first_record.attributes["final_version"] == 2

        second_record = finished[1].log_record
        assert second_record.body == "Critic retry requested"
        assert second_record.severity_number == SeverityNumber.WARN
        assert second_record.severity_text == "WARN"
        assert second_record.attributes is not None
        assert second_record.attributes["report_id"] == "rep-123"
        assert second_record.attributes["run_id"] == "run-456"
    finally:
        structlog.contextvars.clear_contextvars()
        provider.shutdown()


def test_shutdown_drains_within_bounded_time_without_force_flush(
    monkeypatch: pytest.MonkeyPatch,
    tmp_path: Path,
) -> None:
    """OTelHandle.shutdown omits redundant force_flush and finishes within a few seconds even when export fails."""
    token_file = tmp_path / "rask_ingest_token"
    token_file.write_text(_VALID_TOKEN, encoding="utf-8")
    monkeypatch.setenv("OTEL_ENABLED", "true")
    monkeypatch.setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://127.0.0.1:1")
    monkeypatch.setenv("RASK_INGEST_TOKEN_FILE", str(token_file))

    with (
        patch("acolyte.infra.otel.OTLPLogExporter"),
        patch("acolyte.infra.otel.BatchLogRecordProcessor"),
        patch("acolyte.infra.otel.LoggerProvider") as mock_provider_cls,
    ):
        mock_provider = mock_provider_cls.return_value
        handle = init_otel_provider()
        assert isinstance(handle, OTelHandle)
        assert handle.logger_provider is mock_provider
        handle.shutdown()

        mock_provider.force_flush.assert_not_called()
        mock_provider.shutdown.assert_called_once()

    real_handle = init_otel_provider()
    assert real_handle.logger_provider is not None
    otel_logger = real_handle.logger_provider.get_logger("acolyte.test")
    for idx in range(1200):
        otel_logger.emit(body=f"record-{idx}", severity_number=SeverityNumber.INFO, severity_text="INFO")

    start = time.monotonic()
    real_handle.shutdown()
    elapsed = time.monotonic() - start
    assert elapsed < 4.0


def test_bridge_processor_swallows_str_and_emit_errors_preserving_stdout_json(
    capsys: pytest.CaptureFixture[str],
) -> None:
    """A raising __str__ attribute or a raising Logger.emit must not break log.info or stdout JSON."""

    class _BadStr:
        def __str__(self) -> str:
            msg = "str boom"
            raise ValueError(msg)

        def __repr__(self) -> str:
            return "<BadStr>"

    exporter = InMemoryLogRecordExporter()
    provider = LoggerProvider()
    provider.add_log_record_processor(SimpleLogRecordProcessor(exporter))

    try:
        configure_logging(log_level="info", logger_provider=provider)
        log = structlog.get_logger("acolyte.safe")
        log.info("Handled bad str", bad=_BadStr(), report_id="rep-str")

        first_line = capsys.readouterr().out.strip()
        first_json = json.loads(first_line)
        assert first_json["event"] == "Handled bad str"
        assert first_json["report_id"] == "rep-str"

        otel_logger = provider.get_logger("acolyte")
        with patch.object(otel_logger, "emit", side_effect=RuntimeError("emit boom")):
            log.info("Handled emit failure", report_id="rep-emit")

        second_line = capsys.readouterr().out.strip()
        second_json = json.loads(second_line)
        assert second_json["event"] == "Handled emit failure"
        assert second_json["report_id"] == "rep-emit"
    finally:
        provider.shutdown()


def test_bridge_processor_exports_exception_details_and_preserves_exception_field_type(
    capsys: pytest.CaptureFixture[str],
) -> None:
    """log.exception and exc_info variants populate exception.* attributes; exception fields keep their type."""
    exporter = InMemoryLogRecordExporter()
    provider = LoggerProvider()
    provider.add_log_record_processor(SimpleLogRecordProcessor(exporter))

    try:
        configure_logging(log_level="info", logger_provider=provider)
        log = structlog.get_logger("acolyte.exc")

        try:
            _ = 1 / 0
        except ZeroDivisionError:
            log.exception("Divide failed", report_id="rep-div")

        try:
            _ = int("not-an-int")
        except ValueError as err:
            log.warning("Explicit exc instance", exc_info=err)
            log.warning("Explicit exc tuple", exc_info=sys.exc_info())

        log.error("Exception as field", error=KeyError("missing_key"))
        _ = capsys.readouterr()

        finished = exporter.get_finished_logs()
        assert len(finished) == 4

        div_attrs = dict(finished[0].log_record.attributes or {})
        assert div_attrs.get("exception.type") == "ZeroDivisionError"
        assert "division by zero" in str(div_attrs.get("exception.message", ""))
        assert "ZeroDivisionError" in str(div_attrs.get("exception.stacktrace", ""))

        inst_attrs = dict(finished[1].log_record.attributes or {})
        assert inst_attrs.get("exception.type") == "ValueError"

        tuple_attrs = dict(finished[2].log_record.attributes or {})
        assert tuple_attrs.get("exception.type") == "ValueError"

        field_attrs = dict(finished[3].log_record.attributes or {})
        assert "KeyError" in str(field_attrs.get("error", ""))
    finally:
        provider.shutdown()


def test_bridge_processor_serializes_non_primitive_attributes_as_json(
    capsys: pytest.CaptureFixture[str],
) -> None:
    """Container attributes (dict/list) use json.dumps(v, default=str); bytes decode; primitives stay as-is."""
    exporter = InMemoryLogRecordExporter()
    provider = LoggerProvider()
    provider.add_log_record_processor(SimpleLogRecordProcessor(exporter))

    try:
        configure_logging(log_level="info", logger_provider=provider)
        log = structlog.get_logger("acolyte.json_attrs")
        log.info(
            "Structured attributes",
            meta={"a": 1, "b": "two"},
            tags=["x", "y"],
            raw=b"bytes",
            count=3,
            flag=True,
            ratio=0.5,
            label="ok",
        )
        _ = capsys.readouterr()

        finished = exporter.get_finished_logs()
        assert len(finished) == 1
        attrs = dict(finished[0].log_record.attributes or {})
        assert attrs["meta"] == json.dumps({"a": 1, "b": "two"}, default=str)
        assert attrs["tags"] == json.dumps(["x", "y"], default=str)
        assert attrs["raw"] == "bytes"
        assert attrs["count"] == 3
        assert attrs["flag"] is True
        assert attrs["ratio"] == 0.5
        assert attrs["label"] == "ok"
    finally:
        provider.shutdown()


def test_serialize_attr_value_exports_uuid_and_datetime_without_quotes(
    capsys: pytest.CaptureFixture[str],
) -> None:
    """UUID, datetime, and bytes attributes export without JSON quotes while dicts export as JSON."""
    exporter = InMemoryLogRecordExporter()
    provider = LoggerProvider()
    provider.add_log_record_processor(SimpleLogRecordProcessor(exporter))

    report_uuid = UUID("00000000-0000-0000-0000-000000000001")
    created_at = datetime(2026, 11, 9, 1, 0, tzinfo=UTC)

    try:
        configure_logging(log_level="info", logger_provider=provider)
        log = structlog.get_logger("acolyte.scalar_attrs")
        log.info(
            "Scalar attributes",
            report_id=report_uuid,
            created_at=created_at,
            meta={"k": "v"},
            payload=b"hello",
        )
        _ = capsys.readouterr()

        finished = exporter.get_finished_logs()
        assert len(finished) == 1
        attrs = dict(finished[0].log_record.attributes or {})
        assert attrs["report_id"] == "00000000-0000-0000-0000-000000000001"
        assert attrs["created_at"] == str(created_at)
        assert attrs["meta"] == json.dumps({"k": "v"}, default=str)
        assert attrs["payload"] == "hello"
    finally:
        provider.shutdown()


def test_resolve_exception_missing_instance_does_not_instantiate_or_drop_record(
    capsys: pytest.CaptureFixture[str],
) -> None:
    """exc_info=(Type, None, None) with a required-arg exception class returns None and still exports the record."""

    class _RequiredArgError(Exception):
        def __init__(self, code: int) -> None:
            super().__init__(f"error {code}")
            self.code = code

    exporter = InMemoryLogRecordExporter()
    provider = LoggerProvider()
    provider.add_log_record_processor(SimpleLogRecordProcessor(exporter))

    try:
        configure_logging(log_level="info", logger_provider=provider)
        log = structlog.get_logger("acolyte.exc_tuple")
        log.error(
            "Missing exception instance",
            exc_info=(_RequiredArgError, None, None),
            report_id="rep-missing-exc",
        )
        _ = capsys.readouterr()

        finished = exporter.get_finished_logs()
        assert len(finished) == 1
        record = finished[0].log_record
        assert record.body == "Missing exception instance"
        attrs = dict(record.attributes or {})
        assert attrs["report_id"] == "rep-missing-exc"
        assert "exception.type" not in attrs
    finally:
        provider.shutdown()


def test_configure_logging_enables_cache_logger_on_first_use() -> None:
    """configure_logging keeps cache_logger_on_first_use=True for production hot-path performance."""
    configure_logging(log_level="info", logger_provider=None)
    assert structlog.get_config()["cache_logger_on_first_use"] is True


def test_pyproject_declares_minimum_otel_sdk_floor() -> None:
    """pyproject.toml declares a lower bound >=1.45.1 for opentelemetry-sdk and opentelemetry-exporter-otlp-proto-http."""
    pyproject_path = Path(__file__).resolve().parents[2] / "pyproject.toml"
    data = tomllib.loads(pyproject_path.read_text(encoding="utf-8"))
    reqs = {Requirement(dep).name: Requirement(dep) for dep in data["project"]["dependencies"]}
    min_version = Version("1.45.1")
    for pkg in ("opentelemetry-sdk", "opentelemetry-exporter-otlp-proto-http"):
        assert pkg in reqs
        lower_bounds = [Version(s.version) for s in reqs[pkg].specifier if s.operator in {">=", "=="}]
        assert lower_bounds
        assert max(lower_bounds) >= min_version


def test_main_renders_otel_startup_event_as_json(
    monkeypatch: pytest.MonkeyPatch,
    capsys: pytest.CaptureFixture[str],
) -> None:
    """main.py configures structlog before init_otel_provider so startup OTel events are JSON on stdout."""
    monkeypatch.setenv("OTEL_ENABLED", "false")
    orig_pool = main_module._pool
    orig_http_client = main_module._http_client
    structlog.reset_defaults()
    _ = capsys.readouterr()
    try:
        importlib.reload(main_module)
        lines = [line for line in capsys.readouterr().out.strip().splitlines() if line.strip()]
        assert len(lines) >= 1
        parsed_events = [json.loads(line) for line in lines]
        otel_events = [e for e in parsed_events if e.get("event") == "otel_logs_disabled"]
        assert len(otel_events) == 1
        assert otel_events[0]["level"] == "info"
        assert "timestamp" in otel_events[0]
    finally:
        main_module._pool = orig_pool
        main_module._http_client = orig_http_client
