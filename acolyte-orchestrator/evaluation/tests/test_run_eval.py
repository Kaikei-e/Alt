"""Tests for the run_eval CLI scaffolding and generator selection."""

from __future__ import annotations

import json
from pathlib import Path
from unittest import mock

import pytest

from evaluation import run_eval
from evaluation.dataset import EvalCase
from evaluation.generators.recorded_fixture import RecordedFixtureGenerator, slugify
from evaluation.run_eval import _aggregate, _build_generator, _dataset_digest, run


def _fake_gen(case: EvalCase) -> tuple[str, dict, dict, dict]:
    return (
        "[S1] about AI chips",
        {"S1": {"source_id": "gold-aichip-en-1", "language": "en"}},
        {"gold-aichip-en-1": {"language": "en"}},
        {"S1": "AI chips are ramping in Q1 2026"},
    )


def _case() -> EvalCase:
    return EvalCase(
        topic="2026 Q1 AI chip market outlook",
        query_lang="ja",
        gold_source_ids=frozenset({"gold-aichip-en-1"}),
        expected_lang_mix={"en": 1.0},
    )


def test_run_scores_each_case_with_precision_and_lang_mix():
    results = run([_case()], _fake_gen)
    assert len(results) == 1
    assert results[0]["citation_precision"] == 1.0
    assert results[0]["lang_mix_ratio"] == {"en": 1.0}


def test_run_leaves_faithfulness_none_when_judge_omitted():
    results = run([_case()], _fake_gen)
    assert results[0]["faithfulness"] is None


def test_run_invokes_judge_when_supplied():
    judge_calls: list[str] = []

    def judge(prompt: str) -> float:
        judge_calls.append(prompt)
        return 0.75

    results = run([_case()], _fake_gen, judge=judge)
    assert results[0]["faithfulness"] == 0.75
    assert judge_calls, "judge should have been called"


def test_aggregate_handles_empty_dataset():
    summary = _aggregate([])
    assert summary == {
        "cases": 0,
        "citation_precision_mean": None,
        "faithfulness_mean": None,
        "lang_en_share_mean": None,
    }


def test_aggregate_computes_means_only_over_non_null():
    summary = _aggregate(
        [
            {"citation_precision": 1.0, "faithfulness": None, "lang_mix_ratio": {"en": 0.5}},
            {"citation_precision": None, "faithfulness": 0.75, "lang_mix_ratio": {"en": 0.25}},
        ]
    )
    assert summary["cases"] == 2
    assert summary["citation_precision_mean"] == 1.0
    assert summary["faithfulness_mean"] == 0.75
    assert summary["lang_en_share_mean"] == pytest.approx(0.375)


def test_dataset_digest_is_deterministic(tmp_path: Path):
    path = tmp_path / "a.jsonl"
    path.write_text("line\n")
    digest = _dataset_digest(path)
    assert digest == _dataset_digest(path)
    assert len(digest) == 64


def test_build_generator_scaffold_raises_on_call():
    args = mock.Mock(generator="scaffold", fixtures="", section_key="analysis")
    gen = _build_generator(args)
    with pytest.raises(RuntimeError, match="no generator wired"):
        gen(_case())


def test_build_generator_fixture_requires_dir():
    args = mock.Mock(generator="fixture", fixtures="", section_key="analysis")
    with pytest.raises(SystemExit):
        _build_generator(args)


def test_build_generator_fixture_returns_recorded_generator(tmp_path: Path):
    args = mock.Mock(generator="fixture", fixtures=str(tmp_path), section_key="analysis")
    gen = _build_generator(args)
    assert isinstance(gen, RecordedFixtureGenerator)


def test_main_writes_json_with_metadata(tmp_path: Path):
    dataset = tmp_path / "ds.jsonl"
    dataset.write_text(
        json.dumps(
            {
                "topic": "2026 Q1 AI chip market outlook",
                "query_lang": "ja",
                "gold_source_ids": ["gold-aichip-en-1"],
                "expected_lang_mix": {"en": 1.0},
            },
            ensure_ascii=False,
        )
        + "\n",
        encoding="utf-8",
    )
    fixtures = tmp_path / "fix"
    fixtures.mkdir()
    (fixtures / f"{slugify('2026 Q1 AI chip market outlook')}.json").write_text(
        json.dumps(
            {
                "body": "[S1] about AI chips",
                "source_map": {"S1": {"source_id": "gold-aichip-en-1", "language": "en"}},
                "articles_by_id": {"gold-aichip-en-1": {"language": "en"}},
                "evidence_by_short_id": {"S1": "AI chips are ramping"},
            },
            ensure_ascii=False,
        ),
        encoding="utf-8",
    )
    out = tmp_path / "out.json"
    rc = run_eval.main(
        [
            "--dataset",
            str(dataset),
            "--generator",
            "fixture",
            "--fixtures",
            str(fixtures),
            "--output",
            str(out),
        ]
    )
    assert rc == 0
    payload = json.loads(out.read_text())
    assert payload["metadata"]["dataset_sha256"]
    assert payload["metadata"]["generator"] == "fixture"
    assert payload["summary"]["cases"] == 1
    assert payload["results"][0]["citation_precision"] == 1.0
