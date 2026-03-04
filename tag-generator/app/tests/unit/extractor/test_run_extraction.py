"""Tests for _run_extraction fallback threshold behaviour.

Verifies that primary extraction yielding fewer than min_primary_tags
triggers fallback augmentation, and that merge/dedup/cap logic works.
"""

from unittest.mock import MagicMock, patch

import pytest

from tag_extractor.config import TagExtractionConfig
from tag_extractor.extract import TagExtractor


@pytest.fixture()
def extractor():
    """Create a TagExtractor with min_primary_tags=3 and mocked models."""
    config = TagExtractionConfig(
        min_primary_tags=3,
        top_keywords=10,
        use_onnx_runtime=False,
    )
    mock_manager = MagicMock()
    mock_manager.get_models.return_value = (MagicMock(), MagicMock(), MagicMock())
    mock_manager.get_stopwords.return_value = (set(), set())
    mock_manager.get_runtime_metadata.return_value = {
        "embedder_backend": "test",
        "embedder_metadata": {},
    }
    ext = TagExtractor(config=config, model_manager=mock_manager)
    ext._models_loaded = True
    ext._stopwords_loaded = True
    ext._ja_stopwords = set()
    ext._en_stopwords = set()
    return ext


class TestRunExtractionFallbackThreshold:
    """Tests for _run_extraction min_primary_tags threshold."""

    def test_no_fallback_when_primary_returns_enough_tags(self, extractor: TagExtractor):
        """Primary returns >= min_primary_tags → no fallback invoked."""
        primary_tags = ["python", "machine-learning", "nlp"]
        primary_conf = {"python": 0.9, "machine-learning": 0.8, "nlp": 0.7}

        with (
            patch.object(extractor, "_extract_keywords_english", return_value=(primary_tags, primary_conf)),
            patch.object(extractor, "_fallback_extraction") as mock_fallback,
        ):
            tags, confidences = extractor._run_extraction("some english text", "en")

        assert tags == primary_tags
        assert confidences == primary_conf
        mock_fallback.assert_not_called()

    def test_fallback_invoked_when_primary_returns_one_tag(self, extractor: TagExtractor):
        """Primary returns 1 tag (< 3) → fallback invoked, results merged with primary first."""
        primary_tags = ["sapporo"]
        primary_conf = {"sapporo": 0.9}
        fallback_tags = ["hokkaido", "japan", "travel", "beer"]

        with (
            patch.object(extractor, "_extract_keywords_english", return_value=(primary_tags, primary_conf)),
            patch.object(extractor, "_fallback_extraction", return_value=fallback_tags),
        ):
            tags, confidences = extractor._run_extraction("text about sapporo", "en")

        assert tags[0] == "sapporo"  # Primary tag comes first
        assert "hokkaido" in tags
        assert "japan" in tags
        assert len(tags) == 5  # 1 primary + 4 fallback
        assert confidences["sapporo"] == 0.9  # Primary confidence preserved

    def test_fallback_invoked_when_primary_returns_zero_tags(self, extractor: TagExtractor):
        """Primary returns 0 tags → fallback invoked (existing behaviour preserved)."""
        fallback_tags = ["technology", "software"]

        with (
            patch.object(extractor, "_extract_keywords_english", return_value=([], {})),
            patch.object(extractor, "_fallback_extraction", return_value=fallback_tags),
        ):
            tags, confidences = extractor._run_extraction("some text", "en")

        assert tags == ["technology", "software"]
        assert len(confidences) == 2

    def test_merged_results_capped_at_top_keywords(self, extractor: TagExtractor):
        """Primary (2) + fallback (8) merged and capped at top_keywords=10."""
        primary_tags = ["ai", "deep-learning"]
        primary_conf = {"ai": 0.95, "deep-learning": 0.85}
        fallback_tags = ["neural", "tensorflow", "pytorch", "gpu", "cuda", "training", "model", "data"]

        with (
            patch.object(extractor, "_extract_keywords_english", return_value=(primary_tags, primary_conf)),
            patch.object(extractor, "_fallback_extraction", return_value=fallback_tags),
        ):
            tags, confidences = extractor._run_extraction("text about AI", "en")

        assert len(tags) <= extractor.config.top_keywords
        assert tags[0] == "ai"
        assert tags[1] == "deep-learning"
        assert len(tags) == 10  # 2 primary + 8 fallback = 10 = top_keywords

    def test_graceful_degradation_when_fallback_fails(self, extractor: TagExtractor):
        """Primary returns 1 tag + fallback raises → return the 1 primary tag."""
        primary_tags = ["sapporo"]
        primary_conf = {"sapporo": 0.9}

        with (
            patch.object(extractor, "_extract_keywords_english", return_value=(primary_tags, primary_conf)),
            patch.object(extractor, "_fallback_extraction", side_effect=Exception("fallback broke")),
        ):
            tags, confidences = extractor._run_extraction("text", "en")

        assert tags == ["sapporo"]
        assert confidences == {"sapporo": 0.9}

    def test_deduplication_between_primary_and_fallback(self, extractor: TagExtractor):
        """Shared tags between primary and fallback → no duplicates, primary confidence kept."""
        primary_tags = ["python", "ml"]
        primary_conf = {"python": 0.9, "ml": 0.8}
        fallback_tags = ["python", "data-science", "ml", "tensorflow"]  # "python" and "ml" overlap

        with (
            patch.object(extractor, "_extract_keywords_english", return_value=(primary_tags, primary_conf)),
            patch.object(extractor, "_fallback_extraction", return_value=fallback_tags),
        ):
            tags, confidences = extractor._run_extraction("text", "en")

        assert tags.count("python") == 1
        assert tags.count("ml") == 1
        assert tags == ["python", "ml", "data-science", "tensorflow"]
        assert confidences["python"] == 0.9  # Primary confidence preserved, not overwritten
