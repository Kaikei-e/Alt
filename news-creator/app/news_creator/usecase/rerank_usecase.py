"""Rerank usecase - cross-encoder re-ranking for RAG retrieval.

Research basis:
- Pinecone: +15-30% NDCG@10 improvement with cross-encoder
- ZeroEntropy: -35% LLM hallucinations with re-ranking
- Recommended models:
  - BAAI/bge-reranker-v2-m3 (multilingual, 568M params, ~150ms)
  - cross-encoder/ms-marco-MiniLM-L-6-v2 (English, 100M params, ~60ms)
"""

import logging
import time
from typing import List, Tuple, Optional

logger = logging.getLogger(__name__)

# Lazy import to avoid loading model at startup
_cross_encoder = None
_loaded_model_name = None


def _get_cross_encoder(model_name: str):
    """Lazily load cross-encoder model.

    Args:
        model_name: Name of the cross-encoder model to load

    Returns:
        CrossEncoder instance
    """
    global _cross_encoder, _loaded_model_name

    if _cross_encoder is None or _loaded_model_name != model_name:
        from sentence_transformers import CrossEncoder

        logger.info(
            "Loading cross-encoder model",
            extra={"model_name": model_name}
        )
        load_start = time.time()
        _cross_encoder = CrossEncoder(model_name)
        _loaded_model_name = model_name
        load_elapsed = time.time() - load_start
        logger.info(
            "Cross-encoder model loaded",
            extra={"model_name": model_name, "load_time_s": round(load_elapsed, 2)}
        )

    return _cross_encoder


class RerankUsecase:
    """Usecase for cross-encoder re-ranking of RAG retrieval candidates.

    Uses sentence-transformers CrossEncoder for efficient batch scoring.
    """

    # Default model - multilingual, good balance of accuracy and speed
    DEFAULT_MODEL = "BAAI/bge-reranker-v2-m3"

    def __init__(self, model_name: Optional[str] = None):
        """Initialize rerank usecase.

        Args:
            model_name: Cross-encoder model name. If None, uses DEFAULT_MODEL.
        """
        self.model_name = model_name or self.DEFAULT_MODEL

    async def rerank(
        self,
        query: str,
        candidates: List[str],
        top_k: Optional[int] = None,
    ) -> Tuple[List[Tuple[int, float]], str, Optional[float]]:
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
            }
        )

        try:
            # Load model (lazily)
            cross_encoder = _get_cross_encoder(self.model_name)

            # Prepare query-candidate pairs
            pairs = [(query, candidate) for candidate in candidates]

            # Score all pairs in batch
            scores = cross_encoder.predict(pairs)

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
                }
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
