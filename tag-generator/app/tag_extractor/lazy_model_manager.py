"""
Lazy model loading manager for tag-generator service

This module provides efficient, on-demand model loading with caching
to reduce memory usage and startup time.
"""

import asyncio
import logging
import os
import time
from collections.abc import Callable
from dataclasses import dataclass
from enum import Enum
from pathlib import Path
from threading import Lock
from typing import Any

logger = logging.getLogger(__name__)


class ModelType(Enum):
    """Supported model types"""

    NLTK = "nltk"
    SENTENCE_TRANSFORMER = "sentence_transformer"
    UNIDIC = "unidic"
    FUGASHI = "fugashi"


@dataclass
class ModelInfo:
    """Model information and metadata"""

    model_type: ModelType
    name: str
    size_mb: float
    load_time_ms: int
    last_accessed: float
    access_count: int
    loaded: bool = False
    instance: Any | None = None


class LazyModelManager:
    """
    Lazy model loading manager with caching and memory optimization
    """

    def __init__(self, cache_dir: str = "/models", max_memory_mb: int = 1024):
        self.cache_dir = Path(cache_dir)
        self.max_memory_mb = max_memory_mb
        self.models: dict[str, ModelInfo] = {}
        self._locks: dict[str, Lock] = {}
        self._global_lock = Lock()
        self._total_memory_mb: float = 0.0

        # Create cache directory
        self.cache_dir.mkdir(parents=True, exist_ok=True)

        # Set up environment variables for model caching
        self._setup_environment()

    def _setup_environment(self):
        """Setup environment variables for model caching"""
        os.environ["NLTK_DATA"] = str(self.cache_dir / "nltk_data")
        os.environ["SENTENCE_TRANSFORMERS_HOME"] = str(self.cache_dir / "sentence_transformers")
        os.environ["TRANSFORMERS_CACHE"] = str(self.cache_dir / "transformers")
        os.environ["HF_HOME"] = str(self.cache_dir / "huggingface")

    def _get_model_lock(self, model_name: str) -> Lock:
        """Get or create a lock for a specific model"""
        with self._global_lock:
            if model_name not in self._locks:
                self._locks[model_name] = Lock()
            return self._locks[model_name]

    def _estimate_memory_usage(self, model_type: ModelType) -> float:
        """Estimate memory usage for different model types"""
        memory_estimates = {
            ModelType.NLTK: 50,  # MB
            ModelType.SENTENCE_TRANSFORMER: 400,  # MB
            ModelType.UNIDIC: 30,  # MB
            ModelType.FUGASHI: 20,  # MB
        }
        return memory_estimates.get(model_type, 100)

    def _should_evict_models(self, required_memory: float) -> bool:
        """Check if models should be evicted to free memory"""
        return (self._total_memory_mb + required_memory) > self.max_memory_mb

    def _evict_least_recently_used(self, required_memory: float):
        """Evict least recently used models to free memory"""
        # Sort models by last accessed time
        loaded_models = [(name, info) for name, info in self.models.items() if info.loaded]
        loaded_models.sort(key=lambda x: x[1].last_accessed)

        freed_memory: float = 0.0
        for name, info in loaded_models:
            if freed_memory >= required_memory:
                break

            logger.info(f"Evicting model {name} to free memory")
            info.instance = None
            info.loaded = False
            freed_memory += info.size_mb
            self._total_memory_mb -= info.size_mb

    async def get_nltk_stopwords(self) -> Any:
        """Get NLTK stopwords with lazy loading"""
        return await self._load_model("nltk_stopwords", ModelType.NLTK, self._load_nltk_stopwords)

    async def get_nltk_tokenizer(self) -> Any:
        """Get NLTK tokenizer with lazy loading"""
        return await self._load_model("nltk_tokenizer", ModelType.NLTK, self._load_nltk_tokenizer)

    async def get_sentence_transformer(self, model_name: str = "paraphrase-multilingual-MiniLM-L12-v2") -> Any:
        """Get SentenceTransformer with lazy loading"""
        return await self._load_model(
            f"sentence_transformer_{model_name}",
            ModelType.SENTENCE_TRANSFORMER,
            lambda: self._load_sentence_transformer(model_name),
        )

    async def get_fugashi_tagger(self) -> Any:
        """Get Fugashi tagger with lazy loading"""
        return await self._load_model("fugashi_tagger", ModelType.FUGASHI, self._load_fugashi_tagger)

    async def _load_model(self, model_name: str, model_type: ModelType, loader: Callable) -> Any:
        """Generic model loading with caching and memory management"""
        # Check if model is already loaded
        if model_name in self.models and self.models[model_name].loaded:
            model_info = self.models[model_name]
            model_info.last_accessed = time.time()
            model_info.access_count += 1
            return model_info.instance

        # Get model-specific lock
        model_lock = self._get_model_lock(model_name)

        with model_lock:
            # Double-check pattern
            if model_name in self.models and self.models[model_name].loaded:
                model_info = self.models[model_name]
                model_info.last_accessed = time.time()
                model_info.access_count += 1
                return model_info.instance

            # Estimate memory requirement
            estimated_memory = self._estimate_memory_usage(model_type)

            # Check if we need to evict models
            if self._should_evict_models(estimated_memory):
                self._evict_least_recently_used(estimated_memory)

            # Load the model
            logger.info(f"Loading model {model_name} of type {model_type.value}")
            start_time = time.time()

            try:
                model_instance = await asyncio.get_event_loop().run_in_executor(None, loader)

                load_time_ms = int((time.time() - start_time) * 1000)

                # Create or update model info
                model_info = ModelInfo(
                    model_type=model_type,
                    name=model_name,
                    size_mb=estimated_memory,
                    load_time_ms=load_time_ms,
                    last_accessed=time.time(),
                    access_count=1,
                    loaded=True,
                    instance=model_instance,
                )

                self.models[model_name] = model_info
                self._total_memory_mb += estimated_memory

                logger.info(f"Model {model_name} loaded successfully in {load_time_ms}ms")
                return model_instance

            except Exception as e:
                logger.error(f"Failed to load model {model_name}: {e}")
                raise

    def _load_nltk_stopwords(self) -> Any:
        """Load NLTK stopwords"""
        try:
            import nltk
            from nltk.corpus import stopwords

            # Download if not available
            try:
                stopwords.words("english")
            except LookupError:
                nltk.download("stopwords", quiet=True)

            return stopwords

        except ImportError:
            logger.error("NLTK not installed")
            raise

    def _load_nltk_tokenizer(self) -> Any:
        """Load NLTK tokenizer"""
        try:
            import nltk
            from nltk.tokenize import word_tokenize

            # Download if not available
            try:
                word_tokenize("test")
            except LookupError:
                nltk.download("punkt", quiet=True)
                nltk.download("punkt_tab", quiet=True)

            return word_tokenize

        except ImportError:
            logger.error("NLTK not installed")
            raise

    def _load_sentence_transformer(self, model_name: str) -> Any:
        """Load SentenceTransformer model"""
        try:
            from sentence_transformers import SentenceTransformer  # type: ignore

            # Load with CPU device for consistency
            return SentenceTransformer(model_name, device="cpu")

        except ImportError:
            logger.error("sentence-transformers not installed")
            raise

    def _load_fugashi_tagger(self) -> Any:
        """Load Fugashi tagger"""
        try:
            import fugashi

            # Create tagger with unidic-lite
            return fugashi.Tagger()  # type: ignore

        except ImportError:
            logger.error("fugashi not installed")
            raise
        except Exception as e:
            logger.error(f"Failed to create fugashi tagger: {e}")
            raise

    async def preload_models(self, model_names: list[str]):
        """Preload specified models for better performance"""
        logger.info(f"Preloading models: {model_names}")

        preload_tasks = []
        for model_name in model_names:
            if model_name == "nltk_stopwords":
                preload_tasks.append(self.get_nltk_stopwords())
            elif model_name == "nltk_tokenizer":
                preload_tasks.append(self.get_nltk_tokenizer())
            elif model_name.startswith("sentence_transformer"):
                preload_tasks.append(self.get_sentence_transformer())
            elif model_name == "fugashi_tagger":
                preload_tasks.append(self.get_fugashi_tagger())

        await asyncio.gather(*preload_tasks, return_exceptions=True)
        logger.info("Model preloading completed")

    def get_model_stats(self) -> dict[str, Any]:
        """Get statistics about loaded models"""
        stats = {
            "total_models": len(self.models),
            "loaded_models": len([m for m in self.models.values() if m.loaded]),
            "total_memory_mb": self._total_memory_mb,
            "memory_usage_percent": (self._total_memory_mb / self.max_memory_mb) * 100,
            "models": {},
        }

        for name, info in self.models.items():
            stats["models"][name] = {
                "type": info.model_type.value,
                "loaded": info.loaded,
                "size_mb": info.size_mb,
                "access_count": info.access_count,
                "load_time_ms": info.load_time_ms,
                "last_accessed": info.last_accessed,
            }

        return stats

    def cleanup(self):
        """Clean up loaded models"""
        logger.info("Cleaning up loaded models")
        for _name, info in self.models.items():
            if info.loaded:
                info.instance = None
                info.loaded = False
        self._total_memory_mb = 0


# Global instance
_model_manager: LazyModelManager | None = None


def get_model_manager() -> LazyModelManager:
    """Get global model manager instance"""
    global _model_manager
    if _model_manager is None:
        cache_dir = os.environ.get("MODELS_DIR", "/models")
        max_memory = int(os.environ.get("MAX_MODEL_MEMORY_MB", "1024"))
        _model_manager = LazyModelManager(cache_dir, max_memory)
    return _model_manager
