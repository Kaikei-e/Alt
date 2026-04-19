"""Tests for language_quota.rebalance_by_language.

The rebalance guarantees soft language representation when the unfiltered
pool contains candidates of the target language. It never removes items a
language that is *not* oversubscribed.
"""

from acolyte.domain.language_quota import rebalance_by_language


def _item(item_id: str, language: str, score: float) -> dict:
    return {"id": item_id, "language": language, "score": score, "title": f"t-{item_id}"}


class TestRebalanceByLanguage:
    def test_promotes_highest_scored_en_when_pool_has_en_and_curated_missing(self):
        curated = [
            _item("a", "ja", 0.9),
            _item("b", "ja", 0.85),
            _item("c", "ja", 0.7),
        ]
        pool = curated + [
            _item("x", "en", 0.8),
            _item("y", "en", 0.5),
        ]
        result = rebalance_by_language(curated, pool, {"en": 0.2})

        ids = [item["id"] for item in result]
        assert "x" in ids
        # Lowest-scored ja (id=c at 0.7) should be swapped out
        assert "c" not in ids
        # Higher-scored ja stays
        assert "a" in ids
        assert "b" in ids

    def test_no_change_when_pool_has_no_target_language(self):
        curated = [_item("a", "ja", 0.9), _item("b", "ja", 0.7)]
        pool = list(curated)
        result = rebalance_by_language(curated, pool, {"en": 0.2})
        assert [item["id"] for item in result] == ["a", "b"]

    def test_no_change_when_quota_already_met(self):
        curated = [
            _item("a", "ja", 0.9),
            _item("b", "en", 0.8),
            _item("c", "ja", 0.7),
        ]
        pool = list(curated) + [_item("x", "en", 0.6)]
        result = rebalance_by_language(curated, pool, {"en": 0.2})
        assert [item["id"] for item in result] == [c["id"] for c in curated]

    def test_respects_curated_length(self):
        curated = [_item("a", "ja", 0.9), _item("b", "ja", 0.8)]
        pool = list(curated) + [_item("x", "en", 0.7), _item("y", "en", 0.6)]
        result = rebalance_by_language(curated, pool, {"en": 0.5})
        assert len(result) == len(curated)
        langs = sorted(item["language"] for item in result)
        assert langs == ["en", "ja"]

    def test_mutable_default_not_shared_across_calls(self):
        result1 = rebalance_by_language([_item("a", "ja", 0.9)], [_item("a", "ja", 0.9)], None)
        result2 = rebalance_by_language([_item("b", "en", 0.9)], [_item("b", "en", 0.9)], None)
        assert result1 is not result2
        assert result1[0]["id"] == "a"
        assert result2[0]["id"] == "b"

    def test_empty_curated_returns_empty(self):
        assert rebalance_by_language([], [_item("x", "en", 0.9)], {"en": 0.2}) == []

    def test_quota_zero_means_no_enforcement(self):
        curated = [_item("a", "ja", 0.9), _item("b", "ja", 0.8)]
        pool = list(curated) + [_item("x", "en", 0.6)]
        result = rebalance_by_language(curated, pool, {"en": 0.0})
        assert [item["id"] for item in result] == ["a", "b"]

    def test_items_without_language_treated_as_und(self):
        curated = [_item("a", "ja", 0.9), {"id": "b", "title": "no-lang", "score": 0.5}]
        pool = list(curated) + [_item("x", "en", 0.8)]
        result = rebalance_by_language(curated, pool, {"en": 0.3})
        ids = [item["id"] for item in result]
        assert "x" in ids
        # Item with missing language treated as und and replaced first
        assert "b" not in ids
