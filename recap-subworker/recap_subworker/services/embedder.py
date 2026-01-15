"""Sentence embedding utilities."""

from __future__ import annotations

import hashlib
import math
import os
import time
from dataclasses import dataclass
from itertools import islice
from typing import Iterable, Iterator, Literal, Sequence

import numpy as np
import structlog

from ..infra.cache import LRUCache

from threading import Lock

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
    ollama_embed_model: str = "mxbai-embed-large"
    ollama_embed_timeout: float = 30.0


class Embedder:
    """Embedding facade supporting multiple backends."""

    def __init__(self, config: EmbedderConfig) -> None:
        self.config = config
        self._model = None
        self._cache = LRUCache[str, np.ndarray](config.cache_size)
        self._hash_dimension = 256
        self._lock = Lock()

    def _load_sentence_transformer(self):
        from sentence_transformers import SentenceTransformer  # lazy import

        logger.info(
            "Loading SentenceTransformer",
            model_id=self.config.model_id,
            backend=self.config.backend
        )

        if self.config.model_id != "intfloat/multilingual-e5-large":
            logger.warn(
                "Model ID mismatch recommendation",
                current=self.config.model_id,
                recommended="intfloat/multilingual-e5-large"
            )

        model_kwargs = {
            "low_cpu_mem_usage": False,
            "trust_remote_code": False,
        }

        if self.config.device.startswith("cuda"):
            import torch
            logger.info("Enabling FP16 for CUDA device")
            model_kwargs["torch_dtype"] = torch.float16

        logger.info("Initializing SentenceTransformer model (this may take time for large models)...")
        model = SentenceTransformer(
            self.config.model_id,
            device=self.config.device,
            model_kwargs=model_kwargs,
        )
        logger.info("SentenceTransformer model initialized", device=self.config.device)
        return model

    def _load_onnx_model(self):
        """Load ONNX model for inference."""
        if not ONNX_AVAILABLE:
            raise ImportError(
                "onnxruntime is required for ONNX backend. "
                "Install with: pip install onnxruntime"
            )
        if not TRANSFORMERS_AVAILABLE:
            raise ImportError(
                "transformers is required for ONNX backend. "
                "Install with: pip install transformers"
            )

        if not self.config.onnx_model_path:
            raise ValueError(
                "onnx_model_path is required when backend='onnx'. "
                "Set RECAP_SUBWORKER_ONNX_MODEL_PATH environment variable."
            )

        if not os.path.exists(self.config.onnx_model_path):
            raise FileNotFoundError(
                f"ONNX model not found: {self.config.onnx_model_path}"
            )

        tokenizer_name = self.config.onnx_tokenizer_name or self.config.model_id

        logger.info(
            "Loading ONNX model",
            model_path=self.config.onnx_model_path,
            tokenizer_name=tokenizer_name,
            pooling=self.config.onnx_pooling,
        )

        # Build session options
        sess_options = ort.SessionOptions()
        sess_options.enable_mem_pattern = True
        sess_options.enable_cpu_mem_arena = True

        # Graph optimization level
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

        # Thread configuration
        intra_threads = os.getenv("RECAP_ONNX_INTRA_OP_THREADS")
        inter_threads = os.getenv("RECAP_ONNX_INTER_OP_THREADS")
        if intra_threads and intra_threads.isdigit():
            sess_options.intra_op_num_threads = int(intra_threads)
        if inter_threads and inter_threads.isdigit():
            sess_options.inter_op_num_threads = int(inter_threads)

        # Execution mode
        execution_mode = os.getenv("RECAP_ONNX_EXECUTION_MODE", "ORT_SEQUENTIAL")
        if execution_mode.upper() == "ORT_PARALLEL":
            sess_options.execution_mode = ort.ExecutionMode.ORT_PARALLEL
        else:
            sess_options.execution_mode = ort.ExecutionMode.ORT_SEQUENTIAL

        # Resolve providers
        configured_providers = os.getenv("RECAP_ONNX_RUNTIME_PROVIDERS")
        available = set(ort.get_available_providers())

        if configured_providers:
            requested = [
                p.strip() for p in configured_providers.split(",") if p.strip()
            ]
            providers = [p for p in requested if p in available]
            if not providers:
                logger.warning(
                    "Requested ONNX providers unavailable, falling back to CPUExecutionProvider",
                    requested=requested,
                    available=list(available),
                )
                providers = ["CPUExecutionProvider"]
        else:
            providers = ["CPUExecutionProvider"] if "CPUExecutionProvider" in available else list(available)

        # Create inference session
        session = ort.InferenceSession(
            self.config.onnx_model_path,
            sess_options=sess_options,
            providers=providers,
        )

        # Load tokenizer
        tokenizer = AutoTokenizer.from_pretrained(tokenizer_name, use_fast=True)

        # Return a simple object that exposes .encode() method
        class OnnxModelAdapter:
            def __init__(self, session, tokenizer, pooling, max_length):
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
                    batch = sentences[i:i + batch_size]

                    # Tokenize
                    tokens = self.tokenizer(
                        list(batch),
                        padding=True,
                        truncation=True,
                        max_length=self.max_length,
                        return_tensors="np",
                    )

                    # Run inference
                    ort_inputs = {k: v for k, v in tokens.items()}
                    hidden_states = self.session.run(None, ort_inputs)[0]

                    # Pooling
                    if self.pooling == "mean":
                        emb = hidden_states.mean(axis=1)
                    else:  # cls
                        emb = hidden_states[:, 0, :]

                    # Normalize
                    if normalize_embeddings:
                        norms = np.linalg.norm(emb, axis=1, keepdims=True)
                        norms = np.where(norms == 0, 1.0, norms)
                        emb = emb / norms

                    embeddings_list.append(emb.astype(np.float32))

                if not embeddings_list:
                    embedding_dim = self.session.get_outputs()[0].shape[-1]
                    return np.zeros((0, embedding_dim), dtype=np.float32)

                return np.vstack(embeddings_list)

        return OnnxModelAdapter(session, tokenizer, self.config.onnx_pooling, self.config.onnx_max_length)

    def _load_ollama_remote_model(self):
        """Load Ollama remote embedding client."""
        import httpx

        if not self.config.ollama_embed_url:
            raise ValueError(
                "ollama_embed_url is required when backend='ollama-remote'. "
                "Set OLLAMA_EMBED_URL environment variable."
            )

        logger.info(
            "Initializing Ollama remote embedding client",
            url=self.config.ollama_embed_url,
            model=self.config.ollama_embed_model,
            timeout=self.config.ollama_embed_timeout,
        )

        class OllamaRemoteAdapter:
            """Adapter for Ollama /api/embed endpoint."""

            # Maximum characters per chunk for embedding
            # mxbai-embed-large has 512 token context; ~4 chars/token gives ~2K chars
            # Using conservative 800 chars to ensure single texts fit
            MAX_CHUNK_CHARS = 800

            def __init__(self, url: str, model: str, timeout: float):
                self.url = url.rstrip("/")
                self.model = model
                self.timeout = timeout
                self._client = httpx.Client(timeout=timeout)
                self._embedding_dim: int | None = None

            def _call_embed_api(self, texts: list[str]) -> list[list[float]]:
                """Call Ollama /api/embed endpoint for a batch of short texts."""
                try:
                    response = self._client.post(
                        f"{self.url}/api/embed",
                        json={"model": self.model, "input": texts},
                    )
                    response.raise_for_status()
                except httpx.HTTPStatusError as exc:
                    raise RuntimeError(
                        f"Ollama API error: {exc.response.status_code} - {exc.response.text[:200]}"
                    ) from None
                except httpx.RequestError as exc:
                    raise RuntimeError(f"Ollama API request failed: {exc}") from None
                data = response.json()
                return data["embeddings"]

            def _embed_single_text(self, text: str) -> list[float]:
                """Embed a single text, chunking if necessary."""
                if len(text) <= self.MAX_CHUNK_CHARS:
                    # Short text: embed directly
                    embeddings = self._call_embed_api([text])
                    return embeddings[0]

                # Long text: split into chunks and average embeddings
                chunks = []
                for i in range(0, len(text), self.MAX_CHUNK_CHARS):
                    chunk = text[i:i + self.MAX_CHUNK_CHARS]
                    if chunk.strip():
                        chunks.append(chunk)

                if not chunks:
                    chunks = [text[:self.MAX_CHUNK_CHARS]]

                logger.debug(
                    "Chunking long text for embedding",
                    original_length=len(text),
                    chunk_count=len(chunks),
                )

                # Embed all chunks in one API call for efficiency
                chunk_embeddings = self._call_embed_api(chunks)

                # Average the chunk embeddings
                avg_embedding = np.mean(chunk_embeddings, axis=0).tolist()
                return avg_embedding

            def _get_embeddings(self, texts: list[str]) -> list[list[float]]:
                """Get embeddings for multiple texts with chunking support."""
                results = []
                for text in texts:
                    emb = self._embed_single_text(text)
                    results.append(emb)
                return results

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
                    batch = list(sentences[i:i + batch_size])
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

            def close(self):
                self._client.close()

        return OllamaRemoteAdapter(
            self.config.ollama_embed_url,
            self.config.ollama_embed_model,
            self.config.ollama_embed_timeout,
        )

    def _ensure_model(self):
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

    def encode(self, sentences: Sequence[str]) -> np.ndarray:
        """Generate embeddings for sentences."""

        if not sentences:
            return np.empty((0, 0), dtype=np.float32)

        total_sentences = len(sentences)
        cached = self._fetch_cached(sentences)
        pending = [s for s in sentences if s not in cached]
        fresh: dict[str, np.ndarray] = {}

        if pending:
            self._ensure_model()
            if self.config.backend == "hash":
                for sentence in pending:
                    vector = self._hash_sentence(sentence)
                    fresh[sentence] = vector
                    self._cache.set(sentence, vector)
            else:
                model = self._model
                assert model is not None

                # Manual batching with progress logging for large batches
                if len(pending) > self.config.batch_size * 2:
                    logger.info(
                        "Starting embedding generation with progress tracking",
                        total_sentences=total_sentences,
                        cached_count=len(cached),
                        pending_count=len(pending),
                        batch_size=self.config.batch_size,
                        estimated_batches=math.ceil(len(pending) / self.config.batch_size),
                    )

                    all_embeddings = []
                    start_time = time.time()

                    for batch_idx in range(0, len(pending), self.config.batch_size):
                        batch = pending[batch_idx:batch_idx + self.config.batch_size]
                        batch_start = time.time()

                        batch_embeddings = model.encode(  # type: ignore[attr-defined]
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
                        remaining_batches = total_batches - batch_num
                        eta_seconds = avg_time_per_batch * remaining_batches

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
                else:
                    # Small batches: use direct encoding without progress logging
                    embeddings = model.encode(  # type: ignore[attr-defined]
                        pending,
                        batch_size=self.config.batch_size,
                        normalize_embeddings=True,
                    )

                for sentence, vector in zip(pending, embeddings):
                    stored = np.asarray(vector, dtype=np.float32)
                    fresh[sentence] = stored
                    self._cache.set(sentence, stored)

        merged: dict[str, np.ndarray] = {}
        merged.update(cached)
        merged.update(fresh)

        ordered_vectors: list[np.ndarray] = []
        for sentence in sentences:
            vector = merged.get(sentence)
            if vector is not None:
                ordered_vectors.append(vector)
                continue

            if self.config.backend != "hash":
                raise KeyError(f"embedding missing for sentence: {sentence[:32]}")

            vector = self._hash_sentence(sentence)
            self._cache.set(sentence, vector)
            merged[sentence] = vector
            ordered_vectors.append(vector)

        if not ordered_vectors:
            return np.empty((0, 0), dtype=np.float32)
        return np.vstack(ordered_vectors)

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
        if self._model is not None and hasattr(self._model, "close"):
            try:
                self._model.close()
            except Exception:
                pass
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
