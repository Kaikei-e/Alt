"""Tests for Gemma-4 raw prompt to OpenAI chat messages conversion."""

import pytest

from news_creator.domain.prompts import (
    CHUNK_SUMMARY_PROMPT_TEMPLATE,
    SUMMARY_PROMPT_TEMPLATE,
    wrap_gemma_prompt,
)


def _convert(prompt: str) -> list[dict[str, str]]:
    from news_creator.gateway.chat_messages import gemma_prompt_to_messages

    return gemma_prompt_to_messages(prompt)


def test_single_user_turn_with_trailing_model_header():
    """Single user turn followed by <|turn>model header converts to one user message."""
    prompt = "<|turn>user\nSummarize this article.<turn|>\n<|turn>model\n"

    messages = _convert(prompt)

    assert messages == [{"role": "user", "content": "Summarize this article."}]


def test_single_user_turn_with_trailing_model_header_without_newline():
    """Trailing <|turn>model without trailing newline or with spaces is dropped."""
    prompt = "<|turn>user\nHello world<turn|>\n<|turn>model   \n  "

    messages = _convert(prompt)

    assert messages == [{"role": "user", "content": "Hello world"}]


def test_system_and_user_turns_from_wrap_gemma_prompt():
    """wrap_gemma_prompt output with system and user turns maps to system + user."""
    prompt = wrap_gemma_prompt(
        user_prompt="Article body here",
        system_prompt="You are a news editor.",
    )

    messages = _convert(prompt)

    assert messages == [
        {"role": "system", "content": "You are a news editor."},
        {"role": "user", "content": "Article body here"},
    ]


def test_multi_turn_maps_model_role_to_assistant():
    """Closed model turns are mapped to role='assistant' while ending on 'user'."""
    prompt = (
        "<|turn>system\nSystem rules<turn|>\n"
        "<|turn>user\nFirst question<turn|>\n"
        "<|turn>model\nFirst answer<turn|>\n"
        "<|turn>user\nFollow-up question<turn|>\n"
        "<|turn>model\n"
    )

    messages = _convert(prompt)

    assert messages == [
        {"role": "system", "content": "System rules"},
        {"role": "user", "content": "First question"},
        {"role": "assistant", "content": "First answer"},
        {"role": "user", "content": "Follow-up question"},
    ]


def test_prompt_without_turn_markers_becomes_single_user_message():
    """A plain prompt with no Gemma turn markers becomes a single user message."""
    messages = _convert("  Plain article text to summarize.  \n")

    assert messages == [{"role": "user", "content": "Plain article text to summarize."}]


def test_renders_real_summary_prompt_template_with_sample_article():
    """Rendered SUMMARY_PROMPT_TEMPLATE converts with no turn tokens left and keeps article."""
    article_text = (
        "Acme Corp announced on 2026-11-07 that 50 enterprise customers in Tokyo "
        "adopted its new platform, increasing quarterly revenue by 1.2 billion yen."
    )
    rendered = SUMMARY_PROMPT_TEMPLATE.format(
        content=article_text,
        current_date="2026-11-07",
    )

    messages = _convert(rendered)

    assert len(messages) == 1
    assert messages[-1]["role"] == "user"
    for msg in messages:
        assert "<|turn>" not in msg["content"]
        assert "<turn|>" not in msg["content"]
    assert article_text in messages[-1]["content"]


def test_renders_real_chunk_summary_prompt_template():
    """Rendered CHUNK_SUMMARY_PROMPT_TEMPLATE converts cleanly for hierarchical map phase."""
    chunk_text = "Tokyo University researchers published 3 new findings on 2026-11-07."
    rendered = CHUNK_SUMMARY_PROMPT_TEMPLATE.format(content=chunk_text)

    messages = _convert(rendered)

    assert len(messages) == 1
    assert messages[-1]["role"] == "user"
    for msg in messages:
        assert "<|turn>" not in msg["content"]
        assert "<turn|>" not in msg["content"]
    assert chunk_text in messages[-1]["content"]


@pytest.mark.parametrize(
    "malformed_prompt",
    [
        "",
        "   \n\t  ",
        "<|turn>model\n",
        "<|turn>developer\nHello<turn|>\n<|turn>model\n",
        "<|turn>user\nUnclosed user turn without end token",
        "<|turn>user\nFirst<turn|>\n<|turn>user\nUnclosed second\n<|turn>model\n",
        "<|turn>user\nHello<turn|>\n<|turn>model\nUnclosed assistant text",
        "Plain text with stray <turn|> marker",
        "<|turn>user\nHello<turn|> stray <turn|>\n<|turn>model\n",
        "Leading text <|turn>user\nHello<turn|>\n<|turn>model\n",
        "<|turn>system\nOnly system message<turn|>\n<|turn>model\n",
        "<|turn>user\nHi<turn|>\n<|turn>model\nAssistant ends<turn|>\n<|turn>model\n",
    ],
)
def test_malformed_prompt_raises_value_error(malformed_prompt):
    """Unknown roles, unclosed turns, stray <turn|>, empty results, or non-user tail raise ValueError."""
    with pytest.raises(ValueError):
        _convert(malformed_prompt)
