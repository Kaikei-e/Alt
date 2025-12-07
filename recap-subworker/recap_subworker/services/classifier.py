import joblib
import numpy as np
import time
from pathlib import Path
from typing import List, Dict, Any

import structlog

from .embedder import Embedder

logger = structlog.get_logger(__name__)

class GenreClassifierService:
    def __init__(self, model_path: str, embedder: Embedder):
        self.embedder = embedder
        self.model_path = Path(model_path)
        self.model = None

    def _ensure_model(self):
        if self.model is None:
            if not self.model_path.exists():
                raise FileNotFoundError(f"Model not found at {self.model_path}")
            logger.info("Loading classification model", model_path=str(self.model_path))
            self.model = joblib.load(self.model_path)
            logger.info("Classification model loaded", model_path=str(self.model_path), classes=len(self.model.classes_))

    def predict_batch(self, texts: List[str]) -> List[Dict[str, Any]]:
        """
        Predict genres for a batch of texts.

        Note: Input texts should be preprocessed to "title + lead + first N sentences"
        format for consistency with training data. The caller (recap-worker) is
        responsible for constructing this unified format.

        Args:
            texts: List of preprocessed text strings (title + lead + first N sentences)

        Returns:
            List of prediction results with top_genre, confidence, and scores
        """
        self._ensure_model()

        total_texts = len(texts)
        if total_texts == 0:
            return []

        logger.info(
            "Starting batch prediction",
            total_texts=total_texts,
            embedding_batch_size=self.embedder.config.batch_size,
        )

        # E5 expects "passage: " prefix for documents
        # Note: Training should use the same prefix for consistency
        input_texts = [f"passage: {t}" for t in texts]

        # Embedding generation with progress logging
        embed_start = time.time()
        embeddings = self.embedder.encode(input_texts)
        embed_elapsed = time.time() - embed_start

        logger.info(
            "Embedding generation completed",
            total_texts=total_texts,
            embedding_shape=embeddings.shape if len(embeddings) > 0 else None,
            embedding_seconds=round(embed_elapsed, 2),
            embedding_throughput=round(total_texts / embed_elapsed, 2) if embed_elapsed > 0 else 0,
        )

        if len(embeddings) == 0:
            logger.warning("No embeddings generated, returning empty results")
            return []

        # Predict probabilities
        predict_start = time.time()
        probs_batch = self.model.predict_proba(embeddings)
        predict_elapsed = time.time() - predict_start
        classes = self.model.classes_

        logger.info(
            "Model prediction completed",
            total_texts=total_texts,
            num_classes=len(classes),
            prediction_seconds=round(predict_elapsed, 2),
            prediction_throughput=round(total_texts / predict_elapsed, 2) if predict_elapsed > 0 else 0,
        )

        # Build results
        results = []
        for probs in probs_batch:
            scores = {cls: float(prob) for cls, prob in zip(classes, probs)}
            top_class = classes[np.argmax(probs)]
            results.append({
                "top_genre": top_class,
                "confidence": float(scores[top_class]),
                "scores": scores
            })

        logger.info(
            "Batch prediction completed",
            total_texts=total_texts,
            results_count=len(results),
            total_seconds=round(embed_elapsed + predict_elapsed, 2),
        )

        return results
