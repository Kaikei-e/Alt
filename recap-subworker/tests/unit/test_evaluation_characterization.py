"""Characterization tests for EvaluationService.evaluate branches."""

from __future__ import annotations

import json
from pathlib import Path
from unittest.mock import MagicMock, patch

import pytest

from recap_subworker.services.evaluation import EvaluationService

EXPECTED_TWO_SAMPLES = 2
EXPECTED_THREE_SAMPLES = 3
EXPECTED_ACCURACY_PERFECT = 1.0
EXPECTED_ZERO_WIDTH = 0.0
EXPECTED_CV_FOLDS = 2


class _FakeClassifier:
    def __init__(self, predicted_genres_per_call: list[list[str]]) -> None:
        self._predicted_genres = predicted_genres_per_call

    def predict_batch(
        self,
        texts: list[str],
        multi_label: bool = True,
    ) -> list[dict[str, object]]:
        results: list[dict[str, object]] = []
        for idx, _text in enumerate(texts):
            genres = self._predicted_genres[idx % len(self._predicted_genres)]
            results.append(
                {
                    "top_genre": genres[0] if genres else "other",
                    "candidates": [{"genre": g, "score": 0.9} for g in genres],
                }
            )
        return results


def _make_service() -> EvaluationService:
    embedder = MagicMock()
    embedder.config.batch_size = 8
    return EvaluationService(embedder=embedder)


def test_evaluate_missing_path_raises_file_not_found(tmp_path: Path) -> None:
    service = _make_service()
    missing = tmp_path / "does_not_exist.json"
    with pytest.raises(FileNotFoundError, match="Golden data not found"):
        service.evaluate(missing)


def test_evaluate_dict_wrapper_ja_filter_and_bootstrap_ci(tmp_path: Path) -> None:
    golden_path = tmp_path / "golden_ja.json"
    payload = {
        "items": [
            {"content_ja": "日本語テキスト1", "expected_genres": ["ai_data"]},
            {"content_ja": "   ", "expected_genres": ["sports"]},
            {"content_en": "english only", "expected_genres": ["tech"]},
            {"content_ja": "日本語テキスト2", "expected_genres": ["politics"]},
        ]
    }
    golden_path.write_text(json.dumps(payload), encoding="utf-8")

    service = _make_service()
    service.classifier_ja = _FakeClassifier([["ai_data"], ["politics"]])  # type: ignore[assignment]

    results = service.evaluate(golden_path, language="ja", use_bootstrap=True)

    assert results["total_samples"] == EXPECTED_TWO_SAMPLES
    assert results["accuracy"] == EXPECTED_ACCURACY_PERFECT
    assert results["language"] == "ja"
    assert results["accuracy_ci"]["width"] >= EXPECTED_ZERO_WIDTH
    assert "macro_metrics" in results
    assert "ai_data" in results["per_genre_metrics"]
    assert results["confusion_matrix"]["labels"] == ["ai_data", "politics"]


def test_evaluate_list_root_en_filter_genres_column_and_no_bootstrap(tmp_path: Path) -> None:
    golden_path = tmp_path / "golden_en.json"
    payload = [
        {"content_en": "English article one", "genres": ["tech"]},
        {"content_ja": "日本語のみ", "genres": ["sports"]},
        {"content_en": "English article two", "genres": ["science"]},
    ]
    golden_path.write_text(json.dumps(payload), encoding="utf-8")

    service = _make_service()
    service.classifier_en = _FakeClassifier([["tech"], ["science"]])  # type: ignore[assignment]

    results = service.evaluate(golden_path, language="en", use_bootstrap=False)

    assert results["total_samples"] == EXPECTED_TWO_SAMPLES
    assert results["language"] == "en"
    assert results["accuracy_ci"]["width"] == EXPECTED_ZERO_WIDTH
    assert "macro_metrics" not in results


def test_evaluate_content_column_fallback_drops_empty_rows_and_runs_cv(tmp_path: Path) -> None:
    golden_path = tmp_path / "golden_content.json"
    payload = [
        {"content": "Valid article 1", "labels": ["ai_data"]},
        {"content": "   ", "labels": ["ai_data"]},
        {"content": "Valid article 2", "labels": ["politics"]},
    ]
    golden_path.write_text(json.dumps(payload), encoding="utf-8")

    service = _make_service()
    service.classifier_default = _FakeClassifier([["ai_data"], ["politics"]])  # type: ignore[assignment]

    cv_stub = {"cv_accuracy": {"mean": 0.75, "std": 0.05}, "n_folds": EXPECTED_CV_FOLDS}
    with patch.object(service, "_run_cross_validation", return_value=cv_stub) as mock_cv:
        results = service.evaluate(
            golden_path,
            language=None,
            use_bootstrap=False,
            use_cross_validation=True,
            n_folds=EXPECTED_CV_FOLDS,
        )

    assert results["total_samples"] == EXPECTED_TWO_SAMPLES
    assert "language" not in results
    assert results["n_folds"] == EXPECTED_CV_FOLDS
    mock_cv.assert_called_once()


def test_evaluate_swallows_cross_validation_error(tmp_path: Path) -> None:
    golden_path = tmp_path / "golden_cv_err.json"
    payload = [
        {"text": "Article one", "labels": ["ai_data"]},
        {"text": "Article two", "labels": ["ai_data"]},
        {"text": "Article three", "labels": ["politics"]},
    ]
    golden_path.write_text(json.dumps(payload), encoding="utf-8")

    service = _make_service()
    service.classifier_default = _FakeClassifier([["ai_data"], ["ai_data"], ["politics"]])  # type: ignore[assignment]

    with patch.object(service, "_run_cross_validation", side_effect=RuntimeError("cv boom")):
        results = service.evaluate(
            golden_path,
            use_bootstrap=False,
            use_cross_validation=True,
            n_folds=EXPECTED_CV_FOLDS,
        )

    assert results["total_samples"] == EXPECTED_THREE_SAMPLES
    assert "cv_accuracy" not in results


def test_evaluate_missing_required_columns_raises_value_error(tmp_path: Path) -> None:
    golden_path = tmp_path / "golden_bad_cols.json"
    golden_path.write_text(json.dumps([{"title": "only title"}]), encoding="utf-8")

    service = _make_service()
    with pytest.raises(ValueError, match="Golden data must contain 'text' and 'labels' fields"):
        service.evaluate(golden_path)


def test_evaluate_missing_classifier_raises_value_error(tmp_path: Path) -> None:
    golden_path = tmp_path / "golden_no_clf.json"
    payload = [{"content_ja": "日本語記事", "labels": ["ai_data"]}]
    golden_path.write_text(json.dumps(payload), encoding="utf-8")

    service = _make_service()
    service.classifier_ja = None
    with pytest.raises(ValueError, match="No classifier available for language: ja"):
        service.evaluate(golden_path, language="ja")
