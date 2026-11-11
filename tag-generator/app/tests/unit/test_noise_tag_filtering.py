"""Regression and root-cause tests for noise-free tag extraction."""

import re
from collections import Counter
from typing import Any
from unittest.mock import MagicMock, patch

import pytest
from fugashi import Tagger as FugashiTagger
from nltk.tokenize import wordpunct_tokenize

from tag_extractor.config import TagExtractionConfig
from tag_extractor.extract import TagExtractor
from tag_extractor.input_sanitizer import InputSanitizer, SanitizationConfig
from tag_extractor.model_manager import ModelManager, reset_model_manager
from tag_generator.exceptions import ModelLoadError

_BASIC_SKLEARN_LIKE_STOPWORDS = frozenset(
    {
        "a",
        "an",
        "and",
        "are",
        "as",
        "at",
        "be",
        "by",
        "for",
        "from",
        "has",
        "have",
        "he",
        "in",
        "is",
        "it",
        "its",
        "of",
        "on",
        "or",
        "she",
        "that",
        "the",
        "their",
        "they",
        "this",
        "to",
        "was",
        "we",
        "were",
        "will",
        "with",
    }
)

_NLTK_REALISTIC_ENGLISH_STOPWORDS = [
    "the",
    "a",
    "an",
    "is",
    "are",
    "was",
    "were",
    "be",
    "been",
    "being",
    "have",
    "has",
    "had",
    "do",
    "does",
    "did",
    "will",
    "should",
    "can",
    "to",
    "of",
    "in",
    "for",
    "on",
    "at",
    "by",
    "with",
    "from",
    "as",
    "but",
    "or",
    "and",
    "not",
    "this",
    "that",
    "he",
    "she",
    "it",
    "they",
]

SYNTHETIC_ENGLISH_TITLE = "Android &amp; Google Security Architecture&nbsp;Review 2026"

SYNTHETIC_ENGLISH_CONTENT = (
    "<p>In 2026,&nbsp;engineers at Google&nbsp;&amp;&nbsp;Anthropic reviewed 10 sandboxes "
    "and 0 regressions across https://security.example.org/2026/10/android-audit and "
    "https://docs.example.com/claude?id=10&amp;rev=0.</p>"
    "<p>He said teams would like to harden Android kernel isolation, and he said "
    "developers like deterministic policy checks with Claude when reviewing Google "
    "security telemetry. He said they would like to keep 0 unverified builds in 2026 "
    "and would audit 10 release channels like Android security benchmarks.</p>"
)

SYNTHETIC_JAPANESE_TITLE = "2026年 午前10時 Androidとデータベース開発のセキュリティ設計&nbsp;&amp;&nbsp;検証"

SYNTHETIC_JAPANESE_CONTENT = (
    "<p>2026年10月10日午前10時、開発チームはデータベースとセキュリティの設計指針を"
    "https://tech.example.jp/2026/10/db-dev?rev=0&amp;mode=10&nbsp;で公開した。</p>"
    "<p>同日午前、Android向けデータベース開発におけるゼロトラストなセキュリティ検証を実施し、"
    "午後0時までに10件のテストで重大な不備が0件であることを確認した。"
    "翌日午前にもデータベース開発とAndroidのセキュリティ監査を継続する。</p>"
)

NOISE_ENGLISH_TAGS = {
    "like",
    "said",
    "would",
    "could",
    "he said",
    "would like",
    "2026",
    "0",
    "10",
    "https",
    "http",
    "www",
    "nbsp",
    "amp",
    "quot",
    "lt",
    "gt",
}

NOISE_JAPANESE_TAGS = {
    "午前",
    "午後",
    "午前10時",
    "同日午前",
    "翌日午前",
    "午後0時",
    "2026",
    "2026年",
    "10月",
    "10日",
    "10時",
    "0時",
    "10件",
    "0件",
    "0",
    "10",
    "https",
    "http",
    "www",
    "nbsp",
    "amp",
}


class _CandidateGeneratingKeyBERT:
    """Deterministic KeyBERT test double matching CountVectorizer token generation."""

    def extract_keywords(
        self,
        docs: str,
        candidates: list[str] | None = None,
        keyphrase_ngram_range: tuple[int, int] = (1, 1),
        stop_words: str | list[str] | None = "english",
        top_n: int = 10,
        **_: Any,
    ) -> list[tuple[str, float]]:
        if candidates is not None:
            scored = [(cand, round(max(0.2, 0.9 - idx * 0.04), 3)) for idx, cand in enumerate(candidates[:top_n])]
            return scored

        if stop_words == "english":
            active_stopwords: set[str] = set(_BASIC_SKLEARN_LIKE_STOPWORDS)
        elif isinstance(stop_words, list | set | tuple):
            active_stopwords = {w.lower() for w in stop_words}
        else:
            active_stopwords = set()

        raw_tokens = re.findall(r"(?u)\b\w\w+\b", docs.lower())
        filtered_tokens = [tok for tok in raw_tokens if tok not in active_stopwords]

        min_n, max_n = keyphrase_ngram_range
        counts: Counter[str] = Counter()
        for n in range(min_n, max_n + 1):
            for idx in range(len(filtered_tokens) - n + 1):
                gram = " ".join(filtered_tokens[idx : idx + n])
                counts[gram] += 1

        ranked = counts.most_common(top_n)
        if not ranked:
            return []
        max_count = ranked[0][1]
        return [(term, round(0.25 + 0.7 * (count / max_count), 3)) for term, count in ranked]


class TestInputNormalisationBeforeExtraction:
    """Verify HTML entities and URLs are normalised before keyword tokenisation."""

    def test_sanitizer_decodes_html_entities_and_preserves_xss_stripping(self) -> None:
        sanitizer = InputSanitizer(SanitizationConfig(strip_urls=True))
        result = sanitizer.sanitize(
            title="Claude &amp; Android&nbsp;Security &lt;script&gt;alert(1)&lt;/script&gt;",
            content=(
                "<p>Read&nbsp;more at https://security.example.org/2026/report "
                "and&amp;nbsp;compare Google &amp; Claude defences.</p>"
            ),
        )

        assert result.is_valid is True
        assert result.sanitized_input is not None
        combined = f"{result.sanitized_input.title} {result.sanitized_input.content}".lower()
        assert "nbsp" not in combined
        assert "&amp;" not in combined
        assert "https://" not in combined
        assert "security.example.org" not in combined
        assert "alert(1)" not in combined
        assert "<script>" not in combined
        assert "claude" in combined
        assert "android" in combined
        assert "google" in combined


class TestEnglishNoiseFilteringEndToEnd:
    """Verify English primary and fallback paths reject noise classes while keeping topics."""

    def setup_method(self) -> None:
        reset_model_manager()

    def test_primary_english_path_excludes_noise_and_keeps_topical_tags(self) -> None:
        manager = ModelManager()
        fake_keybert = _CandidateGeneratingKeyBERT()

        with (
            patch.object(manager, "get_models", return_value=(MagicMock(), fake_keybert, MagicMock())),
            patch("nltk.corpus.stopwords.words", return_value=_NLTK_REALISTIC_ENGLISH_STOPWORDS),
            patch("nltk.word_tokenize", side_effect=wordpunct_tokenize),
            patch("tag_extractor.extract.detect", return_value="en"),
        ):
            extractor = TagExtractor(
                config=TagExtractionConfig(top_keywords=10),
                model_manager=manager,
            )
            outcome = extractor.extract_tags_with_metrics(SYNTHETIC_ENGLISH_TITLE, SYNTHETIC_ENGLISH_CONTENT)

        lowered_tags = {tag.lower() for tag in outcome.tags}
        leaked_noise = lowered_tags & NOISE_ENGLISH_TAGS
        assert not leaked_noise, f"Noise tags leaked into English primary output: {leaked_noise} (tags={outcome.tags})"
        assert {"android", "security", "google", "claude"} <= lowered_tags, (
            f"Expected topical tags missing from {outcome.tags}"
        )

    def test_primary_english_post_filter_blocks_noisy_keybert_phrases_and_tokens(self) -> None:
        manager = ModelManager()
        noisy_keybert = MagicMock()
        noisy_keybert.extract_keywords.return_value = [
            ("Would Like", 0.91),
            ("He Said", 0.89),
            ("https", 0.88),
            ("nbsp", 0.87),
            ("amp", 0.86),
            ("like", 0.85),
            ("said", 0.84),
            ("would", 0.83),
            ("2026", 0.82),
            ("10", 0.81),
            ("0", 0.80),
            ("Android", 0.78),
            ("security", 0.76),
            ("Google", 0.74),
            ("Claude", 0.72),
        ]

        with (
            patch.object(manager, "get_models", return_value=(MagicMock(), noisy_keybert, MagicMock())),
            patch("nltk.corpus.stopwords.words", return_value=_NLTK_REALISTIC_ENGLISH_STOPWORDS),
            patch("nltk.word_tokenize", side_effect=wordpunct_tokenize),
            patch("tag_extractor.extract.detect", return_value="en"),
        ):
            extractor = TagExtractor(
                config=TagExtractionConfig(top_keywords=10),
                model_manager=manager,
            )
            outcome = extractor.extract_tags_with_metrics(SYNTHETIC_ENGLISH_TITLE, SYNTHETIC_ENGLISH_CONTENT)

        lowered_tags = {tag.lower() for tag in outcome.tags}
        leaked_noise = lowered_tags & NOISE_ENGLISH_TAGS
        assert not leaked_noise, f"Noisy KeyBERT candidates survived filtering: {leaked_noise} (tags={outcome.tags})"
        assert {"android", "security", "google", "claude"} <= lowered_tags

    def test_fallback_english_path_excludes_years_modals_reporting_verbs_and_url_html_fragments(self) -> None:
        manager = ModelManager()
        empty_keybert = MagicMock()
        empty_keybert.extract_keywords.return_value = []

        with (
            patch.object(manager, "get_models", return_value=(MagicMock(), empty_keybert, MagicMock())),
            patch("nltk.corpus.stopwords.words", return_value=_NLTK_REALISTIC_ENGLISH_STOPWORDS),
            patch("nltk.word_tokenize", side_effect=wordpunct_tokenize),
            patch("tag_extractor.extract.detect", return_value="en"),
        ):
            extractor = TagExtractor(
                config=TagExtractionConfig(top_keywords=10, min_token_length=1),
                model_manager=manager,
            )
            outcome = extractor.extract_tags_with_metrics(SYNTHETIC_ENGLISH_TITLE, SYNTHETIC_ENGLISH_CONTENT)

        lowered_tags = {tag.lower() for tag in outcome.tags}
        leaked_noise = lowered_tags & NOISE_ENGLISH_TAGS
        assert not leaked_noise, f"Noise tags leaked into English fallback output: {leaked_noise} (tags={outcome.tags})"
        assert {"android", "security", "google", "claude"} & lowered_tags, (
            f"Expected topical tags in fallback output, got {outcome.tags}"
        )


class TestJapaneseNoiseFilteringEndToEnd:
    """Verify Japanese morphological extraction rejects temporal/numeric/HTML/URL noise."""

    def setup_method(self) -> None:
        reset_model_manager()

    def test_japanese_extraction_excludes_temporal_numeric_url_html_noise_and_keeps_topics(self) -> None:
        manager = ModelManager()
        real_tagger = FugashiTagger()
        fake_keybert = _CandidateGeneratingKeyBERT()

        with (
            patch.object(manager, "get_models", return_value=(MagicMock(), fake_keybert, real_tagger)),
            patch("nltk.corpus.stopwords.words", return_value=_NLTK_REALISTIC_ENGLISH_STOPWORDS),
            patch("tag_extractor.extract.detect", return_value="ja"),
        ):
            extractor = TagExtractor(
                config=TagExtractionConfig(top_keywords=10, use_japanese_semantic=False),
                model_manager=manager,
            )
            outcome = extractor.extract_tags_with_metrics(SYNTHETIC_JAPANESE_TITLE, SYNTHETIC_JAPANESE_CONTENT)

        tag_set = set(outcome.tags)
        lowered_tags = {tag.lower() for tag in outcome.tags}
        leaked_ja = tag_set & NOISE_JAPANESE_TAGS
        leaked_ascii = lowered_tags & {"https", "http", "www", "nbsp", "amp", "2026", "0", "10"}
        assert not leaked_ja, f"Japanese noise tags leaked: {leaked_ja} (tags={outcome.tags})"
        assert not leaked_ascii, f"ASCII/HTML/URL noise leaked into Japanese tags: {leaked_ascii} (tags={outcome.tags})"
        assert not any("午前" in tag or "午後" in tag for tag in outcome.tags), (
            f"Temporal expression leaked into Japanese tags: {outcome.tags}"
        )
        assert "開発" in tag_set or any("開発" in tag for tag in tag_set), (
            f"Topical tag '開発' missing from {outcome.tags}"
        )
        assert "データベース" in tag_set or any("データベース" in tag for tag in tag_set), (
            f"Topical tag 'データベース' missing from {outcome.tags}"
        )
        assert "セキュリティ" in tag_set or any("セキュリティ" in tag for tag in tag_set), (
            f"Topical tag 'セキュリティ' missing from {outcome.tags}"
        )

    def test_standalone_kaihatsu_is_not_blocked_as_stopword(self) -> None:
        manager = ModelManager()
        real_tagger = FugashiTagger()

        with (
            patch.object(manager, "get_models", return_value=(MagicMock(), MagicMock(), real_tagger)),
            patch("nltk.corpus.stopwords.words", return_value=_NLTK_REALISTIC_ENGLISH_STOPWORDS),
            patch("tag_extractor.extract.detect", return_value="ja"),
        ):
            extractor = TagExtractor(
                config=TagExtractionConfig(top_keywords=10, use_japanese_semantic=False),
                model_manager=manager,
            )
            outcome = extractor.extract_tags_with_metrics(
                "午前10時の開発とデータベース",
                "午前10時に開発を開始し、同日午前に開発の品質とデータベースを確認した。",
            )

        assert "開発" in outcome.tags, f"Standalone '開発' must be extractable, got {outcome.tags}"
        assert "データベース" in outcome.tags, f"Standalone 'データベース' must be extractable, got {outcome.tags}"
        assert "午前" not in outcome.tags
        assert not any("午前" in tag for tag in outcome.tags)


class TestStopwordLoadingFailFast:
    """Missing stopword resources must fail fast rather than silently disabling filtering."""

    def setup_method(self) -> None:
        reset_model_manager()

    @patch("pathlib.Path.open", side_effect=FileNotFoundError("stopwords file missing"))
    def test_missing_stopword_file_raises_model_load_error(self, _mock_open: MagicMock) -> None:
        manager = ModelManager()
        with pytest.raises(ModelLoadError):
            manager.get_stopwords()

    @patch("nltk.corpus.stopwords.words", side_effect=LookupError("Resource stopwords not found"))
    def test_missing_nltk_stopwords_raises_model_load_error(self, _mock_nltk: MagicMock) -> None:
        manager = ModelManager()
        with pytest.raises(ModelLoadError):
            manager.get_stopwords()
