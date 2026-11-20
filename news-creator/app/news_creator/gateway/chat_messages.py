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


def _parse_single_turn(body: str, pos: int) -> tuple[dict[str, str], int]:
    if not body.startswith(_TURN_START, pos):
        msg = "Unexpected text outside <|turn> block"
        raise ValueError(msg)

    role_start = pos + len(_TURN_START)
    end_idx = body.find(_TURN_END, role_start)
    if end_idx == -1:
        msg = "Unclosed <|turn> block"
        raise ValueError(msg)

    newline_idx = body.find("\n", role_start, end_idx)
    if newline_idx == -1:
        msg = "Missing newline after <|turn> role"
        raise ValueError(msg)

    raw_role = body[role_start:newline_idx].strip()
    mapped_role = _ROLE_MAP.get(raw_role)
    if mapped_role is None:
        msg = f"Unknown turn role: {raw_role!r}"
        raise ValueError(msg)

    turn_content_raw = body[newline_idx + 1 : end_idx]
    if _TURN_START in turn_content_raw or _TURN_END in turn_content_raw:
        msg = "Nested or stray turn marker inside turn body"
        raise ValueError(msg)

    content = turn_content_raw.strip()
    if not content:
        msg = "Turn content must not be empty"
        raise ValueError(msg)

    return {"role": mapped_role, "content": content}, end_idx + len(_TURN_END)


def gemma_prompt_to_messages(prompt: str) -> list[dict[str, str]]:
    """Parse a Gemma-4 raw prompt into an OpenAI-compatible messages list."""
    stripped = prompt.strip()
    if not stripped:
        msg = "Prompt must not be empty"
        raise ValueError(msg)

    if stripped.endswith(_TRAILING_MODEL_HEADER):
        body = stripped[: -len(_TRAILING_MODEL_HEADER)].strip()
    else:
        body = stripped

    if not body:
        msg = "Prompt contains no closed turns"
        raise ValueError(msg)

    messages: list[dict[str, str]] = []
    pos = 0
    length = len(body)

    while pos < length:
        while pos < length and body[pos].isspace():
            pos += 1
        if pos >= length:
            break

        message, pos = _parse_single_turn(body, pos)
        messages.append(message)

    if not messages:
        msg = "No messages parsed from prompt"
        raise ValueError(msg)

    if messages[-1]["role"] != "user":
        msg = "Last message in prompt must have role 'user'"
        raise ValueError(msg)

    return messages
