"""Rerank usecase - cross-encoder re-ranking for RAG retrieval.

Research basis:
- Pinecone: +15-30% NDCG@10 improvement with cross-encoder
- ZeroEntropy: -35% LLM hallucinations with re-ranking
- Recommended models:
  - BAAI/bge-reranker-v2-m3 (multilingual, 568M params, ~150ms)
  - cross-encoder/ms-marco-MiniLM-L-6-v2 (English, 100M params, ~60ms)
"""

import asyncio
import logging
import threading
import time

logger = logging.getLogger(__name__)

# Lazy import to avoid loading model at startup. _get_cross_encoder runs
# inside a worker thread (called via asyncio.to_thread), so concurrent first
# requests race across real OS threads, not just coroutines -- guarded with a
# threading.Lock, not asyncio.Lock.
_cross_encoder = None
_loaded_model_name = None
_load_lock = threading.Lock()


def _get_cross_encoder(model_name: str):
    """Lazily load cross-encoder model.

    Args:
        model_name: Name of the cross-encoder model to load

    Returns:
        CrossEncoder instance
    """
    global _cross_encoder, _loaded_model_name

    if _cross_encoder is not None and _loaded_model_name == model_name:
        return _cross_encoder

    with _load_lock:
        # Re-check inside the lock: another thread may have finished loading
        # while this one was waiting.
        if _cross_encoder is None or _loaded_model_name != model_name:
            from sentence_transformers import CrossEncoder

            logger.info("Loading cross-encoder model", extra={"model_name": model_name})
            load_start = time.time()
            _cross_encoder = CrossEncoder(model_name, device="cpu")
            _loaded_model_name = model_name
            load_elapsed = time.time() - load_start
            logger.info(
                "Cross-encoder model loaded",
                extra={"model_name": model_name, "load_time_s": round(load_elapsed, 2)},
            )

    return _cross_encoder


class RerankUsecase:
    """Usecase for cross-encoder re-ranking of RAG retrieval candidates.

    Uses sentence-transformers CrossEncoder for efficient batch scoring.
    """

    # Default model - multilingual, good balance of accuracy and speed
    DEFAULT_MODEL = "BAAI/bge-reranker-v2-m3"

    def __init__(self, model_name: str | None = None):
        """Initialize rerank usecase.

        Args:
            model_name: Cross-encoder model name. If None, uses DEFAULT_MODEL.
        """
        self.model_name = model_name or self.DEFAULT_MODEL

    async def warmup(self) -> None:
        """Eagerly load the cross-encoder model off the event loop.

        Called at service startup so the first real /rerank request doesn't
        pay the (potentially multi-second) model-load cost synchronously
        inside a request-handling coroutine. This is a startup optimization,
        not a hard requirement -- a network-isolated deployment (no egress
        to the model hub) must not crash the whole service at boot; rerank()
        falls back to loading the model lazily on first real use.
        """
        logger.info("Warming up cross-encoder model", extra={"model": self.model_name})
        try:
            await asyncio.to_thread(_get_cross_encoder, self.model_name)
            logger.info(
                "Cross-encoder warmup complete", extra={"model": self.model_name}
            )
        except Exception as e:
            logger.warning(
                f"Cross-encoder warmup failed, will load lazily on first use: {e}",
                extra={"model": self.model_name},
                exc_info=True,
            )

    async def rerank(
        self,
        query: str,
        candidates: list[str],
        top_k: int | None = None,
    ) -> tuple[list[tuple[int, float]], str, float | None]:
        """
        Re-rank candidates using cross-encoder scoring.

        Args:
            query: Query to score candidates against
            candidates: List of candidate texts to re-rank
            top_k: Optional limit on returned results (default: return all)

        Returns:
            Tuple of:
            - List of (original_index, score) tuples sorted by score descending
            - Model name used
            - Processing time in milliseconds

        Raises:
            ValueError: If query or candidates are empty
            RuntimeError: If model loading or inference fails
        """
        if not query or not query.strip():
            raise ValueError("query cannot be empty")

        if not candidates:
            raise ValueError("candidates list cannot be empty")

        start_time = time.time()

        logger.info(
            "Re-ranking candidates",
            extra={
                "query": query[:100],
                "candidate_count": len(candidates),
                "top_k": top_k,
                "model": self.model_name,
            },
        )

        try:
            # Load model (lazily) off the event loop -- first call also does
            # the (CPU-heavy, multi-second) model load synchronously.
            cross_encoder = await asyncio.to_thread(_get_cross_encoder, self.model_name)

            # Prepare query-candidate pairs
            pairs = [(query, candidate) for candidate in candidates]

            # Score all pairs in batch off the event loop -- CrossEncoder.predict()
            # is a blocking CPU-bound call; running it inline would stall the
            # event loop (and SSE heartbeats) for the duration of inference.
            scores = await asyncio.to_thread(cross_encoder.predict, pairs)

            # Create (index, score) tuples and sort by score descending
            indexed_scores = list(enumerate(scores))
            indexed_scores.sort(key=lambda x: x[1], reverse=True)

            # Apply top_k limit if specified
            if top_k is not None and top_k < len(indexed_scores):
                indexed_scores = indexed_scores[:top_k]

            # Convert numpy floats to Python floats
            results = [(idx, float(score)) for idx, score in indexed_scores]

            elapsed_ms = (time.time() - start_time) * 1000

            logger.info(
                "Re-ranking completed",
                extra={
                    "query": query[:100],
                    "candidate_count": len(candidates),
                    "result_count": len(results),
                    "top_score": round(results[0][1], 4) if results else None,
                    "model": self.model_name,
                    "elapsed_ms": round(elapsed_ms, 2),
                },
            )

            return results, self.model_name, elapsed_ms

        except Exception as e:
            elapsed_ms = (time.time() - start_time) * 1000
            logger.error(
                "Re-ranking failed",
                extra={
                    "query": query[:100],
                    "candidate_count": len(candidates),
                    "error": str(e),
                    "error_type": type(e).__name__,
                    "elapsed_ms": round(elapsed_ms, 2),
                },
                exc_info=True,
            )
            raise RuntimeError(f"Re-ranking failed: {e}") from e
