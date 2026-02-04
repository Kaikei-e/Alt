import os
import re
import time
import unicodedata
from collections import Counter
from dataclasses import dataclass, field
from typing import Any, cast

import nltk
import structlog
from langdetect import LangDetectException, detect

# Re-export heavy model classes for compatibility with unit tests that patch
# them directly on this module path (e.g., `patch("tag_extractor.extract.SentenceTransformer")`).
# Importing them lazily would complicate patching semantics, so we import once
# at import time. This does not load actual model weights because instantiation
# happens elsewhere; it only brings the symbols into the namespace.

try:
    from fugashi import Tagger as _Tagger  # type: ignore
    from keybert import KeyBERT as _KeyBERT  # type: ignore
    from sentence_transformers import (  # type: ignore
        SentenceTransformer as _SentenceTransformer,
    )

    # Alias for outward exposure
    SentenceTransformer = _SentenceTransformer  # noqa: N816 (keep original casing for patching)
    KeyBERT = _KeyBERT  # noqa: N816
    Tagger = _Tagger  # noqa: N816
except ImportError:
    # Fallback for environments without ML dependencies (e.g., production builds)
    # These will be mocked in tests
    SentenceTransformer = None  # type: ignore
    KeyBERT = None  # type: ignore
    Tagger = None  # type: ignore

# Local imports depending on re-export must come after alias definitions for consistency
from .input_sanitizer import InputSanitizer, SanitizationConfig
from .model_manager import ModelConfig, get_model_manager

logger = structlog.get_logger(__name__)


@dataclass
class TagExtractionConfig:
    model_name: str = "paraphrase-multilingual-MiniLM-L12-v2"
    device: str = "cpu"
    top_keywords: int = 10
    min_score_threshold: float = 0.15  # Lower threshold for better extraction
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
    use_onnx_runtime: bool = True
    onnx_model_path: str | None = None  # Will be set in __post_init__
    onnx_tokenizer_name: str = "sentence-transformers/paraphrase-multilingual-MiniLM-L12-v2"
    onnx_pooling: str = "cls"
    onnx_batch_size: int = 16
    onnx_max_length: int = 256
    use_fp16: bool = False  # Enable FP16 for ~50% memory reduction (set via TAG_USE_FP16=true)
    # Enable semantic scoring for Japanese text using KeyBERT
    # When enabled, candidates from Fugashi are scored using KeyBERT embeddings
    use_japanese_semantic: bool = True
    # MMR diversity for Japanese KeyBERT scoring
    japanese_mmr_diversity: float = 0.5

    def __post_init__(self) -> None:
        """Set default ONNX model path if not provided."""
        # Default path: /models/onnx/model.onnx (can be overridden via TAG_ONNX_MODEL_PATH)
        if self.onnx_model_path is None:
            self.onnx_model_path = os.getenv("TAG_ONNX_MODEL_PATH", "/models/onnx/model.onnx")

        # Auto-disable ONNX runtime if model file doesn't exist
        if self.use_onnx_runtime and self.onnx_model_path is not None:
            if not os.path.exists(self.onnx_model_path):
                logger.info(
                    "ONNX runtime requested but model file not found; disabling ONNX runtime",
                    model_path=self.onnx_model_path,
                    use_onnx_runtime=self.use_onnx_runtime,
                )
                self.use_onnx_runtime = False

        # Enable FP16 via environment variable (TAG_USE_FP16=true)
        if os.getenv("TAG_USE_FP16", "").lower() in ("true", "1", "yes"):
            self.use_fp16 = True
            logger.info("FP16 mode enabled via TAG_USE_FP16 environment variable")


@dataclass
class TagExtractionOutcome:
    """Metrics-bearing container for tag extraction results."""

    tags: list[str]
    confidence: float
    tag_count: int
    inference_ms: float
    language: str
    model_name: str
    sanitized_length: int
    embedding_backend: str = "unknown"
    tag_confidences: dict[str, float] = field(default_factory=dict)  # Individual tag confidence scores
    embedding_metadata: dict[str, Any] = field(default_factory=dict)


class TagExtractor:
    """A class for extracting tags from text using KeyBERT and language-specific processing."""

    def __init__(
        self,
        config: TagExtractionConfig | None = None,
        sanitizer_config: SanitizationConfig | None = None,
    ):
        self.config = config or TagExtractionConfig()
        self._model_manager = get_model_manager()
        self._models_loaded = False
        self._input_sanitizer = InputSanitizer(sanitizer_config)
        self._embedding_backend: str = "unknown"
        self._embedding_metadata: dict[str, Any] = {}
        # Lazily populated model handles (set by _lazy_load_models via ModelManager)
        self._embedder: Any | None = None
        self._keybert: Any | None = None
        self._ja_tagger: Any | None = None

    def _compute_confidence(self, tags: list[str], sanitized_length: int) -> float:
        """Derive a lightweight confidence score from tag coverage and article size."""
        if not tags:
            return 0.0

        coverage = min(len(tags) / max(1, self.config.top_keywords), 1.0)
        length_factor = min(sanitized_length / 1200.0, 1.0)
        score = 0.7 * coverage + 0.3 * length_factor
        return round(max(0.0, min(score, 1.0)), 3)

    def _run_extraction(self, raw_text: str, lang: str) -> tuple[list[str], dict[str, float]]:
        """Run the primary extraction logic with fallback handling.

        Returns:
            Tuple of (tag_list, tag_confidences_dict)
        """
        try:
            keywords: list[str]
            confidences: dict[str, float]

            if lang == "ja":
                keywords, confidences = self._extract_keywords_japanese(raw_text)
            else:
                keywords, confidences = self._extract_keywords_english(raw_text)

            if keywords:
                logger.debug("Extraction successful", keywords=keywords)
                return keywords, confidences

            logger.debug("Primary extraction yielded no tags, invoking fallback")
            try:
                fallback_keywords = self._fallback_extraction(raw_text, lang)
                # For fallback, assign default confidence based on position
                fallback_confidences = {tag: max(0.3, 0.7 - (i * 0.1)) for i, tag in enumerate(fallback_keywords)}
            except Exception as fallback_error:
                logger.error("Fallback extraction failed", error=fallback_error)
                return [], {}

            if fallback_keywords:
                logger.debug("Fallback extraction succeeded", keywords=fallback_keywords)
                return fallback_keywords, fallback_confidences

        except Exception as e:
            logger.error("Extraction error", error=e)
            try:
                fallback_keywords = self._fallback_extraction(raw_text, lang)
                # For fallback, assign default confidence based on position
                fallback_confidences = {tag: max(0.3, 0.7 - (i * 0.1)) for i, tag in enumerate(fallback_keywords)}
            except Exception as fallback_error:
                logger.error("Fallback extraction failed after exception", error=fallback_error)
                return [], {}

            if fallback_keywords:
                logger.debug("Emergency fallback successful", keywords=fallback_keywords)
                return fallback_keywords, fallback_confidences

        logger.warning("No tags could be extracted")
        return [], {}

    def _truncate_content(self, title: str, content: str, max_content_length: int) -> tuple[str, str]:
        """
        Truncate content to fit within sanitization limits while preserving title.

        Args:
            title: Article title
            content: Article content
            max_content_length: Maximum allowed content length

        Returns:
            Tuple of (truncated_title, truncated_content)
        """
        # Truncate title if needed (preserve max_title_length)
        max_title_length = 1000
        truncated_title = title[:max_title_length] if len(title) > max_title_length else title

        # Truncate content if needed
        if len(content) > max_content_length:
            # Try to truncate at sentence boundary for better quality
            truncated_content = content[:max_content_length]
            # Find last sentence boundary (period, exclamation, question mark)
            last_sentence_end = max(
                truncated_content.rfind("."),
                truncated_content.rfind("!"),
                truncated_content.rfind("?"),
            )
            if last_sentence_end > max_content_length * 0.8:  # Only use if we keep at least 80% of content
                truncated_content = content[: last_sentence_end + 1]
            else:
                truncated_content = content[:max_content_length]
            logger.info(
                "Content truncated for sanitization",
                original_length=len(content),
                truncated_length=len(truncated_content),
            )
        else:
            truncated_content = content

        return truncated_title, truncated_content

    def extract_tags_with_metrics(self, title: str, content: str) -> TagExtractionOutcome:
        """
        Extract tags and capture metrics for cascade decisions.
        """
        start_time = time.perf_counter()

        # Truncate content before sanitization to avoid "Content too long" errors
        max_content_length = self._input_sanitizer.config.max_content_length
        truncated_title, truncated_content = self._truncate_content(title, content, max_content_length)

        sanitization_result = self._input_sanitizer.sanitize(truncated_title, truncated_content)

        if not sanitization_result.is_valid or sanitization_result.sanitized_input is None:
            # Log rejected article information for debugging (debug level to reduce log noise)
            title_preview = title[:100] if len(title) > 100 else title
            content_preview = content[:100] if len(content) > 100 else content
            logger.debug(
                "Input sanitization failed",
                violations=sanitization_result.violations,
                title_preview=title_preview,
                content_preview=content_preview,
                title_length=len(title),
                content_length=len(content),
            )
            return TagExtractionOutcome(
                tags=[],
                confidence=0.0,
                tag_count=0,
                inference_ms=0.0,
                language="und",
                model_name=self.config.model_name,
                sanitized_length=0,
                embedding_backend=self._embedding_backend,
                embedding_metadata=self._embedding_metadata,
            )

        sanitized_input = sanitization_result.sanitized_input
        raw_text = f"{sanitized_input.title}\n{sanitized_input.content}".strip()
        sanitized_length = sanitized_input.sanitized_length

        if len(raw_text) < self.config.min_text_length:
            logger.info("Sanitized input too short for extraction", char_count=len(raw_text))
            return TagExtractionOutcome(
                tags=[],
                confidence=0.0,
                tag_count=0,
                inference_ms=0.0,
                language="und",
                model_name=self.config.model_name,
                sanitized_length=sanitized_length,
                embedding_backend=self._embedding_backend,
                embedding_metadata=self._embedding_metadata,
            )

        logger.debug(
            "Processing sanitized text",
            char_count=len(raw_text),
            original_length=sanitized_input.original_length,
            sanitized_length=sanitized_length,
        )

        lang = self._detect_language(raw_text)
        logger.debug("Detected language", lang=lang)
        tags, tag_confidences = self._run_extraction(raw_text, lang)
        inference_ms = (time.perf_counter() - start_time) * 1000

        confidence = self._compute_confidence(tags, sanitized_length)
        outcome = TagExtractionOutcome(
            tags=tags,
            confidence=confidence,
            tag_count=len(tags),
            inference_ms=inference_ms,
            language=lang,
            model_name=self.config.model_name,
            sanitized_length=sanitized_length,
            embedding_backend=self._embedding_backend,
            tag_confidences=tag_confidences,
            embedding_metadata=self._embedding_metadata,
        )

        logger.debug(
            "Tag extraction metrics",
            tag_count=outcome.tag_count,
            inference_ms=round(outcome.inference_ms, 2),
            language=outcome.language,
        )
        return outcome

    def _lazy_load_models(self) -> None:
        """Lazy load models using the singleton model manager."""
        if not self._models_loaded:
            model_config = ModelConfig(
                model_name=self.config.model_name,
                device=self.config.device,
                use_onnx=self.config.use_onnx_runtime,
                onnx_model_path=self.config.onnx_model_path,
                onnx_tokenizer_name=self.config.onnx_tokenizer_name,
                onnx_pooling=self.config.onnx_pooling,
                onnx_batch_size=self.config.onnx_batch_size,
                onnx_max_length=self.config.onnx_max_length,
                use_fp16=self.config.use_fp16,
            )
            self._embedder, self._keybert, self._ja_tagger = self._model_manager.get_models(model_config)
            self._models_loaded = True
            runtime_metadata = self._model_manager.get_runtime_metadata()
            self._embedding_backend = runtime_metadata.get("embedder_backend", "unknown")
            self._embedding_metadata = runtime_metadata.get("embedder_metadata", {})
            logger.info(
                "Models loaded via ModelManager",
                embedder_backend=self._embedding_backend,
                embedder_class=type(self._embedder).__name__,
                fp16_enabled=self.config.use_fp16,
            )

    def _load_stopwords(self) -> None:
        """Load stopwords using the model manager."""
        if not hasattr(self, "_stopwords_loaded"):
            self._ja_stopwords, self._en_stopwords = self._model_manager.get_stopwords()
            self._stopwords_loaded = True

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
            # Keep English words in Japanese text as-is
            return normalized
        else:
            return text.lower()

    def _extract_compound_nouns_fugashi(self, text: str) -> list[str]:
        """
        Extract compound nouns by chaining consecutive noun tokens.

        This method identifies sequences of consecutive nouns and joins them
        to form compound nouns, which are more meaningful as tags than
        individual morphemes.

        Args:
            text: Input Japanese text

        Returns:
            List of compound nouns (2+ consecutive nouns joined)
        """
        self._lazy_load_models()

        if self._ja_tagger is None:
            raise RuntimeError("Japanese tagger not initialized")

        parsed = list(self._ja_tagger(text))
        compounds: list[str] = []
        current_compound: list[str] = []

        # POS tags that should be included in compound nouns
        noun_pos_tags = {
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
        connector_surfaces = {"の", "・", "＝", "－", "-"}

        for i, token in enumerate(parsed):
            pos1 = token.feature.pos1
            surface = token.surface

            # Check if token is a noun
            is_noun = pos1 in noun_pos_tags or pos1.startswith("名詞")

            # Check if it's a connector that might join nouns
            is_connector = surface in connector_surfaces

            if is_noun:
                current_compound.append(surface)
            elif is_connector and current_compound:
                # Check if next token is also a noun
                if i + 1 < len(parsed):
                    next_pos = parsed[i + 1].feature.pos1
                    next_is_noun = next_pos in noun_pos_tags or next_pos.startswith("名詞")
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

    def _extract_compound_japanese_words(self, text: str) -> list[str]:
        """Extract compound words and important phrases from Japanese text."""
        self._lazy_load_models()
        compound_words = []

        # Phase 1: Extract compounds using consecutive noun chaining
        chained_compounds = self._extract_compound_nouns_fugashi(text)
        compound_words.extend(chained_compounds)

        # Phase 2: Regex patterns for mixed-script and special compounds
        patterns = [
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

        for pattern in patterns:
            matches = re.findall(pattern, text)
            compound_words.extend(matches)

        # Phase 3: Use fugashi for proper noun sequence extraction
        if self._ja_tagger is None:
            raise RuntimeError("Japanese tagger not initialized")
        parsed = list(self._ja_tagger(text))
        i = 0
        while i < len(parsed):
            if parsed[i].feature.pos1 in self.config.japanese_pos_tags:
                # Check if it's a proper noun or organization
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
        seen: set[str] = set()
        unique_compounds: list[str] = []
        for word in compound_words:
            word_normalized = word.strip()
            if word_normalized not in seen and 2 <= len(word_normalized) <= 30:
                seen.add(word_normalized)
                unique_compounds.append(word_normalized)

        return unique_compounds

    def _extract_keywords_japanese(self, text: str) -> tuple[list[str], dict[str, float]]:
        """Extract keywords specifically for Japanese text.

        This method combines morphological analysis with optional semantic scoring:
        1. Extract compound nouns and single nouns using Fugashi
        2. Score candidates using frequency
        3. Optionally re-score using KeyBERT semantic similarity

        Returns:
            Tuple of (tag_list, tag_confidences_dict)
        """
        self._lazy_load_models()
        self._load_stopwords()

        # Extract compound words and important terms
        compounds = self._extract_compound_japanese_words(text)

        # Count frequencies
        term_freq = Counter(compounds)

        # Also extract single important nouns
        if self._ja_tagger is None:
            raise RuntimeError("Japanese tagger not initialized")
        single_nouns = []
        for word in self._ja_tagger(text):
            if (
                word.feature.pos1 in self.config.japanese_pos_tags
                and 2 <= len(word.surface) <= 10
                and word.surface not in self._ja_stopwords
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

        # Filter candidates by frequency or length
        candidates = [
            term
            for term, freq in combined_freq.most_common(self.config.top_keywords * 3)
            if freq >= 2 or len(term) >= 4
        ]

        if not candidates:
            return [], {}

        # Try semantic scoring with KeyBERT
        if self.config.use_japanese_semantic and self._keybert is not None and len(candidates) >= 2:
            try:
                return self._score_japanese_candidates_with_keybert(text, candidates, combined_freq)
            except Exception as e:
                logger.warning("Japanese semantic scoring failed, falling back to frequency", error=str(e))

        # Fallback: frequency-based scoring
        return self._score_candidates_by_frequency(candidates, combined_freq)

    def _score_japanese_candidates_with_keybert(
        self, text: str, candidates: list[str], freq_counter: Counter[str]
    ) -> tuple[list[str], dict[str, float]]:
        """
        Score Japanese keyword candidates using KeyBERT semantic similarity.

        This uses the KeyBERT's candidates parameter to score pre-extracted
        noun phrases against the document embedding, combining semantic
        relevance with frequency information.

        Args:
            text: Original document text
            candidates: List of candidate keywords from morphological analysis
            freq_counter: Frequency counts for each candidate

        Returns:
            Tuple of (tag_list, tag_confidences_dict)
        """
        if self._keybert is None:
            raise RuntimeError("KeyBERT not initialized")

        # Use KeyBERT with candidate list for semantic scoring
        keywords = self._keybert.extract_keywords(
            text,
            candidates=candidates,
            top_n=min(len(candidates), self.config.top_keywords * 2),
            use_mmr=self.config.use_mmr,
            diversity=self.config.japanese_mmr_diversity,
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

            if len(result) >= self.config.top_keywords:
                break

        logger.debug(
            "Japanese KeyBERT scoring completed",
            candidates_count=len(candidates),
            result_count=len(result),
        )

        return result, tag_confidences

    def _score_candidates_by_frequency(
        self, candidates: list[str], freq_counter: Counter[str]
    ) -> tuple[list[str], dict[str, float]]:
        """
        Score candidates using frequency-based ranking.

        This is the fallback method when semantic scoring is unavailable.

        Args:
            candidates: List of candidate keywords
            freq_counter: Frequency counts for each candidate

        Returns:
            Tuple of (tag_list, tag_confidences_dict)
        """
        # Sort by frequency
        sorted_candidates = sorted(candidates, key=lambda x: -freq_counter.get(x, 0))

        result = sorted_candidates[: self.config.top_keywords]
        max_freq = max(freq_counter.values()) if freq_counter else 1

        tag_confidences = {tag: round(freq_counter.get(tag, 1) / max_freq, 3) for tag in result}

        return result, tag_confidences

    def _extract_keywords_english(self, text: str) -> tuple[list[str], dict[str, float]]:
        """Extract keywords specifically for English text using KeyBERT.

        Returns:
            Tuple of (tag_list, tag_confidences_dict)
        """
        self._lazy_load_models()

        if self._keybert is None:
            raise RuntimeError("KeyBERT not initialized")

        try:
            # First extract both single words and phrases
            single_keywords = self._keybert.extract_keywords(
                text,
                keyphrase_ngram_range=(1, 1),  # Single words only
                top_n=self.config.top_keywords * 3,
                use_mmr=True,
                diversity=0.3,
            )

            phrase_keywords = self._keybert.extract_keywords(
                text,
                keyphrase_ngram_range=(2, 3),  # Phrases only
                top_n=self.config.top_keywords,
                use_mmr=True,
                diversity=0.5,
            )

            # Combine and process keywords
            all_keywords = []
            seen_words = set()

            # Process phrases first to identify important compound terms
            for phrase_tuple in cast(list[tuple[str, float]], phrase_keywords):
                phrase = phrase_tuple[0].strip().lower()
                score = phrase_tuple[1]
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
            for word_tuple in cast(list[tuple[str, float]], single_keywords):
                word = word_tuple[0].strip().lower()
                score = word_tuple[1]
                if score >= self.config.min_score_threshold and word not in seen_words:
                    # Skip generic words
                    if len(word) > 2 and not word.isdigit():
                        all_keywords.append((word, score))
                        seen_words.add(word)

            # Sort by score and filter
            all_keywords.sort(key=lambda x: x[1], reverse=True)

            # Final filtering and cleaning
            result = []
            tag_confidences: dict[str, float] = {}
            seen_final: set[str] = set()

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
                        # Normalize score to 0.0-1.0 range and store
                        normalized_score = min(max(score, 0.0), 1.0)
                        tag_confidences[keyword_clean] = round(normalized_score, 3)
                        seen_final.add(keyword_lower)

                        if len(result) >= self.config.top_keywords:
                            break

            return result, tag_confidences

        except Exception as e:
            logger.error("KeyBERT extraction failed for English", error=e)
            return [], {}

    def _tokenize_english(self, text: str) -> list[str]:
        """Tokenize English text using NLTK."""
        self._load_stopwords()
        tokens = nltk.word_tokenize(text)
        result = []

        for token in tokens:
            if re.fullmatch(r"\w+", token) and len(token) > self.config.min_token_length:
                normalized = self._normalize_text(token, "en")
                if normalized not in self._en_stopwords:
                    result.append(normalized)

        return result

    def _get_candidate_tokens(self, text: str) -> list[str]:
        """Get candidate tokens for fallback extraction (primarily English text).

        This helper exists mainly for clarity and testability and currently
        delegates to the English tokenizer.
        """
        return self._tokenize_english(text)

    def _fallback_extraction(self, text: str, lang: str) -> list[str]:
        """Fallback extraction method when primary method fails."""
        if lang == "ja":
            # For Japanese, use the frequency-based approach
            keywords, _ = self._extract_keywords_japanese(text)
            return keywords
        else:
            # For English, try tokenization and frequency
            tokens = self._get_candidate_tokens(text)
            if tokens:
                token_freq = Counter(tokens)
                return [term for term, _ in token_freq.most_common(self.config.top_keywords)]
            return []

    def extract_tags(self, title: str, content: str) -> list[str]:
        """
        Legacy compatibility wrapper that returns only the tag list.
        """
        return self.extract_tags_with_metrics(title, content).tags


# Maintain backward compatibility
def extract_tags(title: str, content: str) -> list[str]:
    """
    Legacy function for backward compatibility.
    Now includes input sanitization by default.

    Args:
        title: The title text
        content: The content text

    Returns:
        List of extracted tags
    """
    extractor = TagExtractor()
    return extractor.extract_tags(title, content)
