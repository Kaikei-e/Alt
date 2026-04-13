"""Report repository port — interface for report CRUD and versioning."""

from __future__ import annotations

from typing import TYPE_CHECKING, Protocol
from uuid import UUID

from acolyte.domain.report import ChangeItem, Report, ReportSection, ReportVersion, SectionVersion

if TYPE_CHECKING:
    from acolyte.domain.brief import ReportBrief


class ReportRepositoryPort(Protocol):
    async def create_report(self, title: str, report_type: str) -> Report: ...

    async def create_brief(self, report_id: UUID, brief: ReportBrief) -> None: ...

    async def get_brief(self, report_id: UUID) -> ReportBrief | None: ...

    async def get_report(self, report_id: UUID) -> Report | None: ...

    async def list_reports(self, cursor: str | None, limit: int) -> tuple[list[Report], str | None]: ...

    async def bump_version(
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
    ) -> int: ...

    async def get_report_version(self, report_id: UUID, version_no: int) -> ReportVersion | None: ...

    async def list_report_versions(
        self, report_id: UUID, cursor: str | None, limit: int
    ) -> tuple[list[ReportVersion], str | None]: ...

    async def get_change_items(self, report_id: UUID, version_no: int) -> list[ChangeItem]: ...

    async def create_section(self, report_id: UUID, section_key: str, display_order: int) -> ReportSection: ...

    async def get_sections(self, report_id: UUID) -> list[ReportSection]: ...

    async def bump_section_version(
        self,
        report_id: UUID,
        section_key: str,
        expected_version: int,
        body: str,
        citations: list[dict] | None = None,
    ) -> int: ...

    async def get_section_version(
        self, report_id: UUID, section_key: str, version_no: int
    ) -> SectionVersion | None: ...

    async def has_active_run(self, report_id: UUID) -> bool: ...

    async def delete_report(self, report_id: UUID) -> None: ...
