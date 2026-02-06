"""Japanese keyword extraction using Fugashi morphological analysis and KeyBERT semantic scoring."""

import re
from collections import Counter
from typing import Any

import structlog

from .config import TagExtractionConfig
from .tag_validator import is_valid_japanese_tag as _shared_is_valid_japanese_tag

logger = structlog.get_logger(__name__)

# POS tags that should be included in compound nouns
NOUN_POS_TAGS = {
    "名詞",
    "名詞-普通名詞-一般",
    "名詞-普通名詞-サ変可能",
    "名詞-普通名詞-形状詞可能",
    "名詞-固有名詞-一般",
    "名詞-固有名詞-人名",
    "名詞-固有名詞-組織",
    "名詞-固有名詞-地域",
    "名詞-数詞",
    "名詞-接尾辞-名詞的",
}

# Tags that can connect nouns (e.g., の between nouns)
CONNECTOR_SURFACES = {"の", "・", "＝", "－", "-"}

# Regex patterns for mixed-script and special compounds
COMPOUND_PATTERNS = [
    # Tech terms with mixed scripts
    r"[A-Za-z][A-Za-z0-9]*[ァ-ヶー]+(?:[A-Za-z0-9]*)?",  # e.g., "GitHubリポジトリ"
    r"[ァ-ヶー]+[A-Za-z][A-Za-z0-9]*",  # e.g., "データセットID"
    r"[A-Z][A-Za-z0-9]*(?:\.[A-Za-z]+)?",  # CamelCase and dotted (e.g., "TensorFlow", "Next.js")
    r"[A-Z]{2,}(?:[a-z]+)?",  # Acronyms like "AWS", "API", "CEO"
    r"[一-龥]{2,}[ァ-ヶー]+",  # Kanji + Katakana compounds
    # Important proper nouns with titles
    r"[一-龥ァ-ヶー]{2,}(?:大統領|首相|総理|議員|知事|市長|社長|CEO)",
    r"[一-龥ァ-ヶー]{2,}(?:会社|企業|組織|団体|協会|連盟|大学|研究所)",
    # Tech-specific patterns
    r"[ァ-ヶー]{3,}(?:システム|サービス|プラットフォーム|フレームワーク|ライブラリ)",
    r"[ァ-ヶー]{2,}(?:アーキテクチャ|インフラ|ネットワーク|セキュリティ)",
    # Alphanumeric + Japanese
    r"[A-Za-z0-9]+[一-龥ァ-ヶー]{2,}",  # e.g., "IoT機器", "Web技術", "AI技術"
    r"[一-龥ァ-ヶー]{2,}[A-Za-z0-9]+",  # e.g., "機械学習API"
    r"\d+[A-Za-zァ-ヶー一-龥]+",  # Number + Word (e.g., "5G通信", "3Dプリンター")
]


def extract_compound_nouns_fugashi(text: str, ja_tagger: Any) -> list[str]:
    """
    Extract compound nouns by chaining consecutive noun tokens.

    Identifies sequences of consecutive nouns and joins them
    to form compound nouns, which are more meaningful as tags than
    individual morphemes.

    Args:
        text: Input Japanese text
        ja_tagger: Fugashi tagger instance

    Returns:
        List of compound nouns (2+ consecutive nouns joined)
    """
    if ja_tagger is None:
        raise RuntimeError("Japanese tagger not initialized")

    parsed = list(ja_tagger(text))
    compounds: list[str] = []
    current_compound: list[str] = []

    for i, token in enumerate(parsed):
        pos1 = token.feature.pos1
        surface = token.surface

        # Check if token is a noun
        is_noun = pos1 in NOUN_POS_TAGS or pos1.startswith("名詞")

        # Check if it's a connector that might join nouns
        is_connector = surface in CONNECTOR_SURFACES

        if is_noun:
            current_compound.append(surface)
        elif is_connector and current_compound:
            # Check if next token is also a noun
            if i + 1 < len(parsed):
                next_pos = parsed[i + 1].feature.pos1
                next_is_noun = next_pos in NOUN_POS_TAGS or next_pos.startswith("名詞")
                if next_is_noun:
                    current_compound.append(surface)
                    continue
            # Not followed by noun, finalize current compound
            if len(current_compound) >= 2:
                compound = "".join(current_compound)
                if 3 <= len(compound) <= 30:
                    compounds.append(compound)
            current_compound = []
        else:
            # Non-noun token, finalize current compound
            if len(current_compound) >= 2:
                compound = "".join(current_compound)
                if 3 <= len(compound) <= 30:
                    compounds.append(compound)
            current_compound = []

    # Handle remaining compound at end of text
    if len(current_compound) >= 2:
        compound = "".join(current_compound)
        if 3 <= len(compound) <= 30:
            compounds.append(compound)

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
    chained_compounds = extract_compound_nouns_fugashi(text, ja_tagger)
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
        if word_normalized not in seen and 2 <= len(word_normalized) <= 30:
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
            # Extract nouns (名詞) for matching candidates
            if word.feature.pos1.startswith("名詞"):
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
) -> tuple[list[str], dict[str, float]]:
    """
    Score Japanese keyword candidates using KeyBERT semantic similarity.

    Uses KeyBERT's candidates parameter to score pre-extracted
    noun phrases against the document embedding, combining semantic
    relevance with frequency information.

    Args:
        text: Original document text
        candidates: List of candidate keywords from morphological analysis
        freq_counter: Frequency counts for each candidate
        keybert: KeyBERT instance
        ja_tagger: Fugashi tagger instance
        config: Tag extraction configuration

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

    # Use KeyBERT with candidate list for semantic scoring
    keywords = keybert.extract_keywords(
        text,
        candidates=candidates,
        top_n=min(len(candidates), config.top_keywords * 2),
        use_mmr=config.use_mmr,
        diversity=config.japanese_mmr_diversity,
        vectorizer=vectorizer,
    )

    # Build result with combined scores
    result: list[str] = []
    tag_confidences: dict[str, float] = {}

    # Get max frequency for normalization
    max_freq = max(freq_counter.values()) if freq_counter else 1

    for keyword, semantic_score in keywords:
        if keyword in result:
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
            word.feature.pos1 in config.japanese_pos_tags
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
        try:
            return score_japanese_candidates_with_keybert(text, candidates, combined_freq, keybert, ja_tagger, config)
        except Exception as e:
            logger.warning("Japanese semantic scoring failed, falling back to frequency", error=str(e))

    # Fallback: frequency-based scoring
    return score_candidates_by_frequency(candidates, combined_freq, config.top_keywords)
