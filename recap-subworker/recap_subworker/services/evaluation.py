from typing import Dict, List, Any, Optional
from pydantic import BaseModel, Field
from sklearn.metrics import classification_report, accuracy_score, hamming_loss
import structlog

# Conditional import for deepeval to avoid potential crash if not installed/configured in all envs immediately
try:
    from deepeval.metrics import FaithfulnessMetric, ContextualPrecisionMetric
    from deepeval.test_case import LLMTestCase
    DEEPEVAL_AVAILABLE = True
except ImportError:
    FaithfulnessMetric = None
    ContextualPrecisionMetric = None
    LLMTestCase = None
    DEEPEVAL_AVAILABLE = False

logger = structlog.get_logger()

class ClassificationMetrics(BaseModel):
    accuracy: float
    hamming_loss: float
    macro_f1: float
    micro_f1: float
    per_genre: Dict[str, Dict[str, float]]

class SummarizationMetrics(BaseModel):
    relevance: float = 0.0
    brevity: float = 0.0 # Not standard deepeval, we might calculate this manually
    consistency: float = 0.0
    faithfulness: float = 0.0

class EvaluationService:
    def __init__(self):
        pass

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

        return ClassificationMetrics(
            accuracy=acc,
            hamming_loss=hl,
            macro_f1=report['macro avg']['f1-score'],
            micro_f1=report['micro avg']['f1-score'],
            per_genre={k: v for k, v in report.items() if k not in ['macro avg', 'micro avg', 'weighted avg', 'samples']}
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
            # Construct test case
            # Context usually represents the retrieval context. For summarization, source_text is the input.
            # DeepEval FaithfulnessMetric needs 'retrieval_context' to check against.
            # Here we assume 'source_text' is the truth.
            # If source_text is very long, we might need to chunk it, but let's assume it fits for now or is passed as context.

            # Using source_text as retrieval_context for faithfulness
            test_case = LLMTestCase(
                input=source_text,
                actual_output=generated_summary,
                retrieval_context=[source_text] if not context else context
            )

            # 1. Faithfulness
            faithfulness = FaithfulnessMetric(threshold=0.5, include_reason=False)
            faithfulness.measure(test_case)
            metrics.faithfulness = faithfulness.score

            # 2. Relevance (ContextualPrecision might be a proxy, or we use a custom rubric)
            # For strict Summarization relevance, DeepEval has SummarizationMetric but it's often for RAG.
            # We will try to map available metrics.
            # Let's use a simpler heuristic for now or skip if too complex to setup without OpenAI key conf.

            # For Brevity, we can just measure compression ratio or similar non-LLM metric for now to save cost.
            metrics.brevity = len(generated_summary) / (len(source_text) + 1)

        except Exception as e:
            logger.error("Error during summary evaluation", error=str(e))

        return metrics
