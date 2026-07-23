"""Unit tests for StartRunUsecase — the circuit breaker on repeated pipeline failures.

Rejects a new run when the *most recent* run for the report failed with a
failure_code the pipeline itself considers non-transient ('pipeline_error',
'no_evidence', 'no_content') and that failure happened within the cooldown
window. A run that failed with any other code, or that failed further back
than the cooldown, or a most-recent run that isn't a failure at all, must
not trip the breaker.

A breaker rejection raises the dedicated ``StartRunRejectedError`` (a
``ValueError`` subclass) so connect_service can map it to
``Code.FAILED_PRECONDITION`` instead of the generic ``Code.NOT_FOUND`` used
for "report not found".
"""

from __future__ import annotations

from datetime import UTC, datetime, timedelta
from typing import TYPE_CHECKING
from uuid import UUID, uuid4

import pytest

from acolyte.domain.report import ChangeItem, Report, ReportSection, ReportVersion, SectionVersion
from acolyte.domain.run import ReportJob, ReportRun
from acolyte.usecase.start_run_uc import StartRunRejectedError, StartRunUsecase

if TYPE_CHECKING:
    from acolyte.domain.brief import ReportBrief


class _FakeReportRepo:
    """Stubs the full ReportRepositoryPort — StartRunUsecase only calls get_report()."""

    def __init__(self) -> None:
        self.reports: dict[UUID, Report] = {}

    async def get_report(self, report_id: UUID) -> Report | None:
        return self.reports.get(report_id)

    async def create_report(self, title: str, report_type: str) -> Report:
        raise NotImplementedError

    async def create_brief(self, report_id: UUID, brief: ReportBrief) -> None:
        raise NotImplementedError

    async def get_brief(self, report_id: UUID) -> ReportBrief | None:
        raise NotImplementedError

    async def list_reports(self, cursor: str | None, limit: int) -> tuple[list[Report], str | None]:
        raise NotImplementedError

    async def bump_version(  # noqa: PLR0913 — mirrors ReportRepositoryPort's signature
        self,
        report_id: UUID,
        expected_version: int,
        change_reason: str,
        change_items: list[ChangeItem],
        *,
        prompt_template_version: str | None = None,
        scope_snapshot: dict | None = None,
        outline_snapshot: list[dict] | dict | None = None,
        summary_snapshot: str | None = None,
    ) -> int:
        raise NotImplementedError

    async def get_report_version(self, report_id: UUID, version_no: int) -> ReportVersion | None:
        raise NotImplementedError

    async def list_report_versions(
        self, report_id: UUID, cursor: str | None, limit: int
    ) -> tuple[list[ReportVersion], str | None]:
        raise NotImplementedError

    async def get_change_items(self, report_id: UUID, version_no: int) -> list[ChangeItem]:
        raise NotImplementedError

    async def create_section(self, report_id: UUID, section_key: str, display_order: int) -> ReportSection:
        raise NotImplementedError

    async def get_sections(self, report_id: UUID) -> list[ReportSection]:
        raise NotImplementedError

    async def bump_section_version(
        self,
        report_id: UUID,
        section_key: str,
        expected_version: int,
        body: str,
        citations: list[dict] | None = None,
    ) -> int:
        raise NotImplementedError

    async def get_section_version(self, report_id: UUID, section_key: str, version_no: int) -> SectionVersion | None:
        raise NotImplementedError

    async def has_active_run(self, report_id: UUID) -> bool:
        raise NotImplementedError

    async def delete_report(self, report_id: UUID) -> None:
        raise NotImplementedError


class _FakeJobQueue:
    def __init__(self) -> None:
        self.latest_run: ReportRun | None = None
        self.created: list[tuple[UUID, int]] = []

    async def get_latest_run_for_report(self, report_id: UUID) -> ReportRun | None:
        return self.latest_run

    async def create_run(self, report_id: UUID, target_version_no: int) -> ReportRun:
        self.created.append((report_id, target_version_no))
        return ReportRun(
            run_id=uuid4(),
            report_id=report_id,
            target_version_no=target_version_no,
            run_status="pending",
        )

    # Rest of JobQueuePort — not exercised by StartRunUsecase.
    async def get_run(self, run_id: UUID) -> ReportRun | None:
        raise NotImplementedError

    async def get_active_run_for_report(self, report_id: UUID) -> ReportRun | None:
        raise NotImplementedError

    async def claim_job(self, worker_id: str) -> ReportJob | None:
        raise NotImplementedError

    async def update_job_status(self, job_id: UUID, status: str) -> None:
        raise NotImplementedError

    async def complete_job(self, job_id: UUID) -> None:
        raise NotImplementedError

    async def fail_job(self, job_id: UUID, failure_message: str) -> None:
        raise NotImplementedError

    async def mark_running(self, run_id: UUID, planner_model: str, writer_model: str, critic_model: str) -> None:
        raise NotImplementedError

    async def complete_run(self, run_id: UUID) -> None:
        raise NotImplementedError

    async def fail_run(self, run_id: UUID, failure_code: str, failure_message: str) -> None:
        raise NotImplementedError


def _report(report_id: UUID, current_version: int = 0) -> Report:
    return Report(
        report_id=report_id,
        title="Weekly briefing",
        report_type="weekly_briefing",
        current_version=current_version,
        latest_successful_run_id=None,
        created_at=datetime.now(UTC),
    )


def _failed_run(*, failure_code: str, finished_at: datetime | None) -> ReportRun:
    return ReportRun(
        run_id=uuid4(),
        report_id=uuid4(),
        target_version_no=1,
        run_status="failed",
        finished_at=finished_at,
        failure_code=failure_code,
        failure_message="boom",
    )


@pytest.mark.asyncio
async def test_execute_raises_when_report_not_found() -> None:
    repo = _FakeReportRepo()
    jobs = _FakeJobQueue()
    uc = StartRunUsecase(repo, jobs)

    with pytest.raises(ValueError, match="not found") as exc_info:
        await uc.execute(uuid4())

    # Plain ValueError, not the breaker-specific subclass — connect_service
    # must be able to tell "report not found" (Code.NOT_FOUND) apart from
    # a breaker rejection (Code.FAILED_PRECONDITION).
    assert not isinstance(exc_info.value, StartRunRejectedError)


@pytest.mark.asyncio
async def test_execute_creates_run_when_no_prior_run_exists() -> None:
    report_id = uuid4()
    repo = _FakeReportRepo()
    repo.reports[report_id] = _report(report_id, current_version=2)
    jobs = _FakeJobQueue()
    jobs.latest_run = None
    uc = StartRunUsecase(repo, jobs)

    run = await uc.execute(report_id)

    assert run.report_id == report_id
    assert jobs.created == [(report_id, 3)]


@pytest.mark.asyncio
async def test_execute_rejects_run_within_cooldown_after_pipeline_error() -> None:
    report_id = uuid4()
    repo = _FakeReportRepo()
    repo.reports[report_id] = _report(report_id)
    now = datetime(2026, 7, 23, 12, 0, tzinfo=UTC)
    jobs = _FakeJobQueue()
    jobs.latest_run = _failed_run(failure_code="pipeline_error", finished_at=now - timedelta(minutes=3))
    uc = StartRunUsecase(repo, jobs)

    with pytest.raises(StartRunRejectedError, match="cooldown"):
        await uc.execute(report_id, now=now)

    assert jobs.created == []


@pytest.mark.asyncio
async def test_execute_rejects_run_within_cooldown_after_no_evidence_failure() -> None:
    """'no_evidence' is not emitted by any producer yet, but the breaker must already honor it."""
    report_id = uuid4()
    repo = _FakeReportRepo()
    repo.reports[report_id] = _report(report_id)
    now = datetime(2026, 7, 23, 12, 0, tzinfo=UTC)
    jobs = _FakeJobQueue()
    jobs.latest_run = _failed_run(failure_code="no_evidence", finished_at=now - timedelta(minutes=1))
    uc = StartRunUsecase(repo, jobs)

    with pytest.raises(StartRunRejectedError, match="cooldown"):
        await uc.execute(report_id, now=now)


@pytest.mark.asyncio
async def test_execute_rejects_run_within_cooldown_after_no_content_failure() -> None:
    """'no_content' (finalize_guard's content-store total-wipeout code) must also trip the breaker."""
    report_id = uuid4()
    repo = _FakeReportRepo()
    repo.reports[report_id] = _report(report_id)
    now = datetime(2026, 7, 23, 12, 0, tzinfo=UTC)
    jobs = _FakeJobQueue()
    jobs.latest_run = _failed_run(failure_code="no_content", finished_at=now - timedelta(minutes=1))
    uc = StartRunUsecase(repo, jobs)

    with pytest.raises(StartRunRejectedError, match="cooldown"):
        await uc.execute(report_id, now=now)

    assert jobs.created == []


@pytest.mark.asyncio
async def test_execute_allows_run_after_cooldown_elapses() -> None:
    report_id = uuid4()
    repo = _FakeReportRepo()
    repo.reports[report_id] = _report(report_id, current_version=1)
    now = datetime(2026, 7, 23, 12, 0, tzinfo=UTC)
    jobs = _FakeJobQueue()
    jobs.latest_run = _failed_run(failure_code="pipeline_error", finished_at=now - timedelta(minutes=11))
    uc = StartRunUsecase(repo, jobs)

    run = await uc.execute(report_id, now=now)

    assert run.report_id == report_id
    assert jobs.created == [(report_id, 2)]


@pytest.mark.asyncio
async def test_execute_allows_run_when_latest_failure_code_is_not_circuit_breaker_eligible() -> None:
    report_id = uuid4()
    repo = _FakeReportRepo()
    repo.reports[report_id] = _report(report_id)
    now = datetime(2026, 7, 23, 12, 0, tzinfo=UTC)
    jobs = _FakeJobQueue()
    jobs.latest_run = _failed_run(failure_code="pipeline_crashed", finished_at=now - timedelta(minutes=1))
    uc = StartRunUsecase(repo, jobs)

    run = await uc.execute(report_id, now=now)

    assert run.report_id == report_id


@pytest.mark.asyncio
async def test_execute_allows_run_when_latest_run_succeeded() -> None:
    """Only the most recent run matters — a prior failure doesn't linger past a later success."""
    report_id = uuid4()
    repo = _FakeReportRepo()
    repo.reports[report_id] = _report(report_id)
    now = datetime(2026, 7, 23, 12, 0, tzinfo=UTC)
    jobs = _FakeJobQueue()
    jobs.latest_run = ReportRun(
        run_id=uuid4(),
        report_id=report_id,
        target_version_no=1,
        run_status="succeeded",
        finished_at=now - timedelta(minutes=1),
    )
    uc = StartRunUsecase(repo, jobs)

    run = await uc.execute(report_id, now=now)

    assert run.report_id == report_id


@pytest.mark.asyncio
async def test_execute_respects_injected_failure_cooldown() -> None:
    report_id = uuid4()
    repo = _FakeReportRepo()
    repo.reports[report_id] = _report(report_id)
    now = datetime(2026, 7, 23, 12, 0, tzinfo=UTC)
    jobs = _FakeJobQueue()
    jobs.latest_run = _failed_run(failure_code="pipeline_error", finished_at=now - timedelta(minutes=3))
    uc = StartRunUsecase(repo, jobs, failure_cooldown=timedelta(minutes=1))

    run = await uc.execute(report_id, now=now)

    assert run.report_id == report_id
