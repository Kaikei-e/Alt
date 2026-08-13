"""Reconcile orphaned runs usecase — startup crash recovery."""

from __future__ import annotations

from typing import TYPE_CHECKING

import structlog

if TYPE_CHECKING:
    from acolyte.port.job_queue import JobQueuePort

logger = structlog.get_logger(__name__)

# Not in start_run_uc._CIRCUIT_BREAKER_FAILURE_CODES on purpose — a crash
# during startup isn't evidence the pipeline itself is broken, so a fresh
# StartReportRun call shouldn't be cooldown-blocked by it.
_ORPHANED_FAILURE_CODE = "orphaned_after_restart"


class ReconcileOrphanedRunsUsecase:
    """Fail every unfinished run left behind by a crashed/restarted process.

    Nothing else in this service revisits an unfinished report_runs row —
    there is no poller and no automatic checkpoint-driven resume — so a run
    orphaned by a crash stays 'pending' or 'running' forever, wedging
    has_active_run (delete_report guard) and GetReport.active_run for its
    report indefinitely. This usecase runs once at lifespan startup, before
    the service accepts traffic, and moves every such run to 'failed' so a
    client (or operator, via scripts/resume_run.py) can act on it.

    This is deliberately *not* an automatic resume: whether a crashed run's
    in-flight side effects (news-creator calls already issued, evidence
    already gathered) are safe to replay from checkpoint is a per-pipeline
    design question, not a safe default to guess at startup.
    """

    def __init__(self, job_queue: JobQueuePort) -> None:
        self._job_queue = job_queue

    async def execute(self) -> int:
        orphaned = await self._job_queue.list_running_runs()
        for run in orphaned:
            logger.warning(
                "Reconciling orphaned run at startup",
                run_id=str(run.run_id),
                report_id=str(run.report_id),
                run_status=run.run_status,
            )
            await self._job_queue.fail_run(
                run.run_id,
                _ORPHANED_FAILURE_CODE,
                f"Run was still '{run.run_status}' at process startup — "
                "likely a crash or restart before the pipeline finished.",
            )
        return len(orphaned)
