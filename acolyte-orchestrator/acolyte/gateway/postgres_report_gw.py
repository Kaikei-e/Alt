"""PostgreSQL report gateway — ReportRepositoryPort implementation."""

from __future__ import annotations

import json
from typing import TYPE_CHECKING
from uuid import UUID

import structlog

from acolyte.domain.brief import ReportBrief
from acolyte.domain.exceptions import StaleVersionError
from acolyte.domain.report import ChangeItem, Report, ReportSection, ReportVersion, SectionVersion

if TYPE_CHECKING:
    from psycopg_pool import AsyncConnectionPool

logger = structlog.get_logger(__name__)

# Re-export for callers that historically imported from this module.
__all__ = ["PostgresReportGateway", "StaleVersionError"]

# LangGraph writes a checkpoint per super-step (article bodies included) into
# tables AsyncPostgresSaver.setup() creates — they are outside Atlas and carry
# no FK to reports, so nothing but this gateway ever reclaims them. Same three
# tables AsyncPostgresSaver.adelete_thread() clears, in FK-safe order.
_CHECKPOINT_TABLES = ("checkpoint_writes", "checkpoint_blobs", "checkpoints")

# Must stay in step with AcolyteConnectService._thread_id_for_run: the
# checkpoint namespace is derived from run_id and is the only link back.
_THREAD_ID_PREFIX = "acolyte-run:"


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
            assert r is not None
            return Report(
                report_id=r[0],
                title=r[1],
                report_type=r[2],
                current_version=r[3],
                latest_successful_run_id=r[4],
                created_at=r[5],
            )

    async def create_brief(self, report_id: UUID, brief: ReportBrief) -> None:
        async with self._pool.connection() as conn:
            await conn.execute(
                "INSERT INTO report_briefs "
                "(report_id, topic, report_type, time_range, entities, exclude_topics, constraints_jsonb) "
                "VALUES (%s, %s, %s, %s, %s, %s, %s)",
                [
                    report_id,
                    brief.topic,
                    brief.report_type,
                    brief.time_range,
                    brief.entities,
                    brief.exclude_topics,
                    json.dumps(brief.constraints),
                ],
            )

    async def get_brief(self, report_id: UUID) -> ReportBrief | None:
        async with self._pool.connection() as conn:
            cur = await conn.execute(
                "SELECT topic, report_type, time_range, entities, exclude_topics, constraints_jsonb "
                "FROM report_briefs WHERE report_id = %s",
                [report_id],
            )
            r = await cur.fetchone()
            if r is None:
                return None
            return ReportBrief(
                topic=r[0],
                report_type=r[1],
                time_range=r[2],
                entities=r[3] or [],
                exclude_topics=r[4] or [],
                constraints=r[5] or {},
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

    async def bump_version(  # noqa: PLR0913 — implements ReportRepositoryPort's bump_version() signature
        self,
        report_id: UUID,
        expected_version: int,
        change_reason: str,
        change_items: list[ChangeItem],
        *,
        prompt_template_version: str | None = None,
        scope_snapshot: dict | None = None,
        outline_snapshot: list[dict] | dict | None = None,
        summary_snapshot: str | None = None,
    ) -> int:
        """Bump report version with optimistic locking."""
        async with self._pool.connection() as conn, conn.transaction():
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
        async with self._pool.connection() as conn, conn.transaction():
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

    async def has_active_run(self, report_id: UUID) -> bool:
        async with self._pool.connection() as conn:
            cur = await conn.execute(
                "SELECT EXISTS(SELECT 1 FROM report_runs "
                "WHERE report_id = %s AND run_status IN ('pending', 'running'))",
                [report_id],
            )
            r = await cur.fetchone()
            return bool(r and r[0])

    async def delete_report(self, report_id: UUID) -> None:
        async with self._pool.connection() as conn, conn.transaction():
            # Read the runs first: DELETE FROM reports cascades report_runs away,
            # and afterwards nothing can tell which checkpoint threads were this
            # report's. Same transaction so the purge commits with the delete.
            cur = await conn.execute("SELECT run_id FROM report_runs WHERE report_id = %s", [report_id])
            thread_ids = [f"{_THREAD_ID_PREFIX}{r[0]}" for r in await cur.fetchall()]
            if thread_ids:
                await self._purge_checkpoint_threads(conn, thread_ids)
            await conn.execute("DELETE FROM reports WHERE report_id = %s", [report_id])

    async def _purge_checkpoint_threads(self, conn: object, thread_ids: list[str]) -> None:
        """Drop the LangGraph checkpoints of these threads inside the caller's transaction."""
        cur = await conn.execute("SELECT to_regclass('checkpoints') IS NOT NULL")  # type: ignore[attr-defined]
        row = await cur.fetchone()
        if row is None or not row[0]:
            # CHECKPOINT_ENABLED=false never calls setup(), so the tables do not
            # exist and there is nothing to reclaim — but the delete must still
            # go through. Logged rather than swallowed: with checkpointing on,
            # this line means setup() never ran against this database.
            logger.info(
                "checkpoint_purge_skipped",
                reason="langgraph checkpoint tables absent in acolyte-db",
                thread_count=len(thread_ids),
            )
            return

        for table in _CHECKPOINT_TABLES:
            await conn.execute(  # type: ignore[attr-defined]
                f"DELETE FROM {table} WHERE thread_id = ANY(%s)",  # noqa: S608 — table names are the module constant, never input
                [thread_ids],
            )
        logger.info("checkpoint_purged", thread_count=len(thread_ids))

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
