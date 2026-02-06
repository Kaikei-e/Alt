"""Usecase: Batch process articles for tag generation."""

from __future__ import annotations

from typing import TYPE_CHECKING, Any

import structlog

from tag_generator.domain.errors import BatchProcessingError
from tag_generator.domain.models import BatchResult

if TYPE_CHECKING:
    from tag_generator.cascade import CascadeController
    from tag_generator.config import TagGeneratorConfig
    from tag_generator.port.article_repository import ArticleRepositoryPort
    from tag_generator.port.cursor_store import CursorStorePort
    from tag_generator.port.tag_repository import TagRepositoryPort
    from tag_generator.usecase.extract_tags import ExtractTagsUsecase

logger = structlog.get_logger(__name__)


class BatchProcessUsecase:
    """Batch process multiple articles for tag extraction."""

    def __init__(
        self,
        article_repo: ArticleRepositoryPort,
        tag_repo: TagRepositoryPort,
        extract_usecase: ExtractTagsUsecase,
        cascade_controller: CascadeController,
        cursor_store: CursorStorePort,
        config: TagGeneratorConfig,
    ) -> None:
        self._article_repo = article_repo
        self._tag_repo = tag_repo
        self._extract_usecase = extract_usecase
        self._cascade = cascade_controller
        self._cursor_store = cursor_store
        self._config = config

    def process_articles_as_batch(
        self,
        conn: Any,
        articles: list[dict[str, Any]],
    ) -> BatchResult:
        """Extract tags for a list of articles and upsert in a single transaction.

        Transaction management is handled by the caller.
        """
        result = BatchResult(total_processed=len(articles))

        if not articles:
            return result

        article_tags_batch: list[dict[str, Any]] = []
        cascade_refine_requests = 0

        for i, article in enumerate(articles):
            try:
                article_id = article["id"]
                title = article["title"]
                content = article["content"]

                extraction = self._extract_usecase.execute(article_id, title, content)

                if extraction.is_empty:
                    continue

                # Build the outcome-like object for cascade evaluation
                outcome = self._extract_usecase._tag_extractor.extract_tags_with_metrics(title, content)
                decision = self._cascade.evaluate(outcome)
                if decision.needs_refine:
                    cascade_refine_requests += 1

                article_tags_batch.append(
                    {
                        "article_id": article_id,
                        "tags": extraction.tag_names,
                        "tag_confidences": extraction.tag_confidences,
                        "cascade": decision.as_dict(),
                    }
                )

                if (i + 1) % self._config.progress_log_interval == 0:
                    logger.debug("Extracted tags progress", completed=i + 1, total=len(articles))

            except Exception as e:
                logger.error("Error extracting tags for article", article_id=article.get("id", "unknown"), error=str(e))
                result.failed += 1
                continue

        logger.info(
            "Prepared batch with cascade metrics",
            batch_articles=len(article_tags_batch),
            refine_candidates=cascade_refine_requests,
        )

        if article_tags_batch:
            try:
                upsert_result = self._tag_repo.batch_upsert_tags_no_commit(conn, article_tags_batch)
                result.successful = upsert_result.get("processed_articles", 0)
                result.failed += upsert_result.get("failed_articles", 0)

                if not upsert_result.get("success"):
                    if result.failed > 0:
                        raise BatchProcessingError(f"Batch processing failed for {result.failed} articles")
            except Exception:
                raise
        else:
            logger.warning("No articles with tags to process")

        return result
