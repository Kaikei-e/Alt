"""Regression & TDD tests for legacy report visibility and BackfillReportOwnersUsecase.

P1 Review Tracking: https://github.com/Kaikei-e/Alt/pull/244#discussion_r4057337646
Pins legacy report visibility regression and specifies the production contract for:
- Explicit single-owner UUID backfill
- Per-report mapping backfill
- Atomicity (no partial update on incomplete mapping)
- Preserving existing report owners
- Idempotence
- Startup gate fail-fast when unmapped legacy rows exist (never guessing NOTIFICATION_USER_ID)
- Environment variable aliases and type safety for mapping values
"""

from __future__ import annotations

import json
from datetime import UTC, datetime
from pathlib import Path
from unittest.mock import AsyncMock, MagicMock
from uuid import UUID, uuid4

import pytest

from acolyte.config.settings import Settings
from acolyte.domain.report import Report
from acolyte.gateway.memory_report_gw import MemoryReportGateway
from acolyte.gateway.postgres_report_gw import PostgresReportGateway
from acolyte.usecase.backfill_report_owners_uc import (
    BackfillReportOwnersUsecase,
    UnmappedLegacyReportsError,
)
from acolyte.usecase.get_report_uc import GetReportUsecase
from acolyte.usecase.list_reports_uc import ListReportsUsecase
from tests.conftest import TEST_USER_ID


def _create_report(repo: MemoryReportGateway, *, user_id: UUID | None = None) -> Report:
    """Helper to insert a report with explicit or None user_id into memory gateway."""
    rid = uuid4()
    report = Report(
        report_id=rid,
        title=f"Report {rid}",
        report_type="weekly_briefing",
        current_version=1,
        latest_successful_run_id=None,
        created_at=datetime.now(UTC),
        user_id=user_id,
    )
    repo._reports[rid] = report
    return report


@pytest.mark.asyncio
async def test_legacy_null_user_id_report_is_hidden_without_backfill() -> None:
    """Valuable denial test: un-backfilled legacy reports (user_id=NULL) are hidden from

    both list_reports and get_report, pinning why backfill is mandatory.
    """
    repo = MemoryReportGateway()
    legacy_report = _create_report(repo, user_id=None)

    reports, _ = await ListReportsUsecase(repo).execute(cursor=None, limit=20, user_id=TEST_USER_ID)
    assert legacy_report not in reports

    rep, _ = await GetReportUsecase(repo).execute(legacy_report.report_id, user_id=TEST_USER_ID)
    assert rep is None


@pytest.mark.asyncio
async def test_backfill_explicit_single_owner_assignment_leaves_existing_owners_unchanged() -> None:
    """Contract: single_owner_id backfills all unowned legacy rows while preserving existing owners."""
    repo = MemoryReportGateway()
    legacy_1 = _create_report(repo, user_id=None)
    legacy_2 = _create_report(repo, user_id=None)
    existing_owner = uuid4()
    already_owned = _create_report(repo, user_id=existing_owner)

    target_owner = uuid4()
    uc = BackfillReportOwnersUsecase(repo)
    updated_count = await uc.execute(single_owner_id=target_owner)

    assert updated_count == 2
    assert repo._reports[legacy_1.report_id].user_id == target_owner
    assert repo._reports[legacy_2.report_id].user_id == target_owner
    assert repo._reports[already_owned.report_id].user_id == existing_owner


@pytest.mark.asyncio
async def test_backfill_per_report_mapping_assignment() -> None:
    """Contract: per-report mapping correctly assigns distinct target owners to distinct reports."""
    repo = MemoryReportGateway()
    rep_a = _create_report(repo, user_id=None)
    rep_b = _create_report(repo, user_id=None)
    owner_a = uuid4()
    owner_b = uuid4()

    mapping = {rep_a.report_id: owner_a, rep_b.report_id: owner_b}
    uc = BackfillReportOwnersUsecase(repo)
    updated_count = await uc.execute(mapping=mapping)

    assert updated_count == 2
    assert repo._reports[rep_a.report_id].user_id == owner_a
    assert repo._reports[rep_b.report_id].user_id == owner_b


@pytest.mark.asyncio
async def test_backfill_atomic_no_partial_update_on_missing_mapping() -> None:
    """Contract: if mapping is incomplete, backfill raises UnmappedLegacyReportsError

    and makes NO changes (atomic rollback / fail-fast).
    """
    repo = MemoryReportGateway()
    rep_a = _create_report(repo, user_id=None)
    rep_b = _create_report(repo, user_id=None)
    owner_a = uuid4()

    # Incomplete mapping: omits rep_b
    uc = BackfillReportOwnersUsecase(repo)
    with pytest.raises(UnmappedLegacyReportsError) as exc_info:
        await uc.execute(mapping={rep_a.report_id: owner_a})

    assert str(rep_b.report_id) in str(exc_info.value)
    # Neither report was modified
    assert repo._reports[rep_a.report_id].user_id is None
    assert repo._reports[rep_b.report_id].user_id is None


@pytest.mark.asyncio
async def test_backfill_idempotence() -> None:
    """Contract: repeating the backfill is idempotent; subsequent runs update 0 rows."""
    repo = MemoryReportGateway()
    legacy = _create_report(repo, user_id=None)
    target_owner = uuid4()

    uc = BackfillReportOwnersUsecase(repo)
    first_run = await uc.execute(single_owner_id=target_owner)
    assert first_run == 1
    assert repo._reports[legacy.report_id].user_id == target_owner

    second_run = await uc.execute(single_owner_id=target_owner)
    assert second_run == 0
    assert repo._reports[legacy.report_id].user_id == target_owner


@pytest.mark.asyncio
async def test_startup_gate_fails_fast_on_unmapped_legacy_rows_without_config() -> None:
    """Contract: when unowned legacy rows exist and neither single_owner_id nor mapping is supplied,

    the startup gate fails fast with UnmappedLegacyReportsError containing bounded diagnostic report IDs.
    """
    repo = MemoryReportGateway()
    legacy = _create_report(repo, user_id=None)

    uc = BackfillReportOwnersUsecase(repo)
    with pytest.raises(UnmappedLegacyReportsError) as exc_info:
        await uc.execute()

    assert str(legacy.report_id) in str(exc_info.value)
    assert repo._reports[legacy.report_id].user_id is None


@pytest.mark.asyncio
async def test_startup_gate_passes_when_no_unowned_reports() -> None:
    """Contract: when all reports already have valid user_id owners, gate passes cleanly returning 0."""
    repo = MemoryReportGateway()
    _create_report(repo, user_id=uuid4())

    uc = BackfillReportOwnersUsecase(repo)
    result = await uc.execute()
    assert result == 0


@pytest.mark.asyncio
async def test_backfill_conflicting_options_raises_value_error() -> None:
    """Contract: providing both single_owner_id and mapping simultaneously is rejected."""
    repo = MemoryReportGateway()
    uc = BackfillReportOwnersUsecase(repo)
    with pytest.raises(ValueError, match="Cannot backfill with both"):
        await uc.execute(single_owner_id=uuid4(), mapping={uuid4(): uuid4()})


def test_settings_resolve_legacy_report_owner_id() -> None:
    """Contract: Settings parses valid UUID or raises fail-fast on malformed UUID."""
    valid_uid = uuid4()
    s = Settings(legacy_report_owner_id=str(valid_uid))
    assert s.resolve_legacy_report_owner_id() == valid_uid

    empty_s = Settings(legacy_report_owner_id="")
    assert empty_s.resolve_legacy_report_owner_id() is None

    invalid_s = Settings(legacy_report_owner_id="not-a-uuid")
    with pytest.raises(RuntimeError, match="not a valid UUID"):
        invalid_s.resolve_legacy_report_owner_id()


def test_settings_resolve_legacy_report_mapping(tmp_path: Path) -> None:
    """Contract: Settings parses JSON mapping file or raises fail-fast on missing/invalid file."""
    rep_id = uuid4()
    owner_id = uuid4()
    p = tmp_path / "mapping.json"
    p.write_text(json.dumps({str(rep_id): str(owner_id)}))

    s = Settings(legacy_report_mapping_file=str(p))
    mapping = s.resolve_legacy_report_mapping()
    assert mapping == {rep_id: owner_id}

    missing_s = Settings(legacy_report_mapping_file=str(tmp_path / "missing.json"))
    with pytest.raises(RuntimeError, match="missing or unreadable"):
        missing_s.resolve_legacy_report_mapping()

    invalid_json_s = Settings(legacy_report_mapping_file=str(p))
    p.write_text("invalid json content")
    with pytest.raises(RuntimeError, match="invalid JSON"):
        invalid_json_s.resolve_legacy_report_mapping()


def test_settings_resolve_legacy_report_mapping_invalid_types(tmp_path: Path) -> None:
    """Contract: Settings validates that mapping is a dict and entries are valid UUID strings."""
    rep_id = uuid4()
    p = tmp_path / "mapping.json"

    # Non-dict top level (JSON list) raises TypeError (TRY004)
    p.write_text(json.dumps(["report-id-1", "user-id-1"]))
    s = Settings(legacy_report_mapping_file=str(p))
    with pytest.raises(TypeError, match="must contain a JSON object"):
        s.resolve_legacy_report_mapping()

    # Null value raises TypeError
    p.write_text(json.dumps({str(rep_id): None}))
    with pytest.raises(TypeError, match="must be string UUIDs"):
        s.resolve_legacy_report_mapping()

    # Integer value raises TypeError
    p.write_text(json.dumps({str(rep_id): 12345}))
    with pytest.raises(TypeError, match="must be string UUIDs"):
        s.resolve_legacy_report_mapping()

    # List value raises TypeError
    p.write_text(json.dumps({str(rep_id): [str(uuid4())]}))
    with pytest.raises(TypeError, match="must be string UUIDs"):
        s.resolve_legacy_report_mapping()

    # Malformed UUID in value raises ValueError
    p.write_text(json.dumps({str(rep_id): "not-a-valid-uuid"}))
    with pytest.raises(ValueError, match="must be valid UUIDs"):
        s.resolve_legacy_report_mapping()

    # Malformed UUID in key raises ValueError
    p.write_text(json.dumps({"not-a-valid-uuid": str(uuid4())}))
    with pytest.raises(ValueError, match="must be valid UUIDs"):
        s.resolve_legacy_report_mapping()


def test_settings_environment_variable_aliases(monkeypatch: pytest.MonkeyPatch, tmp_path: Path) -> None:
    """Contract: Settings loads legacy backfill configuration via environment variable aliases."""
    expected_owner = uuid4()
    p = tmp_path / "env_mapping.json"
    p.write_text(json.dumps({str(uuid4()): str(expected_owner)}))

    # Test uppercase compose alias ACOLYTE_LEGACY_REPORT_OWNER_ID
    monkeypatch.setenv("ACOLYTE_LEGACY_REPORT_OWNER_ID", str(expected_owner))
    monkeypatch.setenv("ACOLYTE_LEGACY_REPORT_MAPPING_FILE", str(p))

    s = Settings()
    assert s.legacy_report_owner_id == str(expected_owner)
    assert s.resolve_legacy_report_owner_id() == expected_owner
    assert s.legacy_report_mapping_file == str(p)
    assert s.resolve_legacy_report_mapping() is not None


@pytest.mark.asyncio
async def test_postgres_gateway_backfill_transaction_lock_and_atomicity() -> None:
    """Contract: PostgresReportGateway queries SELECT ... FOR UPDATE inside a transaction

    and aborts cleanly with UnmappedLegacyReportsError on missing mapping.
    """
    mock_conn = AsyncMock()
    mock_cursor = AsyncMock()
    rep_id = uuid4()
    mock_cursor.fetchall.return_value = [(rep_id,)]
    mock_conn.execute.return_value = mock_cursor

    mock_tx = MagicMock()
    mock_tx.__aenter__ = AsyncMock(return_value=None)
    mock_tx.__aexit__ = AsyncMock(return_value=None)
    mock_conn.transaction = MagicMock(return_value=mock_tx)

    mock_pool = MagicMock()
    mock_pool_conn = AsyncMock()
    mock_pool_conn.__aenter__.return_value = mock_conn
    mock_pool_conn.__aexit__.return_value = None
    mock_pool.connection.return_value = mock_pool_conn

    gw = PostgresReportGateway(mock_pool)

    # 1. Missing mapping raises and prevents update statements (rollback/atomicity)
    with pytest.raises(UnmappedLegacyReportsError):
        await gw.backfill_owners(mapping={})

    mock_conn.execute.assert_called_once_with("SELECT report_id FROM reports WHERE user_id IS NULL FOR UPDATE")
    mock_tx.__aenter__.assert_awaited_once()
    mock_tx.__aexit__.assert_awaited_once()
    # Check that transaction exit received the UnmappedLegacyReportsError for rollback
    assert mock_tx.__aexit__.call_args[0][0] is UnmappedLegacyReportsError

    # 2. Single owner updates within transaction and commits cleanly
    mock_conn.execute.reset_mock()
    mock_tx.__aenter__.reset_mock()
    mock_tx.__aexit__.reset_mock()
    owner_id = uuid4()
    count = await gw.backfill_owners(single_owner_id=owner_id)
    assert count == 1
    assert mock_conn.execute.call_count == 2
    mock_tx.__aenter__.assert_awaited_once()
    mock_tx.__aexit__.assert_awaited_once_with(None, None, None)

    # 3. When no unowned reports exist, returns 0 and executes no updates
    mock_conn.execute.reset_mock()
    mock_tx.__aenter__.reset_mock()
    mock_tx.__aexit__.reset_mock()
    mock_cursor.fetchall.return_value = []
    zero_count = await gw.backfill_owners(single_owner_id=owner_id)
    assert zero_count == 0
    assert mock_conn.execute.call_count == 1
    mock_tx.__aenter__.assert_awaited_once()
    mock_tx.__aexit__.assert_awaited_once_with(None, None, None)
