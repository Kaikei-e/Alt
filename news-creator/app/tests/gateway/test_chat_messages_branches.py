"""Branch coverage tests for gemma_prompt_to_messages."""

from __future__ import annotations

import pytest

from news_creator.gateway.chat_messages import gemma_prompt_to_messages


def test_gemma_prompt_to_messages_error_paths() -> None:
    trailing_only = "<|turn>user\nhi<turn|>\n   \n<|turn>model"
    assert gemma_prompt_to_messages(trailing_only) == [
        {"role": "user", "content": "hi"}
    ]

    with pytest.raises(ValueError, match="Missing newline"):
        gemma_prompt_to_messages("<|turn>user<turn|>")

    with pytest.raises(ValueError, match="Nested or stray turn marker"):
        gemma_prompt_to_messages("<|turn>user\nhello <|turn>user<turn|>")

    with pytest.raises(ValueError, match="Turn content must not be empty"):
        gemma_prompt_to_messages("<|turn>user\n   <turn|>")

    with pytest.raises(ValueError, match="Prompt contains no closed turns"):
        gemma_prompt_to_messages("<|turn>model")
