from __future__ import annotations

import os
import time
from collections.abc import Iterable, Iterator, Sequence
from dataclasses import dataclass
from itertools import islice
from typing import TYPE_CHECKING

import structlog

if TYPE_CHECKING:
    import numpy as np
    import onnxruntime as ort
    from transformers import AutoTokenizer
else:
    try:
        import numpy as np
    except ImportError:  # pragma: no cover
        np = None  # type: ignore[assignment]

    try:
        import onnxruntime as ort  # type: ignore
    except ImportError:  # pragma: no cover
        ort = None  # type: ignore[assignment]

    try:
        from transformers import AutoTokenizer
    except ImportError:  # pragma: no cover
        AutoTokenizer = None  # type: ignore[assignment]


logger = structlog.get_logger(__name__)


class OnnxRuntimeMissing(ImportError):
    """Raised when ONNX runtime dependencies are not available."""


@dataclass
class OnnxEmbeddingConfig:
    """Configuration for the ONNX embedding helper."""

    model_path: str
    tokenizer_name: str
    pooling: str = "cls"
    batch_size: int = 16
    max_length: int = 256


class OnnxEmbeddingModel:
    """Lightweight adapter that exposes `.encode()` like SentenceTransformer."""

    def __init__(self, config: OnnxEmbeddingConfig) -> None:
        if ort is None or AutoTokenizer is None or np is None:
            raise OnnxRuntimeMissing("onnxruntime, transformers, and numpy are required for ONNX embedding support")
        assert np is not None  # For type checker
        assert ort is not None  # For type checker
        assert AutoTokenizer is not None  # For type checker

        if config.pooling not in {"cls", "mean"}:
            raise ValueError("Pooling must be 'cls' or 'mean'")

        self._config = config
        self._batch_size_override = self._read_int_env("TAG_ONNX_RUNTIME_BATCH_SIZE")
        self._max_batch_tokens = self._read_int_env("TAG_ONNX_MAX_BATCH_TOKENS")
        self._providers = self._resolve_providers()
        self._session_options = self._build_session_options()
        self._session = ort.InferenceSession(
            config.model_path,
            sess_options=self._session_options,
            providers=self._providers,
        )
        self._tokenizer = AutoTokenizer.from_pretrained(config.tokenizer_name, use_fast=True)
        self._embedding_dimension = self._session.get_outputs()[0].shape[-1]

    def encode(
        self,
        texts: Sequence[str],
        batch_size: int | None = None,
        show_progress_bar: bool = False,  # Mirror SentenceTransformer signature
    ) -> np.ndarray:  # type: ignore[return-value]
        if batch_size is None:
            batch_size = self._config.batch_size
        batch_size = self._effective_batch_size(batch_size)

        embeddings: list[np.ndarray] = []  # type: ignore[type-arg]
        total_tokens = 0
        total_batches = 0
        start_time = time.perf_counter()

        for batch in self._batch(texts, batch_size):
            total_batches += 1
            tokens = self._tokenizer(
                list(batch),
                padding=True,
                truncation=True,
                max_length=self._config.max_length,
                return_tensors="np",
            )
            total_tokens += int(tokens["input_ids"].size)

            ort_inputs = dict(tokens.items())

            hidden_states = self._session.run(None, ort_inputs)[0]
            if self._config.pooling == "mean":
                emb = hidden_states.mean(axis=1)
            else:
                emb = hidden_states[:, 0, :]

            embeddings.append(emb)

        if not embeddings:
            return np.zeros((0, self._session.get_outputs()[0].shape[-1]), dtype=np.float32)

        total_time_ms = (time.perf_counter() - start_time) * 1000
        logger.debug(
            "ONNX inference completed",
            sequences=len(texts),
            total_batches=total_batches,
            batch_size=batch_size,
            total_tokens=total_tokens,
            elapsed_ms=round(total_time_ms, 2),
            providers=self._providers,
        )
        return np.vstack(embeddings)

    @staticmethod
    def _batch(iterable: Iterable[str], size: int) -> Iterator[list[str]]:
        iterator = iter(iterable)
        while True:
            chunk = list(islice(iterator, size))
            if not chunk:
                break
            yield chunk

    def describe(self) -> dict[str, object]:
        """Expose runtime metadata for logging and analytics."""
        return {
            "backend": "onnx",
            "model_path": self._config.model_path,
            "tokenizer_name": self._config.tokenizer_name,
            "pooling": self._config.pooling,
            "batch_size": self._effective_batch_size(self._config.batch_size),
            "max_length": self._config.max_length,
            "providers": self._providers,
            "graph_optimization_level": getattr(self._session_options, "graph_optimization_level", "unknown"),
            "embedding_dimension": self._embedding_dimension,
            "max_batch_tokens": self._max_batch_tokens,
        }

    def _resolve_providers(self) -> list[str]:
        """Determine which ONNX Runtime providers to use."""
        configured = os.getenv("TAG_ONNX_RUNTIME_PROVIDERS")
        available = set(ort.get_available_providers())
        if configured:
            requested = [provider.strip() for provider in configured.split(",") if provider.strip()]
            providers = [p for p in requested if p in available]
            if not providers:
                logger.warning(
                    "Requested ONNX providers are unavailable, falling back to CPUExecutionProvider",
                    requested=requested,
                    available=list(available),
                )
                return ["CPUExecutionProvider"]
            return providers
        if "CPUExecutionProvider" in available:
            return ["CPUExecutionProvider"]
        return list(available)

    def _build_session_options(self) -> ort.SessionOptions:  # type: ignore[return-value]
        """Build tuned SessionOptions for ONNX Runtime."""
        options = ort.SessionOptions()
        options.enable_mem_pattern = True
        options.enable_cpu_mem_arena = True

        graph_opt_level = os.getenv("TAG_ONNX_GRAPH_OPT_LEVEL", "ORT_ENABLE_ALL")
        try:
            options.graph_optimization_level = getattr(ort.GraphOptimizationLevel, graph_opt_level)
        except AttributeError:  # pragma: no cover - invalid env
            logger.warning(
                "Invalid TAG_ONNX_GRAPH_OPT_LEVEL provided, defaulting to ORT_ENABLE_ALL",
                requested=graph_opt_level,
            )
            options.graph_optimization_level = ort.GraphOptimizationLevel.ORT_ENABLE_ALL

        intra_threads = os.getenv("TAG_ONNX_INTRA_OP_THREADS")
        inter_threads = os.getenv("TAG_ONNX_INTER_OP_THREADS")

        if intra_threads and intra_threads.isdigit():
            options.intra_op_num_threads = int(intra_threads)
        if inter_threads and inter_threads.isdigit():
            options.inter_op_num_threads = int(inter_threads)

        execution_mode = os.getenv("TAG_ONNX_EXECUTION_MODE", "ORT_SEQUENTIAL")
        if execution_mode.upper() == "ORT_PARALLEL":
            options.execution_mode = ort.ExecutionMode.ORT_PARALLEL
        else:
            options.execution_mode = ort.ExecutionMode.ORT_SEQUENTIAL

        return options

    def _effective_batch_size(self, requested: int) -> int:
        """Clamp batch size based on overrides and max token thresholds."""
        size = requested
        if self._batch_size_override:
            size = self._batch_size_override
        if self._max_batch_tokens:
            max_by_tokens = max(1, self._max_batch_tokens // self._config.max_length)
            size = min(size, max_by_tokens)
        return max(1, size)

    @staticmethod
    def _read_int_env(env_name: str) -> int | None:
        value = os.getenv(env_name)
        if value and value.isdigit():
            return int(value)
        return None
