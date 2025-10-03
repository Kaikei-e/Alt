"""Handler layer - REST endpoints."""

from news_creator.handler.summarize_handler import create_summarize_router
from news_creator.handler.generate_handler import create_generate_router
from news_creator.handler.health_handler import router as health_router, create_health_router

__all__ = ["create_summarize_router", "create_generate_router", "health_router", "create_health_router"]
