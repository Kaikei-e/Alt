"""Worker-side helpers for executing the evidence pipeline in isolation."""

from __future__ import annotations

from typing import Any, Sequence

from ..domain.models import EvidenceRequest, WarmupResponse
from ..infra.config import Settings
from .embedder import Embedder, EmbedderConfig
from .pipeline import EvidencePipeline


_PIPELINE: EvidencePipeline | None = None


def initialize(settings_payload: dict[str, Any]) -> None:
    """Initializer invoked inside worker processes to build the pipeline."""

    settings = Settings(**settings_payload)
    config = EmbedderConfig(
        model_id=settings.model_id,
        distill_model_id=settings.distill_model_id,
        backend=settings.model_backend,
        device=settings.device,
        batch_size=settings.batch_size,
        cache_size=settings.embed_cache_size,
    )
    embedder = Embedder(config)
    global _PIPELINE
    _PIPELINE = EvidencePipeline(settings=settings, embedder=embedder, process_pool=None)


def _require_pipeline() -> EvidencePipeline:
    if _PIPELINE is None:  # pragma: no cover - runtime safeguard
        raise RuntimeError("Pipeline worker not initialized")
    return _PIPELINE


def run_pipeline(payload: dict[str, Any]) -> dict[str, Any]:
    """Execute the evidence pipeline and return a JSON-serializable response."""

    pipeline = _require_pipeline()
    request = EvidenceRequest.model_validate(payload)
    response = pipeline.run(request)
    return response.model_dump(mode="json")


def warmup(samples: Sequence[str] | None = None) -> dict[str, Any]:
    """Trigger model warmup inside the worker process."""

    pipeline = _require_pipeline()
    response: WarmupResponse = pipeline.warmup(samples)
    return response.model_dump(mode="json")
