"""Tests for ADR 98 business context logging."""

from recap_evaluator.utils.logging import add_business_context


class TestAddBusinessContext:
    """Test the add_business_context structlog processor."""

    def test_renames_job_id_to_alt_format(self):
        event_dict = {"event": "test", "job_id": "job-123"}
        result = add_business_context(None, "", event_dict)

        assert result["alt.job.id"] == "job-123"
        assert "job_id" not in result

    def test_renames_processing_stage_to_alt_format(self):
        event_dict = {"event": "test", "processing_stage": "evaluation"}
        result = add_business_context(None, "", event_dict)

        assert result["alt.processing.stage"] == "evaluation"
        assert "processing_stage" not in result

    def test_renames_article_id_to_alt_format(self):
        event_dict = {"event": "test", "article_id": "art-456"}
        result = add_business_context(None, "", event_dict)

        assert result["alt.article.id"] == "art-456"
        assert "article_id" not in result

    def test_adds_ai_pipeline_identifier(self):
        event_dict = {"event": "test"}
        result = add_business_context(None, "", event_dict)

        assert result["alt.ai.pipeline"] == "recap-evaluation"

    def test_preserves_other_fields(self):
        event_dict = {"event": "test", "custom_key": "value", "job_id": "j1"}
        result = add_business_context(None, "", event_dict)

        assert result["custom_key"] == "value"
        assert result["alt.job.id"] == "j1"

    def test_all_transforms_together(self):
        event_dict = {
            "event": "test",
            "job_id": "j1",
            "processing_stage": "clustering",
            "article_id": "art-789",
        }
        result = add_business_context(None, "", event_dict)

        assert result["alt.job.id"] == "j1"
        assert result["alt.processing.stage"] == "clustering"
        assert result["alt.article.id"] == "art-789"
        assert result["alt.ai.pipeline"] == "recap-evaluation"
