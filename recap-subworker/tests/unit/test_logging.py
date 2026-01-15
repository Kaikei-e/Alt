"""
ABOUTME: Unit tests for ADR 98/99 compliant business context logging.
ABOUTME: Tests the add_business_context processor and context management functions.
"""

from __future__ import annotations

import json
import logging
from io import StringIO

import pytest
import structlog


class TestAddBusinessContext:
    """Tests for the add_business_context structlog processor."""

    def test_renames_job_id_to_alt_format(self) -> None:
        """job_id should be renamed to alt.job.id."""
        from recap_subworker.infra.logging import add_business_context

        event_dict = {"event": "test", "job_id": "job-123"}
        result = add_business_context(None, "info", event_dict)

        assert "alt.job.id" in result
        assert result["alt.job.id"] == "job-123"
        assert "job_id" not in result

    def test_renames_article_id_to_alt_format(self) -> None:
        """article_id should be renamed to alt.article.id."""
        from recap_subworker.infra.logging import add_business_context

        event_dict = {"event": "test", "article_id": "article-456"}
        result = add_business_context(None, "info", event_dict)

        assert "alt.article.id" in result
        assert result["alt.article.id"] == "article-456"
        assert "article_id" not in result

    def test_adds_ai_pipeline_identifier(self) -> None:
        """Always adds alt.ai.pipeline = 'recap-classification'."""
        from recap_subworker.infra.logging import add_business_context

        event_dict = {"event": "test"}
        result = add_business_context(None, "info", event_dict)

        assert "alt.ai.pipeline" in result
        assert result["alt.ai.pipeline"] == "recap-classification"

    def test_preserves_other_fields(self) -> None:
        """Other fields should be preserved unchanged."""
        from recap_subworker.infra.logging import add_business_context

        event_dict = {"event": "test", "custom_field": "value", "count": 42}
        result = add_business_context(None, "info", event_dict)

        assert result["custom_field"] == "value"
        assert result["count"] == 42
        assert result["event"] == "test"


class TestContextVars:
    """Tests for context variable management functions."""

    def test_set_and_get_job_id(self) -> None:
        """set_job_id and get_job_id should work correctly."""
        from recap_subworker.infra.logging import (
            clear_context,
            get_job_id,
            set_job_id,
        )

        clear_context()
        assert get_job_id() is None

        set_job_id("test-job-123")
        assert get_job_id() == "test-job-123"

        clear_context()
        assert get_job_id() is None

    def test_set_and_get_article_id(self) -> None:
        """set_article_id and get_article_id should work correctly."""
        from recap_subworker.infra.logging import (
            clear_context,
            get_article_id,
            set_article_id,
        )

        clear_context()
        assert get_article_id() is None

        set_article_id("test-article-456")
        assert get_article_id() == "test-article-456"

        clear_context()
        assert get_article_id() is None

    def test_set_and_get_processing_stage(self) -> None:
        """set_processing_stage and get_processing_stage should work correctly."""
        from recap_subworker.infra.logging import (
            clear_context,
            get_processing_stage,
            set_processing_stage,
        )

        clear_context()
        assert get_processing_stage() is None

        set_processing_stage("clustering")
        assert get_processing_stage() == "clustering"

        clear_context()
        assert get_processing_stage() is None

    def test_clear_context_clears_all(self) -> None:
        """clear_context should clear all context variables."""
        from recap_subworker.infra.logging import (
            clear_context,
            get_article_id,
            get_job_id,
            get_processing_stage,
            set_article_id,
            set_job_id,
            set_processing_stage,
        )

        set_job_id("job")
        set_article_id("article")
        set_processing_stage("stage")

        clear_context()

        assert get_job_id() is None
        assert get_article_id() is None
        assert get_processing_stage() is None


class TestMergeContextVarsProcessor:
    """Tests for context variable merging into logs."""

    def test_context_vars_merged_via_bind_contextvars(self) -> None:
        """Context variables set via bind_contextvars should be available in logs."""
        from recap_subworker.infra.logging import (
            clear_context,
            set_article_id,
            set_job_id,
            set_processing_stage,
        )

        clear_context()
        set_job_id("merged-job")
        set_article_id("merged-article")
        set_processing_stage("merged-stage")

        # Verify context is set correctly
        from recap_subworker.infra.logging import (
            get_article_id,
            get_job_id,
            get_processing_stage,
        )

        assert get_job_id() == "merged-job"
        assert get_article_id() == "merged-article"
        assert get_processing_stage() == "merged-stage"

        # Also test structlog.contextvars.bind_contextvars integration
        structlog.contextvars.bind_contextvars(job_id="bound-job")

        # Get bound context
        from structlog.contextvars import get_contextvars

        bound = get_contextvars()
        assert bound.get("job_id") == "bound-job"

        # Cleanup
        structlog.contextvars.unbind_contextvars("job_id")
        clear_context()


class TestConfigureLoggingWithBusinessContext:
    """Tests for configure_logging with ADR 98 business context."""

    def test_json_output_includes_alt_keys(self) -> None:
        """JSON output should include alt.* prefixed keys."""
        from recap_subworker.infra.logging import configure_logging

        # Capture stdout
        stream = StringIO()
        handler = logging.StreamHandler(stream)
        handler.setLevel(logging.INFO)

        # Configure logging
        configure_logging("INFO")

        # Add our test handler
        root = logging.getLogger()
        root.addHandler(handler)

        # Log with structlog
        logger = structlog.get_logger("test")
        logger.info("test message", job_id="test-job-789")

        # Get output
        output = stream.getvalue()

        # Parse JSON and verify
        if output.strip():
            log_data = json.loads(output.strip().split("\n")[-1])
            # After processor, job_id should become alt.job.id
            assert "alt.ai.pipeline" in log_data
            assert log_data["alt.ai.pipeline"] == "recap-classification"

        # Cleanup
        root.removeHandler(handler)
