"""Japanese keyword extraction using Fugashi morphological analysis and KeyBERT semantic scoring."""

import re
from collections import Counter
from typing import Any

import structlog

from .config import TagExtractionConfig
from .tag_validator import is_valid_japanese_tag as _shared_is_valid_japanese_tag

logger = structlog.get_logger(__name__)

_EXCLUDED_UNIDIC_POS2 = frozenset({"数詞", "助動詞語幹"})
_EXCLUDED_UNIDIC_POS3 = frozenset({"副詞可能", "助数詞可能"})

# Tags that can connect nouns (e.g., の between nouns)
CONNECTOR_SURFACES = {"の", "・", "＝", "－", "-"}

# Regex patterns for mixed-script and special compounds
COMPOUND_PATTERNS = [
    # Tech terms with mixed scripts
    r"(?<![A-Za-z0-9])[A-Za-z][A-Za-z0-9]*[ァ-ヶー]+(?:[A-Za-z0-9]*)?",  # e.g., "GitHubリポジトリ"
    r"[ァ-ヶー]+[A-Za-z][A-Za-z0-9]*",  # e.g., "データセットID"
    r"(?<![A-Za-z0-9])[A-Za-z][A-Za-z0-9]*(?:\+\+|#)",  # Symbolic languages (e.g., "C++", "C#")
    r"(?<![A-Za-z0-9])[A-Za-z][A-Za-z0-9]*(?:[.-]\d+(?:\.\d+)*|/\d+)",  # e.g., "GPT-4", "HTTP/3"
    r"(?<![A-Za-z0-9])[a-z]+[A-Z][A-Za-z0-9]*",  # Mixed-case brands (e.g., "iOS17", "macOS")
    r"(?<![A-Za-z0-9])[A-Z][A-Za-z0-9]*(?:\.[A-Za-z]+)?",  # CamelCase and dotted (e.g., "TensorFlow", "Next.js")
    r"(?<![A-Za-z0-9])[A-Z]{2,}(?:[a-z]+)?",  # Acronyms like "AWS", "API", "CEO"
    r"[一-龥]{2,}[ァ-ヶー]+",  # Kanji + Katakana compounds
    # Important proper nouns with titles
    r"[一-龥ァ-ヶー]{2,}(?:大統領|首相|総理|議員|知事|市長|社長|CEO)",
    r"[一-龥ァ-ヶー]{2,}(?:会社|企業|組織|団体|協会|連盟|大学|研究所)",
    # Tech-specific patterns
    r"[ァ-ヶー]{3,}(?:システム|サービス|プラットフォーム|フレームワーク|ライブラリ)",
    r"[ァ-ヶー]{2,}(?:アーキテクチャ|インフラ|ネットワーク|セキュリティ)",
    # Alphanumeric (requiring at least one ASCII letter) + Japanese
    r"(?<![A-Za-z0-9])[A-Za-z][A-Za-z0-9]*[一-龥ァ-ヶー]{2,}",  # e.g., "IoT機器", "Web技術", "AI技術"
    r"[一-龥ァ-ヶー]{2,}[A-Za-z][A-Za-z0-9]*",  # e.g., "機械学習API"
    r"\d+[A-Za-z][A-Za-z0-9ァ-ヶー一-龥]*",  # Number + Alpha-prefixed word (e.g., "5G通信", "3Dプリンター")
    r"第[0-9〇零一二三四五六七八九十百]+[一-龥ァ-ヶー]{1,6}",  # Ordinal compounds (e.g., "第3世代")
]


def is_japanese_noun(
    token: Any,
    allowed_pos_tags: set[str] | tuple[str, ...] | None = None,
) -> bool:
    """Return True when a Fugashi token is a noun (including numerals/counters for compound chaining)."""
    feature = getattr(token, "feature", None)
    pos1 = getattr(feature, "pos1", "")
    if not isinstance(pos1, str) or not pos1:
        return False
    if allowed_pos_tags is not None:
        return pos1 in allowed_pos_tags
    return pos1.startswith("名詞")


def is_topical_japanese_noun(
    token: Any,
    allowed_pos_tags: set[str] | tuple[str, ...] | None = None,
) -> bool:
    """Return True when a Fugashi token represents a topical noun rather than a numeral, counter, or temporal noun."""
    if not is_japanese_noun(token, allowed_pos_tags):
        return False

    feature = getattr(token, "feature", None)
    pos2 = getattr(feature, "pos2", "")
    if isinstance(pos2, str) and pos2 in _EXCLUDED_UNIDIC_POS2:
        return False

    pos3 = getattr(feature, "pos3", "")
    if isinstance(pos3, str) and pos3 in _EXCLUDED_UNIDIC_POS3:
        return False

    return True


def _is_compound_chain_token(token: Any, next_token: Any | None, has_preceding: bool) -> bool:
    """Check if token belongs in a contiguous Japanese noun compound chain."""
    if is_japanese_noun(token):
        return True
    feature = getattr(token, "feature", None)
    pos1 = getattr(feature, "pos1", "")
    pos2 = getattr(feature, "pos2", "")
    if pos1 == "接頭辞" and getattr(token, "surface", "") == "第":
        return next_token is not None and is_japanese_noun(next_token)
    if has_preceding and pos1 == "接尾辞" and pos2 == "名詞的":
        return True
    return False


def _finalize_compound(tokens: list[Any], compounds: list[str], max_length: int) -> None:
    """Append joined compound if it has >=2 tokens, contains a topical noun, and passes tag validation."""
    if len(tokens) < 2:
        return
    if not any(is_topical_japanese_noun(tok) for tok in tokens):
        return
    compound = "".join(tok.surface for tok in tokens)
    if 3 <= len(compound) <= max_length and _shared_is_valid_japanese_tag(compound, max_length=max_length):
        compounds.append(compound)


def extract_compound_nouns_fugashi(text: str, ja_tagger: Any, max_length: int = 15) -> list[str]:
    """
    Extract compound nouns by chaining consecutive noun tokens.

    Identifies sequences of consecutive nouns (including internal numerals,
    counters, and nominal suffixes) and joins them to form compound nouns,
    rejecting compounds that lack any topical noun or match pure numeric+counter
    expressions.

    Args:
        text: Input Japanese text
        ja_tagger: Fugashi tagger instance
        max_length: Maximum allowed character length for compound nouns

    Returns:
        List of compound nouns (2+ consecutive tokens joined)
    """
    if ja_tagger is None:
        raise RuntimeError("Japanese tagger not initialized")

    # Split on line breaks before invoking MeCab because MeCab discards whitespace
    # tokens and would otherwise glue the last noun of a title/paragraph onto the
    # first noun of the next line.
    segments = [seg for seg in re.split(r"[\r\n]+", text) if seg] or [text]
    compounds: list[str] = []

    for segment in segments:
        parsed = list(ja_tagger(segment))
        current_tokens: list[Any] = []

        for i, token in enumerate(parsed):
            surface = token.surface
            next_tok = parsed[i + 1] if i + 1 < len(parsed) else None
            is_chainable = _is_compound_chain_token(token, next_tok, bool(current_tokens))
            is_connector = surface in CONNECTOR_SURFACES

            if is_chainable:
                current_tokens.append(token)
            elif (
                is_connector
                and current_tokens
                and any(is_topical_japanese_noun(tok) for tok in current_tokens)
                and next_tok is not None
                and _is_compound_chain_token(next_tok, parsed[i + 2] if i + 2 < len(parsed) else None, False)
            ):
                current_tokens.append(token)
            else:
                _finalize_compound(current_tokens, compounds, max_length)
                current_tokens = []

        _finalize_compound(current_tokens, compounds, max_length)

    return compounds


def extract_compound_japanese_words(text: str, ja_tagger: Any, config: TagExtractionConfig) -> list[str]:
    """Extract compound words and important phrases from Japanese text.

    Args:
        text: Input Japanese text
        ja_tagger: Fugashi tagger instance
        config: Tag extraction configuration

    Returns:
        List of unique compound words
    """
    compound_words: list[str] = []

    # Phase 1: Extract compounds using consecutive noun chaining
    chained_compounds = extract_compound_nouns_fugashi(text, ja_tagger, max_length=config.max_tag_length)
    compound_words.extend(chained_compounds)

    # Phase 2: Regex patterns for mixed-script and special compounds
    for pattern in COMPOUND_PATTERNS:
        matches = re.findall(pattern, text)
        # Filter by max length to avoid sentence fragments
        compound_words.extend(m for m in matches if len(m) <= config.max_tag_length)

    # Phase 3: Use fugashi for proper noun sequence extraction
    if ja_tagger is None:
        raise RuntimeError("Japanese tagger not initialized")
    parsed = list(ja_tagger(text))
    i = 0
    while i < len(parsed):
        if parsed[i].feature.pos1 in config.japanese_pos_tags:
            # Check if it's a proper noun or organization
            if parsed[i].feature.pos2 in ["固有名詞", "組織", "人名", "地域"]:
                compound = parsed[i].surface
                j = i + 1

                # Look for connected proper nouns
                while j < len(parsed):
                    if parsed[j].feature.pos1 in config.japanese_pos_tags:
                        if parsed[j].feature.pos2 in [
                            "固有名詞",
                            "組織",
                            "人名",
                            "地域",
                        ]:
                            compound += parsed[j].surface
                            j += 1
                        else:
                            break
                    elif parsed[j].surface in ["・", "＝", "－"]:
                        # Include connectors in proper nouns
                        if j + 1 < len(parsed) and parsed[j + 1].feature.pos1 in config.japanese_pos_tags:
                            compound += parsed[j].surface + parsed[j + 1].surface
                            j += 2
                        else:
                            break
                    else:
                        break

                if 3 <= len(compound) <= config.max_tag_length:  # Length bounds for compound words
                    compound_words.append(compound)
                i = j
            else:
                i += 1
        else:
            i += 1

    # Deduplicate while preserving order
    seen: set[str] = set()
    unique_compounds: list[str] = []
    for word in compound_words:
        word_normalized = word.strip()
        if word_normalized not in seen and _shared_is_valid_japanese_tag(
            word_normalized, max_length=config.max_tag_length
        ):
            seen.add(word_normalized)
            unique_compounds.append(word_normalized)

    return unique_compounds


def make_japanese_analyzer(ja_tagger: Any):
    """Create a custom analyzer for CountVectorizer that uses Fugashi.

    The default token_pattern in CountVectorizer uses word boundaries (\\b)
    which don't work for Japanese text (no spaces between words). This
    function returns a callable that uses Fugashi to properly tokenize
    Japanese text, extracting nouns and English words.

    Args:
        ja_tagger: Fugashi tagger instance

    Returns:
        A callable that takes a string and returns a list of tokens.
    """
    tagger = ja_tagger

    def analyzer(text: str) -> list[str]:
        if tagger is None:
            # Fallback to simple split if tagger not available
            return text.split()

        tokens = []
        for word in tagger(text):
            # Exclude single-token numerals, counters, and temporal/adverbial nouns
            # (数詞, 助動詞語幹, 副詞可能, 助数詞可能) from the KeyBERT vectorizer vocabulary.
            if is_japanese_noun(word):
                if is_topical_japanese_noun(word):
                    tokens.append(word.surface)
            # Also preserve English words (ASCII alphabetic)
            elif word.surface.isascii() and word.surface.isalpha():
                tokens.append(word.surface)
        return tokens

    return analyzer


def score_japanese_candidates_with_keybert(
    text: str,
    candidates: list[str],
    freq_counter: Counter[str],
    keybert: Any,
    ja_tagger: Any,
    config: TagExtractionConfig,
    ja_stopwords: set[str],
) -> tuple[list[str], dict[str, float]]:
    """
    Score Japanese keyword candidates using KeyBERT semantic similarity.

    Passing a custom ``vectorizer`` causes KeyBERT to ignore ``candidates`` and
    score only the unigram tokens emitted by ``make_japanese_analyzer``, so
    multi-morpheme compound candidates are not reachable on this semantic path.
    KeyBERT output is filtered against ``candidates``, ``is_valid_japanese_tag``,
    and ``ja_stopwords`` before combining semantic and frequency scores.

    Args:
        text: Original document text
        candidates: List of pre-validated candidate keywords from morphological analysis
        freq_counter: Frequency counts for each candidate
        keybert: KeyBERT instance
        ja_tagger: Fugashi tagger instance
        config: Tag extraction configuration
        ja_stopwords: Japanese stopword set

    Returns:
        Tuple of (tag_list, tag_confidences_dict)
    """
    if keybert is None:
        raise RuntimeError("KeyBERT not initialized")

    # CRITICAL FIX (ADR-176, Phase 2): Use custom CountVectorizer with:
    # 1. lowercase=False - Preserve case to match uppercase candidates (GitHub, AWS, etc.)
    # 2. analyzer=make_japanese_analyzer() - Use Fugashi for Japanese tokenization
    #    (the default token_pattern uses word boundaries \b which don't work for Japanese)
    from sklearn.feature_extraction.text import CountVectorizer

    vectorizer = CountVectorizer(
        analyzer=make_japanese_analyzer(ja_tagger),
        lowercase=False,  # Preserve case to match uppercase candidates
    )

    keywords = keybert.extract_keywords(
        text,
        candidates=candidates,
        top_n=max(len(candidates), config.top_keywords * 2),
        use_mmr=config.use_mmr,
        diversity=config.japanese_mmr_diversity,
        vectorizer=vectorizer,
    )

    # Build result with combined scores after validating against candidate set and stopwords
    candidate_set = set(candidates)
    result: list[str] = []
    tag_confidences: dict[str, float] = {}

    # Get max frequency for normalization
    max_freq = max(freq_counter.values()) if freq_counter else 1

    for keyword, semantic_score in keywords:
        if (
            keyword in result
            or keyword not in candidate_set
            or keyword in ja_stopwords
            or not _shared_is_valid_japanese_tag(keyword, max_length=config.max_tag_length)
        ):
            continue

        # Combine semantic score (0-1) with normalized frequency
        freq = freq_counter.get(keyword, 1)
        freq_score = freq / max_freq

        # Weighted combination: 60% semantic, 40% frequency
        combined_score = (0.6 * semantic_score) + (0.4 * freq_score)

        result.append(keyword)
        tag_confidences[keyword] = round(combined_score, 3)

        if len(result) >= config.top_keywords:
            break

    logger.debug(
        "Japanese KeyBERT scoring completed",
        candidates_count=len(candidates),
        result_count=len(result),
    )

    return result, tag_confidences


def score_candidates_by_frequency(
    candidates: list[str], freq_counter: Counter[str], top_keywords: int
) -> tuple[list[str], dict[str, float]]:
    """
    Score candidates using frequency-based ranking.

    This is the fallback method when semantic scoring is unavailable.

    Args:
        candidates: List of candidate keywords
        freq_counter: Frequency counts for each candidate
        top_keywords: Number of top keywords to return

    Returns:
        Tuple of (tag_list, tag_confidences_dict)
    """
    # Sort by frequency
    sorted_candidates = sorted(candidates, key=lambda x: -freq_counter.get(x, 0))

    result = sorted_candidates[:top_keywords]
    max_freq = max(freq_counter.values()) if freq_counter else 1

    tag_confidences = {tag: round(freq_counter.get(tag, 1) / max_freq, 3) for tag in result}

    return result, tag_confidences


def fallback_japanese(
    text: str,
    ja_tagger: Any,
    ja_stopwords: set[str],
    config: TagExtractionConfig,
) -> list[str]:
    """Frequency-only Japanese fallback (no KeyBERT).

    Distinct from ``extract_keywords_japanese`` so a primary KeyBERT/semantic
    failure does not re-run the same failing path.
    """
    if ja_tagger is None:
        return []

    compounds = extract_compound_japanese_words(text, ja_tagger, config)
    term_freq = Counter(compounds)

    single_nouns: list[str] = []
    for word in ja_tagger(text):
        if (
            is_topical_japanese_noun(word, config.japanese_pos_tags)
            and 2 <= len(word.surface) <= 10
            and word.surface not in ja_stopwords
        ):
            single_nouns.append(word.surface)
    single_freq = Counter(single_nouns)

    combined_freq: Counter[str] = Counter()
    for term, freq in term_freq.items():
        combined_freq[term] = freq * 2
    for term, freq in single_freq.items():
        if term not in combined_freq:
            combined_freq[term] = freq

    candidates = [
        term
        for term, freq in combined_freq.most_common(config.top_keywords * 5)
        if (freq >= 2 or len(term) >= 4) and _shared_is_valid_japanese_tag(term, max_length=config.max_tag_length)
    ][: config.top_keywords * 3]

    if not candidates:
        return []

    keywords, _ = score_candidates_by_frequency(candidates, combined_freq, config.top_keywords)
    return keywords


def extract_keywords_japanese(
    text: str,
    ja_tagger: Any,
    keybert: Any,
    ja_stopwords: set[str],
    config: TagExtractionConfig,
) -> tuple[list[str], dict[str, float]]:
    """Extract keywords specifically for Japanese text.

    Combines morphological analysis with optional semantic scoring:
    1. Extract compound nouns and single nouns using Fugashi
    2. Score candidates using frequency
    3. Optionally re-score using KeyBERT semantic similarity

    Args:
        text: Input Japanese text
        ja_tagger: Fugashi tagger instance
        keybert: KeyBERT instance
        ja_stopwords: Set of Japanese stopwords
        config: Tag extraction configuration

    Returns:
        Tuple of (tag_list, tag_confidences_dict)
    """
    # Extract compound words and important terms
    compounds = extract_compound_japanese_words(text, ja_tagger, config)

    # Count frequencies
    term_freq = Counter(compounds)

    # Also extract single important nouns
    if ja_tagger is None:
        raise RuntimeError("Japanese tagger not initialized")
    single_nouns = []
    for word in ja_tagger(text):
        if (
            is_topical_japanese_noun(word, config.japanese_pos_tags)
            and 2 <= len(word.surface) <= 10
            and word.surface not in ja_stopwords
        ):
            single_nouns.append(word.surface)

    # Add single noun frequencies
    single_freq = Counter(single_nouns)

    # Combine frequencies, giving priority to compounds
    combined_freq: Counter[str] = Counter()
    for term, freq in term_freq.items():
        combined_freq[term] = freq * 2  # Boost compound words

    for term, freq in single_freq.items():
        if term not in combined_freq:
            combined_freq[term] = freq

    # Filter candidates by frequency, length, and quality validation
    # Get more candidates initially to account for filtering
    candidates = [
        term
        for term, freq in combined_freq.most_common(config.top_keywords * 5)
        if (freq >= 2 or len(term) >= 4) and _shared_is_valid_japanese_tag(term, max_length=config.max_tag_length)
    ][: config.top_keywords * 3]  # Limit after filtering

    if not candidates:
        return [], {}

    # Try semantic scoring with KeyBERT
    if config.use_japanese_semantic and keybert is not None and len(candidates) >= 2:
        return score_japanese_candidates_with_keybert(
            text, candidates, combined_freq, keybert, ja_tagger, config, ja_stopwords
        )

    # Fallback: frequency-based scoring
    return score_candidates_by_frequency(candidates, combined_freq, config.top_keywords)
