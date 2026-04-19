"""Citation format validation.

Report bodies must cite sources only with `[Sn]` markers resolved via
`SourceMap`. Any other bracketed expression (full article titles, pipe-
separated source strings) or bare URL pollutes the prose and is rejected
so the revision loop can regenerate the paragraph.
"""

from __future__ import annotations

import re

_BAD_BRACKET_RE = re.compile(r"\[(?!S\d+\])[^\[\]]+\]")
_BARE_URL_RE = re.compile(r"https?://\S+")


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
