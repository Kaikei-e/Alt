"""Parser for recap card generation tagged free text output."""

from __future__ import annotations

import logging
import re
from dataclasses import dataclass

from news_creator.domain.models import (
    Card422Reason,
    CardContent,
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
                        i < n and line[i] not in ("。", "！", "？", "!", "?", "[")
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


def measure_japanese_ratio(
    text: str, threshold: float = 0.6
) -> tuple[bool, float, dict[str, int]]:
    """
    Measure Japanese character ratio and character counts (G1 gate).

    Returns:
        (is_sufficient, measured_ratio, character_counts)
    """
    substantive = re.sub(r"[\s0-9\[\]!?,.。！？:/\-_~#*`'\"]", "", text)
    total_chars = len(text)
    substantive_chars = len(substantive)
    ja_chars = len(JAPANESE_CHAR_RE.findall(substantive))
    raw_ratio = (ja_chars / substantive_chars) if substantive_chars > 0 else 0.0
    sufficient = raw_ratio >= threshold if substantive_chars > 0 else False
    character_counts = {
        "japanese": ja_chars,
        "substantive": substantive_chars,
        "total": total_chars,
    }
    return sufficient, round(raw_ratio, 4), character_counts


def parse_card_output(
    raw_text: str,
    valid_refs: set[int],
    ja_ratio_threshold: float = 0.6,
) -> CardParseResult:
    """
    Parse LLM raw output into structured CardContent.

    Args:
        raw_text: Raw LLM response string
        valid_refs: Set of valid reference numbers (from input items)
        ja_ratio_threshold: Minimum Japanese character ratio (default 0.6)

    Returns:
        CardParseResult with either CardContent or 422 failure reason
    """
    cleaned = clean_raw_output(raw_text)
    if not cleaned:
        return CardParseResult(success=False, reason="empty_output")

    sections = extract_sections(cleaned)
    if "見出し" not in sections or "何が起きた" not in sections:
        return CardParseResult(success=False, reason="parse_failed")

    headline = sections["見出し"].strip()
    # Headline must be non-empty and <= 40 chars
    if not headline or len(headline) > 40:
        return CardParseResult(success=False, reason="parse_failed")

    # Split what_ja into sentences
    what_raw = sections["何が起きた"].strip()
    what_sentences_text = split_sentences(what_raw)
    if len(what_sentences_text) < 2 or len(what_sentences_text) > 3:
        return CardParseResult(success=False, reason="parse_failed")

    what_ja: list[CardSentence] = []
    all_refs: set[int] = set()

    for sent_text in what_sentences_text:
        # Each sentence must end with [n]
        if not ENDS_WITH_REF_RE.search(sent_text):
            return CardParseResult(success=False, reason="parse_failed")

        refs = [int(m) for m in REF_EXTRACTION_RE.findall(sent_text)]
        if not refs:
            return CardParseResult(success=False, reason="parse_failed")

        # Every ref must exist in valid_refs
        for r in refs:
            if r not in valid_refs:
                return CardParseResult(success=False, reason="unknown_ref")
            all_refs.add(r)

        what_ja.append(CardSentence(text=sent_text, refs=refs))

    # Parse why_ja (optional, or '該当なし')
    why_ja: CardSentence | None = None
    why_raw = sections.get("なぜ重要", "").strip()

    if why_raw and not why_raw.startswith("該当なし"):
        why_sentences_text = split_sentences(why_raw)
        if len(why_sentences_text) != 1:
            return CardParseResult(success=False, reason="parse_failed")

        why_text = why_sentences_text[0]
        if not ENDS_WITH_REF_RE.search(why_text):
            return CardParseResult(success=False, reason="parse_failed")

        why_refs = [int(m) for m in REF_EXTRACTION_RE.findall(why_text)]
        if not why_refs:
            return CardParseResult(success=False, reason="parse_failed")

        for r in why_refs:
            if r not in valid_refs:
                return CardParseResult(success=False, reason="unknown_ref")
            all_refs.add(r)

        why_ja = CardSentence(text=why_text, refs=why_refs)

    # G1 Language gate: Japanese character ratio must be >= threshold
    full_card_text = headline + " " + " ".join(s.text for s in what_ja)
    if why_ja:
        full_card_text += " " + why_ja.text

    ja_sufficient, ja_ratio, char_counts = measure_japanese_ratio(
        full_card_text, ja_ratio_threshold
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

    return CardParseResult(success=True, card=card_content)
