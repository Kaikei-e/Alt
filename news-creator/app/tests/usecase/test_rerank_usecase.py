"""Tests for RerankUsecase."""

import pytest
from unittest.mock import patch, MagicMock
import numpy as np

from news_creator.usecase.rerank_usecase import RerankUsecase


class TestRerankUsecase:
    """Test suite for RerankUsecase."""

    @pytest.fixture
    def mock_cross_encoder(self):
        """Create a mock CrossEncoder."""
        mock = MagicMock()
        # Return descending scores: [0.9, 0.7, 0.5] -> indices [0, 1, 2]
        mock.predict.return_value = np.array([0.9, 0.7, 0.5])
        return mock

    @pytest.fixture
    def usecase(self):
        """Create RerankUsecase instance."""
        return RerankUsecase(model_name="test-model")

    @pytest.mark.asyncio
    async def test_rerank_success(self, usecase, mock_cross_encoder):
        """Test successful re-ranking."""
        with patch(
            "news_creator.usecase.rerank_usecase._get_cross_encoder",
            return_value=mock_cross_encoder
        ):
            results, model, time_ms = await usecase.rerank(
                query="test query",
                candidates=["candidate 1", "candidate 2", "candidate 3"]
            )

        # Results should be sorted by score descending
        assert len(results) == 3
        assert results[0] == (0, 0.9)  # Highest score
        assert results[1] == (1, 0.7)
        assert results[2] == (2, 0.5)  # Lowest score

        assert model == "test-model"
        assert time_ms is not None

        # Verify CrossEncoder was called with correct pairs
        mock_cross_encoder.predict.assert_called_once()
        call_args = mock_cross_encoder.predict.call_args[0][0]
        assert len(call_args) == 3
        assert call_args[0] == ("test query", "candidate 1")
        assert call_args[1] == ("test query", "candidate 2")
        assert call_args[2] == ("test query", "candidate 3")

    @pytest.mark.asyncio
    async def test_rerank_with_top_k(self, usecase, mock_cross_encoder):
        """Test re-ranking with top_k limit."""
        with patch(
            "news_creator.usecase.rerank_usecase._get_cross_encoder",
            return_value=mock_cross_encoder
        ):
            results, model, time_ms = await usecase.rerank(
                query="test query",
                candidates=["candidate 1", "candidate 2", "candidate 3"],
                top_k=2
            )

        # Only top 2 results should be returned
        assert len(results) == 2
        assert results[0] == (0, 0.9)
        assert results[1] == (1, 0.7)

    @pytest.mark.asyncio
    async def test_rerank_empty_query(self, usecase):
        """Test re-ranking with empty query raises ValueError."""
        with pytest.raises(ValueError, match="query cannot be empty"):
            await usecase.rerank(query="", candidates=["candidate"])

    @pytest.mark.asyncio
    async def test_rerank_empty_candidates(self, usecase):
        """Test re-ranking with empty candidates raises ValueError."""
        with pytest.raises(ValueError, match="candidates list cannot be empty"):
            await usecase.rerank(query="test query", candidates=[])

    @pytest.mark.asyncio
    async def test_rerank_model_error(self, usecase):
        """Test re-ranking handles model errors gracefully."""
        mock_encoder = MagicMock()
        mock_encoder.predict.side_effect = RuntimeError("Model failed")

        with patch(
            "news_creator.usecase.rerank_usecase._get_cross_encoder",
            return_value=mock_encoder
        ):
            with pytest.raises(RuntimeError, match="Re-ranking failed"):
                await usecase.rerank(
                    query="test query",
                    candidates=["candidate 1"]
                )

    @pytest.mark.asyncio
    async def test_rerank_scores_sorted_correctly(self, usecase):
        """Test that results are correctly sorted by score descending."""
        mock_encoder = MagicMock()
        # Return scores in non-sorted order: middle score highest
        mock_encoder.predict.return_value = np.array([0.3, 0.9, 0.1, 0.7])

        with patch(
            "news_creator.usecase.rerank_usecase._get_cross_encoder",
            return_value=mock_encoder
        ):
            results, _, _ = await usecase.rerank(
                query="test query",
                candidates=["a", "b", "c", "d"]
            )

        # Results should be sorted: b(0.9), d(0.7), a(0.3), c(0.1)
        assert results[0] == (1, 0.9)  # "b" had highest score
        assert results[1] == (3, 0.7)  # "d"
        assert results[2] == (0, 0.3)  # "a"
        assert results[3] == (2, 0.1)  # "c"

    def test_default_model(self):
        """Test default model is set correctly."""
        usecase = RerankUsecase()
        assert usecase.model_name == "BAAI/bge-reranker-v2-m3"

    def test_custom_model(self):
        """Test custom model is accepted."""
        usecase = RerankUsecase(model_name="custom/model")
        assert usecase.model_name == "custom/model"
