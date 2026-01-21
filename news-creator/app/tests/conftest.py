"""
Shared pytest fixtures for news-creator tests.

TDD Best Practices:
- Use dependency injection to make code testable
- Mock external services (Ollama) at the port boundary
- Use FastAPI's dependency_overrides for integration tests
"""

import pytest
from unittest.mock import AsyncMock, Mock
from fastapi.testclient import TestClient

from news_creator.config.config import NewsCreatorConfig
from news_creator.domain.models import LLMGenerateResponse
from news_creator.port.llm_provider_port import LLMProviderPort


@pytest.fixture
def mock_config() -> Mock:
    """Create a mock NewsCreatorConfig for unit testing.

    This fixture provides a complete mock configuration that can be used
    across different test modules. Modify specific values in individual tests
    as needed.
    """
    config = Mock(spec=NewsCreatorConfig)
    config.llm_service_url = "http://localhost:11435"
    config.model_name = "gemma3-4b-12k"
    config.llm_timeout_seconds = 60
    config.llm_keep_alive = -1
    config.ollama_request_concurrency = 1
    config.oom_detection_enabled = False
    config.model_routing_enabled = False
    config.llm_num_ctx = 12288
    config.summary_num_predict = 1200
    config.llm_temperature = 0.25
    config.max_repetition_retries = 2
    config.llm_repeat_penalty = 1.1
    config.repetition_threshold = 2.0
    config.hierarchical_threshold_chars = 100000
    config.hierarchical_threshold_clusters = 50
    config.hierarchical_chunk_max_chars = 20000

    # Mock methods
    config.is_base_model_name = Mock(return_value=False)
    config.is_bucket_model_name = Mock(return_value=False)
    config.get_keep_alive_for_model = Mock(return_value=-1)
    config.get_llm_options = Mock(return_value={
        "num_ctx": 12288,
        "num_predict": 1200,
        "temperature": 0.25,
    })

    return config


@pytest.fixture
def mock_llm_provider() -> AsyncMock:
    """Create a mock LLM provider implementing LLMProviderPort.

    This is the primary fixture for testing usecases and handlers
    without requiring an actual Ollama server.

    Usage:
        def test_summarize(mock_llm_provider):
            mock_llm_provider.generate.return_value = LLMGenerateResponse(
                response='{"title": "Test", "bullets": ["Point 1"]}',
                model="gemma3-4b-12k",
            )
            # ... test code
    """
    provider = AsyncMock(spec=LLMProviderPort)
    provider.initialize = AsyncMock()
    provider.cleanup = AsyncMock()
    provider.generate = AsyncMock(return_value=LLMGenerateResponse(
        response="Default mock response",
        model="gemma3-4b-12k",
        done=True,
        prompt_eval_count=100,
        eval_count=50,
        total_duration=1_000_000_000,
    ))
    return provider


@pytest.fixture
def mock_llm_response_success() -> LLMGenerateResponse:
    """Create a successful LLM response for recap summary tests."""
    import json
    return LLMGenerateResponse(
        response=json.dumps({
            "title": "Test Summary Title",
            "bullets": [
                "First important point from the articles.",
                "Second key finding or development.",
                "Third conclusion or insight.",
            ],
            "language": "en"
        }),
        model="gemma3-4b-12k",
        done=True,
        prompt_eval_count=512,
        eval_count=256,
        total_duration=2_000_000_000,
    )


@pytest.fixture
def sample_recap_request():
    """Create a sample RecapSummaryRequest for testing."""
    from uuid import uuid4
    from news_creator.domain.models import (
        RecapSummaryRequest,
        RecapClusterInput,
        RepresentativeSentence,
        RecapSummaryOptions,
    )

    return RecapSummaryRequest(
        job_id=uuid4(),
        genre="tech",
        clusters=[
            RecapClusterInput(
                cluster_id=0,
                representative_sentences=[
                    RepresentativeSentence(text="AI company announces new product."),
                    RepresentativeSentence(text="Market responds positively to announcement."),
                ],
                top_terms=["AI", "product", "market"],
            ),
            RecapClusterInput(
                cluster_id=1,
                representative_sentences=[
                    RepresentativeSentence(text="Regulatory concerns raised by experts."),
                ],
                top_terms=["regulation", "concerns"],
            ),
        ],
        options=RecapSummaryOptions(max_bullets=5, temperature=0.3),
    )


# Integration test fixtures

@pytest.fixture
def test_app(mock_llm_provider):
    """Create a test FastAPI application with mocked dependencies.

    This fixture overrides the OllamaGateway with a mock provider,
    allowing integration tests without an actual Ollama server.

    Usage:
        def test_endpoint(test_app):
            with TestClient(test_app) as client:
                response = client.get("/health")
                assert response.status_code == 200
    """
    from main import app, container

    # Store original gateway
    original_gateway = container.ollama_gateway

    # Replace with mock
    container.ollama_gateway = mock_llm_provider

    # Also update usecases that depend on the gateway
    container.summarize_usecase.llm_provider = mock_llm_provider
    container.recap_summary_usecase.llm_provider = mock_llm_provider
    container.expand_query_usecase.llm_provider = mock_llm_provider

    yield app

    # Restore original
    container.ollama_gateway = original_gateway


@pytest.fixture
def test_client(test_app):
    """Create a TestClient with mocked dependencies.

    Usage:
        def test_health_endpoint(test_client):
            response = test_client.get("/health")
            assert response.status_code == 200
    """
    with TestClient(test_app) as client:
        yield client
