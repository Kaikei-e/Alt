"""Regression and root-cause tests for noise-free tag extraction."""

import ast
import importlib.util
import inspect
import re
import sys
from collections import Counter
from typing import Any, cast
from unittest.mock import MagicMock, patch

import pytest
from fugashi import Tagger as FugashiTagger
from nltk.tokenize import wordpunct_tokenize

import tag_extractor.english_extractor as english_extractor_module
import tag_extractor.japanese_extractor as japanese_extractor_module
from tag_extractor.config import TagExtractionConfig
from tag_extractor.english_extractor import (
    ENGLISH_STOP_WORDS,
    extract_keywords_english,
    tokenize_english,
)
from tag_extractor.extract import TagExtractor
from tag_extractor.input_sanitizer import InputSanitizer, SanitizationConfig
from tag_extractor.japanese_extractor import (
    extract_compound_japanese_words,
    is_topical_japanese_noun,
    make_japanese_analyzer,
)
from tag_extractor.model_manager import ModelManager, reset_model_manager
from tag_extractor.tag_validator import is_valid_english_tag
from tag_generator.exceptions import ModelLoadError


class _MinimalCountVectorizer:
    """CountVectorizer test double for minimal dev environments without scikit-learn."""

    def __init__(
        self,
        lowercase: bool = True,
        token_pattern: str | None = None,
        analyzer: Any | None = None,
        **kwargs: Any,
    ) -> None:
        self.lowercase = lowercase
        self.token_pattern = token_pattern
        self.analyzer = analyzer
        self._kwargs = kwargs
        self._vocabulary: list[str] = []

    def build_analyzer(self) -> Any:
        if callable(self.analyzer):
            return self.analyzer
        pattern = re.compile(self.token_pattern or r"(?u)\b\w\w+\b")
        return lambda doc: pattern.findall(doc.lower() if self.lowercase else doc)

    def fit(self, raw_documents: list[str]) -> _MinimalCountVectorizer:
        analyze = self.build_analyzer()
        vocab: set[str] = set()
        for doc in raw_documents:
            vocab.update(analyze(doc))
        self._vocabulary = sorted(vocab)
        return self

    def get_feature_names_out(self) -> list[str]:
        return list(self._vocabulary)


if isinstance(sys.modules.get("sklearn"), MagicMock):
    sys.modules["sklearn.feature_extraction.text"].CountVectorizer = _MinimalCountVectorizer
elif importlib.util.find_spec("sklearn") is None:
    _mock_sklearn = MagicMock()
    _mock_sklearn.feature_extraction.text.CountVectorizer = _MinimalCountVectorizer
    sys.modules["sklearn"] = _mock_sklearn
    sys.modules["sklearn.feature_extraction"] = _mock_sklearn.feature_extraction
    sys.modules["sklearn.feature_extraction.text"] = _mock_sklearn.feature_extraction.text

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
    """Deterministic KeyBERT test double matching KeyBERT v0.9.0 vectorizer/candidate precedence."""

    def extract_keywords(
        self,
        docs: str | list[str],
        candidates: list[str] | None = None,
        keyphrase_ngram_range: tuple[int, int] = (1, 1),
        stop_words: str | list[str] | None = "english",
        top_n: int = 10,
        vectorizer: Any | None = None,
        **_: Any,
    ) -> list[tuple[str, float]]:
        # Mirrors KeyBERT v0.9.0 (keybert/_model.py): when `vectorizer` is passed,
        # `candidates`, `keyphrase_ngram_range`, and `stop_words` are ignored and
        # the candidate vocabulary comes solely from `vectorizer.fit(docs)`.
        if vectorizer is not None:
            doc_list = [docs] if isinstance(docs, str) else list(docs)
            vocab: set[str] | None = None
            if callable(getattr(vectorizer, "fit", None)):
                fitted = vectorizer.fit(doc_list)
                if callable(getattr(fitted, "get_feature_names_out", None)):
                    vocab = set(fitted.get_feature_names_out())
            counts: Counter[str] = Counter()
            build_analyzer = getattr(vectorizer, "build_analyzer", None)
            if callable(build_analyzer):
                analyze = build_analyzer()
                for doc in doc_list:
                    tokens = analyze(doc)
                    counts.update(tok for tok in tokens if vocab is None or tok in vocab)
            elif callable(getattr(vectorizer, "analyzer", None)):
                for doc in doc_list:
                    text_in = doc.lower() if getattr(vectorizer, "lowercase", True) else doc
                    tokens = vectorizer.analyzer(text_in)
                    counts.update(tok for tok in tokens if vocab is None or tok in vocab)
            ranked = counts.most_common(top_n)
            if not ranked:
                return []
            max_count = ranked[0][1]
            return [(term, round(0.25 + 0.7 * (count / max_count), 3)) for term, count in ranked]

        if candidates is not None:
            scored = [(cand, round(max(0.2, 0.9 - idx * 0.04), 3)) for idx, cand in enumerate(candidates[:top_n])]
            return scored

        doc_text = docs if isinstance(docs, str) else " ".join(docs)
        if stop_words == "english":
            active_stopwords: set[str] = set(ENGLISH_STOP_WORDS)
        elif isinstance(stop_words, list | set | tuple):
            active_stopwords = {w.lower() for w in stop_words}
        else:
            active_stopwords = set()

        raw_tokens = re.findall(r"(?u)\b\w\w+\b", doc_text.lower())
        filtered_tokens = [tok for tok in raw_tokens if tok not in active_stopwords]

        min_n, max_n = keyphrase_ngram_range
        counts = Counter()
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
        sanitizer = InputSanitizer()
        result = sanitizer.sanitize(
            title="Claude &amp; Android&nbsp;Security <script>alert(1)</script>",
            content=(
                "<p>Read&nbsp;more at https://security.example.org/2026/report "
                "and&nbsp;compare Google &amp; Claude defences.</p>"
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

    def test_sanitizer_decodes_entities_once_after_nh3_preserving_angle_bracket_comparisons(self) -> None:
        sanitizer = InputSanitizer()
        result = sanitizer.sanitize(
            title="Bounds Check",
            content="Verify x&lt;y and z&gt;w hold across all policy assertions.",
        )

        assert result.is_valid is True
        assert result.sanitized_input is not None
        assert "x<y and z>w" in result.sanitized_input.content

    def test_sanitizer_pins_entity_encoded_markup_decoding_as_tokenisation_only(self) -> None:
        sanitizer = InputSanitizer()
        result = sanitizer.sanitize(
            title="Escaped Markup Notes",
            content=" Prose mentions &lt;div&gt;Android&lt;/div&gt; and &lt;span&gt;Claude&lt;/span&gt; tokens.",
        )

        assert result.is_valid is True
        assert result.sanitized_input is not None
        assert "<div>Android</div> and <span>Claude</span>" in result.sanitized_input.content

        html_sanitizer = InputSanitizer(SanitizationConfig(allow_html=True))
        html_result = html_sanitizer.sanitize(
            title="Escaped Markup Notes",
            content="<p>Prose mentions &lt;script&gt;alert(1)&lt;/script&gt; safely.</p>",
        )
        assert html_result.is_valid is True
        assert html_result.sanitized_input is not None
        assert "<script>" not in html_result.sanitized_input.content

        doc = (InputSanitizer._sanitize_text.__doc__ or "").lower()
        assert "tokenisation" in doc or "tokenization" in doc

    def test_extractor_respects_explicit_strip_urls_false_without_duplicate_strip(self) -> None:
        extractor = TagExtractor(sanitizer_config=SanitizationConfig(strip_urls=False))
        with patch.object(
            extractor,
            "_extract_keywords_english",
            return_value=(["security", "android", "claude"], {"security": 0.9, "android": 0.8, "claude": 0.7}),
        ) as mock_extract:
            extractor.extract_tags_with_metrics(
                "Security Audit https://audit.example.org/spec",
                "Reference https://docs.example.org/guide for Android and Claude verification.",
            )

        passed_text = mock_extract.call_args[0][0]
        assert "https://audit.example.org/spec" in passed_text
        assert "https://docs.example.org/guide" in passed_text


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

    def test_primary_english_merges_sklearn_stopwords_and_enforces_two_char_caps_or_digit_rule(self) -> None:
        manager = ModelManager()
        fake_keybert = _CandidateGeneratingKeyBERT()
        title = "AI and 5G Architecture Review"
        content = (
            "Something was done across three or five clusters, and several teams at Acme Inc "
            "and Beta Ltd noted something due upon ten checks (eg ie co). "
            "Mr Smith and Dr Patel watched Go benchmarks while AI and 5G systems scaled; "
            "something across three or several nodes used AI and 5G."
        )

        with (
            patch.object(manager, "get_models", return_value=(MagicMock(), fake_keybert, MagicMock())),
            patch("nltk.corpus.stopwords.words", return_value=_NLTK_REALISTIC_ENGLISH_STOPWORDS),
            patch("nltk.word_tokenize", side_effect=wordpunct_tokenize),
            patch("tag_extractor.extract.detect", return_value="en"),
        ):
            extractor = TagExtractor(
                config=TagExtractionConfig(top_keywords=12),
                model_manager=manager,
            )
            outcome = extractor.extract_tags_with_metrics(title, content)

        lowered_tags = {tag.lower() for tag in outcome.tags}
        forbidden = {
            "something",
            "three",
            "five",
            "ten",
            "done",
            "due",
            "several",
            "upon",
            "inc",
            "ltd",
            "co",
            "eg",
            "ie",
            "mr",
            "dr",
            "go",
        }
        leaked = lowered_tags & forbidden
        assert not leaked, f"sklearn stopwords or non-all-caps 2-char words leaked: {leaked} (tags={outcome.tags})"
        assert {"ai", "5g"} <= lowered_tags, f"Expected AI and 5G to be preserved, got {outcome.tags}"

    def test_primary_english_post_filter_blocks_noisy_keybert_phrases_and_tokens(self) -> None:
        manager = ModelManager()
        noisy_keybert = MagicMock()
        noisy_keybert.extract_keywords.return_value = [
            ("would like", 0.91),
            ("he said", 0.89),
            ("https", 0.88),
            ("nbsp", 0.87),
            ("amp", 0.86),
            ("like", 0.85),
            ("said", 0.84),
            ("would", 0.83),
            ("something", 0.825),
            ("several", 0.822),
            ("2026", 0.82),
            ("10", 0.81),
            ("0", 0.80),
            ("android", 0.78),
            ("security", 0.76),
            ("google", 0.74),
            ("claude", 0.72),
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
        leaked_noise = lowered_tags & (NOISE_ENGLISH_TAGS | {"something", "several"})
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

    def test_fallback_tokenizer_rejects_title_case_short_tokens_and_keeps_all_caps_digit_or_symbol(self) -> None:
        with patch("nltk.word_tokenize", return_value=["Mr", "Dr", "St", "Go", "AI", "5G", "C", "#", "C", "++"]):
            tokens = tokenize_english(
                "Mr Dr St Go AI 5G C# C++",
                en_stopwords=set(),
                config=TagExtractionConfig(min_token_length=2),
            )

        assert "mr" not in tokens
        assert "dr" not in tokens
        assert "st" not in tokens
        assert "go" not in tokens
        assert {"ai", "5g", "c#", "c++"} <= set(tokens)

    def test_english_dedup_keeps_distinct_prefix_and_gerund_entities_and_collapses_only_plurals(self) -> None:
        mock_kb = MagicMock()
        mock_kb.extract_keywords.return_value = [
            ("intel", 0.96),
            ("intelligence", 0.955),
            ("space", 0.95),
            ("spacex", 0.945),
            ("graph", 0.94),
            ("graphql", 0.935),
            ("cloud", 0.93),
            ("cloudflare", 0.925),
            ("india", 0.92),
            ("indiana", 0.915),
            ("market", 0.91),
            ("marketing", 0.905),
            ("stream", 0.90),
            ("streaming", 0.895),
            ("bank", 0.89),
            ("banking", 0.885),
            ("host", 0.88),
            ("hosting", 0.875),
            ("fund", 0.87),
            ("funding", 0.865),
            ("cod", 0.86),
            ("coding", 0.855),
            ("car", 0.85),
            ("cares", 0.845),
            ("model", 0.75),
            ("models", 0.74),
            ("patch", 0.73),
            ("patches", 0.72),
            ("box", 0.71),
            ("boxes", 0.70),
        ]
        tags, _ = extract_keywords_english(
            (
                "Intel intelligence Space SpaceX graph GraphQL cloud Cloudflare India Indiana "
                "market marketing stream streaming bank banking host hosting fund funding cod coding car cares"
            ),
            mock_kb,
            TagExtractionConfig(top_keywords=35),
            en_stopwords=set(),
        )

        tag_set = set(tags)
        assert {
            "intel",
            "intelligence",
            "space",
            "spacex",
            "graph",
            "graphql",
            "cloud",
            "cloudflare",
            "india",
            "indiana",
            "market",
            "marketing",
            "stream",
            "streaming",
            "bank",
            "banking",
            "host",
            "hosting",
            "fund",
            "funding",
            "cod",
            "coding",
            "car",
            "cares",
        } <= tag_set
        assert {"model", "patch", "box"} <= tag_set
        assert not ({"models", "patches", "boxes"} & tag_set)

    def test_english_stop_words_matches_exact_318_word_sklearn_set_with_single_source(self) -> None:
        assert isinstance(ENGLISH_STOP_WORDS, frozenset)
        assert len(ENGLISH_STOP_WORDS) == 318
        assert "hereupon" in ENGLISH_STOP_WORDS
        assert not ({"hereof", "hereto", "herewith"} & ENGLISH_STOP_WORDS)

        tree = ast.parse(inspect.getsource(english_extractor_module))
        top_level_tries = [node for node in tree.body if isinstance(node, ast.Try)]
        sklearn_imports = [
            node
            for node in ast.walk(tree)
            if (isinstance(node, ast.ImportFrom) and (node.module or "").startswith("sklearn"))
            or (isinstance(node, ast.Import) and any(alias.name.startswith("sklearn") for alias in node.names))
        ]
        assert not top_level_tries, "english_extractor must not use try/except ImportError stopword fallback"
        assert not sklearn_imports, (
            "english_extractor must use a single vendored stopword source without sklearn import"
        )

    def test_extract_keywords_english_requires_en_stopwords_parameter(self) -> None:
        sig = inspect.signature(extract_keywords_english)
        assert sig.parameters["en_stopwords"].default is inspect.Parameter.empty
        with pytest.raises(TypeError):
            cast(Any, extract_keywords_english)("Sample text", MagicMock(), TagExtractionConfig())

    def test_english_validator_and_stopwords_preserve_legitimate_phrases_and_gerund_topics(self) -> None:
        manager = ModelManager()
        with patch("nltk.corpus.stopwords.words", return_value=_NLTK_REALISTIC_ENGLISH_STOPWORDS):
            _, en_stopwords = manager.get_stopwords()

        assert is_valid_english_tag("net neutrality", stopwords=en_stopwords) is True
        assert is_valid_english_tag("HTTP API", stopwords=en_stopwords) is True
        assert is_valid_english_tag("net", stopwords=en_stopwords) is False
        assert is_valid_english_tag("http", stopwords=en_stopwords) is False
        assert is_valid_english_tag("decision making", stopwords=en_stopwords) is True
        assert is_valid_english_tag("critical thinking", stopwords=en_stopwords) is True
        assert is_valid_english_tag("financial reporting", stopwords=en_stopwords) is True


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
        assert "開発" not in tag_set, f"Generic standalone stopword '開発' must be filtered: {outcome.tags}"
        assert "データベース開発" in tag_set, f"Compound 'データベース開発' missing from {outcome.tags}"
        assert "セキュリティ" in tag_set or any("セキュリティ" in tag for tag in tag_set), (
            f"Topical tag 'セキュリティ' missing from {outcome.tags}"
        )

    def test_japanese_semantic_path_excludes_temporal_numeric_counter_and_stopword_noise(self) -> None:
        manager = ModelManager()
        real_tagger = FugashiTagger()
        fake_keybert = _CandidateGeneratingKeyBERT()

        with (
            patch.object(manager, "get_models", return_value=(MagicMock(), fake_keybert, real_tagger)),
            patch("nltk.corpus.stopwords.words", return_value=_NLTK_REALISTIC_ENGLISH_STOPWORDS),
            patch("tag_extractor.extract.detect", return_value="ja"),
        ):
            # Default use_japanese_semantic=True exercises production KeyBERT + CountVectorizer path
            extractor = TagExtractor(
                config=TagExtractionConfig(top_keywords=10),
                model_manager=manager,
            )
            outcome = extractor.extract_tags_with_metrics(SYNTHETIC_JAPANESE_TITLE, SYNTHETIC_JAPANESE_CONTENT)

        tag_set = set(outcome.tags)
        forbidden_semantic_noise = NOISE_JAPANESE_TAGS | {
            "年",
            "月",
            "日",
            "時",
            "件",
            "同日",
            "翌日",
            "今後",
            "一部",
            "開発",
        }
        leaked = tag_set & forbidden_semantic_noise
        assert not leaked, f"Semantic Japanese path leaked noise/stopwords: {leaked} (tags={outcome.tags})"
        assert {"セキュリティ", "データベース", "Android"} <= tag_set, (
            f"Expected topical tags missing from semantic Japanese output: {outcome.tags}"
        )

    def test_make_japanese_analyzer_excludes_numerals_counters_and_temporal_nouns(self) -> None:
        real_tagger = FugashiTagger()
        analyzer = make_japanese_analyzer(real_tagger)
        tokens = set(
            analyzer(
                "2026年10月10日午前10時、同日午前に10件と0件を確認し、今後も一部でAndroidとセキュリティを検証する。"
            )
        )

        forbidden_vocab = {"0", "10", "2026", "件", "午前", "同日", "今後", "一部", "年", "時", "月", "日"}
        leaked = tokens & forbidden_vocab
        assert not leaked, f"Analyzer leaked numeral/counter/temporal tokens into KeyBERT vocabulary: {leaked}"
        assert {"Android", "セキュリティ", "検証"} <= tokens

    def test_score_japanese_candidates_with_keybert_filters_unvalidated_keybert_output(self) -> None:
        manager = ModelManager()
        real_tagger = FugashiTagger()
        noisy_keybert = MagicMock()
        noisy_keybert.extract_keywords.return_value = [
            ("午前", 0.96),
            ("2026", 0.95),
            ("10", 0.94),
            ("件", 0.93),
            ("開発", 0.92),
            ("https", 0.91),
            ("nbsp", 0.90),
            ("未候補語", 0.89),
            ("セキュリティ", 0.85),
            ("データベース開発", 0.82),
        ]

        with (
            patch.object(manager, "get_models", return_value=(MagicMock(), noisy_keybert, real_tagger)),
            patch("nltk.corpus.stopwords.words", return_value=_NLTK_REALISTIC_ENGLISH_STOPWORDS),
        ):
            extractor = TagExtractor(
                config=TagExtractionConfig(top_keywords=10, use_japanese_semantic=True),
                model_manager=manager,
            )
            extractor.warmup()
            tags, _ = extractor._score_japanese_candidates_with_keybert(
                "2026年午前10時にデータベース開発とセキュリティを検証",
                candidates=["データベース開発", "セキュリティ", "Android", "開発", "2026", "https"],
                freq_counter=Counter(
                    {
                        "データベース開発": 2,
                        "セキュリティ": 2,
                        "Android": 1,
                        "開発": 2,
                        "2026": 2,
                        "https": 1,
                    }
                ),
            )

        assert tags == ["セキュリティ", "データベース開発"]

    def test_generic_kaihatsu_stopword_blocks_standalone_but_keeps_compounds(self) -> None:
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
                "午前10時の開発とデータベース開発",
                "午前10時に開発を開始し、同日午前にデータベース開発とデータベースの品質を確認した。",
            )

        assert "開発" not in outcome.tags, f"Standalone '開発' is a generic stopword, got {outcome.tags}"
        assert "データベース開発" in outcome.tags, f"Compound 'データベース開発' must be kept, got {outcome.tags}"
        assert "データベース" in outcome.tags, f"Standalone 'データベース' must be extractable, got {outcome.tags}"
        assert "午前" not in outcome.tags
        assert not any("午前" in tag for tag in outcome.tags)

    def test_japanese_compounds_keep_internal_numerals_counters_and_temporal_modifiers_without_fragments(
        self,
    ) -> None:
        real_tagger = FugashiTagger()
        config = TagExtractionConfig(top_keywords=20, use_japanese_semantic=False)
        text = (
            "2026年午前10時、同日午前に10件の報告で二酸化炭素の削減と二要素認証、"
            "ドル円相場、3Dプリンター、5G通信および5G通信網、第3世代の製品、"
            "2026年問題、四半期決算、年末商戦、二重価格を検証した。"
            "二酸化炭素と二要素認証、ドル円相場、3Dプリンター、5G通信、5G通信網、"
            "第3世代、2026年問題、四半期決算、年末商戦、二重価格が焦点となった。"
        )

        compounds = set(extract_compound_japanese_words(text, real_tagger, config))

        expected_compounds = {
            "二酸化炭素",
            "二要素認証",
            "ドル円相場",
            "3Dプリンター",
            "5G通信",
            "5G通信網",
            "第3世代",
            "2026年問題",
            "四半期決算",
            "年末商戦",
            "二重価格",
        }
        forbidden_fragments_and_noise = {
            "酸化炭素",
            "要素認証",
            "円相場",
            "Dプリンター",
            "G通信",
            "G通信網",
            "世代の製品",
            "2026年",
            "10件",
            "午前10時",
            "同日午前",
            "午前",
        }

        missing = expected_compounds - compounds
        leaked = forbidden_fragments_and_noise & compounds
        assert not missing, f"Expected intact Japanese compounds missing: {missing} (compounds={sorted(compounds)})"
        assert not leaked, f"Fragmented or noise compounds leaked: {leaked} (compounds={sorted(compounds)})"

    def test_is_topical_japanese_noun_respects_allowed_pos_tags(self) -> None:
        token = MagicMock()
        token.feature = MagicMock()
        token.feature.pos1 = "名詞"
        token.feature.pos2 = "普通名詞"
        token.feature.pos3 = "一般"

        assert is_topical_japanese_noun(token, allowed_pos_tags={"固有名詞"}) is False
        assert is_topical_japanese_noun(token, allowed_pos_tags={"名詞"}) is True
        assert not hasattr(japanese_extractor_module, "NOUN_POS_TAGS")


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
