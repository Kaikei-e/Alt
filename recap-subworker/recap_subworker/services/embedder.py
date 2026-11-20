"""Sentence embedding utilities."""

from __future__ import annotations

import contextlib
import hashlib
import math
import os
import time
from collections.abc import Iterable, Sequence
from dataclasses import dataclass, field
from pathlib import Path
from threading import Lock
from typing import TYPE_CHECKING, Literal, Protocol

import httpx
import numpy as np
import structlog

from ..infra.cache import LRUCache
from ..infra.embedding_identity import (
    DEFAULT_OLLAMA_EMBED_MODEL,
    DEFAULT_SENTENCE_TRANSFORMER_MODEL_ID,
    canonicalize_embedding_id,
)

if TYPE_CHECKING:
    from pydantic import SecretStr

    from ..infra.config import Settings

logger = structlog.get_logger(__name__)

# Optional ONNX imports
try:
    import onnxruntime as ort

    ONNX_AVAILABLE = True
except ImportError:
    ONNX_AVAILABLE = False
    ort = None  # type: ignore[assignment, unused-ignore]

try:
    from transformers import AutoTokenizer

    TRANSFORMERS_AVAILABLE = True
except ImportError:
    TRANSFORMERS_AVAILABLE = False
    AutoTokenizer = None  # type: ignore[assignment, unused-ignore]


BackendLiteral = Literal["sentence-transformers", "onnx", "hash", "ollama-remote"]


@dataclass(slots=True)
class EmbedderConfig:
    model_id: str
    distill_model_id: str
    backend: BackendLiteral
    device: str
    batch_size: int
    cache_size: int
    onnx_model_path: str | None = None
    onnx_tokenizer_name: str | None = None
    onnx_pooling: Literal["cls", "mean"] = "mean"
    onnx_max_length: int = 512
    # Ollama remote settings
    ollama_embed_url: str | None = None
    ollama_embed_model: str = DEFAULT_OLLAMA_EMBED_MODEL
    # ADR-890 followup: bge-m3 (8192 token capacity) と cold model load を
    # 見越して、Ollama 公式ガイドラインの ≥60s を上回る値を既定にする。
    # 2026-05-12 17:00 UTC の事故では 30s 既定で全 chunk が timeout し、
    # classification 0 results → 3-day Recap batch fail の root cause になった。
    ollama_embed_timeout: float = 120.0
    # Operator escape hatch propagated from Settings.allow_embedding_drift —
    # when True, sidecar↔runtime embedder identity drift downgrades to a
    # warning instead of ConfigValidationError. See classifier.py.
    allow_embedding_drift: bool = False
    inference_service_token: SecretStr | None = field(kw_only=True)

    @classmethod
    def from_settings(cls, settings: Settings) -> EmbedderConfig:
        """Build the config every Embedder construction site shares.

        Constructing EmbedderConfig field by field at each site let the
        process-pool workers drift from the container (no bearer token, no
        ONNX options), so all sites derive it from Settings here.
        """
        return cls(
            model_id=settings.model_id,
            distill_model_id=settings.distill_model_id,
            backend=settings.model_backend,
            device=settings.device,
            batch_size=settings.batch_size,
            cache_size=settings.embed_cache_size,
            onnx_model_path=settings.onnx_model_path,
            onnx_tokenizer_name=settings.onnx_tokenizer_name,
            onnx_pooling=settings.onnx_pooling,
            onnx_max_length=settings.onnx_max_length,
            ollama_embed_url=settings.ollama_embed_url,
            ollama_embed_model=settings.ollama_embed_model,
            ollama_embed_timeout=settings.ollama_embed_timeout,
            allow_embedding_drift=settings.allow_embedding_drift,
            inference_service_token=settings.inference_service_token,
        )


class _EmbeddingBackend(Protocol):
    """Structural interface shared by SentenceTransformer, ONNX, and Ollama adapters."""

    def encode(
        self,
        sentences: Sequence[str],
        batch_size: int,
        normalize_embeddings: bool = True,
        show_progress_bar: bool = False,
    ) -> np.ndarray: ...


class _OnnxOutput(Protocol):
    """Minimal ONNX Runtime output metadata protocol."""

    @property
    def shape(self) -> Sequence[int | str]: ...


class _OnnxSession(Protocol):
    """Minimal ONNX Runtime InferenceSession protocol."""

    def run(
        self, output_names: list[str] | None, input_feed: dict[str, np.ndarray]
    ) -> Sequence[np.ndarray]: ...

    def get_outputs(self) -> Sequence[_OnnxOutput]: ...


class _OnnxTokenizer(Protocol):
    """Minimal HuggingFace tokenizer protocol for ONNX inference."""

    def __call__(
        self,
        text: list[str],
        *,
        padding: bool,
        truncation: bool,
        max_length: int,
        return_tensors: str,
    ) -> dict[str, np.ndarray]: ...


class _OnnxModelAdapter:
    """Adapter exposing `.encode()` over an ONNX Runtime session and tokenizer."""

    def __init__(
        self,
        session: _OnnxSession,
        tokenizer: _OnnxTokenizer,
        pooling: Literal["cls", "mean"],
        max_length: int,
    ) -> None:
        self.session = session
        self.tokenizer = tokenizer
        self.pooling = pooling
        self.max_length = max_length

    def encode(
        self,
        sentences: Sequence[str],
        batch_size: int,
        normalize_embeddings: bool = True,
        show_progress_bar: bool = False,
    ) -> np.ndarray:
        embeddings_list: list[np.ndarray] = []

        for i in range(0, len(sentences), batch_size):
            batch = sentences[i : i + batch_size]

            # Tokenize
            tokens = self.tokenizer(
                list(batch),
                padding=True,
                truncation=True,
                max_length=self.max_length,
                return_tensors="np",
            )

            # Run inference
            ort_inputs = dict(tokens.items())
            hidden_states = self.session.run(None, ort_inputs)[0]

            # Pooling
            emb = hidden_states.mean(axis=1) if self.pooling == "mean" else hidden_states[:, 0, :]

            # Normalize
            if normalize_embeddings:
                norms = np.linalg.norm(emb, axis=1, keepdims=True)
                norms = np.where(norms == 0, 1.0, norms)
                emb = emb / norms

            embeddings_list.append(emb.astype(np.float32))

        if not embeddings_list:
            embedding_dim = int(self.session.get_outputs()[0].shape[-1])
            return np.zeros((0, embedding_dim), dtype=np.float32)

        return np.vstack(embeddings_list)


class _OllamaRemoteAdapter:
    """Adapter for Ollama /api/embed endpoint."""

    # Maximum characters per chunk for embedding
    # mxbai-embed-large has 512 token context
    # Japanese text uses ~1-3 tokens per character (more than English)
    # Using conservative 400 chars to ensure texts fit (512 / 1.3 ≈ 400)
    MAX_CHUNK_CHARS = 400

    # ADR-890 followup: bge-m3 endpoint への transient timeout/connect error を
    # 吸収する exponential backoff retry。3 試行 (initial + 2 retry, sleep 1s/2s)。
    # HTTPStatusError (4xx/5xx) は retry しない (内容由来のエラーなので逆効果)。
    _RETRY_ATTEMPTS = 3
    _RETRY_BACKOFFS = (1.0, 2.0)

    def __init__(self, url: str, model: str, timeout: float, token: str | None = None) -> None:
        self.url = url.rstrip("/")
        self.model = model
        self.timeout = timeout
        headers = {}
        if token:
            headers["Authorization"] = f"Bearer {token}"
        self._client = httpx.Client(timeout=timeout, headers=headers, follow_redirects=False)
        self._embedding_dim: int | None = None

    def _call_embed_api(self, texts: list[str]) -> list[list[float]]:
        """Call Ollama /api/embed endpoint for a batch of short texts."""
        last_exc: httpx.RequestError | None = None
        for attempt in range(self._RETRY_ATTEMPTS):
            try:
                response = self._client.post(
                    f"{self.url}/api/embed",
                    json={"model": self.model, "input": texts},
                )
                response.raise_for_status()
                data = response.json()
                return data["embeddings"]
            except httpx.HTTPStatusError as exc:
                # 4xx/5xx はリクエスト/モデル由来。retry しても直らない。
                msg = f"Ollama API error: {exc.response.status_code} - {exc.response.text[:200]}"
                raise RuntimeError(msg) from exc
            except httpx.RequestError as exc:
                last_exc = exc
                if attempt + 1 >= self._RETRY_ATTEMPTS:
                    break
                backoff = self._RETRY_BACKOFFS[attempt]
                logger.warning(
                    "Ollama remote embed API transient failure, retrying",
                    attempt=attempt + 1,
                    max_attempts=self._RETRY_ATTEMPTS,
                    backoff_seconds=backoff,
                    error=str(exc),
                )
                time.sleep(backoff)
        msg = f"Ollama API request failed: {last_exc}"
        raise RuntimeError(msg) from last_exc

    def _embed_single_text(self, text: str) -> list[float]:
        """Embed a single text, chunking if necessary."""
        if len(text) <= self.MAX_CHUNK_CHARS:
            # Short text: embed directly
            embeddings = self._call_embed_api([text])
            return embeddings[0]

        # Long text: split into chunks and average embeddings
        chunks = []
        for i in range(0, len(text), self.MAX_CHUNK_CHARS):
            chunk = text[i : i + self.MAX_CHUNK_CHARS]
            if chunk.strip():
                chunks.append(chunk)

        if not chunks:
            chunks = [text[: self.MAX_CHUNK_CHARS]]

        logger.debug(
            "Chunking long text for embedding",
            original_length=len(text),
            chunk_count=len(chunks),
        )

        # Embed each chunk separately to avoid batch context length issues
        chunk_embeddings = []
        for chunk in chunks:
            emb = self._call_embed_api([chunk])[0]
            chunk_embeddings.append(emb)

        # Average the chunk embeddings
        return np.mean(chunk_embeddings, axis=0).tolist()

    def _get_embeddings(self, texts: list[str]) -> list[list[float]]:
        """Get embeddings for multiple texts with chunking support.

        Short texts (<= MAX_CHUNK_CHARS) are batched into a single
        /api/embed call; long texts still go through per-text chunking.
        """
        results: list[list[float] | None] = [None] * len(texts)
        short_indices: list[int] = []
        short_texts: list[str] = []

        for i, text in enumerate(texts):
            if len(text) <= self.MAX_CHUNK_CHARS:
                short_indices.append(i)
                short_texts.append(text)
            else:
                results[i] = self._embed_single_text(text)

        if short_texts:
            embeddings = self._call_embed_api(short_texts)
            for idx, emb in zip(short_indices, embeddings, strict=True):
                results[idx] = emb

        return [emb for emb in results if emb is not None]

    def encode(
        self,
        sentences: Sequence[str],
        batch_size: int,
        normalize_embeddings: bool = True,
        show_progress_bar: bool = False,
    ) -> np.ndarray:
        if not sentences:
            return np.zeros((0, self._embedding_dim or 1024), dtype=np.float32)

        all_embeddings: list[np.ndarray] = []

        for i in range(0, len(sentences), batch_size):
            batch = list(sentences[i : i + batch_size])
            embeddings = self._get_embeddings(batch)
            batch_array = np.array(embeddings, dtype=np.float32)

            if self._embedding_dim is None and batch_array.shape[1] > 0:
                self._embedding_dim = batch_array.shape[1]

            if normalize_embeddings:
                norms = np.linalg.norm(batch_array, axis=1, keepdims=True)
                norms = np.where(norms == 0, 1.0, norms)
                batch_array = batch_array / norms

            all_embeddings.append(batch_array)

        if not all_embeddings:
            return np.zeros((0, self._embedding_dim or 1024), dtype=np.float32)

        return np.vstack(all_embeddings)

    def close(self) -> None:
        self._client.close()


class Embedder:
    """Embedding facade supporting multiple backends."""

    def __init__(self, config: EmbedderConfig) -> None:
        self.config = config
        self._model: _EmbeddingBackend | None = None
        self._cache = LRUCache[str, np.ndarray](config.cache_size)
        self._hash_dimension = 256
        self._lock = Lock()

    def _load_sentence_transformer(self) -> _EmbeddingBackend:
        from sentence_transformers import (  # noqa: PLC0415 - heavy ML import deferred until worker loads SentenceTransformer
            SentenceTransformer,
        )

        logger.info(
            "Loading SentenceTransformer",
            model_id=self.config.model_id,
            backend=self.config.backend,
        )

        if canonicalize_embedding_id(self.config.model_id) != canonicalize_embedding_id(
            DEFAULT_SENTENCE_TRANSFORMER_MODEL_ID
        ):
            logger.warning(
                "Model ID mismatch recommendation",
                current=self.config.model_id,
                recommended=DEFAULT_SENTENCE_TRANSFORMER_MODEL_ID,
            )

        model_kwargs = {
            "low_cpu_mem_usage": False,
            "trust_remote_code": False,
        }

        if self.config.device.startswith("cuda"):
            import torch  # noqa: PLC0415 - heavy torch import deferred until CUDA SentenceTransformer load

            logger.info("Enabling FP16 for CUDA device")
            model_kwargs["torch_dtype"] = torch.float16

        logger.info(
            "Initializing SentenceTransformer model (this may take time for large models)..."
        )
        model = SentenceTransformer(
            self.config.model_id,
            device=self.config.device,
            model_kwargs=model_kwargs,
        )
        logger.info("SentenceTransformer model initialized", device=self.config.device)
        return model

    def _build_onnx_session_options(self) -> object:
        """Build ONNX Runtime SessionOptions from environment configuration."""
        if ort is None:
            msg = "onnxruntime is required for ONNX backend. Install with: pip install onnxruntime"
            raise ImportError(msg)
        sess_options = ort.SessionOptions()
        sess_options.enable_mem_pattern = True
        sess_options.enable_cpu_mem_arena = True

        graph_opt_level = os.getenv("RECAP_ONNX_GRAPH_OPT_LEVEL", "ORT_ENABLE_ALL")
        try:
            sess_options.graph_optimization_level = getattr(
                ort.GraphOptimizationLevel, graph_opt_level
            )
        except AttributeError:
            logger.warning(
                "Invalid RECAP_ONNX_GRAPH_OPT_LEVEL, defaulting to ORT_ENABLE_ALL",
                requested=graph_opt_level,
            )
            sess_options.graph_optimization_level = ort.GraphOptimizationLevel.ORT_ENABLE_ALL

        intra_threads = os.getenv("RECAP_ONNX_INTRA_OP_THREADS")
        inter_threads = os.getenv("RECAP_ONNX_INTER_OP_THREADS")
        if intra_threads and intra_threads.isdigit():
            sess_options.intra_op_num_threads = int(intra_threads)
        if inter_threads and inter_threads.isdigit():
            sess_options.inter_op_num_threads = int(inter_threads)

        execution_mode = os.getenv("RECAP_ONNX_EXECUTION_MODE", "ORT_SEQUENTIAL")
        if execution_mode.upper() == "ORT_PARALLEL":
            sess_options.execution_mode = ort.ExecutionMode.ORT_PARALLEL
        else:
            sess_options.execution_mode = ort.ExecutionMode.ORT_SEQUENTIAL

        return sess_options

    @staticmethod
    def _resolve_onnx_providers() -> list[str]:
        """Resolve ONNX execution providers from environment and runtime availability."""
        if ort is None:
            msg = "onnxruntime is required for ONNX backend. Install with: pip install onnxruntime"
            raise ImportError(msg)
        configured_providers = os.getenv("RECAP_ONNX_RUNTIME_PROVIDERS")
        available = set(ort.get_available_providers())

        if not configured_providers:
            return (
                ["CPUExecutionProvider"] if "CPUExecutionProvider" in available else list(available)
            )

        requested = [p.strip() for p in configured_providers.split(",") if p.strip()]
        providers = [p for p in requested if p in available]
        if not providers:
            logger.warning(
                "Requested ONNX providers unavailable, falling back to CPUExecutionProvider",
                requested=requested,
                available=list(available),
            )
            return ["CPUExecutionProvider"]
        return providers

    def _load_onnx_model(self) -> _EmbeddingBackend:
        """Load ONNX model for inference."""
        if not ONNX_AVAILABLE or ort is None:
            msg = "onnxruntime is required for ONNX backend. Install with: pip install onnxruntime"
            raise ImportError(msg)
        if not TRANSFORMERS_AVAILABLE or AutoTokenizer is None:
            msg = (
                "transformers is required for ONNX backend. Install with: pip install transformers"
            )
            raise ImportError(msg)

        if not self.config.onnx_model_path:
            msg = (
                "onnx_model_path is required when backend='onnx'. "
                "Set RECAP_SUBWORKER_ONNX_MODEL_PATH environment variable."
            )
            raise ValueError(msg)

        if not Path(self.config.onnx_model_path).exists():
            msg = f"ONNX model not found: {self.config.onnx_model_path}"
            raise FileNotFoundError(msg)

        tokenizer_name = self.config.onnx_tokenizer_name or self.config.model_id

        logger.info(
            "Loading ONNX model",
            model_path=self.config.onnx_model_path,
            tokenizer_name=tokenizer_name,
            pooling=self.config.onnx_pooling,
        )

        sess_options = self._build_onnx_session_options()
        providers = self._resolve_onnx_providers()

        session = ort.InferenceSession(
            self.config.onnx_model_path,
            sess_options=sess_options,
            providers=providers,
        )
        tokenizer = AutoTokenizer.from_pretrained(tokenizer_name, use_fast=True)

        return _OnnxModelAdapter(
            session, tokenizer, self.config.onnx_pooling, self.config.onnx_max_length
        )

    def _load_ollama_remote_model(self) -> _EmbeddingBackend:
        """Load Ollama remote embedding client."""
        if not self.config.ollama_embed_url:
            msg = (
                "ollama_embed_url is required when backend='ollama-remote'. "
                "Set OLLAMA_EMBED_URL environment variable."
            )
            raise ValueError(msg)

        logger.info(
            "Initializing Ollama remote embedding client",
            url=self.config.ollama_embed_url,
            model=self.config.ollama_embed_model,
            timeout=self.config.ollama_embed_timeout,
        )

        token = self.config.inference_service_token
        token_str = token.get_secret_value() if token is not None else None
        return _OllamaRemoteAdapter(
            self.config.ollama_embed_url,
            self.config.ollama_embed_model,
            self.config.ollama_embed_timeout,
            token=token_str,
        )

    def _ensure_model(self) -> None:
        if self._model is not None:
            return

        with self._lock:
            if self._model is not None:
                return

            if self.config.backend == "sentence-transformers":
                self._model = self._load_sentence_transformer()
            elif self.config.backend == "onnx":
                self._model = self._load_onnx_model()
            elif self.config.backend == "ollama-remote":
                self._model = self._load_ollama_remote_model()
            else:
                # hash backend does not require lazy model initialization
                self._model = None

    def _encode_with_progress(
        self,
        model: _EmbeddingBackend,
        pending: list[str],
        cached_count: int,
        total_sentences: int,
    ) -> np.ndarray:
        """Encode a large pending batch while logging per-batch progress and ETA."""
        logger.info(
            "Starting embedding generation with progress tracking",
            total_sentences=total_sentences,
            cached_count=cached_count,
            pending_count=len(pending),
            batch_size=self.config.batch_size,
            estimated_batches=math.ceil(len(pending) / self.config.batch_size),
        )

        all_embeddings = []
        start_time = time.time()

        for batch_idx in range(0, len(pending), self.config.batch_size):
            batch = pending[batch_idx : batch_idx + self.config.batch_size]
            batch_start = time.time()

            batch_embeddings = model.encode(
                batch,
                batch_size=len(batch),
                normalize_embeddings=True,
                show_progress_bar=False,  # Disable tqdm progress bar
            )

            batch_elapsed = time.time() - batch_start
            batch_num = (batch_idx // self.config.batch_size) + 1
            total_batches = math.ceil(len(pending) / self.config.batch_size)
            progress_pct = (batch_num / total_batches) * 100
            elapsed_total = time.time() - start_time
            avg_time_per_batch = elapsed_total / batch_num
            eta_seconds = avg_time_per_batch * (total_batches - batch_num)

            logger.info(
                "Embedding batch progress",
                batch_num=batch_num,
                total_batches=total_batches,
                progress_percent=round(progress_pct, 1),
                batch_size=len(batch),
                batch_seconds=round(batch_elapsed, 2),
                elapsed_seconds=round(elapsed_total, 2),
                avg_seconds_per_batch=round(avg_time_per_batch, 2),
                eta_seconds=round(eta_seconds, 2),
                eta_minutes=round(eta_seconds / 60, 1),
            )

            all_embeddings.extend(batch_embeddings)

        embeddings = np.array(all_embeddings)
        total_elapsed = time.time() - start_time
        logger.info(
            "Embedding generation completed",
            total_sentences=len(pending),
            total_seconds=round(total_elapsed, 2),
            throughput_per_sec=round(len(pending) / total_elapsed, 2) if total_elapsed > 0 else 0,
        )
        return embeddings

    def _encode_pending(
        self,
        pending: list[str],
        cached_count: int,
        total_sentences: int,
    ) -> dict[str, np.ndarray]:
        """Encode uncached sentences and store them in the cache."""
        fresh: dict[str, np.ndarray] = {}
        self._ensure_model()
        if self.config.backend == "hash":
            for sentence in pending:
                vector = self._hash_sentence(sentence)
                fresh[sentence] = vector
                self._cache.set(sentence, vector)
            return fresh

        model = self._model
        if model is None:
            msg = "Embedder model must be initialized before encoding"
            raise RuntimeError(msg)

        # Manual batching with progress logging for large batches
        if len(pending) > self.config.batch_size * 2:
            embeddings = self._encode_with_progress(model, pending, cached_count, total_sentences)
        else:
            # Small batches: use direct encoding without progress logging
            embeddings = model.encode(
                pending,
                batch_size=self.config.batch_size,
                normalize_embeddings=True,
            )

        for sentence, vector in zip(pending, embeddings, strict=True):
            stored = np.asarray(vector, dtype=np.float32)
            fresh[sentence] = stored
            self._cache.set(sentence, stored)
        return fresh

    def _assemble_ordered_vectors(
        self,
        sentences: Sequence[str],
        merged: dict[str, np.ndarray],
    ) -> np.ndarray:
        """Assemble cached and freshly computed vectors in input sentence order."""
        ordered_vectors: list[np.ndarray] = []
        for sentence in sentences:
            vector = merged.get(sentence)
            if vector is not None:
                ordered_vectors.append(vector)
                continue

            if self.config.backend != "hash":
                msg = f"embedding missing for sentence: {sentence[:32]}"
                raise KeyError(msg)

            vector = self._hash_sentence(sentence)
            self._cache.set(sentence, vector)
            merged[sentence] = vector
            ordered_vectors.append(vector)

        if not ordered_vectors:
            return np.empty((0, 0), dtype=np.float32)
        return np.vstack(ordered_vectors)

    def encode(self, sentences: Sequence[str]) -> np.ndarray:
        """Generate embeddings for sentences."""
        if not sentences:
            return np.empty((0, 0), dtype=np.float32)

        cached = self._fetch_cached(sentences)
        pending = [s for s in sentences if s not in cached]
        fresh = self._encode_pending(pending, len(cached), len(sentences)) if pending else {}

        merged: dict[str, np.ndarray] = {}
        merged.update(cached)
        merged.update(fresh)
        return self._assemble_ordered_vectors(sentences, merged)

    def _fetch_cached(self, sentences: Sequence[str]) -> dict[str, np.ndarray]:
        cached: dict[str, np.ndarray] = {}
        for sentence in sentences:
            vector = self._cache.get(sentence)
            if vector is not None:
                cached[sentence] = vector
        return cached

    def warmup(self, samples: Iterable[str]) -> int:
        """Prime the model by embedding sample sentences."""

        sample_list = [s for s in samples if s]
        if not sample_list:
            return 0
        self.encode(sample_list)
        return len(sample_list)

    def close(self) -> None:
        """Release resources (if any)."""
        close_fn = getattr(self._model, "close", None)
        if callable(close_fn):
            with contextlib.suppress(Exception):
                close_fn()
        self._model = None

    def _hash_sentence(self, sentence: str) -> np.ndarray:
        digest = hashlib.sha256(sentence.encode("utf-8")).digest()
        seed = int.from_bytes(digest[:8], "little")
        rng = np.random.default_rng(seed)
        vector = rng.normal(loc=0.0, scale=1.0, size=self._hash_dimension)
        norm = np.linalg.norm(vector)
        if not math.isfinite(norm) or norm == 0.0:
            return np.zeros(self._hash_dimension, dtype=np.float32)
        return (vector / norm).astype(np.float32)
