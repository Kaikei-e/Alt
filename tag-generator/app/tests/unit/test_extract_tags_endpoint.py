"""Tests for the POST /api/v1/extract-tags endpoint.

Service-to-service endpoint for extracting semantic tags from arbitrary text.
Used by recap-worker to tag recap genre outputs.
"""

import os
from unittest.mock import MagicMock, patch

import pytest
from fastapi.testclient import TestClient


@pytest.fixture()
def client():
    """Create a test client with mocked background service."""
    from auth_service import app

    return TestClient(app)


@pytest.fixture()
def mock_tag_extractor():
    """Mock the background tag service's tag extractor."""
    from tag_extractor.extract import TagExtractionOutcome

    mock_outcome = TagExtractionOutcome(
        tags=["artificial-intelligence", "large-language-model", "gpu-computing"],
        confidence=0.85,
        tag_count=3,
        inference_ms=150.0,
        language="en",
        model_name="paraphrase-multilingual-MiniLM-L12-v2",
        sanitized_length=500,
        tag_confidences={
            "artificial-intelligence": 0.92,
            "large-language-model": 0.88,
            "gpu-computing": 0.75,
        },
        embedding_backend="onnxruntime",
    )

    mock_extractor = MagicMock()
    mock_extractor.extract_tags_with_metrics.return_value = mock_outcome
    return mock_extractor


class TestExtractTagsEndpoint:
    """Tests for POST /api/v1/extract-tags."""

    def test_success_returns_tags(self, client, mock_tag_extractor):
        """Valid request returns extracted tags."""
        with patch("auth_service._background_tag_service") as mock_service:
            mock_service.tag_extractor = mock_tag_extractor

            resp = client.post(
                "/api/v1/extract-tags",
                json={"title": "Technology", "content": "AI and LLM are transforming computing with GPU acceleration."},
            )

        assert resp.status_code == 200
        body = resp.json()
        assert body["success"] is True
        assert "artificial-intelligence" in body["tags"]
        assert "large-language-model" in body["tags"]
        assert isinstance(body["confidence"], float)
        mock_tag_extractor.extract_tags_with_metrics.assert_called_once_with(
            "Technology",
            "AI and LLM are transforming computing with GPU acceleration.",
        )

    def test_empty_content_returns_empty_tags(self, client):
        """Empty content returns empty tag list."""
        from tag_extractor.extract import TagExtractionOutcome

        empty_outcome = TagExtractionOutcome(
            tags=[],
            confidence=0.0,
            tag_count=0,
            inference_ms=5.0,
            language="und",
            model_name="paraphrase-multilingual-MiniLM-L12-v2",
            sanitized_length=0,
            tag_confidences={},
            embedding_backend="onnxruntime",
        )

        mock_extractor = MagicMock()
        mock_extractor.extract_tags_with_metrics.return_value = empty_outcome

        with patch("auth_service._background_tag_service") as mock_service:
            mock_service.tag_extractor = mock_extractor

            resp = client.post(
                "/api/v1/extract-tags",
                json={"title": "Empty", "content": ""},
            )

        assert resp.status_code == 200
        body = resp.json()
        assert body["success"] is True
        assert body["tags"] == []

    def test_background_service_not_ready_returns_503(self, client):
        """When background service hasn't initialized yet, returns 503."""
        with patch("auth_service._background_tag_service", None):
            resp = client.post(
                "/api/v1/extract-tags",
                json={"title": "Test", "content": "Some content"},
            )

        assert resp.status_code == 503

    def test_extraction_error_returns_500(self, client):
        """When tag extraction fails, returns 500."""
        mock_extractor = MagicMock()
        mock_extractor.extract_tags_with_metrics.side_effect = RuntimeError("model failed")

        with patch("auth_service._background_tag_service") as mock_service:
            mock_service.tag_extractor = mock_extractor

            resp = client.post(
                "/api/v1/extract-tags",
                json={"title": "Test", "content": "Some content"},
            )

        assert resp.status_code == 500
