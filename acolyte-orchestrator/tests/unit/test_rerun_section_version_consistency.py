"""RerunSection must never leave a section body no report version owns.

RerunSectionUsecase reads ``report.current_version`` and the section's
``current_version``, then spends tens of seconds inside the writer LLM call,
then commits the section body and the report version in two separate
transactions using those *pre-LLM* numbers. Any concurrent writer that bumps
``reports.current_version`` in that window — another section rerun, or a
pipeline finalize — makes the report bump raise StaleVersionError *after* the
body is already committed. ``report_section_versions`` then holds a body that
no report version and no change_item references, GetReport renders it, and the
caller sees INTERNAL and retries on top of the corruption.

Both tests drive the real Connect handler so they pin what a caller observes,
not the usecase's internal call order.
"""

from __future__ import annotations

from collections.abc import Callable
from datetime import UTC, datetime
from typing import TYPE_CHECKING
from unittest.mock import MagicMock
from uuid import UUID, uuid4

import pytest
from connectrpc.code import Code
from connectrpc.errors import ConnectError

from acolyte.domain.exceptions import StaleVersionError
from acolyte.domain.report import ChangeItem, Report, ReportSection, ReportVersion, SectionVersion
from acolyte.gen.proto.alt.acolyte.v1 import acolyte_pb2
from acolyte.handler.connect_service import AcolyteConnectService
from acolyte.port.llm_provider import LLMResponse

if TYPE_CHECKING:
    from acolyte.domain.brief import ReportBrief


class _LockingRepo:
    """In-memory ReportRepositoryPort with real optimistic-lock semantics.

    The fakes in test_rerun_section.py accept any ``expected_version``; this
    one rejects a stale one exactly like PostgresReportGateway does, which is
    what surfaces the ordering defect.
    """

    def __init__(self, report_id: UUID) -> None:
        self.report = Report(
            report_id=report_id,
            title="Weekly briefing",
            report_type="weekly_briefing",
            current_version=1,
            latest_successful_run_id=None,
            created_at=datetime.now(UTC),
        )
        self.sections: dict[str, ReportSection] = {
            "summary": ReportSection(report_id=report_id, section_key="summary", current_version=1, display_order=0),
            "analysis": ReportSection(report_id=report_id, section_key="analysis", current_version=1, display_order=1),
        }
        self.section_bodies: dict[tuple[str, int], str] = {("summary", 1): "Original summary body."}
        self.report_versions: dict[int, ReportVersion] = {
            1: ReportVersion(
                report_id=report_id,
                version_no=1,
                change_seq=1,
                change_reason="gen",
                created_at=datetime.now(UTC),
                outline_snapshot=[{"key": "summary", "title": "Executive Summary"}],
            ),
        }
        self.change_items: dict[int, list[ChangeItem]] = {1: []}
        self.active_run = False
        self.steal_report_version_lock = False

    # --- concurrent-writer helper -------------------------------------

    def concurrent_rerun_of_same_section(self) -> None:
        """Simulate a sibling RerunSection landing on *our* section.

        The has_active_run guard only covers pipeline runs, so two reruns can
        share a section. The loser must abort on the section lock rather than
        overwrite the winner's body with one written from pre-LLM evidence.
        """
        ours = self.sections["summary"]
        self.sections["summary"] = ReportSection(
            report_id=ours.report_id,
            section_key=ours.section_key,
            current_version=ours.current_version + 1,
            display_order=ours.display_order,
        )
        self.section_bodies[("summary", ours.current_version + 1)] = "Sibling rerun body."
        new_v = self.report.current_version + 1
        self.report = Report(
            report_id=self.report.report_id,
            title=self.report.title,
            report_type=self.report.report_type,
            current_version=new_v,
            latest_successful_run_id=self.report.latest_successful_run_id,
            created_at=self.report.created_at,
        )

    def concurrent_rerun_of_other_section(self) -> None:
        """Simulate another RerunSection landing on this report.

        It bumps the report version (and its own section), leaving *our*
        section version untouched — the window in which the section bump
        still succeeds but the report bump cannot.
        """
        other = self.sections["analysis"]
        self.sections["analysis"] = ReportSection(
            report_id=other.report_id,
            section_key=other.section_key,
            current_version=other.current_version + 1,
            display_order=other.display_order,
        )
        new_v = self.report.current_version + 1
        self.report = Report(
            report_id=self.report.report_id,
            title=self.report.title,
            report_type=self.report.report_type,
            current_version=new_v,
            latest_successful_run_id=self.report.latest_successful_run_id,
            created_at=self.report.created_at,
        )
        self.report_versions[new_v] = ReportVersion(
            report_id=self.report.report_id,
            version_no=new_v,
            change_seq=new_v,
            change_reason="Section rerun: analysis",
            created_at=datetime.now(UTC),
        )
        self.change_items[new_v] = [ChangeItem(field_name="section:analysis", change_kind="regenerated")]

    def orphaned_section_bodies(self) -> list[tuple[str, int]]:
        """Committed section bodies that no report change_item accounts for."""
        claimed = {item.field_name.removeprefix("section:") for items in self.change_items.values() for item in items}
        return [(key, version_no) for (key, version_no) in self.section_bodies if version_no > 1 and key not in claimed]

    # --- ReportRepositoryPort ------------------------------------------

    async def get_report(self, report_id: UUID) -> Report | None:
        return self.report if report_id == self.report.report_id else None

    async def get_sections(self, report_id: UUID) -> list[ReportSection]:
        return sorted(self.sections.values(), key=lambda s: s.display_order)

    async def get_report_version(self, report_id: UUID, version_no: int) -> ReportVersion | None:
        return self.report_versions.get(version_no)

    async def get_section_version(self, report_id: UUID, section_key: str, version_no: int) -> SectionVersion | None:
        body = self.section_bodies.get((section_key, version_no))
        if body is None:
            return None
        return SectionVersion(
            report_id=report_id,
            section_key=section_key,
            version_no=version_no,
            body=body,
            citations=[{"source_id": "S1", "source_type": "article", "quote": "quoted evidence"}],
        )

    async def bump_version(
        self,
        report_id: UUID,
        expected_version: int,
        change_reason: str,
        change_items: list[ChangeItem],
        **kwargs: object,
    ) -> int:
        if self.steal_report_version_lock:
            # Another writer lands in the gap between the caller's re-read
            # and this statement — the residual race no app-layer re-read can
            # close, and exactly what the optimistic lock exists to catch.
            self.steal_report_version_lock = False
            self.concurrent_rerun_of_other_section()
        if expected_version != self.report.current_version:
            raise StaleVersionError(report_id, expected_version)
        new_v = expected_version + 1
        self.report = Report(
            report_id=self.report.report_id,
            title=self.report.title,
            report_type=self.report.report_type,
            current_version=new_v,
            latest_successful_run_id=self.report.latest_successful_run_id,
            created_at=self.report.created_at,
        )
        self.report_versions[new_v] = ReportVersion(
            report_id=report_id,
            version_no=new_v,
            change_seq=new_v,
            change_reason=change_reason,
            created_at=datetime.now(UTC),
        )
        self.change_items[new_v] = list(change_items)
        return new_v

    async def bump_section_version(
        self,
        report_id: UUID,
        section_key: str,
        expected_version: int,
        body: str,
        citations: list[dict] | None = None,
    ) -> int:
        section = self.sections[section_key]
        if expected_version != section.current_version:
            raise StaleVersionError(report_id, expected_version)
        new_v = expected_version + 1
        self.sections[section_key] = ReportSection(
            report_id=section.report_id,
            section_key=section.section_key,
            current_version=new_v,
            display_order=section.display_order,
        )
        self.section_bodies[(section_key, new_v)] = body
        return new_v

    async def has_active_run(self, report_id: UUID) -> bool:
        return self.active_run

    async def get_brief(self, report_id: UUID) -> ReportBrief | None:
        return None

    # Unused stubs for the rest of ReportRepositoryPort.
    async def create_report(self, title: str, report_type: str) -> Report:
        raise NotImplementedError

    async def create_brief(self, report_id: UUID, brief: ReportBrief) -> None:
        raise NotImplementedError

    async def list_reports(self, cursor: str | None, limit: int) -> tuple[list[Report], str | None]:
        raise NotImplementedError

    async def list_report_versions(
        self, report_id: UUID, cursor: str | None, limit: int
    ) -> tuple[list[ReportVersion], str | None]:
        raise NotImplementedError

    async def get_change_items(self, report_id: UUID, version_no: int) -> list[ChangeItem]:
        return self.change_items.get(version_no, [])

    async def create_section(self, report_id: UUID, section_key: str, display_order: int) -> ReportSection:
        raise NotImplementedError

    async def delete_report(self, report_id: UUID) -> None:
        raise NotImplementedError


class _SlowLLM:
    """Writer stub whose generate() is the window a concurrent writer uses."""

    def __init__(self, during_generate: Callable[[], None] | None = None) -> None:
        self._during_generate = during_generate
        self.call_count = 0

    async def generate(self, prompt: str, **kwargs: object) -> LLMResponse:
        self.call_count += 1
        if self._during_generate is not None:
            self._during_generate()
        return LLMResponse(text="Regenerated summary body.", model="fake")


def _service(repo: _LockingRepo, llm: _SlowLLM) -> AcolyteConnectService:
    return AcolyteConnectService(MagicMock(), repo, llm=llm)  # type: ignore[bad-argument-type]


@pytest.mark.asyncio
async def test_rerun_survives_a_concurrent_writer_during_the_llm_call() -> None:
    """The report version read before the writer call is stale by the time it
    returns. Bumping with that pre-LLM number fails the optimistic lock after
    the body is already committed; re-reading immediately before the bumps
    lets the rerun land cleanly instead.
    """
    report_id = uuid4()
    repo = _LockingRepo(report_id)
    llm = _SlowLLM(during_generate=repo.concurrent_rerun_of_other_section)
    service = _service(repo, llm)

    await service.rerun_section(
        acolyte_pb2.RerunSectionRequest(report_id=str(report_id), section_key="summary"),
        ctx=None,  # type: ignore[bad-argument-type]
    )

    assert repo.orphaned_section_bodies() == []
    assert repo.change_items[3] == [ChangeItem(field_name="section:summary", change_kind="regenerated")]
    assert repo.section_bodies[("summary", 2)] == "Regenerated summary body."


@pytest.mark.asyncio
async def test_rerun_writes_no_body_when_the_report_version_lock_is_lost() -> None:
    """The residual race (a writer landing between the re-read and the bump)
    must abort before any section body is committed. Committing the body
    first leaves content GetReport renders that no report version or
    change_item owns, and the caller sees INTERNAL and retries on top of it.
    """
    report_id = uuid4()
    repo = _LockingRepo(report_id)
    repo.steal_report_version_lock = True
    service = _service(repo, _SlowLLM())

    with pytest.raises(ConnectError):
        await service.rerun_section(
            acolyte_pb2.RerunSectionRequest(report_id=str(report_id), section_key="summary"),
            ctx=None,  # type: ignore[bad-argument-type]
        )

    assert repo.orphaned_section_bodies() == []
    assert repo.sections["summary"].current_version == 1


@pytest.mark.asyncio
async def test_rerun_refuses_while_a_pipeline_run_is_active() -> None:
    """A pipeline run rewrites every section on finalize. Rerunning underneath
    it can only lose the race for the same optimistic locks, so refuse up
    front rather than burn a multi-second LLM call and half-commit.
    """
    report_id = uuid4()
    repo = _LockingRepo(report_id)
    repo.active_run = True
    llm = _SlowLLM()
    service = _service(repo, llm)

    with pytest.raises(ConnectError) as excinfo:
        await service.rerun_section(
            acolyte_pb2.RerunSectionRequest(report_id=str(report_id), section_key="summary"),
            ctx=None,  # type: ignore[bad-argument-type]
        )

    # "a run owns this report" is a precondition the caller can wait out, not a
    # server fault. INTERNAL would both mislead the client and log a traceback
    # into the error-rate SLI on every occurrence. delete_report already returns
    # FAILED_PRECONDITION for the identical has_active_run check.
    assert excinfo.value.code == Code.FAILED_PRECONDITION
    assert llm.call_count == 0
    assert repo.sections["summary"].current_version == 1
    assert repo.report.current_version == 1


@pytest.mark.asyncio
async def test_rerun_commits_body_and_version_together_when_uncontended() -> None:
    """The happy path still lands both bumps: a new report version whose
    change_item names the section, and the regenerated body itself.
    """
    report_id = uuid4()
    repo = _LockingRepo(report_id)
    llm = _SlowLLM()
    service = _service(repo, llm)

    await service.rerun_section(
        acolyte_pb2.RerunSectionRequest(report_id=str(report_id), section_key="summary"),
        ctx=None,  # type: ignore[bad-argument-type]
    )

    assert repo.report.current_version == 2
    assert repo.change_items[2] == [ChangeItem(field_name="section:summary", change_kind="regenerated")]
    assert repo.section_bodies[("summary", 2)] == "Regenerated summary body."


@pytest.mark.asyncio
async def test_rerun_aborts_when_a_sibling_rerun_took_the_same_section() -> None:
    """Re-reading the section row before the bump would make this last-writer-wins.

    Both reruns read v1. The sibling lands v2 while our LLM call is out; our
    body was written from the v1 citations, so overwriting it as v3 silently
    discards the sibling's section from the rendered report. The section-level
    optimistic lock exists to stop exactly that, so it must be checked against
    the version we read *before* the writer call.
    """
    report_id = uuid4()
    repo = _LockingRepo(report_id)
    llm = _SlowLLM(during_generate=repo.concurrent_rerun_of_same_section)
    service = _service(repo, llm)

    with pytest.raises(ConnectError):
        await service.rerun_section(
            acolyte_pb2.RerunSectionRequest(report_id=str(report_id), section_key="summary"),
            ctx=None,  # type: ignore[bad-argument-type]
        )

    assert repo.section_bodies[("summary", 2)] == "Sibling rerun body.", "the sibling's body must survive"
    assert ("summary", 3) not in repo.section_bodies, "the losing rerun must not write a body from pre-LLM evidence"
