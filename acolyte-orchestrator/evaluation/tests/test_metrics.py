"""Unit tests for evaluation metrics."""

from __future__ import annotations

from evaluation.metrics import (
    citation_precision,
    extract_short_ids,
    faithfulness,
    lang_mix_ratio,
)


class TestExtractShortIds:
    def test_returns_unique_short_ids_in_order(self):
        body = "First [S1]. Second [S2]. Again [S1]. Third [S10]."
        assert extract_short_ids(body) == ["S1", "S2", "S10"]

    def test_returns_empty_for_no_citations(self):
        assert extract_short_ids("no citations") == []


class TestCitationPrecision:
    def test_all_emitted_ids_in_gold_yields_1(self):
        body = "Claim [S1] and claim [S2]."
        source_map = {"S1": {"source_id": "a"}, "S2": {"source_id": "b"}}
        gold = {"a", "b"}
        assert citation_precision(body, source_map, gold) == 1.0

    def test_none_in_gold_yields_0(self):
        body = "Claim [S1]."
        source_map = {"S1": {"source_id": "x"}}
        gold = {"a", "b"}
        assert citation_precision(body, source_map, gold) == 0.0

    def test_partial_overlap(self):
        body = "[S1] and [S2]."
        source_map = {"S1": {"source_id": "a"}, "S2": {"source_id": "z"}}
        gold = {"a", "b"}
        assert citation_precision(body, source_map, gold) == 0.5

    def test_no_citations_yields_none(self):
        body = "No citations."
        source_map: dict = {}
        assert citation_precision(body, source_map, set()) is None


class TestLangMixRatio:
    def test_single_language(self):
        source_map = {"S1": {"source_id": "a"}, "S2": {"source_id": "b"}}
        articles = {"a": {"language": "ja"}, "b": {"language": "ja"}}
        body = "[S1] [S2]"
        result = lang_mix_ratio(body, source_map, articles)
        assert result == {"ja": 1.0}

    def test_mixed_languages(self):
        source_map = {
            "S1": {"source_id": "a"},
            "S2": {"source_id": "b"},
            "S3": {"source_id": "c"},
            "S4": {"source_id": "d"},
        }
        articles = {
            "a": {"language": "ja"},
            "b": {"language": "en"},
            "c": {"language": "ja"},
            "d": {"language": "en"},
        }
        body = "[S1] [S2] [S3] [S4]"
        result = lang_mix_ratio(body, source_map, articles)
        assert result == {"ja": 0.5, "en": 0.5}

    def test_unknown_article_defaults_to_und(self):
        source_map = {"S1": {"source_id": "a"}}
        articles: dict = {}
        body = "[S1]"
        result = lang_mix_ratio(body, source_map, articles)
        assert result == {"und": 1.0}

    def test_no_citations_returns_empty(self):
        assert lang_mix_ratio("no citations", {}, {}) == {}


class TestFaithfulness:
    def test_judge_returns_score(self):
        def fake_judge(prompt: str) -> float:
            return 0.87

        score = faithfulness(
            body="Claim supported [S1].",
            evidence_by_short_id={"S1": "evidence text"},
            judge=fake_judge,
        )
        assert score == 0.87

    def test_skips_when_no_citations(self):
        def fake_judge(prompt: str) -> float:
            raise AssertionError("judge should not be called")

        score = faithfulness(
            body="unsupported body",
            evidence_by_short_id={},
            judge=fake_judge,
        )
        assert score is None
