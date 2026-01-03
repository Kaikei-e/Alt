"""Batch processing logic for tag generation."""

import gc
from typing import TYPE_CHECKING, Any, cast

import structlog
from psycopg2.extensions import connection as Connection

logger = structlog.get_logger(__name__)

if TYPE_CHECKING:
    from article_fetcher.fetch import ArticleFetcher
    from tag_extractor.extract import TagExtractor
    from tag_generator.cascade import CascadeController
    from tag_generator.config import TagGeneratorConfig
    from tag_generator.cursor_manager import CursorManager
    from tag_generator.database import DatabaseConnectionError
    from tag_inserter.upsert_tags import TagInserter


class BatchProcessor:
    """Handles batch processing of articles for tag generation."""

    def __init__(
        self,
        config: "TagGeneratorConfig",
        article_fetcher: "ArticleFetcher",
        tag_extractor: "TagExtractor",
        tag_inserter: "TagInserter",
        cascade_controller: "CascadeController",
        cursor_manager: "CursorManager",
    ):
        """Initialize batch processor with dependencies."""
        self.config = config
        self.article_fetcher = article_fetcher
        self.tag_extractor = tag_extractor
        self.tag_inserter = tag_inserter
        self.cascade_controller = cascade_controller
        self.cursor_manager = cursor_manager

        # Backfill state management
        self.backfill_completed: bool = False
        self.consecutive_empty_backfill_fetches: int = 0
        self.max_empty_backfill_fetches: int = 3  # Consider backfill complete after 3 empty fetches

    def _cleanup_memory(self) -> None:
        """Explicit memory cleanup to prevent accumulation."""
        if self.config.enable_gc_collection:
            gc.collect()

    def _fetch_untagged_articles_fallback(self, conn: Connection) -> list[dict[str, Any]]:
        """
        Fallback method to fetch untagged articles when cursor pagination fails.

        Args:
            conn: Database connection

        Returns:
            List of untagged articles
        """
        try:
            # Use the ArticleFetcher's method for fetching untagged articles
            untagged_articles = self.article_fetcher.fetch_articles_by_status(
                conn, has_tags=False, limit=self.config.batch_limit
            )

            logger.info(f"Fallback method retrieved {len(untagged_articles)} untagged articles")
            return untagged_articles

        except Exception as e:
            logger.error(f"Fallback method failed to fetch untagged articles: {e}")
            return []

    def _has_existing_tags(self, conn: Connection) -> bool:
        """Check whether any tags already exist in the database."""
        try:
            with conn.cursor() as cursor:
                cursor.execute("SELECT EXISTS (SELECT 1 FROM article_tags)")
                result = cursor.fetchone()
                return bool(result and result[0])
        except Exception as exc:
            logger.warning("Failed to check existing tags", error=str(exc))
            return False

    def process_articles_as_batch(self, conn: Connection, articles: list[dict[str, Any]]) -> dict[str, Any]:
        """
        Process multiple articles as a single batch transaction.
        Note: Transaction management is handled by the caller.

        Args:
            conn: Database connection (should already be in transaction mode)
            articles: List of articles to process

        Returns:
            Dictionary with batch processing results
        """
        batch_stats: dict[str, int] = {
            "total_processed": 0,
            "successful": 0,
            "failed": 0,
        }

        # Prepare batch data for tag insertion
        article_tags_batch = []
        cascade_refine_requests = 0

        # Extract tags for all articles first
        for i, article in enumerate(articles):
            try:
                article_id = article["id"]
                title = article["title"]
                content = article["content"]

                outcome = self.tag_extractor.extract_tags_with_metrics(title, content)

                if not outcome.tags:
                    continue

                decision = self.cascade_controller.evaluate(outcome)
                if decision.needs_refine:
                    cascade_refine_requests += 1

                article_tags_batch.append(
                    {
                        "article_id": article_id,
                        "tags": outcome.tags,
                        "tag_confidences": outcome.tag_confidences,
                        "cascade": decision.as_dict(),
                    }
                )

                logger.debug(
                    "Cascade decision recorded",
                    article_id=article_id,
                    **decision.as_dict(),
                )

                # Log progress during tag extraction
                if (i + 1) % self.config.progress_log_interval == 0:
                    logger.info(f"Extracted tags for {i + 1}/{len(articles)} articles...")

                # Periodic memory cleanup during batch processing
                if (i + 1) % self.config.memory_cleanup_interval == 0:
                    self._cleanup_memory()

            except Exception as e:
                logger.error(f"Error extracting tags for article {article.get('id', 'unknown')}: {e}")
                batch_stats["failed"] += 1
                continue

        logger.info(
            "Prepared batch with cascade metrics",
            batch_articles=len(article_tags_batch),
            refine_candidates=cascade_refine_requests,
        )

        # Perform batch upsert of all tags in the current transaction
        if article_tags_batch:
            try:
                logger.info(f"Upserting tags for {len(article_tags_batch)} articles in current transaction...")

                # Use the batch upsert method (transaction managed by caller)
                result = self.tag_inserter.batch_upsert_tags_no_commit(conn, article_tags_batch)

                batch_stats["successful"] = result.get("processed_articles", 0)
                batch_stats["failed"] += result.get("failed_articles", 0)
                batch_stats["total_processed"] = len(articles)

                if result.get("success"):
                    logger.info(f"Successfully batch processed {batch_stats['successful']} articles")
                else:
                    logger.warning(f"Batch processing completed with {batch_stats['failed']} failures")
                    # If batch processing failed, raise exception to trigger rollback
                    if batch_stats["failed"] > 0:
                        raise DatabaseConnectionError(f"Batch processing failed for {batch_stats['failed']} articles")

            except Exception as e:
                logger.error(f"Batch upsert failed: {e}")
                batch_stats["failed"] = len(articles)
                batch_stats["total_processed"] = len(articles)
                # Re-raise to trigger transaction rollback at higher level
                raise
        else:
            logger.warning("No articles with tags to process")
            batch_stats["total_processed"] = len(articles)

        return batch_stats

    def process_article_batch_forward(self, conn: Connection, cursor_manager: "CursorManager") -> dict[str, Any]:
        """Process articles newer than the current forward cursor.
        If no new articles are found and backfill is not completed, fall back to backfill processing.
        """
        start_created_at, start_id = cursor_manager.get_forward_cursor_position(conn)

        batch_stats = {
            "total_processed": 0,
            "successful": 0,
            "failed": 0,
            "last_created_at": start_created_at,
            "last_id": start_id,
            "has_more_pending": False,
        }

        articles = self.article_fetcher.fetch_new_articles(conn, start_created_at, start_id, self.config.batch_limit)

        batch_stats["has_more_pending"] = len(articles) >= self.config.batch_limit

        if not articles:
            logger.info("No new articles found for forward processing")
            # If backfill is not completed, fall back to backfill processing
            if not self.backfill_completed:
                logger.info("Backfill not completed, falling back to backfill processing")
                return self.process_article_batch_backfill(conn, cursor_manager)
            return batch_stats

        try:
            if conn.autocommit:
                conn.autocommit = False

            batch_stats = self.process_articles_as_batch(conn, articles)

            last_article = articles[-1]
            latest_created_at = (
                last_article["created_at"]
                if isinstance(last_article["created_at"], str)
                else last_article["created_at"].isoformat()
            )

            batch_stats["last_created_at"] = latest_created_at
            batch_stats["last_id"] = last_article["id"]
            batch_stats["has_more_pending"] = len(articles) >= self.config.batch_limit

            if cast(int, batch_stats.get("successful", 0)) > 0:
                cursor_manager.update_forward_cursor_position(latest_created_at, last_article["id"])
                cursor_manager.update_cursor_position(latest_created_at, last_article["id"])
                conn.commit()
            else:
                conn.rollback()
                logger.warning("Transaction rolled back due to forward batch failure")
        except Exception as exc:
            logger.error(f"Error during forward batch processing: {exc}")
            try:
                conn.rollback()
            except Exception as rollback_error:
                logger.error(f"Failed to rollback forward transaction: {rollback_error}")
            raise
        finally:
            try:
                if not conn.autocommit:
                    conn.autocommit = True
            except Exception as exc:
                logger.warning(f"Failed to restore autocommit mode after forward batch: {exc}")

        return batch_stats

    def process_article_batch_backfill(self, conn: Connection, cursor_manager: "CursorManager") -> dict[str, Any]:
        """
        Process a batch of articles for tag generation using true batch processing.
        In hybrid mode, checks for new articles first, then processes backfill.
        Includes fallback mechanism for cursor pagination failures.

        Args:
            conn: Database connection
            cursor_manager: Cursor manager for position tracking

        Returns:
            Dictionary with batch processing results
        """
        # Hybrid mode: Check for new articles first if forward cursor is available
        new_articles_processed = 0
        if cursor_manager.forward_cursor_created_at and cursor_manager.forward_cursor_id:
            try:
                forward_stats = self.process_article_batch_forward(conn, cursor_manager)
                new_articles_processed = cast(int, forward_stats.get("successful", 0))
                if new_articles_processed > 0:
                    logger.info(f"Hybrid mode: Processed {new_articles_processed} new articles before backfill")
                    # If we processed new articles and reached batch limit, return early
                    if cast(int, forward_stats.get("total_processed", 0)) >= self.config.batch_limit:
                        return forward_stats
            except Exception as exc:
                logger.warning(f"Hybrid mode: Failed to process new articles, continuing with backfill: {exc}")

        last_created_at, last_id = cursor_manager.get_initial_cursor_position()

        batch_stats = {
            "total_processed": 0,
            "successful": 0,
            "failed": 0,
            "last_created_at": last_created_at,
            "last_id": last_id,
            "has_more_pending": False,
        }

        # Avoid heavy processing when nothing is pending
        try:
            untagged_count = self.article_fetcher.count_untagged_articles(conn)
            if untagged_count == 0:
                logger.info("No untagged articles available; skipping backfill batch")
                # If we processed new articles in hybrid mode, return those stats
                if new_articles_processed > 0:
                    return {
                        "total_processed": new_articles_processed,
                        "successful": new_articles_processed,
                        "failed": 0,
                        "has_more_pending": False,
                    }
                return batch_stats
        except Exception as exc:
            logger.warning("Could not count untagged articles before backfill", error=str(exc))

        # Collect articles for batch processing (keep autocommit for fetching)
        articles_to_process: list[dict[str, Any]] = []
        fetch_attempts = 0

        while len(articles_to_process) < int(self.config.batch_limit):
            try:
                # Fetch articles using cursor pagination
                articles = self.article_fetcher.fetch_articles(conn, last_created_at, last_id)

                if not articles:
                    fetch_attempts += 1
                    self.consecutive_empty_backfill_fetches += 1
                    logger.info(f"No articles found with cursor pagination (attempt {fetch_attempts})")

                    # Check if backfill should be considered complete
                    if self.consecutive_empty_backfill_fetches >= self.max_empty_backfill_fetches:
                        if not self.backfill_completed:
                            logger.info("Backfill completed: no more articles found in consecutive fetches")
                            self.backfill_completed = True
                        # Try fallback approach when cursor pagination fails to find articles
                        # Note: fetch_attempts is local and resets each call, so we use
                        # consecutive_empty_backfill_fetches (class-level) to track across calls
                        if len(articles_to_process) == 0:
                            logger.warning(
                                "Cursor pagination consistently failing, switching to untagged article fallback"
                            )
                            fallback_articles = self._fetch_untagged_articles_fallback(conn)
                            if fallback_articles:
                                articles_to_process.extend(fallback_articles[: self.config.batch_limit])
                                logger.info(f"Fallback method found {len(fallback_articles)} untagged articles")
                                # Update cursor based on the last article processed
                                if articles_to_process:
                                    last_article = articles_to_process[-1]
                                    if isinstance(last_article["created_at"], str):
                                        last_created_at = last_article["created_at"]
                                    else:
                                        last_created_at = last_article["created_at"].isoformat()
                                    last_id = last_article["id"]
                                # Reset backfill completion since we found articles
                                self.backfill_completed = False
                                self.consecutive_empty_backfill_fetches = 0
                                break
                            else:
                                logger.info("No untagged articles found via fallback method")
                                break
                        else:
                            logger.info(
                                f"No more articles found. Collected {len(articles_to_process)} articles for batch processing"
                            )
                            break
                    else:
                        logger.info(
                            f"No more articles found. Collected {len(articles_to_process)} articles for batch processing"
                        )
                        break

                logger.info(f"Fetched {len(articles)} articles")
                fetch_attempts = 0  # Reset counter on successful fetch
                self.consecutive_empty_backfill_fetches = 0  # Reset backfill completion counter

                # Add articles to batch, respecting the batch limit
                for article in articles:
                    if len(articles_to_process) >= self.config.batch_limit:
                        logger.info(f"Reached batch limit of {self.config.batch_limit} articles")
                        break

                    articles_to_process.append(article)

                    # Update cursor position for next fetch (convert datetime to string)
                    if isinstance(article["created_at"], str):
                        last_created_at = article["created_at"]
                    else:
                        last_created_at = article["created_at"].isoformat()
                    last_id = article["id"]

                # Break if we've reached the batch limit
                if len(articles_to_process) >= self.config.batch_limit:
                    break

            except Exception as e:
                logger.error(f"Error during article collection: {e}")
                # Try fallback method on exception
                if len(articles_to_process) == 0:
                    logger.warning("Attempting fallback method due to fetch error")
                    try:
                        fallback_articles = self._fetch_untagged_articles_fallback(conn)
                        if fallback_articles:
                            articles_to_process.extend(fallback_articles[: self.config.batch_limit])
                            logger.info(f"Fallback method recovered {len(fallback_articles)} articles")
                    except Exception as fallback_error:
                        logger.error(f"Fallback method also failed: {fallback_error}")
                break

        # Start explicit transaction for batch processing only
        try:
            if conn.autocommit:
                conn.autocommit = False

            if articles_to_process:
                logger.info(f"Processing batch of {len(articles_to_process)} articles...")
                batch_stats = self.process_articles_as_batch(conn, articles_to_process)
                # Ensure string format for batch stats
                batch_stats["last_created_at"] = last_created_at
                batch_stats["last_id"] = last_id
                batch_stats["has_more_pending"] = len(articles_to_process) >= self.config.batch_limit

                # Commit the transaction if articles were processed (including skipped)
                # Only rollback on actual errors (failed > 0)
                total_processed = cast(int, batch_stats.get("total_processed", 0))
                failed = cast(int, batch_stats.get("failed", 0))
                successful = cast(int, batch_stats.get("successful", 0))

                if total_processed > 0 and failed == 0:
                    # Update cursor even if all articles were skipped (no tags extracted)
                    cursor_manager.update_cursor_position(last_created_at, last_id)
                    newest_article = articles_to_process[0]
                    newest_created_at = (
                        newest_article["created_at"]
                        if isinstance(newest_article["created_at"], str)
                        else newest_article["created_at"].isoformat()
                    )
                    cursor_manager.update_forward_cursor_position(newest_created_at, newest_article["id"])
                    logger.info(
                        f"Cursor advanced: processed {total_processed}, successful {successful}, "
                        f"position: {last_created_at}, ID: {last_id}"
                    )
                    conn.commit()
                elif failed > 0:
                    conn.rollback()
                    logger.warning(f"Transaction rolled back due to {failed} processing errors")
                else:
                    conn.commit()  # No articles processed, commit cleanly
            else:
                # No articles to process, still commit to end transaction cleanly
                conn.commit()
                # If we processed new articles in hybrid mode, return those stats
                if new_articles_processed > 0:
                    batch_stats["total_processed"] = new_articles_processed
                    batch_stats["successful"] = new_articles_processed
                    batch_stats["failed"] = 0
                    return batch_stats

        except Exception as e:
            logger.error(f"Error during batch processing: {e}")
            try:
                conn.rollback()
            except Exception as rollback_error:
                logger.error(f"Failed to rollback transaction: {rollback_error}")
            raise
        finally:
            # Reset autocommit mode
            try:
                if not conn.autocommit:
                    conn.autocommit = True
            except Exception as e:
                logger.warning(f"Failed to restore autocommit mode: {e}")

        return batch_stats

    def process_article_batch(self, conn: Connection, cursor_manager: "CursorManager") -> dict[str, Any]:
        """Choose processing strategy based on tagging state and backfill completion."""
        # If backfill is completed, prioritize forward processing
        if self.backfill_completed and self._has_existing_tags(conn):
            return self.process_article_batch_forward(conn, cursor_manager)

        # If backfill is not completed, do backfill processing
        # This will also check for new articles in hybrid mode
        return self.process_article_batch_backfill(conn, cursor_manager)
