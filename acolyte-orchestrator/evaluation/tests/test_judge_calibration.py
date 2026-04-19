"""Calibration dataset structural test.

The judge-calibration dataset holds hand-labeled (body, evidence,
expected_score) tuples used to quantify judge agreement against a human
rubric. The *actual* judge vs. human agreement measurement is a manual
offline run against Gemma4 — running it in CI would depend on news-creator
and be flaky.

This test only validates the dataset **shape** and **rubric compliance**
so a malformed update cannot sneak through. For the scoring evaluation
itself, use ``evaluation.judges.calibrate`` (future work) or invoke the
judge directly and compare against ``expected_score``.
"""

from __future__ import annotations

import json
from pathlib import Path

import pytest

_ALLOWED = {0.0, 0.25, 0.5, 0.75, 1.0}


@pytest.fixture
def calibration_path() -> Path:
    return Path(__file__).resolve().parents[1] / "datasets" / "judge_calibration.jsonl"


def test_calibration_dataset_exists(calibration_path: Path) -> None:
    assert calibration_path.exists(), "evaluation/datasets/judge_calibration.jsonl is missing"


def test_calibration_dataset_has_enough_entries(calibration_path: Path) -> None:
    lines = [
        line
        for line in calibration_path.read_text(encoding="utf-8").splitlines()
        if line.strip() and not line.startswith("#")
    ]
    # ADR-000778 plan specifies at least 10 calibration cases.
    assert len(lines) >= 10, f"expected ≥ 10 entries, found {len(lines)}"


def test_calibration_dataset_entries_are_well_formed(calibration_path: Path) -> None:
    for raw in calibration_path.read_text(encoding="utf-8").splitlines():
        if not raw.strip() or raw.startswith("#"):
            continue
        entry = json.loads(raw)
        assert "body" in entry and isinstance(entry["body"], str)
        assert "evidence" in entry and isinstance(entry["evidence"], dict)
        assert "expected_score" in entry
        assert entry["expected_score"] in _ALLOWED, f"expected_score {entry['expected_score']!r} is not a rubric bin"
        # Each evidence mapping must have at least one [Sn]-keyed quote.
        assert entry["evidence"], "evidence must be non-empty"
        for short_id, quote in entry["evidence"].items():
            assert short_id.startswith("S"), f"bad short_id: {short_id!r}"
            assert isinstance(quote, str) and quote.strip()


def test_calibration_dataset_covers_each_rubric_bin(calibration_path: Path) -> None:
    seen: set[float] = set()
    for raw in calibration_path.read_text(encoding="utf-8").splitlines():
        if not raw.strip() or raw.startswith("#"):
            continue
        entry = json.loads(raw)
        seen.add(float(entry["expected_score"]))
    # The calibration set must exercise the full rubric range.
    assert seen == _ALLOWED, f"missing bins: {_ALLOWED - seen}"


def test_mock_judge_disagrees_with_rubric(calibration_path: Path) -> None:
    """Guard against anyone pointing CI at the mock judge and calling the
    run "calibrated". MockRubricJudge returns a constant, so its agreement
    with the calibration rubric should be well below perfect — if that
    stops being true, someone has replaced the mock with something it
    shouldn't be.
    """
    from evaluation.judges.mock import MockRubricJudge

    judge = MockRubricJudge(mock_score=0.5)
    agreements = 0
    total = 0
    for raw in calibration_path.read_text(encoding="utf-8").splitlines():
        if not raw.strip() or raw.startswith("#"):
            continue
        total += 1
        entry = json.loads(raw)
        if judge("ignored") == float(entry["expected_score"]):
            agreements += 1
    # With 5 evenly-spaced bins and a fixed 0.5 mock, agreement is 1/5 at
    # uniform distribution — our dataset has more bins than 0.5 so the
    # mock cannot reach the calibration threshold of 7/10.
    assert total >= 10
    assert agreements < 7, "mock judge hit 70% agreement with rubric; this should be impossible"
