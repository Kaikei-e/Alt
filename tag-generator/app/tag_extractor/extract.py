import logging
import re
import unicodedata
from typing import List, Optional, Tuple, Set, Dict
from dataclasses import dataclass
from collections import Counter

from langdetect import detect, LangDetectException
import nltk
import fugashi
from .model_manager import get_model_manager, ModelConfig

# Configure logging
logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)

@dataclass
class TagExtractionConfig:
    model_name: str = "paraphrase-multilingual-MiniLM-L12-v2"
    device: str = "cpu"
    top_keywords: int = 10
    min_score_threshold: float = 0.15  # Lower threshold for better extraction
    keyphrase_ngram_range: Tuple[int, int] = (1, 3)
    use_mmr: bool = True
    diversity: float = 0.5
    min_token_length: int = 2
    min_text_length: int = 10
    japanese_pos_tags: Tuple[str, ...] = (
        "名詞", "固有名詞", "地名", "組織名", "人名", 
        "名詞-普通名詞-一般", "名詞-普通名詞-サ変可能", "名詞-普通名詞-形状詞可能", 
        "名詞-固有名詞-一般", "名詞-固有名詞-人名", "名詞-固有名詞-組織", "名詞-固有名詞-地域", 
        "名詞-数詞", "名詞-副詞可能", "名詞-代名詞", "名詞-接尾辞-名詞的", "名詞-非自立" 
    )
    extract_compound_words: bool = True
    use_frequency_boost: bool = True

class TagExtractor:
    """A class for extracting tags from text using KeyBERT and language-specific processing."""

    def __init__(self, config: Optional[TagExtractionConfig] = None):
        self.config = config or TagExtractionConfig()
        self._model_manager = get_model_manager()
        self._models_loaded = False

    def _lazy_load_models(self) -> None:
        """Lazy load models using the singleton model manager."""
        if not self._models_loaded:
            model_config = ModelConfig(
                model_name=self.config.model_name,
                device=self.config.device
            )
            self._embedder, self._keybert, self._ja_tagger = self._model_manager.get_models(model_config)
            self._models_loaded = True
            logger.debug("Models loaded via ModelManager")

    def _load_stopwords(self) -> None:
        """Load stopwords using the model manager."""
        if not hasattr(self, '_stopwords_loaded'):
            self._ja_stopwords, self._en_stopwords = self._model_manager.get_stopwords()
            self._stopwords_loaded = True

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
            # NFKC normalization for Japanese
            normalized = unicodedata.normalize("NFKC", text)
            # Keep English words in Japanese text as-is
            return normalized
        else:
            return text.lower()

    def _extract_compound_japanese_words(self, text: str) -> List[str]:
        """Extract compound words and important phrases from Japanese text."""
        self._lazy_load_models()
        compound_words = []

        # Patterns for compound words - more restrictive to avoid over-splitting
        patterns = [
            # Tech terms with mixed scripts
            r'[A-Za-z][A-Za-z0-9]*[ァ-ヶー]+(?:[A-Za-z0-9]*)?',  # e.g., "GitHubリポジトリ", "JAビル"
            r'[ァ-ヶー]+[A-Za-z][A-Za-z0-9]*',  # e.g., "データセットID"
            r'[A-Z]{2,}(?:[a-z]+)?',  # Acronyms like "JA", "AI", "CEO"
            r'[一-龥]{2,}[ァ-ヶー]+',  # Kanji + Katakana compounds
            r'[一-龥]+(?:の)[一-龥]+',  # Kanji + の + Kanji (e.g., "日本の首相")
            # Important proper nouns
            r'[一-龥]{2,4}(?:大統領|首相|総理|議員|知事|市長)',  # Political titles
            r'[一-龥]{2,4}(?:会社|企業|組織|団体|協会|連盟)',  # Organizations
            r'[ァ-ヶー]{3,}(?:システム|サービス|プラットフォーム)',  # Tech terms
            # Additional patterns for Japanese compound words
            r'[ァ-ヶー]{2,}(?:[ァ-ヶー]+)?(?:[A-Za-z0-9]+)?', # Katakana compounds (e.g., "クラウドコンピューティング", "AIモデル")
            r'[一-龥]{2,}[A-Za-z0-9]+', # Kanji + Alphanumeric (e.g., "情報IT", "技術AI")
            r'[A-Za-z0-9]+[一-龥]{2,}', # Alphanumeric + Kanji (e.g., "IoT機器", "Web技術")
            r'\d+[A-Za-zァ-ヶー一-龥]+', # Number + Word (e.g., "5G通信", "3Dプリンター")
        ]

        for pattern in patterns:
            matches = re.findall(pattern, text)
            compound_words.extend(matches)

        # Use fugashi for more intelligent noun phrase extraction
        parsed = list(self._ja_tagger(text))
        i = 0
        while i < len(parsed):
            if parsed[i].feature.pos1 in self.config.japanese_pos_tags:
                # Check if it's a proper noun or organization
                if parsed[i].feature.pos2 in ['固有名詞', '組織', '人名', '地域']:
                    compound = parsed[i].surface
                    j = i + 1

                    # Look for connected proper nouns
                    while j < len(parsed):
                        if parsed[j].feature.pos1 in self.config.japanese_pos_tags:
                            if parsed[j].feature.pos2 in ['固有名詞', '組織', '人名', '地域']:
                                compound += parsed[j].surface
                                j += 1
                            else:
                                break
                        elif parsed[j].surface in ['・', '＝', '－']:
                            # Include connectors in proper nouns
                            if j + 1 < len(parsed) and parsed[j + 1].feature.pos1 in self.config.japanese_pos_tags:
                                compound += parsed[j].surface + parsed[j + 1].surface
                                j += 2
                            else:
                                break
                        else:
                            break

                    if len(compound) >= 3:  # Minimum length for compound words
                        compound_words.append(compound)
                    i = j
                else:
                    i += 1
            else:
                i += 1

        # Deduplicate while preserving order
        seen = set()
        unique_compounds = []
        for word in compound_words:
            if word not in seen and len(word) >= 2:
                seen.add(word)
                unique_compounds.append(word)

        return unique_compounds

    def _extract_keywords_japanese(self, text: str) -> List[str]:
        """Extract keywords specifically for Japanese text."""
        self._lazy_load_models()
        self._load_stopwords()

        # Extract compound words and important terms
        compounds = self._extract_compound_japanese_words(text)

        # Count frequencies
        term_freq = Counter(compounds)

        # Also extract single important nouns
        single_nouns = []
        for word in self._ja_tagger(text):
            if (word.feature.pos1 in self.config.japanese_pos_tags and
                2 <= len(word.surface) <= 10 and
                word.surface not in self._ja_stopwords):
                single_nouns.append(word.surface)

        # Add single noun frequencies
        single_freq = Counter(single_nouns)

        # Combine frequencies, giving priority to compounds
        combined_freq = Counter()
        for term, freq in term_freq.items():
            combined_freq[term] = freq * 2  # Boost compound words

        for term, freq in single_freq.items():
            if term not in combined_freq:
                combined_freq[term] = freq

        # Get top keywords by frequency
        top_keywords = []
        for term, freq in combined_freq.most_common(self.config.top_keywords * 2):
            if freq >= 2 or len(term) >= 4:  # Include terms that appear 2+ times or are longer
                top_keywords.append(term)

        # Limit to configured number
        return top_keywords[:self.config.top_keywords]

    def _extract_keywords_english(self, text: str) -> List[str]:
        """Extract keywords specifically for English text using KeyBERT."""
        self._lazy_load_models()

        try:
            # First extract both single words and phrases
            single_keywords = self._keybert.extract_keywords(
                text,
                keyphrase_ngram_range=(1, 1),  # Single words only
                top_n=self.config.top_keywords * 3,
                use_mmr=True,
                diversity=0.3
            )

            phrase_keywords = self._keybert.extract_keywords(
                text,
                keyphrase_ngram_range=(2, 3),  # Phrases only
                top_n=self.config.top_keywords,
                use_mmr=True,
                diversity=0.5
            )

            # Combine and process keywords
            all_keywords = []
            seen_words = set()

            # Process phrases first to identify important compound terms
            for phrase, score in phrase_keywords:
                phrase = phrase.strip().lower()
                # Only keep phrases with high scores or specific patterns
                if score >= self.config.min_score_threshold * 1.5:  # Higher threshold for phrases
                    # Check if it's a meaningful compound (e.g., "apple intelligence", "mac mini")
                    words = phrase.split()
                    if len(words) >= 2:
                        # Check for tech terms, product names, or proper nouns
                        if any(w[0].isupper() for w in phrase.split() if w):
                            all_keywords.append((phrase, score))
                            # Mark individual words as seen to avoid duplication
                            seen_words.update(words)

            # Then add important single words not already in phrases
            for word, score in single_keywords:
                word = word.strip().lower()
                if score >= self.config.min_score_threshold and word not in seen_words:
                    # Skip generic words
                    if len(word) > 2 and not word.isdigit():
                        all_keywords.append((word, score))
                        seen_words.add(word)

            # Sort by score and filter
            all_keywords.sort(key=lambda x: x[1], reverse=True)

            # Final filtering and cleaning
            result = []
            seen_final = set()

            for keyword, score in all_keywords:
                # Clean and check for duplicates
                keyword_clean = keyword.strip()
                keyword_lower = keyword_clean.lower()

                # Skip if we've seen this or a very similar variant
                if keyword_lower not in seen_final:
                    # Check for substring relationships
                    is_substring = False
                    for seen in seen_final:
                        if keyword_lower in seen or seen in keyword_lower:
                            # Only skip if the longer one has higher score
                            is_substring = True
                            break

                    if not is_substring:
                        result.append(keyword_clean)
                        seen_final.add(keyword_lower)

                        if len(result) >= self.config.top_keywords:
                            break

            return result

        except Exception as e:
            logger.error(f"KeyBERT extraction failed for English: {e}")
            return []

    def _tokenize_english(self, text: str) -> List[str]:
        """Tokenize English text using NLTK."""
        self._load_stopwords()
        tokens = nltk.word_tokenize(text)
        result = []

        for token in tokens:
            if (re.fullmatch(r"\w+", token) and
                len(token) > self.config.min_token_length):
                normalized = self._normalize_text(token, "en")
                if normalized not in self._en_stopwords:
                    result.append(normalized)

        return result

    def _fallback_extraction(self, text: str, lang: str) -> List[str]:
        """Fallback extraction method when primary method fails."""
        if lang == "ja":
            # For Japanese, use the frequency-based approach
            return self._extract_keywords_japanese(text)
        else:
            # For English, try tokenization and frequency
            tokens = self._tokenize_english(text)
            if tokens:
                token_freq = Counter(tokens)
                return [term for term, _ in token_freq.most_common(self.config.top_keywords)]
            return []

    def extract_tags(self, title: str, content: str) -> List[str]:
        """
        Extract tags from title and content with language-specific processing.

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

        # Language-specific extraction
        try:
            if lang == "ja":
                # Japanese-specific extraction
                keywords = self._extract_keywords_japanese(raw_text)
            else:
                # English and other languages use KeyBERT
                keywords = self._extract_keywords_english(raw_text)

            if keywords:
                logger.info(f"Extraction successful: {keywords}")
                return keywords
            else:
                # Try fallback method
                logger.info("Primary extraction failed, trying fallback method")
                fallback_keywords = self._fallback_extraction(raw_text, lang)
                if fallback_keywords:
                    logger.info(f"Fallback extraction successful: {fallback_keywords}")
                    return fallback_keywords

        except Exception as e:
            logger.error(f"Extraction error: {e}")
            # Try fallback on any error
            try:
                fallback_keywords = self._fallback_extraction(raw_text, lang)
                if fallback_keywords:
                    logger.info(f"Emergency fallback successful: {fallback_keywords}")
                    return fallback_keywords
            except Exception as e2:
                logger.error(f"Fallback also failed: {e2}")

        logger.warning("No tags could be extracted")
        return []

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