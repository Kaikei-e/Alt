"""Unit tests for OnnxEmbeddingModel, TagExtractor._truncate_content, and fallback_japanese."""

from __future__ import annotations

import math
import os
from typing import Any
from unittest.mock import MagicMock, patch

import pytest

from tag_extractor.config import TagExtractionConfig
from tag_extractor.extract import TagExtractor
from tag_extractor.japanese_extractor import fallback_japanese
from tag_extractor.onnx_embedder import (
    OnnxEmbeddingConfig,
    OnnxEmbeddingModel,
    OnnxRuntimeMissing,
)


class _FakeArray:
    """Lightweight tensor stub supporting the numpy operations used by OnnxEmbeddingModel."""

    def __init__(
        self,
        data: list[Any],
        shape: tuple[int, ...],
        dtype: str = "float32",
    ) -> None:
        self._data = data
        self.shape = shape
        self.dtype = dtype

    @property
    def size(self) -> int:
        return math.prod(self.shape)

    def tolist(self) -> list[Any]:
        return self._data

    def __getitem__(self, key: Any) -> _FakeArray:
        # Supports hidden_states[:, 0, :] for CLS pooling on (batch, seq, hidden_dim)
        if isinstance(key, tuple) and len(key) == 3 and key[1] == 0:
            cls_rows = [seq_rows[0] for seq_rows in self._data]
            return _FakeArray(
                cls_rows,
                shape=(len(cls_rows), self.shape[2]),
                dtype=self.dtype,
            )
        raise IndexError(f"Unsupported slice key: {key!r}")

    def mean(self, axis: int = 1) -> _FakeArray:
        if axis != 1:
            raise ValueError(f"Unsupported axis: {axis}")
        dim = self.shape[2]
        averaged: list[list[float]] = []
        for seq_rows in self._data:
            seq_len = len(seq_rows)
            averaged.append([sum(row[d] for row in seq_rows) / seq_len for d in range(dim)])
        return _FakeArray(
            averaged,
            shape=(len(averaged), dim),
            dtype=self.dtype,
        )


class _FakeNumpy:
    """Minimal numpy module replacement for environments without the optional ML group."""

    float32 = "float32"

    @staticmethod
    def zeros(shape: tuple[int, ...], dtype: str = "float32") -> _FakeArray:
        return _FakeArray([], shape=shape, dtype=dtype)

    @staticmethod
    def vstack(arrays: list[_FakeArray]) -> _FakeArray:
        rows = [row for arr in arrays for row in arr.tolist()]
        dim = arrays[0].shape[1] if arrays else 0
        dtype = arrays[0].dtype if arrays else "float32"
        return _FakeArray(rows, shape=(len(rows), dim), dtype=dtype)


def _build_mock_ort_and_tokenizer(
    hidden_states: _FakeArray,
    available_providers: list[str] | None = None,
    hidden_dim: int = 4,
) -> tuple[_FakeNumpy, MagicMock, MagicMock, MagicMock]:
    """Create mocked numpy, onnxruntime, InferenceSession, and AutoTokenizer."""
    fake_np = _FakeNumpy()

    mock_ort = MagicMock()
    mock_ort.get_available_providers.return_value = (
        available_providers if available_providers is not None else ["CPUExecutionProvider"]
    )
    mock_ort.GraphOptimizationLevel.ORT_ENABLE_ALL = "ORT_ENABLE_ALL"
    mock_ort.ExecutionMode.ORT_SEQUENTIAL = "ORT_SEQUENTIAL"
    mock_ort.ExecutionMode.ORT_PARALLEL = "ORT_PARALLEL"

    mock_session = MagicMock()
    mock_output_meta = MagicMock()
    mock_output_meta.shape = [None, None, hidden_dim]
    mock_session.get_outputs.return_value = [mock_output_meta]
    mock_session.run.return_value = [hidden_states]
    mock_ort.InferenceSession.return_value = mock_session

    mock_tokenizer_cls = MagicMock()
    mock_tokenizer_instance = MagicMock()

    def _tokenize(
        batch: list[str],
        padding: bool = True,
        truncation: bool = True,
        max_length: int = 256,
        return_tensors: str = "np",
    ) -> dict[str, _FakeArray]:
        batch_len = len(batch)
        seq_len = hidden_states.shape[1] if len(hidden_states.shape) == 3 else 3
        ones = [[1] * seq_len for _ in range(batch_len)]
        return {
            "input_ids": _FakeArray(ones, shape=(batch_len, seq_len), dtype="int64"),
            "attention_mask": _FakeArray(ones, shape=(batch_len, seq_len), dtype="int64"),
        }

    mock_tokenizer_instance.side_effect = _tokenize
    mock_tokenizer_cls.from_pretrained.return_value = mock_tokenizer_instance

    return fake_np, mock_ort, mock_session, mock_tokenizer_cls


class TestOnnxEmbeddingModel:
    """Specifications for OnnxEmbeddingModel in tag_extractor/onnx_embedder.py."""

    def test_raises_value_error_on_invalid_pooling(self) -> None:
        """Initializing with pooling other than 'cls' or 'mean' raises ValueError."""
        hidden = _FakeArray([[[0.0] * 4] * 3], shape=(1, 3, 4))
        fake_np, mock_ort, _, mock_tokenizer_cls = _build_mock_ort_and_tokenizer(hidden)

        config = OnnxEmbeddingConfig(
            model_path="/models/onnx/model.onnx",
            tokenizer_name="dummy-tokenizer",
            pooling="invalid",
        )
        with (
            patch("tag_extractor.onnx_embedder.np", fake_np),
            patch("tag_extractor.onnx_embedder.ort", mock_ort),
            patch("tag_extractor.onnx_embedder.AutoTokenizer", mock_tokenizer_cls),
            pytest.raises(ValueError, match="Pooling must be 'cls' or 'mean'"),
        ):
            OnnxEmbeddingModel(config)

    def test_raises_onnx_runtime_missing_when_dependencies_absent(self) -> None:
        """Raises OnnxRuntimeMissing when ort or AutoTokenizer is None."""
        config = OnnxEmbeddingConfig(
            model_path="/models/onnx/model.onnx",
            tokenizer_name="dummy-tokenizer",
        )
        with (
            patch("tag_extractor.onnx_embedder.ort", None),
            pytest.raises(OnnxRuntimeMissing),
        ):
            OnnxEmbeddingModel(config)

    def test_encode_empty_list_returns_zero_rows_float32_array(self) -> None:
        """encode([]) returns an empty float32 array with shape (0, hidden_dim)."""
        hidden_dim = 6
        hidden = _FakeArray([[[0.0] * hidden_dim] * 3], shape=(1, 3, hidden_dim))
        fake_np, mock_ort, _, mock_tokenizer_cls = _build_mock_ort_and_tokenizer(hidden, hidden_dim=hidden_dim)
        config = OnnxEmbeddingConfig(
            model_path="/models/onnx/model.onnx",
            tokenizer_name="dummy-tokenizer",
            pooling="cls",
        )
        with (
            patch("tag_extractor.onnx_embedder.np", fake_np),
            patch("tag_extractor.onnx_embedder.ort", mock_ort),
            patch("tag_extractor.onnx_embedder.AutoTokenizer", mock_tokenizer_cls),
        ):
            model = OnnxEmbeddingModel(config)
            result = model.encode([])

        assert result.shape == (0, hidden_dim)
        assert result.dtype == fake_np.float32

    def test_encode_cls_and_mean_pooling(self) -> None:
        """encode() pools first token for 'cls' and averages across sequence for 'mean'."""
        # Shape: (batch=2, seq=3, hidden_dim=4)
        hidden = _FakeArray(
            [
                [
                    [1.0, 2.0, 3.0, 4.0],
                    [4.0, 5.0, 6.0, 7.0],
                    [7.0, 8.0, 9.0, 10.0],
                ],
                [
                    [2.0, 0.0, 2.0, 0.0],
                    [4.0, 2.0, 4.0, 2.0],
                    [6.0, 4.0, 6.0, 4.0],
                ],
            ],
            shape=(2, 3, 4),
            dtype="float32",
        )
        fake_np, mock_ort, _, mock_tokenizer_cls = _build_mock_ort_and_tokenizer(hidden, hidden_dim=4)

        cls_config = OnnxEmbeddingConfig(
            model_path="/models/onnx/model.onnx",
            tokenizer_name="dummy-tokenizer",
            pooling="cls",
            batch_size=4,
        )
        mean_config = OnnxEmbeddingConfig(
            model_path="/models/onnx/model.onnx",
            tokenizer_name="dummy-tokenizer",
            pooling="mean",
            batch_size=4,
        )

        with (
            patch("tag_extractor.onnx_embedder.np", fake_np),
            patch("tag_extractor.onnx_embedder.ort", mock_ort),
            patch("tag_extractor.onnx_embedder.AutoTokenizer", mock_tokenizer_cls),
        ):
            cls_model = OnnxEmbeddingModel(cls_config)
            cls_embeddings = cls_model.encode(["first text", "second text"])

            mean_model = OnnxEmbeddingModel(mean_config)
            mean_embeddings = mean_model.encode(["first text", "second text"])

        assert cls_embeddings.shape == (2, 4)
        assert cls_embeddings.tolist() == [
            [1.0, 2.0, 3.0, 4.0],
            [2.0, 0.0, 2.0, 0.0],
        ]

        assert mean_embeddings.shape == (2, 4)
        assert mean_embeddings.tolist() == [
            [4.0, 5.0, 6.0, 7.0],
            [4.0, 2.0, 4.0, 2.0],
        ]

    def test_effective_batch_size_respects_runtime_override_and_max_tokens(self) -> None:
        """_effective_batch_size applies TAG_ONNX_RUNTIME_BATCH_SIZE and clamps by TAG_ONNX_MAX_BATCH_TOKENS."""
        hidden = _FakeArray([[[0.0] * 4] * 3], shape=(1, 3, 4))
        fake_np, mock_ort, _, mock_tokenizer_cls = _build_mock_ort_and_tokenizer(hidden)

        config = OnnxEmbeddingConfig(
            model_path="/models/onnx/model.onnx",
            tokenizer_name="dummy-tokenizer",
            batch_size=16,
            max_length=256,
        )

        # Case 1: TAG_ONNX_RUNTIME_BATCH_SIZE overrides requested batch size
        with (
            patch.dict(
                os.environ,
                {"TAG_ONNX_RUNTIME_BATCH_SIZE": "8"},
                clear=False,
            ),
            patch("tag_extractor.onnx_embedder.np", fake_np),
            patch("tag_extractor.onnx_embedder.ort", mock_ort),
            patch("tag_extractor.onnx_embedder.AutoTokenizer", mock_tokenizer_cls),
        ):
            os.environ.pop("TAG_ONNX_MAX_BATCH_TOKENS", None)
            model_override = OnnxEmbeddingModel(config)
            assert model_override._effective_batch_size(16) == 8  # noqa: SLF001

        # Case 2: TAG_ONNX_MAX_BATCH_TOKENS clamps batch size based on max_length (512 // 256 == 2)
        with (
            patch.dict(
                os.environ,
                {
                    "TAG_ONNX_RUNTIME_BATCH_SIZE": "12",
                    "TAG_ONNX_MAX_BATCH_TOKENS": "512",
                },
                clear=False,
            ),
            patch("tag_extractor.onnx_embedder.np", fake_np),
            patch("tag_extractor.onnx_embedder.ort", mock_ort),
            patch("tag_extractor.onnx_embedder.AutoTokenizer", mock_tokenizer_cls),
        ):
            model_clamped = OnnxEmbeddingModel(config)
            assert model_clamped._effective_batch_size(16) == 2  # noqa: SLF001

        # Case 3: Very small TAG_ONNX_MAX_BATCH_TOKENS still clamps to at least 1
        with (
            patch.dict(
                os.environ,
                {"TAG_ONNX_MAX_BATCH_TOKENS": "64"},
                clear=False,
            ),
            patch("tag_extractor.onnx_embedder.np", fake_np),
            patch("tag_extractor.onnx_embedder.ort", mock_ort),
            patch("tag_extractor.onnx_embedder.AutoTokenizer", mock_tokenizer_cls),
        ):
            os.environ.pop("TAG_ONNX_RUNTIME_BATCH_SIZE", None)
            model_min_one = OnnxEmbeddingModel(config)
            assert model_min_one._effective_batch_size(16) == 1  # noqa: SLF001

    def test_resolve_providers_falls_back_to_cpu_when_requested_unavailable(
        self,
    ) -> None:
        """_resolve_providers falls back to ['CPUExecutionProvider'] when configured providers do not match."""
        hidden = _FakeArray([[[0.0] * 4] * 3], shape=(1, 3, 4))
        fake_np, mock_ort, _, mock_tokenizer_cls = _build_mock_ort_and_tokenizer(
            hidden, available_providers=["CPUExecutionProvider"]
        )
        config = OnnxEmbeddingConfig(
            model_path="/models/onnx/model.onnx",
            tokenizer_name="dummy-tokenizer",
        )

        with (
            patch.dict(
                os.environ,
                {"TAG_ONNX_RUNTIME_PROVIDERS": "CUDAExecutionProvider,TensorRTExecutionProvider"},
                clear=False,
            ),
            patch("tag_extractor.onnx_embedder.np", fake_np),
            patch("tag_extractor.onnx_embedder.ort", mock_ort),
            patch("tag_extractor.onnx_embedder.AutoTokenizer", mock_tokenizer_cls),
        ):
            model = OnnxEmbeddingModel(config)
            assert model._providers == ["CPUExecutionProvider"]  # noqa: SLF001
            assert model.describe()["providers"] == ["CPUExecutionProvider"]


class TestTagExtractorTruncateContent:
    """Specifications for TagExtractor._truncate_content in tag_extractor/extract.py."""

    @pytest.mark.parametrize("punct", [".", "!", "?"])
    def test_truncates_at_sentence_boundary_after_80_percent_threshold(self, punct: str) -> None:
        """Truncates at last sentence end (+1) when '.', '!', or '?' appears past 80% of max_content_length."""
        extractor = TagExtractor(
            config=TagExtractionConfig(use_onnx_runtime=False),
        )
        max_len = 100
        # Place sentence boundary at index 85 (> 80% of 100)
        prefix = "a" * 85
        content = prefix + punct + ("b" * 30)
        assert len(content) == 116

        truncated_title, truncated_content = extractor._truncate_content(  # noqa: SLF001
            "Sample Title", content, max_len
        )

        assert truncated_title == "Sample Title"
        assert len(truncated_content) == 86
        assert truncated_content == prefix + punct

    def test_truncates_at_exact_max_content_length_when_no_boundary_past_80_percent(
        self,
    ) -> None:
        """Truncates at exact max_content_length when no sentence boundary exists past 80%."""
        extractor = TagExtractor(
            config=TagExtractionConfig(use_onnx_runtime=False),
        )
        max_len = 100
        # Sentence boundary at index 50 (<= 80% of 100), followed by plain characters
        content = ("a" * 50) + "." + ("b" * 80)
        assert len(content) == 131

        _, truncated_content = extractor._truncate_content(  # noqa: SLF001
            "Title", content, max_len
        )

        assert len(truncated_content) == max_len
        assert truncated_content == content[:max_len]

    def test_truncates_title_exceeding_max_title_length(self) -> None:
        """Truncates title to 1000 characters when it exceeds max_title_length."""
        extractor = TagExtractor(
            config=TagExtractionConfig(use_onnx_runtime=False),
        )
        long_title = "T" * 1250
        short_content = "Short content body."

        truncated_title, truncated_content = extractor._truncate_content(  # noqa: SLF001
            long_title, short_content, 500
        )

        assert len(truncated_title) == 1000
        assert truncated_title == "T" * 1000
        assert truncated_content == short_content


def _make_token(surface: str, pos1: str = "名詞", pos2: str = "一般") -> Any:
    """Create a mock Fugashi token with surface and feature.pos1/pos2 attributes."""
    token = MagicMock()
    token.surface = surface
    token.feature.pos1 = pos1
    token.feature.pos2 = pos2
    return token


class TestFallbackJapanese:
    """Specifications for fallback_japanese in tag_extractor/japanese_extractor.py."""

    def test_extracts_valid_nouns_by_frequency_excluding_stopwords_and_short_words(
        self,
    ) -> None:
        """Extracts nouns (pos1=='名詞', len>=2, not in stopwords) ordered by frequency up to top_keywords."""
        # Separate nouns with particles ('を', 'は') so they are evaluated as single nouns
        # rather than chained into compound words.
        tokens = [
            # "検索" appears 4 times
            _make_token("検索", pos1="名詞"),
            _make_token("を", pos1="助詞"),
            _make_token("検索", pos1="名詞"),
            _make_token("を", pos1="助詞"),
            _make_token("検索", pos1="名詞"),
            _make_token("を", pos1="助詞"),
            _make_token("検索", pos1="名詞"),
            _make_token("は", pos1="助詞"),
            # "モデル" appears 3 times
            _make_token("モデル", pos1="名詞"),
            _make_token("を", pos1="助詞"),
            _make_token("モデル", pos1="名詞"),
            _make_token("を", pos1="助詞"),
            _make_token("モデル", pos1="名詞"),
            _make_token("は", pos1="助詞"),
            # "学習" appears 2 times
            _make_token("学習", pos1="名詞"),
            _make_token("を", pos1="助詞"),
            _make_token("学習", pos1="名詞"),
            _make_token("は", pos1="助詞"),
            # Ignored tokens: single-char noun, stopword noun, and non-noun verb
            _make_token("木", pos1="名詞"),
            _make_token("を", pos1="助詞"),
            _make_token("木", pos1="名詞"),
            _make_token("は", pos1="助詞"),
            _make_token("これ", pos1="名詞"),
            _make_token("を", pos1="助詞"),
            _make_token("これ", pos1="名詞"),
            _make_token("は", pos1="助詞"),
            _make_token("走る", pos1="動詞"),
            _make_token("走る", pos1="動詞"),
        ]

        mock_tagger = MagicMock(side_effect=lambda _text: list(tokens))
        ja_stopwords = {"これ", "それ", "ため"}
        config = TagExtractionConfig(top_keywords=2, use_onnx_runtime=False)

        result = fallback_japanese("テスト", mock_tagger, ja_stopwords, config)

        assert result == ["検索", "モデル"]
        assert "木" not in result
        assert "これ" not in result
        assert "走る" not in result

    def test_returns_empty_list_when_tagger_is_none_or_raises_during_extraction(
        self,
    ) -> None:
        """Returns [] when ja_tagger is None or when analysis raises an exception in TagExtractor fallback."""
        config = TagExtractionConfig(top_keywords=5, use_onnx_runtime=False)

        # Direct call with ja_tagger=None returns []
        assert fallback_japanese("テスト文章", None, set(), config) == []

        # When ja_tagger raises an exception during analysis, TagExtractor's fallback handling returns []
        failing_tagger = MagicMock(side_effect=RuntimeError("MeCab dictionary corrupted"))
        extractor = TagExtractor(config=config)
        extractor._models_loaded = True  # noqa: SLF001
        extractor._ja_tagger = failing_tagger  # noqa: SLF001
        extractor._ja_stopwords = set()  # noqa: SLF001
        extractor._stopwords_loaded = True  # type: ignore[attr-defined]  # noqa: SLF001

        tags, confidences = extractor._run_extraction("日本語のテスト記事本文です", "ja")  # noqa: SLF001
        assert tags == []
        assert confidences == {}
