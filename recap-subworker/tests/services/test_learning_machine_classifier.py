"""Tests for LearningMachineStudentClassifier below_threshold flag."""

import pytest
import numpy as np
import torch
from unittest.mock import MagicMock, patch


@pytest.fixture
def mock_student_model():
    """Create a mock StudentDistilBERT model."""
    model = MagicMock()
    model.eval = MagicMock()
    model.to = MagicMock(return_value=model)
    return model


@pytest.fixture
def taxonomy_file(tmp_path):
    """Create a temporary taxonomy file."""
    taxonomy = tmp_path / "genres.yaml"
    taxonomy.write_text(
        "genres:\n"
        "  - ai_data\n"
        "  - cybersecurity\n"
        "  - diplomacy_security\n"
        "  - tech_industry\n"
    )
    return str(taxonomy)


class TestBelowThresholdFlag:
    """Tests for the below_threshold flag in _predict_with_model output."""

    def test_below_threshold_true_when_confidence_low(
        self, mock_student_model, taxonomy_file
    ):
        """When top confidence < default_threshold (0.3), below_threshold should be True."""
        # Simulate low confidence: all scores near uniform
        probs = torch.tensor([[0.10, 0.08, 0.07, 0.05]])
        logits = torch.tensor([[0.0, 0.0, 0.0, 0.0]])
        mock_student_model.predict = MagicMock(return_value=(probs, logits))

        with patch(
            "recap_subworker.services.learning_machine_classifier.StudentDistilBERT"
        ):
            from recap_subworker.services.learning_machine_classifier import (
                LearningMachineStudentClassifier,
            )

            classifier = LearningMachineStudentClassifier.__new__(
                LearningMachineStudentClassifier
            )
            classifier.device = torch.device("cpu")
            classifier.genres = ["ai_data", "cybersecurity", "diplomacy_security", "tech_industry"]
            classifier.id2label = {i: g for i, g in enumerate(classifier.genres)}
            classifier.label2id = {g: i for i, g in enumerate(classifier.genres)}
            classifier.num_labels = len(classifier.genres)
            classifier.model_ja = mock_student_model
            classifier.model_en = None

            results = classifier._predict_with_model(
                mock_student_model, ["test text"], multi_label=False, top_k=5
            )

        assert len(results) == 1
        result = results[0]
        assert "below_threshold" in result
        assert result["below_threshold"] is True
        assert result["confidence"] < 0.3

    def test_below_threshold_false_when_confidence_high(
        self, mock_student_model, taxonomy_file
    ):
        """When top confidence >= default_threshold (0.3), below_threshold should be False."""
        # Simulate high confidence: one dominant class
        probs = torch.tensor([[0.85, 0.05, 0.05, 0.05]])
        logits = torch.tensor([[0.0, 0.0, 0.0, 0.0]])
        mock_student_model.predict = MagicMock(return_value=(probs, logits))

        with patch(
            "recap_subworker.services.learning_machine_classifier.StudentDistilBERT"
        ):
            from recap_subworker.services.learning_machine_classifier import (
                LearningMachineStudentClassifier,
            )

            classifier = LearningMachineStudentClassifier.__new__(
                LearningMachineStudentClassifier
            )
            classifier.device = torch.device("cpu")
            classifier.genres = ["ai_data", "cybersecurity", "diplomacy_security", "tech_industry"]
            classifier.id2label = {i: g for i, g in enumerate(classifier.genres)}
            classifier.label2id = {g: i for i, g in enumerate(classifier.genres)}
            classifier.num_labels = len(classifier.genres)
            classifier.model_ja = mock_student_model
            classifier.model_en = None

            results = classifier._predict_with_model(
                mock_student_model, ["test text"], multi_label=False, top_k=5
            )

        assert len(results) == 1
        result = results[0]
        assert "below_threshold" in result
        assert result["below_threshold"] is False
        assert result["confidence"] >= 0.3

    def test_below_threshold_at_boundary(self, mock_student_model, taxonomy_file):
        """When top confidence == 0.3 (exactly at threshold), below_threshold should be False."""
        probs = torch.tensor([[0.30, 0.25, 0.25, 0.20]])
        logits = torch.tensor([[0.0, 0.0, 0.0, 0.0]])
        mock_student_model.predict = MagicMock(return_value=(probs, logits))

        with patch(
            "recap_subworker.services.learning_machine_classifier.StudentDistilBERT"
        ):
            from recap_subworker.services.learning_machine_classifier import (
                LearningMachineStudentClassifier,
            )

            classifier = LearningMachineStudentClassifier.__new__(
                LearningMachineStudentClassifier
            )
            classifier.device = torch.device("cpu")
            classifier.genres = ["ai_data", "cybersecurity", "diplomacy_security", "tech_industry"]
            classifier.id2label = {i: g for i, g in enumerate(classifier.genres)}
            classifier.label2id = {g: i for i, g in enumerate(classifier.genres)}
            classifier.num_labels = len(classifier.genres)
            classifier.model_ja = mock_student_model
            classifier.model_en = None

            results = classifier._predict_with_model(
                mock_student_model, ["test text"], multi_label=False, top_k=5
            )

        assert len(results) == 1
        result = results[0]
        assert "below_threshold" in result
        # 0.3 is not < 0.3, so below_threshold should be False
        assert result["below_threshold"] is False
