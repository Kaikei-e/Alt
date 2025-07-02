import json
import sys
from io import StringIO
import pytest
from tag_generator.logging_config import setup_logging
import structlog


def test_setup_logging_produces_json_output():
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
    assert log_json["service"] == "tag-generator"
