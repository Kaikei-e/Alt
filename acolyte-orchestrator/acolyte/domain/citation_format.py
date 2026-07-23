"""Citation format validation.

Report bodies must cite sources only with `[Sn]` markers resolved via
`SourceMap`. Any other bracketed expression (full article titles, pipe-
separated source strings) or bare URL pollutes the prose and is rejected
so the revision loop can regenerate the paragraph.

`validate_citation_format` only checks the *syntax* of bracketed markers.
`validate_citation_grounding` checks that every `[Sn]` marker actually
present in the body *resolves* to a source the writer was given as
evidence — catching hallucinated citations where the LLM invents an `[Sn]`
that was never backed by real evidence_ids.
"""

from __future__ import annotations

import re
from collections.abc import Iterable

_BAD_BRACKET_RE = re.compile(r"\[(?!S\d+\])[^\[\]]+\]")
_BARE_URL_RE = re.compile(r"https?://\S+")
_SN_MARKER_RE = re.compile(r"\[(S\d+)\]")


def validate_citation_format(body: str) -> tuple[bool, str]:
    """Return ``(True, "")`` when body uses only ``[Sn]`` citations.

    Rules:
    - Any bracket other than ``[Sn]`` is rejected. This catches
      ``[Title | Source | Tags]`` pollution as well as legacy ``[1]`` / ``[42]``
      Perplexity-style markers that bypass ``SourceMap``.
    - Bare ``http(s)://`` URLs are rejected; URL metadata belongs in the
      Sources footer rendered by the finalizer.

    The reason string is short and safe to inline into a revision feedback.
    """
    match = _BAD_BRACKET_RE.search(body)
    if match is not None:
        snippet = match.group(0)[:80]
        return False, f"inline_title_in_brackets: {snippet}"

    url_match = _BARE_URL_RE.search(body)
    if url_match is not None:
        snippet = url_match.group(0)[:80]
        return False, f"bare_url: {snippet}"

    return True, ""


def validate_citation_grounding(body: str, valid_source_ids: Iterable[str]) -> tuple[bool, str]:
    """Return ``(True, "")`` when every ``[Sn]`` marker in body resolves to a real source.

    ``valid_source_ids`` is the set of short IDs (e.g. ``{"S1", "S2"}``) that
    were actually supplied to the writer as evidence for the claim being
    rendered. Any ``[Sn]`` marker in the body outside that set is a
    hallucinated citation — the report asserting a source that does not
    back it — and is rejected so the revision loop can regenerate the
    paragraph, mirroring ``validate_citation_format``.

    A body with no ``[Sn]`` markers always passes, regardless of
    ``valid_source_ids`` — this function only guards markers that exist.
    """
    valid = set(valid_source_ids)
    unknown = list(dict.fromkeys(m for m in _SN_MARKER_RE.findall(body) if m not in valid))
    if unknown:
        return False, f"unknown_citation_id: {', '.join(unknown)}"
    return True, ""
