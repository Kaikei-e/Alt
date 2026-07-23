"""Connect-RPC AcolyteService implementation."""

from __future__ import annotations

import asyncio
import json
from typing import TYPE_CHECKING, Any
from uuid import UUID

import structlog
from connectrpc.code import Code
from connectrpc.errors import ConnectError

import acolyte.gen  # noqa: F401 — must precede generated imports
from acolyte.domain.brief import ReportBrief
from acolyte.gen.proto.alt.acolyte.v1 import acolyte_pb2
from acolyte.usecase.create_report_uc import CreateReportUsecase
from acolyte.usecase.get_report_uc import GetReportUsecase
from acolyte.usecase.list_reports_uc import ListReportsUsecase
from acolyte.usecase.rerun_section_uc import RerunSectionUsecase
from acolyte.usecase.start_run_uc import StartRunRejectedError, StartRunUsecase

if TYPE_CHECKING:
    from collections.abc import AsyncIterator

    from connectrpc.request import RequestContext
    from langgraph.graph.state import CompiledStateGraph

    from acolyte.config.settings import Settings
    from acolyte.port.job_queue import JobQueuePort
    from acolyte.port.llm_provider import LLMProviderPort
    from acolyte.port.report_repository import ReportRepositoryPort

logger = structlog.get_logger(__name__)

# Cap on concurrently running background pipelines per process. Not
# settings-driven — this bounds in-process resource usage (LLM/DB
# connections), not a business-tunable knob.
_MAX_CONCURRENT_RUNS = 4


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
        self._background_tasks: set[asyncio.Task[None]] = set()
        self._run_semaphore = asyncio.Semaphore(_MAX_CONCURRENT_RUNS)

    def set_graph(self, graph: CompiledStateGraph) -> None:
        """Inject the compiled LangGraph at startup time."""
        self._graph = graph

    @staticmethod
    def _thread_id_for_run(run_id: str) -> str:
        """Build the LangGraph checkpoint namespace for a run."""
        return f"acolyte-run:{run_id}"

    def _graph_config(self, run_id: str) -> dict[str, dict[str, str]]:
        """Build graph invocation config with stable thread_id."""
        return {"configurable": {"thread_id": self._thread_id_for_run(run_id)}}

    async def create_report(
        self, request: acolyte_pb2.CreateReportRequest, ctx: RequestContext
    ) -> acolyte_pb2.CreateReportResponse:
        report = await CreateReportUsecase(self._repo).execute(request.title, request.report_type)
        scope = dict(request.scope) if request.scope else {}
        if scope.get("topic"):
            brief = ReportBrief.from_scope(scope, request.report_type)
            await self._repo.create_brief(report.report_id, brief)
        return acolyte_pb2.CreateReportResponse(report_id=str(report.report_id))

    async def get_report(
        self, request: acolyte_pb2.GetReportRequest, ctx: RequestContext
    ) -> acolyte_pb2.GetReportResponse:
        report, sections = await GetReportUsecase(self._repo).execute(UUID(request.report_id))
        if report is None:
            raise ConnectError(Code.NOT_FOUND, f"Report {request.report_id} not found")

        brief = await self._repo.get_brief(report.report_id)

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

        # Surface any in-flight run so the FE can resume polling after a
        # navigation/reload without remembering the run_id client-side.
        active_run_proto: acolyte_pb2.ReportRun | None = None
        if self._jobs is not None:
            active_run = await self._jobs.get_active_run_for_report(report.report_id)
            if active_run is not None:
                active_run_proto = acolyte_pb2.ReportRun(
                    run_id=str(active_run.run_id),
                    report_id=str(active_run.report_id),
                    target_version_no=active_run.target_version_no,
                    run_status=active_run.run_status,
                    planner_model=active_run.planner_model or "",
                    writer_model=active_run.writer_model or "",
                    critic_model=active_run.critic_model or "",
                    started_at=active_run.started_at.isoformat() if active_run.started_at else None,
                    finished_at=active_run.finished_at.isoformat() if active_run.finished_at else None,
                    failure_code=active_run.failure_code,
                    failure_message=active_run.failure_message,
                )

        response = acolyte_pb2.GetReportResponse(
            report=acolyte_pb2.Report(
                report_id=str(report.report_id),
                title=report.title,
                report_type=report.report_type,
                current_version=report.current_version,
                created_at=report.created_at.isoformat(),
                scope=brief.to_scope() if brief else {},
            ),
            sections=proto_sections,
        )
        if active_run_proto is not None:
            response.active_run.CopyFrom(active_run_proto)
        return response

    async def list_reports(
        self, request: acolyte_pb2.ListReportsRequest, ctx: RequestContext
    ) -> acolyte_pb2.ListReportsResponse:
        cursor = request.cursor if request.cursor else None
        limit = request.limit if request.limit > 0 else 20
        reports, next_cursor = await ListReportsUsecase(self._repo).execute(cursor, limit)

        summaries = []
        for r in reports:
            latest_run_status = ""
            if self._jobs is not None:
                latest_run = await self._jobs.get_latest_run_for_report(r.report_id)
                if latest_run is not None:
                    latest_run_status = latest_run.run_status
            summaries.append(
                acolyte_pb2.ReportSummary(
                    report_id=str(r.report_id),
                    title=r.title,
                    report_type=r.report_type,
                    current_version=r.current_version,
                    latest_run_status=latest_run_status,
                    created_at=r.created_at.isoformat(),
                )
            )

        return acolyte_pb2.ListReportsResponse(
            reports=summaries,
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

        report_id = UUID(request.report_id)
        try:
            run = await StartRunUsecase(self._repo, self._jobs).execute(report_id)
        except StartRunRejectedError as e:
            # Circuit-breaker cooldown, not a missing report — distinct code
            # so a client/retry-loop can tell them apart.
            raise ConnectError(Code.FAILED_PRECONDITION, str(e)) from e
        except ValueError as e:
            raise ConnectError(Code.NOT_FOUND, str(e)) from e
        report = await self._repo.get_report(report_id)

        # Launch pipeline in background if graph is wired
        if self._graph is not None and report is not None:
            brief = await self._repo.get_brief(report.report_id)
            brief_dict = brief.to_dict() if brief else {"topic": report.title}
            task = asyncio.create_task(
                self._run_pipeline(str(report.report_id), str(run.run_id), brief_dict),
                name=f"acolyte-run-{run.run_id}",
            )
            self._background_tasks.add(task)
            task.add_done_callback(self._background_tasks.discard)

        return acolyte_pb2.StartReportRunResponse(run_id=str(run.run_id))

    async def resume_pipeline(self, report_id: str, run_id: str, brief_dict: dict[str, Any]) -> None:
        """Public entry point for operator tooling (e.g. scripts/resume_run.py)
        to resume a checkpointed run outside of start_report_run's background task.
        """
        await self._run_pipeline(report_id, run_id, brief_dict)

    async def _run_pipeline(self, report_id: str, run_id: str, brief_dict: dict[str, Any]) -> None:
        """Execute LangGraph pipeline in background, bounded by max_concurrent_runs."""
        async with self._run_semaphore:
            await self._run_pipeline_locked(report_id, run_id, brief_dict)

    async def _run_pipeline_locked(self, report_id: str, run_id: str, brief_dict: dict[str, Any]) -> None:  # noqa: PLR0912, PLR0915 — run-lifecycle state machine (checkpoint resume/DLQ/status transitions), splitting would obscure the single narrative
        if self._graph is None:
            raise RuntimeError("Pipeline graph not configured")  # noqa: TRY003 — internal wiring invariant, not a domain error to catch

        if self._jobs is not None:
            # Single default_model for all three roles — the pipeline is
            # mono-model today; report_runs keeps separate columns for a
            # future per-role routing split.
            model = self._settings.default_model
            await self._jobs.mark_running(UUID(run_id), model, model, model)

        config = self._graph_config(run_id)
        initial_state = {
            "report_id": report_id,
            "run_id": run_id,
            "brief": brief_dict,
            "revision_count": 0,
        }

        logger.info(
            "Pipeline started", report_id=report_id, run_id=run_id, thread_id=config["configurable"]["thread_id"]
        )
        try:
            invoke_input = initial_state

            if self._settings.checkpoint_enabled:
                snapshot = await self._graph.aget_state(config)  # type: ignore[bad-argument-type]
                if snapshot.next:
                    logger.info(
                        "Resuming pipeline from checkpoint",
                        report_id=report_id,
                        run_id=run_id,
                        pending_nodes=list(snapshot.next),
                    )
                    invoke_input = None
                elif snapshot.values and snapshot.values.get("final_version_no") is not None:
                    # Terminal checkpoint with final_version_no — truly completed
                    logger.info(
                        "Pipeline already completed for run, reusing terminal checkpoint",
                        report_id=report_id,
                        run_id=run_id,
                    )
                    result = dict(snapshot.values)
                    final_version = result.get("final_version_no")
                    error = result.get("error")
                    if error:
                        logger.error("Pipeline failed", report_id=report_id, run_id=run_id, error=error)
                        if self._jobs is not None:
                            failure_code = result.get("failure_code") or "pipeline_error"
                            await self._jobs.fail_run(UUID(run_id), failure_code, str(error))
                    else:
                        logger.info(
                            "Pipeline completed",
                            report_id=report_id,
                            run_id=run_id,
                            final_version=final_version,
                        )
                        if self._jobs is not None:
                            await self._jobs.complete_run(UUID(run_id))
                    return
                elif snapshot.values:
                    # Terminal state but no final_version_no — suspicious/incomplete
                    logger.warning(
                        "Terminal checkpoint without final_version_no, re-running pipeline",
                        report_id=report_id,
                        run_id=run_id,
                        state_keys=list(snapshot.values.keys()),
                    )

            # durability="sync" ensures checkpoint is persisted before proceeding
            # to the next super-step (critical for 70+ minute runs)
            durability = "sync" if self._settings.checkpoint_enabled else None

            result = await self._graph.ainvoke(  # type: ignore[bad-argument-type]
                invoke_input,
                config=config,
                durability=durability,
            )

            final_version = result.get("final_version_no")
            error = result.get("error")

            if error:
                logger.error("Pipeline failed", report_id=report_id, run_id=run_id, error=error)
                if self._jobs is not None:
                    failure_code = result.get("failure_code") or "pipeline_error"
                    await self._jobs.fail_run(UUID(run_id), failure_code, str(error))
            else:
                logger.info("Pipeline completed", report_id=report_id, run_id=run_id, final_version=final_version)
                if self._jobs is not None:
                    await self._jobs.complete_run(UUID(run_id))

        except Exception as exc:
            logger.exception("Pipeline crashed", report_id=report_id, run_id=run_id, error=str(exc))
            if self._jobs is not None:
                try:
                    await self._jobs.fail_run(UUID(run_id), "pipeline_crashed", str(exc))
                except Exception as mark_exc:
                    logger.exception(
                        "Failed to mark run failed after crash",
                        report_id=report_id,
                        run_id=run_id,
                        error=str(mark_exc),
                    )

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

        uc = RerunSectionUsecase(self._repo, self._llm)
        try:
            await uc.execute(UUID(request.report_id), request.section_key)
        except ValueError as e:
            raise ConnectError(Code.NOT_FOUND, str(e)) from e
        except ConnectError:
            # Typed error raised from a lower layer (e.g. the usecase
            # re-mapping a repo ValueError). Preserve the code untouched.
            raise
        except Exception as exc:
            # Last-line visibility so a 500 still ships a traceback to
            # the structured log instead of disappearing behind the
            # Connect-RPC INTERNAL envelope. Mirrors the pattern used by
            # the pipeline runner above (line 284).
            logger.exception(
                "RerunSection failed",
                report_id=request.report_id,
                section_key=request.section_key,
                error_type=type(exc).__name__,
                error=str(exc),
            )
            raise ConnectError(Code.INTERNAL, f"rerun failed: {type(exc).__name__}") from exc

        return acolyte_pb2.RerunSectionResponse(run_id="")

    async def delete_report(
        self, request: acolyte_pb2.DeleteReportRequest, ctx: RequestContext
    ) -> acolyte_pb2.DeleteReportResponse:
        try:
            rid = UUID(request.report_id)
        except ValueError as e:
            raise ConnectError(Code.INVALID_ARGUMENT, f"Invalid report_id: {e}") from e

        report = await self._repo.get_report(rid)
        if report is None:
            raise ConnectError(Code.NOT_FOUND, f"Report {rid} not found")

        if await self._repo.has_active_run(rid):
            raise ConnectError(
                Code.FAILED_PRECONDITION,
                "A report generation is in progress; wait or cancel before deleting.",
            )

        await self._repo.delete_report(rid)
        logger.info("Report deleted", report_id=str(rid))
        return acolyte_pb2.DeleteReportResponse()

    async def health_check(
        self, request: acolyte_pb2.HealthCheckRequest, ctx: RequestContext
    ) -> acolyte_pb2.HealthCheckResponse:
        return acolyte_pb2.HealthCheckResponse(status="ok")
