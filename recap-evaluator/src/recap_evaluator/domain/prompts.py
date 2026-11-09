"""Gemma 4 raw prompt formatting helpers."""

import re

_TURN_INJECTION_RE = re.compile(r"<\|turn>|<turn\|>|<\|think\|>", re.IGNORECASE)


def wrap_gemma_prompt(
    user_prompt: str,
    system_prompt: str = "",
) -> str:
    """Wrap system and user prompts in Gemma 4 turn tokens."""
    user = _TURN_INJECTION_RE.sub("", user_prompt).strip()
    sys = _TURN_INJECTION_RE.sub("", system_prompt).strip()
    if sys:
        return f"<|turn>system\n{sys}\n<turn|>\n<|turn>user\n{user}\n<turn|>\n<|turn>model\n"
    return f"<|turn>user\n{user}\n<turn|>\n<|turn>model\n"
