import joblib
import json
import numpy as np
import time
from pathlib import Path
from threading import Lock
from typing import List, Dict, Any

import structlog
from sudachipy import tokenizer, dictionary

from .embedder import Embedder

logger = structlog.get_logger(__name__)

class SudachiTokenizer:
    def __init__(self, mode="C"):
        self.mode_str = mode
        self._init_tokenizer()

    def _init_tokenizer(self):
        self.tokenizer = dictionary.Dictionary().create()
        if self.mode_str == "A":
            self.mode = tokenizer.Tokenizer.SplitMode.A
        elif self.mode_str == "B":
            self.mode = tokenizer.Tokenizer.SplitMode.B
        else:
            self.mode = tokenizer.Tokenizer.SplitMode.C

    def tokenize(self, text):
        return [m.surface() for m in self.tokenizer.tokenize(text, self.mode)]

    def __call__(self, text):
        return self.tokenize(text)

    def __getstate__(self):
        return {"mode_str": self.mode_str}

    def __setstate__(self, state):
        self.mode_str = state["mode_str"]
        self._init_tokenizer()

class GenreClassifierService:
    def __init__(self, model_path: str, embedder: Embedder):
        self.embedder = embedder
        self.model_path = Path(model_path)
        self.tfidf_path = self.model_path.parent / "tfidf_vectorizer.joblib"
        self.thresholds_path = self.model_path.parent / "genre_thresholds.json"

        self.model = None
        self.tfidf = None
        self.thresholds = None
        self._lock = Lock()

    def _ensure_model(self, threshold_overrides: Dict[str, float] = None):
        # Double-checked locking pattern: check outside lock first for performance
        if self.model is None:
            with self._lock:
                # Check again inside lock to avoid race condition
                if self.model is None:
                    if not self.model_path.exists():
                        raise FileNotFoundError(f"Model not found at {self.model_path}")

                    logger.info("Loading classification artifacts", model_path=str(self.model_path))
                    self.model = joblib.load(self.model_path)

                    if self.tfidf_path.exists():
                        self.tfidf = joblib.load(self.tfidf_path)
                        # Fix: Re-attach tokenizer as it might be lost during pickling or require re-initialization
                        self.tfidf.tokenizer = SudachiTokenizer()
                        logger.info("TF-IDF vectorizer loaded with re-attached tokenizer")
                    else:
                        logger.warning("TF-IDF vectorizer not found, will use embeddings only if model allows")

                    # Load base thresholds
                    if self.thresholds_path.exists():
                        with open(self.thresholds_path) as f:
                            self.thresholds = json.load(f)
                        logger.info("Base thresholds loaded", count=len(self.thresholds))
                    else:
                        self.thresholds = {}
                        logger.warning("Thresholds not found, using default 0.5")

        # Apply overrides if provided (can change at runtime even if model is loaded)
        # This part is safe to run outside lock since model/thresholds are already initialized
        # but we ensure thresholds is not None before copying
        if self.thresholds is None:
            # This should not happen if initialization completed, but add safety check
            with self._lock:
                if self.thresholds is None:
                    # Fallback: ensure thresholds is at least an empty dict
                    self.thresholds = {}

        if threshold_overrides:
             # Make sure we don't mutate the base thresholds permanently if we were to support dynamic updates better
             # But for now, simple update is fine or just used during lookup.
             # Actually, better to store overrides separately or merge effectively.
             # For this scope, let's update simple dict if it's safe.
             # But _ensure_model is usually called once or lazily.
             # If overrides change, we might need to refresh.
             # Let's assume passed overrides are always current.
             self.current_thresholds = self.thresholds.copy()
             self.current_thresholds.update(threshold_overrides)
        else:
             self.current_thresholds = self.thresholds.copy()

        if self.model is None: # Should be loaded by now
             logger.info("Classification model and artifacts loaded")


    def predict_batch(self, texts: List[str], multi_label: bool = False, top_k: int = 5, threshold_overrides: Dict[str, float] = None) -> List[Dict[str, Any]]:
        """
        Predict genres for a batch of texts using Hybrid Features (Embedding + TF-IDF)
        and Dynamic Thresholding.

        Args:
            texts: List of texts to classify.
            multi_label: If True, returns all genres above threshold (up to top_k).
            top_k: Maximum number of genres to return per text.
            threshold_overrides: Dictionary of genre-specific thresholds to override defaults.
        """
        self._ensure_model(threshold_overrides)

        total_texts = len(texts)
        if total_texts == 0:
            return []

        logger.info(
            "Starting batch prediction",
            total_texts=total_texts,
            embedding_batch_size=self.embedder.config.batch_size,
            multi_label=multi_label,
        )

        input_texts = [f"passage: {t}" for t in texts]

        # 1. Embeddings
        embed_start = time.time()
        embeddings = self.embedder.encode(input_texts)
        embed_elapsed = time.time() - embed_start

        logger.info(
            "Embedding generation completed",
            embedding_seconds=round(embed_elapsed, 2),
        )

        if len(embeddings) == 0:
            return []

        # 2. TF-IDF
        features = embeddings
        if self.tfidf:
            tfidf_start = time.time()
            tfidf_features = self.tfidf.transform(texts) # Original texts for TF-IDF
            # Concatenate
            combined_features = np.hstack((embeddings, tfidf_features.toarray()))
            logger.info("TF-IDF extraction completed", seconds=round(time.time() - tfidf_start, 2))

            # Check what the model expects
            expected_features = getattr(self.model, "n_features_in_", None)
            if expected_features:
                 if expected_features == combined_features.shape[1]:
                     features = combined_features
                 elif expected_features == embeddings.shape[1]:
                     logger.warning(
                         "Model expects fewer features than generated. Using embeddings only.",
                         expected=expected_features,
                         got=combined_features.shape[1]
                     )
                     features = embeddings
                 else:
                     logger.error(
                         "Feature dimension mismatch",
                         expected=expected_features,
                         got_combined=combined_features.shape[1],
                         got_embeddings=embeddings.shape[1]
                     )
                     # Fallback to combined and let it crash or raise clear error?
                     # If we are here, probably crash is inevitable or we try combined.
                     features = combined_features
            else:
                 features = combined_features

        # 3. Predict Probabilities
        predict_start = time.time()
        try:
            probs_batch = self.model.predict_proba(features)
        except ValueError as e:
            # Fallback for "X has Y features, but expects Z" if generic check failed
            msg = str(e)
            if "expecting" in msg and str(embeddings.shape[1]) in msg:
                 logger.warning("ValueError during prediction, retrying with embeddings only", error=msg)
                 probs_batch = self.model.predict_proba(embeddings)
            else:
                 raise e
        predict_elapsed = time.time() - predict_start
        classes = self.model.classes_

        # 4. Apply Thresholds
        results = []
        for probs in probs_batch:
            scores = {cls: float(prob) for cls, prob in zip(classes, probs)}

            # Find candidate classes that pass threshold
            candidates = []
            for cls, score in scores.items():
                threshold = self.current_thresholds.get(cls, 0.5)
                if score >= threshold:
                    candidates.append({"genre": cls, "score": score, "threshold": threshold})

            # Sort candidates by score descending
            candidates.sort(key=lambda x: x["score"], reverse=True)

            if multi_label:
                # In multi-label mode, return top_k candidates that passed threshold
                # If no candidates passed, return empty or fallback?
                # Usually multi-label allows empty.
                # But for existing consumers, we might want at least one if we fallback to single behavior?
                # Let's stick to: candidates list.

                # If no candidates, effectively "other" or empty.
                # But let's fill top_genre for backward compatibility regardless.
                if candidates:
                    top_match = candidates[0]
                    top_genre = top_match["genre"]
                    confidence = top_match["score"]
                    final_candidates = candidates[:top_k]
                else:
                    # No genre passed threshold
                    top_genre = 'other'
                    confidence = scores.get('other', 0.0)
                    if 'other' not in scores:
                         # Fallback if 'other' not in model
                         idx = np.argmax(probs)
                         top_genre = classes[idx]
                         confidence = float(scores[top_genre])

                    final_candidates = [] # Empty candidates list if nothing passed threshold

                results.append({
                    "top_genre": top_genre,
                    "confidence": confidence,
                    "scores": scores,
                    "candidates": final_candidates
                })

            else:
                # Single label mode (backward compatible logic)
                if candidates:
                    top_match = candidates[0]
                    top_genre = top_match["genre"]
                    confidence = top_match["score"]
                    # If we enforce strictly > threshold, this is it.
                else:
                    # Fallback
                    top_genre = 'other'
                    confidence = scores.get('other', 0.0)
                    if 'other' not in scores:
                        top_genre = classes[np.argmax(probs)]
                        confidence = float(scores[top_genre])

                results.append({
                    "top_genre": top_genre,
                    "confidence": confidence,
                    "scores": scores,
                    "candidates": candidates[:top_k] # Still provide candidates for debug/info
                })

        logger.info(
            "Batch prediction completed",
            total_texts=total_texts,
            total_seconds=round(embed_elapsed + predict_elapsed, 2),
        )

        return results
