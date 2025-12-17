"""Token counter using character-based estimation for rough token counting."""

import logging

logger = logging.getLogger(__name__)


def count_tokens(text: str) -> int:
    """
    Count tokens in text using character-based estimation.

    This is a rough estimation suitable for model routing.
    The calculation uses a conservative estimate: 1 character ≈ 0.75 tokens
    (accounting for Japanese characters which are typically 1 token per character,
    and English/mixed text which is typically less).

    Args:
        text: Input text to count tokens for

    Returns:
        Estimated number of tokens (rough calculation)
    """
    if not text:
        return 1

    # Rough estimation: 1 character ≈ 0.75 tokens
    # This accounts for:
    # - Japanese: 1 character ≈ 1 token (but spaces/punctuation reduce average)
    # - English: 1 character ≈ 0.25-0.5 tokens
    # - Mixed: average ≈ 0.75 tokens per character
    # Using integer division for performance: (len(text) * 3) // 4
    estimated_tokens = max(1, (len(text) * 3) // 4)

    return estimated_tokens


def count_tokens_with_template(prompt_template: str, **kwargs) -> int:
    """
    Count tokens in a formatted prompt template.

    Args:
        prompt_template: Prompt template string (may contain {placeholders})
        **kwargs: Template variables

    Returns:
        Number of tokens after template formatting
    """
    try:
        # Format template if it has placeholders
        if kwargs:
            formatted_prompt = prompt_template.format(**kwargs)
        else:
            formatted_prompt = prompt_template
        return count_tokens(formatted_prompt)
    except Exception as e:
        logger.warning(
            f"Template formatting failed: {e}. Counting tokens in raw template.",
            exc_info=True,
        )
        return count_tokens(prompt_template)

