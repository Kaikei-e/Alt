"""Connect-RPC AcolyteService implementation."""

from __future__ import annotations

import asyncio
import json
from typing import TYPE_CHECKING
from uuid import UUID

import structlog
from connectrpc.code import Code
from connectrpc.errors import ConnectError

import acolyte.gen  # noqa: F401, I001 — must precede generated imports
from acolyte.gen.proto.alt.acolyte.v1 import acolyte_pb2

if TYPE_CHECKING:
    from collections.abc import AsyncIterator

    from connectrpc.request import RequestContext
    from langgraph.graph.state import CompiledStateGraph

    from acolyte.config.settings import Settings
    from acolyte.port.job_queue import JobQueuePort
    from acolyte.port.llm_provider import LLMProviderPort
    from acolyte.port.report_repository import ReportRepositoryPort

logger = structlog.get_logger(__name__)


class AcolyteConnectService:
    """Connect-RPC implementation of alt.acolyte.v1.AcolyteService."""

    def __init__(
        self,
        settings: Settings,
        report_repo: ReportRepositoryPort,
        job_queue: JobQueuePort | None = None,
        graph: CompiledStateGraph | None = None,
        llm: LLMProviderPort | None = None,
    ) -> None:
        self._settings = settings
        self._repo = report_repo
        self._jobs = job_queue
        self._graph = graph
        self._llm = llm

    def _verify_token(self, ctx: RequestContext) -> None:
        """Verify X-Service-Token header. Skip if secret is empty (dev mode)."""
        secret = self._settings.resolve_service_secret()
        if not secret:
            return
        token = ctx.request_headers().get("x-service-token")
        if not token or token != secret:
            raise ConnectError(Code.UNAUTHENTICATED, "Invalid or missing service token")

    async def create_report(
        self, request: acolyte_pb2.CreateReportRequest, ctx: RequestContext
    ) -> acolyte_pb2.CreateReportResponse:
        from acolyte.domain.brief import ReportBrief

        report = await self._repo.create_report(request.title, request.report_type)
        scope = dict(request.scope) if request.scope else {}
        if scope.get("topic"):
            brief = ReportBrief.from_scope(scope, request.report_type)
            await self._repo.create_brief(report.report_id, brief)
        return acolyte_pb2.CreateReportResponse(report_id=str(report.report_id))

    async def get_report(
        self, request: acolyte_pb2.GetReportRequest, ctx: RequestContext
    ) -> acolyte_pb2.GetReportResponse:
        report = await self._repo.get_report(UUID(request.report_id))
        if report is None:
            raise ConnectError(Code.NOT_FOUND, f"Report {request.report_id} not found")

        sections = await self._repo.get_sections(report.report_id)

        proto_sections = []
        for sec in sections:
            sv = await self._repo.get_section_version(report.report_id, sec.section_key, sec.current_version)
            proto_sections.append(
                acolyte_pb2.ReportSection(
                    section_key=sec.section_key,
                    current_version=sec.current_version,
                    display_order=sec.display_order,
                    body=sv.body if sv else "",
                    citations_json=json.dumps(sv.citations) if sv and sv.citations else "[]",
                )
            )

        return acolyte_pb2.GetReportResponse(
            report=acolyte_pb2.Report(
                report_id=str(report.report_id),
                title=report.title,
                report_type=report.report_type,
                current_version=report.current_version,
                created_at=report.created_at.isoformat(),
            ),
            sections=proto_sections,
        )

    async def list_reports(
        self, request: acolyte_pb2.ListReportsRequest, ctx: RequestContext
    ) -> acolyte_pb2.ListReportsResponse:
        cursor = request.cursor if request.cursor else None
        limit = request.limit if request.limit > 0 else 20
        reports, next_cursor = await self._repo.list_reports(cursor, limit)

        return acolyte_pb2.ListReportsResponse(
            reports=[
                acolyte_pb2.ReportSummary(
                    report_id=str(r.report_id),
                    title=r.title,
                    report_type=r.report_type,
                    current_version=r.current_version,
                    latest_run_status="",
                    created_at=r.created_at.isoformat(),
                )
                for r in reports
            ],
            next_cursor=next_cursor or "",
            has_more=next_cursor is not None,
        )

    async def get_report_version(
        self, request: acolyte_pb2.GetReportVersionRequest, ctx: RequestContext
    ) -> acolyte_pb2.GetReportVersionResponse:
        raise ConnectError(Code.UNIMPLEMENTED, "Not implemented")

    async def list_report_versions(
        self, request: acolyte_pb2.ListReportVersionsRequest, ctx: RequestContext
    ) -> acolyte_pb2.ListReportVersionsResponse:
        report_id = UUID(request.report_id)
        cursor = request.cursor if request.cursor else None
        limit = request.limit if request.limit > 0 else 20
        versions, next_cursor = await self._repo.list_report_versions(report_id, cursor, limit)

        result = []
        for v in versions:
            items = await self._repo.get_change_items(report_id, v.version_no)
            result.append(
                acolyte_pb2.ReportVersionSummary(
                    version_no=v.version_no,
                    change_reason=v.change_reason,
                    created_at=v.created_at.isoformat() if v.created_at else "",
                    change_items=[
                        acolyte_pb2.ChangeItem(
                            field_name=ci.field_name,
                            change_kind=ci.change_kind,
                            old_fingerprint=ci.old_fingerprint or "",
                            new_fingerprint=ci.new_fingerprint or "",
                        )
                        for ci in items
                    ],
                )
            )

        return acolyte_pb2.ListReportVersionsResponse(
            versions=result,
            next_cursor=next_cursor or "",
            has_more=next_cursor is not None,
        )

    async def diff_report_versions(
        self, request: acolyte_pb2.DiffReportVersionsRequest, ctx: RequestContext
    ) -> acolyte_pb2.DiffReportVersionsResponse:
        raise ConnectError(Code.UNIMPLEMENTED, "Not implemented")

    async def start_report_run(
        self, request: acolyte_pb2.StartReportRunRequest, ctx: RequestContext
    ) -> acolyte_pb2.StartReportRunResponse:
        if self._jobs is None:
            raise ConnectError(Code.UNIMPLEMENTED, "Job queue not configured")
        report = await self._repo.get_report(UUID(request.report_id))
        if report is None:
            raise ConnectError(Code.NOT_FOUND, f"Report {request.report_id} not found")
        run = await self._jobs.create_run(report.report_id, report.current_version + 1)

        # Launch pipeline in background if graph is wired
        if self._graph is not None:
            brief = await self._repo.get_brief(report.report_id)
            brief_dict = brief.to_dict() if brief else {"topic": report.title}
            asyncio.create_task(
                self._run_pipeline(str(report.report_id), str(run.run_id), brief_dict),
                name=f"acolyte-run-{run.run_id}",
            )

        return acolyte_pb2.StartReportRunResponse(run_id=str(run.run_id))

    async def _run_pipeline(self, report_id: str, run_id: str, brief_dict: dict) -> None:
        """Execute LangGraph pipeline in background."""
        logger.info("Pipeline started", report_id=report_id, run_id=run_id)
        try:
            result = await self._graph.ainvoke({
                "report_id": report_id,
                "run_id": run_id,
                "brief": brief_dict,
                "revision_count": 0,
            })

            final_version = result.get("final_version_no")
            error = result.get("error")

            if error:
                logger.error("Pipeline failed", report_id=report_id, run_id=run_id, error=error)
            else:
                logger.info("Pipeline completed", report_id=report_id, run_id=run_id, final_version=final_version)

        except Exception as exc:
            import traceback

            tb = traceback.format_exc()
            logger.error("Pipeline crashed", report_id=report_id, run_id=run_id, error=str(exc), traceback=tb)

    async def get_run_status(
        self, request: acolyte_pb2.GetRunStatusRequest, ctx: RequestContext
    ) -> acolyte_pb2.GetRunStatusResponse:
        if self._jobs is None:
            raise ConnectError(Code.UNIMPLEMENTED, "Job queue not configured")
        run = await self._jobs.get_run(UUID(request.run_id))
        if run is None:
            raise ConnectError(Code.NOT_FOUND, f"Run {request.run_id} not found")
        return acolyte_pb2.GetRunStatusResponse(
            run=acolyte_pb2.ReportRun(
                run_id=str(run.run_id),
                report_id=str(run.report_id),
                target_version_no=run.target_version_no,
                run_status=run.run_status,
            ),
        )

    def stream_run_progress(
        self, request: acolyte_pb2.StreamRunProgressRequest, ctx: RequestContext
    ) -> AsyncIterator[acolyte_pb2.StreamRunProgressResponse]:
        raise ConnectError(Code.UNIMPLEMENTED, "Not implemented")

    async def rerun_section(
        self, request: acolyte_pb2.RerunSectionRequest, ctx: RequestContext
    ) -> acolyte_pb2.RerunSectionResponse:
        if self._llm is None:
            raise ConnectError(Code.UNIMPLEMENTED, "LLM provider not configured")
        if not request.section_key:
            raise ConnectError(Code.INVALID_ARGUMENT, "section_key is required")

        from acolyte.usecase.rerun_section_uc import RerunSectionUsecase

        uc = RerunSectionUsecase(self._repo, self._llm)
        try:
            await uc.execute(UUID(request.report_id), request.section_key)
        except ValueError as e:
            raise ConnectError(Code.NOT_FOUND, str(e))

        return acolyte_pb2.RerunSectionResponse(run_id="")

    async def health_check(
        self, request: acolyte_pb2.HealthCheckRequest, ctx: RequestContext
    ) -> acolyte_pb2.HealthCheckResponse:
        return acolyte_pb2.HealthCheckResponse(status="ok")
