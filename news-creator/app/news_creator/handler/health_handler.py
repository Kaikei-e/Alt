"""Health check handler."""

import logging
from fastapi import APIRouter
from typing import Dict, Any, Optional

from news_creator.port.llm_provider_port import LLMProviderPort

logger = logging.getLogger(__name__)

def create_health_router(ollama_gateway: LLMProviderPort | None = None) -> APIRouter:
    """
    Create health check router with optional Ollama gateway dependency.

    Args:
        ollama_gateway: Optional LLM provider (OllamaGateway or
            DistributingGateway) for checking model availability and queue status

    Returns:
        Configured APIRouter
    """
    router = APIRouter()

    @router.get("/queue/status")
    async def queue_status() -> dict[str, Any]:
        """
        Queue status endpoint for backpressure monitoring.

        Returns:
            Dict with queue depths, available slots, and accepting state
        """
        if ollama_gateway is not None:
            return ollama_gateway.queue_status()
        return {
            "rt_queue": 0,
            "be_queue": 0,
            "total_slots": 0,
            "available_slots": 0,
            "accepting": True,
            "max_queue_depth": 0,
        }

    @router.get("/health")
    async def health_check() -> dict[str, Any]:
        """
        Health check endpoint that includes Ollama model status.

        Returns:
            Dict with status, service name, models list, and optional error
        """
        response: dict[str, Any] = {
            "status": "healthy",
            "service": "news-creator",
            "models": [],
        }

        # If ollama_gateway is provided, check for available models
        if ollama_gateway is not None:
            try:
                models = await ollama_gateway.list_models()
                response["models"] = models
                logger.debug(f"Health check: {len(models)} models available")
            except Exception as err:
                logger.warning(f"Failed to fetch models during health check: {err}")
                response["error"] = str(err)
                # Still return healthy status - service is up even if Ollama is not ready

        return response

    return router
