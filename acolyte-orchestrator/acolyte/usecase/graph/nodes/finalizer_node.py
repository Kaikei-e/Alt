"""Finalizer node — persists the generated report as a new version."""

from __future__ import annotations

import re
from typing import TYPE_CHECKING
from uuid import UUID

import structlog

from acolyte.domain.report import ChangeItem
from acolyte.domain.source_map import SourceMap

if TYPE_CHECKING:
    from acolyte.port.report_repository import ReportRepositoryPort
    from acolyte.usecase.graph.state import ReportGenerationState

logger = structlog.get_logger(__name__)

_SHORT_ID_RE = re.compile(r"\[S(\d+)\]")
_SOURCES_FOOTER_HEADER = "\n\n---\nSources:\n"


def _extract_referenced_short_ids(body: str) -> list[str]:
    """Return short IDs ([S1], [S2], ...) in first-occurrence order."""
    seen: set[str] = set()
    ordered: list[str] = []
    for match in _SHORT_ID_RE.finditer(body):
        short_id = f"S{match.group(1)}"
        if short_id not in seen:
            seen.add(short_id)
            ordered.append(short_id)
    return ordered


def _format_source_entry(short_id: str, entry) -> str:
    """Render one footer line: '- [S1] Title — Publisher (URL)'."""
    line = f"- [{short_id}] {entry.title}"
    publisher = (entry.publisher or "").strip()
    url = (entry.url or "").strip()
    if publisher and url:
        line += f" — {publisher} ({url})"
    elif publisher:
        line += f" — {publisher}"
    elif url:
        line += f" ({url})"
    return line


def render_sources_footer(body: str, source_map: SourceMap) -> str:
    """Append a Sources footer listing only short IDs referenced in body.

    [Sn] markers in the prose are preserved verbatim. Each referenced source is
    emitted once, in first-occurrence order. Unknown short IDs are skipped in
    the footer but kept inline as-is.
    """
    referenced = _extract_referenced_short_ids(body)
    if not referenced:
        return body

    footer_lines: list[str] = []
    for short_id in referenced:
        entry = source_map.resolve(short_id)
        if entry is None:
            continue
        footer_lines.append(_format_source_entry(short_id, entry))

    if not footer_lines:
        return body

    return body + _SOURCES_FOOTER_HEADER + "\n".join(footer_lines)


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

        # Persist the best revision body when available, not only when latest is empty.
        best = state.get("best_sections")
        if best:
            for key in list(sections.keys()):
                if key in best and best[key]:
                    sections[key] = best[key]
                    logger.info("Finalizer using best_sections", section_key=key, body_len=len(best[key]))

        # Append a Sources footer listing referenced [Sn] entries.
        source_map_data = state.get("source_map")
        if source_map_data:
            sm = SourceMap.from_dict(source_map_data)
            for key in list(sections.keys()):
                sections[key] = render_sources_footer(sections[key], sm)

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
