"""Background scheduler for periodic genre learning tasks."""

from __future__ import annotations

import asyncio
from datetime import datetime, timezone

import structlog

from ..db.session import get_session_factory
from ..infra.config import Settings
from .genre_learning import GenreLearningService
from .learning_client import LearningClient

logger = structlog.get_logger(__name__)


class LearningScheduler:
    """Scheduler that runs genre learning tasks periodically."""

    def __init__(
        self,
        settings: Settings,
        interval_hours: float = 4.0,
    ) -> None:
        self.settings = settings
        self.interval_seconds = interval_hours * 3600.0
        self._task: asyncio.Task | None = None
        self._running = False

    async def start(self) -> None:
        """Start the background scheduler."""
        if self._running:
            logger.warning("learning scheduler already running")
            return

        # Start the scheduler loop
        self._running = True
        self._task = asyncio.create_task(self._run_loop())
        logger.info(
            "learning scheduler started",
            interval_hours=self.interval_seconds / 3600.0,
        )

    async def stop(self) -> None:
        """Stop the background scheduler."""
        self._running = False
        if self._task:
            self._task.cancel()
            try:
                await self._task
            except asyncio.CancelledError:
                pass
        logger.info("learning scheduler stopped")

    async def _run_loop(self) -> None:
        """Main scheduler loop."""
        while self._running:
            try:
                await self._execute_learning()
            except asyncio.CancelledError:
                break
            except Exception as exc:
                logger.error(
                    "learning scheduler task failed",
                    error=str(exc),
                    exc_info=True,
                )

            if not self._running:
                break

            try:
                await asyncio.sleep(self.interval_seconds)
            except asyncio.CancelledError:
                break

    async def _execute_learning(self) -> None:
        """Execute a single learning task."""
        start_time = datetime.now(timezone.utc)
        logger.info(
            "starting scheduled genre learning task",
            recap_worker_url=self.settings.recap_worker_learning_url,
        )

        try:
            logger.debug("creating database session factory")
            session_factory = get_session_factory(self.settings)

            logger.debug("opening database session")
            async with session_factory() as session:
                # Create learning service
                cluster_genres = [
                    genre.strip()
                    for genre in self.settings.learning_cluster_genres.split(",")
                    if genre.strip()
                ]
                logger.debug(
                    "creating learning service",
                    cluster_genres=cluster_genres,
                    graph_margin=self.settings.learning_graph_margin,
                )
                service = GenreLearningService(
                    session=session,
                    graph_margin=self.settings.learning_graph_margin,
                    cluster_genres=cluster_genres,
                )

                # Generate learning result
                logger.info("fetching learning data from database")
                learning_result = await service.generate_learning_result()
                logger.info(
                    "learning result generated",
                    total_records=learning_result.summary.total_records,
                    graph_boost_count=learning_result.summary.graph_boost_count,
                    graph_boost_percentage=learning_result.summary.graph_boost_percentage,
                    has_cluster_draft=learning_result.cluster_draft is not None,
                )

                # Create client and send to recap-worker
                logger.debug(
                    "creating HTTP client",
                    url=self.settings.recap_worker_learning_url,
                    timeout_seconds=self.settings.learning_request_timeout_seconds,
                )
                client = LearningClient.create(
                    self.settings.recap_worker_learning_url,
                    self.settings.learning_request_timeout_seconds,
                )

                try:
                    logger.debug("building learning payload")
                    payload = self._build_learning_payload(learning_result)
                    logger.info(
                        "sending learning payload to recap-worker",
                        payload_size=len(str(payload)),
                    )
                    response = await client.send_learning_payload(payload)
                    duration = (datetime.now(timezone.utc) - start_time).total_seconds()

                    logger.info(
                        "scheduled learning task completed",
                        duration_seconds=duration,
                        recap_worker_status=response.status_code,
                        total_records=learning_result.summary.total_records,
                    )
                except Exception as send_exc:
                    logger.error(
                        "failed to send learning payload",
                        error=str(send_exc),
                        error_type=type(send_exc).__name__,
                        exc_info=True,
                    )
                    raise
                finally:
                    logger.debug("closing HTTP client")
                    await client.close()

        except Exception as exc:
            duration = (datetime.now(timezone.utc) - start_time).total_seconds()
            logger.error(
                "scheduled learning task failed",
                error=str(exc),
                error_type=type(exc).__name__,
                duration_seconds=duration,
                exc_info=True,
            )

    def _build_learning_payload(self, result) -> dict[str, object]:
        """Build the learning payload for recap-worker."""
        from dataclasses import asdict

        summary = asdict(result.summary)
        payload: dict[str, object] = {
            "summary": summary,
            "graph_override": {
                "graph_margin": result.summary.graph_margin_reference,
            },
            "metadata": {
                "captured_at": datetime.now(timezone.utc).isoformat(),
                "entries_observed": result.summary.total_records,
            },
        }
        if result.cluster_draft:
            payload["cluster_draft"] = result.cluster_draft
        return payload

