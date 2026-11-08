"""Unit tests for acolyte-orchestrator OpenTelemetry log export and structlog bridge."""

from __future__ import annotations

import json
from typing import TYPE_CHECKING
from unittest.mock import MagicMock, patch

import pytest
import requests
import structlog
from opentelemetry._logs import SeverityNumber
from opentelemetry.sdk._logs import LoggerProvider
from opentelemetry.sdk._logs.export import InMemoryLogRecordExporter, SimpleLogRecordProcessor
from structlog.testing import capture_logs

from acolyte.infra.logging import configure_logging
from acolyte.infra.otel import (
    OTelConfig,
    OTelHandle,
    _NoRedirectSession,
    init_otel_provider,
    load_rask_ingest_token,
)

if TYPE_CHECKING:
    from pathlib import Path

_VALID_TOKEN = "ValidRaskIngestToken1234567890abcdefgh="


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


def test_shutdown_flushes_and_shuts_down_logger_provider(
    monkeypatch: pytest.MonkeyPatch,
    tmp_path: Path,
) -> None:
    """The shutdown callable on OTelHandle flushes and shuts down the LoggerProvider."""
    token_file = tmp_path / "rask_ingest_token"
    token_file.write_text(_VALID_TOKEN, encoding="utf-8")
    monkeypatch.setenv("OTEL_ENABLED", "true")
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

        mock_provider.force_flush.assert_called_once()
        mock_provider.shutdown.assert_called_once()
