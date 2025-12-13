"""Learning Machine Student Classifier for multi-language genre classification."""

import json
import logging
from pathlib import Path
from typing import List, Dict, Any, Optional, Tuple

import torch
import structlog
import yaml

from ..learning_machine.student.model import StudentDistilBERT

logger = structlog.get_logger(__name__)


def detect_language_simple(text: str, min_chars: int = 50) -> str:
    """Simple language detection for Japanese/English.

    Args:
        text: Input text
        min_chars: Minimum character count to make a decision (default: 50)

    Returns:
        "ja", "en", or "unknown"
    """
    if len(text) < min_chars:
        return "unknown"

    # Check for Japanese characters (Hiragana, Katakana, Kanji)
    has_japanese = any(
        "\u3040" <= char <= "\u309F" or  # Hiragana
        "\u30A0" <= char <= "\u30FF" or  # Katakana
        "\u4E00" <= char <= "\u9FAF"     # CJK Unified Ideographs
        for char in text
    )

    # Check for English (ASCII alphabetic)
    has_english = any(char.isascii() and char.isalpha() for char in text)

    # Count ratio of Japanese vs English characters
    jp_chars = sum(1 for char in text if "\u3040" <= char <= "\u309F" or "\u30A0" <= char <= "\u30FF" or "\u4E00" <= char <= "\u9FAF")
    en_chars = sum(1 for char in text if char.isascii() and char.isalpha())
    total_chars = len([c for c in text if c.isalnum() or "\u3040" <= c <= "\u309F" or "\u30A0" <= c <= "\u30FF" or "\u4E00" <= c <= "\u9FAF"])

    if total_chars == 0:
        return "unknown"

    jp_ratio = jp_chars / total_chars if total_chars > 0 else 0
    en_ratio = en_chars / total_chars if total_chars > 0 else 0

    # Decision logic: if Japanese characters present and ratio > 0.1, classify as Japanese
    # Otherwise, if English characters present, classify as English
    if has_japanese and jp_ratio > 0.1:
        return "ja"
    elif has_english and en_ratio > 0.3:
        return "en"
    elif has_japanese:
        return "ja"  # Fallback: if Japanese chars exist, prefer Japanese
    elif has_english:
        return "en"  # Fallback: if English chars exist, prefer English

    return "unknown"


class LearningMachineStudentClassifier:
    """Multi-language genre classifier using learning_machine student models."""

    def __init__(
        self,
        student_ja_dir: Optional[str] = None,
        student_en_dir: Optional[str] = None,
        taxonomy_path: Optional[str] = None,
        device: str = "cpu",
    ):
        """Initialize the classifier with language-specific student models.

        Args:
            student_ja_dir: Path to Japanese student model directory
            student_en_dir: Path to English student model directory
            taxonomy_path: Path to genres.yaml taxonomy file
            device: Device to run inference on ("cpu" or "cuda")
        """
        self.device = torch.device(device if torch.cuda.is_available() and device == "cuda" else "cpu")
        logger.info("Initializing LearningMachineStudentClassifier", device=str(self.device))

        # Load taxonomy
        if taxonomy_path is None:
            taxonomy_path = "recap_subworker/learning_machine/taxonomy/genres.yaml"
        taxonomy_path_obj = Path(taxonomy_path)
        if not taxonomy_path_obj.exists():
            raise FileNotFoundError(f"Taxonomy file not found: {taxonomy_path}")

        with open(taxonomy_path_obj, "r", encoding="utf-8") as f:
            taxonomy_data = yaml.safe_load(f)
        self.genres = taxonomy_data.get("genres", [])
        self.id2label = {i: g for i, g in enumerate(self.genres)}
        self.label2id = {g: i for i, g in enumerate(self.genres)}
        self.num_labels = len(self.genres)

        logger.info(f"Loaded taxonomy with {self.num_labels} genres")

        # Load models
        self.model_ja: Optional[StudentDistilBERT] = None
        self.model_en: Optional[StudentDistilBERT] = None

        if student_ja_dir:
            ja_path = Path(student_ja_dir)
            if ja_path.exists():
                try:
                    self.model_ja = StudentDistilBERT.from_pretrained(str(ja_path), num_labels=self.num_labels)
                    self.model_ja.to(self.device)
                    self.model_ja.eval()
                    logger.info(f"Loaded Japanese model from {student_ja_dir}")
                except Exception as e:
                    logger.error(f"Failed to load Japanese model: {e}")
            else:
                logger.warning(f"Japanese model directory not found: {student_ja_dir}")

        if student_en_dir:
            en_path = Path(student_en_dir)
            if en_path.exists():
                try:
                    self.model_en = StudentDistilBERT.from_pretrained(str(en_path), num_labels=self.num_labels)
                    self.model_en.to(self.device)
                    self.model_en.eval()
                    logger.info(f"Loaded English model from {student_en_dir}")
                except Exception as e:
                    logger.error(f"Failed to load English model: {e}")
            else:
                logger.warning(f"English model directory not found: {student_en_dir}")

        if self.model_ja is None and self.model_en is None:
            raise RuntimeError("At least one model (JA or EN) must be loaded")

    def predict_batch(
        self,
        texts: List[str],
        multi_label: bool = False,
        top_k: int = 5,
        threshold_overrides: Optional[Dict[str, float]] = None,
    ) -> List[Dict[str, Any]]:
        """Predict genres for a batch of texts.

        Args:
            texts: List of texts to classify
            multi_label: If True, returns all genres above threshold (up to top_k)
            top_k: Maximum number of genres to return per text
            threshold_overrides: Dictionary of genre-specific thresholds

        Returns:
            List of prediction dictionaries with "top_genre", "confidence", "scores", and optionally "candidates"
        """
        if not texts:
            return []

        # Detect language for each text
        lang_detections = [detect_language_simple(text) for text in texts]

        # Group texts by language
        ja_texts = []
        en_texts = []
        unknown_texts = []
        ja_indices = []
        en_indices = []
        unknown_indices = []

        for i, (text, lang) in enumerate(zip(texts, lang_detections)):
            if lang == "ja" and self.model_ja is not None:
                ja_texts.append(text)
                ja_indices.append(i)
            elif lang == "en" and self.model_en is not None:
                en_texts.append(text)
                en_indices.append(i)
            else:
                # Unknown or no model available - use fallback
                unknown_texts.append(text)
                unknown_indices.append(i)

        # Run inference for each language group
        results = [None] * len(texts)

        # Japanese predictions
        if ja_texts and self.model_ja:
            ja_results = self._predict_with_model(self.model_ja, ja_texts, multi_label, top_k, threshold_overrides)
            for idx, result in zip(ja_indices, ja_results):
                results[idx] = result

        # English predictions
        if en_texts and self.model_en:
            en_results = self._predict_with_model(self.model_en, en_texts, multi_label, top_k, threshold_overrides)
            for idx, result in zip(en_indices, en_results):
                results[idx] = result

        # Unknown/fallback: use Japanese model if available, otherwise English
        if unknown_texts:
            fallback_model = self.model_ja if self.model_ja else self.model_en
            if fallback_model:
                fallback_results = self._predict_with_model(fallback_model, unknown_texts, multi_label, top_k, threshold_overrides)
                for idx, result in zip(unknown_indices, fallback_results):
                    results[idx] = result
            else:
                # No model available - return empty predictions
                for idx in unknown_indices:
                    results[idx] = {
                        "top_genre": None,
                        "confidence": 0.0,
                        "scores": {},
                        "candidates": [],
                    }

        return results

    def _predict_with_model(
        self,
        model: StudentDistilBERT,
        texts: List[str],
        multi_label: bool,
        top_k: int,
        threshold_overrides: Optional[Dict[str, float]] = None,
    ) -> List[Dict[str, Any]]:
        """Run inference with a specific model."""
        with torch.no_grad():
            probs, logits = model.predict(texts, max_length=256, device=self.device)

        # Default threshold (can be made configurable)
        default_threshold = 0.3

        results = []
        for i, text in enumerate(texts):
            prob_dist = probs[i].cpu().numpy()

            # Get top predictions
            top_indices = prob_dist.argsort()[::-1][:top_k]

            # Build scores dictionary
            scores = {self.id2label[idx]: float(prob_dist[idx]) for idx in range(len(self.genres))}

            # Get top genre
            top_idx = top_indices[0]
            top_genre = self.id2label[top_idx]
            top_confidence = float(prob_dist[top_idx])

            # Build candidates list (for multi_label mode)
            candidates = []
            for idx in top_indices:
                genre = self.id2label[idx]
                score = float(prob_dist[idx])
                threshold = (threshold_overrides.get(genre, default_threshold)
                            if threshold_overrides is not None else default_threshold)

                if multi_label and score >= threshold:
                    candidates.append({
                        "genre": genre,
                        "score": score,
                        "confidence": score,
                    })

            # If not multi_label or no candidates above threshold, use top prediction
            if not multi_label or not candidates:
                candidates = [{
                    "genre": top_genre,
                    "score": top_confidence,
                    "confidence": top_confidence,
                }]

            results.append({
                "top_genre": top_genre,
                "confidence": top_confidence,
                "scores": scores,
                "candidates": candidates[:top_k],
            })

        return results

