"""Cross-encoder rerank server (torch/MPS for local dev, ONNX CPU int8 in Docker).

Provides a REST API compatible with the rag-orchestrator's rerank client.
The backend is selected via RERANK_BACKEND:
  - "torch" (default): MPS/CUDA/CPU via sentence-transformers CrossEncoder.
  - "onnx": CPU-only, dynamic int8-quantized (avx2) ONNX Runtime. Used by the
    Docker container; the quantized model is exported once into a mounted
    volume on first boot if it isn't already there.

The served model is chosen with RERANK_MODEL (see SUPPORTED_MODELS).

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
import re
import shutil
import tempfile
import time
from collections.abc import AsyncIterator, Iterator, Sequence
from concurrent.futures import ThreadPoolExecutor
from contextlib import asynccontextmanager, contextmanager
from pathlib import Path

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

DEFAULT_MODEL = "cl-nagoya/ruri-v3-reranker-310m"

# Rerankers whose ONNX export path was checked against the pinned toolchain
# (optimum-onnx 0.1.0 / transformers 4.57.6). The XLM-RoBERTa pair needs no
# change at all; ModernBERT works because optimum-onnx registers
# ModernBertOnnxConfig for text-classification with MIN_TRANSFORMERS_VERSION
# 4.48.0, which the pinned 4.57.6 satisfies.
#
# RERANK_MODEL is not restricted to this table -- an unlisted model only loses
# the startup assurance, and the startup log says so.
SUPPORTED_MODELS: dict[str, str] = {
    "BAAI/bge-reranker-v2-m3": "XLM-RoBERTa, Apache-2.0, JQaRA 0.673",
    "hotchpotch/japanese-bge-reranker-v2-m3-v1": "XLM-RoBERTa, MIT, JQaRA 0.6918",
    "hotchpotch/japanese-reranker-base-v2": "ModernBERT, MIT, JQaRA 0.7845",
    "hotchpotch/japanese-reranker-xsmall-v2": "ModernBERT, MIT, JQaRA 0.7845, fastest",
    # Highest JQaRA of the four. Publishes no onnx/ folder, so first boot always
    # runs the local export; the community conversions on the Hub have
    # single-digit download counts and are not the supported route.
    "cl-nagoya/ruri-v3-reranker-310m": "ModernBERT, Apache-2.0, JQaRA 0.8688, 8192 ctx",
}

# The character class keeps a repo id from escaping RERANK_MODEL_CACHE_ROOT
# when the cache dir is derived from it below.
_HF_REPO_ID_PATTERN = re.compile(r"^[A-Za-z0-9][A-Za-z0-9._-]*/[A-Za-z0-9][A-Za-z0-9._-]*$")


def _resolve_model() -> str:
    """Read RERANK_MODEL, rejecting anything that is not a plain HF repo id."""
    model = os.environ.get("RERANK_MODEL", DEFAULT_MODEL).strip()
    if not _HF_REPO_ID_PATTERN.match(model):
        raise ValueError(
            f"RERANK_MODEL={model!r} is not a Hugging Face repo id of the form 'org/name'"
        )
    return model


def _resolve_model_dir(model: str) -> str:
    """Where the exported/quantized ONNX copy of `model` lives.

    Derived from the model name by default so that flipping RERANK_MODEL cannot
    silently serve the previous model's cached export out of the same volume.
    """
    explicit = os.environ.get("RERANK_MODEL_DIR")
    if explicit:
        return explicit
    cache_root = os.environ.get("RERANK_MODEL_CACHE_ROOT", "/models")
    return str(Path(cache_root, f"{model.split('/')[-1]}-onnx"))


# Backend selection: "torch" preserves the MPS/CUDA path used for local
# experiments; "onnx" is the CPU int8 path used by the Docker container.
RERANK_BACKEND = os.environ.get("RERANK_BACKEND", "torch")
RERANK_MODEL = _resolve_model()
RERANK_MODEL_DIR = _resolve_model_dir(RERANK_MODEL)
RERANK_BATCH_SIZE = int(os.environ.get("RERANK_BATCH_SIZE", "16"))
RERANK_MAX_LENGTH = int(os.environ.get("RERANK_MAX_LENGTH", "512"))

# Pairs handed to a single predict() call. Defaults to one batch, which is what
# makes the deadline check in _predict_sync fire between batches instead of only
# once per request.
RERANK_CHUNK_SIZE = int(os.environ.get("RERANK_CHUNK_SIZE", str(RERANK_BATCH_SIZE)))

# Dynamic int8 quantization tuned for CPUs without AVX-512 (see
# sentence_transformers.backend.export_dynamic_quantized_onnx_model), saved
# by sentence-transformers under "<RERANK_MODEL_DIR>/onnx/<this file name>".
ONNX_QUANTIZED_FILE_NAME = "model_quint8_avx2.onnx"

# Total server-side budget for one /v1/rerank call, covering the wait for the
# inference lock as well as inference itself. Keep it BELOW rag-orchestrator's
# RERANK_TIMEOUT (compose/rag.yaml: 12s) so the server gives up first and the
# client sees a 504 instead of a dead connection -- otherwise the server keeps
# burning CPU on a result nobody will read.
SERVER_TIMEOUT_SECONDS = float(os.environ.get("RERANK_SERVER_TIMEOUT", "10"))

# Per-candidate inference cost, used to derive the early-reject bound below.
# ADR-000951 measured two very different rates on this same int8 CPU path:
#   ~52ms/candidate   Ask Augur production traffic (10 candidates ~520ms)
#   ~250ms/candidate  synthetic ~500-token passages, which saturate
#                     RERANK_MAX_LENGTH (15 candidates, p95 3.8s)
# The bound uses the long-passage rate, because MAX_CANDIDATE_LENGTH lets any
# request be the long-passage case. ADR-000951 also records 30 long passages at
# 14-17s, i.e. cost grows faster than linearly once past one batch; the
# per-chunk deadline check in _predict_sync is what catches that tail, not this
# static bound.
CANDIDATE_BUDGET_MS = float(os.environ.get("RERANK_CANDIDATE_BUDGET_MS", "250"))


def _default_max_candidates() -> int:
    """Largest candidate list that fits the server budget at the long-passage rate."""
    return max(1, int(SERVER_TIMEOUT_SECONDS * 1000 / CANDIDATE_BUDGET_MS))


# Bound the candidate list and per-candidate length so an oversized request is
# rejected up front rather than timing out halfway through. Derived rather than
# hard-coded so that raising RERANK_SERVER_TIMEOUT cannot silently leave the two
# knobs contradicting each other.
MAX_CANDIDATES = int(os.environ.get("RERANK_MAX_CANDIDATES", str(_default_max_candidates())))
MAX_CANDIDATE_LENGTH = int(os.environ.get("RERANK_MAX_CANDIDATE_LENGTH", "4000"))
MAX_TOP_K = MAX_CANDIDATES

# CrossEncoder is not safe for concurrent inference on a single instance, and
# predict() is a blocking call — serialize access and run it off the event
# loop so /health and other requests stay responsive during inference.
_inference_semaphore = asyncio.Semaphore(1)

# Dedicated single-worker executor for inference. asyncio.timeout() only
# cancels the *awaiting* coroutine; it cannot stop a thread that's already
# running (concurrent.futures.Future.cancel() is a no-op once execution has
# started). With asyncio.to_thread's shared default executor, a timed-out
# request releases _inference_semaphore while its thread keeps calling
# model.predict() in the background, letting the next request's predict()
# call run concurrently on the same non-thread-safe CrossEncoder instance.
# Routing every inference job through this single-worker executor instead
# guarantees at most one predict() call runs at a time regardless: an
# orphaned job just keeps the next one queued behind it.
_inference_executor = ThreadPoolExecutor(max_workers=1, thread_name_prefix="rerank-inference")


class InferenceDeadlineExceeded(RuntimeError):
    """The request's deadline passed before the remaining chunks could be scored."""


def _predict_sync(
    model: CrossEncoder, pairs: list[tuple[str, str]], deadline: float
) -> list[float]:
    """Score `pairs` in chunks, abandoning the work once `deadline` has passed.

    Chunking buys two things over one big predict() call: the deadline is
    re-checked between chunks, so an abandoned request stops burning CPU
    instead of running to completion in an orphaned thread; and pairs are
    ordered longest-first across the whole request, so padding waste stays
    inside a chunk rather than being spread over every batch.

    `deadline` is a time.monotonic() timestamp.
    """
    order = sorted(range(len(pairs)), key=lambda i: len(pairs[i][1]), reverse=True)
    scores = [0.0] * len(pairs)

    with torch.inference_mode():
        for start in range(0, len(order), RERANK_CHUNK_SIZE):
            if time.monotonic() >= deadline:
                raise InferenceDeadlineExceeded(
                    f"deadline passed after {start}/{len(order)} candidates"
                )
            indices = order[start : start + RERANK_CHUNK_SIZE]
            chunk = [pairs[i] for i in indices]
            if RERANK_BACKEND == "onnx":
                chunk_scores = model.predict(chunk, batch_size=RERANK_BATCH_SIZE)
            else:
                chunk_scores = model.predict(chunk)
            for i, score in zip(indices, chunk_scores, strict=True):
                scores[i] = float(score)

    return scores


class RerankRequest(BaseModel):
    """Request body for rerank endpoint."""

    model_config = ConfigDict(strict=True, frozen=True)

    query: str = Field(..., description="The query to rank candidates against")
    candidates: list[str] = Field(..., description="List of candidate texts to rank")
    model: str | None = Field(
        None, description="Model name; must match the server's RERANK_MODEL when given"
    )
    top_k: int | None = Field(
        None,
        ge=1,
        le=MAX_TOP_K,
        description="Return only top K results",
    )

    @field_validator("candidates")
    @classmethod
    def _validate_candidates(cls, candidates: list[str]) -> list[str]:
        if len(candidates) > MAX_CANDIDATES:
            raise ValueError(
                f"too many candidates: {len(candidates)} exceeds "
                f"RERANK_MAX_CANDIDATES={MAX_CANDIDATES}; a larger list cannot be "
                f"scored within RERANK_SERVER_TIMEOUT={SERVER_TIMEOUT_SECONDS}s"
            )
        for candidate in candidates:
            if len(candidate) > MAX_CANDIDATE_LENGTH:
                raise ValueError(
                    f"candidate exceeds max length of {MAX_CANDIDATE_LENGTH} characters"
                )
        return candidates

    @field_validator("model")
    @classmethod
    def _validate_model(cls, model: str | None) -> str | None:
        if model is not None and model != RERANK_MODEL:
            raise ValueError(
                f"unsupported model {model!r}; this server serves {RERANK_MODEL!r} "
                f"(set via RERANK_MODEL)"
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
    """Load the FP16 torch CrossEncoder (MPS/CUDA/CPU).

    Local-experiment path only. The fp16 logits here are the configuration that
    sentence-transformers 6.0.0 upcasts to float32 before the sigmoid, because
    saturating fp16 collapses close scores onto 1.0 and flattens the ranking.
    That fix is out of reach while optimum-onnx caps transformers below 4.58, so
    prefer the onnx backend for anything ranking-sensitive.
    """
    model = CrossEncoder(
        RERANK_MODEL,
        device=DEVICE,
        model_kwargs={"dtype": "float16"},
    )
    model.model.eval()
    for param in model.model.parameters():
        param.requires_grad = False
    return model


def _export_tmp_root() -> Path:
    """Scratch dir for ONNX Runtime's `ort.quant.*` trees during first-boot export."""
    return Path(RERANK_MODEL_DIR) / ".export-tmp"


def _reset_export_tmp() -> Path:
    """Delete and recreate the export scratch so a killed first-boot cannot accumulate.

    `export_dynamic_quantized_onnx_model` writes ~1.2GiB into tempfile.gettempdir().
    SIGKILL skips TemporaryDirectory cleanup; Docker `restart: unless-stopped` keeps
    the same writable layer, so each retry left another copy under /tmp (observed:
    ~180 trees / 221GiB in overlay2). Keep scratch on the /models volume and wipe
    it before every export and after success or a catchable failure.
    """
    root = _export_tmp_root()
    if root.exists():
        shutil.rmtree(root, ignore_errors=True)
    root.mkdir(parents=True, exist_ok=True)
    return root


def _purge_stray_ort_temp_dirs() -> None:
    """Remove leftover `ort.quant.*` dirs from the process temp dir (pre-fix overlay)."""
    tmp = Path(tempfile.gettempdir())
    for path in tmp.glob("ort.quant.*"):
        if path.is_dir():
            shutil.rmtree(path, ignore_errors=True)


@contextmanager
def _onnx_export_tmpdir() -> Iterator[None]:
    """Point TMPDIR at the wiped volume scratch for the duration of one export."""
    root = _reset_export_tmp()
    previous = os.environ.get("TMPDIR")
    os.environ["TMPDIR"] = str(root)
    try:
        yield
    finally:
        if previous is None:
            os.environ.pop("TMPDIR", None)
        else:
            os.environ["TMPDIR"] = previous
        _reset_export_tmp()


def _export_quantized_onnx_model(model_dir: str) -> None:
    """One-time export: fp32 CrossEncoder -> ONNX -> dynamic int8 (avx2).

    Saves the full model (config/tokenizer) plus the quantized ONNX file into
    `model_dir` so subsequent restarts can load directly without re-exporting.
    """
    from sentence_transformers.backend import export_dynamic_quantized_onnx_model

    logger.info(
        "Exporting quantized ONNX model %s to %s (one-time; downloads the base model)",
        RERANK_MODEL,
        model_dir,
    )
    with _onnx_export_tmpdir():
        export_model = CrossEncoder(RERANK_MODEL, backend="onnx")
        export_model.save_pretrained(model_dir)
        export_dynamic_quantized_onnx_model(export_model, "avx2", model_dir)
    logger.info("Quantized ONNX export complete")


def _load_onnx_model() -> CrossEncoder:
    """Load the CPU int8 ONNX CrossEncoder, exporting/quantizing it first if needed.

    No dtype is passed: dynamic quantization stores int8 *weights* but keeps
    activations and output logits in fp32, so the sigmoid sees fp32 here.
    """
    _purge_stray_ort_temp_dirs()
    _reset_export_tmp()
    quantized_path = Path(RERANK_MODEL_DIR, "onnx", ONNX_QUANTIZED_FILE_NAME)
    if not quantized_path.is_file():
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
    logger.info(
        "rerank_model_selected model=%s known_good=%s notes=%s backend=%s device=%s dir=%s",
        RERANK_MODEL,
        RERANK_MODEL in SUPPORTED_MODELS,
        SUPPORTED_MODELS.get(RERANK_MODEL, "unverified export path"),
        RERANK_BACKEND,
        DEVICE,
        RERANK_MODEL_DIR,
    )
    loader = _load_onnx_model if RERANK_BACKEND == "onnx" else _load_torch_model
    try:
        model = await asyncio.to_thread(loader)
    except (OSError, ValueError, RuntimeError, ImportError):
        logger.exception(
            "rerank_model_load_failed model=%s backend=%s (health stays 503)",
            RERANK_MODEL,
            RERANK_BACKEND,
        )
        return
    app.state.model = model
    logger.info("rerank_model_loaded model=%s backend=%s", RERANK_MODEL, RERANK_BACKEND)


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


def _chunk_count(candidate_count: int) -> int:
    return -(-candidate_count // RERANK_CHUNK_SIZE)


@app.post("/v1/rerank", response_model=RerankResponse)
async def rerank(req: RerankRequest, request: Request) -> RerankResponse:
    """Rerank candidates based on query relevance.

    Returns candidates sorted by relevance score in descending order.
    """
    model: CrossEncoder | None = request.app.state.model
    if model is None:
        raise HTTPException(status_code=503, detail="Model not loaded")

    if not req.candidates:
        return RerankResponse(results=[], model=RERANK_MODEL, processing_time_ms=0.0)

    start = time.perf_counter()
    deadline = time.monotonic() + SERVER_TIMEOUT_SECONDS

    pairs = [(req.query, candidate) for candidate in req.candidates]

    # The timeout wraps the lock acquisition too: a request queued behind a
    # long inference has already spent part of the client's budget, and
    # starting a fresh full-length inference at that point produces a result
    # the client has stopped waiting for.
    try:
        async with asyncio.timeout(SERVER_TIMEOUT_SECONDS):
            async with _inference_semaphore:
                loop = asyncio.get_running_loop()
                scores = await loop.run_in_executor(
                    _inference_executor, _predict_sync, model, pairs, deadline
                )
    except (TimeoutError, InferenceDeadlineExceeded) as exc:
        logger.warning(
            "rerank_timeout model=%s candidates=%d chunks=%d batch_size=%d "
            "elapsed_ms=%.1f budget_ms=%.1f",
            RERANK_MODEL,
            len(req.candidates),
            _chunk_count(len(req.candidates)),
            RERANK_BATCH_SIZE,
            (time.perf_counter() - start) * 1000,
            SERVER_TIMEOUT_SECONDS * 1000,
        )
        raise HTTPException(status_code=504, detail="Rerank inference timed out") from exc

    results = _rank(scores, req.top_k)
    elapsed_ms = (time.perf_counter() - start) * 1000

    logger.info(
        "rerank_completed model=%s candidates=%d returned=%d chunks=%d batch_size=%d "
        "elapsed_ms=%.1f budget_ms=%.1f",
        RERANK_MODEL,
        len(req.candidates),
        len(results),
        _chunk_count(len(req.candidates)),
        RERANK_BATCH_SIZE,
        elapsed_ms,
        SERVER_TIMEOUT_SECONDS * 1000,
    )

    return RerankResponse(results=results, model=RERANK_MODEL, processing_time_ms=elapsed_ms)


def _rank(scores: Sequence[float], top_k: int | None) -> list[RerankResult]:
    """Sort scores descending, keeping original indices, then apply top_k."""
    indexed_scores = sorted(enumerate(scores), key=lambda pair: pair[1], reverse=True)
    if top_k is not None and top_k > 0:
        indexed_scores = indexed_scores[:top_k]
    return [RerankResult(index=idx, score=float(score)) for idx, score in indexed_scores]


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
        model=RERANK_MODEL,
    )


@app.get("/", response_model=RootResponse)
async def root() -> RootResponse:
    """Root endpoint with service info."""
    return RootResponse(
        service="rerank-server",
        version="1.0.0",
        device=DEVICE,
        model=RERANK_MODEL,
    )


if __name__ == "__main__":
    import uvicorn

    uvicorn.run(app, host="0.0.0.0", port=8080)
