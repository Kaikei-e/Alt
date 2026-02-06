"""TDD RED phase: Tests for RegenerateTagsUsecase."""

from unittest.mock import MagicMock

from tag_generator.domain.models import BatchResult, Tag, TagExtractionResult


def _make_extraction_result(article_id: str = "a-1") -> TagExtractionResult:
    return TagExtractionResult(
        article_id=article_id,
        tags=[Tag(name="ml", confidence=0.9), Tag(name="ai", confidence=0.8)],
        language="en",
        inference_ms=42.0,
        overall_confidence=0.85,
    )


class TestRegenerateTagsUsecase:
    def _make_usecase(self):
        from tag_generator.usecase.regenerate_tags import RegenerateTagsUsecase

        mock_article_repo = MagicMock()
        mock_tag_repo = MagicMock()
        mock_extract_usecase = MagicMock()
        mock_cascade = MagicMock()
        mock_config = MagicMock()
        mock_config.batch_limit = 75
        mock_config.progress_log_interval = 10
        mock_config.memory_cleanup_interval = 25
        mock_config.enable_gc_collection = False

        usecase = RegenerateTagsUsecase(
            article_repo=mock_article_repo,
            tag_repo=mock_tag_repo,
            extract_usecase=mock_extract_usecase,
            cascade_controller=mock_cascade,
            config=mock_config,
        )
        return usecase, mock_article_repo, mock_tag_repo, mock_extract_usecase, mock_cascade

    def test_regenerate_no_low_confidence_articles(self):
        usecase, mock_article_repo, _, _, _ = self._make_usecase()
        mock_conn = MagicMock()
        mock_article_repo.fetch_low_confidence_articles.return_value = []

        result = usecase.execute(mock_conn, confidence_threshold=0.5)

        assert isinstance(result, BatchResult)
        assert result.total_processed == 0

    def test_regenerate_processes_articles(self):
        usecase, mock_article_repo, mock_tag_repo, mock_extract_usecase, mock_cascade = self._make_usecase()
        mock_conn = MagicMock()
        mock_conn.autocommit = True

        mock_article_repo.fetch_low_confidence_articles.return_value = [
            {"id": "a-1", "title": "Old Article", "content": "Content", "avg_confidence": 0.3},
        ]
        mock_extract_usecase.execute.return_value = _make_extraction_result()
        mock_cascade.evaluate.return_value = MagicMock(needs_refine=False, as_dict=lambda: {})
        mock_tag_repo.batch_upsert_tags_with_comparison.return_value = {
            "success": True,
            "processed_articles": 1,
            "failed_articles": 0,
            "errors": [],
            "message": None,
            "skipped_lower_confidence": 0,
            "updated_higher_confidence": 1,
        }

        result = usecase.execute(mock_conn, confidence_threshold=0.5)

        assert isinstance(result, BatchResult)
        assert result.total_processed == 1

    def test_regenerate_fetch_failure(self):
        usecase, mock_article_repo, _, _, _ = self._make_usecase()
        mock_conn = MagicMock()
        mock_article_repo.fetch_low_confidence_articles.side_effect = Exception("DB error")

        result = usecase.execute(mock_conn, confidence_threshold=0.5)
        assert result.failed == 1
