"""Tests for security hardening (Phase 6).

Covers:
- Sensitive data filtering in logs
- Event payload validation
- Batch size hard limits
"""

import json
import sys
from io import StringIO

import pytest
import structlog
from pydantic import ValidationError

from tag_generator.handler.event_payload import TagGenerationRequestPayload
from tag_generator.infra.config import BatchConfig
from tag_generator.infra.logging_config import filter_sensitive_data, setup_logging


class TestSensitiveDataFilter:
    """Tests for the structlog sensitive data filter processor."""

    def test_redacts_password_key(self) -> None:
        event = {"event": "test", "password": "secret123"}
        result = filter_sensitive_data(None, "", event)
        assert result["password"] == "***REDACTED***"  # noqa: S105 - test fixture value, not a credential

    def test_redacts_key_containing_password(self) -> None:
        event = {"event": "test", "db_password": "secret123"}
        result = filter_sensitive_data(None, "", event)
        assert result["db_password"] == "***REDACTED***"  # noqa: S105 - test fixture value, not a credential

    def test_redacts_token_key(self) -> None:
        event = {"event": "test", "auth_token": "tok-abc"}
        result = filter_sensitive_data(None, "", event)
        assert result["auth_token"] == "***REDACTED***"  # noqa: S105 - test fixture value, not a credential

    def test_redacts_secret_key(self) -> None:
        event = {"event": "test", "client_secret": "xyz"}
        result = filter_sensitive_data(None, "", event)
        assert result["client_secret"] == "***REDACTED***"  # noqa: S105 - test fixture value, not a credential

    def test_redacts_dsn_key(self) -> None:
        event = {"event": "test", "database_dsn": "postgresql://user:pass@host/db"}
        result = filter_sensitive_data(None, "", event)
        assert result["database_dsn"] == "***REDACTED***"

    def test_redacts_authorization_key(self) -> None:
        event = {"event": "test", "authorization": "Bearer xyz"}
        result = filter_sensitive_data(None, "", event)
        assert result["authorization"] == "***REDACTED***"

    def test_redacts_api_key(self) -> None:
        event = {"event": "test", "api_key": "ak-123"}
        result = filter_sensitive_data(None, "", event)
        assert result["api_key"] == "***REDACTED***"

    def test_case_insensitive_matching(self) -> None:
        event = {"event": "test", "API_KEY": "ak-123", "Password": "secret"}
        result = filter_sensitive_data(None, "", event)
        assert result["API_KEY"] == "***REDACTED***"
        assert result["Password"] == "***REDACTED***"  # noqa: S105 - test fixture value, not a credential

    def test_preserves_non_sensitive_keys(self) -> None:
        event = {"event": "test", "article_id": "a-1", "level": "info"}
        result = filter_sensitive_data(None, "", event)
        assert result["article_id"] == "a-1"
        assert result["level"] == "info"

    def test_preserves_event_key(self) -> None:
        event = {"event": "db_connected", "host": "localhost"}
        result = filter_sensitive_data(None, "", event)
        assert result["event"] == "db_connected"

    def test_handles_empty_event_dict(self) -> None:
        event = {}
        result = filter_sensitive_data(None, "", event)
        assert result == {}

    def test_integration_with_setup_logging(self, monkeypatch: pytest.MonkeyPatch) -> None:
        """Verify sensitive data is actually redacted in JSON output."""
        monkeypatch.setenv("OTEL_ENABLED", "false")

        old_stdout = sys.stdout
        sys.stdout = captured = StringIO()
        try:
            setup_logging(enable_otel=False)
            logger = structlog.get_logger("security_test")
            logger.info("db_connect", password="supersecret", host="localhost")  # noqa: S106 - test fixture value, not a credential
            output = captured.getvalue()
        finally:
            sys.stdout = old_stdout

        assert output
        log_json = json.loads(output)
        assert log_json.get("password") == "***REDACTED***"
        assert log_json.get("host") == "localhost"


class TestEventPayloadValidation:
    """Tests for Pydantic event payload validation."""

    def test_valid_payload(self) -> None:
        payload = TagGenerationRequestPayload(
            article_id="a-123",
            title="Test Title",
            content="Test content body",
            feed_id="f-456",
        )
        assert payload.article_id == "a-123"
        assert payload.feed_id == "f-456"

    def test_article_id_required(self) -> None:
        with pytest.raises(ValidationError, match="article_id"):
            TagGenerationRequestPayload(
                article_id="",
                title="Title",
                content="Content",
            )

    def test_title_max_length(self) -> None:
        with pytest.raises(ValidationError, match="title"):
            TagGenerationRequestPayload(
                article_id="a-1",
                title="x" * 2001,
                content="Content",
            )

    def test_content_max_length(self) -> None:
        with pytest.raises(ValidationError, match="content"):
            TagGenerationRequestPayload(
                article_id="a-1",
                title="Title",
                content="x" * 100_001,
            )

    def test_feed_id_defaults_to_empty(self) -> None:
        payload = TagGenerationRequestPayload(
            article_id="a-1",
            title="Title",
            content="Content",
        )
        assert payload.feed_id == ""

    def test_from_event_payload_dict(self) -> None:
        raw = {
            "article_id": "a-1",
            "title": "Title",
            "content": "Body text",
            "feed_id": "f-1",
            "extra_field": "ignored",
        }
        payload = TagGenerationRequestPayload.model_validate(raw)
        assert payload.article_id == "a-1"

    def test_article_id_max_length(self) -> None:
        with pytest.raises(ValidationError, match="article_id"):
            TagGenerationRequestPayload(
                article_id="x" * 37,
                title="Title",
                content="Content",
            )


class TestBatchSizeHardLimit:
    """Tests for batch size hard limits in config."""

    def test_batch_limit_max_1000(self) -> None:
        over_limit: int = 1001
        with pytest.raises(ValidationError, match="less than or equal to 1000"):
            BatchConfig(batch_limit=over_limit)

    def test_batch_limit_within_bounds(self) -> None:
        expected_limit = 500
        config = BatchConfig(batch_limit=expected_limit)
        assert config.batch_limit == expected_limit

    def test_batch_limit_default(self) -> None:
        default_batch_limit = 75
        config = BatchConfig()
        assert config.batch_limit == default_batch_limit

    def test_batch_limit_must_be_positive(self) -> None:
        zero_limit: int = 0
        with pytest.raises(ValidationError, match="greater than 0"):
            BatchConfig(batch_limit=zero_limit)
