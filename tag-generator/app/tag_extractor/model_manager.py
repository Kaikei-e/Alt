"""
Model manager for efficient model loading and sharing.
Implements singleton pattern for ML models to improve performance.
"""

import logging
import threading
from typing import Optional, Set
from dataclasses import dataclass

from sentence_transformers import SentenceTransformer
from keybert import KeyBERT
from fugashi import Tagger

logger = logging.getLogger(__name__)


@dataclass
class ModelConfig:
    """Configuration for model loading."""
    model_name: str = "paraphrase-multilingual-MiniLM-L12-v2"
    device: str = 'cpu'


class ModelManager:
    """
    Thread-safe singleton model manager for efficient model sharing.
    Ensures models are loaded only once and shared across all TagExtractor instances.
    """

    _instance: Optional['ModelManager'] = None
    _lock = threading.Lock()
    _models_lock = threading.Lock()

    def __new__(cls) -> 'ModelManager':
        """Ensure singleton instance."""
        if cls._instance is None:
            with cls._lock:
                if cls._instance is None:
                    cls._instance = super().__new__(cls)
                    cls._instance._initialized = False
        return cls._instance

    def __init__(self):
        """Initialize model manager (called only once)."""
        if not getattr(self, '_initialized', False):
            self._embedder: Optional[SentenceTransformer] = None
            self._keybert: Optional[KeyBERT] = None
            self._ja_tagger: Optional[Tagger] = None
            self._ja_stopwords: Optional[Set[str]] = None
            self._en_stopwords: Optional[Set[str]] = None
            self._config: Optional[ModelConfig] = None
            self._initialized = True
            logger.info("ModelManager singleton initialized")

    def get_models(self, config: ModelConfig) -> tuple[SentenceTransformer, KeyBERT, Tagger]:
        """
        Get or load models with thread-safe lazy loading.

        Args:
            config: Model configuration

        Returns:
            Tuple of (embedder, keybert, ja_tagger)
        """
        with self._models_lock:
            if (self._embedder is None or
                self._config is None or
                self._config.model_name != config.model_name or
                self._config.device != config.device):

                logger.info(f"Loading models with config: {config}")
                self._load_models(config)
                self._config = config

            return self._embedder, self._keybert, self._ja_tagger

    def get_stopwords(self) -> tuple[Set[str], Set[str]]:
        """
        Get or load stopwords with thread-safe lazy loading.

        Returns:
            Tuple of (ja_stopwords, en_stopwords)
        """
        with self._models_lock:
            if self._ja_stopwords is None or self._en_stopwords is None:
                self._load_stopwords()

            return self._ja_stopwords, self._en_stopwords

    def _load_models(self, config: ModelConfig) -> None:
        """Load ML models (called within lock)."""
        try:
            logger.info(f"Loading SentenceTransformer model: {config.model_name}")
            self._embedder = SentenceTransformer(
                config.model_name,
                device=config.device
            )

            logger.info("Loading KeyBERT model")
            self._keybert = KeyBERT(self._embedder)

            logger.info("Loading Japanese tagger")
            self._ja_tagger = Tagger()

            logger.info("Models loaded successfully")

        except Exception as e:
            logger.error(f"Failed to load models: {e}")
            # Reset to None so we can retry
            self._embedder = None
            self._keybert = None
            self._ja_tagger = None
            raise

    def _load_stopwords(self) -> None:
        """Load stopwords files (called within lock)."""
        import os
        import nltk

        current_dir = os.path.dirname(os.path.dirname(__file__))
        ja_stopwords_path = os.path.join(current_dir, "tag_extractor", "stopwords_ja.txt")
        en_stopwords_path = os.path.join(current_dir, "tag_extractor", "stopwords_en.txt")

        # Load Japanese stopwords
        try:
            with open(ja_stopwords_path, 'r', encoding='utf-8') as f:
                self._ja_stopwords = set(line.strip() for line in f if line.strip())
            logger.info(f"Loaded {len(self._ja_stopwords)} Japanese stopwords")
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
            logger.info(f"Loaded {len(self._en_stopwords)} English stopwords")
        except Exception as e:
            logger.warning(f"Could not load NLTK English stopwords: {e}")

    def is_loaded(self) -> bool:
        """Check if models are loaded."""
        with self._models_lock:
            return (self._embedder is not None and
                   self._keybert is not None and
                   self._ja_tagger is not None)

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