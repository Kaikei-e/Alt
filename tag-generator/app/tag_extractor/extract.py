import logging
import os
import re
import unicodedata
from typing import List, Optional, Tuple, Set
from dataclasses import dataclass

from sentence_transformers import SentenceTransformer
from keybert import KeyBERT
from langdetect import detect, LangDetectException
from fugashi import Tagger
import nltk

# Configure logging
logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)

@dataclass
class TagExtractionConfig:
    """Configuration for tag extraction parameters."""
    model_name: str = "paraphrase-multilingual-MiniLM-L12-v2"
    device: str = 'cpu'
    top_keywords: int = 5
    min_score_threshold: float = 0.1
    min_token_length: int = 2
    min_text_length: int = 10
    japanese_pos_tags: Tuple[str, ...] = ("名詞", "固有名詞")

class TagExtractor:
    """A class for extracting tags from text using KeyBERT and language-specific processing."""

    def __init__(self, config: Optional[TagExtractionConfig] = None):
        self.config = config or TagExtractionConfig()
        self._embedder: Optional[SentenceTransformer] = None
        self._keybert: Optional[KeyBERT] = None
        self._ja_tagger: Optional[Tagger] = None
        self._ja_stopwords: Optional[Set[str]] = None
        self._en_stopwords: Optional[Set[str]] = None

    def _lazy_load_models(self) -> None:
        """Lazy load models to avoid loading them at import time."""
        if self._embedder is None:
            logger.info("Loading SentenceTransformer model...")
            self._embedder = SentenceTransformer(
                self.config.model_name,
                device=self.config.device
            )
            self._keybert = KeyBERT(self._embedder)
            self._ja_tagger = Tagger("-Owakati")
            logger.info("Models loaded successfully")

    def _load_stopwords(self) -> None:
        """Load stopwords for Japanese and English."""
        if self._ja_stopwords is None:
            current_dir = os.path.dirname(__file__)
            ja_stopwords_path = os.path.join(current_dir, "stopwords_ja.txt")
            en_stopwords_path = os.path.join(current_dir, "stopwords_en.txt")

            # Load Japanese stopwords
            try:
                with open(ja_stopwords_path, 'r', encoding='utf-8') as f:
                    self._ja_stopwords = set(line.strip() for line in f if line.strip())
            except FileNotFoundError:
                logger.warning(f"Japanese stopwords file not found: {ja_stopwords_path}")
                self._ja_stopwords = set()

            # Load English stopwords
            try:
                with open(en_stopwords_path, 'r', encoding='utf-8') as f:
                    self._en_stopwords = set(line.strip().lower() for line in f if line.strip())
            except FileNotFoundError:
                logger.warning(f"English stopwords file not found: {en_stopwords_path}")
                self._en_stopwords = set()

            # Add NLTK English stopwords
            try:
                if self._en_stopwords is None:
                    self._en_stopwords = set()
                self._en_stopwords.update(set(nltk.corpus.stopwords.words("english")))
            except Exception as e:
                logger.warning(f"Could not load NLTK English stopwords: {e}")

    def _detect_language(self, text: str) -> str:
        """Detect the language of the text."""
        try:
            return detect(text.replace("\n", " "))
        except LangDetectException:
            logger.warning("Language detection failed, defaulting to English")
            return "en"

    def _normalize_text(self, text: str, lang: str) -> str:
        """Normalize text based on language."""
        if lang == "ja":
            return unicodedata.normalize("NFKC", text)
        else:
            return text.lower()

    def _tokenize_japanese(self, text: str) -> List[str]:
        """Tokenize Japanese text using fugashi."""
        self._lazy_load_models()
        self._load_stopwords()
        tokens = []

        if self._ja_tagger is None or self._ja_stopwords is None:
            return []

        for word in self._ja_tagger(text):
            if (word.feature.pos1 in self.config.japanese_pos_tags and
                len(word.surface) > 1):
                normalized = self._normalize_text(word.surface, "ja")
                if normalized not in self._ja_stopwords:
                    tokens.append(normalized)

        return tokens

    def _tokenize_english(self, text: str) -> List[str]:
        """Tokenize English text using NLTK."""
        self._load_stopwords()
        tokens = nltk.word_tokenize(text)
        result = []

        if self._en_stopwords is None:
            return []

        for token in tokens:
            if (re.fullmatch(r"\w+", token) and
                len(token) > self.config.min_token_length):
                normalized = self._normalize_text(token, "en")
                if normalized not in self._en_stopwords:
                    result.append(normalized)

        return result

    def _get_candidate_tokens(self, text: str, lang: str) -> List[str]:
        """Get candidate tokens based on language."""
        if lang == "ja":
            return self._tokenize_japanese(text)
        else:
            return self._tokenize_english(text)

    def _extract_keywords_direct(self, text: str) -> List[Tuple[str, float]]:
        """Extract keywords directly from text using KeyBERT."""
        self._lazy_load_models()
        if self._keybert is None:
            return []
        try:
            return self._keybert.extract_keywords(text, top_n=self.config.top_keywords)
        except Exception as e:
            logger.error(f"Direct KeyBERT extraction failed: {e}")
            return []

    def _extract_keywords_from_candidates(self, candidates: List[str]) -> List[Tuple[str, float]]:
        """Extract keywords from processed candidate tokens."""
        if not candidates:
            return []

        self._lazy_load_models()
        if self._keybert is None:
            return []

        try:
            text_for_keybert = " ".join(candidates)
            return self._keybert.extract_keywords(text_for_keybert, top_n=self.config.top_keywords)
        except Exception as e:
            logger.error(f"Candidate-based KeyBERT extraction failed: {e}")
            return []

    def _filter_keywords(self, keywords: List[Tuple[str, float]]) -> List[str]:
        """Filter keywords based on score threshold."""
        return [
            keyword for keyword, score in keywords
            if score >= self.config.min_score_threshold
        ][:self.config.top_keywords]

    def extract_tags(self, title: str, content: str) -> List[str]:
        """
        Extract tags from title and content.

        Args:
            title: The title text
            content: The content text

        Returns:
            List of extracted tags
        """
        raw_text = f"{title}\n{content}"

        # Validate input
        if len(raw_text.strip()) < self.config.min_text_length:
            logger.info(f"Input too short ({len(raw_text)} chars), skipping extraction")
            return []

        logger.info(f"Processing text with {len(raw_text)} characters")

        # Detect language
        lang = self._detect_language(raw_text)
        logger.info(f"Detected language: {lang}")

        # Try direct extraction first
        keywords = self._extract_keywords_direct(raw_text)

        if keywords:
            result = self._filter_keywords(keywords)
            logger.info(f"Direct extraction successful: {result}")
            return result

        # Fallback to candidate-based extraction
        logger.info("Direct extraction failed, trying candidate-based approach")
        candidates = self._get_candidate_tokens(raw_text, lang)
        logger.info(f"Found {len(candidates)} candidate tokens")

        if not candidates:
            logger.warning("No candidate tokens found")
            return []

        keywords = self._extract_keywords_from_candidates(candidates)
        result = self._filter_keywords(keywords)

        logger.info(f"Final extraction result: {result}")
        return result

# Maintain backward compatibility
def extract_tags(title: str, content: str) -> List[str]:
    """
    Legacy function for backward compatibility.

    Args:
        title: The title text
        content: The content text

    Returns:
        List of extracted tags
    """
    extractor = TagExtractor()
    return extractor.extract_tags(title, content)
