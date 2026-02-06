"""Property-based tests using Hypothesis.

Tests invariants that must hold for ALL inputs, not just hand-picked examples.
Covers InputSanitizer, TagValidator, and domain models.
"""

import unicodedata

from hypothesis import HealthCheck, given, settings
from hypothesis import strategies as st

from tag_extractor.input_sanitizer import InputSanitizer
from tag_extractor.tag_validator import clean_noun_phrase, is_valid_japanese_tag
from tag_generator.domain.models import Article, BatchResult, Tag, TagExtractionResult

# ---------------------------------------------------------------------------
# Strategies
# ---------------------------------------------------------------------------

# Printable text that avoids control characters (which InputSanitizer rejects)
safe_text = st.text(
    alphabet=st.characters(
        whitelist_categories=("L", "N", "P", "S", "Z"),
        blacklist_characters="\x00\x01\x02\x03\x04\x05\x06\x07\x08\x0b\x0c\x0e\x0f"
        "\x10\x11\x12\x13\x14\x15\x16\x17\x18\x19\x1a\x1b\x1c\x1d\x1e\x1f",
    ),
    min_size=1,
    max_size=200,
)

# Japanese-like text
japanese_text = st.text(
    alphabet=st.sampled_from(
        list("あいうえおかきくけこさしすせそたちつてとなにぬねのはひふへほまみむめもやゆよらりるれろわをん")
        + list("アイウエオカキクケコサシスセソタチツテトナニヌネノハヒフヘホマミムメモヤユヨラリルレロワヲン")
        + list("人工知能機械学習自然言語処理技術革新")
    ),
    min_size=2,
    max_size=20,
)


# ---------------------------------------------------------------------------
# InputSanitizer Properties
# ---------------------------------------------------------------------------


class TestInputSanitizerProperties:
    """Property-based tests for InputSanitizer."""

    @given(title=safe_text, content=safe_text)
    @settings(max_examples=100, suppress_health_check=[HealthCheck.too_slow])
    def test_sanitize_never_raises(self, title, content):
        """Sanitize should never raise an unhandled exception."""
        sanitizer = InputSanitizer()
        result = sanitizer.sanitize(title, content)
        # Must always return a result object
        assert result is not None
        assert isinstance(result.is_valid, bool)
        assert isinstance(result.violations, list)

    @given(title=safe_text.filter(lambda t: 1 <= len(t.strip()) <= 1000), content=safe_text)
    @settings(max_examples=50, suppress_health_check=[HealthCheck.too_slow])
    def test_sanitized_output_is_nfc_normalized(self, title, content):
        """Valid sanitized output must be NFC-normalized Unicode."""
        sanitizer = InputSanitizer()
        result = sanitizer.sanitize(title, content)
        if result.is_valid and result.sanitized_input:
            assert result.sanitized_input.title == unicodedata.normalize("NFC", result.sanitized_input.title)
            assert result.sanitized_input.content == unicodedata.normalize("NFC", result.sanitized_input.content)

    @given(title=safe_text, content=safe_text)
    @settings(max_examples=50, suppress_health_check=[HealthCheck.too_slow])
    def test_sanitized_output_has_no_control_chars(self, title, content):
        """Sanitized output must not contain control characters."""
        sanitizer = InputSanitizer()
        result = sanitizer.sanitize(title, content)
        if result.is_valid and result.sanitized_input:
            for char in result.sanitized_input.title + result.sanitized_input.content:
                assert ord(char) >= 32 or char in "\t\n\r"

    @given(
        title=st.text(min_size=1001, max_size=1050, alphabet=st.characters(whitelist_categories=("L",))),
    )
    @settings(max_examples=10)
    def test_title_over_limit_is_invalid(self, title):
        """Titles exceeding max length must be rejected."""
        sanitizer = InputSanitizer()
        result = sanitizer.sanitize(title, "valid content")
        assert not result.is_valid


# ---------------------------------------------------------------------------
# TagValidator Properties
# ---------------------------------------------------------------------------


class TestTagValidatorProperties:
    """Property-based tests for Japanese tag validator."""

    @given(tag=st.text(min_size=0, max_size=1))
    def test_short_tags_always_rejected(self, tag):
        """Tags shorter than 2 characters are always invalid."""
        assert not is_valid_japanese_tag(tag)

    @given(tag=st.text(min_size=16, max_size=50, alphabet=st.characters(whitelist_categories=("L",))))
    def test_long_tags_rejected_at_default_limit(self, tag):
        """Tags longer than 15 characters (default) are invalid."""
        assert not is_valid_japanese_tag(tag)

    @given(num=st.integers(min_value=0, max_value=999999))
    def test_number_only_tags_rejected(self, num):
        """Numeric-only strings are never valid tags."""
        assert not is_valid_japanese_tag(str(num))

    @given(tag=japanese_text.filter(lambda t: 2 <= len(t) <= 15))
    @settings(max_examples=100)
    def test_clean_noun_phrase_never_longer(self, tag):
        """clean_noun_phrase should never make a phrase longer."""
        cleaned = clean_noun_phrase(tag)
        assert len(cleaned) <= len(tag)

    @given(tag=japanese_text.filter(lambda t: 2 <= len(t) <= 15))
    @settings(max_examples=50)
    def test_clean_noun_phrase_converges(self, tag):
        """Repeated cleaning must eventually converge (fixed point)."""
        current = tag
        for _ in range(10):
            cleaned = clean_noun_phrase(current)
            if cleaned == current:
                break
            current = cleaned
        # After convergence, one more pass should be no-op
        assert clean_noun_phrase(current) == current

    @given(
        fragment=st.sampled_from(["https", "www", "com", "org", "html", "gt", "lt", "amp", "nbsp"]),
    )
    def test_url_html_fragments_rejected(self, fragment):
        """URL/HTML fragments are always invalid tags."""
        assert not is_valid_japanese_tag(fragment)


# ---------------------------------------------------------------------------
# Domain Model Properties
# ---------------------------------------------------------------------------


class TestDomainModelProperties:
    """Property-based tests for domain models."""

    @given(
        name=st.text(min_size=1, max_size=50, alphabet=st.characters(whitelist_categories=("L", "N"))),
        confidence=st.floats(min_value=0.0, max_value=1.0, allow_nan=False),
    )
    def test_tag_confidence_preserved(self, name, confidence):
        """Tag always preserves its confidence value."""
        tag = Tag(name=name, confidence=confidence)
        assert tag.name == name
        assert tag.confidence == confidence

    @given(
        tags=st.lists(
            st.tuples(
                st.text(min_size=1, max_size=20, alphabet=st.characters(whitelist_categories=("L",))),
                st.floats(min_value=0.0, max_value=1.0, allow_nan=False),
            ),
            min_size=0,
            max_size=10,
        )
    )
    def test_extraction_result_tag_names_matches_tags(self, tags):
        """tag_names property must always match the tags list."""
        tag_objects = [Tag(name=n, confidence=c) for n, c in tags]
        result = TagExtractionResult(
            article_id="a-1",
            tags=tag_objects,
            language="en",
            inference_ms=1.0,
            overall_confidence=0.5,
        )
        assert result.tag_names == [t.name for t in tag_objects]
        assert result.is_empty == (len(tag_objects) == 0)

    @given(
        total=st.integers(min_value=0, max_value=10000),
        successful=st.integers(min_value=0, max_value=10000),
        failed=st.integers(min_value=0, max_value=10000),
    )
    def test_batch_result_is_success_iff_no_failures(self, total, successful, failed):
        """BatchResult.is_success is True exactly when failed == 0."""
        result = BatchResult(total_processed=total, successful=successful, failed=failed)
        assert result.is_success == (failed == 0)

    @given(
        article_id=st.text(min_size=1, max_size=36, alphabet=st.characters(whitelist_categories=("L", "N"))),
        title=st.text(min_size=1, max_size=100),
        content=st.text(min_size=1, max_size=500),
    )
    def test_article_roundtrip(self, article_id, title, content):
        """Article.from_dict(a.to_dict()) preserves key fields."""
        article = Article(id=article_id, title=title, content=content, created_at="2024-01-01T00:00:00")
        roundtripped = Article.from_dict(article.to_dict())
        assert roundtripped.id == article.id
        assert roundtripped.title == article.title
        assert roundtripped.content == article.content
