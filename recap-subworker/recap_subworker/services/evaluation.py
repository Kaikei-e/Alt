import json
from pathlib import Path
from typing import Dict, List, Any, Optional

import pandas as pd
import structlog
from pydantic import BaseModel
from sklearn.metrics import classification_report, accuracy_score, hamming_loss, precision_recall_fscore_support, confusion_matrix
from sklearn.preprocessing import MultiLabelBinarizer
try:
    from statsmodels.stats.proportion import proportion_confint
except ImportError:
    proportion_confint = None

from ..infra.config import get_settings
from .classifier import GenreClassifierService, SudachiTokenizer
from .embedder import Embedder, EmbedderConfig

# Conditional import for deepeval
try:
    from deepeval.metrics import FaithfulnessMetric
    from deepeval.test_case import LLMTestCase
    DEEPEVAL_AVAILABLE = True
except ImportError:
    FaithfulnessMetric = None
    LLMTestCase = None
    DEEPEVAL_AVAILABLE = False

logger = structlog.get_logger()

class ClassificationMetrics(BaseModel):
    accuracy: float
    hamming_loss: float
    macro_precision: float
    macro_recall: float
    macro_f1: float
    micro_precision: float
    micro_recall: float
    micro_f1: float
    per_genre: Dict[str, Dict[str, float]]


class SummarizationMetrics(BaseModel):
    relevance: float = 0.0
    brevity: float = 0.0
    consistency: float = 0.0
    faithfulness: float = 0.0

class EvaluationService:
    def __init__(
        self,
        weights_path: Optional[str] = None,
        weights_ja_path: Optional[str] = None,
        weights_en_path: Optional[str] = None,
        vectorizer_ja_path: Optional[str] = None,
        vectorizer_en_path: Optional[str] = None,
        thresholds_ja_path: Optional[str] = None,
        thresholds_en_path: Optional[str] = None,
        use_bootstrap: bool = True,
        n_bootstrap: int = 1000,
        use_cross_validation: bool = False,
        n_folds: int = 5,
    ):
        self.settings = get_settings()
        # Fallback to default from settings if generic pointer is used,
        # but favor explicit JA/EN paths if provided
        self.weights_path = weights_path or self.settings.genre_classifier_model_path

        self.weights_ja = weights_ja_path
        self.weights_en = weights_en_path
        self.vectorizer_ja = vectorizer_ja_path
        self.vectorizer_en = vectorizer_en_path
        self.thresholds_ja = thresholds_ja_path
        self.thresholds_en = thresholds_en_path

        self.use_bootstrap = use_bootstrap
        self.n_bootstrap = n_bootstrap
        self.use_cross_validation = use_cross_validation
        self.n_folds = n_folds

        self.classifier_ja = None
        self.classifier_en = None
        self.classifier_default = None

        # Initialize components for evaluation
        self._init_classifiers()

    def _init_classifiers(self):
        # Configure Embedder
        config = EmbedderConfig(
            model_id=self.settings.model_id,
            distill_model_id=self.settings.distill_model_id,
            backend=self.settings.model_backend,
            device=self.settings.device,
            batch_size=self.settings.batch_size,
            cache_size=self.settings.embed_cache_size,
        )
        self.embedder = Embedder(config)

        # Initialize JA Classifier if config provided
        if self.weights_ja:
            logger.info("Initializing JA classifier", path=self.weights_ja)
            self.classifier_ja = GenreClassifierService(
                self.weights_ja,
                self.embedder,
                vectorizer_path=self.vectorizer_ja,
                thresholds_path=self.thresholds_ja
            )

        # Initialize EN Classifier if config provided
        if self.weights_en:
            logger.info("Initializing EN classifier", path=self.weights_en)
            self.classifier_en = GenreClassifierService(
                self.weights_en,
                self.embedder,
                vectorizer_path=self.vectorizer_en,
                thresholds_path=self.thresholds_en
            )

        # Initialize Default Classifier (fallback)
        if self.weights_path:
             # Note: Model path might be relative to app root or absolute
             model_path = Path(self.weights_path)
             if not model_path.is_absolute():
                  # Assuming purely relative to CWD if not absolute, or adapt as needed.
                  # In docker, CWD is /app.
                  pass
             self.classifier_default = GenreClassifierService(str(model_path), self.embedder)
             if not self.classifier_ja:
                 self.classifier_ja = self.classifier_default
             if not self.classifier_en:
                 self.classifier_en = self.classifier_default

    def evaluate(self, golden_data_path: str, language: Optional[str] = None) -> Dict[str, Any]:
        """
        Evaluate classifier against golden dataset.

        Args:
            golden_data_path: Path to golden classification JSON file
            language: Optional language filter ("ja" or "en"). If None, evaluates all languages.
        """
        golden_path = Path(golden_data_path)
        if not golden_path.exists():
             raise FileNotFoundError(f"Golden data not found: {golden_path}")

        # Load Golden Data
        with open(golden_path, "r") as f:
            data = json.load(f)

        # Handle wrapper structure
        if isinstance(data, dict) and "items" in data:
            items = data["items"]
        else:
            items = data

        # Filter by language if specified
        if language:
            filtered_items = []
            for item in items:
                # Check content_ja/content_en or lang field
                if language == "ja" and (item.get("content_ja") or item.get("lang") == "ja"):
                    filtered_items.append(item)
                elif language == "en" and (item.get("content_en") or item.get("lang") == "en"):
                    filtered_items.append(item)
            items = filtered_items
            logger.info(f"Filtered to {len(items)} items for language: {language}")

        # Expecting data format: list of {"text": "...", "labels": ["genre1", "genre2"]}
        # Or {"text": ..., "genres": ...}

        df = pd.DataFrame(items)

        # Map typical field names from our golden set
        # Handle language-specific content fields
        if language == "ja" and "content_ja" in df.columns:
            if "text" not in df.columns:
                df.rename(columns={"content_ja": "text"}, inplace=True)
        elif language == "en" and "content_en" in df.columns:
            if "text" not in df.columns:
                df.rename(columns={"content_en": "text"}, inplace=True)
        elif "content_ja" in df.columns and "text" not in df.columns:
            # Default to Japanese if both exist
            df.rename(columns={"content_ja": "text"}, inplace=True)
        elif "content" in df.columns and "text" not in df.columns:
            df.rename(columns={"content": "text"}, inplace=True)

        if "expected_genres" in df.columns and "labels" not in df.columns:
            df.rename(columns={"expected_genres": "labels"}, inplace=True)

        if "genres" in df.columns:
            df.rename(columns={"genres": "labels"}, inplace=True)

        if "labels" not in df.columns or "text" not in df.columns:
             raise ValueError(f"Golden data must contain 'text' and 'labels' fields. Available: {df.columns.tolist()}")

        # Prepare X and y_true
        X = df["text"].tolist()
        y_true_labels = df["labels"].tolist()

        # MultiLabelBinarizer for metrics
        mlb = MultiLabelBinarizer()
        y_true_bin = mlb.fit_transform(y_true_labels)
        classes = mlb.classes_

        if language == "ja":
            classifier = self.classifier_ja
        elif language == "en":
            classifier = self.classifier_en
        else:
            classifier = self.classifier_default or self.classifier_ja

        if not classifier:
            raise ValueError("No classifier available for language: " + str(language))

        # Predict
        # Use predict_batch from GenreClassifierService
        # It accepts threshold_overrides, etc. We use defaults for now.
        logger.info("Starting evaluation prediction", count=len(X), language=language)
        predictions = classifier.predict_batch(X, multi_label=True)

        # Extract predicted labels
        # predictions[i]["candidates"] contains list of {"genre": str, "score": float, ...} for all pass-threshold genres
        y_pred_labels = []
        for p in predictions:
            # candidates are sorted by score.
            # In multi-label, we take all candidates.
            pred_genres = [c["genre"] for c in p.get("candidates", [])]
            y_pred_labels.append(pred_genres)

        y_pred_bin = mlb.transform(y_pred_labels)

        # Calculate Metrics
        # Re-use evaluate_classification for standard metrics
        metrics = self.evaluate_classification(y_true_bin, y_pred_bin, target_names=list(classes))

        # Construct full result dict similar to what router expects
        # Router expects: accuracy, macro_f1, per_genre_metrics, etc.
        # plus CI intervals if requested (bootstrap).

        results = {
            "accuracy": metrics.accuracy,
            "macro_precision": metrics.macro_precision,
            "macro_recall": metrics.macro_recall,
            "macro_f1": metrics.macro_f1,
            "micro_precision": metrics.micro_precision,
            "micro_recall": metrics.micro_recall,
            "micro_f1": metrics.micro_f1,
            "per_genre_metrics": {},
            "confusion_matrix": {}, # TODO if needed
            "total_samples": len(X),
            "total_tp": 0,
            "total_fp": 0,
            "total_fn": 0,
        }

        # Detail per genre
        # metrics.per_genre has structure from classification_report: {genre: {'precision': ..., 'recall': ..., 'f1-score': ..., 'support': ...}}
        for genre in classes:
            if genre in metrics.per_genre:
                 m = metrics.per_genre[genre]
                 results["per_genre_metrics"][genre] = {
                     "precision": m["precision"],
                     "recall": m["recall"],
                     "f1": m["f1-score"],
                     "support": m["support"],
                     "tp": 0, "fp": 0, "fn": 0, # TODO: calculate these if strictly needed for UI
                     "threshold": 0.5 # Default
                 }

        # Bootstrap for CI (Simplistic version) -> SWITCHING to Wilson Score Interval for Accuracy
        if self.use_bootstrap and proportion_confint:
             # Calculate CI for accuracy using Wilson score interval
             count_correct = int(metrics.accuracy * len(X))
             n_obs = len(X)
             lower, upper = proportion_confint(count_correct, n_obs, alpha=0.05, method='wilson')
             width = upper - lower

             results["accuracy_ci"] = {
                 "point": metrics.accuracy,
                 "lower": lower,
                 "upper": upper,
                 "width": width
             }

             # For Macro F1, analytic CI is complex. We stick to point estimate or simple bootstrap if really needed.
             # User specifically asked for Accuracy CI to not be width 0.
             results["macro_metrics"] = {
                  "precision": 0.0, "precision_ci": {"point":0,"lower":0,"upper":0},
                  "recall": 0.0, "recall_ci": {"point":0,"lower":0,"upper":0},
                  "f1": metrics.macro_f1, "f1_ci": {"point":metrics.macro_f1,"lower":metrics.macro_f1,"upper":metrics.macro_f1}
             }
        else:
             results["accuracy_ci"] = {"point": metrics.accuracy, "lower": metrics.accuracy, "upper": metrics.accuracy, "width": 0.0}

        # Confusion Matrix (Top-1 approximation)
        try:
            # Ground Truth: Take first label or 'other'
            y_true_single = [labels[0] if labels else 'other' for labels in y_true_labels]

            # Prediction: Use 'top_genre' from predictions
            y_pred_single = [p.get("top_genre", "other") for p in predictions]

            # Compute Matrix
            # Use sorted unique labels from both true and pred to cover all cases
            unique_labels = sorted(list(set(y_true_single) | set(y_pred_single)))
            cm = confusion_matrix(y_true_single, y_pred_single, labels=unique_labels)

            results["confusion_matrix"] = {
                "labels": unique_labels,
                "matrix": cm.tolist()
            }
        except Exception as e:
            logger.warning("Failed to generate confusion matrix", error=str(e))
            results["confusion_matrix"] = {}

        # Add language metadata if filtered
        if language:
            results["language"] = language

        return results

    def evaluate_by_language(self, golden_data_path: str) -> Dict[str, Any]:
        """Evaluate classifier separately for each language (ja, en).

        Returns:
            Dictionary with "ja" and "en" keys containing evaluation results for each language.
        """
        results = {}

        for lang in ["ja", "en"]:
            try:
                lang_results = self.evaluate(golden_data_path, language=lang)
                results[lang] = lang_results
            except Exception as e:
                logger.warning(f"Failed to evaluate for language {lang}: {e}")
                results[lang] = {"error": str(e)}

        return results

    def evaluate_classification(
        self,
        y_true: List[List[int]],
        y_pred: List[List[int]],
        target_names: Optional[List[str]] = None
    ) -> ClassificationMetrics:
        """
        Evaluate multi-label classification performance.
        y_true, y_pred: List of binary vectors indicating genre presence.
        """
        # Calculate standard metrics
        acc = accuracy_score(y_true, y_pred)
        hl = hamming_loss(y_true, y_pred)

        # Classification report returns a dict with 'macro avg', 'micro avg', 'weighted avg', and per-class label
        report = classification_report(
            y_true,
            y_pred,
            target_names=target_names,
            output_dict=True,
            zero_division=0
        )

        # Calculate macro/micro using precision_recall_fscore_support for reliability
        macro_p, macro_r, macro_f, _ = precision_recall_fscore_support(y_true, y_pred, average='macro', zero_division=0)
        micro_p, micro_r, micro_f, _ = precision_recall_fscore_support(y_true, y_pred, average='micro', zero_division=0)

        # Update report structure implicitly or explicit return
        # Per genre metrics
        per_genre = {k: v for k, v in report.items() if k not in ['macro avg', 'micro avg', 'weighted avg', 'samples']}

        return ClassificationMetrics(
            accuracy=acc,
            hamming_loss=hl,
            macro_precision=macro_p,
            macro_recall=macro_r,
            macro_f1=macro_f,
            micro_precision=micro_p,
            micro_recall=micro_r,
            micro_f1=micro_f,
            per_genre=per_genre
        )

    async def evaluate_summary(
        self,
        source_text: str,
        generated_summary: str,
        context: Optional[List[str]] = None
    ) -> SummarizationMetrics:
        """
        Evaluate summary quality using DeepEval metrics if available.
        Note: This is potentially slow and costly (LLM calls).
        """
        metrics = SummarizationMetrics()

        if not DEEPEVAL_AVAILABLE:
            logger.warning("DeepEval not available. Skipping LLM-based summary evaluation.")
            return metrics

        try:
            test_case = LLMTestCase(
                input=source_text,
                actual_output=generated_summary,
                retrieval_context=[source_text] if not context else context
            )

            faithfulness = FaithfulnessMetric(threshold=0.5, include_reason=False)
            faithfulness.measure(test_case)
            metrics.faithfulness = faithfulness.score

            metrics.brevity = len(generated_summary) / (len(source_text) + 1)

        except Exception as e:
            logger.error("Error during summary evaluation", error=str(e))

        return metrics
