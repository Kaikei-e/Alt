"""Unit tests for FinalizerNode's forced-accept change_reason suffix.

When critique["forced_accept"] is True (critic hit MAX_REVISIONS with an
unresolved "revise" verdict), the persisted version's change_reason must
carry a structured suffix so it is distinguishable from a genuine accept.
"""

from __future__ import annotations

from datetime import UTC, datetime
from typing import TYPE_CHECKING
from uuid import UUID, uuid4

from acolyte.domain.report import ChangeItem, Report, ReportSection, ReportVersion, SectionVersion
from acolyte.usecase.graph.nodes.finalizer_node import FinalizerNode

if TYPE_CHECKING:
    from acolyte.domain.brief import ReportBrief


class FakeRepo:
    def __init__(self) -> None:
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
            ReportSection(report_id=self.report_id, section_key="analysis", current_version=0, display_order=0)
        ]
        self.last_change_reason: str | None = None

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
        self.last_change_reason = change_reason
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
        return ReportSection(
            report_id=report_id, section_key=section_key, current_version=0, display_order=display_order
        )

    async def bump_section_version(
        self,
        report_id: UUID,
        section_key: str,
        expected_version: int,
        body: str,
        citations: list[dict] | None = None,
    ) -> int:
        return expected_version + 1

    async def get_section_version(self, report_id: UUID, section_key: str, version_no: int) -> SectionVersion | None:
        return None

    async def has_active_run(self, report_id: UUID) -> bool:
        return False

    async def delete_report(self, report_id: UUID) -> None:
        return None


async def test_change_reason_gets_forced_accept_suffix() -> None:
    repo = FakeRepo()
    node = FinalizerNode(repo)

    state = {
        "report_id": str(repo.report_id),
        "outline": [{"key": "analysis"}],
        "brief": {"topic": "AI"},
        "sections": {"analysis": "Body content."},
        "section_citations": {"analysis": []},
        "revision_count": 3,
        "critique": {"verdict": "revise", "forced_accept": True},
    }

    result = await node(state)

    assert result["final_version_no"] == 1
    assert repo.last_change_reason == "LangGraph pipeline generation [forced-accept-after-3-revisions]"


async def test_change_reason_unchanged_without_forced_accept() -> None:
    repo = FakeRepo()
    node = FinalizerNode(repo)

    state = {
        "report_id": str(repo.report_id),
        "outline": [{"key": "analysis"}],
        "brief": {"topic": "AI"},
        "sections": {"analysis": "Body content."},
        "section_citations": {"analysis": []},
        "revision_count": 1,
        "critique": {"verdict": "accept"},
    }

    result = await node(state)

    assert result["final_version_no"] == 1
    assert repo.last_change_reason == "LangGraph pipeline generation"


async def test_change_reason_unchanged_when_no_critique() -> None:
    repo = FakeRepo()
    node = FinalizerNode(repo)

    state = {
        "report_id": str(repo.report_id),
        "outline": [{"key": "analysis"}],
        "brief": {"topic": "AI"},
        "sections": {"analysis": "Body content."},
        "section_citations": {"analysis": []},
    }

    result = await node(state)

    assert result["final_version_no"] == 1
    assert repo.last_change_reason == "LangGraph pipeline generation"
