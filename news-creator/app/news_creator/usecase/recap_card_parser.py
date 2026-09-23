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
from news_creator.domain.prompts import neutralize_control_tokens

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
    cleaned = re.sub(r"<\|channel>thought.*?<channel\|>", "", text, flags=re.DOTALL)
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
    text = re.sub(r"(?<=\])\s*(?:・|および|、|,)\s*(?=\[\d+\])", " ", text)
    return text


def split_sentences(text: str) -> list[str]:
    """Split Japanese text into sentences respecting terminal citation markers [n]."""
    text = text.strip()
    if not text:
        return []

    lines = [line.strip() for line in text.splitlines() if line.strip()]
    sentences: list[str] = []

    for line in lines:
        start = 0
        i = 0
        n = len(line)
        while i < n:
            if line[i] in ("。", "！", "？", "!", "?"):
                i += 1
                # Consume any trailing whitespace
                while i < n and line[i] in (" ", "\u3000", "\t"):
                    i += 1
                # Consume any trailing [digits] citation tags
                while i < n and line[i] == "[":
                    close_idx = line.find("]", i)
                    if close_idx != -1 and line[i + 1 : close_idx].isdigit():
                        i = close_idx + 1
                        while i < n and line[i] in (" ", "\u3000", "\t"):
                            i += 1
                    else:
                        break
                # Optional trailing punctuation after citation (e.g. [1]。)
                if i < n and line[i] in ("。", "！", "？", "!", "?"):
                    i += 1
                sent = line[start:i].strip()
                if sent:
                    sentences.append(sent)
                start = i
            elif line[i] == "[":
                # Check if this is a citation at end of sentence missing terminal punctuation
                close_idx = line.find("]", i)
                if close_idx != -1 and line[i + 1 : close_idx].isdigit():
                    i = close_idx + 1
                    while i < n and line[i] in (" ", "\u3000", "\t"):
                        i += 1
                    while i < n and line[i] == "[":
                        c2 = line.find("]", i)
                        if c2 != -1 and line[i + 1 : c2].isdigit():
                            i = c2 + 1
                            while i < n and line[i] in (" ", "\u3000", "\t"):
                                i += 1
                        else:
                            break
                    if i < n and line[i] in ("。", "！", "？", "!", "?"):
                        i += 1
                    if i >= n or (
                        i < n
                        and line[i] not in ("。", "！", "？", "!", "?", "[", "、", ",")
                    ):
                        sent = line[start:i].strip()
                        if sent:
                            sentences.append(sent)
                        start = i
                else:
                    i += 1
            else:
                i += 1
        if start < n:
            sent = line[start:n].strip()
            if sent:
                sentences.append(sent)

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
                if len(tokens) <= 4:
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
    if "見出し" not in sections or "何が起きた" not in sections:
        return CardParseResult(
            success=False, reason="parse_failed", detail="missing_tag"
        )

    if "出典" in sections and not sections["出典"].strip():
        return CardParseResult(
            success=False, reason="parse_failed", detail="sources_tag"
        )

    headline = sections["見出し"].strip()
    if not headline:
        return CardParseResult(
            success=False, reason="parse_failed", detail="missing_tag"
        )
    if len(headline) > 60:
        return CardParseResult(
            success=False, reason="parse_failed", detail="headline_too_long"
        )

    what_raw = normalize_citations(sections["何が起きた"].strip())
    what_sentences_text = split_sentences(what_raw)
    if len(what_sentences_text) < 2 or len(what_sentences_text) > 3:
        return CardParseResult(
            success=False, reason="parse_failed", detail="sentence_count"
        )

    what_ja: list[CardSentence] = []
    all_refs: set[int] = set()

    for sent_text in what_sentences_text:
        refs = [int(m) for m in REF_EXTRACTION_RE.findall(sent_text)]
        if not refs:
            return CardParseResult(
                success=False, reason="parse_failed", detail="missing_citation"
            )
        if not ENDS_WITH_REF_RE.search(sent_text):
            return CardParseResult(
                success=False, reason="parse_failed", detail="unknown_citation_format"
            )

        for r in refs:
            if r not in valid_refs:
                return CardParseResult(success=False, reason="unknown_ref")
            all_refs.add(r)

        what_ja.append(CardSentence(text=sent_text, refs=refs))

    why_ja: CardSentence | None = None
    why_raw = sections.get("なぜ重要", "").strip()

    if why_raw and not why_raw.startswith("該当なし"):
        why_raw = normalize_citations(why_raw)
        why_sentences_text = split_sentences(why_raw)
        if len(why_sentences_text) != 1:
            return CardParseResult(
                success=False, reason="parse_failed", detail="why_format"
            )

        why_text = why_sentences_text[0]
        why_refs = [int(m) for m in REF_EXTRACTION_RE.findall(why_text)]
        if not why_refs or not ENDS_WITH_REF_RE.search(why_text):
            return CardParseResult(
                success=False, reason="parse_failed", detail="why_format"
            )

        for r in why_refs:
            if r not in valid_refs:
                return CardParseResult(success=False, reason="unknown_ref")
            all_refs.add(r)

        why_ja = CardSentence(text=why_text, refs=why_refs)

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

    used_refs = sorted(list(all_refs))

    card_content = CardContent(
        headline_ja=headline,
        what_ja=what_ja,
        why_ja=why_ja,
        used_refs=used_refs,
    )

    return CardParseResult(
        success=True,
        card=card_content,
        measured_ratio=ja_ratio,
        character_counts=char_counts,
    )
