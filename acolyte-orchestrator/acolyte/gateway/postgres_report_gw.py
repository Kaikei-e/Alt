"""PostgreSQL report gateway — ReportRepositoryPort implementation."""

from __future__ import annotations

import json
from typing import TYPE_CHECKING
from uuid import UUID

import structlog

from acolyte.domain.report import ChangeItem, Report, ReportSection, ReportVersion, SectionVersion

if TYPE_CHECKING:
    from psycopg_pool import AsyncConnectionPool

logger = structlog.get_logger(__name__)


class StaleVersionError(Exception):
    """Raised when an optimistic lock fails due to version mismatch."""

    def __init__(self, report_id: UUID, expected_version: int) -> None:
        self.report_id = report_id
        self.expected_version = expected_version
        super().__init__(f"Stale version: report {report_id} expected v{expected_version}")


class PostgresReportGateway:
    """Report CRUD and versioning backed by PostgreSQL."""

    def __init__(self, pool: AsyncConnectionPool) -> None:
        self._pool = pool

    async def create_report(self, title: str, report_type: str) -> Report:
        async with self._pool.connection() as conn:
            row = await conn.execute(
                "INSERT INTO reports (title, report_type) VALUES (%s, %s) "
                "RETURNING report_id, title, report_type, current_version, latest_successful_run_id, created_at",
                [title, report_type],
            )
            r = await row.fetchone()
            return Report(
                report_id=r[0],
                title=r[1],
                report_type=r[2],
                current_version=r[3],
                latest_successful_run_id=r[4],
                created_at=r[5],
            )

    async def get_report(self, report_id: UUID) -> Report | None:
        async with self._pool.connection() as conn:
            row = await conn.execute(
                "SELECT report_id, title, report_type, current_version, latest_successful_run_id, created_at "
                "FROM reports WHERE report_id = %s",
                [report_id],
            )
            r = await row.fetchone()
            if r is None:
                return None
            return Report(
                report_id=r[0],
                title=r[1],
                report_type=r[2],
                current_version=r[3],
                latest_successful_run_id=r[4],
                created_at=r[5],
            )

    async def list_reports(self, cursor: str | None, limit: int) -> tuple[list[Report], str | None]:
        async with self._pool.connection() as conn:
            if cursor:
                row = await conn.execute(
                    "SELECT report_id, title, report_type, current_version, latest_successful_run_id, created_at "
                    "FROM reports WHERE created_at < %s ORDER BY created_at DESC LIMIT %s",
                    [cursor, limit + 1],
                )
            else:
                row = await conn.execute(
                    "SELECT report_id, title, report_type, current_version, latest_successful_run_id, created_at "
                    "FROM reports ORDER BY created_at DESC LIMIT %s",
                    [limit + 1],
                )
            rows = await row.fetchall()

        reports = [
            Report(
                report_id=r[0],
                title=r[1],
                report_type=r[2],
                current_version=r[3],
                latest_successful_run_id=r[4],
                created_at=r[5],
            )
            for r in rows[:limit]
        ]
        next_cursor = reports[-1].created_at.isoformat() if len(rows) > limit else None
        return reports, next_cursor

    async def bump_version(
        self,
        report_id: UUID,
        expected_version: int,
        change_reason: str,
        change_items: list[ChangeItem],
        *,
        prompt_template_version: str | None = None,
        scope_snapshot: dict | None = None,
        outline_snapshot: dict | None = None,
        summary_snapshot: str | None = None,
    ) -> int:
        """Bump report version with optimistic locking."""
        async with self._pool.connection() as conn:
            async with conn.transaction():
                cur = await conn.execute(
                    "UPDATE reports SET current_version = current_version + 1 "
                    "WHERE report_id = %s AND current_version = %s "
                    "RETURNING current_version",
                    [report_id, expected_version],
                )
                row = await cur.fetchone()
                if row is None:
                    raise StaleVersionError(report_id, expected_version)
                new_version = row[0]

                await conn.execute(
                    "INSERT INTO report_versions "
                    "(report_id, version_no, change_reason, prompt_template_version, "
                    "scope_snapshot, outline_snapshot, summary_snapshot) "
                    "VALUES (%s, %s, %s, %s, %s, %s, %s)",
                    [
                        report_id,
                        new_version,
                        change_reason,
                        prompt_template_version,
                        json.dumps(scope_snapshot) if scope_snapshot else None,
                        json.dumps(outline_snapshot) if outline_snapshot else None,
                        summary_snapshot,
                    ],
                )

                for item in change_items:
                    await conn.execute(
                        "INSERT INTO report_change_items "
                        "(report_id, version_no, field_name, change_kind, old_fingerprint, new_fingerprint) "
                        "VALUES (%s, %s, %s, %s, %s, %s)",
                        [
                            report_id,
                            new_version,
                            item.field_name,
                            item.change_kind,
                            item.old_fingerprint,
                            item.new_fingerprint,
                        ],
                    )

                return new_version

    async def get_report_version(self, report_id: UUID, version_no: int) -> ReportVersion | None:
        async with self._pool.connection() as conn:
            cur = await conn.execute(
                "SELECT report_id, version_no, change_seq, change_reason, created_at, "
                "prompt_template_version, scope_snapshot, outline_snapshot, summary_snapshot "
                "FROM report_versions WHERE report_id = %s AND version_no = %s",
                [report_id, version_no],
            )
            r = await cur.fetchone()
            if r is None:
                return None
            return ReportVersion(
                report_id=r[0],
                version_no=r[1],
                change_seq=r[2],
                change_reason=r[3],
                created_at=r[4],
                prompt_template_version=r[5],
                scope_snapshot=r[6],
                outline_snapshot=r[7],
                summary_snapshot=r[8],
            )

    async def list_report_versions(
        self, report_id: UUID, cursor: str | None, limit: int
    ) -> tuple[list[ReportVersion], str | None]:
        async with self._pool.connection() as conn:
            if cursor:
                cur = await conn.execute(
                    "SELECT report_id, version_no, change_seq, change_reason, created_at, "
                    "prompt_template_version, scope_snapshot, outline_snapshot, summary_snapshot "
                    "FROM report_versions WHERE report_id = %s AND version_no < %s "
                    "ORDER BY version_no DESC LIMIT %s",
                    [report_id, int(cursor), limit + 1],
                )
            else:
                cur = await conn.execute(
                    "SELECT report_id, version_no, change_seq, change_reason, created_at, "
                    "prompt_template_version, scope_snapshot, outline_snapshot, summary_snapshot "
                    "FROM report_versions WHERE report_id = %s ORDER BY version_no DESC LIMIT %s",
                    [report_id, limit + 1],
                )
            rows = await cur.fetchall()

        versions = [
            ReportVersion(
                report_id=r[0],
                version_no=r[1],
                change_seq=r[2],
                change_reason=r[3],
                created_at=r[4],
                prompt_template_version=r[5],
                scope_snapshot=r[6],
                outline_snapshot=r[7],
                summary_snapshot=r[8],
            )
            for r in rows[:limit]
        ]
        next_cursor = str(versions[-1].version_no) if len(rows) > limit else None
        return versions, next_cursor

    async def get_change_items(self, report_id: UUID, version_no: int) -> list[ChangeItem]:
        async with self._pool.connection() as conn:
            cur = await conn.execute(
                "SELECT field_name, change_kind, old_fingerprint, new_fingerprint "
                "FROM report_change_items WHERE report_id = %s AND version_no = %s",
                [report_id, version_no],
            )
            rows = await cur.fetchall()
        return [
            ChangeItem(
                field_name=r[0],
                change_kind=r[1],
                old_fingerprint=r[2],
                new_fingerprint=r[3],
            )
            for r in rows
        ]

    async def create_section(self, report_id: UUID, section_key: str, display_order: int) -> ReportSection:
        async with self._pool.connection() as conn:
            await conn.execute(
                "INSERT INTO report_sections (report_id, section_key, display_order) VALUES (%s, %s, %s)",
                [report_id, section_key, display_order],
            )
        return ReportSection(
            report_id=report_id,
            section_key=section_key,
            current_version=0,
            display_order=display_order,
        )

    async def get_sections(self, report_id: UUID) -> list[ReportSection]:
        async with self._pool.connection() as conn:
            cur = await conn.execute(
                "SELECT report_id, section_key, current_version, display_order "
                "FROM report_sections WHERE report_id = %s ORDER BY display_order",
                [report_id],
            )
            rows = await cur.fetchall()
        return [ReportSection(report_id=r[0], section_key=r[1], current_version=r[2], display_order=r[3]) for r in rows]

    async def bump_section_version(
        self,
        report_id: UUID,
        section_key: str,
        expected_version: int,
        body: str,
        citations: list[dict] | None = None,
    ) -> int:
        async with self._pool.connection() as conn:
            async with conn.transaction():
                cur = await conn.execute(
                    "UPDATE report_sections SET current_version = current_version + 1 "
                    "WHERE report_id = %s AND section_key = %s AND current_version = %s "
                    "RETURNING current_version",
                    [report_id, section_key, expected_version],
                )
                row = await cur.fetchone()
                if row is None:
                    raise StaleVersionError(report_id, expected_version)
                new_version = row[0]

                await conn.execute(
                    "INSERT INTO report_section_versions "
                    "(report_id, section_key, version_no, body, citations_jsonb) "
                    "VALUES (%s, %s, %s, %s, %s)",
                    [report_id, section_key, new_version, body, json.dumps(citations or [])],
                )

                return new_version

    async def get_section_version(self, report_id: UUID, section_key: str, version_no: int) -> SectionVersion | None:
        async with self._pool.connection() as conn:
            cur = await conn.execute(
                "SELECT report_id, section_key, version_no, body, citations_jsonb, created_at "
                "FROM report_section_versions "
                "WHERE report_id = %s AND section_key = %s AND version_no = %s",
                [report_id, section_key, version_no],
            )
            r = await cur.fetchone()
            if r is None:
                return None
            return SectionVersion(
                report_id=r[0],
                section_key=r[1],
                version_no=r[2],
                body=r[3],
                citations=r[4] if r[4] else [],
                created_at=r[5],
            )
