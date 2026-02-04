"""
GiNZA-based Japanese text extractor.

Provides noun phrase extraction and named entity recognition using GiNZA,
a Japanese NLP library built on spaCy with Sudachi tokenizer.

GiNZA offers:
- Dependency parsing for accurate noun phrase chunking
- Named entity recognition (NER) for proper nouns
- Better handling of compound nouns than regex-based approaches

Usage:
    from tag_extractor.ginza_extractor import GinzaExtractor

    extractor = GinzaExtractor()
    noun_phrases = extractor.extract_noun_phrases(text)
    entities = extractor.extract_named_entities(text)
"""

import threading
from dataclasses import dataclass, field
from typing import TYPE_CHECKING, Any

import structlog

if TYPE_CHECKING:
    import spacy

logger = structlog.get_logger(__name__)


@dataclass
class GinzaConfig:
    """Configuration for GiNZA extractor."""

    model_name: str = "ja_ginza"
    # Maximum text length to process (longer texts are truncated)
    max_text_length: int = 50000
    # Minimum noun phrase length (characters)
    min_phrase_length: int = 2
    # Maximum noun phrase length (characters)
    max_phrase_length: int = 30
    # Entity types to extract (None = all types)
    entity_types: list[str] | None = None
    # Enable caching of parsed documents
    enable_cache: bool = True
    # Maximum cache size
    max_cache_size: int = 100


@dataclass
class ExtractionResult:
    """Result of a GiNZA extraction operation."""

    items: list[str] = field(default_factory=list)
    scores: dict[str, float] = field(default_factory=dict)
    metadata: dict[str, Any] = field(default_factory=dict)


class GinzaExtractor:
    """
    Japanese text extractor using GiNZA NLP library.

    Thread-safe singleton pattern for efficient model reuse.
    """

    _instance: "GinzaExtractor | None" = None
    _lock = threading.Lock()
    _init_lock = threading.Lock()

    def __new__(cls, config: GinzaConfig | None = None) -> "GinzaExtractor":
        """Ensure singleton instance."""
        if cls._instance is None:
            with cls._lock:
                if cls._instance is None:
                    instance = super().__new__(cls)
                    instance._initialized = False
                    cls._instance = instance
        return cls._instance

    def __init__(self, config: GinzaConfig | None = None) -> None:
        """Initialize the GiNZA extractor."""
        with self._init_lock:
            if not getattr(self, "_initialized", False):
                self.config = config or GinzaConfig()
                self._nlp: spacy.Language | None = None
                self._available: bool | None = None
                self._cache: dict[str, Any] = {}
                self._initialized = True
                logger.info("GinzaExtractor singleton initialized", config=self.config)

    def _lazy_load_model(self) -> bool:
        """
        Lazily load the GiNZA model.

        Returns:
            True if model is available, False otherwise.
        """
        if self._nlp is not None:
            return True

        if self._available is False:
            return False

        try:
            import spacy

            logger.info("Loading GiNZA model", model_name=self.config.model_name)
            self._nlp = spacy.load(self.config.model_name)
            self._available = True
            logger.info(
                "GiNZA model loaded successfully",
                model_name=self.config.model_name,
                pipeline=self._nlp.pipe_names,
            )
            return True
        except ImportError as e:
            logger.warning(
                "GiNZA/spaCy not available",
                error=str(e),
                help="Install with: uv sync --group ml-extended",
            )
            self._available = False
            return False
        except OSError as e:
            logger.warning(
                "GiNZA model not found",
                model_name=self.config.model_name,
                error=str(e),
                help="Install model with: python -m spacy download ja_ginza",
            )
            self._available = False
            return False

    def is_available(self) -> bool:
        """Check if GiNZA is available and model can be loaded."""
        return self._lazy_load_model()

    def _get_doc(self, text: str) -> "spacy.tokens.Doc | None":
        """
        Get parsed spaCy document, with optional caching.

        Args:
            text: Input text to parse

        Returns:
            Parsed spaCy Doc or None if model not available
        """
        if not self._lazy_load_model():
            return None

        # Truncate long texts
        if len(text) > self.config.max_text_length:
            text = text[: self.config.max_text_length]
            logger.debug("Text truncated for GiNZA processing", max_length=self.config.max_text_length)

        # Check cache
        cache_key = str(hash(text))
        if self.config.enable_cache:
            if cache_key in self._cache:
                return self._cache[cache_key]

        # Parse document
        if self._nlp is None or not callable(self._nlp):
            return None
        doc = self._nlp(text)  # pyright: ignore[reportOptionalCall]

        # Update cache
        if self.config.enable_cache:
            if len(self._cache) >= self.config.max_cache_size:
                # Simple LRU: remove oldest entry
                oldest_key = next(iter(self._cache))
                del self._cache[oldest_key]
            self._cache[cache_key] = doc

        return doc

    def extract_noun_phrases(self, text: str) -> list[str]:
        """
        Extract noun phrases from Japanese text using dependency parsing.

        Uses spaCy's noun_chunks which leverages GiNZA's dependency parser
        to identify grammatically correct noun phrases.

        Args:
            text: Input Japanese text

        Returns:
            List of noun phrases, ordered by position in text
        """
        doc = self._get_doc(text)
        if doc is None:
            return []

        phrases: list[str] = []
        seen: set[str] = set()

        for chunk in doc.noun_chunks:
            phrase = chunk.text.strip()
            phrase_lower = phrase.lower()

            # Apply length filters
            if not (self.config.min_phrase_length <= len(phrase) <= self.config.max_phrase_length):
                continue

            # Skip duplicates
            if phrase_lower in seen:
                continue

            seen.add(phrase_lower)
            phrases.append(phrase)

        logger.debug("Extracted noun phrases", count=len(phrases))
        return phrases

    def extract_named_entities(self, text: str) -> list[str]:
        """
        Extract named entities from Japanese text.

        GiNZA recognizes various entity types including:
        - PERSON: Person names
        - ORG: Organizations
        - GPE: Geopolitical entities (countries, cities)
        - PRODUCT: Products
        - EVENT: Events
        - WORK_OF_ART: Creative works
        - LOC: Locations
        - NORP: Nationalities, religious/political groups

        Args:
            text: Input Japanese text

        Returns:
            List of named entities, ordered by position in text
        """
        doc = self._get_doc(text)
        if doc is None:
            return []

        entities: list[str] = []
        seen: set[str] = set()

        for ent in doc.ents:
            # Filter by entity type if configured
            if self.config.entity_types and ent.label_ not in self.config.entity_types:
                continue

            entity = ent.text.strip()
            entity_lower = entity.lower()

            # Apply length filters
            if not (self.config.min_phrase_length <= len(entity) <= self.config.max_phrase_length):
                continue

            # Skip duplicates
            if entity_lower in seen:
                continue

            seen.add(entity_lower)
            entities.append(entity)

        logger.debug("Extracted named entities", count=len(entities))
        return entities

    def extract_with_scores(self, text: str) -> ExtractionResult:
        """
        Extract noun phrases and entities with relevance scores.

        Scores are based on:
        - Position in text (earlier = higher score)
        - Frequency of occurrence
        - Entity type (named entities get a boost)

        Args:
            text: Input Japanese text

        Returns:
            ExtractionResult with items, scores, and metadata
        """
        doc = self._get_doc(text)
        if doc is None:
            return ExtractionResult()

        items: list[str] = []
        scores: dict[str, float] = {}
        seen: set[str] = set()

        # Count frequency of each token/phrase
        frequency: dict[str, int] = {}

        # Extract named entities (higher priority)
        for ent in doc.ents:
            entity = ent.text.strip()
            if self.config.min_phrase_length <= len(entity) <= self.config.max_phrase_length:
                frequency[entity] = frequency.get(entity, 0) + 1

        # Extract noun phrases
        for chunk in doc.noun_chunks:
            phrase = chunk.text.strip()
            if self.config.min_phrase_length <= len(phrase) <= self.config.max_phrase_length:
                frequency[phrase] = frequency.get(phrase, 0) + 1

        # Calculate scores
        max_freq = max(frequency.values()) if frequency else 1
        entity_set = {ent.text.strip() for ent in doc.ents}

        for i, (item, freq) in enumerate(sorted(frequency.items(), key=lambda x: -x[1])):
            item_lower = item.lower()
            if item_lower in seen:
                continue

            seen.add(item_lower)
            items.append(item)

            # Base score from frequency
            freq_score = freq / max_freq

            # Position bonus (earlier items get slight boost)
            position_score = max(0.0, 1.0 - (i * 0.02))

            # Entity bonus
            entity_bonus = 0.2 if item in entity_set else 0.0

            # Combined score
            score = (freq_score * 0.5) + (position_score * 0.3) + entity_bonus
            scores[item] = round(min(score, 1.0), 3)

        return ExtractionResult(
            items=items,
            scores=scores,
            metadata={
                "total_tokens": len(doc),
                "noun_chunk_count": len(list(doc.noun_chunks)),
                "entity_count": len(doc.ents),
            },
        )

    def extract_compound_nouns(self, text: str) -> list[str]:
        """
        Extract compound nouns using dependency parsing.

        Identifies sequences of nouns that form compound words
        by analyzing the dependency structure.

        Args:
            text: Input Japanese text

        Returns:
            List of compound nouns
        """
        doc = self._get_doc(text)
        if doc is None:
            return []

        compounds: list[str] = []
        seen: set[str] = set()

        # Look for compound noun patterns in dependency tree
        for token in doc:
            # Check if this is a noun that could be the head of a compound
            if token.pos_ not in ("NOUN", "PROPN"):
                continue

            # Collect all children that are part of compound
            compound_tokens = []
            for child in token.subtree:
                if child.pos_ in ("NOUN", "PROPN", "NUM") or child.dep_ == "compound":
                    compound_tokens.append(child)

            if len(compound_tokens) >= 2:
                # Sort by position and join
                compound_tokens.sort(key=lambda t: t.i)
                compound = "".join(t.text for t in compound_tokens)

                if (
                    compound not in seen
                    and self.config.min_phrase_length <= len(compound) <= self.config.max_phrase_length
                ):
                    seen.add(compound)
                    compounds.append(compound)

        logger.debug("Extracted compound nouns", count=len(compounds))
        return compounds

    def clear_cache(self) -> None:
        """Clear the document cache."""
        self._cache.clear()
        logger.debug("GiNZA document cache cleared")


def get_ginza_extractor(config: GinzaConfig | None = None) -> GinzaExtractor:
    """Get the singleton GiNZA extractor instance."""
    return GinzaExtractor(config)
