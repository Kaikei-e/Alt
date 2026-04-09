"""Unit tests for version bump logic."""

from __future__ import annotations

from datetime import UTC, datetime
from uuid import UUID, uuid4

import pytest

from acolyte.domain.report import ChangeItem, Report, ReportVersion
from acolyte.gateway.postgres_report_gw import StaleVersionError


class FakeVersionableRepo:
    """Fake that simulates optimistic locking for version bump."""

    def __init__(self) -> None:
        self.reports: dict[UUID, Report] = {}
        self.versions: list[ReportVersion] = []
        self.change_items_store: list[tuple[UUID, int, ChangeItem]] = []

    async def create_report(self, title: str, report_type: str) -> Report:
        report = Report(
            report_id=uuid4(),
            title=title,
            report_type=report_type,
            current_version=0,
            latest_successful_run_id=None,
            created_at=datetime.now(UTC),
        )
        self.reports[report.report_id] = report
        return report

    async def bump_version(
        self,
        report_id: UUID,
        expected_version: int,
        change_reason: str,
        change_items: list[ChangeItem],
        **kwargs: object,
    ) -> int:
        report = self.reports.get(report_id)
        if report is None:
            raise ValueError(f"Report {report_id} not found")
        if report.current_version != expected_version:
            raise StaleVersionError(report_id, expected_version)

        new_version = expected_version + 1
        # Update mutable report (simulate SQL UPDATE)
        self.reports[report_id] = Report(
            report_id=report.report_id,
            title=report.title,
            report_type=report.report_type,
            current_version=new_version,
            latest_successful_run_id=report.latest_successful_run_id,
            created_at=report.created_at,
        )
        # Append immutable version record
        self.versions.append(
            ReportVersion(
                report_id=report_id,
                version_no=new_version,
                change_seq=len(self.versions) + 1,
                change_reason=change_reason,
                created_at=datetime.now(UTC),
            )
        )
        for item in change_items:
            self.change_items_store.append((report_id, new_version, item))
        return new_version


@pytest.mark.asyncio
async def test_bump_version_increments_correctly() -> None:
    repo = FakeVersionableRepo()
    report = await repo.create_report("Test", "weekly_briefing")

    new_v = await repo.bump_version(
        report.report_id,
        0,
        "Initial generation",
        [ChangeItem(field_name="scope", change_kind="added")],
    )

    assert new_v == 1
    assert repo.reports[report.report_id].current_version == 1
    assert len(repo.versions) == 1
    assert repo.versions[0].change_reason == "Initial generation"


@pytest.mark.asyncio
async def test_bump_version_records_change_items() -> None:
    repo = FakeVersionableRepo()
    report = await repo.create_report("Test", "weekly_briefing")

    items = [
        ChangeItem(field_name="scope", change_kind="added"),
        ChangeItem(field_name="title", change_kind="updated", old_fingerprint="abc", new_fingerprint="def"),
    ]
    await repo.bump_version(report.report_id, 0, "Initial", items)

    assert len(repo.change_items_store) == 2
    assert repo.change_items_store[0][2].field_name == "scope"
    assert repo.change_items_store[1][2].change_kind == "updated"


@pytest.mark.asyncio
async def test_stale_version_raises_error() -> None:
    repo = FakeVersionableRepo()
    report = await repo.create_report("Test", "weekly_briefing")

    # First bump succeeds
    await repo.bump_version(report.report_id, 0, "v1", [])

    # Second bump with stale version fails
    with pytest.raises(StaleVersionError):
        await repo.bump_version(report.report_id, 0, "v2 stale", [])


@pytest.mark.asyncio
async def test_sequential_bumps() -> None:
    repo = FakeVersionableRepo()
    report = await repo.create_report("Test", "weekly_briefing")

    v1 = await repo.bump_version(report.report_id, 0, "v1", [ChangeItem(field_name="scope", change_kind="added")])
    v2 = await repo.bump_version(report.report_id, v1, "v2", [ChangeItem(field_name="outline", change_kind="updated")])
    v3 = await repo.bump_version(
        report.report_id,
        v2,
        "v3",
        [ChangeItem(field_name="section:executive_summary", change_kind="regenerated")],
    )

    assert v3 == 3
    assert repo.reports[report.report_id].current_version == 3
    assert len(repo.versions) == 3
