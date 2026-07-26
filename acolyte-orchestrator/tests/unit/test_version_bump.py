"""Unit tests for version bump logic.

Exercised against the real ``MemoryReportGateway`` (the production
``ReportRepositoryPort`` implementation used in dev/tests), not a
hand-rolled duplicate. A hand-rolled fake copy of the optimistic-locking
logic would keep passing even if the real gateway's bump_version()
regressed — these tests must fail when the real implementation breaks.
"""

from __future__ import annotations

from uuid import uuid4

import pytest

from acolyte.domain.exceptions import StaleVersionError
from acolyte.domain.report import ChangeItem
from acolyte.gateway.memory_report_gw import MemoryReportGateway


@pytest.mark.asyncio
async def test_bump_version_increments_correctly() -> None:
    repo = MemoryReportGateway()
    report = await repo.create_report("Test", "weekly_briefing")

    new_v = await repo.bump_version(
        report.report_id,
        0,
        "Initial generation",
        [ChangeItem(field_name="scope", change_kind="added")],
    )

    assert new_v == 1
    updated = await repo.get_report(report.report_id)
    assert updated is not None
    assert updated.current_version == 1
    versions, _cursor = await repo.list_report_versions(report.report_id, None, 10)
    assert len(versions) == 1
    assert versions[0].change_reason == "Initial generation"
    assert versions[0].version_no == 1


@pytest.mark.asyncio
async def test_bump_version_records_change_items() -> None:
    repo = MemoryReportGateway()
    report = await repo.create_report("Test", "weekly_briefing")

    items = [
        ChangeItem(field_name="scope", change_kind="added"),
        ChangeItem(field_name="title", change_kind="updated", old_fingerprint="abc", new_fingerprint="def"),
    ]
    await repo.bump_version(report.report_id, 0, "Initial", items)

    recorded = await repo.get_change_items(report.report_id, 1)
    assert len(recorded) == 2
    assert recorded[0].field_name == "scope"
    assert recorded[1].change_kind == "updated"
    assert recorded[1].old_fingerprint == "abc"
    assert recorded[1].new_fingerprint == "def"


@pytest.mark.asyncio
async def test_stale_version_raises_error() -> None:
    repo = MemoryReportGateway()
    report = await repo.create_report("Test", "weekly_briefing")

    # First bump succeeds
    await repo.bump_version(report.report_id, 0, "v1", [])

    # Second bump with stale (already-consumed) expected_version fails.
    with pytest.raises(StaleVersionError, match=str(report.report_id)):
        await repo.bump_version(report.report_id, 0, "v2 stale", [])

    # The rejected bump must not have mutated state (still at v1, not v2).
    unchanged = await repo.get_report(report.report_id)
    assert unchanged is not None
    assert unchanged.current_version == 1
    versions, _cursor = await repo.list_report_versions(report.report_id, None, 10)
    assert len(versions) == 1


@pytest.mark.asyncio
async def test_bump_version_unknown_report_raises_stale_version_error() -> None:
    """A report_id the gateway has never seen must fail closed with
    StaleVersionError, not KeyError/AttributeError from a missing lookup."""
    repo = MemoryReportGateway()
    missing_id = uuid4()

    with pytest.raises(StaleVersionError, match=str(missing_id)):
        await repo.bump_version(missing_id, 0, "v1", [])


@pytest.mark.asyncio
async def test_sequential_bumps() -> None:
    repo = MemoryReportGateway()
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
    updated = await repo.get_report(report.report_id)
    assert updated is not None
    assert updated.current_version == 3
    versions, _cursor = await repo.list_report_versions(report.report_id, None, 10)
    assert len(versions) == 3
