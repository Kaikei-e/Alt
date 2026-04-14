"""Application factory — Composition Root + DI wiring."""

from __future__ import annotations

import os
import ssl
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
from acolyte.domain.fusion import RRFFusion
from acolyte.gateway.checkpoint_factory import create_checkpointer
from acolyte.gateway.memory_content_store import MemoryContentStore
from acolyte.gateway.memory_job_gw import MemoryJobGateway
from acolyte.gateway.ollama_gw import OllamaGateway
from acolyte.gateway.postgres_report_gw import PostgresReportGateway
from acolyte.gateway.search_indexer_gw import SearchIndexerGateway
from acolyte.gateway.vllm_gw import VllmGateway
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


def _build_mtls_context() -> ssl.SSLContext | None:
    """Build an SSLContext that presents the acolyte-orchestrator leaf cert.

    Returns None when MTLS_ENFORCE is not enabled. Raises when enforcement is
    requested but required env is missing or certs are unreadable (fail-closed).
    """
    if os.getenv("MTLS_ENFORCE") != "true":
        return None
    cert = os.getenv("MTLS_CERT_FILE", "")
    key = os.getenv("MTLS_KEY_FILE", "")
    ca = os.getenv("MTLS_CA_FILE", "")
    if not (cert and key and ca):
        msg = "MTLS_ENFORCE=true but MTLS_CERT_FILE/KEY_FILE/CA_FILE not fully set"
        raise RuntimeError(msg)
    ctx = ssl.create_default_context(ssl.Purpose.SERVER_AUTH, cafile=ca)
    ctx.load_cert_chain(certfile=cert, keyfile=key)
    ctx.minimum_version = ssl.TLSVersion.TLSv1_3
    return ctx


# HTTP client for Ollama and search-indexer (600s timeout for 26B model with 8192 num_predict).
# When MTLS_ENFORCE=true the shared AsyncClient presents the acolyte-orchestrator
# leaf cert on every handshake; every downstream must trust alt-ca.
_mtls_ctx = _build_mtls_context()
_http_client = httpx.AsyncClient(
    timeout=httpx.Timeout(connect=10, read=600, write=10, pool=10),
    limits=httpx.Limits(max_connections=10, max_keepalive_connections=5),
    verify=_mtls_ctx if _mtls_ctx is not None else True,
)
if _mtls_ctx is not None:
    logger.info("acolyte-orchestrator outbound: mTLS enforce enabled")

# LLM gateway — provider selection via LLM_PROVIDER env var
if settings.llm_provider == "vllm":
    _llm_gw = VllmGateway(_http_client, settings)
else:
    _llm_gw = OllamaGateway(_http_client, settings)

# Run-scoped content store (article body cache for hydrator top-N fetch)
_content_store = MemoryContentStore()

# Evidence gateway (search-indexer / Meilisearch)
_search_gw = SearchIndexerGateway(_http_client, settings, _content_store)

# Fusion strategy for hybrid retrieval (Issue 7: RRF default, CC future)
_fusion = RRFFusion(k=60)


def _compile_graph(*, checkpointer: object | None = None):
    """Compile the LangGraph report pipeline with optional checkpointing."""
    return build_report_graph(
        _llm_gw,
        _search_gw,
        _report_repo,
        content_store=_content_store,
        fusion=_fusion,
        checkpointer=checkpointer,
        settings=settings,
    )


async def health_endpoint(request: Request) -> JSONResponse:
    """Health check endpoint for Docker healthcheck."""
    return JSONResponse({"status": "ok", "service": "acolyte-orchestrator"})


def create_app() -> Starlette:
    """Create Starlette ASGI application instance."""
    initial_graph = None if settings.checkpoint_enabled else _compile_graph()
    connect_service = AcolyteConnectService(settings, _report_repo, _job_queue, initial_graph, llm=_llm_gw)

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
        try:
            if settings.checkpoint_enabled:
                async with create_checkpointer(_dsn) as checkpointer:
                    connect_service.set_graph(_compile_graph(checkpointer=checkpointer))
                    logger.info("LangGraph checkpointing enabled")
                    yield
            else:
                logger.info("LangGraph checkpointing disabled")
                yield
        finally:
            await _http_client.aclose()
            await _pool.close()
            logger.info("Shutting down acolyte-orchestrator")

    asgi_app = AcolyteServiceASGIApplication(connect_service)

    app = Starlette(
        lifespan=lifespan,
        routes=[
            Route("/health", health_endpoint),
            Mount(asgi_app.path, app=asgi_app),
        ],
    )
    app.state.connect_service = connect_service

    return app
