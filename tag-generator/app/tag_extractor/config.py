"""Configuration models for tag extraction and ML model management."""

import os

import structlog
from pydantic import BaseModel, Field, model_validator

logger = structlog.get_logger(__name__)


class TagExtractionConfig(BaseModel):
    """Configuration for tag extraction behaviour.

    Controls model selection, scoring thresholds, language-specific settings,
    and ONNX runtime options.
    """

    model_name: str = "paraphrase-multilingual-MiniLM-L12-v2"
    device: str = "cpu"
    top_keywords: int = Field(default=10, gt=0)
    min_score_threshold: float = Field(
        default=0.15, ge=0.0, le=1.0, description="Lower threshold for better extraction"
    )
    keyphrase_ngram_range: tuple[int, int] = (1, 3)
    use_mmr: bool = True
    diversity: float = Field(default=0.5, ge=0.0, le=1.0)
    min_token_length: int = Field(default=2, gt=0)
    min_text_length: int = Field(default=10, gt=0)
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
    onnx_model_path: str | None = None
    onnx_tokenizer_name: str = "sentence-transformers/paraphrase-multilingual-MiniLM-L12-v2"
    onnx_pooling: str = "cls"
    onnx_batch_size: int = Field(default=16, gt=0)
    onnx_max_length: int = Field(default=256, gt=0)
    use_fp16: bool = Field(
        default=False, description="Enable FP16 for ~50% memory reduction (set via TAG_USE_FP16=true)"
    )
    use_japanese_semantic: bool = Field(
        default=True,
        description="Enable semantic scoring for Japanese text using KeyBERT",
    )
    japanese_mmr_diversity: float = Field(
        default=0.5, ge=0.0, le=1.0, description="MMR diversity for Japanese KeyBERT scoring"
    )
    max_tag_length: int = Field(default=15, gt=0, description="Maximum tag length for quality filtering")

    @model_validator(mode="after")
    def resolve_env_overrides(self) -> "TagExtractionConfig":
        """Resolve environment variable overrides and auto-detect ONNX availability."""
        # Default ONNX path from env
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

        # Enable FP16 via environment variable
        if os.getenv("TAG_USE_FP16", "").lower() in ("true", "1", "yes"):
            self.use_fp16 = True
            logger.info("FP16 mode enabled via TAG_USE_FP16 environment variable")

        return self


class ModelConfig(BaseModel):
    """Configuration for ML model loading."""

    model_name: str = "paraphrase-multilingual-MiniLM-L12-v2"
    device: str = "cpu"
    use_onnx: bool = False
    onnx_model_path: str | None = None
    onnx_tokenizer_name: str = "sentence-transformers/paraphrase-multilingual-MiniLM-L12-v2"
    onnx_pooling: str = "cls"
    onnx_batch_size: int = Field(default=16, gt=0)
    onnx_max_length: int = Field(default=256, gt=0)
    use_fp16: bool = Field(default=False, description="Enable FP16 for ~50% memory reduction (GPU recommended)")
    use_ginza: bool = False
    ginza_model_name: str = "ja_ginza"
