"""Finalizer node — persists the generated report as a new version."""

from __future__ import annotations

from typing import TYPE_CHECKING
from uuid import UUID

import structlog

from acolyte.domain.report import ChangeItem

if TYPE_CHECKING:
    from acolyte.port.report_repository import ReportRepositoryPort
    from acolyte.usecase.graph.state import ReportGenerationState

logger = structlog.get_logger(__name__)


class FinalizerNode:
    def __init__(self, report_repo: ReportRepositoryPort) -> None:
        self._report_repo = report_repo

    async def __call__(self, state: ReportGenerationState) -> dict:
        report_id = UUID(state["report_id"])
        sections = state.get("sections", {})
        outline = state.get("outline", [])
        brief = state.get("brief") or state.get("scope") or {}
        section_citations = state.get("section_citations", {})

        report = await self._report_repo.get_report(report_id)
        if report is None:
            return {"error": f"Report {report_id} not found"}

        change_items = [ChangeItem(field_name=f"section:{key}", change_kind="regenerated") for key in sections]

        new_version = await self._report_repo.bump_version(
            report_id,
            report.current_version,
            "LangGraph pipeline generation",
            change_items,
            scope_snapshot=brief,
            outline_snapshot=outline,
        )

        # Use best_sections if available (paragraph-level writer tracks best revision)
        best = state.get("best_sections")
        if best:
            for key in list(sections.keys()):
                if key in best and best[key] and (not sections.get(key)):
                    sections[key] = best[key]
                    logger.info("Finalizer using best_sections fallback", section_key=key, body_len=len(best[key]))

        # Persist sections
        existing_sections = await self._report_repo.get_sections(report_id)
        existing_keys = {s.section_key for s in existing_sections}

        for i, section_def in enumerate(outline):
            key = section_def.get("key", "")
            body = sections.get(key, "")

            if key not in existing_keys:
                await self._report_repo.create_section(report_id, key, i)
                section = await self._report_repo.get_sections(report_id)
                sec = next((s for s in section if s.section_key == key), None)
                expected_v = sec.current_version if sec else 0
            else:
                sec = next((s for s in existing_sections if s.section_key == key), None)
                expected_v = sec.current_version if sec else 0

            citations = section_citations.get(key)
            await self._report_repo.bump_section_version(report_id, key, expected_v, body, citations=citations)

        logger.info("Finalizer completed", report_id=str(report_id), new_version=new_version)
        return {"final_version_no": new_version}
