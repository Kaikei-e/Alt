import json
import sys
from io import StringIO

import pytest
import structlog

from tag_generator.logging_config import add_business_context, setup_logging


def test_setup_logging_produces_json_output():
    import os

    # Redirect stdout to capture log output
    old_stdout = sys.stdout
    sys.stdout = captured_output = StringIO()

    try:
        # Configure logging
        setup_logging()

        # Get a logger and log a message
        logger = structlog.get_logger("test_logger")
        logger.info("test_message", key="value")

        # Get the output and restore stdout
        log_output = captured_output.getvalue()
    finally:
        sys.stdout = old_stdout

    # Assertions
    assert log_output, "Log output should not be empty"

    try:
        log_json = json.loads(log_output)
    except json.JSONDecodeError:
        pytest.fail("Log output is not valid JSON.")

    assert log_json["level"] == "info"
    assert log_json["msg"] == "test_message"
    assert log_json["key"] == "value"
    assert "timestamp" in log_json
    # Check for the actual service name from environment or default
    expected_service = os.getenv("SERVICE_NAME", "tag-generator")
    assert log_json["service"] == expected_service


class TestAddBusinessContext:
    """Test ADR 98 business context transformations."""

    def test_renames_article_id_to_alt_format(self):
        event_dict = {"event": "test", "article_id": "art-123"}
        result = add_business_context(None, "", event_dict)

        assert result["alt.article.id"] == "art-123"
        assert "article_id" not in result

    def test_renames_feed_id_to_alt_format(self):
        event_dict = {"event": "test", "feed_id": "feed-456"}
        result = add_business_context(None, "", event_dict)

        assert result["alt.feed.id"] == "feed-456"
        assert "feed_id" not in result

    def test_renames_processing_stage_to_alt_format(self):
        event_dict = {"event": "test", "processing_stage": "extraction"}
        result = add_business_context(None, "", event_dict)

        assert result["alt.processing.stage"] == "extraction"
        assert "processing_stage" not in result

    def test_adds_ai_pipeline_identifier(self):
        event_dict = {"event": "test"}
        result = add_business_context(None, "", event_dict)

        assert result["alt.ai.pipeline"] == "tag-extraction"

    def test_preserves_other_fields(self):
        event_dict = {"event": "test", "custom_key": "value", "article_id": "a1"}
        result = add_business_context(None, "", event_dict)

        assert result["custom_key"] == "value"
        assert result["alt.article.id"] == "a1"

    def test_all_transforms_together(self):
        event_dict = {
            "event": "test",
            "article_id": "art-1",
            "feed_id": "feed-2",
            "processing_stage": "tagging",
        }
        result = add_business_context(None, "", event_dict)

        assert result["alt.article.id"] == "art-1"
        assert result["alt.feed.id"] == "feed-2"
        assert result["alt.processing.stage"] == "tagging"
        assert result["alt.ai.pipeline"] == "tag-extraction"
