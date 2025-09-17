"""
Tag extraction with lazy loading model manager

This module provides enhanced tag extraction with lazy loading for better
memory efficiency and startup performance.
"""

import asyncio
import re
import unicodedata
from collections import Counter
from dataclasses import dataclass
from typing import cast

import structlog
from langdetect import LangDetectException, detect

from .input_sanitizer import InputSanitizer, SanitizationConfig
from .lazy_model_manager import get_model_manager

logger = structlog.get_logger(__name__)


@dataclass
class TagExtractionConfig:
    model_name: str = "paraphrase-multilingual-MiniLM-L12-v2"
    device: str = "cpu"
    top_keywords: int = 10
    min_score_threshold: float = 0.15
    keyphrase_ngram_range: tuple[int, int] = (1, 3)
    use_mmr: bool = True
    diversity: float = 0.5
    min_token_length: int = 2
    min_text_length: int = 10
    japanese_pos_tags: tuple[str, ...] = (
        "名詞",
        "固有名詞",
        "地名",
        "組織名",
        "人名",
        "名詞-普通名詞-一般",
        "名詞-普通名詞-サ変可能",
        "名詞-普通名詞-形状詞可能",
        "名詞-固有名詞-一般",
        "名詞-固有名詞-人名",
        "名詞-固有名詞-組織",
        "名詞-固有名詞-地域",
        "名詞-数詞",
        "名詞-副詞可能",
        "名詞-代名詞",
        "名詞-接尾辞-名詞的",
        "名詞-非自立",
    )
    extract_compound_words: bool = True
    use_frequency_boost: bool = True


class LazyTagExtractor:
    """Enhanced tag extractor with lazy loading model manager."""

    def __init__(
        self,
        config: TagExtractionConfig | None = None,
        sanitizer_config: SanitizationConfig | None = None,
    ):
        self.config = config or TagExtractionConfig()
        self._model_manager = get_model_manager()
        self._input_sanitizer = InputSanitizer(sanitizer_config)

        # Cache for loaded models
        self._sentence_transformer = None
        self._keybert = None
        self._fugashi_tagger = None
        self._nltk_stopwords = None
        self._nltk_tokenizer = None

    async def _get_sentence_transformer(self):
        """Get SentenceTransformer with lazy loading"""
        if self._sentence_transformer is None:
            self._sentence_transformer = await self._model_manager.get_sentence_transformer(self.config.model_name)
        return self._sentence_transformer

    async def _get_keybert(self):
        """Get KeyBERT with lazy loading"""
        if self._keybert is None:
            sentence_transformer = await self._get_sentence_transformer()
            # Import KeyBERT when needed
            from keybert import KeyBERT

            self._keybert = KeyBERT(model=sentence_transformer)
        return self._keybert

    async def _get_fugashi_tagger(self):
        """Get Fugashi tagger with lazy loading"""
        if self._fugashi_tagger is None:
            self._fugashi_tagger = await self._model_manager.get_fugashi_tagger()
        return self._fugashi_tagger

    async def _get_nltk_stopwords(self):
        """Get NLTK stopwords with lazy loading"""
        if self._nltk_stopwords is None:
            self._nltk_stopwords = await self._model_manager.get_nltk_stopwords()
        return self._nltk_stopwords

    async def _get_nltk_tokenizer(self):
        """Get NLTK tokenizer with lazy loading"""
        if self._nltk_tokenizer is None:
            self._nltk_tokenizer = await self._model_manager.get_nltk_tokenizer()
        return self._nltk_tokenizer

    def _detect_language(self, text: str) -> str:
        """Detect the language of the text."""
        try:
            return str(detect(text.replace("\n", " ")))
        except LangDetectException:
            logger.warning("Language detection failed, defaulting to English")
            return "en"

    def _normalize_text(self, text: str, lang: str) -> str:
        """Normalize text based on language."""
        if lang == "ja":
            # NFKC normalization for Japanese
            normalized = unicodedata.normalize("NFKC", text)
            return normalized
        else:
            return text.lower()

    async def _extract_compound_japanese_words(self, text: str) -> list[str]:
        """Extract compound words and important phrases from Japanese text."""
        tagger = await self._get_fugashi_tagger()
        compound_words = []

        # Patterns for compound words
        patterns = [
            r"[A-Za-z][A-Za-z0-9]*[ァ-ヶー]+(?:[A-Za-z0-9]*)?",
            r"[ァ-ヶー]+[A-Za-z][A-Za-z0-9]*",
            r"[A-Z]{2,}(?:[a-z]+)?",
            r"[一-龥]{2,}[ァ-ヶー]+",
            r"[一-龥]+(?:の)[一-龥]+",
            r"[一-龥]{2,4}(?:大統領|首相|総理|議員|知事|市長)",
            r"[一-龥]{2,4}(?:会社|企業|組織|団体|協会|連盟)",
            r"[ァ-ヶー]{3,}(?:システム|サービス|プラットフォーム)",
            r"[ァ-ヶー]{2,}(?:[ァ-ヶー]+)?(?:[A-Za-z0-9]+)?",
            r"[一-龥]{2,}[A-Za-z0-9]+",
            r"[A-Za-z0-9]+[一-龥]{2,}",
            r"\d+[A-Za-zァ-ヶー一-龥]+",
        ]

        for pattern in patterns:
            matches = re.findall(pattern, text)
            compound_words.extend(matches)

        # Use fugashi for intelligent noun phrase extraction
        parsed = list(tagger(text))
        i = 0
        while i < len(parsed):
            if parsed[i].feature.pos1 in self.config.japanese_pos_tags:
                if parsed[i].feature.pos2 in ["固有名詞", "組織", "人名", "地域"]:
                    compound = parsed[i].surface
                    j = i + 1

                    # Look for connected proper nouns
                    while j < len(parsed):
                        if parsed[j].feature.pos1 in self.config.japanese_pos_tags:
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
                            if j + 1 < len(parsed) and parsed[j + 1].feature.pos1 in self.config.japanese_pos_tags:
                                compound += parsed[j].surface + parsed[j + 1].surface
                                j += 2
                            else:
                                break
                        else:
                            break

                    if len(compound) >= 3:
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

    async def _extract_keywords_japanese(self, text: str) -> list[str]:
        """Extract keywords specifically for Japanese text."""
        # Get Japanese stopwords
        stopwords = await self._get_nltk_stopwords()
        ja_stopwords = set(stopwords.words("japanese")) if hasattr(stopwords, "words") else set()

        # Get tagger
        tagger = await self._get_fugashi_tagger()

        # Extract compound words
        compounds = await self._extract_compound_japanese_words(text)
        term_freq = Counter(compounds)

        # Extract single important nouns
        single_nouns = []
        for word in tagger(text):
            if (
                word.feature.pos1 in self.config.japanese_pos_tags
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

        # Get top keywords by frequency
        top_keywords = []
        for term, freq in combined_freq.most_common(self.config.top_keywords * 2):
            if freq >= 2 or len(term) >= 4:
                top_keywords.append(term)

        return top_keywords[: self.config.top_keywords]

    async def _extract_keywords_english(self, text: str) -> list[str]:
        """Extract keywords specifically for English text using KeyBERT."""
        keybert = await self._get_keybert()

        try:
            # Extract single words and phrases
            single_keywords = keybert.extract_keywords(
                text,
                keyphrase_ngram_range=(1, 1),
                top_n=self.config.top_keywords * 3,
                use_mmr=True,
                diversity=0.3,
            )

            phrase_keywords = keybert.extract_keywords(
                text,
                keyphrase_ngram_range=(2, 3),
                top_n=self.config.top_keywords,
                use_mmr=True,
                diversity=0.5,
            )

            # Combine and process keywords
            all_keywords = []
            seen_words = set()

            # Process phrases first
            for phrase_tuple in cast(list[tuple[str, float]], phrase_keywords):
                phrase = phrase_tuple[0].strip().lower()
                score = phrase_tuple[1]
                if score >= self.config.min_score_threshold * 1.5:
                    words = phrase.split()
                    if len(words) >= 2:
                        if any(w[0].isupper() for w in phrase.split() if w):
                            all_keywords.append((phrase, score))
                            seen_words.update(words)

            # Then add important single words
            for word_tuple in cast(list[tuple[str, float]], single_keywords):
                word = word_tuple[0].strip().lower()
                score = word_tuple[1]
                if score >= self.config.min_score_threshold and word not in seen_words:
                    if len(word) > 2 and not word.isdigit():
                        all_keywords.append((word, score))
                        seen_words.add(word)

            # Sort by score and filter
            all_keywords.sort(key=lambda x: x[1], reverse=True)

            # Final filtering
            result = []
            seen_final: set[str] = set()

            for keyword, _score in all_keywords:
                keyword_clean = keyword.strip()
                keyword_lower = keyword_clean.lower()

                if keyword_lower not in seen_final:
                    is_substring = False
                    for seen in seen_final:
                        if keyword_lower in seen or seen in keyword_lower:
                            is_substring = True
                            break

                    if not is_substring:
                        result.append(keyword_clean)
                        seen_final.add(keyword_lower)

                        if len(result) >= self.config.top_keywords:
                            break

            return result

        except Exception as e:
            logger.error("KeyBERT extraction failed for English", error=e)
            return []

    async def _tokenize_english(self, text: str) -> list[str]:
        """Tokenize English text using NLTK."""
        tokenizer = await self._get_nltk_tokenizer()
        stopwords = await self._get_nltk_stopwords()

        en_stopwords = set(stopwords.words("english")) if hasattr(stopwords, "words") else set()

        tokens = tokenizer(text)
        result = []

        for token in tokens:
            if re.fullmatch(r"\w+", token) and len(token) > self.config.min_token_length:
                normalized = self._normalize_text(token, "en")
                if normalized not in en_stopwords:
                    result.append(normalized)

        return result

    async def _fallback_extraction(self, text: str, lang: str) -> list[str]:
        """Fallback extraction method when primary method fails."""
        if lang == "ja":
            return await self._extract_keywords_japanese(text)
        else:
            tokens = await self._tokenize_english(text)
            if tokens:
                token_freq = Counter(tokens)
                return [term for term, _ in token_freq.most_common(self.config.top_keywords)]
            return []

    async def extract_tags(self, title: str, content: str) -> list[str]:
        """
        Extract tags from title and content with language-specific processing.

        Args:
            title: The title text
            content: The content text

        Returns:
            List of extracted tags
        """
        # Sanitize input first
        sanitization_result = self._input_sanitizer.sanitize(title, content)

        if not sanitization_result.is_valid:
            logger.warning("Input sanitization failed", violations=sanitization_result.violations)
            return []

        # Use sanitized input
        sanitized_input = sanitization_result.sanitized_input
        if sanitized_input is None:
            logger.error("Sanitized input is None despite valid sanitization")
            return []

        sanitized_title = sanitized_input.title
        sanitized_content = sanitized_input.content
        raw_text = f"{sanitized_title}\n{sanitized_content}"

        # Validate input length
        if len(raw_text.strip()) < self.config.min_text_length:
            logger.info(
                "Sanitized input too short, skipping extraction",
                char_count=len(raw_text),
            )
            return []

        logger.info(
            "Processing sanitized text",
            char_count=len(raw_text),
            original_length=sanitized_input.original_length,
            sanitized_length=sanitized_input.sanitized_length,
        )

        # Detect language
        lang = self._detect_language(raw_text)
        logger.info("Detected language", lang=lang)

        # Language-specific extraction
        try:
            if lang == "ja":
                keywords = await self._extract_keywords_japanese(raw_text)
            else:
                keywords = await self._extract_keywords_english(raw_text)

            if keywords:
                logger.info("Extraction successful", keywords=keywords)
                return keywords
            else:
                logger.info("Primary extraction failed, trying fallback method")
                fallback_keywords = await self._fallback_extraction(raw_text, lang)
                if fallback_keywords:
                    logger.info("Fallback extraction successful", keywords=fallback_keywords)
                    return fallback_keywords

        except Exception as e:
            logger.error("Extraction error", error=e)
            try:
                fallback_keywords = await self._fallback_extraction(raw_text, lang)
                if fallback_keywords:
                    logger.info(f"Emergency fallback successful: {fallback_keywords}")
                    return fallback_keywords
            except Exception as e2:
                logger.error("Fallback also failed", error=e2)

        logger.warning("No tags could be extracted")
        return []

    async def preload_models(self):
        """Preload models for better performance"""
        await self._model_manager.preload_models(
            [
                "nltk_stopwords",
                "nltk_tokenizer",
                "sentence_transformer_paraphrase-multilingual-MiniLM-L12-v2",
                "fugashi_tagger",
            ]
        )

    def get_model_stats(self):
        """Get model statistics"""
        return self._model_manager.get_model_stats()


# Async wrapper for backward compatibility
async def extract_tags_async(title: str, content: str) -> list[str]:
    """
    Async function for tag extraction with lazy loading.

    Args:
        title: The title text
        content: The content text

    Returns:
        List of extracted tags
    """
    extractor = LazyTagExtractor()
    return await extractor.extract_tags(title, content)


# Synchronous wrapper for backward compatibility
def extract_tags(title: str, content: str) -> list[str]:
    """
    Synchronous wrapper for tag extraction.

    Args:
        title: The title text
        content: The content text

    Returns:
        List of extracted tags
    """
    return asyncio.run(extract_tags_async(title, content))
