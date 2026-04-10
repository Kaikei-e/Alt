"""Extractive compression — section-aware span selection from article bodies.

Stage 1 of the 2-stage extraction pipeline (resolve doc):
  Stage 1 = CompressorNode (heuristic, quote/span selection) — this module
  Stage 2 = ExtractorNode (LLM structured output, fact normalization)

Compression strategy:
  - Sentence splitting (JP/EN/mixed, conservative on abbreviations/decimals, \n support)
  - Dual scoring: ASCII word overlap + CJK character bi-gram Jaccard
  - Top-K selection within char_budget
  - Selective augmentation: returns [] when nothing is relevant
  - Packing order: score-descending (head-bias for Lost-in-the-Middle mitigation)
"""

from __future__ import annotations

import re
from dataclasses import dataclass

# Minimum relevance score to consider a sentence relevant (selective augmentation).
_RELEVANCE_THRESHOLD = 0.01


@dataclass(frozen=True)
class CompressedSpan:
    """A selected text span from an article body."""

    text: str
    char_offset: int
    relevance_score: float


# Conservative sentence boundary pattern:
#   - Split after 。！？ (Japanese sentence-enders) followed by optional whitespace
#   - Split after . ! ? only when followed by whitespace + uppercase letter or CJK
#     This avoids splitting on U.S., 3.14%, etc.
#   - Split on single newline (\n) as line/paragraph break (for RSS bullet points, headlines)
_SENTENCE_BOUNDARY = re.compile(
    r"(?<=[。！？])\s*"  # Japanese: split after 。！？
    r"|"
    r"(?<=[.!?])\s+(?=[A-Z\u3040-\u9fff])"  # English: . ! ? + space + uppercase/CJK
    r"|"
    r"\n"  # Newline as line break (bullet points, headlines, paragraphs)
)

# CJK character ranges (excluding punctuation/symbols U+3000-303F)
_CJK_CHAR_RE = re.compile(r"[\u3040-\u309f\u30a0-\u30ff\u4e00-\u9fff]")


def split_sentences(text: str) -> list[tuple[str, int]]:
    """Split text into (sentence, char_offset) pairs. Conservative on abbreviations/decimals."""
    if not text:
        return []

    parts = _SENTENCE_BOUNDARY.split(text)
    result: list[tuple[str, int]] = []
    offset = 0
    for part in parts:
        stripped = part.strip()
        if stripped:
            # Find actual start position in original text
            actual_offset = text.find(stripped, offset)
            if actual_offset == -1:
                actual_offset = offset
            result.append((stripped, actual_offset))
            offset = actual_offset + len(stripped)
    return result


def _cjk_bigrams(text: str) -> set[tuple[str, str]]:
    """Extract character bigrams from CJK portions of text."""
    cjk_chars = "".join(_CJK_CHAR_RE.findall(text))
    if len(cjk_chars) < 2:
        return set()
    return {(cjk_chars[i], cjk_chars[i + 1]) for i in range(len(cjk_chars) - 1)}


def score_sentence(sentence: str, query_terms: set[str]) -> float:
    """Score sentence relevance via dual approach: ASCII words + CJK bi-gram Jaccard.

    For ASCII: exact word match (case-insensitive).
    For CJK: character bi-gram Jaccard overlap between query and sentence.
    Final score = max(ascii_score, cjk_score) to ensure either track can contribute.
    """
    if not query_terms:
        return 0.0

    lower = sentence.lower()

    # Track 1: ASCII word matching
    ascii_words = set(re.findall(r"[a-z0-9]+", lower))
    ascii_hits = 0
    for term in query_terms:
        if term in ascii_words or term in lower:
            ascii_hits += 1
    ascii_score = ascii_hits / len(query_terms) if query_terms else 0.0

    # Track 2: CJK bi-gram Jaccard overlap
    sent_bigrams = _cjk_bigrams(lower)
    cjk_score = 0.0
    if sent_bigrams:
        # Compute overlap against ALL query bigrams combined
        query_text = " ".join(query_terms)
        query_bigrams = _cjk_bigrams(query_text)
        if query_bigrams:
            intersection = sent_bigrams & query_bigrams
            union = sent_bigrams | query_bigrams
            cjk_score = len(intersection) / len(union) if union else 0.0

    return max(ascii_score, cjk_score)


def compress_article(body: str, queries: list[str], *, char_budget: int = 1000) -> list[CompressedSpan]:
    """Extractive compression: select relevant sentences within char_budget.

    Returns [] when no sentence scores above threshold (selective augmentation).
    Packing order: score-descending (strongest evidence first).
    """
    if not body or not body.strip():
        return []

    sentences = split_sentences(body)
    if not sentences:
        return []

    terms = _extract_query_terms(queries)

    # Score each sentence
    scored: list[tuple[str, int, float]] = []
    for sent_text, sent_offset in sentences:
        s = score_sentence(sent_text, terms) if terms else 0.0
        scored.append((sent_text, sent_offset, s))

    # Filter by relevance threshold (selective augmentation)
    relevant = [(t, o, s) for t, o, s in scored if s >= _RELEVANCE_THRESHOLD]
    if not relevant:
        return []

    # Sort by score descending for selection
    relevant.sort(key=lambda x: x[2], reverse=True)

    # Select top sentences within char_budget
    selected: list[tuple[str, int, float]] = []
    total_chars = 0
    for t, o, s in relevant:
        if total_chars + len(t) > char_budget and selected:
            break
        selected.append((t, o, s))
        total_chars += len(t)

    # Packing order: score-descending (strongest evidence at head of prompt)
    return [CompressedSpan(text=t, char_offset=o, relevance_score=s) for t, o, s in selected]


def select_top_sentences(
    body: str,
    queries: list[str],
    *,
    max_sentences: int = 3,
    max_len: int = 200,
    position_fallback: bool = False,
) -> list[CompressedSpan]:
    """Select top-N most relevant sentences for given queries.

    Unlike compress_article() which packs within a char_budget,
    this function selects exactly up to max_sentences sentences.

    When position_fallback=False (primary heuristic): returns [] if no
    sentence scores above threshold, allowing caller to try secondary paths.
    When position_fallback=True (tertiary fallback): returns first N sentences
    regardless of score, guaranteeing non-empty output for non-empty body.
    """
    if not body or not body.strip():
        return []

    sentences = split_sentences(body)
    if not sentences:
        return []

    terms = _extract_query_terms(queries)

    scored: list[tuple[str, int, float]] = []
    for sent_text, sent_offset in sentences:
        s = score_sentence(sent_text, terms) if terms else 0.0
        scored.append((sent_text, sent_offset, s))

    relevant = [(t, o, s) for t, o, s in scored if s >= _RELEVANCE_THRESHOLD]

    if relevant:
        relevant.sort(key=lambda x: x[2], reverse=True)
        selected = relevant[:max_sentences]
    elif position_fallback:
        selected = scored[:max_sentences]
    else:
        return []

    return [
        CompressedSpan(
            text=t[:max_len],
            char_offset=o,
            relevance_score=s,
        )
        for t, o, s in selected
    ]


def _extract_query_terms(queries: list[str]) -> set[str]:
    """Extract search terms from query strings.

    ASCII words are extracted individually (word-boundary split).
    CJK text is kept as-is in terms for bi-gram scoring in score_sentence().
    Japanese cannot be tokenized by regex alone, so we pass CJK chunks
    through to score_sentence which uses character bi-gram Jaccard.
    """
    terms: set[str] = set()
    for q in queries:
        # Extract ASCII words
        for word in re.findall(r"[a-zA-Z0-9]+", q.lower()):
            if len(word) > 1:
                terms.add(word)
        # Extract CJK chunks (contiguous CJK characters, excluding punctuation)
        cjk_chunks = re.findall(r"[\u3040-\u309f\u30a0-\u30ff\u4e00-\u9fff]+", q)
        for chunk in cjk_chunks:
            if len(chunk) >= 2:
                terms.add(chunk)
    return terms
