"""Output guard for generated markdown and LLM responses.

Protects end-users from:
1. Phishing / XSS links injected into generated markdown (restricting schemes
   to the ones the frontend renderer accepts).
2. Leaked or forged boundary delimiter tags in LLM outputs.
"""

from __future__ import annotations

import re
from dataclasses import dataclass

from news_creator.domain.prompt_boundary import DEFAULT_DELIMITER_TAG


class OutputGuardError(ValueError):
    """Raised when generated output contains forbidden delimiter tags or dangerous constructs."""


@dataclass(frozen=True)
class OutputGuardResult:
    """Result of output guard validation and sanitization.

    Attributes:
        text: Sanitized markdown text
        contains_delimiters: Whether delimiter tags were detected in the output
        dropped_links_count: Number of dangerous or non-renderable links dropped
        is_valid: True if output passes all security constraints
    """

    text: str
    contains_delimiters: bool
    dropped_links_count: int
    is_valid: bool


# Markdown links and images. The URL tolerates one level of nested parentheses
# ("…/Foo_(disambiguation)") so a dangerous link is never left half-matched,
# with its trailing ")" stranded in the rendered text.
_MD_LINK_RE = re.compile(
    r"(?P<bang>!?)\[(?P<label>[^\]]*)\]\((?P<url>(?:[^()]|\([^()]*\))*)\)"
)

# Matches HTML anchor tags: <a ... href="..." ...>label</a>
_HTML_A_RE = re.compile(
    r"<a\b(?P<attrs>[^>]*)>(?P<label>.*?)</a>", flags=re.IGNORECASE | re.DOTALL
)
_HREF_ATTR_RE = re.compile(
    r"""href\s*=\s*['"](?P<url>[^'"]*)['"]""", flags=re.IGNORECASE
)


def _is_safe_url_scheme(url: str) -> bool:
    """Check if URL uses a scheme the frontend renderer keeps (see simpleMarkdown.ts)."""
    clean_url = url.strip().lower()
    # Reject javascript:, data:, vbscript:, file:, etc.
    if any(
        clean_url.startswith(scheme)
        for scheme in ("javascript:", "data:", "vbscript:", "file:", "about:", "blob:")
    ):
        return False
    return (
        clean_url.startswith("http://")
        or clean_url.startswith("https://")
        or clean_url.startswith("mailto:")
        or clean_url.startswith("/")
    )


def guard_output_markdown(
    text: str,
    reject_on_delimiters: bool = False,
    tag: str = DEFAULT_DELIMITER_TAG,
) -> OutputGuardResult:
    """Validate and sanitize generated markdown output before presentation to users.

    1. Restricts links in markdown to http(s), mailto and same-origin paths, and
       drops javascript:, data: and other dangerous schemes.
    2. Detects, strips, or rejects delimiter tags (<article_content>, </article_content>).

    Args:
        text: Raw generated markdown or text from LLM
        reject_on_delimiters: If True, raises OutputGuardError when delimiter tags are found
        tag: Delimiter tag name to inspect

    Returns:
        OutputGuardResult containing sanitized text and metadata

    Raises:
        OutputGuardError: If reject_on_delimiters is True and delimiter tags are present
    """
    if not text:
        return OutputGuardResult(
            text=text,
            contains_delimiters=False,
            dropped_links_count=0,
            is_valid=True,
        )

    # Check for delimiter tags in the output
    tag_re = re.compile(rf"</?\s*{re.escape(tag)}\s*>", flags=re.IGNORECASE)
    has_delimiters = tag_re.search(text) is not None

    if has_delimiters and reject_on_delimiters:
        raise OutputGuardError(
            f"Generated output contains forbidden delimiter tag <{tag}>"
        )

    # Strip delimiter tags from output text
    cleaned_text = tag_re.sub("", text)

    dropped_count = 0

    # Sanitize markdown links [label](url) and images ![alt](url)
    def _replace_md_link(match: re.Match[str]) -> str:
        nonlocal dropped_count
        if _is_safe_url_scheme(match.group("url")):
            return match.group(0)
        dropped_count += 1
        # An image with a dropped source has nothing left to render, and its
        # alt text is attacker-supplied, so the whole construct goes.
        if match.group("bang"):
            return ""
        return match.group("label")

    cleaned_text = _MD_LINK_RE.sub(_replace_md_link, cleaned_text)

    # Sanitize any raw HTML <a> tags in generated markdown
    def _replace_html_a(match: re.Match[str]) -> str:
        nonlocal dropped_count
        attrs = match.group("attrs")
        label = match.group("label")
        href_match = _HREF_ATTR_RE.search(attrs)
        if href_match:
            url = href_match.group("url")
            if _is_safe_url_scheme(url):
                return match.group(0)
        dropped_count += 1
        return label

    cleaned_text = _HTML_A_RE.sub(_replace_html_a, cleaned_text)

    return OutputGuardResult(
        text=cleaned_text,
        contains_delimiters=has_delimiters,
        dropped_links_count=dropped_count,
        is_valid=True,
    )


def sanitize_output_markdown(
    text: str,
    tag: str = DEFAULT_DELIMITER_TAG,
) -> str:
    """Convenience wrapper returning sanitized markdown string directly."""
    return guard_output_markdown(text, reject_on_delimiters=False, tag=tag).text


class StreamingOutputGuard:
    """Run the output guard over a token stream without buffering the whole summary.

    Only markdown link syntax has to be withheld: a link cannot be judged before
    its URL is complete, so text flows straight through except between an
    unclosed "[" and its ")". Constructs that a single chunk cannot contain --
    an HTML anchor split across chunks, say -- stay the frontend sanitizer's job.
    """

    MAX_PENDING_CHARS: int = 512

    def __init__(self, tag: str = DEFAULT_DELIMITER_TAG) -> None:
        self._tag = tag
        self._pending = ""

    def push(self, chunk: str) -> str:
        """Ingest a chunk; return guarded text (empty while a link is pending)."""
        if not chunk:
            return ""
        self._pending += chunk

        emitted: list[str] = []
        while self._pending:
            start = self._pending.find("[")
            if start == -1:
                emitted.append(self._guard(self._pending))
                self._pending = ""
                break
            if start > 0:
                emitted.append(self._guard(self._pending[:start]))
                self._pending = self._pending[start:]

            end = self._construct_end(self._pending)
            if end is not None:
                emitted.append(self._guard(self._pending[:end]))
                self._pending = self._pending[end:]
                continue

            # A markdown link never spans a line break, and a "[" that is never
            # closed must not stall the stream behind an unbounded buffer.
            newline = self._pending.find("\n")
            cut = newline + 1 if newline != -1 else 0
            if not cut and len(self._pending) > self.MAX_PENDING_CHARS:
                cut = len(self._pending)
            if not cut:
                break
            emitted.append(self._guard(self._pending[:cut]))
            self._pending = self._pending[cut:]

        return "".join(emitted)

    def flush(self) -> str:
        """Release and guard whatever is still pending (end of stream)."""
        pending, self._pending = self._pending, ""
        return self._guard(pending) if pending else ""

    @staticmethod
    def _construct_end(text: str) -> int | None:
        """Index just past the pending "[" construct, or None while it may still grow."""
        label_end = text.find("]")
        if label_end == -1 or label_end + 1 >= len(text):
            return None
        if text[label_end + 1] != "(":
            # A bare "[1]" reference marker, not a link.
            return label_end + 1
        depth = 0
        for index in range(label_end + 1, len(text)):
            if text[index] == "(":
                depth += 1
            elif text[index] == ")":
                depth -= 1
                if depth == 0:
                    return index + 1
        return None

    def _guard(self, text: str) -> str:
        return sanitize_output_markdown(text, tag=self._tag)
