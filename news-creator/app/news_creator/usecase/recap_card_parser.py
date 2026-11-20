"""Parser for recap card generation tagged free text output."""

from __future__ import annotations

import logging
import re
from dataclasses import dataclass

from news_creator.domain.models import (
    Card422Reason,
    CardContent,
    CardParseDetail,
    CardSentence,
)
from news_creator.domain.prompt_boundary import _strip_hidden
from news_creator.domain.prompts import _THINKING_BLOCK_RE, neutralize_control_tokens

logger = logging.getLogger(__name__)

JAPANESE_CHAR_RE = re.compile(r"[\u3040-\u30ff\u3400-\u4dbf\u4e00-\u9fff]")
REF_EXTRACTION_RE = re.compile(r"\[(\d+)\]")
ENDS_WITH_REF_RE = re.compile(r"\[\d+\](?:\s*\[\d+\])*[\s。！？!?]*$")


@dataclass
class CardParseResult:
    """Result of parsing and validating raw LLM card output."""

    success: bool
    card: CardContent | None = None
    reason: Card422Reason | None = None
    detail: CardParseDetail | None = None
    measured_ratio: float | None = None
    character_counts: dict[str, int] | None = None


def clean_raw_output(text: str) -> str:
    """Strip thinking blocks, invisible characters, control tokens, code fences, and normalize brackets."""
    if not text:
        return ""

    # Strip thinking blocks (Gemma 4 channel thoughts and standard <think> blocks)
    cleaned = _THINKING_BLOCK_RE.sub("", text)
    cleaned = re.sub(r"<think>.*?</think>", "", cleaned, flags=re.DOTALL)

    # Strip zero-width, invisible formatting, BiDi overrides, and control chars
    cleaned, _ = _strip_hidden(cleaned)

    # Neutralize turn/control tokens using canonical domain helper
    cleaned, _ = neutralize_control_tokens(cleaned)

    # Normalize full-width brackets ［1］ to [1]
    cleaned = cleaned.replace("［", "[").replace("］", "]")

    # Strip leading/trailing code fences (e.g. ```markdown ... ```) and surrounding whitespace
    cleaned = cleaned.strip()
    cleaned = re.sub(r"^```[a-zA-Z0-9_\-]*\s*\n?", "", cleaned)
    cleaned = re.sub(r"\n?\s*```\s*$", "", cleaned)

    return cleaned.strip()


def normalize_citations(text: str) -> str:
    """Normalize citation formats like [1, 2], [1]・[2], [1]および[2] to [1] [2]."""

    def _expand_bracket_commas(match: re.Match[str]) -> str:
        digits = re.findall(r"\d+", match.group(0))
        return " ".join(f"[{d}]" for d in digits)

    text = re.sub(r"\[\d+(?:\s*[,、]\s*\d+)+\]", _expand_bracket_commas, text)
    return re.sub(r"(?<=\])\s*(?:・|および|、|,)\s*(?=\[\d+\])", " ", text)


_SENTENCE_END_CHARS = ("。", "！", "？", "!", "?")
_WHITESPACE_CHARS = (" ", "\u3000", "\t")
_MAX_ALLOWED_LATIN_TOKENS = 4
_MIN_WHAT_SENTENCES = 2
_MAX_WHAT_SENTENCES = 3
_MAX_HEADLINE_CHARS = 60


def _consume_whitespace(line: str, idx: int) -> int:
    n = len(line)
    while idx < n and line[idx] in _WHITESPACE_CHARS:
        idx += 1
    return idx


def _consume_citations(line: str, idx: int) -> int:
    n = len(line)
    while idx < n and line[idx] == "[":
        close_idx = line.find("]", idx)
        if close_idx != -1 and line[idx + 1 : close_idx].isdigit():
            idx = _consume_whitespace(line, close_idx + 1)
        else:
            break
    return idx


def _split_line_sentences(line: str) -> list[str]:
    sentences: list[str] = []
    start = 0
    i = 0
    n = len(line)
    while i < n:
        if line[i] in _SENTENCE_END_CHARS:
            i = _consume_citations(line, _consume_whitespace(line, i + 1))
            if i < n and line[i] in _SENTENCE_END_CHARS:
                i += 1
            sent = line[start:i].strip()
            if sent:
                sentences.append(sent)
            start = i
        elif line[i] == "[":
            after_cites = _consume_citations(line, i)
            if after_cites == i:
                i += 1
                continue
            i = after_cites
            if i < n and line[i] in _SENTENCE_END_CHARS:
                i += 1
            if i >= n or line[i] not in (*_SENTENCE_END_CHARS, "[", "、", ","):
                sent = line[start:i].strip()
                if sent:
                    sentences.append(sent)
                start = i
        else:
            i += 1

    if start < n:
        sent = line[start:n].strip()
        if sent:
            sentences.append(sent)
    return sentences


def split_sentences(text: str) -> list[str]:
    """Split Japanese text into sentences respecting terminal citation markers [n]."""
    text = text.strip()
    if not text:
        return []

    lines = [line.strip() for line in text.splitlines() if line.strip()]
    sentences: list[str] = []
    for line in lines:
        sentences.extend(_split_line_sentences(line))
    return sentences


def extract_sections(text: str) -> dict[str, str]:
    """Extract sections demarcated by Japanese square bracket tags 【...】."""
    # Normalize markdown wrappers like **【見出し】** -> 【見出し】
    normalized = re.sub(r"[*#`]+【", "【", text)
    normalized = re.sub(r"】[*#`]+", "】", normalized)

    tag_matches = list(
        re.finditer(r"【(見出し|何が起きた|なぜ重要|出典)】", normalized)
    )
    sections: dict[str, str] = {}
    for idx, match in enumerate(tag_matches):
        tag_name = match.group(1)
        start_pos = match.end()
        end_pos = (
            tag_matches[idx + 1].start()
            if idx + 1 < len(tag_matches)
            else len(normalized)
        )
        sections[tag_name] = normalized[start_pos:end_pos].strip()
    return sections


LATIN_RUN_PATTERN = (
    r"(?<![A-Za-z0-9])[A-Za-z0-9]+(?:[ \-\.]+[A-Za-z0-9]+)*(?![A-Za-z0-9])"
)
LATIN_RUN_RE = re.compile(LATIN_RUN_PATTERN)


def strip_source_latin_runs(text: str, source_texts: list[str]) -> str:
    """Strip Latin-script runs that are exact members of the allowed <= 4 token source runs."""
    if not source_texts or not text:
        return text

    allowed_runs: set[str] = set()
    for src in source_texts:
        for m in LATIN_RUN_RE.finditer(src):
            run = m.group(0)
            if any("a" <= c <= "z" or "A" <= c <= "Z" for c in run):
                tokens = run.split()
                if len(tokens) <= _MAX_ALLOWED_LATIN_TOKENS:
                    allowed_runs.add(run.lower())

    if not allowed_runs:
        return text

    def _replace_run(match: re.Match[str]) -> str:
        run = match.group(0)
        if run.lower() in allowed_runs:
            return ""
        return run

    return LATIN_RUN_RE.sub(_replace_run, text)


def measure_japanese_ratio(
    text: str,
    threshold: float = 0.6,
    source_texts: list[str] | None = None,
) -> tuple[bool, float, dict[str, int]]:
    """
    Measure Japanese character ratio and character counts (G1 gate).

    Returns:
        (is_sufficient, measured_ratio, character_counts)
    """
    total_chars = len(text)
    stripped_text = (
        strip_source_latin_runs(text, source_texts) if source_texts else text
    )
    stripped_chars = total_chars - len(stripped_text)

    substantive = re.sub(r"[\s0-9\[\]!?,.。！？:/\-_~#*`'\"]", "", stripped_text)
    substantive_chars = len(substantive)
    ja_chars = len(JAPANESE_CHAR_RE.findall(substantive))
    raw_ratio = (ja_chars / substantive_chars) if substantive_chars > 0 else 0.0
    sufficient = raw_ratio >= threshold if substantive_chars > 0 else False
    character_counts = {
        "japanese": ja_chars,
        "substantive": substantive_chars,
        "total": total_chars,
        "stripped": stripped_chars,
    }
    return sufficient, round(raw_ratio, 4), character_counts


def _parse_what_sentences(
    what_raw: str,
    valid_refs: set[int],
    all_refs: set[int],
) -> tuple[list[CardSentence] | None, CardParseResult | None]:
    what_sentences_text = split_sentences(what_raw)
    if (
        len(what_sentences_text) < _MIN_WHAT_SENTENCES
        or len(what_sentences_text) > _MAX_WHAT_SENTENCES
    ):
        return None, CardParseResult(
            success=False, reason="parse_failed", detail="sentence_count"
        )

    what_ja: list[CardSentence] = []
    for sent_text in what_sentences_text:
        refs = [int(m) for m in REF_EXTRACTION_RE.findall(sent_text)]
        if not refs:
            return None, CardParseResult(
                success=False, reason="parse_failed", detail="missing_citation"
            )
        if not ENDS_WITH_REF_RE.search(sent_text):
            return None, CardParseResult(
                success=False, reason="parse_failed", detail="unknown_citation_format"
            )
        for r in refs:
            if r not in valid_refs:
                return None, CardParseResult(success=False, reason="unknown_ref")
            all_refs.add(r)
        what_ja.append(CardSentence(text=sent_text, refs=refs))
    return what_ja, None


def _parse_why_sentence(
    why_raw: str,
    valid_refs: set[int],
    all_refs: set[int],
) -> tuple[CardSentence | None, CardParseResult | None]:
    if not why_raw or why_raw.startswith("該当なし"):
        return None, None

    normalized_why = normalize_citations(why_raw)
    why_sentences_text = split_sentences(normalized_why)
    if len(why_sentences_text) != 1:
        return None, CardParseResult(
            success=False, reason="parse_failed", detail="why_format"
        )

    why_text = why_sentences_text[0]
    why_refs = [int(m) for m in REF_EXTRACTION_RE.findall(why_text)]
    if not why_refs or not ENDS_WITH_REF_RE.search(why_text):
        return None, CardParseResult(
            success=False, reason="parse_failed", detail="why_format"
        )
    for r in why_refs:
        if r not in valid_refs:
            return None, CardParseResult(success=False, reason="unknown_ref")
        all_refs.add(r)
    return CardSentence(text=why_text, refs=why_refs), None


def _validate_sections_and_headline(
    sections: dict[str, str],
) -> tuple[str | None, CardParseResult | None]:
    if "見出し" not in sections or "何が起きた" not in sections:
        return None, CardParseResult(
            success=False, reason="parse_failed", detail="missing_tag"
        )
    if "出典" in sections and not sections["出典"].strip():
        return None, CardParseResult(
            success=False, reason="parse_failed", detail="sources_tag"
        )
    headline = normalize_citations(sections["見出し"].strip())
    headline = re.sub(r"(?:\s*\[\d+\]\s*)+", " ", headline).strip()
    if not headline:
        return None, CardParseResult(
            success=False, reason="parse_failed", detail="missing_tag"
        )
    if len(headline) > _MAX_HEADLINE_CHARS:
        return None, CardParseResult(
            success=False, reason="parse_failed", detail="headline_too_long"
        )
    return headline, None


def parse_card_output(
    raw_text: str,
    valid_refs: set[int],
    ja_ratio_threshold: float = 0.6,
    source_texts: list[str] | None = None,
) -> CardParseResult:
    """
    Parse LLM raw output into structured CardContent.

    Args:
        raw_text: Raw LLM response string
        valid_refs: Set of valid reference numbers (from input items)
        ja_ratio_threshold: Minimum Japanese character ratio (default 0.6)
        source_texts: Optional source texts (titles/ledes) used to filter known Latin proper nouns

    Returns:
        CardParseResult with either CardContent or 422 failure reason
    """
    cleaned = clean_raw_output(raw_text)
    if not cleaned:
        return CardParseResult(success=False, reason="empty_output")

    sections = extract_sections(cleaned)
    headline, err = _validate_sections_and_headline(sections)
    if err is not None or headline is None:
        return err or CardParseResult(
            success=False, reason="parse_failed", detail="missing_tag"
        )

    all_refs: set[int] = set()
    what_raw = normalize_citations(sections["何が起きた"].strip())
    what_ja, what_err = _parse_what_sentences(what_raw, valid_refs, all_refs)
    if what_err is not None or what_ja is None:
        return what_err or CardParseResult(
            success=False, reason="parse_failed", detail="sentence_count"
        )

    why_raw = sections.get("なぜ重要", "").strip()
    why_ja, why_err = _parse_why_sentence(why_raw, valid_refs, all_refs)
    if why_err is not None:
        return why_err

    full_card_text = headline + " " + " ".join(s.text for s in what_ja)
    if why_ja:
        full_card_text += " " + why_ja.text

    ja_sufficient, ja_ratio, char_counts = measure_japanese_ratio(
        full_card_text, ja_ratio_threshold, source_texts=source_texts
    )
    if not ja_sufficient:
        return CardParseResult(
            success=False,
            reason="language",
            measured_ratio=ja_ratio,
            character_counts=char_counts,
        )

    card_content = CardContent(
        headline_ja=headline,
        what_ja=what_ja,
        why_ja=why_ja,
        used_refs=sorted(all_refs),
    )

    return CardParseResult(
        success=True,
        card=card_content,
        measured_ratio=ja_ratio,
        character_counts=char_counts,
    )
