"""Japanese keyword extraction using Fugashi morphological analysis and KeyBERT semantic scoring."""

import re
from collections import Counter
from collections.abc import Callable, Sequence

import structlog

from .config import JapaneseTaggerLike, JapaneseTokenLike, KeyBERTLike, TagExtractionConfig
from .tag_validator import is_valid_japanese_tag as _shared_is_valid_japanese_tag

logger = structlog.get_logger(__name__)

_EXCLUDED_UNIDIC_POS2 = frozenset({"数詞", "助動詞語幹"})
_EXCLUDED_UNIDIC_POS3 = frozenset({"副詞可能", "助数詞可能"})
_PROPER_NOUN_POS2 = frozenset({"固有名詞", "組織", "人名", "地域"})
_MIN_COMPOUND_TOKEN_COUNT = 2
_MIN_COMPOUND_CHAR_LEN = 3
_MIN_SINGLE_NOUN_LEN = 2
_MAX_SINGLE_NOUN_LEN = 10
_MIN_FALLBACK_FREQ = 2
_MIN_FALLBACK_LEN = 4
_MIN_KEYBERT_CANDIDATES = 2


class _JapaneseTaggerNotInitializedError(RuntimeError):
    def __init__(self) -> None:
        super().__init__("Japanese tagger not initialized")


class _KeyBERTNotInitializedError(RuntimeError):
    def __init__(self) -> None:
        super().__init__("KeyBERT not initialized")


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
    token: object,
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
    token: object,
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
    return not (isinstance(pos3, str) and pos3 in _EXCLUDED_UNIDIC_POS3)


def _is_compound_chain_token(token: object, next_token: object | None, has_preceding: bool) -> bool:
    """Check if token belongs in a contiguous Japanese noun compound chain."""
    if is_japanese_noun(token):
        return True
    feature = getattr(token, "feature", None)
    pos1 = getattr(feature, "pos1", "")
    pos2 = getattr(feature, "pos2", "")
    if pos1 == "接頭辞" and getattr(token, "surface", "") == "第":
        return next_token is not None and is_japanese_noun(next_token)
    return bool(has_preceding and pos1 == "接尾辞" and pos2 == "名詞的")


def _finalize_compound(tokens: Sequence[JapaneseTokenLike], compounds: list[str], max_length: int) -> None:
    """Append joined compound if it has >=2 tokens, contains a topical noun, and passes tag validation."""
    if len(tokens) < _MIN_COMPOUND_TOKEN_COUNT:
        return
    if not any(is_topical_japanese_noun(tok) for tok in tokens):
        return
    compound = "".join(tok.surface for tok in tokens)
    if _MIN_COMPOUND_CHAR_LEN <= len(compound) <= max_length and _shared_is_valid_japanese_tag(
        compound, max_length=max_length
    ):
        compounds.append(compound)


def extract_compound_nouns_fugashi(
    text: str,
    ja_tagger: JapaneseTaggerLike | None,
    max_length: int = 15,
) -> list[str]:
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
        raise _JapaneseTaggerNotInitializedError

    # Split on line breaks before invoking MeCab because MeCab discards whitespace
    # tokens and would otherwise glue the last noun of a title/paragraph onto the
    # first noun of the next line.
    segments = [seg for seg in re.split(r"[\r\n]+", text) if seg] or [text]
    compounds: list[str] = []

    for segment in segments:
        parsed = list(ja_tagger(segment))
        current_tokens: list[JapaneseTokenLike] = []

        for i, token in enumerate(parsed):
            surface = token.surface
            next_tok = parsed[i + 1] if i + 1 < len(parsed) else None
            is_chainable = _is_compound_chain_token(token, next_tok, bool(current_tokens))
            is_connector = surface in CONNECTOR_SURFACES

            if is_chainable or (
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


def _extract_proper_noun_compounds(
    text: str,
    ja_tagger: JapaneseTaggerLike,
    config: TagExtractionConfig,
) -> list[str]:
    parsed = list(ja_tagger(text))
    compounds: list[str] = []
    i = 0
    while i < len(parsed):
        pos1 = getattr(parsed[i].feature, "pos1", "")
        pos2 = getattr(parsed[i].feature, "pos2", "")
        if pos1 not in config.japanese_pos_tags or pos2 not in _PROPER_NOUN_POS2:
            i += 1
            continue

        compound = parsed[i].surface
        j = i + 1
        while j < len(parsed):
            tok = parsed[j]
            tok_pos1 = getattr(tok.feature, "pos1", "")
            tok_pos2 = getattr(tok.feature, "pos2", "")
            if tok_pos1 in config.japanese_pos_tags:
                if tok_pos2 in _PROPER_NOUN_POS2:
                    compound += tok.surface
                    j += 1
                else:
                    break
            elif (
                tok.surface in {"・", "＝", "－"}
                and j + 1 < len(parsed)
                and getattr(parsed[j + 1].feature, "pos1", "") in config.japanese_pos_tags
            ):
                compound += tok.surface + parsed[j + 1].surface
                j += 2
            else:
                break

        if _MIN_COMPOUND_CHAR_LEN <= len(compound) <= config.max_tag_length:
            compounds.append(compound)
        i = j
    return compounds


def extract_compound_japanese_words(
    text: str,
    ja_tagger: JapaneseTaggerLike | None,
    config: TagExtractionConfig,
) -> list[str]:
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
        raise _JapaneseTaggerNotInitializedError
    compound_words.extend(_extract_proper_noun_compounds(text, ja_tagger, config))

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


def make_japanese_analyzer(ja_tagger: JapaneseTaggerLike | None) -> Callable[[str], list[str]]:
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


def score_japanese_candidates_with_keybert(  # noqa: PLR0913 - language-specific scoring pipeline inputs
    text: str,
    candidates: list[str],
    freq_counter: Counter[str],
    keybert: KeyBERTLike | None,
    ja_tagger: JapaneseTaggerLike | None,
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
        raise _KeyBERTNotInitializedError

    # CRITICAL FIX (ADR-176, Phase 2): Use custom CountVectorizer with:
    # 1. lowercase=False - Preserve case to match uppercase candidates (GitHub, AWS, etc.)
    # 2. analyzer=make_japanese_analyzer() - Use Fugashi for Japanese tokenization
    #    (the default token_pattern uses word boundaries \b which don't work for Japanese)
    from sklearn.feature_extraction.text import (  # noqa: PLC0415 - optional ml dependency loaded lazily
        CountVectorizer,
    )

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


def _build_japanese_candidates(
    text: str,
    ja_tagger: JapaneseTaggerLike,
    ja_stopwords: set[str],
    config: TagExtractionConfig,
) -> tuple[list[str], Counter[str]]:
    compounds = extract_compound_japanese_words(text, ja_tagger, config)
    term_freq = Counter(compounds)
    single_nouns = [
        word.surface
        for word in ja_tagger(text)
        if is_topical_japanese_noun(word, config.japanese_pos_tags)
        and _MIN_SINGLE_NOUN_LEN <= len(word.surface) <= _MAX_SINGLE_NOUN_LEN
        and word.surface not in ja_stopwords
    ]
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
        if (freq >= _MIN_FALLBACK_FREQ or len(term) >= _MIN_FALLBACK_LEN)
        and _shared_is_valid_japanese_tag(term, max_length=config.max_tag_length)
    ][: config.top_keywords * 3]
    return candidates, combined_freq


def fallback_japanese(
    text: str,
    ja_tagger: JapaneseTaggerLike | None,
    ja_stopwords: set[str],
    config: TagExtractionConfig,
) -> list[str]:
    """Frequency-only Japanese fallback (no KeyBERT).

    Distinct from ``extract_keywords_japanese`` so a primary KeyBERT/semantic
    failure does not re-run the same failing path.
    """
    if ja_tagger is None:
        return []

    candidates, combined_freq = _build_japanese_candidates(text, ja_tagger, ja_stopwords, config)
    if not candidates:
        return []

    keywords, _ = score_candidates_by_frequency(candidates, combined_freq, config.top_keywords)
    return keywords


def extract_keywords_japanese(
    text: str,
    ja_tagger: JapaneseTaggerLike | None,
    keybert: KeyBERTLike | None,
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
    if ja_tagger is None:
        raise _JapaneseTaggerNotInitializedError

    candidates, combined_freq = _build_japanese_candidates(text, ja_tagger, ja_stopwords, config)
    if not candidates:
        return [], {}

    # Try semantic scoring with KeyBERT
    if config.use_japanese_semantic and keybert is not None and len(candidates) >= _MIN_KEYBERT_CANDIDATES:
        return score_japanese_candidates_with_keybert(
            text, candidates, combined_freq, keybert, ja_tagger, config, ja_stopwords
        )

    # Fallback: frequency-based scoring
    return score_candidates_by_frequency(candidates, combined_freq, config.top_keywords)
