"""Health check handler."""

from typing import Any

from fastapi import APIRouter
from fastapi.responses import JSONResponse

from news_creator.infra.health_deep import Check, DeepHealthRunner
from news_creator.port.llm_provider_port import LLMProviderPort


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

    async def probe_ollama() -> None:
        if ollama_gateway is None:
            raise RuntimeError("unavailable")
        try:
            models = await ollama_gateway.list_models()
        except TimeoutError:
            raise
        except (RuntimeError, OSError, ConnectionError, ValueError) as exc:
            raise RuntimeError("unavailable") from exc
        if not models:
            raise RuntimeError("unavailable")

    deep_runner = DeepHealthRunner(
        "news-creator",
        [Check(name="ollama", critical=True, probe=probe_ollama)],
    )

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
    async def health_check() -> dict[str, str]:
        """Cheap liveness. No upstream I/O; /health/deep owns Ollama reachability."""
        return {"status": "healthy", "service": "news-creator"}

    @router.get("/health/deep")
    async def health_deep() -> JSONResponse:
        """Dependency reachability. Compose probes must not hit this path."""
        report = await deep_runner.run()
        return JSONResponse(status_code=report.http_status, content=report.as_dict())

    return router
