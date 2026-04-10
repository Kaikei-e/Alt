"""RerunSection usecase — regenerate a single report section."""

from __future__ import annotations

from typing import TYPE_CHECKING
from uuid import UUID

import structlog

from acolyte.domain.report import ChangeItem
from acolyte.usecase.graph.nodes.writer_node import WRITER_PROMPT, _format_evidence

if TYPE_CHECKING:
    from acolyte.port.llm_provider import LLMProviderPort
    from acolyte.port.report_repository import ReportRepositoryPort

logger = structlog.get_logger(__name__)


class RerunSectionUsecase:
    """Regenerate a single section using the existing brief and outline."""

    def __init__(self, repo: ReportRepositoryPort, llm: LLMProviderPort) -> None:
        self._repo = repo
        self._llm = llm

    async def execute(self, report_id: UUID, section_key: str) -> int:
        """Rerun a single section. Returns new report version number."""
        report = await self._repo.get_report(report_id)
        if report is None:
            raise ValueError(f"Report {report_id} not found")

        sections = await self._repo.get_sections(report_id)
        target = next((s for s in sections if s.section_key == section_key), None)
        if target is None:
            raise ValueError(f"Section {section_key} not found in report {report_id}")

        brief = await self._repo.get_brief(report_id)
        topic = brief.topic if brief else report.title

        # Resolve section title from latest version's outline snapshot
        section_title = section_key
        latest_version = await self._repo.get_report_version(report_id, report.current_version)
        if latest_version and latest_version.outline_snapshot:
            for entry in latest_version.outline_snapshot:
                if entry.get("key") == section_key:
                    section_title = entry.get("title", section_key)
                    break

        # Generate new section body (writer-only, no evidence re-retrieval)
        prompt = WRITER_PROMPT.format(
            title=section_title,
            topic=topic,
            evidence_block=_format_evidence([]),
            revision_note="",
        )
        response = await self._llm.generate(prompt, num_predict=2000)

        # Bump section version
        await self._repo.bump_section_version(
            report_id,
            section_key,
            target.current_version,
            response.text,
        )

        # Bump report version with change tracking
        new_report_v = await self._repo.bump_version(
            report_id,
            report.current_version,
            f"Section rerun: {section_key}",
            [ChangeItem(field_name=f"section:{section_key}", change_kind="regenerated")],
        )

        logger.info(
            "Section rerun completed", report_id=str(report_id), section_key=section_key, new_version=new_report_v
        )
        return new_report_v
