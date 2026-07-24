"""Rerank server for M-series Mac (torch/MPS) and Docker (ONNX CPU int8).

Provides a REST API compatible with the rag-orchestrator's rerank client.
The backend is selected via RERANK_BACKEND:
  - "torch" (default): MPS/CUDA/CPU via sentence-transformers CrossEncoder.
    Used for the Mac deployment (see deploy.sh).
  - "onnx": CPU-only, dynamic int8-quantized (avx2) ONNX Runtime. Used by the
    Docker container; the quantized model is exported once into a mounted
    volume on first boot if it isn't already there.

Usage:
    uvicorn rerank_server:app --host 0.0.0.0 --port 8080

Requirements:
    pip install sentence-transformers fastapi uvicorn torch
    # onnx backend additionally needs: onnxruntime optimum[onnxruntime]
"""

from __future__ import annotations

import asyncio
import logging
import os
import time
from collections.abc import AsyncIterator
from contextlib import asynccontextmanager
from pathlib import Path
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

# Backend selection: "torch" preserves the existing Mac/MPS deployment;
# "onnx" is the CPU int8 path used by the Docker container.
RERANK_BACKEND = os.environ.get("RERANK_BACKEND", "torch")
RERANK_MODEL_DIR = os.environ.get("RERANK_MODEL_DIR", "/models/bge-reranker-v2-m3-onnx")
RERANK_BATCH_SIZE = int(os.environ.get("RERANK_BATCH_SIZE", "16"))
RERANK_MAX_LENGTH = int(os.environ.get("RERANK_MAX_LENGTH", "512"))

# Dynamic int8 quantization tuned for CPUs without AVX-512 (see
# sentence_transformers.backend.export_dynamic_quantized_onnx_model), saved
# by sentence-transformers under "<RERANK_MODEL_DIR>/onnx/<this file name>".
ONNX_QUANTIZED_FILE_NAME = "model_quint8_avx2.onnx"

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
        if RERANK_BACKEND == "onnx":
            return model.predict(pairs, batch_size=RERANK_BATCH_SIZE)
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


def _load_torch_model() -> CrossEncoder:
    """Load the FP16 torch CrossEncoder (MPS/CUDA/CPU)."""
    model = CrossEncoder(
        DEFAULT_MODEL,
        device=DEVICE,
        model_kwargs={"dtype": "float16"},
    )
    model.model.eval()
    for param in model.model.parameters():
        param.requires_grad = False
    return model


def _export_quantized_onnx_model(model_dir: str) -> None:
    """One-time export: fp32 CrossEncoder -> ONNX -> dynamic int8 (avx2).

    Saves the full model (config/tokenizer) plus the quantized ONNX file into
    `model_dir` so subsequent restarts can load directly without re-exporting.
    """
    from sentence_transformers.backend import export_dynamic_quantized_onnx_model

    logger.info("Exporting quantized ONNX model to %s (one-time; downloads the base model)", model_dir)
    export_model = CrossEncoder(DEFAULT_MODEL, backend="onnx")
    export_model.save_pretrained(model_dir)
    export_dynamic_quantized_onnx_model(export_model, "avx2", model_dir)
    logger.info("Quantized ONNX export complete")


def _load_onnx_model() -> CrossEncoder:
    """Load the CPU int8 ONNX CrossEncoder, exporting/quantizing it first if needed."""
    quantized_path = Path(RERANK_MODEL_DIR, "onnx", ONNX_QUANTIZED_FILE_NAME)
    if not quantized_path.exists():
        _export_quantized_onnx_model(RERANK_MODEL_DIR)
    return CrossEncoder(
        RERANK_MODEL_DIR,
        device=DEVICE,
        backend="onnx",
        model_kwargs={"file_name": ONNX_QUANTIZED_FILE_NAME, "provider": "CPUExecutionProvider"},
        max_length=RERANK_MAX_LENGTH,
    )


async def _load_model(app: FastAPI) -> None:
    """Load the model off the event loop and publish it to app.state.

    Runs as a background task (not awaited by `lifespan` before yielding) so
    the ASGI server binds its socket and /health starts answering 503
    immediately, instead of the port staying closed for as long as a slow
    first-boot ONNX export takes.
    """
    logger.info("Loading model %s on device: %s (backend=%s)", DEFAULT_MODEL, DEVICE, RERANK_BACKEND)
    loader = _load_onnx_model if RERANK_BACKEND == "onnx" else _load_torch_model
    try:
        model = await asyncio.to_thread(loader)
    except Exception:
        logger.exception("Model load failed (backend=%s); /health will stay 503", RERANK_BACKEND)
        return
    app.state.model = model
    logger.info("Model loaded successfully (backend=%s)", RERANK_BACKEND)


@asynccontextmanager
async def lifespan(app: FastAPI) -> AsyncIterator[None]:
    """Kick off model loading in the background; readiness is gated by /health."""
    app.state.model = None
    load_task = asyncio.create_task(_load_model(app))
    yield
    load_task.cancel()
    app.state.model = None


app = FastAPI(
    title="Rerank Server",
    description="Cross-encoder reranking service (torch/MPS on Mac, ONNX int8 CPU in Docker)",
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
