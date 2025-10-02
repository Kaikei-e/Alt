"""
News Creator Service - Refactored with Clean Architecture.

This is the main entry point that wires together all layers:
- Config: Environment-based configuration
- Domain: Models and business entities
- Port: Interfaces for external dependencies
- Gateway: Anti-Corruption Layer for external services
- Driver: HTTP clients for external APIs
- Usecase: Business logic orchestration
- Handler: REST API endpoints
"""

import logging
from contextlib import asynccontextmanager

from fastapi import FastAPI

from news_creator.config.config import NewsCreatorConfig
from news_creator.gateway.ollama_gateway import OllamaGateway
from news_creator.usecase.summarize_usecase import SummarizeUsecase
from news_creator.handler import (
    create_summarize_router,
    create_generate_router,
    health_router,
)

# Configure logging
logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)


class DependencyContainer:
    """Dependency Injection Container for News Creator Service."""

    def __init__(self):
        """Initialize all dependencies with proper layering."""
        # Config layer
        self.config = NewsCreatorConfig()

        # Gateway layer (ACL)
        self.ollama_gateway = OllamaGateway(self.config)

        # Usecase layer
        self.summarize_usecase = SummarizeUsecase(
            config=self.config,
            llm_provider=self.ollama_gateway,
        )

    async def initialize(self) -> None:
        """Initialize all async resources."""
        await self.ollama_gateway.initialize()
        logger.info("All dependencies initialized")

    async def cleanup(self) -> None:
        """Cleanup all async resources."""
        await self.ollama_gateway.cleanup()
        logger.info("All dependencies cleaned up")


# Global dependency container
container = DependencyContainer()


@asynccontextmanager
async def lifespan(app: FastAPI):
    """Application lifespan management."""
    await container.initialize()
    yield
    await container.cleanup()


# Create FastAPI application
app = FastAPI(
    title="News Creator Service",
    version="2.0.0",
    description="LLM-based content generation service with Clean Architecture",
    lifespan=lifespan,
)

# Register routers with dependency injection
app.include_router(
    create_summarize_router(container.summarize_usecase),
    tags=["summarization"]
)
app.include_router(
    create_generate_router(container.ollama_gateway),
    tags=["generation"]
)
app.include_router(
    health_router,
    tags=["health"]
)


if __name__ == "__main__":
    import uvicorn
    uvicorn.run(app, host="0.0.0.0", port=8001)
