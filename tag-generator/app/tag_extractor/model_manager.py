"""
Model manager for efficient model loading and sharing.
Implements singleton pattern for ML models to improve performance.
"""

import threading
from dataclasses import dataclass
from typing import TYPE_CHECKING, Any, Optional

from tag_extractor.onnx_embedder import (
    OnnxEmbeddingConfig,
    OnnxEmbeddingModel,
    OnnxRuntimeMissing,
)

if TYPE_CHECKING:
    from keybert import KeyBERT  # type: ignore
    from sentence_transformers import SentenceTransformer  # type: ignore

try:
    from fugashi import Tagger  # pyright: ignore
    from keybert import KeyBERT  # type: ignore
    from sentence_transformers import SentenceTransformer  # type: ignore
except ImportError:
    # Fallback for environments without ML dependencies (e.g., production builds)
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
    use_onnx: bool = False
    onnx_model_path: str | None = None
    onnx_tokenizer_name: str = "sentence-transformers/paraphrase-multilingual-MiniLM-L12-v2"
    onnx_pooling: str = "cls"
    onnx_batch_size: int = 16
    onnx_max_length: int = 256
    use_fp16: bool = False  # Enable FP16 for ~50% memory reduction (GPU recommended)


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
            self._embedder_backend: str | None = None
            self._embedder_metadata: dict[str, Any] = {}
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
            if self._embedder is None or self._config is None or self._config != config:
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
            self._embedder_backend = None
            self._embedder_metadata = {}
            if Tagger is None:
                logger.error(
                    "Tagger (fugashi) not available",
                    help="Install with: pip install fugashi[unidic-lite]",
                )
                raise ImportError("Tagger not available. Install with: pip install fugashi[unidic-lite]")

            use_onnx = config.use_onnx
            if config.use_onnx and not config.onnx_model_path:
                logger.warning(
                    "ONNX runtime is enabled but onnx_model_path is not configured; falling back to SentenceTransformer"
                )
                use_onnx = False

            if use_onnx:
                if not config.onnx_model_path:
                    logger.warning("ONNX model path is not set; falling back to SentenceTransformer")
                    use_onnx = False
                else:
                    try:
                        logger.info("Loading ONNX embedder", model_path=config.onnx_model_path)
                        onnx_config = OnnxEmbeddingConfig(
                            model_path=config.onnx_model_path,
                            tokenizer_name=config.onnx_tokenizer_name,
                            pooling=config.onnx_pooling,
                            batch_size=config.onnx_batch_size,
                            max_length=config.onnx_max_length,
                        )
                        self._embedder = OnnxEmbeddingModel(onnx_config)
                        logger.info("ONNX embedder loaded successfully", model_path=config.onnx_model_path)
                        self._embedder_backend = "onnx"
                        self._embedder_metadata = self._embedder.describe()
                    except OnnxRuntimeMissing as e:
                        logger.error("ONNX runtime dependencies missing", error=str(e))
                        use_onnx = False
                        logger.info("Falling back to SentenceTransformer")
                    except Exception as e:
                        logger.error("Failed to initialize ONNX embedder", error=str(e))
                        use_onnx = False
                        logger.info("Falling back to SentenceTransformer due to error")

            if not use_onnx:
                if SentenceTransformer is None or KeyBERT is None:
                    logger.error("SentenceTransformer/KeyBERT dependencies missing")
                    raise ImportError("SentenceTransformer and KeyBERT are required")

                logger.info("Loading SentenceTransformer model", model_name=config.model_name)
                self._embedder = SentenceTransformer(config.model_name, device=config.device)
                logger.info("SentenceTransformer loaded successfully")

                # Apply FP16 conversion for memory optimization (~50% reduction)
                fp16_applied = False
                if config.use_fp16:
                    try:
                        self._embedder.half()
                        fp16_applied = True
                        logger.info(
                            "FP16 conversion applied to SentenceTransformer",
                            device=config.device,
                            expected_memory_reduction="~50%",
                        )
                    except Exception as fp16_error:
                        logger.warning(
                            "FP16 conversion failed, continuing with FP32",
                            error=str(fp16_error),
                        )

                self._embedder_backend = "sentence_transformer"
                embedding_dim = None
                if hasattr(self._embedder, "get_sentence_embedding_dimension"):
                    try:
                        embedding_dim = self._embedder.get_sentence_embedding_dimension()  # type: ignore[attr-defined]
                    except Exception:  # pragma: no cover - best effort metadata
                        embedding_dim = None
                self._embedder_metadata = {
                    "backend": "sentence_transformer",
                    "model_name": config.model_name,
                    "device": config.device,
                    "fp16": fp16_applied,
                }
                if embedding_dim is not None:
                    self._embedder_metadata["embedding_dimension"] = embedding_dim

            if self._embedder is None:
                raise RuntimeError("Embedder was not initialized")
            logger.info("Loading KeyBERT model")
            self._keybert = KeyBERT(self._embedder)  # pyright: ignore[reportArgumentType,reportOptionalCall]
            logger.info(
                "KeyBERT loaded successfully",
                embedder_backend=self._embedder_backend,
                embedder_class=type(self._embedder).__name__,
            )

            logger.info("Loading Japanese tagger")
            self._ja_tagger = Tagger()
            logger.info("Japanese tagger loaded successfully")

            logger.info("All models loaded successfully")

        except ImportError as e:
            logger.error("Import error while loading models", error=str(e))
            # Reset to None so we can retry
            self._embedder = None
            self._keybert = None
            self._ja_tagger = None
            self._embedder_backend = None
            self._embedder_metadata = {}
            # Re-raise the original exception
            raise
        except Exception as e:
            logger.error(
                "Unexpected error while loading models",
                error=str(e),
                error_type=type(e).__name__,
            )
            # Reset to None so we can retry
            self._embedder = None
            self._keybert = None
            self._ja_tagger = None
            self._embedder_backend = None
            self._embedder_metadata = {}
            # Re-raise the original exception
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
            self._embedder_backend = None
            self._embedder_metadata = {}
            logger.info("Models cleared")

    def get_runtime_metadata(self) -> dict[str, Any]:
        """Expose runtime metadata about the currently loaded embedder."""
        with self._models_lock:
            backend = self._embedder_backend or "unknown"
            metadata = dict(self._embedder_metadata or {})
            return {
                "embedder_backend": backend,
                "embedder_metadata": metadata,
            }


def get_model_manager() -> ModelManager:
    """Get the singleton model manager instance."""
    return ModelManager()
