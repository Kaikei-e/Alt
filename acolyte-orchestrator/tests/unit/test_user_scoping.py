"""Unit tests for per-user scoping and report ownership isolation."""

from __future__ import annotations

from datetime import UTC, datetime
from unittest.mock import AsyncMock, MagicMock
from uuid import UUID, uuid4

import pytest
from connectrpc.code import Code
from connectrpc.errors import ConnectError

from acolyte.config.settings import Settings
from acolyte.domain.report import Report
from acolyte.gateway.memory_job_gw import MemoryJobGateway
from acolyte.gateway.memory_report_gw import MemoryReportGateway
from acolyte.gen.proto.alt.acolyte.v1 import acolyte_pb2
from acolyte.handler.connect_service import AcolyteConnectService
from acolyte.usecase.create_report_uc import CreateReportUsecase
from acolyte.usecase.get_report_uc import GetReportUsecase
from acolyte.usecase.list_reports_uc import ListReportsUsecase
from acolyte.usecase.rerun_section_uc import RerunSectionUsecase
from acolyte.usecase.start_run_uc import StartRunUsecase
from tests.conftest import make_request_ctx


@pytest.mark.asyncio
async def test_create_report_stores_user_id() -> None:
    repo = MemoryReportGateway()
    user_id = uuid4()
    uc = CreateReportUsecase(repo)
    report = await uc.execute("My Report", "weekly_briefing", user_id=user_id)

    assert report.user_id == user_id
    stored = await repo.get_report(report.report_id)
    assert stored is not None
    assert stored.user_id == user_id


@pytest.mark.asyncio
async def test_get_report_scoped_to_owner() -> None:
    repo = MemoryReportGateway()
    owner_id = uuid4()
    other_id = uuid4()

    report = await repo.create_report("Owner's Report", "weekly_briefing", user_id=owner_id)
    uc = GetReportUsecase(repo)

    # Owner sees report
    res_report, _ = await uc.execute(report.report_id, user_id=owner_id)
    assert res_report is not None
    assert res_report.report_id == report.report_id

    # Other user gets None (NotFound)
    other_report, _ = await uc.execute(report.report_id, user_id=other_id)
    assert other_report is None


@pytest.mark.asyncio
async def test_get_report_null_owner_returns_not_found() -> None:
    repo = MemoryReportGateway()
    calling_user = uuid4()

    # Legacy report with NULL user_id in database
    legacy_id = uuid4()
    legacy_report = Report(
        report_id=legacy_id,
        title="Legacy Report",
        report_type="weekly_briefing",
        current_version=0,
        latest_successful_run_id=None,
        created_at=datetime.now(UTC),
        user_id=None,
    )
    repo._reports[legacy_id] = legacy_report
    uc = GetReportUsecase(repo)

    res_report, _ = await uc.execute(legacy_id, user_id=calling_user)
    assert res_report is None


@pytest.mark.asyncio
async def test_list_reports_scoped_to_owner() -> None:
    repo = MemoryReportGateway()
    user_a = uuid4()
    user_b = uuid4()

    rep_a = await repo.create_report("A's Report", "weekly_briefing", user_id=user_a)
    rep_b = await repo.create_report("B's Report", "weekly_briefing", user_id=user_b)
    legacy_id = uuid4()
    rep_null = Report(
        report_id=legacy_id,
        title="Legacy Report",
        report_type="weekly_briefing",
        current_version=0,
        latest_successful_run_id=None,
        created_at=datetime.now(UTC),
        user_id=None,
    )
    repo._reports[legacy_id] = rep_null

    uc = ListReportsUsecase(repo)

    # User A sees only A's reports
    reports_a, _ = await uc.execute(cursor=None, limit=10, user_id=user_a)
    ids_a = [r.report_id for r in reports_a]
    assert rep_a.report_id in ids_a
    assert rep_b.report_id not in ids_a
    assert rep_null.report_id not in ids_a

    # User B sees only B's reports
    reports_b, _ = await uc.execute(cursor=None, limit=10, user_id=user_b)
    ids_b = [r.report_id for r in reports_b]
    assert rep_b.report_id in ids_b
    assert rep_a.report_id not in ids_b
    assert rep_null.report_id not in ids_b


@pytest.mark.asyncio
async def test_start_run_scoped_to_owner() -> None:
    repo = MemoryReportGateway()
    jobs = MemoryJobGateway()
    owner_id = uuid4()
    other_id = uuid4()

    report = await repo.create_report("Owner's Report", "weekly_briefing", user_id=owner_id)
    uc = StartRunUsecase(repo, jobs)

    # Owner can start run
    run = await uc.execute(report.report_id, user_id=owner_id)
    assert run.report_id == report.report_id

    # Other user gets ValueError (which maps to NotFound in handler)
    with pytest.raises(ValueError):
        await uc.execute(report.report_id, user_id=other_id)


@pytest.mark.asyncio
async def test_rerun_section_scoped_to_owner() -> None:
    repo = MemoryReportGateway()
    llm = MagicMock()
    owner_id = uuid4()
    other_id = uuid4()

    report = await repo.create_report("Owner's Report", "weekly_briefing", user_id=owner_id)
    await repo.create_section(report.report_id, "sec1", 0)
    uc = RerunSectionUsecase(repo, llm)

    # Other user gets ValueError (which maps to NotFound in handler)
    with pytest.raises(ValueError):
        await uc.execute(report.report_id, "sec1", user_id=other_id)


@pytest.mark.asyncio
async def test_delete_report_scoped_to_owner() -> None:
    repo = MemoryReportGateway()
    jobs = MemoryJobGateway()
    owner_id = uuid4()
    other_id = uuid4()

    report = await repo.create_report("Owner's Report", "weekly_briefing", user_id=owner_id)
    service = AcolyteConnectService(Settings(), repo, jobs)

    # Other user cannot delete foreign report -> NotFound
    ctx_other = make_request_ctx("DeleteReport", user_id=other_id)
    with pytest.raises(ConnectError) as exc_info:
        await service.delete_report(
            acolyte_pb2.DeleteReportRequest(report_id=str(report.report_id)),
            ctx=ctx_other,
        )
    assert exc_info.value.code == Code.NOT_FOUND

    # Foreign report is NOT deleted
    assert await repo.get_report(report.report_id) is not None

    # Owner can delete their report
    ctx_owner = make_request_ctx("DeleteReport", user_id=owner_id)
    resp = await service.delete_report(
        acolyte_pb2.DeleteReportRequest(report_id=str(report.report_id)),
        ctx=ctx_owner,
    )
    assert resp is not None
    assert await repo.get_report(report.report_id) is None


@pytest.mark.asyncio
async def test_get_run_status_scoped_to_owner() -> None:
    repo = MemoryReportGateway()
    jobs = MemoryJobGateway()
    owner_id = uuid4()
    other_id = uuid4()

    report = await repo.create_report("Owner's Report", "weekly_briefing", user_id=owner_id)
    run = await jobs.create_run(report.report_id, 1)

    service = AcolyteConnectService(Settings(), repo, jobs)

    # Owner sees run status
    ctx_owner = make_request_ctx("GetRunStatus", user_id=owner_id)
    res = await service.get_run_status(
        acolyte_pb2.GetRunStatusRequest(run_id=str(run.run_id)),
        ctx=ctx_owner,
    )
    assert res.run.run_id == str(run.run_id)

    # Other user gets NotFound
    ctx_other = make_request_ctx("GetRunStatus", user_id=other_id)
    with pytest.raises(ConnectError) as exc_info:
        await service.get_run_status(
            acolyte_pb2.GetRunStatusRequest(run_id=str(run.run_id)),
            ctx=ctx_other,
        )
    assert exc_info.value.code == Code.NOT_FOUND


@pytest.mark.asyncio
async def test_list_report_versions_scoped_to_owner() -> None:
    repo = MemoryReportGateway()
    jobs = MemoryJobGateway()
    owner_id = uuid4()
    other_id = uuid4()

    report = await repo.create_report("Owner's Report", "weekly_briefing", user_id=owner_id)
    service = AcolyteConnectService(Settings(), repo, jobs)

    # Owner can list versions
    ctx_owner = make_request_ctx("ListReportVersions", user_id=owner_id)
    res = await service.list_report_versions(
        acolyte_pb2.ListReportVersionsRequest(report_id=str(report.report_id)),
        ctx=ctx_owner,
    )
    assert res is not None

    # Other user gets NotFound
    ctx_other = make_request_ctx("ListReportVersions", user_id=other_id)
    with pytest.raises(ConnectError) as exc_info:
        await service.list_report_versions(
            acolyte_pb2.ListReportVersionsRequest(report_id=str(report.report_id)),
            ctx=ctx_other,
        )
    assert exc_info.value.code == Code.NOT_FOUND


@pytest.mark.asyncio
async def test_get_report_version_unimplemented() -> None:
    repo = MemoryReportGateway()
    jobs = MemoryJobGateway()
    owner_id = uuid4()
    service = AcolyteConnectService(Settings(), repo, jobs)
    ctx = make_request_ctx("GetReportVersion", user_id=owner_id)
    with pytest.raises(ConnectError) as exc_info:
        await service.get_report_version(
            acolyte_pb2.GetReportVersionRequest(report_id=str(uuid4()), version_no=1),
            ctx=ctx,
        )
    assert exc_info.value.code == Code.UNIMPLEMENTED


@pytest.mark.asyncio
async def test_diff_report_versions_unimplemented() -> None:
    repo = MemoryReportGateway()
    jobs = MemoryJobGateway()
    owner_id = uuid4()
    service = AcolyteConnectService(Settings(), repo, jobs)
    ctx = make_request_ctx("DiffReportVersions", user_id=owner_id)
    with pytest.raises(ConnectError) as exc_info:
        await service.diff_report_versions(
            acolyte_pb2.DiffReportVersionsRequest(report_id=str(uuid4()), from_version=1, to_version=2),
            ctx=ctx,
        )
    assert exc_info.value.code == Code.UNIMPLEMENTED


@pytest.mark.asyncio
async def test_stream_run_progress_scoped_to_owner() -> None:
    repo = MemoryReportGateway()
    jobs = MemoryJobGateway()
    owner_id = uuid4()
    other_id = uuid4()

    report = await repo.create_report("Owner's Report", "weekly_briefing", user_id=owner_id)
    run = await jobs.create_run(report.report_id, 1)
    service = AcolyteConnectService(Settings(), repo, jobs)

    ctx_other = make_request_ctx("StreamRunProgress", user_id=other_id)
    with pytest.raises(ConnectError) as exc_info:
        agen = service.stream_run_progress(
            acolyte_pb2.StreamRunProgressRequest(run_id=str(run.run_id)),
            ctx=ctx_other,
        )
        await anext(agen)
    assert exc_info.value.code == Code.NOT_FOUND


@pytest.mark.asyncio
async def test_unauthenticated_requests_rejected() -> None:
    repo = MemoryReportGateway()
    jobs = MemoryJobGateway()
    service = AcolyteConnectService(Settings(), repo, jobs)
    ctx_unauth = make_request_ctx("ListReports", user_id=None)

    with pytest.raises(ConnectError) as exc_info:
        await service.list_reports(acolyte_pb2.ListReportsRequest(), ctx=ctx_unauth)
    assert exc_info.value.code == Code.UNAUTHENTICATED

    with pytest.raises(ConnectError) as exc_info:
        await service.create_report(
            acolyte_pb2.CreateReportRequest(title="Test", report_type="weekly_briefing"),
            ctx=ctx_unauth,
        )
    assert exc_info.value.code == Code.UNAUTHENTICATED


@pytest.mark.asyncio
async def test_resume_pipeline_derives_owner_from_report() -> None:
    repo = MemoryReportGateway()
    jobs = MemoryJobGateway()
    owner_id = uuid4()
    report = await repo.create_report("Owned Report", "weekly_briefing", user_id=owner_id)
    run = await jobs.create_run(report.report_id, 1)

    graph = MagicMock()
    graph.ainvoke = AsyncMock(return_value={"final_version_no": 1})
    service = AcolyteConnectService(Settings(checkpoint_enabled=False), repo, jobs, graph=graph)

    await service.resume_pipeline(str(report.report_id), str(run.run_id), {"topic": "AI"})
    graph.ainvoke.assert_awaited_once()
    invoked_state = graph.ainvoke.call_args[0][0]
    assert invoked_state["user_id"] == owner_id


@pytest.mark.asyncio
async def test_resume_pipeline_refuses_null_owner_report() -> None:
    repo = MemoryReportGateway()
    jobs = MemoryJobGateway()
    legacy_id = uuid4()
    legacy_report = Report(
        report_id=legacy_id,
        title="Unowned Legacy",
        report_type="weekly_briefing",
        current_version=0,
        latest_successful_run_id=None,
        created_at=datetime.now(UTC),
        user_id=None,
    )
    repo._reports[legacy_id] = legacy_report
    run = await jobs.create_run(legacy_id, 1)

    service = AcolyteConnectService(Settings(), repo, jobs)
    with pytest.raises(ConnectError) as exc_info:
        await service.resume_pipeline(str(legacy_id), str(run.run_id), {"topic": "AI"})
    assert exc_info.value.code == Code.FAILED_PRECONDITION
    assert "backfill" in exc_info.value.message.lower()


def test_disabled_mode_dev_user_resolution() -> None:
    # Valid UUID
    settings = Settings(
        backend_token_verification="disabled",
        user_identity_dev_user_id="00000000-0000-0000-0000-000000000001",
    )
    dev_uid = settings.resolve_dev_user_id()
    assert dev_uid == UUID("00000000-0000-0000-0000-000000000001")

    # Empty UUID when disabled raises RuntimeError
    settings_empty = Settings(
        backend_token_verification="disabled",
        user_identity_dev_user_id="",
    )
    with pytest.raises(RuntimeError, match="USER_IDENTITY_DEV_USER_ID must be set"):
        settings_empty.resolve_dev_user_id()
