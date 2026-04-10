"""Unit tests for FinalizerNode best_sections persistence."""

from __future__ import annotations

from datetime import UTC, datetime
from uuid import uuid4

import pytest

from acolyte.domain.report import Report, ReportSection
from acolyte.usecase.graph.nodes.finalizer_node import FinalizerNode


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
        self.sections = [ReportSection(report_id=self.report_id, section_key="analysis", current_version=0, display_order=0)]
        self.saved_bodies: dict[str, str] = {}

    async def get_report(self, report_id):
        return self.report

    async def bump_version(self, report_id, expected_version, change_reason, change_items, **kwargs):
        return expected_version + 1

    async def get_sections(self, report_id):
        return self.sections

    async def create_section(self, report_id, section_key, display_order):
        return None

    async def bump_section_version(self, report_id, section_key, expected_version, body, citations=None):
        self.saved_bodies[section_key] = body
        return expected_version + 1


@pytest.mark.asyncio
async def test_finalizer_always_prefers_best_sections_when_available() -> None:
    """Finalizer should persist best_sections even if the latest revision is non-empty."""
    repo = FakeRepo()
    node = FinalizerNode(repo)

    state = {
        "report_id": str(repo.report_id),
        "outline": [{"key": "analysis"}],
        "brief": {"topic": "AI"},
        "sections": {"analysis": "Short latest body."},
        "best_sections": {"analysis": "Better body from earlier revision."},
        "section_citations": {"analysis": []},
    }

    result = await node(state)

    assert result["final_version_no"] == 1
    assert repo.saved_bodies["analysis"] == "Better body from earlier revision."
