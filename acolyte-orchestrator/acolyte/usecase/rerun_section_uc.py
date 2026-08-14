"""RerunSection usecase — regenerate a single report section."""

from __future__ import annotations

from typing import TYPE_CHECKING
from uuid import UUID

import structlog

from acolyte.domain.report import ChangeItem
from acolyte.domain.writer_prompt import WRITER_PROMPT, format_evidence
from acolyte.port.llm_provider import LLMMode

if TYPE_CHECKING:
    from acolyte.domain.report import Report, ReportSection
    from acolyte.port.llm_provider import LLMProviderPort
    from acolyte.port.report_repository import ReportRepositoryPort

logger = structlog.get_logger(__name__)


class RerunRejectedError(RuntimeError):
    """Raised when a rerun is refused because a pipeline run owns the report.

    Deliberately not a ValueError: connect_service maps ValueError from this
    usecase to Code.NOT_FOUND, and "a generation run is in progress" is not a
    missing report. A caller that catches this type can map it to
    Code.FAILED_PRECONDITION the way delete_report's active-run guard does.
    """


class RerunSectionUsecase:
    """Regenerate a single section using the existing brief and outline."""

    def __init__(self, repo: ReportRepositoryPort, llm: LLMProviderPort) -> None:
        self._repo = repo
        self._llm = llm

    async def execute(self, report_id: UUID, section_key: str) -> int:
        """Rerun a single section. Returns new report version number."""
        report, target = await self._load_locked_rows(report_id, section_key)

        # A pipeline run rewrites every section and bumps the report version
        # on finalize, so a rerun started underneath it contends for the same
        # optimistic locks and can only lose. Refuse before burning a
        # multi-second writer call. Same guard delete_report uses; like it,
        # this is best-effort (the run can start right after the check).
        if await self._repo.has_active_run(report_id):
            msg = f"Report {report_id} rerun rejected: a report generation run is in progress"
            raise RerunRejectedError(msg)

        brief = await self._repo.get_brief(report_id)
        topic = brief.topic if brief else report.title

        # Resolve section title from latest version's outline snapshot
        section_title = section_key
        latest_version = await self._repo.get_report_version(report_id, report.current_version)
        outline_snapshot = latest_version.outline_snapshot if latest_version else None
        if isinstance(outline_snapshot, list):
            for entry in outline_snapshot:
                if entry.get("key") == section_key:
                    section_title = entry.get("title", section_key)
                    break

        # Reconstruct evidence from the section's existing citations so the
        # rerun isn't forced into the evidence-free "no reference" branch.
        evidence_items = await self._evidence_from_citations(report_id, section_key, target.current_version)

        # Generate new section body (writer-only, no evidence re-retrieval)
        prompt = WRITER_PROMPT.format(
            title=section_title,
            topic=topic,
            evidence_block=format_evidence(evidence_items),
            revision_note="",
        )
        response = await self._llm.generate(prompt, num_predict=2000, think=False, mode=LLMMode.LONGFORM)

        # The writer call above takes tens of seconds; the report version read
        # before it is what a concurrent writer has since moved, so re-read it
        # and let the bump below contend for the number the DB holds now.
        #
        # Only the report row. `target` deliberately keeps its pre-LLM version:
        # that is the section this body was written against, and the evidence
        # above came from its citations. Re-reading it too would make the
        # section lock always match, turning a sibling rerun of the same
        # section into last-writer-wins — our body, built from the older
        # section's evidence, would silently replace theirs.
        report, _ = await self._load_locked_rows(report_id, section_key)

        # Report version first. The port exposes no cross-call transaction,
        # so one of the two writes can always be left dangling; a rerun
        # touches exactly one section, so the only question is which side.
        # reports.current_version is the lock every concurrent writer
        # contends for, and losing it here aborts before any body lands.
        # Body-first instead leaves a section body that no report version and
        # no change_item owns — and GetReport reads section bodies off
        # report_sections.current_version, so it renders that orphan as if it
        # were published while the caller retries on the resulting INTERNAL.
        # (FinalizerNode orders the other way for the opposite reason: it
        # writes N bodies in a loop, where a dangling version pointer would
        # claim sections a mid-loop failure never wrote.)
        new_report_v = await self._repo.bump_version(
            report_id,
            report.current_version,
            f"Section rerun: {section_key}",
            [ChangeItem(field_name=f"section:{section_key}", change_kind="regenerated")],
        )

        # The re-read above shrinks the gap between the two bumps from the
        # whole writer call to microseconds, but it cannot close it. The
        # residual failure (version stamped, body not) leaves the rendered
        # report on its previous, attributable body and is retryable.
        await self._repo.bump_section_version(
            report_id,
            section_key,
            target.current_version,
            response.text,
        )

        logger.info(
            "Section rerun completed", report_id=str(report_id), section_key=section_key, new_version=new_report_v
        )
        return new_report_v

    async def _load_locked_rows(self, report_id: UUID, section_key: str) -> tuple[Report, ReportSection]:
        """Read the two rows whose current_version guards the version bumps."""
        report = await self._repo.get_report(report_id)
        if report is None:
            msg = f"Report {report_id} not found"
            raise ValueError(msg)

        sections = await self._repo.get_sections(report_id)
        target = next((s for s in sections if s.section_key == section_key), None)
        if target is None:
            msg = f"Section {section_key} not found in report {report_id}"
            raise ValueError(msg)

        return report, target

    async def _evidence_from_citations(self, report_id: UUID, section_key: str, current_version: int) -> list[dict]:
        """Rebuild evidence entries from the section's persisted citations."""
        section_version = await self._repo.get_section_version(report_id, section_key, current_version)
        if section_version is None or not section_version.citations:
            return []

        evidence_items: list[dict] = []
        seen_source_ids: set[str] = set()
        for citation in section_version.citations:
            source_id = citation.get("source_id")
            if not source_id or source_id in seen_source_ids:
                continue
            seen_source_ids.add(source_id)
            evidence_items.append(
                {
                    "id": source_id,
                    "type": citation.get("source_type", "article"),
                    "title": source_id,
                    "excerpt": citation.get("quote", ""),
                }
            )
        return evidence_items
