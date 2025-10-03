"""Health check handler."""

import logging
from fastapi import APIRouter
from typing import Dict, Any, Optional

logger = logging.getLogger(__name__)


def create_health_router(ollama_gateway: Optional[Any] = None) -> APIRouter:
    """
    Create health check router with optional Ollama gateway dependency.

    Args:
        ollama_gateway: Optional OllamaGateway instance for checking model availability

    Returns:
        Configured APIRouter
    """
    router = APIRouter()

    @router.get("/health")
    async def health_check() -> Dict[str, Any]:
        """
        Health check endpoint that includes Ollama model status.

        Returns:
            Dict with status, service name, models list, and optional error
        """
        response: Dict[str, Any] = {
            "status": "healthy",
            "service": "news-creator",
            "models": []
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


# Backward compatibility: create a default router without Ollama gateway
router = create_health_router()
