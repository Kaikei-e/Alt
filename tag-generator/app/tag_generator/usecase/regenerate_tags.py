"""Usecase: Regenerate tags for low-confidence articles."""

from __future__ import annotations

from typing import TYPE_CHECKING, Any

import structlog

from tag_generator.domain.models import BatchResult

if TYPE_CHECKING:
    from tag_generator.cascade import CascadeController
    from tag_generator.config import TagGeneratorConfig
    from tag_generator.port.article_repository import ArticleRepositoryPort
    from tag_generator.port.tag_repository import TagRepositoryPort
    from tag_generator.usecase.extract_tags import ExtractTagsUsecase

logger = structlog.get_logger(__name__)


class RegenerateTagsUsecase:
    """Regenerate tags for articles with low-confidence scores."""

    def __init__(
        self,
        article_repo: ArticleRepositoryPort,
        tag_repo: TagRepositoryPort,
        extract_usecase: ExtractTagsUsecase,
        cascade_controller: CascadeController,
        config: TagGeneratorConfig,
    ) -> None:
        self._article_repo = article_repo
        self._tag_repo = tag_repo
        self._extract_usecase = extract_usecase
        self._cascade = cascade_controller
        self._config = config

    def execute(
        self,
        conn: Any,
        confidence_threshold: float = 0.5,
    ) -> BatchResult:
        """Fetch low-confidence articles and regenerate their tags.

        Tags are only updated when the new confidence is higher.
        """
        result = BatchResult()

        try:
            articles = self._article_repo.fetch_low_confidence_articles(
                conn,
                confidence_threshold=confidence_threshold,
                limit=self._config.batch_limit,
            )
        except Exception as e:
            logger.error("Failed to fetch low-confidence articles", error=str(e))
            result.failed = 1
            return result

        if not articles:
            logger.info("No low-confidence articles found for regeneration", threshold=confidence_threshold)
            return result

        result.total_processed = len(articles)

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

                # Re-run extraction for cascade evaluation (outcome-based)
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
                        "old_avg_confidence": article.get("avg_confidence"),
                        "new_confidence": extraction.overall_confidence,
                    }
                )

                if (i + 1) % self._config.progress_log_interval == 0:
                    logger.debug("Regenerated tags progress", completed=i + 1, total=len(articles))

            except Exception as e:
                logger.error(
                    "Error extracting tags for article during regeneration",
                    article_id=article.get("id", "unknown"),
                    error=str(e),
                )
                result.failed += 1
                continue

        if article_tags_batch:
            try:
                if conn.autocommit:
                    conn.autocommit = False

                upsert_result = self._tag_repo.batch_upsert_tags_with_comparison(conn, article_tags_batch)
                result.successful = upsert_result.get("processed_articles", 0)
                result.failed += upsert_result.get("failed_articles", 0)

                if upsert_result.get("success"):
                    conn.commit()
                else:
                    conn.rollback()
            except Exception as e:
                logger.error("Regeneration batch upsert failed", error=str(e))
                try:
                    conn.rollback()
                except Exception:
                    pass
                result.failed = len(articles)
            finally:
                try:
                    if not conn.autocommit:
                        conn.autocommit = True
                except Exception:
                    pass

        return result
