"""Tests for evaluation dataset loader."""

from __future__ import annotations

from pathlib import Path

from evaluation.dataset import EvalCase, load_cases


def test_loads_baseline_dataset() -> None:
    path = Path(__file__).resolve().parents[1] / "datasets" / "baseline.jsonl"
    cases = load_cases(path)
    assert len(cases) >= 3
    first = cases[0]
    assert isinstance(first, EvalCase)
    assert first.topic
    # gold_source_ids may be empty for cases used only with the db-replay
    # generator (lang_mix_ratio only) — precision is None in that case.
    assert isinstance(first.gold_source_ids, frozenset)


def test_skips_blank_and_comment_lines(tmp_path: Path) -> None:
    p = tmp_path / "d.jsonl"
    p.write_text(
        "\n# comment\n"
        '{"topic": "T", "query_lang": "ja", "gold_source_ids": ["x"], "expected_lang_mix": {"ja": 1.0}}\n'
        "\n",
        encoding="utf-8",
    )
    cases = load_cases(p)
    assert len(cases) == 1
    assert cases[0].topic == "T"


def test_expected_lang_mix_default_empty() -> None:
    case = EvalCase.from_dict({"topic": "t", "query_lang": "en", "gold_source_ids": ["a"]})
    assert case.expected_lang_mix == {}
    assert case.gold_source_ids == frozenset({"a"})
