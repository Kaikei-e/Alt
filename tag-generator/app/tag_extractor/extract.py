"""Tag extraction orchestrator.

Dispatches to language-specific extractors after sanitization and language detection.
"""

import time
import unicodedata
from collections import Counter
from dataclasses import dataclass, field
from typing import Any

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
from .config import ModelConfig, TagExtractionConfig
from .english_extractor import (
    extract_keywords_english,
    fallback_english,
    get_candidate_tokens,
    tokenize_english,
)
from .input_sanitizer import InputSanitizer, SanitizationConfig
from .japanese_extractor import (
    extract_compound_japanese_words,
    extract_compound_nouns_fugashi,
    extract_keywords_japanese,
    make_japanese_analyzer,
    score_candidates_by_frequency,
    score_japanese_candidates_with_keybert,
)
from .model_manager import get_model_manager
from .tag_validator import is_valid_japanese_tag as _shared_is_valid_japanese_tag

logger = structlog.get_logger(__name__)


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
        model_manager: Any | None = None,
    ):
        self.config = config or TagExtractionConfig()
        self._model_manager = model_manager or get_model_manager()
        self._models_loaded = False
        self._input_sanitizer = InputSanitizer(sanitizer_config)
        self._embedding_backend: str = "unknown"
        self._embedding_metadata: dict[str, Any] = {}
        # Lazily populated model handles (set by _lazy_load_models via ModelManager)
        self._embedder: Any | None = None
        self._keybert: Any | None = None
        self._ja_tagger: Any | None = None

    # ------------------------------------------------------------------
    # Public API
    # ------------------------------------------------------------------

    def extract_tags_with_metrics(self, title: str, content: str) -> TagExtractionOutcome:
        """Extract tags and capture metrics for cascade decisions."""
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

    # ------------------------------------------------------------------
    # Internal helpers
    # ------------------------------------------------------------------

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
        """Truncate content to fit within sanitization limits while preserving title."""
        max_title_length = 1000
        truncated_title = title[:max_title_length] if len(title) > max_title_length else title

        if len(content) > max_content_length:
            truncated_content = content[:max_content_length]
            last_sentence_end = max(
                truncated_content.rfind("."),
                truncated_content.rfind("!"),
                truncated_content.rfind("?"),
            )
            if last_sentence_end > max_content_length * 0.8:
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
            normalized = unicodedata.normalize("NFKC", text)
            return normalized
        else:
            return text.lower()

    def _is_valid_japanese_tag(self, tag: str) -> bool:
        """Filter out grammatically invalid or low-quality Japanese tags.

        Delegates to the shared tag validator for consistent
        filtering across all extractors.
        """
        return _shared_is_valid_japanese_tag(tag, max_length=self.config.max_tag_length)

    # ------------------------------------------------------------------
    # Language-specific extraction (delegated to submodules)
    # ------------------------------------------------------------------

    def _extract_compound_nouns_fugashi(self, text: str) -> list[str]:
        """Extract compound nouns by chaining consecutive noun tokens."""
        self._lazy_load_models()
        return extract_compound_nouns_fugashi(text, self._ja_tagger)

    def _extract_compound_japanese_words(self, text: str) -> list[str]:
        """Extract compound words and important phrases from Japanese text."""
        self._lazy_load_models()
        return extract_compound_japanese_words(text, self._ja_tagger, self.config)

    def _extract_keywords_japanese(self, text: str) -> tuple[list[str], dict[str, float]]:
        """Extract keywords specifically for Japanese text."""
        self._lazy_load_models()
        self._load_stopwords()
        return extract_keywords_japanese(text, self._ja_tagger, self._keybert, self._ja_stopwords, self.config)

    def _make_japanese_analyzer(self):
        """Create a custom analyzer for CountVectorizer that uses Fugashi."""
        return make_japanese_analyzer(self._ja_tagger)

    def _score_japanese_candidates_with_keybert(
        self, text: str, candidates: list[str], freq_counter: "Counter[str]"
    ) -> tuple[list[str], dict[str, float]]:
        """Score Japanese keyword candidates using KeyBERT semantic similarity."""
        return score_japanese_candidates_with_keybert(
            text, candidates, freq_counter, self._keybert, self._ja_tagger, self.config
        )

    def _score_candidates_by_frequency(
        self, candidates: list[str], freq_counter: "Counter[str]"
    ) -> tuple[list[str], dict[str, float]]:
        """Score candidates using frequency-based ranking."""
        return score_candidates_by_frequency(candidates, freq_counter, self.config.top_keywords)

    def _extract_keywords_english(self, text: str) -> tuple[list[str], dict[str, float]]:
        """Extract keywords specifically for English text using KeyBERT."""
        self._lazy_load_models()
        return extract_keywords_english(text, self._keybert, self.config)

    def _tokenize_english(self, text: str) -> list[str]:
        """Tokenize English text using NLTK."""
        self._load_stopwords()
        return tokenize_english(text, self._en_stopwords, self.config)

    def _get_candidate_tokens(self, text: str) -> list[str]:
        """Get candidate tokens for fallback extraction (primarily English text)."""
        self._load_stopwords()
        return get_candidate_tokens(text, self._en_stopwords, self.config)

    def _fallback_extraction(self, text: str, lang: str) -> list[str]:
        """Fallback extraction method when primary method fails."""
        if lang == "ja":
            # For Japanese, use the frequency-based approach
            keywords, _ = self._extract_keywords_japanese(text)
            return keywords
        else:
            # For English, try tokenization and frequency
            self._load_stopwords()
            return fallback_english(text, self._en_stopwords, self.config)
