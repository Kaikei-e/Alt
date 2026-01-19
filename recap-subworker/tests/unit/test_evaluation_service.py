import pytest
from unittest.mock import MagicMock, patch
from recap_subworker.services.evaluation import EvaluationService

@pytest.fixture
def evaluation_service():
    return EvaluationService()

def test_evaluate_classification(evaluation_service):
    y_true = [[0, 1], [1, 0], [0, 1]]
    y_pred = [[0, 1], [1, 1], [0, 0]]
    target_names = ["GenreA", "GenreB"]

    metrics = evaluation_service.evaluate_classification(y_true, y_pred, target_names=target_names)

    assert metrics.accuracy > 0
    assert metrics.hamming_loss >= 0
    assert metrics.per_genre["GenreA"]["f1-score"] >= 0
    assert metrics.per_genre["GenreB"]["f1-score"] >= 0

@pytest.mark.asyncio
async def test_evaluate_summary_no_deepeval(evaluation_service):
    # Mock DEEPEVAL_AVAILABLE to False
    with patch("recap_subworker.services.evaluation.DEEPEVAL_AVAILABLE", False):
        metrics = await evaluation_service.evaluate_summary("source", "summary")
        assert metrics.faithfulness == 0.0
        assert metrics.brevity == 0.0

@pytest.mark.asyncio
@patch("recap_subworker.services.evaluation.DEEPEVAL_AVAILABLE", True)
@patch("recap_subworker.services.evaluation.FaithfulnessMetric")
@patch("recap_subworker.services.evaluation.LLMTestCase")
async def test_evaluate_summary_with_deepeval_mock(mock_test_case, mock_faithfulness, evaluation_service):
    # Verify that if deepeval is available, we try to use it

    # Setup mocks
    mock_metric_instance = MagicMock()
    mock_metric_instance.score = 0.85
    mock_faithfulness.return_value = mock_metric_instance

    metrics = await evaluation_service.evaluate_summary("source text", "summary text")

    assert metrics.faithfulness == 0.85
    # Brevity is calculated simply length ratio in our implementation
    expected_brevity = len("summary text") / (len("source text") + 1)
    assert metrics.brevity == expected_brevity

    mock_faithfulness.assert_called_once()
    mock_metric_instance.measure.assert_called_once()


class TestEvaluatePathValidation:
    """Tests for path validation in evaluate() method - defense in depth."""

    def test_evaluate_rejects_path_traversal(self, evaluation_service):
        """Path traversal attempts should be rejected with ValueError."""
        with pytest.raises(ValueError, match="not within allowed directories"):
            evaluation_service.evaluate("../../../etc/passwd")

    def test_evaluate_rejects_absolute_path_outside_allowed(self, evaluation_service):
        """Absolute paths outside allowed directories should be rejected."""
        with pytest.raises(ValueError, match="not within allowed directories"):
            evaluation_service.evaluate("/etc/passwd")

    def test_evaluate_by_language_rejects_path_traversal(self, evaluation_service):
        """evaluate_by_language should also reject path traversal attempts."""
        # evaluate_by_language catches exceptions per-language and returns error dict
        results = evaluation_service.evaluate_by_language("../../../etc/passwd")
        # Both ja and en should have error containing the validation message
        assert "error" in results["ja"]
        assert "not within allowed directories" in results["ja"]["error"]
        assert "error" in results["en"]
        assert "not within allowed directories" in results["en"]["error"]
