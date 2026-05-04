"""Pin grammar-safe regex contracts for every Pydantic model whose
``model_json_schema()`` is forwarded to Ollama as ``format=`` (JSON
Schema → GBNF translation).

Background — Ollama's runner converts the JSON Schema into a GBNF
grammar before initialising the constrained sampler. Its translator
passes regex bodies (from ``Field(pattern=...)``) through largely
verbatim, but the GBNF parser only recognises a small subset of regex
escapes.  Anything else (``\\d``, ``\\w``, ``\\s``, the redundant
``\\-`` inside a character class, lookaheads, …) trips
``parse: error parsing grammar: unknown escape at …``, which Ollama
surfaces as the misleading
``failed to load model vocabulary required for format`` HTTP 500.

The Morning Letter daemon hit exactly this on the
``MorningLetterSection.key`` pattern after ADR-000886 turned the
projector daemon back on, sending every letter into the
extractive-fallback path.  This file exists so that the next pattern
that smuggles an unsupported escape into a structured-output schema
fails in CI rather than at 04:29 JST in production.
"""

from __future__ import annotations

import re
from typing import Any, Iterable

import pytest
from pydantic import BaseModel

from news_creator.domain.models import (
    IntermediateSummary,
    MorningLetterContent,
    RecapSummary,
)


# Models whose ``model_json_schema()`` is sent to Ollama as
# ``format=<schema>``. Keep this list in sync with every call site of
# ``model_json_schema()`` under ``news_creator/usecase/``.
SCHEMAS_FORWARDED_TO_OLLAMA: list[type[BaseModel]] = [
    MorningLetterContent,
    IntermediateSummary,
    RecapSummary,
]


# Regex escapes the Ollama JSON-Schema → GBNF translator does not
# accept.  This list mirrors maintainer guidance on
# https://github.com/ollama/ollama/issues/12422 and the failure mode
# observed in the ADR-000886 deploy log
# (``parse: error parsing grammar: unknown escape at \\-]+))``).
#
# Notes:
#   * ``\\d`` / ``\\w`` / ``\\s`` and their negations have no GBNF
#     analogue — replace with explicit character classes
#     (``[0-9]`` / ``[A-Za-z0-9_]`` / ``[ \\t\\n\\r\\f\\v]``).
#   * ``\\-`` is rejected because ``-`` is a character-class
#     metacharacter that does not need escaping; place it at the start
#     or end of the class (``[a-z0-9_-]``) instead.
_DISALLOWED_ESCAPES = (r"\d", r"\D", r"\w", r"\W", r"\s", r"\S", r"\-")


def _walk_patterns(node: Any) -> Iterable[tuple[str, str]]:
    """Yield every ``(json_pointer, pattern)`` pair found inside a JSON Schema."""

    if isinstance(node, dict):
        if "pattern" in node and isinstance(node["pattern"], str):
            yield ("(root)", node["pattern"])
        for key, value in node.items():
            for pointer, pattern in _walk_patterns(value):
                yield (f"{key}/{pointer}", pattern)
    elif isinstance(node, list):
        for index, item in enumerate(node):
            for pointer, pattern in _walk_patterns(item):
                yield (f"[{index}]/{pointer}", pattern)


@pytest.mark.parametrize(
    "model_cls",
    SCHEMAS_FORWARDED_TO_OLLAMA,
    ids=lambda cls: cls.__name__,
)
def test_pattern_avoids_unsupported_gbnf_escapes(model_cls: type[BaseModel]) -> None:
    """Every ``Field(pattern=...)`` reachable from the schema must be
    safe for Ollama's GBNF translator."""

    schema = model_cls.model_json_schema()
    offenders: list[str] = []
    for pointer, pattern in _walk_patterns(schema):
        for escape in _DISALLOWED_ESCAPES:
            # Detect the literal escape sequence as it would arrive in
            # GBNF (single backslash + char). ``re.escape`` lets us
            # build the search pattern without re-applying Python
            # string-escape rules on top.
            if re.search(re.escape(escape), pattern):
                offenders.append(
                    f"  - {model_cls.__name__} :: {pointer}: pattern={pattern!r} contains unsupported escape {escape!r}"
                )

    assert not offenders, (
        "Pydantic Field(pattern=...) reachable from a schema forwarded "
        "to Ollama as format=json_schema must avoid escape sequences "
        "the GBNF parser cannot decode. See "
        "https://github.com/ollama/ollama/issues/12422 for the same "
        "class of failure on \\d.\n" + "\n".join(offenders)
    )


def test_morning_letter_section_key_pattern_uses_dash_not_escape() -> None:
    """Pin the ADR-000886 root-cause fix: ``MorningLetterSection.key`` must
    not write ``\\-`` inside the character class (``-`` at class end is
    enough). This guards against accidental reintroduction of the exact
    pattern that took the Morning Letter offline for three weeks."""

    schema = MorningLetterContent.model_json_schema()
    section_schema = schema["$defs"]["MorningLetterSection"]
    pattern = section_schema["properties"]["key"]["pattern"]

    assert "\\-" not in pattern, (
        f"MorningLetterSection.key pattern reintroduced \\- escape: {pattern!r}. "
        "GBNF rejects this — place the literal '-' at the start or end "
        "of the character class instead."
    )
    # Sanity: the dash is still literally present (we did not lose
    # the by_genre suffix character class entirely).
    assert "by_genre:[" in pattern and "-" in pattern, (
        f"MorningLetterSection.key pattern lost its by_genre suffix: {pattern!r}"
    )
