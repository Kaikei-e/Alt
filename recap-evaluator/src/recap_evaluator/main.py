"""FastAPI application — Composition Root + DI wiring."""

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
from recap_evaluator.gateway.postgres_gateway import PostgresGateway
from recap_evaluator.gateway.recap_worker_gateway import RecapWorkerGateway
from recap_evaluator.handler.evaluation_handler import router as evaluation_router
from recap_evaluator.handler.health_handler import router as health_router
from recap_evaluator.handler.metrics_handler import router as metrics_router
from recap_evaluator.scheduler.evaluation_scheduler import EvaluationScheduler
from recap_evaluator.usecase.get_metrics import GetMetricsUsecase
from recap_evaluator.usecase.run_evaluation import RunEvaluationUsecase
from recap_evaluator.utils.logging import configure_logging, shutdown_logging
from recap_evaluator.utils.otel import instrument_fastapi

# Load settings eagerly — fail fast on missing required env vars
settings = Settings()
alert_thresholds = AlertThresholds()
evaluator_weights = EvaluatorWeights()

configure_logging(log_level=settings.log_level, log_format=settings.log_format)
logger = structlog.get_logger()


@asynccontextmanager
async def lifespan(app: FastAPI):
    """Application lifespan — wire all layers via DI."""
    logger.info(
        "Starting recap-evaluator",
        host=settings.host,
        port=settings.port,
    )

    # --- Driver layer ---
    pool = await asyncpg.create_pool(
        dsn=settings.recap_db_dsn,
        min_size=settings.db_pool_min_size,
        max_size=settings.db_pool_max_size,
    )
    http_client = httpx.AsyncClient(
        timeout=httpx.Timeout(
            connect=5, read=settings.ollama_timeout, write=10, pool=5
        ),
        limits=httpx.Limits(max_connections=30, max_keepalive_connections=10),
    )

    # --- Gateway layer ---
    db_gateway = PostgresGateway(pool)
    ollama_gateway = OllamaGateway(http_client, settings)
    recap_worker_gw = RecapWorkerGateway(http_client, settings)

    # --- Evaluator layer ---
    genre_eval = GenreEvaluator(recap_worker_gw, db_gateway, alert_thresholds)
    cluster_eval = ClusterEvaluator(db_gateway, alert_thresholds)
    summary_eval = SummaryEvaluator(
        ollama_gateway, db_gateway, settings, alert_thresholds, evaluator_weights
    )
    pipeline_eval = PipelineEvaluator(db_gateway, alert_thresholds)

    # --- Usecase layer ---
    run_eval_uc = RunEvaluationUsecase(
        genre_eval, cluster_eval, summary_eval, pipeline_eval, db_gateway
    )
    get_metrics_uc = GetMetricsUsecase(
        genre_eval, cluster_eval, pipeline_eval, db_gateway
    )

    # --- Expose to handlers via app.state ---
    app.state.run_evaluation = run_eval_uc
    app.state.get_metrics = get_metrics_uc
    app.state.genre_evaluator = genre_eval
    app.state.cluster_evaluator = cluster_eval
    app.state.summary_evaluator = summary_eval
    app.state.db = db_gateway

    # --- Scheduler ---
    scheduler = EvaluationScheduler(run_eval_uc, settings)
    scheduler.start()

    # Check Ollama health
    ollama_healthy = await ollama_gateway.health_check()
    if not ollama_healthy:
        logger.warning(
            "Ollama is not available. G-Eval summary evaluation will fail.",
            ollama_url=settings.ollama_url,
            model=settings.ollama_model,
        )

    logger.info("recap-evaluator started successfully")

    yield

    # --- Shutdown ---
    logger.info("Shutting down recap-evaluator")
    scheduler.stop()
    await http_client.aclose()
    await pool.close()
    shutdown_logging()
    logger.info("recap-evaluator stopped")


# Create FastAPI application
app = FastAPI(
    title="Recap Evaluator",
    description="RecapJob精度評価マイクロサービス - 7日間Recapの品質を多角的に評価",
    version="0.1.0",
    lifespan=lifespan,
)

# CORS — restricted origins
app.add_middleware(
    CORSMiddleware,
    allow_origins=settings.cors_allowed_origins,
    allow_credentials=True,
    allow_methods=["*"],
    allow_headers=["*"],
)

# Include routers
app.include_router(health_router)
app.include_router(evaluation_router)
app.include_router(metrics_router)

# Instrument with OpenTelemetry
instrument_fastapi(app)


def run() -> None:
    """Run the application with uvicorn."""
    import uvicorn

    uvicorn.run(
        "recap_evaluator.main:app",
        host=settings.host,
        port=settings.port,
        reload=False,
    )


if __name__ == "__main__":
    run()
