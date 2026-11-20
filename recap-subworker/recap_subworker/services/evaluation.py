import json
from pathlib import Path
from typing import Any

import pandas as pd
import structlog
from pydantic import BaseModel
from sklearn.metrics import (
    accuracy_score,
    classification_report,
    confusion_matrix,
    hamming_loss,
    precision_recall_fscore_support,
)
from sklearn.preprocessing import MultiLabelBinarizer

try:
    from statsmodels.stats.proportion import proportion_confint
except ImportError:
    proportion_confint = None

import numpy as np
from sklearn.base import clone
from sklearn.model_selection import StratifiedKFold

from ..infra.config import get_settings
from ..infra.path_validation import validate_path
from .classifier import GenreClassifierService
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
    per_genre: dict[str, dict[str, float]]


class SummarizationMetrics(BaseModel):
    relevance: float = 0.0
    brevity: float = 0.0
    consistency: float = 0.0
    faithfulness: float = 0.0


class EvaluationService:
    def __init__(  # noqa: PLR0913 - constructor injection of model paths and evaluation config; callers outside services/
        self,
        weights_path: str | None = None,
        weights_ja_path: str | None = None,
        weights_en_path: str | None = None,
        vectorizer_ja_path: str | None = None,
        vectorizer_en_path: str | None = None,
        thresholds_ja_path: str | None = None,
        thresholds_en_path: str | None = None,
        use_bootstrap: bool = True,
        n_bootstrap: int = 1000,
        use_cross_validation: bool = False,
        n_folds: int = 5,
        embedder: Embedder | None = None,
    ) -> None:
        self.settings = get_settings()
        # Validate caller-supplied weights_path override against ALLOWED_BASE_DIRS
        # so arbitrary filesystem paths are rejected (CWE-22), while preserving
        # Settings-derived paths for container initialization.
        if weights_path is not None:
            self.weights_path = str(validate_path(str(weights_path)))
        else:
            self.weights_path = self.settings.genre_classifier_model_path

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

        self.classifier_ja: GenreClassifierService | None = None
        self.classifier_en: GenreClassifierService | None = None
        self.classifier_default: GenreClassifierService | None = None
        self.embedder: Embedder

        # Initialize components for evaluation
        self._init_classifiers(embedder)

    def _init_classifiers(self, embedder: Embedder | None = None) -> None:
        # Reuse an injected (container-managed) Embedder when available to avoid
        # loading the embedding model a second time; only construct a fresh one
        # when the caller has no container to inject from.
        if embedder is not None:
            self.embedder = embedder
        else:
            self.embedder = Embedder(EmbedderConfig.from_settings(self.settings))

        # Initialize JA Classifier if config provided
        if self.weights_ja:
            logger.info("Initializing JA classifier", path=self.weights_ja)
            self.classifier_ja = GenreClassifierService(
                self.weights_ja,
                self.embedder,
                vectorizer_path=self.vectorizer_ja,
                thresholds_path=self.thresholds_ja,
            )

        # Initialize EN Classifier if config provided
        if self.weights_en:
            logger.info("Initializing EN classifier", path=self.weights_en)
            self.classifier_en = GenreClassifierService(
                self.weights_en,
                self.embedder,
                vectorizer_path=self.vectorizer_en,
                thresholds_path=self.thresholds_en,
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

    @staticmethod
    def _prepare_evaluation_inputs(
        df: pd.DataFrame,
    ) -> tuple[list[str], list[list[str]]]:
        """Filter NaN / None / whitespace-only text rows from the golden frame.

        sklearn's TfidfVectorizer.transform rejects NaN documents with
        ValueError("np.nan is an invalid document"). Dropping the offending rows
        here turns a job-level 400 into a row-level silent skip that still
        produces partial evaluation metrics. Labels are kept aligned with texts.
        """
        texts_series = df["text"]
        labels_series = df["labels"]
        mask = texts_series.notna() & texts_series.astype(str).str.strip().ne("")
        kept_texts = [str(t) for t in texts_series[mask].tolist()]
        kept_labels = [list(lbl) for lbl in labels_series[mask].tolist()]
        return kept_texts, kept_labels

    @staticmethod
    def _load_golden_items(
        golden_data_path: str | Path, language: str | None
    ) -> list[dict[str, Any]]:
        """Validate path, load golden JSON items, and optionally filter by language."""
        if isinstance(golden_data_path, Path):
            golden_path = golden_data_path  # 既にバリデーション済み
        else:
            golden_path = validate_path(golden_data_path)  # defense-in-depth
        # codeql[py/path-injection] -- validate_path applies realpath + allow-list; Path inputs are pre-validated by the router
        if not golden_path.exists():
            msg = f"Golden data not found: {golden_path}"
            raise FileNotFoundError(msg)

        # codeql[py/path-injection] -- path already validated above
        with golden_path.open() as f:
            data = json.load(f)

        items = data["items"] if isinstance(data, dict) and "items" in data else data
        if not language:
            return items

        content_field = "content_ja" if language == "ja" else "content_en"
        if language in {"ja", "en"}:
            filtered_items = [
                item
                for item in items
                if isinstance(item.get(content_field), str) and item.get(content_field, "").strip()
            ]
        else:
            filtered_items = []
        logger.info(
            "Filtered golden items by language",
            item_count=len(filtered_items),
            language=language,
        )
        return filtered_items

    @staticmethod
    def _normalize_golden_dataframe(
        items: list[dict[str, Any]], language: str | None
    ) -> pd.DataFrame:
        """Build golden DataFrame and normalize text/labels column names."""
        df = pd.DataFrame(items)
        if "text" not in df.columns:
            text_candidates = (
                ["content_ja"]
                if language == "ja"
                else ["content_en"]
                if language == "en"
                else ["content_ja", "content"]
            )
            for col in text_candidates:
                if col in df.columns:
                    df.rename(columns={col: "text"}, inplace=True)
                    break

        if "expected_genres" in df.columns and "labels" not in df.columns:
            df.rename(columns={"expected_genres": "labels"}, inplace=True)
        if "genres" in df.columns:
            df.rename(columns={"genres": "labels"}, inplace=True)

        if "labels" not in df.columns or "text" not in df.columns:
            msg = f"Golden data must contain 'text' and 'labels' fields. Available: {df.columns.tolist()}"
            raise ValueError(msg)
        return df

    def _resolve_classifier(self, language: str | None) -> GenreClassifierService:
        """Select the classifier for the requested language or raise ValueError."""
        if language == "ja":
            classifier = self.classifier_ja
        elif language == "en":
            classifier = self.classifier_en
        else:
            classifier = self.classifier_default or self.classifier_ja

        if not classifier:
            msg = "No classifier available for language: " + str(language)
            raise ValueError(msg)
        return classifier

    @staticmethod
    def _build_evaluation_results(
        metrics: ClassificationMetrics,
        classes: np.ndarray,
        total_samples: int,
    ) -> dict[str, Any]:
        """Assemble base classification metrics and per-genre breakdown."""
        results: dict[str, Any] = {
            "accuracy": metrics.accuracy,
            "macro_precision": metrics.macro_precision,
            "macro_recall": metrics.macro_recall,
            "macro_f1": metrics.macro_f1,
            "micro_precision": metrics.micro_precision,
            "micro_recall": metrics.micro_recall,
            "micro_f1": metrics.micro_f1,
            "per_genre_metrics": {},
            "confusion_matrix": {},  # TODO if needed
            "total_samples": total_samples,
            "total_tp": 0,
            "total_fp": 0,
            "total_fn": 0,
        }
        for genre in classes:
            if genre in metrics.per_genre:
                m = metrics.per_genre[genre]
                results["per_genre_metrics"][genre] = {
                    "precision": m["precision"],
                    "recall": m["recall"],
                    "f1": m["f1-score"],
                    "support": m["support"],
                    "tp": 0,
                    "fp": 0,
                    "fn": 0,  # TODO: calculate these if strictly needed for UI
                    "threshold": 0.5,  # Default
                }
        return results

    @staticmethod
    def _apply_confidence_intervals(
        results: dict[str, Any],
        metrics: ClassificationMetrics,
        n_obs: int,
        *,
        use_bootstrap: bool,
    ) -> None:
        """Populate Wilson score CI and macro metrics onto results in-place."""
        if use_bootstrap and proportion_confint:
            count_correct = int(metrics.accuracy * n_obs)
            lower, upper = proportion_confint(count_correct, n_obs, alpha=0.05, method="wilson")
            results["accuracy_ci"] = {
                "point": metrics.accuracy,
                "lower": lower,
                "upper": upper,
                "width": upper - lower,
            }
            results["macro_metrics"] = {
                "precision": 0.0,
                "precision_ci": {"point": 0, "lower": 0, "upper": 0},
                "recall": 0.0,
                "recall_ci": {"point": 0, "lower": 0, "upper": 0},
                "f1": metrics.macro_f1,
                "f1_ci": {
                    "point": metrics.macro_f1,
                    "lower": metrics.macro_f1,
                    "upper": metrics.macro_f1,
                },
            }
            return

        results["accuracy_ci"] = {
            "point": metrics.accuracy,
            "lower": metrics.accuracy,
            "upper": metrics.accuracy,
            "width": 0.0,
        }

    @staticmethod
    def _compute_confusion_matrix(
        y_true_labels: list[list[str]],
        predictions: list[dict[str, Any]],
    ) -> dict[str, Any]:
        """Compute top-1 confusion matrix, returning empty dict on failure."""
        try:
            y_true_single = [labels[0] if labels else "other" for labels in y_true_labels]
            y_pred_single = [p.get("top_genre", "other") for p in predictions]
            unique_labels = sorted(set(y_true_single) | set(y_pred_single))
            cm = confusion_matrix(y_true_single, y_pred_single, labels=unique_labels)
        except Exception as e:  # noqa: BLE001 - confusion matrix is diagnostic; failure must not abort evaluation
            logger.warning("Failed to generate confusion matrix", error=str(e))
            return {}
        else:
            return {"labels": unique_labels, "matrix": cm.tolist()}

    def evaluate(
        self,
        golden_data_path: str | Path,
        language: str | None = None,
        *,
        use_bootstrap: bool | None = None,
        use_cross_validation: bool | None = None,
        n_folds: int | None = None,
    ) -> dict[str, Any]:
        """
        Evaluate classifier against golden dataset.

        Args:
            golden_data_path: Path to golden classification JSON file
            language: Optional language filter ("ja" or "en"). If None, evaluates all languages.
            use_bootstrap: Per-call override for whether to compute the Wilson
                score CI (defaults to the value passed to the constructor).
                Accepted as a call argument rather than mutating instance
                state so a shared/cached EvaluationService instance stays
                safe to reuse across concurrent requests with different
                per-request evaluation options.
            use_cross_validation: Per-call override for whether to run
                Stratified K-Fold CV.
            n_folds: Per-call override for the number of CV folds.
        """
        use_bootstrap = self.use_bootstrap if use_bootstrap is None else use_bootstrap
        use_cross_validation = (
            self.use_cross_validation if use_cross_validation is None else use_cross_validation
        )
        n_folds = self.n_folds if n_folds is None else n_folds

        items = self._load_golden_items(golden_data_path, language)
        df = self._normalize_golden_dataframe(items, language)

        total_rows = len(df)
        X, y_true_labels = self._prepare_evaluation_inputs(df)  # noqa: N806 - ML feature matrix convention
        dropped_rows = total_rows - len(X)
        if dropped_rows:
            logger.warning(
                "evaluation: dropped rows with empty/NaN text before prediction",
                dropped_rows=dropped_rows,
                kept_rows=len(X),
            )

        mlb = MultiLabelBinarizer()
        y_true_bin = mlb.fit_transform(y_true_labels)
        classes = mlb.classes_

        classifier = self._resolve_classifier(language)
        logger.info("Starting evaluation prediction", count=len(X), language=language)
        predictions = classifier.predict_batch(X, multi_label=True)

        y_pred_labels = [[c["genre"] for c in p.get("candidates", [])] for p in predictions]
        y_pred_bin = mlb.transform(y_pred_labels)
        metrics = self.evaluate_classification(y_true_bin, y_pred_bin, target_names=list(classes))

        results = self._build_evaluation_results(metrics, classes, len(X))
        self._apply_confidence_intervals(results, metrics, len(X), use_bootstrap=use_bootstrap)
        results["confusion_matrix"] = self._compute_confusion_matrix(y_true_labels, predictions)

        if use_cross_validation and n_folds > 1:
            try:
                logger.info("Starting Cross-Validation", n_folds=n_folds)
                cv_metrics = self._run_cross_validation(
                    X, y_true_labels, classifier, n_folds=n_folds
                )
                results.update(cv_metrics)
            except Exception:
                logger.exception("Cross-validation failed")

        if language:
            results["language"] = language

        return results

    def evaluate_by_language(self, golden_data_path: str | Path) -> dict[str, Any]:
        """Evaluate classifier separately for each language (ja, en).

        Returns:
            Dictionary with "ja" and "en" keys containing evaluation results for each language.
        """
        results = {}

        for lang in ["ja", "en"]:
            try:
                lang_results = self.evaluate(golden_data_path, language=lang)
                results[lang] = lang_results
            except Exception as e:  # noqa: BLE001 - per-language failure is recorded in result dict rather than aborting other languages
                logger.warning(
                    "Failed to evaluate for language",
                    lang=lang,
                    error=str(e),
                )
                results[lang] = {"error": str(e)}

        return results

    def evaluate_classification(
        self,
        y_true: list[list[int]],
        y_pred: list[list[int]],
        target_names: list[str] | None = None,
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
            y_true, y_pred, target_names=target_names, output_dict=True, zero_division=0
        )

        # Calculate macro/micro using precision_recall_fscore_support for reliability
        macro_p, macro_r, macro_f, _ = precision_recall_fscore_support(
            y_true, y_pred, average="macro", zero_division=0
        )
        micro_p, micro_r, micro_f, _ = precision_recall_fscore_support(
            y_true, y_pred, average="micro", zero_division=0
        )

        # Update report structure implicitly or explicit return
        # Per genre metrics
        per_genre = {
            k: v
            for k, v in report.items()
            if k not in ["macro avg", "micro avg", "weighted avg", "samples"]
        }

        return ClassificationMetrics(
            accuracy=acc,
            hamming_loss=hl,
            macro_precision=macro_p,
            macro_recall=macro_r,
            macro_f1=macro_f,
            micro_precision=micro_p,
            micro_recall=micro_r,
            micro_f1=micro_f,
            per_genre=per_genre,
        )

    async def evaluate_summary(
        self, source_text: str, generated_summary: str, context: list[str] | None = None
    ) -> SummarizationMetrics:
        """
        Evaluate summary quality using DeepEval metrics if available.
        Note: This is potentially slow and costly (LLM calls).
        """
        metrics = SummarizationMetrics()

        if not DEEPEVAL_AVAILABLE:
            logger.warning("DeepEval not available. Skipping LLM-based summary evaluation.")
            return metrics

        if LLMTestCase is None or FaithfulnessMetric is None:
            msg = "DeepEval is not available"
            raise RuntimeError(msg)

        try:
            test_case = LLMTestCase(
                input=source_text,
                actual_output=generated_summary,
                retrieval_context=context if context else [source_text],
            )

            faithfulness = FaithfulnessMetric(threshold=0.5, include_reason=False)
            faithfulness.measure(test_case)
            metrics.faithfulness = faithfulness.score

            metrics.brevity = len(generated_summary) / (len(source_text) + 1)

        except Exception as e:
            logger.exception("Error during summary evaluation", error=str(e))

        return metrics

    def _run_cross_validation(
        self,
        texts: list[str],
        y_true_labels: list[list[str]],
        classifier: GenreClassifierService,
        *,
        n_folds: int | None = None,
    ) -> dict[str, Any]:
        """
        Run Stratified K-Fold CV by retraining the model on folds.
        Approximates the training pipeline: Embed + TF-IDF -> LogisticRegression.
        """
        n_folds = self.n_folds if n_folds is None else n_folds
        # 1. Prepare Features (Once)
        logger.info("Generating features for CV...")
        input_texts = [f"passage: {t}" for t in texts]
        embeddings = classifier.embedder.encode(input_texts)

        features = embeddings
        if classifier.tfidf:
            tfidf_features = classifier.tfidf.transform(texts)
            features = np.hstack((embeddings, tfidf_features.toarray()))

        # 2. Prepare Labels (Single Label for Stratification/Training)
        # Use first label as primary
        y_single = [labels[0] if labels else "other" for labels in y_true_labels]
        y_single = np.array(y_single)

        # 3. Stratified K-Fold
        skf = StratifiedKFold(n_splits=n_folds, shuffle=True, random_state=42)

        accuracies = []
        macro_f1s = []
        micro_f1s = []

        mlb = MultiLabelBinarizer()
        # Ensure MLB uses all classes from the FULL dataset (or classifier classes)
        # Using classifier.model.classes_ is safest to match production schema,
        # but here we check against ground truth labels.
        # Let's fit MLB on all y_true_labels
        mlb.fit(y_true_labels)

        for fold, (train_index, test_index) in enumerate(skf.split(features, y_single), start=1):
            X_train, X_test = features[train_index], features[test_index]  # noqa: N806 - ML feature matrix convention
            y_train_single = y_single[train_index]

            # True labels for this fold (Test) - Multi-label
            y_test_true_labels = [y_true_labels[i] for i in test_index]
            y_test_bin = mlb.transform(y_test_true_labels)

            # Clone and Train Model
            # We assume classifier.model is a LogisticRegression (or compatible)
            model = clone(classifier.model)
            model.fit(X_train, y_train_single)

            # Predict Probas
            probs_batch = model.predict_proba(X_test)
            classes = model.classes_

            # Apply Thresholds (Reuse current thresholds)
            y_pred_labels = []
            for probs in probs_batch:
                scores = {cls: float(prob) for cls, prob in zip(classes, probs, strict=False)}

                candidates = []
                for cls, score in scores.items():
                    threshold = classifier.current_thresholds.get(cls, 0.5)
                    if score >= threshold:
                        candidates.append((cls, score))

                candidates.sort(key=lambda x: x[1], reverse=True)
                # If using top_k logic from predict_batch?
                # Let's keep all > threshold
                preds = [c[0] for c in candidates]
                y_pred_labels.append(preds)

            y_pred_bin = mlb.transform(y_pred_labels)

            # Calculate Metrics for this fold
            acc = accuracy_score(y_test_bin, y_pred_bin)
            _, _, macro_f, _ = precision_recall_fscore_support(
                y_test_bin, y_pred_bin, average="macro", zero_division=0
            )
            _, _, micro_f, _ = precision_recall_fscore_support(
                y_test_bin, y_pred_bin, average="micro", zero_division=0
            )

            accuracies.append(acc)
            macro_f1s.append(macro_f)
            micro_f1s.append(micro_f)

            logger.info("Fold finished", fold=fold, accuracy=acc, macro_f1=macro_f)

        return {
            "cv_accuracy": {"mean": float(np.mean(accuracies)), "std": float(np.std(accuracies))},
            "cv_macro_f1": {"mean": float(np.mean(macro_f1s)), "std": float(np.std(macro_f1s))},
            "cv_micro_f1": {"mean": float(np.mean(micro_f1s)), "std": float(np.std(micro_f1s))},
            "n_folds": n_folds,
        }
