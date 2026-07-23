"""Start report run usecase."""

from __future__ import annotations

from datetime import UTC, datetime, timedelta
from typing import TYPE_CHECKING
from uuid import UUID

if TYPE_CHECKING:
    from acolyte.domain.run import ReportRun
    from acolyte.port.job_queue import JobQueuePort
    from acolyte.port.report_repository import ReportRepositoryPort

# failure_code values the circuit breaker treats as non-transient — a run
# that failed this recently with one of these shouldn't be silently retried.
# 'no_content' is report_graph._finalize_guard's content-store total-wipeout
# code (hydrator/compressor/quote_selector/fact_normalizer all produced
# nothing groundable despite curated evidence).
_CIRCUIT_BREAKER_FAILURE_CODES = frozenset({"pipeline_error", "no_evidence", "no_content"})

# Cooldown window default (10 minutes). The composition root can override
# this per instance — e.g. from Settings — via the failure_cooldown param.
_DEFAULT_FAILURE_COOLDOWN = timedelta(minutes=10)


class StartRunRejectedError(ValueError):
    """Raised when the circuit breaker rejects a new run.

    A ValueError subclass (not a sibling type) so existing ``except
    ValueError`` call sites keep working, but distinct from a plain
    ValueError (e.g. "report not found") so connect_service can map a
    breaker rejection to Code.FAILED_PRECONDITION instead of the
    Code.NOT_FOUND used for a missing report.
    """


class StartRunUsecase:
    def __init__(
        self,
        report_repo: ReportRepositoryPort,
        job_queue: JobQueuePort,
        *,
        failure_cooldown: timedelta = _DEFAULT_FAILURE_COOLDOWN,
    ) -> None:
        self._report_repo = report_repo
        self._job_queue = job_queue
        self._failure_cooldown = failure_cooldown

    async def execute(self, report_id: UUID, *, now: datetime | None = None) -> ReportRun:
        report = await self._report_repo.get_report(report_id)
        if report is None:
            raise ValueError(f"Report {report_id} not found")  # noqa: TRY003 — caught generically as ValueError at the connect_service Handler boundary

        latest_run = await self._job_queue.get_latest_run_for_report(report_id)
        if latest_run is not None and self._tripped_by(latest_run, now or datetime.now(UTC)):
            raise StartRunRejectedError(  # noqa: TRY003 — caught explicitly as StartRunRejectedError at the connect_service Handler boundary
                f"Report {report_id} run rejected: most recent run {latest_run.run_id} failed "
                f"({latest_run.failure_code}) within the circuit-breaker cooldown"
            )

        return await self._job_queue.create_run(report_id, report.current_version + 1)

    def _tripped_by(self, run: ReportRun, now: datetime) -> bool:
        """True when `run` is a recent-enough non-transient failure to block a retry."""
        if run.run_status != "failed" or run.failure_code not in _CIRCUIT_BREAKER_FAILURE_CODES:
            return False
        if run.finished_at is None:
            return False
        return now - run.finished_at < self._failure_cooldown
