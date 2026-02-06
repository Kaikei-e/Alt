"""TDD RED phase: Tests for BatchProcessUsecase."""

from dataclasses import dataclass, field
from unittest.mock import MagicMock

from tag_generator.domain.models import BatchResult, Tag, TagExtractionResult


@dataclass
class FakeOutcome:
    tags: list[str] = field(default_factory=lambda: ["ml", "ai"])
    confidence: float = 0.85
    tag_count: int = 2
    inference_ms: float = 42.0
    language: str = "en"
    model_name: str = "test-model"
    sanitized_length: int = 500
    tag_confidences: dict = field(default_factory=lambda: {"ml": 0.9, "ai": 0.8})
    embedding_backend: str = "test"
    embedding_metadata: dict = field(default_factory=dict)


def _make_extraction_result(article_id: str = "a-1") -> TagExtractionResult:
    return TagExtractionResult(
        article_id=article_id,
        tags=[Tag(name="ml", confidence=0.9), Tag(name="ai", confidence=0.8)],
        language="en",
        inference_ms=42.0,
        overall_confidence=0.85,
    )


class TestBatchProcessUsecase:
    def _make_usecase(self):
        from tag_generator.usecase.batch_process import BatchProcessUsecase

        mock_article_repo = MagicMock()
        mock_tag_repo = MagicMock()
        mock_extract_usecase = MagicMock()
        mock_cascade = MagicMock()
        mock_cursor_store = MagicMock()
        mock_config = MagicMock()
        mock_config.batch_limit = 75
        mock_config.progress_log_interval = 10
        mock_config.memory_cleanup_interval = 25
        mock_config.enable_gc_collection = False

        usecase = BatchProcessUsecase(
            article_repo=mock_article_repo,
            tag_repo=mock_tag_repo,
            extract_usecase=mock_extract_usecase,
            cascade_controller=mock_cascade,
            cursor_store=mock_cursor_store,
            config=mock_config,
        )
        return usecase, mock_article_repo, mock_tag_repo, mock_extract_usecase, mock_cascade, mock_cursor_store

    def test_process_articles_as_batch_success(self):
        usecase, _, mock_tag_repo, mock_extract_usecase, mock_cascade, _ = self._make_usecase()
        mock_conn = MagicMock()

        articles = [
            {"id": "a-1", "title": "Title 1", "content": "Content 1"},
            {"id": "a-2", "title": "Title 2", "content": "Content 2"},
        ]

        mock_extract_usecase.execute.return_value = _make_extraction_result()
        mock_cascade.evaluate.return_value = MagicMock(needs_refine=False, as_dict=lambda: {})
        mock_tag_repo.batch_upsert_tags_no_commit.return_value = {
            "success": True,
            "processed_articles": 2,
            "failed_articles": 0,
            "errors": [],
            "message": None,
        }

        result = usecase.process_articles_as_batch(mock_conn, articles)

        assert isinstance(result, BatchResult)
        assert result.successful == 2
        assert result.failed == 0

    def test_process_articles_as_batch_empty(self):
        usecase, _, _, _, _, _ = self._make_usecase()
        mock_conn = MagicMock()

        result = usecase.process_articles_as_batch(mock_conn, [])
        assert isinstance(result, BatchResult)
        assert result.total_processed == 0

    def test_process_articles_as_batch_extraction_failure(self):
        usecase, _, mock_tag_repo, mock_extract_usecase, _, _ = self._make_usecase()
        mock_conn = MagicMock()

        articles = [{"id": "a-1", "title": "Title 1", "content": "Content 1"}]
        mock_extract_usecase.execute.side_effect = Exception("model crash")

        result = usecase.process_articles_as_batch(mock_conn, articles)
        assert result.failed == 1
