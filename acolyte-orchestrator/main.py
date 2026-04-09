"""Application factory — Composition Root + DI wiring."""

from __future__ import annotations

from contextlib import asynccontextmanager
from typing import TYPE_CHECKING

import httpx
import structlog
from psycopg_pool import AsyncConnectionPool
from starlette.applications import Starlette
from starlette.responses import JSONResponse
from starlette.routing import Mount, Route

import acolyte.gen  # noqa: F401, I001 — must precede generated imports
from acolyte.config.settings import Settings
from acolyte.gateway.memory_job_gw import MemoryJobGateway
from acolyte.gateway.ollama_gw import OllamaGateway
from acolyte.gateway.postgres_report_gw import PostgresReportGateway
from acolyte.gateway.search_indexer_gw import SearchIndexerGateway
from acolyte.gen.proto.alt.acolyte.v1.acolyte_connect import AcolyteServiceASGIApplication
from acolyte.handler.connect_service import AcolyteConnectService
from acolyte.infra.logging import configure_logging
from acolyte.usecase.graph.report_graph import build_report_graph

if TYPE_CHECKING:
    from collections.abc import AsyncGenerator

    from starlette.requests import Request


settings = Settings()
configure_logging(log_level=settings.log_level)
logger = structlog.get_logger(__name__)

# DB pool (opened in lifespan)
_dsn = settings.resolve_db_dsn()
_pool = AsyncConnectionPool(_dsn, min_size=settings.db_pool_min_size, max_size=settings.db_pool_max_size, open=False)
_report_repo = PostgresReportGateway(_pool)
_job_queue = MemoryJobGateway()

# HTTP client for Ollama and search-indexer (300s timeout for 26B model — ADR-579)
_http_client = httpx.AsyncClient(
    timeout=httpx.Timeout(connect=10, read=300, write=10, pool=10),
    limits=httpx.Limits(max_connections=10, max_keepalive_connections=5),
)

# LLM gateway (Ollama on AIX — ADR-579: consistent options to prevent model reload)
_ollama_gw = OllamaGateway(_http_client, settings)

# Evidence gateway (search-indexer / Meilisearch)
_search_gw = SearchIndexerGateway(_http_client, settings)

# LangGraph pipeline
_graph = build_report_graph(_ollama_gw, _search_gw, _report_repo)


@asynccontextmanager
async def lifespan(app: Starlette) -> AsyncGenerator[None]:
    """Application lifespan — open DB pool on startup, close on shutdown."""
    logger.info("Starting acolyte-orchestrator", host=settings.host, port=settings.port)
    await _pool.open()
    logger.info(
        "Database connection pool opened",
        dsn=_dsn.split("@")[-1],
        llm_url=settings.news_creator_url,
        model=settings.default_model,
    )
    yield
    await _http_client.aclose()
    await _pool.close()
    logger.info("Shutting down acolyte-orchestrator")


async def health_endpoint(request: Request) -> JSONResponse:
    """Health check endpoint for Docker healthcheck."""
    return JSONResponse({"status": "ok", "service": "acolyte-orchestrator"})


def create_app() -> Starlette:
    """Create Starlette ASGI application instance."""
    connect_service = AcolyteConnectService(settings, _report_repo, _job_queue, _graph)
    asgi_app = AcolyteServiceASGIApplication(connect_service)

    app = Starlette(
        lifespan=lifespan,
        routes=[
            Route("/health", health_endpoint),
            Mount(asgi_app.path, app=asgi_app),
        ],
    )

    return app
