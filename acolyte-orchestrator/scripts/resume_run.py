#!/usr/bin/env python3
"""Operator tool: resume an Acolyte pipeline run from its last checkpoint.

Usage:
    uv run python scripts/resume_run.py --run-id <uuid>

Requires:
    - ACOLYTE_DB_DSN environment variable (or defaults to localhost)
    - NEWS_CREATOR_URL for LLM inference
    - SEARCH_INDEXER_URL for evidence retrieval
    - CHECKPOINT_ENABLED=true (enforced at runtime)

The script resolves run_id -> report_id -> brief from the database,
then invokes the same _run_pipeline code path that StartReportRun uses.
Because the thread_id is deterministic (acolyte-run:{run_id}), the
checkpointer will find the existing checkpoint and resume from the
last successful super-step.
"""

from __future__ import annotations

import argparse
import asyncio
import os
import sys
from typing import TYPE_CHECKING, Any
from uuid import UUID

import structlog

if TYPE_CHECKING:
    from acolyte.gateway.postgres_job_gw import PostgresJobGateway
    from acolyte.gateway.postgres_report_gw import PostgresReportGateway

logger = structlog.get_logger(__name__)


async def _resolve_run_brief(
    job_gw: PostgresJobGateway,
    repo: PostgresReportGateway,
    run_id: str,
) -> tuple[str, dict[str, Any]]:
    """Resolve run_id -> report_id + brief dict, or exit on missing rows."""
    run = await job_gw.get_run(UUID(run_id))
    if run is None:
        logger.error("Run not found", run_id=run_id)
        sys.exit(1)

    report_id = str(run.report_id)
    brief = await repo.get_brief(run.report_id)
    if brief is None:
        logger.error("Brief not found for report", report_id=report_id)
        sys.exit(1)
    return report_id, brief.to_dict()


async def _resume(run_id: str) -> None:
    """Resume a pipeline run from its checkpoint."""
    # Late imports to avoid loading the full app at import time (keeps --help fast)
    import httpx  # noqa: PLC0415
    from psycopg_pool import AsyncConnectionPool  # noqa: PLC0415

    from acolyte.config.settings import Settings  # noqa: PLC0415
    from acolyte.domain.fusion import RRFFusion  # noqa: PLC0415
    from acolyte.gateway.checkpoint_factory import create_checkpointer  # noqa: PLC0415
    from acolyte.gateway.memory_content_store import MemoryContentStore  # noqa: PLC0415
    from acolyte.gateway.ollama_gw import OllamaGateway  # noqa: PLC0415
    from acolyte.gateway.postgres_job_gw import PostgresJobGateway  # noqa: PLC0415
    from acolyte.gateway.postgres_report_gw import PostgresReportGateway  # noqa: PLC0415
    from acolyte.gateway.search_indexer_gw import SearchIndexerGateway  # noqa: PLC0415
    from acolyte.gateway.vllm_gw import VllmGateway  # noqa: PLC0415
    from acolyte.handler.connect_service import AcolyteConnectService  # noqa: PLC0415
    from acolyte.infra.logging import configure_logging  # noqa: PLC0415
    from acolyte.infra.mtls_client import build_ssl_context  # noqa: PLC0415
    from acolyte.usecase.graph.report_graph import build_report_graph  # noqa: PLC0415

    settings = Settings()
    if not settings.checkpoint_enabled:
        logger.error("CHECKPOINT_ENABLED must be true for resume")
        sys.exit(1)

    configure_logging(log_level=settings.log_level)
    dsn = settings.resolve_db_dsn()

    async with AsyncConnectionPool(dsn, min_size=1, max_size=3) as pool:
        repo = PostgresReportGateway(pool)
        job_gw = PostgresJobGateway(pool)
        report_id, brief_dict = await _resolve_run_brief(job_gw, repo, run_id)

        # Mirror main.py: mTLS context + LLM provider selection
        mtls_ctx = build_ssl_context()
        if mtls_ctx is not None:
            logger.info("resume_run outbound: mTLS enforce enabled")

        async with httpx.AsyncClient(
            timeout=httpx.Timeout(connect=10, read=600, write=10, pool=10),
            limits=httpx.Limits(max_connections=5, max_keepalive_connections=2),
            verify=mtls_ctx if mtls_ctx is not None else True,
        ) as http_client:
            llm = (
                VllmGateway(http_client, settings)
                if settings.llm_provider == "vllm"
                else OllamaGateway(http_client, settings)
            )
            content_store = MemoryContentStore()
            evidence = SearchIndexerGateway(http_client, settings, content_store)
            fusion = RRFFusion(k=60)

            async with create_checkpointer(dsn) as checkpointer:
                graph = build_report_graph(
                    llm,
                    evidence,
                    repo,
                    content_store=content_store,
                    fusion=fusion,
                    checkpointer=checkpointer,
                    settings=settings,
                )

                service = AcolyteConnectService(settings, repo, job_gw, graph=graph)
                logger.info(
                    "Resuming pipeline",
                    run_id=run_id,
                    report_id=report_id,
                    llm_provider=settings.llm_provider,
                    mtls=mtls_ctx is not None,
                    cert_file=os.environ.get("MTLS_CERT_FILE"),
                )
                await service.resume_pipeline(report_id, run_id, brief_dict)

        logger.info("Resume complete", run_id=run_id)


def main() -> None:
    parser = argparse.ArgumentParser(description="Resume an Acolyte pipeline run from checkpoint")
    parser.add_argument("--run-id", required=True, help="UUID of the run to resume")
    args = parser.parse_args()

    asyncio.run(_resume(args.run_id))


if __name__ == "__main__":
    main()
