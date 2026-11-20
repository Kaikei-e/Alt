"""
Model manager for efficient model loading and sharing.
Implements singleton pattern for ML models to improve performance.
"""

import threading
from pathlib import Path
from typing import TYPE_CHECKING, Any

import nltk
import structlog

from tag_extractor.config import ModelConfig
from tag_extractor.onnx_embedder import (
    OnnxEmbeddingConfig,
    OnnxEmbeddingModel,
    OnnxRuntimeMissing,
)
from tag_generator.exceptions import ModelLoadError

logger = structlog.get_logger(__name__)

if TYPE_CHECKING:
    from keybert import KeyBERT  # type: ignore
    from sentence_transformers import SentenceTransformer  # type: ignore
    from spacy.language import Language  # type: ignore


class _ModelManagerRuntimeError(RuntimeError):
    @classmethod
    def models_failed(cls) -> _ModelManagerRuntimeError:
        return cls("Models failed to load")

    @classmethod
    def stopwords_failed(cls) -> _ModelManagerRuntimeError:
        return cls("Stopwords failed to load")


class _MissingMLDependencyError(ImportError):
    @classmethod
    def tagger_missing(cls) -> _MissingMLDependencyError:
        return cls("Tagger not available. Install with: pip install fugashi[unidic-lite]")

    @classmethod
    def sentence_transformer_missing(cls) -> _MissingMLDependencyError:
        return cls("SentenceTransformer and KeyBERT are required")

    @classmethod
    def keybert_missing(cls) -> _MissingMLDependencyError:
        return cls("KeyBERT is required. Install with: pip install keybert")


class _ModelManagerLoadError(ModelLoadError):
    @classmethod
    def embedder_uninitialized(cls) -> _ModelManagerLoadError:
        return cls("Embedder was not initialized")

    @classmethod
    def missing_dependency(cls, exc: BaseException) -> _ModelManagerLoadError:
        return cls(f"Missing ML dependency: {exc}")

    @classmethod
    def load_failed(cls, exc: BaseException) -> _ModelManagerLoadError:
        return cls(f"Failed to load models: {exc}")

    @classmethod
    def stopwords_failed(cls, exc: BaseException) -> _ModelManagerLoadError:
        return cls(f"Failed to load stopwords: {exc}")


try:
    from fugashi import Tagger  # pyright: ignore
    from keybert import KeyBERT  # type: ignore
    from sentence_transformers import SentenceTransformer  # type: ignore

    logger.info("ml_dependencies_enabled", detail="fugashi/keybert/sentence-transformers importable")
except ImportError as exc:
    # No local fallback model exists: get_models() raises ModelLoadError the
    # moment code tries to actually use these (see below), so this is loud
    # rather than a silent no-op.
    logger.warning(
        "ml_dependencies_disabled",
        detail="fugashi/keybert/sentence-transformers not installed; model loading will raise ModelLoadError",
        error=str(exc),
    )
    SentenceTransformer = None  # type: ignore
    KeyBERT = None  # type: ignore
    Tagger = None  # type: ignore


class ModelManager:
    """
    Thread-safe model manager for efficient model sharing.
    Use ``get_model_manager()`` to obtain the shared application-level instance.
    """

    def __init__(self) -> None:
        self._models_lock = threading.Lock()
        self._embedder: Any = None
        self._keybert: Any = None
        self._ja_tagger: Any = None
        self._ginza_nlp: Any = None  # GiNZA spaCy model
        self._ja_stopwords: set[str] | None = None
        self._en_stopwords: set[str] | None = None
        self._config: ModelConfig | None = None
        self._embedder_backend: str | None = None
        self._embedder_metadata: dict[str, Any] = {}
        self._ginza_available: bool | None = None
        logger.info("ModelManager initialized")

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

            if self._embedder is None or self._keybert is None or self._ja_tagger is None:
                raise _ModelManagerRuntimeError.models_failed()
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

            if self._ja_stopwords is None or self._en_stopwords is None:
                raise _ModelManagerRuntimeError.stopwords_failed()
            return self._ja_stopwords, self._en_stopwords

    def _reset_loaded_models(self) -> None:
        self._embedder = None
        self._keybert = None
        self._ja_tagger = None
        self._embedder_backend = None
        self._embedder_metadata = {}

    def _try_load_onnx_embedder(self, config: ModelConfig) -> bool:
        if not config.use_onnx:
            return False
        if not config.onnx_model_path:
            logger.warning(
                "ONNX runtime is enabled but onnx_model_path is not configured; falling back to SentenceTransformer"
            )
            return False
        try:
            logger.info("Loading ONNX embedder", model_path=config.onnx_model_path)
            onnx_config = OnnxEmbeddingConfig(
                model_path=config.onnx_model_path,
                tokenizer_name=config.onnx_tokenizer_name,
                tokenizer_revision=config.onnx_tokenizer_revision,
                pooling=config.onnx_pooling,
                batch_size=config.onnx_batch_size,
                max_length=config.onnx_max_length,
            )
            self._embedder = OnnxEmbeddingModel(onnx_config)
            logger.info("ONNX embedder loaded successfully", model_path=config.onnx_model_path)
            self._embedder_backend = "onnx"
            self._embedder_metadata = self._embedder.describe()
        except OnnxRuntimeMissing as e:
            logger.exception("ONNX runtime dependencies missing", error=str(e))
            logger.info("Falling back to SentenceTransformer")
            return False
        except Exception as e:
            logger.exception("Failed to initialize ONNX embedder", error=str(e))
            logger.info("Falling back to SentenceTransformer due to error")
            return False
        else:
            return True

    def _load_sentence_transformer_embedder(self, config: ModelConfig) -> None:
        if SentenceTransformer is None or KeyBERT is None:
            logger.error("SentenceTransformer/KeyBERT dependencies missing")
            raise _MissingMLDependencyError.sentence_transformer_missing()

        logger.info("Loading SentenceTransformer model", model_name=config.model_name)
        self._embedder = SentenceTransformer(config.model_name, device=config.device, revision=config.model_revision)
        logger.info("SentenceTransformer loaded successfully")

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
            except (AttributeError, RuntimeError, TypeError) as fp16_error:
                logger.warning("FP16 conversion failed, continuing with FP32", error=str(fp16_error))

        self._embedder_backend = "sentence_transformer"
        embedding_dim = None
        if hasattr(self._embedder, "get_sentence_embedding_dimension"):
            try:
                embedding_dim = self._embedder.get_sentence_embedding_dimension()  # type: ignore[attr-defined]
            except AttributeError, RuntimeError, TypeError:  # pragma: no cover - best effort metadata
                embedding_dim = None
        self._embedder_metadata = {
            "backend": "sentence_transformer",
            "model_name": config.model_name,
            "device": config.device,
            "fp16": fp16_applied,
        }
        if embedding_dim is not None:
            self._embedder_metadata["embedding_dimension"] = embedding_dim

    def _load_models_impl(self, config: ModelConfig) -> None:
        self._embedder_backend = None
        self._embedder_metadata = {}
        if Tagger is None:
            logger.error(
                "Tagger (fugashi) not available",
                help="Install with: pip install fugashi[unidic-lite]",
            )
            raise _MissingMLDependencyError.tagger_missing()

        if not self._try_load_onnx_embedder(config):
            self._load_sentence_transformer_embedder(config)

        if self._embedder is None:
            raise _ModelManagerLoadError.embedder_uninitialized()
        if KeyBERT is None:
            logger.error("KeyBERT dependency missing")
            raise _MissingMLDependencyError.keybert_missing()
        logger.info("Loading KeyBERT model")
        self._keybert = KeyBERT(self._embedder)
        logger.info(
            "KeyBERT loaded successfully",
            embedder_backend=self._embedder_backend,
            embedder_class=type(self._embedder).__name__,
        )

        logger.info("Loading Japanese tagger")
        self._ja_tagger = Tagger()
        logger.info("Japanese tagger loaded successfully")

        if config.use_ginza:
            self._load_ginza(config.ginza_model_name)

        logger.info("All models loaded successfully")

    def _load_models(self, config: ModelConfig) -> None:
        """Load ML models (called within lock)."""
        try:
            self._load_models_impl(config)
        except ImportError as e:
            logger.exception("Import error while loading models", error=str(e))
            self._reset_loaded_models()
            raise _ModelManagerLoadError.missing_dependency(e) from e
        except ModelLoadError:
            self._reset_loaded_models()
            raise
        except Exception as e:
            logger.exception(
                "Unexpected error while loading models",
                error=str(e),
                error_type=type(e).__name__,
            )
            self._reset_loaded_models()
            raise _ModelManagerLoadError.load_failed(e) from e

    def _load_stopwords(self) -> None:
        """Load stopwords files and NLTK stopwords corpus (fail-fast within lock)."""
        current_dir = Path(__file__).resolve().parent.parent
        ja_stopwords_path = current_dir / "tag_extractor" / "stopwords_ja.txt"
        en_stopwords_path = current_dir / "tag_extractor" / "stopwords_en.txt"

        try:
            with ja_stopwords_path.open(encoding="utf-8") as f:
                ja_stopwords = {stripped for line in f if (stripped := line.strip()) and not stripped.startswith("#")}

            with en_stopwords_path.open(encoding="utf-8") as f:
                en_stopwords = {
                    stripped.lower() for line in f if (stripped := line.strip()) and not stripped.startswith("#")
                }

            en_stopwords.update(word.lower() for word in nltk.corpus.stopwords.words("english"))
        except Exception as e:
            logger.exception("Failed to load required stopword resources", error=str(e), error_type=type(e).__name__)
            raise _ModelManagerLoadError.stopwords_failed(e) from e

        self._ja_stopwords = ja_stopwords
        self._en_stopwords = en_stopwords
        logger.info(
            "Loaded stopwords",
            ja_count=len(self._ja_stopwords),
            en_count=len(self._en_stopwords),
        )

    def _load_ginza(self, model_name: str) -> None:
        """Load GiNZA spaCy model for Japanese NLP."""
        try:
            import spacy  # noqa: PLC0415 - optional ml-extended dependency loaded only when use_ginza=True

            logger.info("Loading GiNZA model", model_name=model_name)
            self._ginza_nlp = spacy.load(model_name)
            self._ginza_available = True
            logger.info(
                "GiNZA model loaded successfully",
                model_name=model_name,
                pipeline=self._ginza_nlp.pipe_names if self._ginza_nlp else [],
            )
        except ImportError as e:
            logger.warning(
                "GiNZA/spaCy not available",
                error=str(e),
                help="Install with: uv sync --group ml-extended",
            )
            self._ginza_available = False
        except OSError as e:
            logger.warning(
                "GiNZA model not found",
                model_name=model_name,
                error=str(e),
                help="Install model with: python -m spacy download ja_ginza",
            )
            self._ginza_available = False

    def get_ginza(self) -> Language | None:
        """
        Get the GiNZA spaCy model if available.

        Returns:
            spaCy Language model or None if not available
        """
        with self._models_lock:
            return self._ginza_nlp

    def is_ginza_available(self) -> bool:
        """Check if GiNZA model is available."""
        with self._models_lock:
            return self._ginza_available is True

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
            self._ginza_nlp = None
            self._ja_stopwords = None
            self._en_stopwords = None
            self._config = None
            self._embedder_backend = None
            self._embedder_metadata = {}
            self._ginza_available = None
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


_default_instance_holder: list[ModelManager | None] = [None]
_instance_lock = threading.Lock()


def get_model_manager() -> ModelManager:
    """Get the shared application-level ModelManager instance."""
    if _default_instance_holder[0] is None:
        with _instance_lock:
            if _default_instance_holder[0] is None:
                _default_instance_holder[0] = ModelManager()
    assert _default_instance_holder[0] is not None
    return _default_instance_holder[0]


def reset_model_manager() -> None:
    """Reset the shared instance (for testing)."""
    with _instance_lock:
        if _default_instance_holder[0] is not None:
            _default_instance_holder[0].clear_models()
        _default_instance_holder[0] = None
