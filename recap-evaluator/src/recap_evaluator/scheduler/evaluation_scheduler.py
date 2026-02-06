"""APScheduler-based evaluation scheduler."""

import structlog
from apscheduler.schedulers.asyncio import AsyncIOScheduler
from apscheduler.triggers.cron import CronTrigger

from recap_evaluator.config import Settings
from recap_evaluator.usecase.run_evaluation import RunEvaluationUsecase

logger = structlog.get_logger()


class EvaluationScheduler:
    """Cron-based scheduled evaluation runner."""

    def __init__(
        self,
        run_evaluation_usecase: RunEvaluationUsecase,
        settings: Settings,
    ) -> None:
        self._usecase = run_evaluation_usecase
        self._settings = settings
        self._scheduler = AsyncIOScheduler()

    async def _run_scheduled_evaluation(self) -> None:
        logger.info("Running scheduled evaluation")
        try:
            run = await self._usecase.execute(
                window_days=self._settings.evaluation_window_days
            )
            logger.info(
                "Scheduled evaluation completed",
                evaluation_id=str(run.evaluation_id),
                overall_alert_level=run.overall_alert_level.value,
            )
        except Exception as e:
            logger.error("Scheduled evaluation failed", error=str(e))

    def start(self) -> None:
        if not self._settings.enable_scheduler:
            logger.info("Scheduler disabled")
            return

        trigger = CronTrigger.from_crontab(self._settings.evaluation_schedule)
        self._scheduler.add_job(
            self._run_scheduled_evaluation,
            trigger,
            id="daily_evaluation",
        )
        self._scheduler.start()
        logger.info(
            "Scheduler started", schedule=self._settings.evaluation_schedule
        )

    def stop(self) -> None:
        if self._scheduler.running:
            self._scheduler.shutdown()
            logger.info("Scheduler stopped")
