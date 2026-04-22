"""RED test for evaluation NaN/empty-text guard.

Context: `/v1/evaluation/genres` previously returned HTTP 400
`np.nan is an invalid document` when golden data contained rows with
missing or empty text. sklearn's TfidfVectorizer.transform() rejects
NaN/None inputs. The evaluation pipeline must strip these rows before
calling the classifier, not crash the whole run.

Review reference: docs/review/3days-recap-rustbert-cache-recovery-2026-04-22.md §残課題 (5).
"""

from __future__ import annotations

import numpy as np
import pandas as pd

from recap_subworker.services.evaluation import EvaluationService


def test_prepare_evaluation_inputs_drops_nan_text() -> None:
    df = pd.DataFrame(
        {
            "text": ["valid text 1", np.nan, "valid text 2", None, "   ", "valid text 3"],
            "labels": [
                ["ai_data"],
                ["ai_data"],
                ["consumer_tech"],
                ["sports"],
                ["health_medicine"],
                ["politics_government"],
            ],
        }
    )

    texts, labels = EvaluationService._prepare_evaluation_inputs(df)

    assert texts == ["valid text 1", "valid text 2", "valid text 3"]
    assert labels == [["ai_data"], ["consumer_tech"], ["politics_government"]]


def test_prepare_evaluation_inputs_allows_fully_populated_frame() -> None:
    df = pd.DataFrame(
        {
            "text": ["a", "b"],
            "labels": [["ai_data"], ["consumer_tech"]],
        }
    )

    texts, labels = EvaluationService._prepare_evaluation_inputs(df)

    assert texts == ["a", "b"]
    assert labels == [["ai_data"], ["consumer_tech"]]


def test_prepare_evaluation_inputs_returns_empty_for_all_nan() -> None:
    df = pd.DataFrame(
        {
            "text": [np.nan, None, ""],
            "labels": [["ai_data"], ["consumer_tech"], ["sports"]],
        }
    )

    texts, labels = EvaluationService._prepare_evaluation_inputs(df)

    assert texts == []
    assert labels == []
