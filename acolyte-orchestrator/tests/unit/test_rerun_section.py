"""Unit tests for RerunSectionUsecase."""

from __future__ import annotations

from datetime import UTC, datetime
from uuid import UUID, uuid4

import pytest

from acolyte.domain.brief import ReportBrief
from acolyte.domain.report import ChangeItem, Report, ReportSection, ReportVersion, SectionVersion
from acolyte.port.llm_provider import LLMResponse
from acolyte.usecase.rerun_section_uc import RerunSectionUsecase


class FakeLLM:
    def __init__(self, text: str = "Regenerated section content.") -> None:
        self._text = text
        self.call_count = 0

    async def generate(self, prompt: str, **kwargs: object) -> LLMResponse:
        self.call_count += 1
        return LLMResponse(text=self._text, model="fake")


class FakeRepo:
    def __init__(self) -> None:
        self.reports: dict[UUID, Report] = {}
        self.briefs: dict[UUID, ReportBrief] = {}
        self.sections: dict[UUID, list[ReportSection]] = {}
        self.section_versions: dict[tuple[UUID, str, int], SectionVersion] = {}
        self.versions: dict[UUID, list[ReportVersion]] = {}
        self.bumped_sections: list[tuple[UUID, str, int, str]] = []
        self.bumped_versions: list[tuple[UUID, int, str]] = []

    async def create_report(self, title: str, report_type: str) -> Report:
        rid = uuid4()
        report = Report(
            report_id=rid,
            title=title,
            report_type=report_type,
            current_version=0,
            latest_successful_run_id=None,
            created_at=datetime.now(UTC),
        )
        self.reports[rid] = report
        return report

    async def get_report(self, report_id: UUID) -> Report | None:
        return self.reports.get(report_id)

    async def create_brief(self, report_id: UUID, brief: ReportBrief) -> None:
        self.briefs[report_id] = brief

    async def get_brief(self, report_id: UUID) -> ReportBrief | None:
        return self.briefs.get(report_id)

    async def get_sections(self, report_id: UUID) -> list[ReportSection]:
        return self.sections.get(report_id, [])

    async def get_report_version(self, report_id: UUID, version_no: int) -> ReportVersion | None:
        for v in self.versions.get(report_id, []):
            if v.version_no == version_no:
                return v
        return None

    async def bump_section_version(
        self, report_id: UUID, section_key: str, expected_version: int, body: str, citations=None
    ) -> int:
        new_v = expected_version + 1
        self.bumped_sections.append((report_id, section_key, new_v, body))
        return new_v

    async def bump_version(
        self, report_id: UUID, expected_version: int, change_reason: str, change_items: list[ChangeItem], **kwargs
    ) -> int:
        new_v = expected_version + 1
        self.bumped_versions.append((report_id, new_v, change_reason))
        report = self.reports[report_id]
        self.reports[report_id] = Report(
            report_id=report.report_id,
            title=report.title,
            report_type=report.report_type,
            current_version=new_v,
            latest_successful_run_id=report.latest_successful_run_id,
            created_at=report.created_at,
        )
        return new_v

    async def list_reports(self, cursor: str | None, limit: int) -> tuple[list[Report], str | None]:
        return list(self.reports.values()), None

    async def list_report_versions(
        self, report_id: UUID, cursor: str | None, limit: int
    ) -> tuple[list[ReportVersion], str | None]:
        return list(self.versions.get(report_id, [])), None

    async def get_change_items(self, report_id: UUID, version_no: int) -> list[ChangeItem]:
        return []

    async def create_section(self, report_id: UUID, section_key: str, display_order: int) -> ReportSection:
        sec = ReportSection(
            report_id=report_id, section_key=section_key, current_version=0, display_order=display_order
        )
        self.sections.setdefault(report_id, []).append(sec)
        return sec

    async def get_section_version(self, report_id: UUID, section_key: str, version_no: int) -> SectionVersion | None:
        return self.section_versions.get((report_id, section_key, version_no))

    async def has_active_run(self, report_id: UUID) -> bool:
        return False

    async def delete_report(self, report_id: UUID) -> None:
        self.reports.pop(report_id, None)
        self.briefs.pop(report_id, None)
        self.sections.pop(report_id, None)


def _make_repo_with_report() -> tuple[FakeRepo, UUID]:
    repo = FakeRepo()
    rid = uuid4()
    repo.reports[rid] = Report(
        report_id=rid,
        title="Test Report",
        report_type="weekly_briefing",
        current_version=1,
        latest_successful_run_id=None,
        created_at=datetime.now(UTC),
    )
    repo.briefs[rid] = ReportBrief(topic="AI semiconductor", report_type="weekly_briefing")
    repo.sections[rid] = [
        ReportSection(report_id=rid, section_key="summary", current_version=1, display_order=0),
        ReportSection(report_id=rid, section_key="analysis", current_version=1, display_order=1),
    ]
    repo.versions[rid] = [
        ReportVersion(
            report_id=rid,
            version_no=1,
            change_seq=1,
            change_reason="gen",
            created_at=datetime.now(UTC),
            outline_snapshot=[
                {"key": "summary", "title": "Executive Summary"},
                {"key": "analysis", "title": "Analysis"},
            ],
        ),
    ]
    return repo, rid


@pytest.mark.asyncio
async def test_rerun_section_generates_new_body() -> None:
    repo, rid = _make_repo_with_report()
    llm = FakeLLM("New summary content.")
    uc = RerunSectionUsecase(repo, llm)

    await uc.execute(rid, "summary")

    assert llm.call_count == 1
    assert len(repo.bumped_sections) == 1
    assert repo.bumped_sections[0][1] == "summary"
    assert repo.bumped_sections[0][3] == "New summary content."


@pytest.mark.asyncio
async def test_rerun_section_bumps_report_version() -> None:
    repo, rid = _make_repo_with_report()
    llm = FakeLLM()
    uc = RerunSectionUsecase(repo, llm)

    new_v = await uc.execute(rid, "summary")

    assert new_v == 2
    assert len(repo.bumped_versions) == 1
    assert "summary" in repo.bumped_versions[0][2]


@pytest.mark.asyncio
async def test_rerun_section_not_found_raises() -> None:
    repo, rid = _make_repo_with_report()
    uc = RerunSectionUsecase(repo, FakeLLM())

    with pytest.raises(ValueError, match="Section"):
        await uc.execute(rid, "nonexistent")


@pytest.mark.asyncio
async def test_rerun_section_report_not_found_raises() -> None:
    repo = FakeRepo()
    uc = RerunSectionUsecase(repo, FakeLLM())

    with pytest.raises(ValueError, match="Report"):
        await uc.execute(uuid4(), "summary")
