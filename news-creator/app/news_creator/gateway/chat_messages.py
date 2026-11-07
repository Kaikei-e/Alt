"""Conversion of raw Gemma-4 turn-delimited prompts into OpenAI chat messages."""

from __future__ import annotations

_TURN_START = "<|turn>"
_TURN_END = "<turn|>"
_TRAILING_MODEL_HEADER = "<|turn>model"
_ROLE_MAP: dict[str, str] = {
    "system": "system",
    "user": "user",
    "model": "assistant",
}


def gemma_prompt_to_messages(prompt: str) -> list[dict[str, str]]:
    """Parse a Gemma-4 raw prompt into an OpenAI-compatible messages list."""
    stripped = prompt.strip()
    if not stripped:
        raise ValueError("Prompt must not be empty")

    if _TURN_START not in stripped and _TURN_END not in stripped:
        return [{"role": "user", "content": stripped}]

    if stripped.endswith(_TRAILING_MODEL_HEADER):
        body = stripped[: -len(_TRAILING_MODEL_HEADER)].strip()
    else:
        body = stripped

    if not body:
        raise ValueError("Prompt contains no closed turns")

    messages: list[dict[str, str]] = []
    pos = 0
    length = len(body)

    while pos < length:
        while pos < length and body[pos].isspace():
            pos += 1
        if pos >= length:
            break

        if not body.startswith(_TURN_START, pos):
            raise ValueError("Unexpected text outside <|turn> block")

        role_start = pos + len(_TURN_START)
        end_idx = body.find(_TURN_END, role_start)
        if end_idx == -1:
            raise ValueError("Unclosed <|turn> block")

        newline_idx = body.find("\n", role_start, end_idx)
        if newline_idx == -1:
            raise ValueError("Missing newline after <|turn> role")

        raw_role = body[role_start:newline_idx].strip()
        mapped_role = _ROLE_MAP.get(raw_role)
        if mapped_role is None:
            raise ValueError(f"Unknown turn role: {raw_role!r}")

        turn_content_raw = body[newline_idx + 1 : end_idx]
        if _TURN_START in turn_content_raw or _TURN_END in turn_content_raw:
            raise ValueError("Nested or stray turn marker inside turn body")

        content = turn_content_raw.strip()
        if not content:
            raise ValueError("Turn content must not be empty")

        messages.append({"role": mapped_role, "content": content})
        pos = end_idx + len(_TURN_END)

    if not messages:
        raise ValueError("No messages parsed from prompt")

    if messages[-1]["role"] != "user":
        raise ValueError("Last message in prompt must have role 'user'")

    return messages
