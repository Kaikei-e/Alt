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

from tag_generator.infra.logging_config import filter_sensitive_data


class TestSensitiveDataFilter:
    """Tests for the structlog sensitive data filter processor."""

    def test_redacts_password_key(self):
        event = {"event": "test", "password": "secret123"}
        result = filter_sensitive_data(None, "", event)
        assert result["password"] == "***REDACTED***"

    def test_redacts_key_containing_password(self):
        event = {"event": "test", "db_password": "secret123"}
        result = filter_sensitive_data(None, "", event)
        assert result["db_password"] == "***REDACTED***"

    def test_redacts_token_key(self):
        event = {"event": "test", "auth_token": "tok-abc"}
        result = filter_sensitive_data(None, "", event)
        assert result["auth_token"] == "***REDACTED***"

    def test_redacts_secret_key(self):
        event = {"event": "test", "client_secret": "xyz"}
        result = filter_sensitive_data(None, "", event)
        assert result["client_secret"] == "***REDACTED***"

    def test_redacts_dsn_key(self):
        event = {"event": "test", "database_dsn": "postgresql://user:pass@host/db"}
        result = filter_sensitive_data(None, "", event)
        assert result["database_dsn"] == "***REDACTED***"

    def test_redacts_authorization_key(self):
        event = {"event": "test", "authorization": "Bearer xyz"}
        result = filter_sensitive_data(None, "", event)
        assert result["authorization"] == "***REDACTED***"

    def test_redacts_api_key(self):
        event = {"event": "test", "api_key": "ak-123"}
        result = filter_sensitive_data(None, "", event)
        assert result["api_key"] == "***REDACTED***"

    def test_case_insensitive_matching(self):
        event = {"event": "test", "API_KEY": "ak-123", "Password": "secret"}
        result = filter_sensitive_data(None, "", event)
        assert result["API_KEY"] == "***REDACTED***"
        assert result["Password"] == "***REDACTED***"

    def test_preserves_non_sensitive_keys(self):
        event = {"event": "test", "article_id": "a-1", "level": "info"}
        result = filter_sensitive_data(None, "", event)
        assert result["article_id"] == "a-1"
        assert result["level"] == "info"

    def test_preserves_event_key(self):
        event = {"event": "db_connected", "host": "localhost"}
        result = filter_sensitive_data(None, "", event)
        assert result["event"] == "db_connected"

    def test_handles_empty_event_dict(self):
        event = {}
        result = filter_sensitive_data(None, "", event)
        assert result == {}

    def test_integration_with_setup_logging(self, monkeypatch):
        """Verify sensitive data is actually redacted in JSON output."""
        monkeypatch.setenv("OTEL_ENABLED", "false")

        old_stdout = sys.stdout
        sys.stdout = captured = StringIO()
        try:
            from tag_generator.infra.logging_config import setup_logging

            setup_logging(enable_otel=False)
            logger = structlog.get_logger("security_test")
            logger.info("db_connect", password="supersecret", host="localhost")  # noqa: S106
            output = captured.getvalue()
        finally:
            sys.stdout = old_stdout

        assert output
        log_json = json.loads(output)
        assert log_json.get("password") == "***REDACTED***"
        assert log_json.get("host") == "localhost"


class TestEventPayloadValidation:
    """Tests for Pydantic event payload validation."""

    def test_valid_payload(self):
        from tag_generator.handler.event_payload import TagGenerationRequestPayload

        payload = TagGenerationRequestPayload(
            article_id="a-123",
            title="Test Title",
            content="Test content body",
            feed_id="f-456",
        )
        assert payload.article_id == "a-123"
        assert payload.feed_id == "f-456"

    def test_article_id_required(self):
        from pydantic import ValidationError

        from tag_generator.handler.event_payload import TagGenerationRequestPayload

        with pytest.raises(ValidationError, match="article_id"):
            TagGenerationRequestPayload(
                article_id="",
                title="Title",
                content="Content",
            )

    def test_title_max_length(self):
        from pydantic import ValidationError

        from tag_generator.handler.event_payload import TagGenerationRequestPayload

        with pytest.raises(ValidationError, match="title"):
            TagGenerationRequestPayload(
                article_id="a-1",
                title="x" * 2001,
                content="Content",
            )

    def test_content_max_length(self):
        from pydantic import ValidationError

        from tag_generator.handler.event_payload import TagGenerationRequestPayload

        with pytest.raises(ValidationError, match="content"):
            TagGenerationRequestPayload(
                article_id="a-1",
                title="Title",
                content="x" * 100_001,
            )

    def test_feed_id_defaults_to_empty(self):
        from tag_generator.handler.event_payload import TagGenerationRequestPayload

        payload = TagGenerationRequestPayload(
            article_id="a-1",
            title="Title",
            content="Content",
        )
        assert payload.feed_id == ""

    def test_from_event_payload_dict(self):
        from tag_generator.handler.event_payload import TagGenerationRequestPayload

        raw = {
            "article_id": "a-1",
            "title": "Title",
            "content": "Body text",
            "feed_id": "f-1",
            "extra_field": "ignored",
        }
        payload = TagGenerationRequestPayload.model_validate(raw)
        assert payload.article_id == "a-1"

    def test_article_id_max_length(self):
        from pydantic import ValidationError

        from tag_generator.handler.event_payload import TagGenerationRequestPayload

        with pytest.raises(ValidationError, match="article_id"):
            TagGenerationRequestPayload(
                article_id="x" * 37,
                title="Title",
                content="Content",
            )


class TestBatchSizeHardLimit:
    """Tests for batch size hard limits in config."""

    def test_batch_limit_max_1000(self):
        from pydantic import ValidationError

        from tag_generator.infra.config import BatchConfig

        with pytest.raises(ValidationError):
            BatchConfig(batch_limit=1001)

    def test_batch_limit_within_bounds(self):
        from tag_generator.infra.config import BatchConfig

        config = BatchConfig(batch_limit=500)
        assert config.batch_limit == 500

    def test_batch_limit_default(self):
        from tag_generator.infra.config import BatchConfig

        config = BatchConfig()
        assert config.batch_limit == 75

    def test_batch_limit_must_be_positive(self):
        from pydantic import ValidationError

        from tag_generator.infra.config import BatchConfig

        with pytest.raises(ValidationError):
            BatchConfig(batch_limit=0)
