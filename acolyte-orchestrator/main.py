"""Application factory — Composition Root + DI wiring."""

from __future__ import annotations

import asyncio
import os
from contextlib import asynccontextmanager, suppress
from typing import TYPE_CHECKING
from uuid import UUID

import httpx
import structlog
from psycopg_pool import AsyncConnectionPool
from starlette.applications import Starlette
from starlette.middleware import Middleware
from starlette.responses import JSONResponse
from starlette.routing import Mount, Route

import acolyte.gen  # noqa: F401 — must precede generated imports
from acolyte.config.settings import Settings
from acolyte.domain.fusion import RRFFusion
from acolyte.driver.datahub_client import DataHubClientFactory
from acolyte.gateway.checkpoint_factory import create_checkpointer
from acolyte.gateway.datahub_notification_gw import DataHubNotificationGateway
from acolyte.gateway.memory_content_store import MemoryContentStore
from acolyte.gateway.news_creator_hyde_gw import build_hyde_generator
from acolyte.gateway.ollama_gw import OllamaGateway
from acolyte.gateway.postgres_job_gw import PostgresJobGateway
from acolyte.gateway.postgres_notification_outbox_gw import PostgresNotificationOutboxGateway
from acolyte.gateway.postgres_report_gw import PostgresReportGateway
from acolyte.gateway.search_indexer_gw import SearchIndexerGateway
from acolyte.gateway.vllm_gw import VllmGateway
from acolyte.gen.proto.alt.acolyte.v1.acolyte_connect import AcolyteServiceASGIApplication
from acolyte.handler.connect_service import AcolyteConnectService
from acolyte.infra.logging import configure_logging
from acolyte.infra.peer_identity import PeerIdentityMiddleware, allowed_peers_from_env
from acolyte.usecase.graph.report_graph import build_report_graph
from acolyte.usecase.reconcile_orphaned_runs_uc import ReconcileOrphanedRunsUsecase
from acolyte.usecase.relay_notifications_uc import RelayNotificationsUsecase

if TYPE_CHECKING:
    from collections.abc import AsyncGenerator

    from langgraph.graph.state import CompiledStateGraph
    from starlette.requests import Request


settings = Settings()
configure_logging(log_level=settings.log_level)
logger = structlog.get_logger(__name__)

# DB pool (opened in lifespan)
_dsn = settings.resolve_db_dsn()
_pool = AsyncConnectionPool(_dsn, min_size=settings.db_pool_min_size, max_size=settings.db_pool_max_size, open=False)
_report_repo = PostgresReportGateway(_pool)

# Notification outbox. One switch drives both halves: the producer writes the
# row inside the run-completion transaction, the relay forwards it to
# alt-data-hub. Missing configuration raises here, before the process serves
# anything — a relay that limps along without a target or a client certificate
# only shows up as an outbox that quietly grows.
_relay_config = settings.resolve_notification_relay_config()
_relay: RelayNotificationsUsecase | None = None
if _relay_config is None:
    logger.warning(
        "notification_outbox_relay_disabled",
        reason="NOTIFICATIONS_ENABLED is not true — report completions enqueue no notifications",
    )
else:
    _relay = RelayNotificationsUsecase(
        PostgresNotificationOutboxGateway(_pool),
        DataHubNotificationGateway(
            DataHubClientFactory(
                base_url=_relay_config.datahub_url,
                cert_file=_relay_config.cert_file,
                key_file=_relay_config.key_file,
                ca_file=_relay_config.ca_file,
            ),
            ttl_seconds=_relay_config.ttl_seconds,
        ),
        worker_id=settings.worker_id,
        batch_size=_relay_config.batch_size,
    )
    logger.info(
        "notification_outbox_relay_enabled",
        datahub_url=_relay_config.datahub_url,
        notification_user_id=str(_relay_config.user_id),
        batch_size=_relay_config.batch_size,
        interval_seconds=_relay_config.interval_seconds,
    )

# Persistent job queue — must stay consistent with PostgresReportGateway.has_active_run
# (which reads report_runs), or the delete_report in-progress guard is always False
# after a restart. MemoryJobGateway is test-only (see tests/conftest.py).
_job_queue = PostgresJobGateway(
    _pool,
    notification_user_id=None if _relay_config is None else _relay_config.user_id,
)


# HTTP client for Ollama and search-indexer (600s timeout for 26B model with 8192 num_predict).
# When MTLS_ENFORCE=true the shared AsyncClient presents the acolyte-orchestrator
# leaf cert on every handshake; every downstream must trust alt-ca. The
# SSLContext is reused across requests and the lifespan spawns a watcher
# that reloads the leaf cert in place whenever pki-agent rotates it.
from acolyte.infra.mtls_client import (  # noqa: E402
    SslContextReloader,
    build_ssl_context,
    watch_cert_rotation,
)

_mtls_ctx = build_ssl_context()
_mtls_reloader: SslContextReloader | None = None
if _mtls_ctx is not None:
    _mtls_reloader = SslContextReloader(
        _mtls_ctx,
        os.environ["MTLS_CERT_FILE"],
        os.environ["MTLS_KEY_FILE"],
    )
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

# Process-global content store (article body cache for hydrator top-N fetch).
# Bounded LRU — see MemoryContentStore docstring for why this cannot be a
# plain unbounded dict.
_content_store = MemoryContentStore(max_size=settings.content_store_max_size)

# Evidence gateway (search-indexer / Meilisearch)
_search_gw = SearchIndexerGateway(_http_client, settings, _content_store)

# Fusion strategy for hybrid retrieval (Issue 7: RRF default, CC future)
_fusion = RRFFusion(k=60)

# HyDE generator (cross-lingual recall expansion via Gemma4). Disabled when
# ``hyde_enabled`` is false so the pipeline falls back to BM25+RRF alone.
# build_hyde_generator is the shared wiring source of truth — scripts/resume_run.py
# must derive the same generator from the same settings, or a resumed run silently
# drops HyDE expansion relative to the production start_report_run path.
_hyde_generator = build_hyde_generator(_llm_gw, settings)


def _compile_graph(*, checkpointer: object | None = None) -> CompiledStateGraph:
    """Compile the LangGraph report pipeline with optional checkpointing."""
    return build_report_graph(
        _llm_gw,
        _search_gw,
        _report_repo,
        content_store=_content_store,
        fusion=_fusion,
        checkpointer=checkpointer,
        settings=settings,
        hyde_generator=_hyde_generator,
    )


async def health_endpoint(request: Request) -> JSONResponse:
    """Health check endpoint for Docker healthcheck."""
    return JSONResponse({"status": "ok", "service": "acolyte-orchestrator"})


# Shutdown budget for pipelines still inside the graph. compose/acolyte.yaml sets
# no stop_grace_period, so the whole teardown has to fit inside Docker's 10s
# default. The window is not there to let a 70-minute run finish — it is there so
# a pipeline one await away from complete_run lands its own terminal write
# instead of being cancelled in FinalizerNode's gap, between the version bump and
# the run's completion. Not settings-driven: it is a property of the container's
# stop grace, not a business knob.
_PIPELINE_DRAIN_GRACE_SECONDS = 2.0

# Deliberately distinct from ReconcileOrphanedRunsUsecase's
# 'orphaned_after_restart'. That code means "found this row already stale at
# boot"; this one means "we stopped it on purpose, and the row is accurate as of
# now" — which is what a scale-down needs, since no later startup may ever come
# to reconcile it.
_SHUTDOWN_FAILURE_CODE = "shutdown_interrupted"

# Mirrors the task name AcolyteConnectService.start_report_run assigns
# (f"acolyte-run-{run_id}"), the only handle a cancelled task carries back to
# its run row.
_RUN_TASK_NAME_PREFIX = "acolyte-run-"

# The statuses list_running_runs treats as unfinished — the only ones a shutdown
# is entitled to rewrite.
_UNFINISHED_RUN_STATUSES = frozenset({"pending", "running"})


async def _fail_interrupted_run(task: asyncio.Task[None]) -> None:
    """Mark the run behind a shutdown-cancelled pipeline task as failed."""
    try:
        run_id = UUID(task.get_name().removeprefix(_RUN_TASK_NAME_PREFIX))
    except ValueError:
        # The naming contract with start_report_run broke. Loud, but not fatal:
        # raising here would skip the pool close and leak connections, and the
        # next boot's ReconcileOrphanedRunsUsecase still catches the row.
        logger.exception("Interrupted pipeline task carries no run id", task_name=task.get_name())
        return

    # Both statements talk to the Postgres this teardown is about to let go of,
    # so neither may hold the rest of the shutdown hostage — the pool and the
    # HTTP client still have to close.
    try:
        # The cancellation can land after complete_run's UPDATE committed but
        # before the coroutine returned. fail_run is an unconditional UPDATE, so
        # without this read it would pair a bumped report version with a failed
        # run — the corruption the drain exists to prevent.
        run = await _job_queue.get_run(run_id)
        if run is not None and run.run_status not in _UNFINISHED_RUN_STATUSES:
            logger.info(
                "Interrupted pipeline had already settled its run",
                run_id=str(run_id),
                run_status=run.run_status,
            )
            return
        await _job_queue.fail_run(
            run_id,
            _SHUTDOWN_FAILURE_CODE,
            "Run was cancelled while the process was shutting down.",
        )
    except Exception as exc:
        logger.exception("Failed to mark interrupted run failed during shutdown", run_id=str(run_id), error=str(exc))


async def _drain_report_pipelines(service: AcolyteConnectService) -> None:
    """Settle in-flight pipelines before the pool and HTTP client go away.

    Nothing else awaits the background tasks start_report_run spawns. Closing
    the pool underneath one raises PoolClosed inside the pipeline, and its own
    crash handler then fails to write fail_run through that same dead pool — so
    the run row says 'running' forever, wedging has_active_run (the
    delete_report guard) and GetReport.active_run for its report. Every
    redeploy that lands mid-run does this.
    """
    # Snapshotted: the handler's done-callback mutates the set as tasks finish.
    in_flight = set(service._background_tasks)
    if not in_flight:
        return

    logger.info(
        "Draining in-flight report pipelines",
        count=len(in_flight),
        grace_seconds=_PIPELINE_DRAIN_GRACE_SECONDS,
    )
    _, unfinished = await asyncio.wait(in_flight, timeout=_PIPELINE_DRAIN_GRACE_SECONDS)
    for task in unfinished:
        task.cancel()
    # return_exceptions, because a pipeline can settle with something other than
    # CancelledError — _run_pipeline_locked raises outside its own except clause
    # on an unconfigured graph. Letting that escape here unwinds the lifespan
    # finally and skips the HTTP client and pool closes below it, leaking both
    # on every redeploy.
    await asyncio.gather(*unfinished, return_exceptions=True)
    for task in unfinished:
        logger.warning("Pipeline cancelled by shutdown", task_name=task.get_name())
        await _fail_interrupted_run(task)


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
        reconciled = await ReconcileOrphanedRunsUsecase(_job_queue).execute()
        if reconciled:
            logger.warning("Reconciled orphaned runs left by a prior process", count=reconciled)
        cert_watch_task: asyncio.Task[None] | None = None
        if _mtls_reloader is not None:
            cert_watch_task = asyncio.create_task(
                watch_cert_rotation(_mtls_reloader, interval_seconds=30.0),
                name="mtls-cert-rotation-watch",
            )
        relay_task: asyncio.Task[None] | None = None
        if _relay is not None and _relay_config is not None:
            relay_task = asyncio.create_task(
                _relay.run_forever(_relay_config.interval_seconds),
                name="notification-outbox-relay",
            )

        async def _drain() -> None:
            # Periodic loops first — they hold no in-flight business state. The
            # pipelines then get their window while the checkpointer connection,
            # the pool and the HTTP client are all still usable.
            for task in (relay_task, cert_watch_task):
                if task is not None:
                    task.cancel()
                    with suppress(asyncio.CancelledError):
                        await task
            await _drain_report_pipelines(connect_service)

        try:
            # The drain sits *inside* the checkpointer scope on purpose. With
            # CHECKPOINT_ENABLED=true the graph runs durability="sync", so every
            # super-step writes through the single connection create_checkpointer
            # owns; draining after that context exits would hand a pipeline its
            # grace window with nothing left to write the terminal checkpoint to.
            if settings.checkpoint_enabled:
                async with create_checkpointer(_dsn) as checkpointer:
                    connect_service.set_graph(_compile_graph(checkpointer=checkpointer))
                    logger.info("LangGraph checkpointing enabled")
                    try:
                        yield
                    finally:
                        await _drain()
            else:
                logger.info("LangGraph checkpointing disabled")
                try:
                    yield
                finally:
                    await _drain()
        finally:
            await _http_client.aclose()
            await _pool.close()
            logger.info("Shutting down acolyte-orchestrator")

    asgi_app = AcolyteServiceASGIApplication(connect_service)

    # capture the peer CN injected by the nginx mTLS sidecar
    # (VERIFY_CLIENT=on) and propagate into request.state + log context.
    # peer_identity_strict defaults False during rollout; set
    # PEER_IDENTITY_STRICT=true at cutover (no rebuild required).
    peer_identity_middleware = Middleware(
        PeerIdentityMiddleware,
        allowed=allowed_peers_from_env(),
        strict=settings.peer_identity_strict,
    )

    app = Starlette(
        lifespan=lifespan,
        middleware=[peer_identity_middleware],
        # /health plus the Connect mount, and nothing else. This listener
        # authenticates nobody, so every route added here is an unauthenticated
        # route — the relay's own signals go out as log fields for that reason.
        routes=[
            Route("/health", health_endpoint),
            Mount(asgi_app.path, app=asgi_app),
        ],
    )
    app.state.connect_service = connect_service

    return app
