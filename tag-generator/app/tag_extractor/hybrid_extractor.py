"""
Hybrid tag extractor combining multiple extraction methods.

This extractor combines:
1. GiNZA (syntax-based): Noun phrase extraction, NER
2. Fugashi (frequency-based): Morphological analysis with frequency ranking
3. KeyBERT (semantic): Embedding-based keyword extraction

The hybrid approach addresses the limitations of each individual method:
- Fugashi alone misses compound nouns and lacks semantic understanding
- GiNZA provides better noun phrases but may miss domain-specific terms
- KeyBERT provides semantic relevance but needs good candidates for Japanese

Usage:
    from tag_extractor.hybrid_extractor import HybridExtractor

    extractor = HybridExtractor()
    tags = extractor.extract_tags(title, content)
"""

import time
from collections import Counter
from dataclasses import dataclass, field
from typing import Any

import structlog

from .ginza_extractor import GinzaConfig, GinzaExtractor, get_ginza_extractor
from .tag_validator import clean_noun_phrase, is_valid_japanese_tag

logger = structlog.get_logger(__name__)


@dataclass
class HybridConfig:
    """Configuration for hybrid extractor."""

    # Maximum number of tags to return
    top_k: int = 10
    # Minimum score threshold for inclusion
    min_score: float = 0.1
    # Whether to use GiNZA (requires ml-extended dependencies)
    use_ginza: bool = True
    # Whether to use KeyBERT semantic scoring
    use_keybert_scoring: bool = True
    # Weight for GiNZA-sourced candidates
    ginza_weight: float = 1.2
    # Weight for Fugashi-sourced candidates
    fugashi_weight: float = 1.0
    # Weight for frequency score
    frequency_weight: float = 0.4
    # Weight for position score (earlier in text = higher)
    position_weight: float = 0.2
    # Weight for semantic score from KeyBERT
    semantic_weight: float = 0.4
    # Minimum phrase length
    min_phrase_length: int = 2
    # Maximum phrase length (reduced from 30 to 15 to prevent sentence fragments)
    max_phrase_length: int = 15
    # GiNZA-specific configuration
    ginza_config: GinzaConfig = field(default_factory=GinzaConfig)


@dataclass
class CandidateTag:
    """A candidate tag with associated scores."""

    text: str
    source: str  # 'ginza', 'fugashi', 'keybert'
    frequency: int = 1
    position: int = 0  # Position in original text
    semantic_score: float = 0.0
    combined_score: float = 0.0


@dataclass
class HybridExtractionResult:
    """Result of hybrid extraction."""

    tags: list[str]
    tag_scores: dict[str, float]
    inference_ms: float
    metadata: dict[str, Any] = field(default_factory=dict)


class HybridExtractor:
    """
    Hybrid tag extractor combining GiNZA, Fugashi, and KeyBERT.

    This extractor provides better tag quality for Japanese text by:
    1. Using GiNZA for accurate noun phrase and NER extraction
    2. Using Fugashi for frequency-based term extraction
    3. Optionally using KeyBERT for semantic relevance scoring
    """

    def __init__(self, config: HybridConfig | None = None) -> None:
        """Initialize the hybrid extractor."""
        self.config = config or HybridConfig()
        self._ginza: GinzaExtractor | None = None
        self._fugashi_tagger: Any = None
        self._keybert: Any = None
        self._embedder: Any = None
        self._stopwords: set[str] = set()
        self._models_loaded = False

        logger.info("HybridExtractor initialized", config=self.config)

    def _lazy_load_models(self) -> None:
        """Lazily load all required models."""
        if self._models_loaded:
            return

        # Load GiNZA if enabled
        if self.config.use_ginza:
            self._ginza = get_ginza_extractor(self.config.ginza_config)
            if not self._ginza.is_available():
                logger.warning("GiNZA not available, falling back to Fugashi-only mode")
                self._ginza = None

        # Load Fugashi tagger
        try:
            from fugashi import Tagger

            self._fugashi_tagger = Tagger()
            logger.info("Fugashi tagger loaded")
        except ImportError:
            logger.warning("Fugashi not available")

        # Load KeyBERT if enabled
        if self.config.use_keybert_scoring:
            try:
                from keybert import KeyBERT
                from sentence_transformers import SentenceTransformer

                model_name = "paraphrase-multilingual-MiniLM-L12-v2"
                self._embedder = SentenceTransformer(model_name, device="cpu")
                self._keybert = KeyBERT(self._embedder)
                logger.info("KeyBERT loaded for semantic scoring", model_name=model_name)
            except ImportError:
                logger.warning("KeyBERT/SentenceTransformer not available")

        # Load stopwords
        self._load_stopwords()

        self._models_loaded = True

    def _load_stopwords(self) -> None:
        """Load Japanese stopwords."""
        import os

        current_dir = os.path.dirname(__file__)
        stopwords_path = os.path.join(current_dir, "stopwords_ja.txt")

        try:
            with open(stopwords_path, encoding="utf-8") as f:
                for line in f:
                    line = line.strip()
                    if line and not line.startswith("#"):
                        self._stopwords.add(line)
            logger.debug("Loaded stopwords", count=len(self._stopwords))
        except FileNotFoundError:
            logger.warning("Stopwords file not found", path=stopwords_path)

    def _extract_candidates_ginza(self, text: str) -> list[CandidateTag]:
        """Extract candidate tags using GiNZA."""
        if self._ginza is None:
            return []

        candidates: list[CandidateTag] = []
        seen: set[str] = set()

        # Get noun phrases
        noun_phrases = self._ginza.extract_noun_phrases(text)
        for i, phrase in enumerate(noun_phrases):
            # Clean the phrase to remove trailing particles/verbs
            cleaned = clean_noun_phrase(phrase)

            # Validate the cleaned phrase
            if not is_valid_japanese_tag(cleaned, max_length=self.config.max_phrase_length):
                continue

            if cleaned.lower() not in seen and cleaned not in self._stopwords:
                seen.add(cleaned.lower())
                candidates.append(
                    CandidateTag(
                        text=cleaned,
                        source="ginza",
                        frequency=1,
                        position=i,
                    )
                )

        # Get named entities (higher priority)
        entities = self._ginza.extract_named_entities(text)
        for entity in entities:
            # Clean the entity
            cleaned = clean_noun_phrase(entity)

            # Validate the cleaned entity
            if not is_valid_japanese_tag(cleaned, max_length=self.config.max_phrase_length):
                continue

            if cleaned.lower() not in seen and cleaned not in self._stopwords:
                seen.add(cleaned.lower())
                # Named entities get a frequency boost
                candidates.append(
                    CandidateTag(
                        text=cleaned,
                        source="ginza",
                        frequency=2,
                        position=0,  # Entities are prioritized
                    )
                )

        return candidates

    def _is_valid_tag(self, tag: str) -> bool:
        """Filter out grammatically invalid or low-quality tags.

        This method delegates to the shared tag validator for consistent
        filtering across all extractors.

        Args:
            tag: The candidate tag to validate

        Returns:
            True if the tag is valid, False otherwise
        """
        return is_valid_japanese_tag(tag, max_length=self.config.max_phrase_length)

    def _extract_candidates_fugashi(self, text: str) -> list[CandidateTag]:
        """Extract candidate tags using Fugashi morphological analysis."""
        if self._fugashi_tagger is None:
            return []

        candidates: list[CandidateTag] = []
        term_freq: Counter[str] = Counter()
        term_positions: dict[str, int] = {}

        # Noun POS tags to extract
        noun_pos = {
            "名詞",
            "名詞-普通名詞-一般",
            "名詞-普通名詞-サ変可能",
            "名詞-固有名詞-一般",
            "名詞-固有名詞-人名",
            "名詞-固有名詞-組織",
            "名詞-固有名詞-地域",
        }

        # Extract compound nouns by chaining consecutive nouns
        parsed = list(self._fugashi_tagger(text))
        current_compound: list[str] = []
        position = 0

        for token in parsed:
            pos = token.feature.pos1
            surface = token.surface

            is_noun = pos in noun_pos or pos.startswith("名詞")

            if is_noun and surface not in self._stopwords:
                current_compound.append(surface)
            else:
                if len(current_compound) >= 2:
                    compound = "".join(current_compound)
                    if self.config.min_phrase_length <= len(compound) <= self.config.max_phrase_length:
                        term_freq[compound] += 1
                        if compound not in term_positions:
                            term_positions[compound] = position
                elif len(current_compound) == 1:
                    single = current_compound[0]
                    if self.config.min_phrase_length <= len(single) <= self.config.max_phrase_length:
                        term_freq[single] += 1
                        if single not in term_positions:
                            term_positions[single] = position
                current_compound = []
                position += 1

        # Handle remaining compound
        if len(current_compound) >= 2:
            compound = "".join(current_compound)
            if self.config.min_phrase_length <= len(compound) <= self.config.max_phrase_length:
                term_freq[compound] += 1
                if compound not in term_positions:
                    term_positions[compound] = position
        elif len(current_compound) == 1:
            single = current_compound[0]
            if self.config.min_phrase_length <= len(single) <= self.config.max_phrase_length:
                term_freq[single] += 1
                if single not in term_positions:
                    term_positions[single] = position

        # Convert to candidates, filtering out invalid tags
        for term, freq in term_freq.items():
            if self._is_valid_tag(term):
                candidates.append(
                    CandidateTag(
                        text=term,
                        source="fugashi",
                        frequency=freq,
                        position=term_positions.get(term, 0),
                    )
                )

        return candidates

    def _score_with_keybert(self, text: str, candidates: list[CandidateTag]) -> None:
        """Score candidates using KeyBERT semantic similarity."""
        if self._keybert is None or not candidates:
            return

        try:
            # Extract candidate strings
            candidate_texts = [c.text for c in candidates]

            # CRITICAL FIX (ADR-176): Use custom CountVectorizer with lowercase=False
            # By default, sklearn's CountVectorizer uses lowercase=True, which lowercases
            # the input text but NOT the candidates list. This causes uppercase candidates
            # (e.g., "GitHub", "AWS", "API") to never match the lowercased text.
            from sklearn.feature_extraction.text import CountVectorizer

            vectorizer = CountVectorizer(
                lowercase=False,  # Preserve case to match uppercase candidates
                token_pattern=r"(?u)\b\w+\b",  # noqa: S106 - Unicode regex, not password
            )

            # Use KeyBERT with candidate list
            keywords = self._keybert.extract_keywords(
                text,
                candidates=candidate_texts,
                top_n=len(candidate_texts),
                use_mmr=True,
                diversity=0.5,
                vectorizer=vectorizer,
            )

            # Map scores back to candidates
            keyword_scores = {kw[0]: kw[1] for kw in keywords}
            for candidate in candidates:
                candidate.semantic_score = keyword_scores.get(candidate.text, 0.0)

        except Exception as e:
            logger.warning("KeyBERT scoring failed", error=str(e))

    def _compute_combined_scores(self, candidates: list[CandidateTag]) -> None:
        """Compute combined scores for all candidates."""
        if not candidates:
            return

        # Normalize frequency scores
        max_freq = max(c.frequency for c in candidates)
        max_position = max(c.position for c in candidates) or 1

        for candidate in candidates:
            # Source weight
            source_weight = self.config.ginza_weight if candidate.source == "ginza" else self.config.fugashi_weight

            # Frequency score (normalized)
            freq_score = candidate.frequency / max_freq

            # Position score (earlier = higher)
            position_score = 1.0 - (candidate.position / (max_position + 1))

            # Semantic score (already 0-1)
            semantic_score = candidate.semantic_score

            # Combined score
            combined = source_weight * (
                (self.config.frequency_weight * freq_score)
                + (self.config.position_weight * position_score)
                + (self.config.semantic_weight * semantic_score)
            )

            candidate.combined_score = combined

    def _deduplicate_candidates(self, candidates: list[CandidateTag]) -> list[CandidateTag]:
        """Deduplicate candidates, keeping the highest-scoring version."""
        seen: dict[str, CandidateTag] = {}

        for candidate in candidates:
            key = candidate.text.lower()
            if key not in seen or candidate.combined_score > seen[key].combined_score:
                seen[key] = candidate

        return list(seen.values())

    def extract_tags_with_result(self, title: str, content: str) -> HybridExtractionResult:
        """
        Extract tags with full result including scores and metadata.

        Args:
            title: Article title
            content: Article content

        Returns:
            HybridExtractionResult with tags, scores, and metadata
        """
        start_time = time.perf_counter()

        self._lazy_load_models()

        # Combine title and content
        text = f"{title}\n{content}".strip()

        # Collect candidates from all sources
        all_candidates: list[CandidateTag] = []

        # Extract from GiNZA
        ginza_candidates = self._extract_candidates_ginza(text)
        all_candidates.extend(ginza_candidates)

        # Extract from Fugashi
        fugashi_candidates = self._extract_candidates_fugashi(text)
        all_candidates.extend(fugashi_candidates)

        # Score with KeyBERT if available
        if self.config.use_keybert_scoring and self._keybert is not None:
            self._score_with_keybert(text, all_candidates)

        # Compute combined scores
        self._compute_combined_scores(all_candidates)

        # Deduplicate
        unique_candidates = self._deduplicate_candidates(all_candidates)

        # Sort by combined score and take top_k
        sorted_candidates = sorted(unique_candidates, key=lambda c: -c.combined_score)
        top_candidates = [c for c in sorted_candidates if c.combined_score >= self.config.min_score][
            : self.config.top_k
        ]

        # Build result
        tags = [c.text for c in top_candidates]
        tag_scores = {c.text: round(c.combined_score, 3) for c in top_candidates}

        inference_ms = (time.perf_counter() - start_time) * 1000

        return HybridExtractionResult(
            tags=tags,
            tag_scores=tag_scores,
            inference_ms=inference_ms,
            metadata={
                "ginza_available": self._ginza is not None,
                "keybert_available": self._keybert is not None,
                "total_candidates": len(all_candidates),
                "unique_candidates": len(unique_candidates),
                "ginza_candidates": len(ginza_candidates),
                "fugashi_candidates": len(fugashi_candidates),
            },
        )

    def extract_tags(self, title: str, content: str) -> list[str]:
        """
        Extract tags from article title and content.

        This is the main interface for tag extraction.

        Args:
            title: Article title
            content: Article content

        Returns:
            List of extracted tags
        """
        result = self.extract_tags_with_result(title, content)
        return result.tags


def get_hybrid_extractor(config: HybridConfig | None = None) -> HybridExtractor:
    """Create a hybrid extractor instance."""
    return HybridExtractor(config)
