"""In-memory report gateway — for testing and initial development."""

from __future__ import annotations

from datetime import UTC, datetime
from uuid import UUID, uuid4

from acolyte.domain.report import ChangeItem, Report, ReportSection, ReportVersion, SectionVersion
from acolyte.gateway.postgres_report_gw import StaleVersionError


class MemoryReportGateway:
    """In-memory ReportRepositoryPort implementation."""

    def __init__(self) -> None:
        self._reports: dict[UUID, Report] = {}
        self._versions: dict[UUID, list[ReportVersion]] = {}
        self._change_items: dict[tuple[UUID, int], list[ChangeItem]] = {}
        self._sections: dict[UUID, list[ReportSection]] = {}
        self._section_versions: dict[tuple[UUID, str, int], SectionVersion] = {}
        self._change_seq = 0

    async def create_report(self, title: str, report_type: str) -> Report:
        report = Report(
            report_id=uuid4(),
            title=title,
            report_type=report_type,
            current_version=0,
            latest_successful_run_id=None,
            created_at=datetime.now(UTC),
        )
        self._reports[report.report_id] = report
        self._versions[report.report_id] = []
        self._sections[report.report_id] = []
        return report

    async def get_report(self, report_id: UUID) -> Report | None:
        return self._reports.get(report_id)

    async def list_reports(self, cursor: str | None, limit: int) -> tuple[list[Report], str | None]:
        all_reports = sorted(self._reports.values(), key=lambda r: r.created_at, reverse=True)
        return all_reports[:limit], None

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
        report = self._reports.get(report_id)
        if report is None or report.current_version != expected_version:
            raise StaleVersionError(report_id, expected_version)

        new_version = expected_version + 1
        self._change_seq += 1

        self._reports[report_id] = Report(
            report_id=report.report_id,
            title=report.title,
            report_type=report.report_type,
            current_version=new_version,
            latest_successful_run_id=report.latest_successful_run_id,
            created_at=report.created_at,
        )
        self._versions[report_id].append(
            ReportVersion(
                report_id=report_id,
                version_no=new_version,
                change_seq=self._change_seq,
                change_reason=change_reason,
                created_at=datetime.now(UTC),
                prompt_template_version=prompt_template_version,
                scope_snapshot=scope_snapshot,
                outline_snapshot=outline_snapshot,
                summary_snapshot=summary_snapshot,
            )
        )
        self._change_items[(report_id, new_version)] = list(change_items)
        return new_version

    async def get_report_version(self, report_id: UUID, version_no: int) -> ReportVersion | None:
        for v in self._versions.get(report_id, []):
            if v.version_no == version_no:
                return v
        return None

    async def list_report_versions(
        self, report_id: UUID, cursor: str | None, limit: int
    ) -> tuple[list[ReportVersion], str | None]:
        versions = sorted(self._versions.get(report_id, []), key=lambda v: v.version_no, reverse=True)
        return versions[:limit], None

    async def get_change_items(self, report_id: UUID, version_no: int) -> list[ChangeItem]:
        return self._change_items.get((report_id, version_no), [])

    async def create_section(self, report_id: UUID, section_key: str, display_order: int) -> ReportSection:
        sec = ReportSection(report_id=report_id, section_key=section_key, current_version=0, display_order=display_order)
        self._sections.setdefault(report_id, []).append(sec)
        return sec

    async def get_sections(self, report_id: UUID) -> list[ReportSection]:
        return sorted(self._sections.get(report_id, []), key=lambda s: s.display_order)

    async def bump_section_version(
        self, report_id: UUID, section_key: str, expected_version: int, body: str, citations: list[dict] | None = None
    ) -> int:
        new_v = expected_version + 1
        sections = self._sections.get(report_id, [])
        for i, s in enumerate(sections):
            if s.section_key == section_key:
                sections[i] = ReportSection(
                    report_id=report_id, section_key=section_key, current_version=new_v, display_order=s.display_order
                )
                break
        self._section_versions[(report_id, section_key, new_v)] = SectionVersion(
            report_id=report_id, section_key=section_key, version_no=new_v, body=body,
            citations=citations or [], created_at=datetime.now(UTC),
        )
        return new_v

    async def get_section_version(self, report_id: UUID, section_key: str, version_no: int) -> SectionVersion | None:
        return self._section_versions.get((report_id, section_key, version_no))
