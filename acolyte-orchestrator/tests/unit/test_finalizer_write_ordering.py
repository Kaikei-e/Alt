"""Unit tests for FinalizerNode write ordering.

The report version and its change_items are the published claim "these sections
were regenerated at vN+1". They must never become visible before the section
bodies they describe are durable.
"""

from __future__ import annotations

from datetime import UTC, datetime
from typing import TYPE_CHECKING
from uuid import UUID, uuid4

import pytest

from acolyte.domain.report import ChangeItem, Report, ReportSection, ReportVersion, SectionVersion
from acolyte.usecase.graph.nodes.finalizer_node import FinalizerNode

if TYPE_CHECKING:
    from acolyte.domain.brief import ReportBrief


class SectionWriteError(RuntimeError):
    """Simulated repository failure while writing one section body."""

    def __init__(self, section_key: str) -> None:
        super().__init__(f"connection lost while writing section {section_key}")


class FakeRepo:
    """Repo double that records writes and can fail one section body write."""

    def __init__(self, section_keys: list[str], fail_on_section: str | None = None) -> None:
        self.report_id = uuid4()
        self.report = Report(
            report_id=self.report_id,
            title="Test",
            report_type="weekly_briefing",
            current_version=0,
            latest_successful_run_id=None,
            created_at=datetime.now(UTC),
        )
        self.sections = [
            ReportSection(report_id=self.report_id, section_key=key, current_version=0, display_order=i)
            for i, key in enumerate(section_keys)
        ]
        self._fail_on_section = fail_on_section
        self.saved_bodies: dict[str, str] = {}
        self.bump_version_calls: list[list[ChangeItem]] = []

    async def create_report(self, title: str, report_type: str) -> Report:
        return self.report

    async def create_brief(self, report_id: UUID, brief: ReportBrief) -> None:
        pass

    async def get_brief(self, report_id: UUID) -> ReportBrief | None:
        return None

    async def get_report(self, report_id: UUID) -> Report | None:
        return self.report

    async def list_reports(self, cursor: str | None, limit: int) -> tuple[list[Report], str | None]:
        return [], None

    async def bump_version(
        self,
        report_id: UUID,
        expected_version: int,
        change_reason: str,
        change_items: list[ChangeItem],
        **kwargs: object,
    ) -> int:
        self.bump_version_calls.append(change_items)
        self.report = Report(
            report_id=self.report.report_id,
            title=self.report.title,
            report_type=self.report.report_type,
            current_version=expected_version + 1,
            latest_successful_run_id=self.report.latest_successful_run_id,
            created_at=self.report.created_at,
        )
        return expected_version + 1

    async def get_report_version(self, report_id: UUID, version_no: int) -> ReportVersion | None:
        return None

    async def list_report_versions(
        self, report_id: UUID, cursor: str | None, limit: int
    ) -> tuple[list[ReportVersion], str | None]:
        return [], None

    async def get_change_items(self, report_id: UUID, version_no: int) -> list[ChangeItem]:
        return []

    async def get_sections(self, report_id: UUID) -> list[ReportSection]:
        return self.sections

    async def create_section(self, report_id: UUID, section_key: str, display_order: int) -> ReportSection:
        section = ReportSection(
            report_id=report_id, section_key=section_key, current_version=0, display_order=display_order
        )
        self.sections.append(section)
        return section

    async def bump_section_version(
        self,
        report_id: UUID,
        section_key: str,
        expected_version: int,
        body: str,
        citations: list[dict] | None = None,
    ) -> int:
        if section_key == self._fail_on_section:
            raise SectionWriteError(section_key)
        self.saved_bodies[section_key] = body
        return expected_version + 1

    async def get_section_version(self, report_id: UUID, section_key: str, version_no: int) -> SectionVersion | None:
        return None

    async def has_active_run(self, report_id: UUID) -> bool:
        return False

    async def delete_report(self, report_id: UUID) -> None:
        return None


@pytest.mark.asyncio
async def test_section_write_failure_leaves_report_version_unpublished() -> None:
    """A mid-loop section failure must not leave current_version pointing at a half-written report."""
    repo = FakeRepo(["intro", "analysis", "outlook"], fail_on_section="analysis")
    node = FinalizerNode(repo)

    state = {
        "report_id": str(repo.report_id),
        "outline": [{"key": "intro"}, {"key": "analysis"}, {"key": "outlook"}],
        "brief": {"topic": "AI"},
        "sections": {"intro": "Intro body.", "analysis": "Analysis body.", "outlook": "Outlook body."},
    }

    with pytest.raises(SectionWriteError, match="connection lost"):
        await node(state)

    assert repo.bump_version_calls == [], "report version was published before all section bodies were durable"
    assert repo.report.current_version == 0


@pytest.mark.asyncio
async def test_change_items_only_claim_sections_that_were_written() -> None:
    """change_items is the regeneration claim — it must not name a section the node never wrote."""
    repo = FakeRepo(["intro"])
    node = FinalizerNode(repo)

    state = {
        "report_id": str(repo.report_id),
        # "dropped" survives in state["sections"] but is no longer in the outline,
        # so the persistence loop never writes it.
        "outline": [{"key": "intro"}],
        "brief": {"topic": "AI"},
        "sections": {"intro": "Intro body.", "dropped": "Stale body from an earlier revision."},
    }

    result = await node(state)

    assert result["final_version_no"] == 1
    assert len(repo.bump_version_calls) == 1
    claimed = {item.field_name for item in repo.bump_version_calls[0]}
    assert claimed == {"section:intro"}
