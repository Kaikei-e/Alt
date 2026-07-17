"""MLX-based rerank server for M-series Mac.

Runs on Apple Silicon using MPS backend for GPU acceleration.
Provides a REST API compatible with the rag-orchestrator's rerank client.

Usage:
    uvicorn rerank_server:app --host 0.0.0.0 --port 8080

Requirements:
    pip install sentence-transformers fastapi uvicorn torch
"""

from __future__ import annotations

import asyncio
import logging
import os
import time
from collections.abc import AsyncIterator
from contextlib import asynccontextmanager
from typing import Any

# Limit MPS memory cache to reduce memory pressure on shared Apple Silicon GPU memory
os.environ.setdefault("PYTORCH_MPS_HIGH_WATERMARK_RATIO", "0.0")

import torch
from fastapi import FastAPI, HTTPException, Request, Response
from pydantic import BaseModel, ConfigDict, Field, field_validator
from sentence_transformers import CrossEncoder

logging.basicConfig(level=logging.INFO, format="%(asctime)s %(levelname)s %(name)s %(message)s")
logger = logging.getLogger("rerank_server")

# Detect device: prefer MPS (Apple Silicon), fallback to CPU
if torch.backends.mps.is_available():
    DEVICE = "mps"
elif torch.cuda.is_available():
    DEVICE = "cuda"
else:
    DEVICE = "cpu"

DEFAULT_MODEL = "BAAI/bge-reranker-v2-m3"

# Bound batch size and per-candidate length so an unbounded request can't
# blow up tokenization/inference memory on the shared Apple Silicon GPU.
MAX_CANDIDATES = int(os.environ.get("RERANK_MAX_CANDIDATES", "200"))
MAX_CANDIDATE_LENGTH = int(os.environ.get("RERANK_MAX_CANDIDATE_LENGTH", "4000"))
INFERENCE_TIMEOUT_SECONDS = float(os.environ.get("RERANK_INFERENCE_TIMEOUT_SECONDS", "30"))
MAX_TOP_K = MAX_CANDIDATES

# CrossEncoder is not safe for concurrent inference on a single instance, and
# predict() is a blocking call — serialize access and run it off the event
# loop so /health and other requests stay responsive during inference.
_inference_semaphore = asyncio.Semaphore(1)


def _predict_sync(model: CrossEncoder, pairs: list[tuple[str, str]]) -> Any:
    """Run blocking CrossEncoder inference. Called via asyncio.to_thread."""
    with torch.inference_mode():
        return model.predict(pairs)


class RerankRequest(BaseModel):
    """Request body for rerank endpoint."""

    model_config = ConfigDict(strict=True, frozen=True)

    query: str = Field(..., description="The query to rank candidates against")
    candidates: list[str] = Field(
        ...,
        max_length=MAX_CANDIDATES,
        description="List of candidate texts to rank",
    )
    model: str = Field(DEFAULT_MODEL, description="Model name (must match loaded model)")
    top_k: int | None = Field(
        None,
        ge=1,
        le=MAX_TOP_K,
        description="Return only top K results",
    )

    @field_validator("candidates")
    @classmethod
    def _validate_candidate_lengths(cls, candidates: list[str]) -> list[str]:
        for candidate in candidates:
            if len(candidate) > MAX_CANDIDATE_LENGTH:
                raise ValueError(
                    f"candidate exceeds max length of {MAX_CANDIDATE_LENGTH} characters"
                )
        return candidates

    @field_validator("model")
    @classmethod
    def _validate_model(cls, model: str) -> str:
        if model != DEFAULT_MODEL:
            raise ValueError(
                f"unsupported model {model!r}; only {DEFAULT_MODEL!r} is available"
            )
        return model


class RerankResult(BaseModel):
    """A single rerank result with index and score."""

    model_config = ConfigDict(strict=True, frozen=True)

    index: int = Field(..., description="Original index of the candidate")
    score: float = Field(..., description="Relevance score")


class RerankResponse(BaseModel):
    """Response body for rerank endpoint."""

    model_config = ConfigDict(strict=True, frozen=True)

    results: list[RerankResult] = Field(..., description="Ranked results")
    model: str = Field(..., description="Model used for reranking")
    processing_time_ms: float | None = Field(None, description="Processing time in milliseconds")


class HealthResponse(BaseModel):
    """Response body for health endpoint."""

    model_config = ConfigDict(strict=True, frozen=True)

    status: str
    device: str
    model: str


class RootResponse(BaseModel):
    """Root endpoint service info."""

    model_config = ConfigDict(strict=True, frozen=True)

    service: str
    version: str
    device: str
    model: str


@asynccontextmanager
async def lifespan(app: FastAPI) -> AsyncIterator[None]:
    """Load model on startup."""
    app.state.model = None
    logger.info("Loading model %s on device: %s", DEFAULT_MODEL, DEVICE)
    model = CrossEncoder(
        DEFAULT_MODEL,
        device=DEVICE,
        model_kwargs={"dtype": "float16"},
    )
    model.model.eval()
    for param in model.model.parameters():
        param.requires_grad = False
    app.state.model = model
    logger.info("Model loaded successfully (FP16, inference-only)")
    yield
    app.state.model = None


app = FastAPI(
    title="Rerank Server",
    description="MPS-accelerated reranking service for Apple Silicon",
    version="1.0.0",
    lifespan=lifespan,
)


@app.post("/v1/rerank", response_model=RerankResponse)
async def rerank(req: RerankRequest, request: Request) -> RerankResponse:
    """Rerank candidates based on query relevance.

    Returns candidates sorted by relevance score in descending order.
    """
    model: CrossEncoder | None = request.app.state.model
    if model is None:
        raise HTTPException(status_code=503, detail="Model not loaded")

    if not req.candidates:
        return RerankResponse(results=[], model=DEFAULT_MODEL, processing_time_ms=0.0)

    start = time.perf_counter()

    # Create query-candidate pairs for cross-encoder
    pairs = [(req.query, candidate) for candidate in req.candidates]

    # Offload the blocking inference call to a worker thread so the event loop
    # (and endpoints like /health) stay responsive, and serialize access since
    # CrossEncoder is not thread-safe for concurrent predict() calls. A bound
    # is required so a hung/oversized inference can't stall requests forever.
    try:
        async with _inference_semaphore:
            async with asyncio.timeout(INFERENCE_TIMEOUT_SECONDS):
                scores = await asyncio.to_thread(_predict_sync, model, pairs)
    except TimeoutError as exc:
        raise HTTPException(status_code=504, detail="Rerank inference timed out") from exc

    # Sort by score descending, keeping track of original indices
    indexed_scores = sorted(enumerate(scores), key=lambda x: x[1], reverse=True)

    # Apply top_k limit if specified
    if req.top_k is not None and req.top_k > 0:
        indexed_scores = indexed_scores[: req.top_k]

    results = [RerankResult(index=idx, score=float(score)) for idx, score in indexed_scores]

    elapsed_ms = (time.perf_counter() - start) * 1000

    return RerankResponse(results=results, model=DEFAULT_MODEL, processing_time_ms=elapsed_ms)


@app.get("/health", response_model=HealthResponse)
async def health(request: Request, response: Response) -> HealthResponse:
    """Readiness check: 503 while the model is still loading.

    An LB/orchestrator that treats 200 as ready would otherwise route the
    first request(s) to a not-yet-loaded model before it 503s them itself.
    """
    model_loaded = request.app.state.model is not None
    if not model_loaded:
        response.status_code = 503
    return HealthResponse(
        status="ok" if model_loaded else "loading",
        device=DEVICE,
        model=DEFAULT_MODEL,
    )


@app.get("/", response_model=RootResponse)
async def root() -> RootResponse:
    """Root endpoint with service info."""
    return RootResponse(
        service="rerank-server",
        version="1.0.0",
        device=DEVICE,
        model=DEFAULT_MODEL,
    )


if __name__ == "__main__":
    import uvicorn

    uvicorn.run(app, host="0.0.0.0", port=8080)
