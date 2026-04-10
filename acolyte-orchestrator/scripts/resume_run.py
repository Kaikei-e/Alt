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
import sys

import structlog

logger = structlog.get_logger(__name__)


async def _resume(run_id: str) -> None:
    """Resume a pipeline run from its checkpoint."""
    # Late imports to avoid loading the full app at import time
    import httpx
    from psycopg_pool import AsyncConnectionPool

    from acolyte.config.settings import Settings
    from acolyte.domain.fusion import RRFFusion
    from acolyte.gateway.checkpoint_factory import create_checkpointer
    from acolyte.gateway.memory_content_store import MemoryContentStore
    from acolyte.gateway.ollama_gw import OllamaGateway
    from acolyte.gateway.postgres_job_gw import PostgresJobGateway
    from acolyte.gateway.postgres_report_gw import PostgresReportGateway
    from acolyte.gateway.search_indexer_gw import SearchIndexerGateway
    from acolyte.handler.connect_service import AcolyteConnectService
    from acolyte.infra.logging import configure_logging
    from acolyte.usecase.graph.report_graph import build_report_graph

    settings = Settings()
    if not settings.checkpoint_enabled:
        logger.error("CHECKPOINT_ENABLED must be true for resume")
        sys.exit(1)

    configure_logging(log_level=settings.log_level)
    dsn = settings.resolve_db_dsn()

    async with AsyncConnectionPool(dsn, min_size=1, max_size=3) as pool:
        repo = PostgresReportGateway(pool)
        job_gw = PostgresJobGateway(pool)

        # Resolve run -> report -> brief
        from uuid import UUID

        run = await job_gw.get_run(UUID(run_id))
        if run is None:
            logger.error("Run not found", run_id=run_id)
            sys.exit(1)

        report_id = str(run.report_id)
        brief = await repo.get_brief(run.report_id)
        if brief is None:
            logger.error("Brief not found for report", report_id=report_id)
            sys.exit(1)

        brief_dict = brief.to_dict()

        async with httpx.AsyncClient(
            timeout=httpx.Timeout(connect=10, read=600, write=10, pool=10),
            limits=httpx.Limits(max_connections=5, max_keepalive_connections=2),
        ) as http_client:
            llm = OllamaGateway(http_client, settings)
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

                service = AcolyteConnectService(settings, repo, graph=graph)
                logger.info("Resuming pipeline", run_id=run_id, report_id=report_id)
                await service._run_pipeline(report_id, run_id, brief_dict)

        logger.info("Resume complete", run_id=run_id)


def main() -> None:
    parser = argparse.ArgumentParser(description="Resume an Acolyte pipeline run from checkpoint")
    parser.add_argument("--run-id", required=True, help="UUID of the run to resume")
    args = parser.parse_args()

    asyncio.run(_resume(args.run_id))


if __name__ == "__main__":
    main()
