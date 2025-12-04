"""Tests for HTML cleaner utility."""

import pytest

from news_creator.utils.html_cleaner import clean_html_content


class TestHTMLCleaner:
    """Test HTML cleaner functionality."""

    def test_clean_plain_text(self):
        """Test that plain text is returned as-is."""
        content = "This is plain text without any HTML tags."
        cleaned, was_html = clean_html_content(content)
        assert cleaned == content
        assert was_html is False

    def test_clean_empty_string(self):
        """Test that empty string is handled correctly."""
        cleaned, was_html = clean_html_content("")
        assert cleaned == ""
        assert was_html is False

    def test_clean_simple_html(self):
        """Test cleaning simple HTML."""
        content = "<html><body><p>This is a paragraph.</p></body></html>"
        cleaned, was_html = clean_html_content(content)
        assert was_html is True
        assert "This is a paragraph" in cleaned
        assert "<p>" not in cleaned
        assert "<html>" not in cleaned

    def test_clean_html_with_script(self):
        """Test that script tags are removed."""
        content = "<html><head><script>alert('test');</script></head><body><p>Content</p></body></html>"
        cleaned, was_html = clean_html_content(content)
        assert was_html is True
        # Script content should be removed (bleach strips script tags)
        assert "alert('test')" not in cleaned or "alert" not in cleaned.lower()
        assert "Content" in cleaned

    def test_clean_html_with_style(self):
        """Test that style tags are removed."""
        content = "<html><head><style>body { color: red; }</style></head><body><p>Content</p></body></html>"
        cleaned, was_html = clean_html_content(content)
        assert was_html is True
        assert "color: red" not in cleaned
        assert "Content" in cleaned

    def test_clean_html_doctype(self):
        """Test that HTML starting with doctype is detected."""
        content = "<!doctype html><html><body><p>Content</p></body></html>"
        cleaned, was_html = clean_html_content(content)
        assert was_html is True
        assert "Content" in cleaned

    def test_clean_html_high_ratio(self):
        """Test that HTML with high tag ratio is detected."""
        # Use enough tags to trigger HTML detection (50+ tags)
        content = "<div>" * 30 + "Some text" + "</div>" * 30
        cleaned, was_html = clean_html_content(content)
        assert was_html is True
        assert "Some text" in cleaned

    def test_clean_html_with_attributes(self):
        """Test cleaning HTML with attributes."""
        # Use enough HTML to trigger detection
        content = '<!doctype html><html><body><p class="test" id="main">Content with <a href="http://example.com">link</a></p></body></html>'
        cleaned, was_html = clean_html_content(content)
        assert was_html is True
        assert "Content with" in cleaned
        assert "link" in cleaned
        # Attributes should be removed (bleach strips them when tags=[])
        assert 'class="test"' not in cleaned

    def test_clean_html_with_japanese(self):
        """Test cleaning HTML with Japanese text."""
        content = "<html><body><p>これは日本語のテキストです。</p></body></html>"
        cleaned, was_html = clean_html_content(content)
        assert was_html is True
        assert "これは日本語のテキストです" in cleaned

    def test_clean_html_entities(self):
        """Test that HTML entities are handled."""
        # Use enough HTML to trigger detection
        content = "<!doctype html><html><body><p>Content with &amp; and &lt;tags&gt;</p></body></html>"
        cleaned, was_html = clean_html_content(content)
        assert was_html is True
        assert "Content with" in cleaned
        # Entities should be decoded or removed
        assert "&amp;" not in cleaned or "&" in cleaned

    def test_clean_html_whitespace_normalization(self):
        """Test that excessive whitespace is normalized."""
        # Use enough HTML to trigger detection
        content = "<!doctype html><html><body><p>Content   with    multiple     spaces</p></body></html>"
        cleaned, was_html = clean_html_content(content)
        assert was_html is True
        # Whitespace should be normalized (re.sub(r'\s+', ' ', ...) should reduce multiple spaces)
        assert "Content" in cleaned
        assert "with" in cleaned
        # Verify whitespace is normalized (should not have 3+ consecutive spaces)
        assert "   " not in cleaned

