"""Input-side sanitisation for attacker-controlled text pasted into prompts.

Article bodies, their titles and the quotes lifted verbatim out of them are
third-party RSS content — attacker-controlled by design (OWASP LLM01 /
CWE-1427). Anything that reaches an LLM prompt through those fields is
neutralised here, at the insertion point.

Output-side controls cannot cover this: ``domain.citation_format`` checks
the model's attribution *after* generation and has no view of instructions
smuggled into the input evidence. ADR-000797 hardened the judge prompt only
and deferred the report-graph half to that validator; this module closes it.

The neutralisers are deliberately narrow — they rewrite only the exact
structural tokens the report-graph prompts use as delimiters — so benign
article text (Japanese prose, markdown, code with angle brackets) comes back
byte-identical. ``sanitize_evidence_excerpt`` keeps the stricter
strip-every-tag behaviour the judge prompt needs, where the excerpt is a
short quote and losing the tag characters costs nothing.
"""

from __future__ import annotations

import re

# XML-ish delimiters of the report-graph prompts (writer paragraph prompts,
# prior-section context blocks). Untrusted text carrying one of these can
# close its own wrapper early and reach the instruction block that follows it.
#
# Only tags that actually delimit a block at one of the call sites below
# belong here. ``<body>`` / ``<evidence>`` frame the *judge* prompt, which
# uses the stricter ``sanitize_evidence_excerpt`` instead — escaping them
# here would buy nothing and would mangle everyday HTML in a web-dev article.
_STRUCTURAL_TAGS = (
    "topic",
    "section",
    "claim",
    "supporting_quotes",
    "evidence_ids",
    "delta_feedback",
    "prior_analysis",
    "prior_sections",
    "target_length",
)

_STRUCTURAL_TAG_RE = re.compile(
    r"<\s*/?\s*(?:" + "|".join(_STRUCTURAL_TAGS) + r")\s*/?\s*>",
    re.IGNORECASE,
)

# Plain-text scaffold headers of the extractor / writer prompts, as regex
# fragments. Only a line-initial occurrence can be mistaken for the real
# scaffolding, so the match is anchored to the start of a line — prose that
# merely mentions "Article Body:" mid-sentence stays byte-identical.
#
# The two-word ASCII headers take ``\s+`` and the whole pattern is matched
# case-insensitively: a model reads "ARTICLE BODY:" or "Article  Body:" as
# exactly the same record boundary, so an exact-case, single-space match
# would leave the breakout wide open.
_SCAFFOLD_HEADERS = (
    r"Article\s+ID",
    r"Article\s+Title",
    r"Article\s+Body",
    "参考記事",
    "トピック",
    "ルール",
    "計画済み分析ポイント",
    "以下のルールに従ってください",
)

_SCAFFOLD_HEADER_RE = re.compile(
    r"^([ \t]*(?:" + "|".join(_SCAFFOLD_HEADERS) + r")[ \t]*):",
    re.MULTILINE | re.IGNORECASE,
)

# Drop anything that looks like an XML/HTML tag before pasting evidence into
# the judge prompt. This neutralises attacker-controlled tag-injection attempts
# (ASI-06 Evaluation Manipulation) that try to close <evidence> early and
# reopen a fake <body> that flatters the report.
_XML_TAG_RE = re.compile(r"<[^>]+>")

_NEWLINE_RUN_RE = re.compile(r"[\r\n]+")


def _escape_angle_brackets(match: re.Match[str]) -> str:
    return match.group(0).replace("<", "&lt;").replace(">", "&gt;")


def neutralize_evidence_text(text: str) -> str:
    """Make attacker-controlled text safe to interpolate into a prompt block.

    Structural tags are HTML-escaped so they can no longer terminate a
    wrapper, and line-initial scaffold headers get a full-width colon so they
    can no longer be read as a record boundary. The wording itself survives —
    the model still sees every word, it just cannot mistake the text for the
    prompt's own frame.
    """
    if not text:
        return text
    escaped = _STRUCTURAL_TAG_RE.sub(_escape_angle_brackets, text)
    return _SCAFFOLD_HEADER_RE.sub(r"\1：", escaped)


def neutralize_evidence_line(text: str) -> str:
    """Neutralise a value the prompt lays out on a single line (e.g. a title).

    A newline inside a one-line field forges a whole extra record, so line
    breaks collapse to a space on top of the block-level neutralisation.
    """
    if not text:
        return text
    return _NEWLINE_RUN_RE.sub(" ", neutralize_evidence_text(text))


def count_prompt_scaffolding(text: str) -> int:
    """Count the tokens :func:`neutralize_evidence_text` would rewrite.

    ``acolyte.domain`` carries no logger, so call sites that do — and that
    know which article or claim they are handling — use this to warn when a
    feed is probing the prompt. A rewrite nobody can see is indistinguishable
    from a feed that never tried.
    """
    if not text:
        return 0
    return len(_STRUCTURAL_TAG_RE.findall(text)) + len(_SCAFFOLD_HEADER_RE.findall(text))


def sanitize_evidence_excerpt(text: str, *, max_chars: int = 600) -> str:
    """Remove XML/HTML tags and cap length before insertion into the prompt."""
    if not text:
        return ""
    cleaned = _XML_TAG_RE.sub("", text).strip()
    if len(cleaned) > max_chars:
        cleaned = cleaned[:max_chars] + "…"
    return cleaned
