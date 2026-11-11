"""English keyword extraction using KeyBERT and NLTK tokenization."""

import re
from collections import Counter
from typing import Any, cast

import nltk
import structlog

from .config import TagExtractionConfig
from .tag_validator import is_valid_english_tag

logger = structlog.get_logger(__name__)

# Canonical 318-word Glasgow IR stopword set distributed as
# sklearn.feature_extraction.text.ENGLISH_STOP_WORDS (BSD-3-Clause; primary
# source: https://ir.dcs.gla.ac.uk/resources/linguistic_utils/stop_words).
# Loaded from scikit-learn when the `ml` dependency group is installed, with an
# identical constant for minimal `dev` unit-test environments without sklearn.
_GLASGOW_IR_ENGLISH_STOP_WORDS: frozenset[str] = frozenset(
    """
    a about above across after afterwards again against all almost alone along
    already also although always am among amongst amoungst amount an and another
    any anyhow anyone anything anyway anywhere are around as at back be became
    because become becomes becoming been before beforehand behind being below
    beside besides between beyond bill both bottom but by call can cannot cant
    co con could couldnt cry de describe detail do done down due during each eg
    eight either eleven else elsewhere empty enough etc even ever every everyone
    everything everywhere except few fifteen fifty fill find fire first five for
    former formerly forty found four from front full further get give go had has
    hasnt have he hence her here hereafter hereby herein hereof hereto herewith
    hers herself him himself his how however hundred i ie if in inc indeed
    interest into is it its itself keep last latter latterly least less ltd made
    many may me meanwhile might mill mine more moreover most mostly move much
    must my myself name namely neither never nevertheless next nine no nobody
    none noone nor not nothing now nowhere of off often on once one only onto or
    other others otherwise our ours ourselves out over own part per perhaps
    please put rather re same see seem seemed seeming seems serious several she
    should show side since sincere six sixty so some somehow someone something
    sometime sometimes somewhere still such system take ten than that the their
    them themselves then thence there thereafter thereby therefore therein
    thereupon these they thick thin third this those though three through
    throughout thru thus to together too top toward towards twelve twenty two un
    under until up upon us very via was we well were what whatever when whence
    whenever where whereafter whereas whereby wherein whereupon wherever whether
    which while whither who whoever whole whom whose why will with within
    without would yet you your yours yourself yourselves
    """.split()
)

try:
    from sklearn.feature_extraction.text import ENGLISH_STOP_WORDS as _SKLEARN_STOP_WORDS

    ENGLISH_STOP_WORDS: frozenset[str] = (
        _SKLEARN_STOP_WORDS if isinstance(_SKLEARN_STOP_WORDS, frozenset) else _GLASGOW_IR_ENGLISH_STOP_WORDS
    )
except ImportError:
    ENGLISH_STOP_WORDS = _GLASGOW_IR_ENGLISH_STOP_WORDS

_ENGLISH_TOKEN_PATTERN = re.compile(r"[A-Za-z0-9]+(?:[+#._/-][A-Za-z0-9+#]+)*|[A-Za-z][A-Za-z0-9]*(?:\+\+|#)")


def _is_allowed_short_english_term(word: str, source_text: str) -> bool:
    """Allow 2-character single words only when they contain a digit or appear all-caps in source text."""
    if len(word) != 2:
        return True
    if any(ch.isdigit() for ch in word):
        return True
    pattern = rf"(?<![A-Za-z0-9]){re.escape(word.upper())}(?![A-Za-z0-9])"
    return bool(re.search(pattern, source_text))


def _is_duplicate_english_keyword(candidate: str, existing: str) -> bool:
    """Check phrase-token or inflection duplication without merging distinct prefix-sharing entities."""
    if candidate == existing:
        return True
    shorter, longer = (candidate, existing) if len(candidate) <= len(existing) else (existing, candidate)
    if " " in longer:
        shorter_words = shorter.split()
        longer_words = longer.split()
        width = len(shorter_words)
        if any(longer_words[i : i + width] == shorter_words for i in range(len(longer_words) - width + 1)):
            return True
    if len(shorter) >= 3 and longer in (
        f"{shorter}s",
        f"{shorter}es",
        f"{shorter}ed",
        f"{shorter}ing",
    ):
        return True
    return False


def extract_keywords_english(
    text: str,
    keybert: Any,
    config: TagExtractionConfig,
    en_stopwords: set[str],
) -> tuple[list[str], dict[str, float]]:
    """Extract keywords specifically for English text using KeyBERT.

    Args:
        text: Input English text
        keybert: KeyBERT instance
        config: Tag extraction configuration
        en_stopwords: English stopword set

    Returns:
        Tuple of (tag_list, tag_confidences_dict)
    """
    if keybert is None:
        raise RuntimeError("KeyBERT not initialized")

    active_stopwords = set(ENGLISH_STOP_WORDS) | {w.lower() for w in en_stopwords}
    stop_words_arg = sorted(active_stopwords)

    # Single call covering both single words and phrases (1-3 ngrams)
    all_raw = keybert.extract_keywords(
        text,
        keyphrase_ngram_range=(1, 3),
        stop_words=stop_words_arg,
        top_n=config.top_keywords * 3,
        use_mmr=True,
        diversity=0.4,
    )

    # Separate into phrases and single words, then merge
    all_keywords = []
    seen_words: set[str] = set()
    min_single_length = max(2, config.min_token_length)
    proper_nouns = {
        match.group(1).lower()
        for match in re.finditer(r"(?<=[a-z,;:]\s)([A-Z][A-Za-z0-9+#._-]{1,})(?![A-Za-z0-9])", text)
    }

    # Process phrases first to identify important compound terms
    for phrase_tuple in cast(list[tuple[str, float]], all_raw):
        phrase = phrase_tuple[0].strip().lower()
        score = phrase_tuple[1]
        words = phrase.split()
        if len(words) >= 2:
            # Only keep phrases with high scores or specific patterns
            if (
                score >= config.min_score_threshold * 1.5
                and any(w[0].isupper() for w in phrase_tuple[0].strip().split() if w)
                and is_valid_english_tag(phrase, stopwords=active_stopwords, min_length=3)
            ):
                all_keywords.append((phrase, score))
                # Mark individual words as seen to avoid duplication
                seen_words.update(w for w in words if w not in active_stopwords)

    # Then add important single words not already in phrases
    for word_tuple in cast(list[tuple[str, float]], all_raw):
        word = word_tuple[0].strip().lower()
        score = word_tuple[1]
        words = word.split()
        if (
            len(words) == 1
            and score >= config.min_score_threshold
            and word not in seen_words
            and is_valid_english_tag(word, stopwords=active_stopwords, min_length=min_single_length)
            and _is_allowed_short_english_term(word, text)
        ):
            adjusted_score = min(1.0, score + 0.05) if word in proper_nouns else score
            all_keywords.append((word, adjusted_score))
            seen_words.add(word)

    # Sort by score and filter
    all_keywords.sort(key=lambda x: x[1], reverse=True)

    # Final filtering and cleaning
    result: list[str] = []
    tag_confidences: dict[str, float] = {}
    seen_final: set[str] = set()

    for keyword, score in all_keywords:
        # Clean and check for duplicates
        keyword_clean = keyword.strip()
        keyword_lower = keyword_clean.lower()

        # Skip if we've seen this or a very similar variant
        if keyword_lower not in seen_final:
            is_duplicate = any(_is_duplicate_english_keyword(keyword_lower, seen) for seen in seen_final)

            if not is_duplicate:
                result.append(keyword_clean)
                # Normalize score to 0.0-1.0 range and store
                normalized_score = min(max(score, 0.0), 1.0)
                tag_confidences[keyword_clean] = round(normalized_score, 3)
                seen_final.add(keyword_lower)

                if len(result) >= config.top_keywords:
                    break

    return result, tag_confidences


def tokenize_english(
    text: str,
    en_stopwords: set[str],
    config: TagExtractionConfig,
) -> list[str]:
    """Tokenize English text using NLTK.

    Args:
        text: Input English text
        en_stopwords: Set of English stopwords
        config: Tag extraction configuration

    Returns:
        List of filtered and normalized tokens
    """
    active_stopwords = set(ENGLISH_STOP_WORDS) | {w.lower() for w in en_stopwords}
    raw_tokens = nltk.word_tokenize(text)
    merged_tokens: list[str] = []
    idx = 0
    while idx < len(raw_tokens):
        tok = raw_tokens[idx]
        if idx + 1 < len(raw_tokens) and re.fullmatch(r"[A-Za-z]", tok) and raw_tokens[idx + 1] in ("++", "#"):
            merged_tokens.append(f"{tok}{raw_tokens[idx + 1]}")
            idx += 2
        else:
            merged_tokens.append(tok)
            idx += 1

    result = []
    min_length = max(2, config.min_token_length)

    for token in merged_tokens:
        if not _ENGLISH_TOKEN_PATTERN.fullmatch(token):
            continue
        is_short_tech_token = len(token) == 2 and (token.isupper() or any(ch.isdigit() or ch in "+#" for ch in token))
        if len(token) > min_length or (is_short_tech_token and config.min_token_length <= 2):
            normalized = _normalize_text_english(token)
            effective_min = 2 if is_short_tech_token else min_length
            if is_valid_english_tag(normalized, stopwords=active_stopwords, min_length=effective_min):
                result.append(normalized)

    return result


def get_candidate_tokens(
    text: str,
    en_stopwords: set[str],
    config: TagExtractionConfig,
) -> list[str]:
    """Get candidate tokens for fallback extraction (primarily English text).

    This helper exists mainly for clarity and testability and currently
    delegates to the English tokenizer.

    Args:
        text: Input English text
        en_stopwords: Set of English stopwords
        config: Tag extraction configuration

    Returns:
        List of candidate tokens
    """
    return tokenize_english(text, en_stopwords, config)


def fallback_english(
    text: str,
    en_stopwords: set[str],
    config: TagExtractionConfig,
) -> list[str]:
    """Fallback extraction for English text using tokenization and frequency.

    Args:
        text: Input English text
        en_stopwords: Set of English stopwords
        config: Tag extraction configuration

    Returns:
        List of extracted keywords
    """
    tokens = get_candidate_tokens(text, en_stopwords, config)
    if tokens:
        token_freq = Counter(tokens)
        return [term for term, _ in token_freq.most_common(config.top_keywords)]
    return []


def _normalize_text_english(text: str) -> str:
    """Normalize English text to lowercase."""
    return text.lower()
