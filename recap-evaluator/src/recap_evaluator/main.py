"""FastAPI application — Composition Root + DI wiring."""

from __future__ import annotations

import asyncio
import os
from collections.abc import AsyncIterator
from contextlib import asynccontextmanager

import asyncpg
import httpx
import structlog
from fastapi import FastAPI
from fastapi.middleware.cors import CORSMiddleware

from recap_evaluator.config import AlertThresholds, EvaluatorWeights, Settings
from recap_evaluator.evaluator.cluster_evaluator import ClusterEvaluator
from recap_evaluator.evaluator.genre_evaluator import GenreEvaluator
from recap_evaluator.evaluator.pipeline_evaluator import PipelineEvaluator
from recap_evaluator.evaluator.summary_evaluator import SummaryEvaluator
from recap_evaluator.gateway.ollama_gateway import OllamaGateway
from recap_evaluator.gateway.postgres_gateway import PostgresGateway, register_jsonb_codec
from recap_evaluator.gateway.recap_worker_gateway import RecapWorkerGateway
from recap_evaluator.handler.evaluation_handler import router as evaluation_router
from recap_evaluator.handler.health_handler import router as health_router
from recap_evaluator.handler.metrics_handler import router as metrics_router
from recap_evaluator.scheduler.evaluation_scheduler import EvaluationScheduler
from recap_evaluator.usecase.get_metrics import GetMetricsUsecase
from recap_evaluator.usecase.run_evaluation import RunEvaluationUsecase
from recap_evaluator.utils.logging import configure_logging, shutdown_logging
from recap_evaluator.utils.otel import instrument_fastapi

logger = structlog.get_logger()


def create_app(
    settings: Settings | None = None,
    alert_thresholds: AlertThresholds | None = None,
    evaluator_weights: EvaluatorWeights | None = None,
) -> FastAPI:
    """Build the FastAPI app.

    Settings are optional so unit tests can import this module (and inject
    fixtures) without requiring RECAP_DB_DSN / RECAP_DB_PASSWORD. Production
    loads Settings inside lifespan — fail-fast at startup, not at import.
    """

    @asynccontextmanager
    async def lifespan(app: FastAPI) -> AsyncIterator[None]:
        cfg = settings if settings is not None else Settings()
        thresholds = alert_thresholds if alert_thresholds is not None else AlertThresholds()
        weights = evaluator_weights if evaluator_weights is not None else EvaluatorWeights()

        configure_logging(log_level=cfg.log_level, log_format=cfg.log_format)
        app.state.settings = cfg

        logger.info(
            "Starting recap-evaluator",
            host=cfg.host,
            port=cfg.port,
        )

        # --- Driver layer ---
        pool = await asyncpg.create_pool(
            dsn=cfg.recap_db_dsn,
            min_size=cfg.db_pool_min_size,
            max_size=cfg.db_pool_max_size,
            init=register_jsonb_codec,
        )
        # mTLS outbound when MTLS_ENFORCE=true (ADR-000737). The SSLContext is
        # kept live (same object) and its leaf cert is re-loaded in-place by the
        # rotation watcher whenever pki-agent updates the on-disk files, so the
        # shared httpx.AsyncClient never needs to be rebuilt.
        from recap_evaluator.infra.mtls_client import (
            SslContextReloader,
            build_ssl_context,
            watch_cert_rotation,
        )

        ssl_ctx = build_ssl_context()
        http_client = httpx.AsyncClient(
            timeout=httpx.Timeout(
                connect=5, read=cfg.ollama_timeout, write=10, pool=5
            ),
            limits=httpx.Limits(max_connections=30, max_keepalive_connections=10),
            verify=ssl_ctx if ssl_ctx is not None else True,
        )
        cert_watch_task: asyncio.Task[None] | None = None
        if ssl_ctx is not None:
            logger.info("recap-evaluator outbound: mTLS enforce enabled")
            cert_path = os.environ["MTLS_CERT_FILE"]
            key_path = os.environ["MTLS_KEY_FILE"]
            reloader = SslContextReloader(ssl_ctx, cert_path, key_path)
            cert_watch_task = asyncio.create_task(
                watch_cert_rotation(reloader, interval_seconds=30.0),
                name="mtls-cert-rotation-watch",
            )

        # --- Gateway layer ---
        db_gateway = PostgresGateway(pool)
        ollama_gateway = OllamaGateway(http_client, cfg)
        recap_worker_gw = RecapWorkerGateway(http_client, cfg)

        # --- Evaluator layer ---
        genre_eval = GenreEvaluator(recap_worker_gw, db_gateway, thresholds)
        cluster_eval = ClusterEvaluator(db_gateway, thresholds)
        summary_eval = SummaryEvaluator(
            ollama_gateway, db_gateway, cfg, thresholds, weights
        )
        pipeline_eval = PipelineEvaluator(db_gateway, thresholds)

        # --- Usecase layer ---
        run_eval_uc = RunEvaluationUsecase(
            genre_eval, cluster_eval, summary_eval, pipeline_eval, db_gateway
        )
        get_metrics_uc = GetMetricsUsecase(
            genre_eval, cluster_eval, pipeline_eval, db_gateway, thresholds
        )

        # --- Expose to handlers via app.state ---
        app.state.run_evaluation = run_eval_uc
        app.state.get_metrics = get_metrics_uc
        app.state.genre_evaluator = genre_eval
        app.state.cluster_evaluator = cluster_eval
        app.state.summary_evaluator = summary_eval
        app.state.db = db_gateway

        # --- Scheduler ---
        scheduler = EvaluationScheduler(run_eval_uc, cfg)
        scheduler.start()

        # Check Ollama health
        ollama_healthy = await ollama_gateway.health_check()
        if not ollama_healthy:
            logger.warning(
                "Ollama is not available. G-Eval summary evaluation will fail.",
                ollama_url=cfg.ollama_url,
                model=cfg.ollama_model,
            )

        logger.info("recap-evaluator started successfully")

        yield

        # --- Shutdown ---
        logger.info("Shutting down recap-evaluator")
        scheduler.stop()
        summary_eval.shutdown()
        if cert_watch_task is not None:
            cert_watch_task.cancel()
            try:
                await cert_watch_task
            except asyncio.CancelledError:
                pass
        await http_client.aclose()
        await pool.close()
        shutdown_logging()
        logger.info("recap-evaluator stopped")

    application = FastAPI(
        title="Recap Evaluator",
        description="RecapJob精度評価マイクロサービス - 7日間Recapの品質を多角的に評価",
        version="0.1.0",
        lifespan=lifespan,
    )

    # CORS — use injected settings when available; otherwise safe defaults
    # until lifespan loads production Settings (middleware is fixed at create).
    cors_origins = (
        settings.cors_allowed_origins
        if settings is not None
        else ["http://localhost:3000"]
    )
    application.add_middleware(
        CORSMiddleware,
        allow_origins=cors_origins,
        allow_credentials=True,
        allow_methods=["*"],
        allow_headers=["*"],
    )

    # peer-identity capture for mTLS audit (ADR-000737).
    from recap_evaluator.infra.peer_identity import (  # noqa: E402
        PeerIdentityMiddleware,
        allowed_peers_from_env,
    )

    application.add_middleware(
        PeerIdentityMiddleware,
        allowed=allowed_peers_from_env(),
        strict=False,
    )

    application.include_router(health_router)
    application.include_router(evaluation_router)
    application.include_router(metrics_router)

    instrument_fastapi(application)
    return application


# Import-safe: Settings are loaded in lifespan, not at module import.
app = create_app()


def run() -> None:
    """Run the application with uvicorn."""
    import uvicorn

    # Fail-fast on missing required env before binding the port.
    cfg = Settings()
    uvicorn.run(
        "recap_evaluator.main:app",
        host=cfg.host,
        port=cfg.port,
        reload=False,
    )


if __name__ == "__main__":
    run()
