"""FastAPI application entry point for recap-evaluator."""

from contextlib import asynccontextmanager

import structlog
from fastapi import FastAPI
from fastapi.middleware.cors import CORSMiddleware

from recap_evaluator.api.routes import router
from recap_evaluator.config import settings
from recap_evaluator.infra.database import db
from recap_evaluator.infra.ollama import ollama_client
from recap_evaluator.utils.logging import configure_logging, shutdown_logging
from recap_evaluator.utils.otel import instrument_fastapi

# Configure logging on module load
configure_logging()
logger = structlog.get_logger()


@asynccontextmanager
async def lifespan(app: FastAPI):
    """Application lifespan manager."""
    # Startup
    logger.info(
        "Starting recap-evaluator",
        host=settings.host,
        port=settings.port,
        log_level=settings.log_level,
    )

    # Connect to database
    await db.connect()

    # Check Ollama health
    ollama_healthy = await ollama_client.health_check()
    if not ollama_healthy:
        logger.warning(
            "Ollama is not available. G-Eval summary evaluation will fail.",
            ollama_url=settings.ollama_url,
            model=settings.ollama_model,
        )

    logger.info("recap-evaluator started successfully")

    yield

    # Shutdown
    logger.info("Shutting down recap-evaluator")
    await db.disconnect()
    shutdown_logging()
    logger.info("recap-evaluator stopped")


# Create FastAPI application
app = FastAPI(
    title="Recap Evaluator",
    description="RecapJob精度評価マイクロサービス - 7日間Recapの品質を多角的に評価",
    version="0.1.0",
    lifespan=lifespan,
)

# Add CORS middleware
app.add_middleware(
    CORSMiddleware,
    allow_origins=["*"],
    allow_credentials=True,
    allow_methods=["*"],
    allow_headers=["*"],
)

# Include API routes
app.include_router(router)

# Instrument FastAPI with OpenTelemetry
instrument_fastapi(app)


@app.get("/health")
async def health_check() -> dict:
    """Health check endpoint."""
    return {
        "status": "healthy",
        "service": "recap-evaluator",
        "version": "0.1.0",
    }


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
