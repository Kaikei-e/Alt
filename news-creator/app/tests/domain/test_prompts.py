"""Tests for prompt templates."""

from news_creator.domain.prompts import SUMMARY_PROMPT_TEMPLATE


def test_summary_prompt_template_includes_content_placeholder():
    """Test that summary prompt template has content placeholder."""
    assert "{content}" in SUMMARY_PROMPT_TEMPLATE
    assert "{current_date}" in SUMMARY_PROMPT_TEMPLATE



def test_summary_prompt_template_formats_correctly():
    """Test that summary prompt template formats correctly with content."""
    test_content = "This is a test article about technology."
    formatted = SUMMARY_PROMPT_TEMPLATE.format(content=test_content, current_date="2025-12-25")

    assert test_content in formatted
    assert "{content}" not in formatted
    assert "ARTICLE TO SUMMARIZE" in formatted


def test_summary_prompt_template_contains_japanese_instructions():
    """Test that summary prompt template includes Japanese instructions."""
    assert "Japanese" in SUMMARY_PROMPT_TEMPLATE or "日本語" in SUMMARY_PROMPT_TEMPLATE
    assert "常体" in SUMMARY_PROMPT_TEMPLATE or "である" in SUMMARY_PROMPT_TEMPLATE


def test_summary_prompt_template_contains_requirements():
    """Test that summary prompt template includes key requirements."""
    # Should include requirements about length, style, etc.
    assert "5W1H" in SUMMARY_PROMPT_TEMPLATE
    assert "300-1000" in SUMMARY_PROMPT_TEMPLATE or "1000" in SUMMARY_PROMPT_TEMPLATE
    assert "RULES AND CONSTRAINTS" in SUMMARY_PROMPT_TEMPLATE or "CONSTRAINTS" in SUMMARY_PROMPT_TEMPLATE
