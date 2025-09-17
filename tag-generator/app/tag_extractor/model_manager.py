"""
Model manager for efficient model loading and sharing.
Implements singleton pattern for ML models to improve performance.
"""

import threading
from dataclasses import dataclass
from typing import TYPE_CHECKING, Any, Optional

if TYPE_CHECKING:
    from keybert import KeyBERT  # type: ignore
    from sentence_transformers import SentenceTransformer  # type: ignore

try:
    from fugashi import Tagger  # pyright: ignore
    from keybert import KeyBERT  # type: ignore
    from sentence_transformers import SentenceTransformer  # type: ignore
except ImportError:
    # Fallback for environments without ML dependencies (e.g., production builds)
    # These will be mocked in tests
    SentenceTransformer = None  # type: ignore
    KeyBERT = None  # type: ignore
    Tagger = None  # type: ignore
import structlog

logger = structlog.get_logger(__name__)


@dataclass
class ModelConfig:
    """Configuration for model loading."""

    model_name: str = "paraphrase-multilingual-MiniLM-L12-v2"
    device: str = "cpu"


class ModelManager:
    """
    Thread-safe singleton model manager for efficient model sharing.
    Ensures models are loaded only once and shared across all TagExtractor instances.
    """

    _instance: Optional["ModelManager"] = None
    _lock = threading.Lock()
    _models_lock = threading.Lock()

    def __new__(cls) -> "ModelManager":
        """Ensure singleton instance."""
        if cls._instance is None:
            with cls._lock:
                if cls._instance is None:
                    cls._instance = super().__new__(cls)
                    cls._instance._initialized = False
        return cls._instance

    def __init__(self):
        """Initialize model manager (called only once)."""
        if not getattr(self, "_initialized", False):
            self._embedder: Any = None
            self._keybert: Any = None
            self._ja_tagger: Any = None
            self._ja_stopwords: set[str] | None = None
            self._en_stopwords: set[str] | None = None
            self._config: ModelConfig | None = None
            self._initialized = True
            logger.info("ModelManager singleton initialized")

    def get_models(self, config: ModelConfig) -> tuple[Any, Any, Any]:
        """
        Get or load models with thread-safe lazy loading.

        Args:
            config: Model configuration

        Returns:
            Tuple of (embedder, keybert, ja_tagger)
        """
        with self._models_lock:
            if (
                self._embedder is None
                or self._config is None
                or self._config.model_name != config.model_name
                or self._config.device != config.device
            ):
                logger.info("Loading models", config=config)
                self._load_models(config)
                self._config = config

            assert self._embedder is not None
            assert self._keybert is not None
            assert self._ja_tagger is not None
            return self._embedder, self._keybert, self._ja_tagger

    def get_stopwords(self) -> tuple[set[str], set[str]]:
        """
        Get or load stopwords with thread-safe lazy loading.

        Returns:
            Tuple of (ja_stopwords, en_stopwords)
        """
        with self._models_lock:
            if self._ja_stopwords is None or self._en_stopwords is None:
                self._load_stopwords()

            assert self._ja_stopwords is not None
            assert self._en_stopwords is not None
            return self._ja_stopwords, self._en_stopwords

    def _load_models(self, config: ModelConfig) -> None:
        """Load ML models (called within lock)."""
        try:
            if SentenceTransformer is None:
                raise ImportError("SentenceTransformer not available")
            if KeyBERT is None:
                raise ImportError("KeyBERT not available")
            if Tagger is None:
                raise ImportError("Tagger not available")

            logger.info("Loading SentenceTransformer model", model_name=config.model_name)
            self._embedder = SentenceTransformer(config.model_name, device=config.device)

            logger.info("Loading KeyBERT model")
            self._keybert = KeyBERT(self._embedder)  # pyright: ignore[reportArgumentType]

            logger.info("Loading Japanese tagger")
            self._ja_tagger = Tagger()

            logger.info("Models loaded successfully")

        except Exception as e:
            logger.error("Failed to load models", error=e)
            # Reset to None so we can retry
            self._embedder = None
            self._keybert = None
            self._ja_tagger = None

    def _load_stopwords(self) -> None:
        """Load stopwords files (called within lock)."""
        import os

        import nltk

        current_dir = os.path.dirname(os.path.dirname(__file__))
        ja_stopwords_path = os.path.join(current_dir, "tag_extractor", "stopwords_ja.txt")
        en_stopwords_path = os.path.join(current_dir, "tag_extractor", "stopwords_en.txt")

        # Load Japanese stopwords
        try:
            with open(ja_stopwords_path, encoding="utf-8") as f:
                self._ja_stopwords = {line.strip() for line in f if line.strip()}
            logger.info("Loaded Japanese stopwords", count=len(self._ja_stopwords))
        except FileNotFoundError:
            logger.warning("Japanese stopwords file not found", path=ja_stopwords_path)
            self._ja_stopwords = set()

        # Load English stopwords
        try:
            with open(en_stopwords_path, encoding="utf-8") as f:
                self._en_stopwords = {line.strip().lower() for line in f if line.strip()}
        except FileNotFoundError:
            logger.warning("English stopwords file not found", path=en_stopwords_path)
            self._en_stopwords = set()

        # Add NLTK English stopwords
        if self._en_stopwords is None:
            self._en_stopwords = set()
        try:
            self._en_stopwords.update(set(nltk.corpus.stopwords.words("english")))
            logger.info("Loaded English stopwords", count=len(self._en_stopwords))
        except Exception as e:
            logger.warning("Could not load NLTK English stopwords", error=e)

    def is_loaded(self) -> bool:
        """Check if models are loaded."""
        with self._models_lock:
            return self._embedder is not None and self._keybert is not None and self._ja_tagger is not None

    def clear_models(self) -> None:
        """Clear loaded models (for testing)."""
        with self._models_lock:
            self._embedder = None
            self._keybert = None
            self._ja_tagger = None
            self._ja_stopwords = None
            self._en_stopwords = None
            self._config = None
            logger.info("Models cleared")


def get_model_manager() -> ModelManager:
    """Get the singleton model manager instance."""
    return ModelManager()
